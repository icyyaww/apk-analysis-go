package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/api"
	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/queue"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestEnvironment 测试环境
type TestEnvironment struct {
	DB          *gorm.DB
	Router      *gin.Engine
	TaskService service.TaskService
	Logger      *logrus.Logger
	CleanupFunc func()
}

// setupTestEnvironment 创建完整的测试环境
func setupTestEnvironment(t *testing.T) *TestEnvironment {
	// 设置日志
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // 降低测试时的日志噪音

	// 创建临时数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to open test database")

	// 自动迁移
	err = db.AutoMigrate(
		&domain.Task{},
		&domain.TaskActivity{},
		&domain.TaskMobSFReport{},
		&domain.TaskDomainAnalysis{},
		&domain.TaskAppDomain{},
		&domain.TaskAILog{},
		&domain.ThirdPartySDKRule{},
	)
	require.NoError(t, err, "Failed to migrate database")

	// 创建 Repository 和 Service
	taskRepo := repository.NewTaskRepository(db)
	taskService := service.NewTaskService(taskRepo, logger)

	// 设置 Gin 为测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 注册 API 路由
	api.RegisterRoutes(router, taskService, logger)

	// Cleanup 函数
	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	return &TestEnvironment{
		DB:          db,
		Router:      router,
		TaskService: taskService,
		Logger:      logger,
		CleanupFunc: cleanup,
	}
}

// TestEndToEnd_CreateAndGetTask 端到端测试: 创建任务并获取
func TestEndToEnd_CreateAndGetTask(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// Step 1: 创建任务
	task, err := env.TaskService.CreateTask(ctx, "test_app.apk")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.NotEmpty(t, task.ID)
	assert.Equal(t, "test_app.apk", task.APKName)
	assert.Equal(t, domain.TaskStatusQueued, task.Status)

	taskID := task.ID

	// Step 2: 通过 API 获取任务
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/tasks/%s", taskID), nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.Task
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, taskID, response.ID)
	assert.Equal(t, "test_app.apk", response.APKName)
}

// TestEndToEnd_TaskLifecycle 端到端测试: 完整的任务生命周期
func TestEndToEnd_TaskLifecycle(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// Step 1: 创建任务
	task, err := env.TaskService.CreateTask(ctx, "lifecycle_test.apk")
	require.NoError(t, err)
	taskID := task.ID

	// Step 2: 更新状态为 Running
	err = env.TaskService.UpdateTaskStatus(ctx, taskID, domain.TaskStatusRunning)
	require.NoError(t, err)

	// Step 3: 更新进度
	err = env.TaskService.UpdateTaskProgress(ctx, taskID, 50, "正在分析 Activity")
	require.NoError(t, err)

	// Step 4: 验证状态和进度
	updatedTask, err := env.TaskService.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusRunning, updatedTask.Status)
	assert.Equal(t, 50, updatedTask.ProgressPercent)
	assert.Equal(t, "正在分析 Activity", updatedTask.CurrentStep)

	// Step 5: 完成任务
	err = env.TaskService.UpdateTaskStatus(ctx, taskID, domain.TaskStatusCompleted)
	require.NoError(t, err)

	// Step 6: 验证最终状态
	completedTask, err := env.TaskService.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusCompleted, completedTask.Status)
	assert.NotNil(t, completedTask.CompletedAt)
}

// TestEndToEnd_ListTasks 端到端测试: 列出任务列表
func TestEndToEnd_ListTasks(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// 创建多个任务
	expectedCount := 5
	for i := 0; i < expectedCount; i++ {
		_, err := env.TaskService.CreateTask(ctx, fmt.Sprintf("app_%d.apk", i))
		require.NoError(t, err)
	}

	// 通过 API 获取任务列表
	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var tasks []*domain.Task
	err := json.Unmarshal(w.Body.Bytes(), &tasks)
	require.NoError(t, err)
	assert.Len(t, tasks, expectedCount)
}

// TestEndToEnd_DeleteTask 端到端测试: 删除任务
func TestEndToEnd_DeleteTask(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// 创建任务
	task, err := env.TaskService.CreateTask(ctx, "to_delete.apk")
	require.NoError(t, err)
	taskID := task.ID

	// 删除任务
	err = env.TaskService.DeleteTask(ctx, taskID)
	require.NoError(t, err)

	// 验证任务已删除
	_, err = env.TaskService.GetTask(ctx, taskID)
	assert.Error(t, err)
}

// TestEndToEnd_GetSystemStats 端到端测试: 获取系统统计
func TestEndToEnd_GetSystemStats(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// 创建不同状态的任务
	tasks := []struct {
		name   string
		status domain.TaskStatus
	}{
		{"queued1.apk", domain.TaskStatusQueued},
		{"queued2.apk", domain.TaskStatusQueued},
		{"running1.apk", domain.TaskStatusRunning},
		{"completed1.apk", domain.TaskStatusCompleted},
		{"failed1.apk", domain.TaskStatusFailed},
	}

	for _, tc := range tasks {
		task, err := env.TaskService.CreateTask(ctx, tc.name)
		require.NoError(t, err)
		if tc.status != domain.TaskStatusQueued {
			err = env.TaskService.UpdateTaskStatus(ctx, task.ID, tc.status)
			require.NoError(t, err)
		}
	}

	// 通过 API 获取统计信息
	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var stats map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Equal(t, float64(5), stats["total_tasks"])
	assert.Equal(t, float64(2), stats["queued"])
	assert.Equal(t, float64(1), stats["running"])
	assert.Equal(t, float64(1), stats["completed"])
	assert.Equal(t, float64(1), stats["failed"])
}

