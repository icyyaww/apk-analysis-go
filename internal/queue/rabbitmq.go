package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// RabbitMQConfig RabbitMQ 配置
type RabbitMQConfig struct {
	Host      string
	Port      int
	User      string
	Password  string
	VHost     string
	Heartbeat time.Duration // 心跳间隔，默认 10 秒
}

// RabbitMQ RabbitMQ 客户端
type RabbitMQ struct {
	config        *RabbitMQConfig
	conn          *amqp.Connection
	channel       *amqp.Channel
	logger        *logrus.Logger
	queueName     string
	maxRetries    int
	prefetchCount int // 预取数量，应与 worker 数量匹配

	// 连接状态管理
	mu     sync.RWMutex
	closed bool

	// 重连通知 - 使用回调函数而非 channel，避免状态不一致
	onReconnect func()
}

// NewRabbitMQ 创建 RabbitMQ 客户端
func NewRabbitMQ(config *RabbitMQConfig, queueName string, logger *logrus.Logger) (*RabbitMQ, error) {
	return NewRabbitMQWithPrefetch(config, queueName, 1, logger)
}

// NewRabbitMQWithPrefetch 创建 RabbitMQ 客户端，支持自定义 prefetch count
// prefetchCount 应与 worker 数量匹配，以实现并行消费
func NewRabbitMQWithPrefetch(config *RabbitMQConfig, queueName string, prefetchCount int, logger *logrus.Logger) (*RabbitMQ, error) {
	if prefetchCount <= 0 {
		prefetchCount = 1
	}

	// 设置默认心跳间隔
	if config.Heartbeat == 0 {
		config.Heartbeat = 10 * time.Second
	}

	mq := &RabbitMQ{
		config:        config,
		logger:        logger,
		queueName:     queueName,
		maxRetries:    10,
		prefetchCount: prefetchCount,
		closed:        false,
	}

	if err := mq.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return mq, nil
}

// SetOnReconnect 设置重连回调函数
func (mq *RabbitMQ) SetOnReconnect(callback func()) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	mq.onReconnect = callback
}

// connect 建立连接
func (mq *RabbitMQ) connect() error {
	// 构建连接 URL
	url := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		mq.config.User,
		mq.config.Password,
		mq.config.Host,
		mq.config.Port,
		mq.config.VHost,
	)

	// 使用 DialConfig 配置心跳参数
	conn, err := amqp.DialConfig(url, amqp.Config{
		Heartbeat: mq.config.Heartbeat, // 心跳间隔（默认 10 秒）
		Locale:    "en_US",
	})
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	// 创建 Channel
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// 设置 QoS (预取数量) - 使用配置的 prefetchCount 以支持并行消费
	if err := ch.Qos(mq.prefetchCount, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// 声明队列
	_, err = ch.QueueDeclare(
		mq.queueName, // name
		true,         // durable (持久化)
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	mq.mu.Lock()
	mq.conn = conn
	mq.channel = ch
	mq.mu.Unlock()

	mq.logger.WithFields(logrus.Fields{
		"host":           mq.config.Host,
		"port":           mq.config.Port,
		"queue":          mq.queueName,
		"heartbeat":      mq.config.Heartbeat,
		"prefetch_count": mq.prefetchCount,
	}).Info("Connected to RabbitMQ")

	return nil
}

// StartConnectionWatcher 启动连接监听器
// 监听 Connection 关闭事件，触发自动重连
func (mq *RabbitMQ) StartConnectionWatcher(ctx context.Context) {
	go mq.watchConnection(ctx)
}

// watchConnection 监听连接状态
func (mq *RabbitMQ) watchConnection(ctx context.Context) {
	for {
		mq.mu.RLock()
		if mq.closed {
			mq.mu.RUnlock()
			mq.logger.Info("Connection watcher stopped: RabbitMQ client closed")
			return
		}
		conn := mq.conn
		mq.mu.RUnlock()

		if conn == nil {
			mq.logger.Warn("Connection is nil, waiting before retry...")
			time.Sleep(time.Second)
			continue
		}

		// 创建新的 NotifyClose channel 监听当前连接
		notifyClose := make(chan *amqp.Error, 1)
		conn.NotifyClose(notifyClose)

		select {
		case <-ctx.Done():
			mq.logger.Info("Connection watcher stopped by context")
			return

		case amqpErr, ok := <-notifyClose:
			// 检查是否主动关闭
			mq.mu.RLock()
			closed := mq.closed
			mq.mu.RUnlock()
			if closed {
				mq.logger.Info("Connection watcher stopped: client was closed")
				return
			}

			if !ok {
				mq.logger.Warn("NotifyClose channel closed unexpectedly")
			} else if amqpErr != nil {
				mq.logger.WithError(amqpErr).Error("RabbitMQ connection closed with error")
			} else {
				mq.logger.Warn("RabbitMQ connection closed gracefully")
			}

			// 执行重连
			mq.handleReconnect(ctx)
		}
	}
}

// handleReconnect 处理重连逻辑
func (mq *RabbitMQ) handleReconnect(ctx context.Context) {
	mq.logger.Info("Starting reconnection process...")

	// 1. 完全关闭旧连接
	mq.closeConnectionsUnsafe()

	// 2. 等待一小段时间确保资源释放
	time.Sleep(500 * time.Millisecond)

	// 3. 尝试重连（带指数退避）
	var lastErr error
	for attempt := 1; attempt <= mq.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			mq.logger.Info("Reconnection cancelled by context")
			return
		default:
		}

		mq.logger.WithField("attempt", fmt.Sprintf("%d/%d", attempt, mq.maxRetries)).
			Info("Attempting to reconnect to RabbitMQ")

		if err := mq.connect(); err != nil {
			lastErr = err
			mq.logger.WithError(err).Warn("Reconnection attempt failed")

			// 指数退避，最大等待 30 秒
			backoff := time.Duration(attempt) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			time.Sleep(backoff)
			continue
		}

		mq.logger.Info("Successfully reconnected to RabbitMQ")

		// 4. 触发重连回调（让 Consumer 重新启动）
		mq.mu.RLock()
		callback := mq.onReconnect
		mq.mu.RUnlock()

		if callback != nil {
			mq.logger.Info("Triggering reconnect callback...")
			callback()
		}

		return
	}

	mq.logger.WithError(lastErr).Errorf("Failed to reconnect after %d attempts", mq.maxRetries)
}

