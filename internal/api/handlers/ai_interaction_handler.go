package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// AIInteractionHandler 处理AI交互相关的API
type AIInteractionHandler struct {
	logger      *logrus.Logger
	upgrader    websocket.Upgrader
	clients     map[string]*websocket.Conn
	clientMutex sync.RWMutex
	broadcast   chan AIInteractionMessage
}

// AIInteractionMessage AI交互消息
type AIInteractionMessage struct {
	TaskID     string      `json:"task_id"`
	Activity   string      `json:"activity,omitempty"`
	Action     *AIAction   `json:"action,omitempty"`
	Screenshot string      `json:"screenshot,omitempty"`
	Status     string      `json:"status,omitempty"`
	Timestamp  int64       `json:"timestamp"`
}

// AIAction AI动作详情
type AIAction struct {
	Type     string `json:"type"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

// NewAIInteractionHandler 创建AI交互处理器
func NewAIInteractionHandler(logger *logrus.Logger) *AIInteractionHandler {
	return &AIInteractionHandler{
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源（生产环境需要限制）
			},
		},
		clients:   make(map[string]*websocket.Conn),
		broadcast: make(chan AIInteractionMessage, 100),
	}
}

// Start 启动广播服务
func (h *AIInteractionHandler) Start() {
	go h.runBroadcaster()
}

// runBroadcaster 运行广播器
func (h *AIInteractionHandler) runBroadcaster() {
	for {
		msg := <-h.broadcast
		h.clientMutex.RLock()
		for taskID, client := range h.clients {
			// 只发送给对应任务的客户端或所有客户端（如果taskID为"all"）
			if msg.TaskID == taskID || taskID == "latest" || msg.TaskID == "all" {
				err := client.WriteJSON(msg)
				if err != nil {
					h.logger.WithError(err).Warn("Failed to write to WebSocket client")
					client.Close()
					h.clientMutex.RUnlock()
					h.clientMutex.Lock()
					delete(h.clients, taskID)
					h.clientMutex.Unlock()
					h.clientMutex.RLock()
				}
			}
		}
		h.clientMutex.RUnlock()
	}
}

// HandleWebSocket 处理WebSocket连接
func (h *AIInteractionHandler) HandleWebSocket(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		taskID = "latest" // 默认监听最新任务
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to upgrade to WebSocket")
		return
	}
	defer conn.Close()

	// 注册客户端
	h.clientMutex.Lock()
	h.clients[taskID] = conn
	h.clientMutex.Unlock()

	h.logger.WithField("task_id", taskID).Info("WebSocket client connected")

	// 保持连接
	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.WithError(err).Warn("WebSocket error")
			}
			break
		}
		// 处理来自客户端的消息（如果需要）
	}

	// 清理断开的连接
	h.clientMutex.Lock()
	delete(h.clients, taskID)
	h.clientMutex.Unlock()

	h.logger.WithField("task_id", taskID).Info("WebSocket client disconnected")
}

// GetAIInteractionPage 返回AI交互监控页面
func (h *AIInteractionHandler) GetAIInteractionPage(c *gin.Context) {
	c.HTML(http.StatusOK, "ai_interaction.html", gin.H{
		"title": "AI智能交互监控",
	})
}

// BroadcastAction 广播AI动作（供内部调用）
func (h *AIInteractionHandler) BroadcastAction(taskID, activity string, action *AIAction) {
	msg := AIInteractionMessage{
		TaskID:    taskID,
		Activity:  activity,
		Action:    action,
		Timestamp: getCurrentTimestamp(),
	}

	select {
	case h.broadcast <- msg:
		h.logger.WithFields(logrus.Fields{
			"task_id":  taskID,
			"activity": activity,
			"action":   action.Type,
		}).Debug("AI action broadcasted")
	default:
		h.logger.Warn("Broadcast channel is full, dropping message")
	}
}

// BroadcastScreenshot 广播截图更新
func (h *AIInteractionHandler) BroadcastScreenshot(taskID string, screenshotURL string) {
	msg := AIInteractionMessage{
		TaskID:     taskID,
		Screenshot: screenshotURL,
		Timestamp:  getCurrentTimestamp(),
	}

	select {
	case h.broadcast <- msg:
	default:
	}
}

// BroadcastStatus 广播状态更新
func (h *AIInteractionHandler) BroadcastStatus(taskID string, status string) {
	msg := AIInteractionMessage{
		TaskID:    taskID,
		Status:    status,
		Timestamp: getCurrentTimestamp(),
	}

	select {
	case h.broadcast <- msg:
	default:
	}
}

func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}