// TestStress_ConcurrentTaskCreation 压力测试: 并发创建任务
func TestStress_ConcurrentTaskCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()
	concurrency := 10
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)
	taskIDs := make(chan string, concurrency)

	// 并发创建 10 个任务
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			task, err := env.TaskService.CreateTask(ctx, fmt.Sprintf("concurrent_app_%d.apk", index))
			if err != nil {
				errors <- err
				return
			}
			taskIDs <- task.ID
		}(i)
	}

	wg.Wait()
	close(errors)
	close(taskIDs)

	// 检查错误
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	assert.Empty(t, errList, "Expected no errors during concurrent task creation")

	// 验证所有任务已创建
	var createdIDs []string
	for id := range taskIDs {
		createdIDs = append(createdIDs, id)
	}
	assert.Len(t, createdIDs, concurrency)

	// 验证所有任务都能查询到
	for _, id := range createdIDs {
		task, err := env.TaskService.GetTask(ctx, id)
		assert.NoError(t, err)
		assert.NotNil(t, task)
	}
}

// TestStress_ConcurrentTaskUpdates 压力测试: 并发更新任务
func TestStress_ConcurrentTaskUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// 创建一个任务
	task, err := env.TaskService.CreateTask(ctx, "stress_update.apk")
	require.NoError(t, err)
	taskID := task.ID

	concurrency := 10
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	// 并发更新进度
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			percent := (index + 1) * 10
			step := fmt.Sprintf("Step %d", index)
			err := env.TaskService.UpdateTaskProgress(ctx, taskID, percent, step)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	assert.Empty(t, errList, "Expected no errors during concurrent updates")

	// 验证任务状态
	updatedTask, err := env.TaskService.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.NotNil(t, updatedTask)
}

// TestStress_ConcurrentAPIRequests 压力测试: 并发 API 请求
func TestStress_ConcurrentAPIRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// 预先创建 5 个任务
	var taskIDs []string
	for i := 0; i < 5; i++ {
		task, err := env.TaskService.CreateTask(ctx, fmt.Sprintf("api_stress_%d.apk", i))
		require.NoError(t, err)
		taskIDs = append(taskIDs, task.ID)
	}

	concurrency := 20
	var wg sync.WaitGroup
	results := make(chan int, concurrency) // HTTP status codes

	// 并发发送 20 个 GET 请求
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// 随机选择一个任务 ID
			taskID := taskIDs[index%len(taskIDs)]

			req := httptest.NewRequest("GET", fmt.Sprintf("/api/tasks/%s", taskID), nil)
			w := httptest.NewRecorder()
			env.Router.ServeHTTP(w, req)

			results <- w.Code
		}(i)
	}

	wg.Wait()
	close(results)

	// 验证所有请求都成功
	successCount := 0
	for code := range results {
		if code == http.StatusOK {
			successCount++
		}
	}
	assert.Equal(t, concurrency, successCount, "All concurrent requests should succeed")
}

// TestStress_HighLoadTaskProcessing 压力测试: 高负载任务处理
func TestStress_HighLoadTaskProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	ctx := context.Background()

	// 模拟高负载: 创建 50 个任务
	taskCount := 50
	var wg sync.WaitGroup
	errors := make(chan error, taskCount)

	startTime := time.Now()

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// 创建任务
			task, err := env.TaskService.CreateTask(ctx, fmt.Sprintf("load_test_%d.apk", index))
			if err != nil {
				errors <- err
				return
			}

			// 模拟任务执行: 更新状态到 Running
			err = env.TaskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusRunning)
			if err != nil {
				errors <- err
				return
			}

			// 模拟进度更新
			for progress := 10; progress <= 100; progress += 10 {
				err = env.TaskService.UpdateTaskProgress(ctx, task.ID, progress, fmt.Sprintf("Progress %d%%", progress))
				if err != nil {
					errors <- err
					return
				}
				time.Sleep(1 * time.Millisecond) // 模拟处理时间
			}

			// 完成任务
			err = env.TaskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusCompleted)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(startTime)

	// 检查错误
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	assert.Empty(t, errList, "Expected no errors during high load processing")

	// 验证统计信息
	stats, err := env.TaskService.GetTaskStatistics(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(taskCount), stats["completed"])

	t.Logf("Processed %d tasks in %v (avg: %v per task)", taskCount, duration, duration/time.Duration(taskCount))
}

// TestEndToEnd_ErrorHandling 端到端测试: 错误处理
func TestEndToEnd_ErrorHandling(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.CleanupFunc()

	// Test 1: 获取不存在的任务
	req := httptest.NewRequest("GET", "/api/tasks/non-existent-id", nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test 2: 删除不存在的任务
	ctx := context.Background()
	err := env.TaskService.DeleteTask(ctx, "non-existent-id")
	assert.Error(t, err)

	// Test 3: 更新不存在任务的状态
	err = env.TaskService.UpdateTaskStatus(ctx, "non-existent-id", domain.TaskStatusRunning)
	assert.Error(t, err)
}

// BenchmarkEndToEnd_TaskCreation 基准测试: 任务创建性能
func BenchmarkEndToEnd_TaskCreation(b *testing.B) {
	// 创建测试环境
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.Task{})

	taskRepo := repository.NewTaskRepository(db)
	taskService := service.NewTaskService(taskRepo, logger)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskService.CreateTask(ctx, fmt.Sprintf("bench_%d.apk", i))
	}
}

// BenchmarkEndToEnd_ConcurrentTaskCreation 基准测试: 并发任务创建性能
func BenchmarkEndToEnd_ConcurrentTaskCreation(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.Task{})

	taskRepo := repository.NewTaskRepository(db)
	taskService := service.NewTaskService(taskRepo, logger)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			taskService.CreateTask(ctx, fmt.Sprintf("concurrent_bench_%d.apk", i))
			i++
		}
	})
}
