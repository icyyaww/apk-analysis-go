package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// TaskHandler 任务处理函数
type TaskHandler func(ctx context.Context, msg *TaskMessage) error

// Consumer 消息消费者
type Consumer struct {
	mq         *RabbitMQ
	logger     *logrus.Logger
	handler    TaskHandler
	workerPool int
	stopChan   chan struct{}
}

// NewConsumer 创建消费者
func NewConsumer(mq *RabbitMQ, handler TaskHandler, workerPool int, logger *logrus.Logger) *Consumer {
	if workerPool <= 0 {
		workerPool = 1
	}

	return &Consumer{
		mq:         mq,
		logger:     logger,
		handler:    handler,
		workerPool: workerPool,
		stopChan:   make(chan struct{}),
	}
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Infof("Starting consumer with %d workers", c.workerPool)

	// 获取消息通道
	msgs, err := c.mq.Consume()
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	// 启动多个 worker goroutine
	for i := 0; i < c.workerPool; i++ {
		go c.worker(ctx, i, msgs)
	}

	c.logger.Info("Consumer started successfully")

	// 监听重连信号
	go c.handleReconnect(ctx)

	return nil
}

// worker 工作协程
func (c *Consumer) worker(ctx context.Context, id int, msgs <-chan amqp.Delivery) {
	c.logger.Infof("Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			c.logger.Infof("Worker %d stopped", id)
			return
		case <-c.stopChan:
			c.logger.Infof("Worker %d stopped by signal", id)
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Warnf("Worker %d: message channel closed", id)
				return
			}

			c.processMessage(ctx, id, msg)
		}
	}
}

// processMessage 处理单条消息
func (c *Consumer) processMessage(ctx context.Context, workerID int, delivery amqp.Delivery) {
	startTime := time.Now()

	// 反序列化消息
	var msg TaskMessage
	if err := json.Unmarshal(delivery.Body, &msg); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal message")
		delivery.Nack(false, false) // 拒绝消息, 不重新入队
		return
	}

	c.logger.WithFields(logrus.Fields{
		"worker_id": workerID,
		"task_id":   msg.TaskID,
		"apk_name":  msg.APKName,
	}).Info("Processing task")

	// 调用处理函数
	if err := c.handler(ctx, &msg); err != nil {
		c.logger.WithError(err).WithFields(logrus.Fields{
			"worker_id": workerID,
			"task_id":   msg.TaskID,
		}).Error("Task processing failed")

		// 任务失败, 根据策略决定是否重新入队
		// 这里暂时不重新入队, 避免无限循环
		delivery.Nack(false, false)
		return
	}

	// 任务成功, 确认消息
	if err := delivery.Ack(false); err != nil {
		c.logger.WithError(err).Error("Failed to acknowledge message")
	}

	duration := time.Since(startTime)
	c.logger.WithFields(logrus.Fields{
		"worker_id": workerID,
		"task_id":   msg.TaskID,
		"duration":  duration.Seconds(),
	}).Info("Task completed successfully")
}

// handleReconnect 处理重连
func (c *Consumer) handleReconnect(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.mq.reconnect:
			c.logger.Warn("Connection lost, attempting to reconnect...")

			if err := c.mq.Reconnect(); err != nil {
				c.logger.WithError(err).Error("Failed to reconnect")
				continue
			}

			// 重新启动消费
			if err := c.Start(ctx); err != nil {
				c.logger.WithError(err).Error("Failed to restart consumer")
			}
		}
	}
}

// Stop 停止消费者
func (c *Consumer) Stop() {
	c.logger.Info("Stopping consumer...")
	close(c.stopChan)
}

// GetActiveWorkers 获取活跃 worker 数量
func (c *Consumer) GetActiveWorkers() int {
	return c.workerPool
}
