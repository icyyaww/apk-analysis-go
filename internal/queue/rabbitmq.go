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
	reconnect     chan bool
	maxRetries    int
	prefetchCount int // 预取数量，应与 worker 数量匹配

	// 连接状态管理
	mu            sync.RWMutex
	closed        bool
	connNotify    chan *amqp.Error
	channelNotify chan *amqp.Error
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
		reconnect:     make(chan bool, 10), // 增大缓冲区，避免信号丢失
		maxRetries:    10,
		prefetchCount: prefetchCount,
		closed:        false,
	}

	if err := mq.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return mq, nil
}

// connect 建立连接
func (mq *RabbitMQ) connect() error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

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
	mq.conn = conn

	// 创建 Channel
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}
	mq.channel = ch

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

	// 设置 Connection 和 Channel 关闭通知
	mq.connNotify = make(chan *amqp.Error, 1)
	mq.channelNotify = make(chan *amqp.Error, 1)
	mq.conn.NotifyClose(mq.connNotify)
	mq.channel.NotifyClose(mq.channelNotify)

	mq.logger.WithFields(logrus.Fields{
		"host":           mq.config.Host,
		"port":           mq.config.Port,
		"queue":          mq.queueName,
		"heartbeat":      mq.config.Heartbeat,
		"prefetch_count": mq.prefetchCount,
	}).Info("Connected to RabbitMQ")

	return nil
}

// StartConnectionWatcher 启动连接监听器（持续监听，直到主动关闭）
// 同时监听 Connection 和 Channel 关闭事件
func (mq *RabbitMQ) StartConnectionWatcher() {
	go func() {
		for {
			mq.mu.RLock()
			if mq.closed {
				mq.mu.RUnlock()
				mq.logger.Info("Connection watcher stopped: RabbitMQ client closed")
				return
			}
			connNotify := mq.connNotify
			channelNotify := mq.channelNotify
			mq.mu.RUnlock()

			// 等待任一关闭事件
			select {
			case err, ok := <-connNotify:
				if !ok {
					// Channel 已关闭，检查是否主动关闭
					mq.mu.RLock()
					closed := mq.closed
					mq.mu.RUnlock()
					if closed {
						return
					}
				}
				if err != nil {
					mq.logger.WithError(err).Error("RabbitMQ connection closed unexpectedly")
				} else {
					mq.logger.Warn("RabbitMQ connection closed")
				}
				mq.triggerReconnect()

			case err, ok := <-channelNotify:
				if !ok {
					mq.mu.RLock()
					closed := mq.closed
					mq.mu.RUnlock()
					if closed {
						return
					}
				}
				if err != nil {
					mq.logger.WithError(err).Error("RabbitMQ channel closed unexpectedly")
				} else {
					mq.logger.Warn("RabbitMQ channel closed")
				}
				mq.triggerReconnect()
			}
		}
	}()
}

// triggerReconnect 触发重连信号（非阻塞）
func (mq *RabbitMQ) triggerReconnect() {
	select {
	case mq.reconnect <- true:
		mq.logger.Debug("Reconnect signal sent")
	default:
		mq.logger.Debug("Reconnect signal already pending")
	}
}

// Reconnect 重新连接
func (mq *RabbitMQ) Reconnect() error {
	// 先关闭旧连接（忽略错误）
	mq.closeConnections()

	retries := 0
	for retries < mq.maxRetries {
		mq.logger.Infof("Attempting to reconnect to RabbitMQ (attempt %d/%d)", retries+1, mq.maxRetries)

		if err := mq.connect(); err != nil {
			mq.logger.WithError(err).Error("Failed to reconnect")
			retries++
			time.Sleep(time.Duration(retries) * time.Second) // 指数退避
			continue
		}

		mq.logger.Info("Successfully reconnected to RabbitMQ")
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", mq.maxRetries)
}

// closeConnections 关闭现有连接（不设置 closed 标志）
func (mq *RabbitMQ) closeConnections() {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if mq.channel != nil {
		mq.channel.Close()
		mq.channel = nil
	}
	if mq.conn != nil {
		mq.conn.Close()
		mq.conn = nil
	}
}

// Publish 发布消息
func (mq *RabbitMQ) Publish(ctx context.Context, body []byte) error {
	if mq.channel == nil {
		return fmt.Errorf("channel is nil")
	}

	return mq.channel.PublishWithContext(
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
	if mq.channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}

	msgs, err := mq.channel.Consume(
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
	if mq.channel == nil {
		return 0, 0, fmt.Errorf("channel is nil")
	}

	queue, err := mq.channel.QueueInspect(mq.queueName)
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

	if mq.channel != nil {
		if err := mq.channel.Close(); err != nil {
			mq.logger.WithError(err).Error("Failed to close channel")
		}
	}

	if mq.conn != nil {
		if err := mq.conn.Close(); err != nil {
			mq.logger.WithError(err).Error("Failed to close connection")
		}
	}

	mq.logger.Info("RabbitMQ connection closed")
	return nil
}

// GetReconnectChan 获取重连信号通道
func (mq *RabbitMQ) GetReconnectChan() <-chan bool {
	return mq.reconnect
}

// IsConnected 检查连接状态
func (mq *RabbitMQ) IsConnected() bool {
	return mq.conn != nil && !mq.conn.IsClosed()
}

// PurgeQueue 清空队列中的所有消息
// 用于服务启动时确保队列与数据库状态一致
func (mq *RabbitMQ) PurgeQueue() (int, error) {
	if mq.channel == nil {
		return 0, fmt.Errorf("channel is nil")
	}

	count, err := mq.channel.QueuePurge(mq.queueName, false)
	if err != nil {
		return 0, fmt.Errorf("failed to purge queue: %w", err)
	}

	mq.logger.WithFields(logrus.Fields{
		"queue":         mq.queueName,
		"purged_count":  count,
	}).Info("Queue purged successfully")

	return count, nil
}
