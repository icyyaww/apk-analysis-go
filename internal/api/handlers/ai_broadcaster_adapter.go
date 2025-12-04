package handlers

import (
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/worker"
)

// AIBroadcasterAdapter 适配器，将AIInteractionHandler转换为worker.AIInteractionBroadcaster接口
type AIBroadcasterAdapter struct {
	handler *AIInteractionHandler
}

// NewAIBroadcasterAdapter 创建广播器适配器
func NewAIBroadcasterAdapter(handler *AIInteractionHandler) *AIBroadcasterAdapter {
	return &AIBroadcasterAdapter{
		handler: handler,
	}
}

// BroadcastAction 广播AI动作
func (a *AIBroadcasterAdapter) BroadcastAction(taskID, activity string, action worker.AIActionData) {
	msg := AIInteractionMessage{
		TaskID:   taskID,
		Activity: activity,
		Action: &AIAction{
			Type:     action.Type,
			X:        action.X,
			Y:        action.Y,
			Reason:   action.Reason,
			Priority: action.Priority,
		},
		Timestamp: time.Now().Unix(),
	}

	select {
	case a.handler.broadcast <- msg:
		a.handler.logger.WithFields(map[string]interface{}{
			"task_id":  taskID,
			"activity": activity,
			"action":   action.Type,
		}).Debug("AI action broadcasted via adapter")
	default:
		a.handler.logger.Warn("Broadcast channel is full, dropping message")
	}
}

// BroadcastScreenshot 广播截图更新
func (a *AIBroadcasterAdapter) BroadcastScreenshot(taskID string, screenshotURL string) {
	msg := AIInteractionMessage{
		TaskID:     taskID,
		Screenshot: screenshotURL,
		Timestamp:  time.Now().Unix(),
	}

	select {
	case a.handler.broadcast <- msg:
		a.handler.logger.WithFields(map[string]interface{}{
			"task_id":       taskID,
			"screenshot_url": screenshotURL,
		}).Debug("Screenshot broadcasted via adapter")
	default:
		a.handler.logger.Warn("Broadcast channel is full, dropping screenshot message")
	}
}

// BroadcastStatus 广播状态更新
func (a *AIBroadcasterAdapter) BroadcastStatus(taskID string, status string) {
	msg := AIInteractionMessage{
		TaskID:    taskID,
		Status:    status,
		Timestamp: time.Now().Unix(),
	}

	select {
	case a.handler.broadcast <- msg:
		a.handler.logger.WithFields(map[string]interface{}{
			"task_id": taskID,
			"status":  status,
		}).Debug("Status broadcasted via adapter")
	default:
		a.handler.logger.Warn("Broadcast channel is full, dropping status message")
	}
}