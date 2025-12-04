package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTaskRepository Mock Repository
type MockTaskRepository struct {
	mock.Mock
}

func (m *MockTaskRepository) Create(ctx context.Context, task *domain.Task) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockTaskRepository) Update(ctx context.Context, task *domain.Task) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockTaskRepository) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Task), args.Error(1)
}

func (m *MockTaskRepository) ListRecent(ctx context.Context, limit int) ([]*domain.Task, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Task), args.Error(1)
}

func (m *MockTaskRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockTaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockTaskRepository) UpdateProgress(ctx context.Context, id string, percent int, step string) error {
	args := m.Called(ctx, id, percent, step)
	return args.Error(0)
}

func (m *MockTaskRepository) FindByStatus(ctx context.Context, status domain.TaskStatus) ([]*domain.Task, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Task), args.Error(1)
}

func (m *MockTaskRepository) CountByStatus(ctx context.Context, status domain.TaskStatus) (int64, error) {
	args := m.Called(ctx, status)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTaskRepository) UpdateMobSFStatus(ctx context.Context, id string, status, hash string, score int) error {
	args := m.Called(ctx, id, status, hash, score)
	return args.Error(0)
}

// TestTaskService_CreateTask 测试创建任务
func TestTaskService_CreateTask(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 成功创建
	mockRepo.On("Create", ctx, mock.AnythingOfType("*domain.Task")).Return(nil)

	task, err := service.CreateTask(ctx, "test.apk")

	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.NotEmpty(t, task.ID, "Task ID should not be empty")
	assert.Equal(t, "test.apk", task.APKName)
	assert.Equal(t, domain.TaskStatusQueued, task.Status)
	assert.Equal(t, 0, task.ProgressPercent)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_CreateTask_Error 测试创建任务失败
func TestTaskService_CreateTask_Error(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 创建失败
	mockRepo.On("Create", ctx, mock.AnythingOfType("*domain.Task")).Return(errors.New("database error"))

	task, err := service.CreateTask(ctx, "test.apk")

	assert.Error(t, err)
	assert.Nil(t, task)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_GetTask 测试获取任务
func TestTaskService_GetTask(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	expectedTask := &domain.Task{
		ID:      "test-task-001",
		APKName: "test.apk",
		Status:  domain.TaskStatusRunning,
	}

	// Mock 成功查找
	mockRepo.On("FindByID", ctx, "test-task-001").Return(expectedTask, nil)

	task, err := service.GetTask(ctx, "test-task-001")

	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, expectedTask.ID, task.ID)
	assert.Equal(t, expectedTask.Status, task.Status)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_GetTask_NotFound 测试获取不存在的任务
func TestTaskService_GetTask_NotFound(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 查找失败
	mockRepo.On("FindByID", ctx, "non-existent").Return(nil, errors.New("not found"))

	task, err := service.GetTask(ctx, "non-existent")

	assert.Error(t, err)
	assert.Nil(t, task)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_ListRecentTasks 测试列出最近任务
func TestTaskService_ListRecentTasks(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	expectedTasks := []*domain.Task{
		{ID: "task-1", APKName: "app1.apk", Status: domain.TaskStatusCompleted},
		{ID: "task-2", APKName: "app2.apk", Status: domain.TaskStatusRunning},
		{ID: "task-3", APKName: "app3.apk", Status: domain.TaskStatusQueued},
	}

	// Mock 成功列出
	mockRepo.On("ListRecent", ctx, 10).Return(expectedTasks, nil)

	tasks, err := service.ListRecentTasks(ctx, 10)

	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
	assert.Equal(t, expectedTasks[0].ID, tasks[0].ID)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_UpdateTaskStatus 测试更新任务状态
func TestTaskService_UpdateTaskStatus(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 成功更新
	mockRepo.On("UpdateStatus", ctx, "task-001", domain.TaskStatusCompleted).Return(nil)

	err := service.UpdateTaskStatus(ctx, "task-001", domain.TaskStatusCompleted)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_UpdateTaskProgress 测试更新任务进度
func TestTaskService_UpdateTaskProgress(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 成功更新
	mockRepo.On("UpdateProgress", ctx, "task-001", 50, "正在分析").Return(nil)

	err := service.UpdateTaskProgress(ctx, "task-001", 50, "正在分析")

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_DeleteTask 测试删除任务
func TestTaskService_DeleteTask(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 成功删除
	mockRepo.On("Delete", ctx, "task-001").Return(nil)

	err := service.DeleteTask(ctx, "task-001")

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

// TestTaskService_GetTasksByStatus 测试按状态获取任务
func TestTaskService_GetTasksByStatus(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	expectedTasks := []*domain.Task{
		{ID: "task-1", Status: domain.TaskStatusRunning},
		{ID: "task-2", Status: domain.TaskStatusRunning},
	}

	// Mock 成功查找
	mockRepo.On("FindByStatus", ctx, domain.TaskStatusRunning).Return(expectedTasks, nil)

	tasks, err := service.GetTasksByStatus(ctx, domain.TaskStatusRunning)

	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	for _, task := range tasks {
		assert.Equal(t, domain.TaskStatusRunning, task.Status)
	}
	mockRepo.AssertExpectations(t)
}

// TestTaskService_GetTaskStatistics 测试获取任务统计
func TestTaskService_GetTaskStatistics(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 各种状态的统计
	mockRepo.On("CountByStatus", ctx, domain.TaskStatusQueued).Return(int64(5), nil)
	mockRepo.On("CountByStatus", ctx, domain.TaskStatusRunning).Return(int64(2), nil)
	mockRepo.On("CountByStatus", ctx, domain.TaskStatusCompleted).Return(int64(100), nil)
	mockRepo.On("CountByStatus", ctx, domain.TaskStatusFailed).Return(int64(3), nil)

	stats, err := service.GetTaskStatistics(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(5), stats["queued"])
	assert.Equal(t, int64(2), stats["running"])
	assert.Equal(t, int64(100), stats["completed"])
	assert.Equal(t, int64(3), stats["failed"])
	mockRepo.AssertExpectations(t)
}

// TestTaskService_ValidateAPKName 测试 APK 名称验证
func TestTaskService_ValidateAPKName(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)

	testCases := []struct {
		name      string
		apkName   string
		shouldErr bool
	}{
		{"Valid APK", "app.apk", false},
		{"Valid APK with path", "/path/to/app.apk", false},
		{"Empty APK name", "", true},
		{"Only spaces", "   ", true},
		{"No .apk extension", "app.txt", true},
		{"Valid with numbers", "app-v1.2.3.apk", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.shouldErr {
				// Mock 成功创建
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Task")).Return(nil).Once()
			}

			task, err := service.CreateTask(context.Background(), tc.apkName)

			if tc.shouldErr {
				assert.Error(t, err, "Expected error for: %s", tc.apkName)
				assert.Nil(t, task)
			} else {
				assert.NoError(t, err, "Unexpected error for: %s", tc.apkName)
				assert.NotNil(t, task)
			}
		})
	}
}

// TestTaskService_ConcurrentOperations 测试并发操作
func TestTaskService_ConcurrentOperations(t *testing.T) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	// Mock 并发创建
	mockRepo.On("Create", ctx, mock.AnythingOfType("*domain.Task")).Return(nil)

	// 并发创建 10 个任务
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			_, err := service.CreateTask(ctx, "test.apk")
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有任务完成
	for i := 0; i < 10; i++ {
		<-done
	}

	mockRepo.AssertNumberOfCalls(t, "Create", 10)
}

// BenchmarkTaskService_CreateTask 性能测试 - 创建任务
func BenchmarkTaskService_CreateTask(b *testing.B) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	mockRepo.On("Create", ctx, mock.AnythingOfType("*domain.Task")).Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.CreateTask(ctx, "bench.apk")
	}
}

// BenchmarkTaskService_GetTask 性能测试 - 获取任务
func BenchmarkTaskService_GetTask(b *testing.B) {
	mockRepo := new(MockTaskRepository)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewTaskService(mockRepo, logger)
	ctx := context.Background()

	task := &domain.Task{
		ID:      "bench-task",
		APKName: "bench.apk",
	}

	mockRepo.On("FindByID", ctx, "bench-task").Return(task, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.GetTask(ctx, "bench-task")
	}
}
