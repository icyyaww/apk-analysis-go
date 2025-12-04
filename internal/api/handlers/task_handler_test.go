package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTaskService Mock Service
type MockTaskService struct {
	mock.Mock
}

func (m *MockTaskService) CreateTask(ctx gin.Context, apkName string) (*domain.Task, error) {
	args := m.Called(apkName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Task), args.Error(1)
}

func (m *MockTaskService) GetTask(ctx gin.Context, id string) (*domain.Task, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Task), args.Error(1)
}

func (m *MockTaskService) ListRecentTasks(ctx gin.Context, limit int) ([]*domain.Task, error) {
	args := m.Called(limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Task), args.Error(1)
}

func (m *MockTaskService) UpdateTaskStatus(ctx gin.Context, id string, status domain.TaskStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockTaskService) UpdateTaskProgress(ctx gin.Context, id string, percent int, step string) error {
	args := m.Called(id, percent, step)
	return args.Error(0)
}

func (m *MockTaskService) DeleteTask(ctx gin.Context, id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockTaskService) GetTasksByStatus(ctx gin.Context, status domain.TaskStatus) ([]*domain.Task, error) {
	args := m.Called(status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Task), args.Error(1)
}

func (m *MockTaskService) GetTaskStatistics(ctx gin.Context) (map[string]int64, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

// setupTestRouter 设置测试路由
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// TestTaskHandler_GetTask 测试获取任务
func TestTaskHandler_GetTask(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks/:id", handler.GetTask)

	expectedTask := &domain.Task{
		ID:        "test-task-001",
		APKName:   "test.apk",
		Status:    domain.TaskStatusCompleted,
		CreatedAt: time.Now(),
	}

	// Mock 成功获取
	mockService.On("GetTask", "test-task-001").Return(expectedTask, nil)

	// 发送请求
	req := httptest.NewRequest("GET", "/api/tasks/test-task-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.Task
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, expectedTask.ID, response.ID)
	assert.Equal(t, expectedTask.APKName, response.APKName)

	mockService.AssertExpectations(t)
}

// TestTaskHandler_GetTask_NotFound 测试获取不存在的任务
func TestTaskHandler_GetTask_NotFound(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks/:id", handler.GetTask)

	// Mock 任务不存在
	mockService.On("GetTask", "non-existent").Return(nil, errors.New("not found"))

	req := httptest.NewRequest("GET", "/api/tasks/non-existent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockService.AssertExpectations(t)
}

// TestTaskHandler_ListTasks 测试列出任务
func TestTaskHandler_ListTasks(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks", handler.ListTasks)

	expectedTasks := []*domain.Task{
		{ID: "task-1", APKName: "app1.apk", Status: domain.TaskStatusCompleted},
		{ID: "task-2", APKName: "app2.apk", Status: domain.TaskStatusRunning},
	}

	// Mock 成功列出
	mockService.On("ListRecentTasks", 50).Return(expectedTasks, nil)

	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []*domain.Task
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, expectedTasks[0].ID, response[0].ID)

	mockService.AssertExpectations(t)
}

// TestTaskHandler_ListTasks_WithLimit 测试带限制的列表
func TestTaskHandler_ListTasks_WithLimit(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks", handler.ListTasks)

	expectedTasks := []*domain.Task{
		{ID: "task-1", APKName: "app1.apk"},
	}

	// Mock 限制为 10
	mockService.On("ListRecentTasks", 10).Return(expectedTasks, nil)

	req := httptest.NewRequest("GET", "/api/tasks?limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []*domain.Task
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 1)

	mockService.AssertExpectations(t)
}

// TestTaskHandler_DeleteTask 测试删除任务
func TestTaskHandler_DeleteTask(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.DELETE("/api/tasks/:id", handler.DeleteTask)

	// Mock 成功删除
	mockService.On("DeleteTask", "task-001").Return(nil)

	req := httptest.NewRequest("DELETE", "/api/tasks/task-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Task deleted successfully", response["message"])

	mockService.AssertExpectations(t)
}

// TestTaskHandler_DeleteTask_Error 测试删除任务失败
func TestTaskHandler_DeleteTask_Error(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.DELETE("/api/tasks/:id", handler.DeleteTask)

	// Mock 删除失败
	mockService.On("DeleteTask", "task-001").Return(errors.New("database error"))

	req := httptest.NewRequest("DELETE", "/api/tasks/task-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

// TestTaskHandler_StopTask 测试停止任务
func TestTaskHandler_StopTask(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.POST("/api/tasks/:id/stop", handler.StopTask)

	runningTask := &domain.Task{
		ID:     "task-001",
		Status: domain.TaskStatusRunning,
	}

	// Mock 获取任务
	mockService.On("GetTask", "task-001").Return(runningTask, nil)
	// Mock 更新状态
	mockService.On("UpdateTaskStatus", "task-001", domain.TaskStatusCancelled).Return(nil)

	req := httptest.NewRequest("POST", "/api/tasks/task-001/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Task stopped", response["message"])

	mockService.AssertExpectations(t)
}

// TestTaskHandler_GetSystemStats 测试获取系统统计
func TestTaskHandler_GetSystemStats(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/stats", handler.GetSystemStats)

	expectedStats := map[string]int64{
		"queued":    5,
		"running":   2,
		"completed": 100,
		"failed":    3,
	}

	// Mock 统计数据
	mockService.On("GetTaskStatistics").Return(expectedStats, nil)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "total_tasks")
	assert.Contains(t, response, "queued")
	assert.Contains(t, response, "running")
	assert.Contains(t, response, "completed")
	assert.Contains(t, response, "failed")

	mockService.AssertExpectations(t)
}

// TestTaskHandler_InvalidID 测试无效的任务ID
func TestTaskHandler_InvalidID(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks/:id", handler.GetTask)

	// Mock 获取失败
	mockService.On("GetTask", "").Return(nil, errors.New("invalid ID"))

	req := httptest.NewRequest("GET", "/api/tasks/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Gin 会返回 404 (没有匹配的路由)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestTaskHandler_ConcurrentRequests 测试并发请求
func TestTaskHandler_ConcurrentRequests(t *testing.T) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks/:id", handler.GetTask)

	task := &domain.Task{
		ID:      "concurrent-task",
		APKName: "test.apk",
	}

	// Mock 并发获取
	mockService.On("GetTask", "concurrent-task").Return(task, nil)

	// 并发发送 10 个请求
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/api/tasks/concurrent-task", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			done <- true
		}()
	}

	// 等待所有请求完成
	for i := 0; i < 10; i++ {
		<-done
	}

	mockService.AssertNumberOfCalls(t, "GetTask", 10)
}

// BenchmarkTaskHandler_GetTask 性能测试 - 获取任务
func BenchmarkTaskHandler_GetTask(b *testing.B) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks/:id", handler.GetTask)

	task := &domain.Task{
		ID:      "bench-task",
		APKName: "bench.apk",
	}

	mockService.On("GetTask", "bench-task").Return(task, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/tasks/bench-task", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkTaskHandler_ListTasks 性能测试 - 列出任务
func BenchmarkTaskHandler_ListTasks(b *testing.B) {
	mockService := new(MockTaskService)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	handler := NewTaskHandler(mockService, logger)
	router := setupTestRouter()
	router.GET("/api/tasks", handler.ListTasks)

	tasks := []*domain.Task{
		{ID: "task-1", APKName: "app1.apk"},
		{ID: "task-2", APKName: "app2.apk"},
	}

	mockService.On("ListRecentTasks", 50).Return(tasks, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/tasks", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
