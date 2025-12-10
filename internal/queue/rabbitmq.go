package queue

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// RabbitMQConfig RabbitMQ 配置
type RabbitMQConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	VHost    string
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
	mq := &RabbitMQ{
		config:        config,
		logger:        logger,
		queueName:     queueName,
		reconnect:     make(chan bool, 1),
		maxRetries:    10,
		prefetchCount: prefetchCount,
	}

	if err := mq.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return mq, nil
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

	// 连接 RabbitMQ
	conn, err := amqp.Dial(url)
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
	mq.logger.WithField("prefetch_count", mq.prefetchCount).Info("RabbitMQ QoS configured")

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

	mq.logger.WithFields(logrus.Fields{
		"host":  mq.config.Host,
		"port":  mq.config.Port,
		"queue": mq.queueName,
	}).Info("Connected to RabbitMQ")

	// 监听连接错误
	go mq.watchConnection()

	return nil
}

// watchConnection 监听连接状态
func (mq *RabbitMQ) watchConnection() {
	closeErr := make(chan *amqp.Error)
	mq.conn.NotifyClose(closeErr)

	err := <-closeErr
	if err != nil {
		mq.logger.WithError(err).Error("RabbitMQ connection closed")
		mq.reconnect <- true
	}
}

// Reconnect 重新连接
func (mq *RabbitMQ) Reconnect() error {
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