// closeConnectionsUnsafe 关闭现有连接（内部使用，调用者需要处理并发）
func (mq *RabbitMQ) closeConnectionsUnsafe() {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if mq.channel != nil {
		if err := mq.channel.Close(); err != nil {
			// 忽略已关闭的错误
			mq.logger.WithError(err).Debug("Error closing channel (may already be closed)")
		}
		mq.channel = nil
	}

	if mq.conn != nil {
		if err := mq.conn.Close(); err != nil {
			// 忽略已关闭的错误
			mq.logger.WithError(err).Debug("Error closing connection (may already be closed)")
		}
		mq.conn = nil
	}
}

// Publish 发布消息
func (mq *RabbitMQ) Publish(ctx context.Context, body []byte) error {
	mq.mu.RLock()
	ch := mq.channel
	mq.mu.RUnlock()

	if ch == nil {
		return fmt.Errorf("channel is nil")
	}

	return ch.PublishWithContext(
		ctx,
		"",           // exchange
		mq.queueName, // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, // 持久化消息
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
		},
	)
}

// Consume 消费消息
func (mq *RabbitMQ) Consume() (<-chan amqp.Delivery, error) {
	mq.mu.RLock()
	ch := mq.channel
	mq.mu.RUnlock()

	if ch == nil {
		return nil, fmt.Errorf("channel is nil")
	}

	msgs, err := ch.Consume(
		mq.queueName, // queue
		"",           // consumer
		false,        // auto-ack (手动确认)
		false,        // exclusive
		false,        // no-local
		false,        // no-wait
		nil,          // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to consume: %w", err)
	}

	return msgs, nil
}

// GetQueueStats 获取队列统计信息
func (mq *RabbitMQ) GetQueueStats() (messageCount, consumerCount int, err error) {
	mq.mu.RLock()
	ch := mq.channel
	mq.mu.RUnlock()

	if ch == nil {
		return 0, 0, fmt.Errorf("channel is nil")
	}

	queue, err := ch.QueueInspect(mq.queueName)
	if err != nil {
		return 0, 0, err
	}

	return queue.Messages, queue.Consumers, nil
}

// Close 关闭连接
func (mq *RabbitMQ) Close() error {
	mq.mu.Lock()
	mq.closed = true
	mq.mu.Unlock()

	mq.closeConnectionsUnsafe()
	mq.logger.Info("RabbitMQ connection closed")
	return nil
}

// IsConnected 检查连接状态
func (mq *RabbitMQ) IsConnected() bool {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	return mq.conn != nil && !mq.conn.IsClosed()
}

// IsClosed 检查客户端是否已关闭
func (mq *RabbitMQ) IsClosed() bool {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	return mq.closed
}

// PurgeQueue 清空队列中的所有消息
// 用于服务启动时确保队列与数据库状态一致
func (mq *RabbitMQ) PurgeQueue() (int, error) {
	mq.mu.RLock()
	ch := mq.channel
	mq.mu.RUnlock()

	if ch == nil {
		return 0, fmt.Errorf("channel is nil")
	}

	count, err := ch.QueuePurge(mq.queueName, false)
	if err != nil {
		return 0, fmt.Errorf("failed to purge queue: %w", err)
	}

	mq.logger.WithFields(logrus.Fields{
		"queue":        mq.queueName,
		"purged_count": count,
	}).Info("Queue purged successfully")

	return count, nil
}
