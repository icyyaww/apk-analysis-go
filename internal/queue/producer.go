package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
)

// TaskMessage 任务消息
type TaskMessage struct {
	TaskID  string `json:"task_id"`
	APKName string `json:"apk_name"`
	APKPath string `json:"apk_path"`
}

// Producer 消息生产者
type Producer struct {
	mq     *RabbitMQ
	logger *logrus.Logger
}

// NewProducer 创建生产者
func NewProducer(mq *RabbitMQ, logger *logrus.Logger) *Producer {
	return &Producer{
		mq:     mq,
		logger: logger,
	}
}

// PublishTask 发布任务消息
func (p *Producer) PublishTask(ctx context.Context, msg *TaskMessage) error {
	// 序列化消息
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 发布到队列
	if err := p.mq.Publish(ctx, body); err != nil {
		p.logger.WithError(err).WithField("task_id", msg.TaskID).Error("Failed to publish task")
		return fmt.Errorf("failed to publish: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"task_id":  msg.TaskID,
		"apk_name": msg.APKName,
	}).Info("Task published to queue")

	return nil
}

// GetQueueSize 获取队列大小
func (p *Producer) GetQueueSize() (int, error) {
	messageCount, _, err := p.mq.GetQueueStats()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue stats: %w", err)
	}
	return messageCount, nil
}
