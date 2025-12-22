package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// TaskHandler 任务处理函数
type TaskHandler func(ctx context.Context, msg *TaskMessage) error

// Consumer 消息消费者
type Consumer struct {
	mq            *RabbitMQ
	logger        *logrus.Logger
	handler       TaskHandler
	workerPool    int
	stopChan      chan struct{}
	workerWg      sync.WaitGroup
	activeWorkers int32
	mu            sync.Mutex
	running       bool
	cancelFunc    context.CancelFunc // 用于取消当前所有 worker
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
		stopChan:   make(chan struct{}, 1), // 有缓冲，避免阻塞
		running:    false,
	}
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		c.logger.Warn("Consumer already running, skipping start")
		return nil
	}
	c.running = true
	c.mu.Unlock()

	c.logger.Infof("Starting consumer with %d workers", c.workerPool)

	// 获取消息通道
	msgs, err := c.mq.Consume()
	if err != nil {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	// 创建可取消的 context
	workerCtx, cancel := context.WithCancel(ctx)
	c.cancelFunc = cancel

	// 启动多个 worker goroutine
	for i := 0; i < c.workerPool; i++ {
		c.workerWg.Add(1)
		go c.worker(workerCtx, i, msgs)
	}

	c.logger.Info("Consumer started successfully")

	// 启动连接监听器
	c.mq.StartConnectionWatcher()

	// 监听重连信号
	go c.handleReconnect(ctx)

	return nil
}

// worker 工作协程
func (c *Consumer) worker(ctx context.Context, id int, msgs <-chan amqp.Delivery) {
	defer c.workerWg.Done()
	atomic.AddInt32(&c.activeWorkers, 1)
	defer atomic.AddInt32(&c.activeWorkers, -1)

	c.logger.Infof("Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			c.logger.Infof("Worker %d stopped by context", id)
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
		case _, ok := <-c.mq.GetReconnectChan():
			if !ok {
				c.logger.Info("Reconnect channel closed, stopping reconnect handler")
				return
			}

			c.logger.Warn("Connection lost, attempting to reconnect...")

			// 停止当前所有 worker
			c.stopWorkers()

			// 尝试重连
			if err := c.mq.Reconnect(); err != nil {
				c.logger.WithError(err).Error("Failed to reconnect, will retry on next signal")
				continue
			}

			// 重新启动消费
			if err := c.restart(ctx); err != nil {
				c.logger.WithError(err).Error("Failed to restart consumer")
			}
		}
	}
}

// stopWorkers 停止所有 worker（等待当前任务完成）
func (c *Consumer) stopWorkers() {
	c.mu.Lock()
	if c.cancelFunc != nil {
		c.cancelFunc()
		c.cancelFunc = nil
	}
	c.running = false
	c.mu.Unlock()

	// 等待所有 worker 退出（最多等待 30 秒）
	done := make(chan struct{})
	go func() {
		c.workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("All workers stopped gracefully")
	case <-time.After(30 * time.Second):
		c.logger.Warn("Timeout waiting for workers to stop")
	}
}

// restart 内部重启方法（重连后调用）
func (c *Consumer) restart(ctx context.Context) error {
	c.mu.Lock()
	c.running = false // 重置状态，允许 Start 继续
	c.mu.Unlock()

	return c.Start(ctx)
}

// Stop 停止消费者
func (c *Consumer) Stop() {
	c.logger.Info("Stopping consumer...")

	c.mu.Lock()
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	c.running = false
	c.mu.Unlock()

	// 发送停止信号
	select {
	case c.stopChan <- struct{}{}:
	default:
	}

	// 等待所有 worker 退出
	c.workerWg.Wait()
	c.logger.Info("Consumer stopped")
}

// GetActiveWorkers 获取活跃 worker 数量
func (c *Consumer) GetActiveWorkers() int {
	return int(atomic.LoadInt32(&c.activeWorkers))
}

// IsRunning 检查消费者是否正在运行
func (c *Consumer) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}
