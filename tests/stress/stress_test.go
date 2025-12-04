package stress

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// StressTestConfig 压力测试配置
type StressTestConfig struct {
	Concurrency       int           // 并发数
	TaskCount         int           // 任务总数
	ProgressUpdates   int           // 每个任务的进度更新次数
	UpdateInterval    time.Duration // 更新间隔
	MaxExecutionTime  time.Duration // 最大执行时间
}

// DefaultStressConfig 默认压力测试配置
var DefaultStressConfig = StressTestConfig{
	Concurrency:      10,
	TaskCount:        100,
	ProgressUpdates:  10,
	UpdateInterval:   10 * time.Millisecond,
	MaxExecutionTime: 30 * time.Second,
}

// StressTestMetrics 压力测试指标
type StressTestMetrics struct {
	TotalTasks        int64
	SuccessfulTasks   int64
	FailedTasks       int64
	TotalDuration     time.Duration
	AverageLatency    time.Duration
	MaxLatency        time.Duration
	MinLatency        time.Duration
	ThroughputPerSec  float64
	ErrorRate         float64
}

// setupStressTestEnv 创建压力测试环境
func setupStressTestEnv(t *testing.T) (service.TaskService, func()) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&domain.Task{},
		&domain.TaskActivity{},
		&domain.TaskMobSFReport{},
		&domain.TaskDomainAnalysis{},
		&domain.TaskAppDomain{},
		&domain.TaskAILog{},
		&domain.ThirdPartySDKRule{},
	)
	require.NoError(t, err)

	taskRepo := repository.NewTaskRepository(db)
	taskService := service.NewTaskService(taskRepo, logger)

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	return taskService, cleanup
}

// TestStress_10ConcurrentTasks 压力测试: 10 个并发任务
func TestStress_10ConcurrentTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	config := StressTestConfig{
		Concurrency:     10,
		TaskCount:       10,
		ProgressUpdates: 10,
		UpdateInterval:  5 * time.Millisecond,
	}

	metrics := runStressTest(t, taskService, config)

	// 验证结果
	assert.Equal(t, int64(10), metrics.SuccessfulTasks)
	assert.Equal(t, int64(0), metrics.FailedTasks)
	assert.Less(t, metrics.AverageLatency, 1*time.Second)

	t.Logf("✅ 10 Concurrent Tasks - Success: %d, Failed: %d, Avg Latency: %v",
		metrics.SuccessfulTasks, metrics.FailedTasks, metrics.AverageLatency)
}

// TestStress_50ConcurrentTasks 压力测试: 50 个并发任务
func TestStress_50ConcurrentTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	config := StressTestConfig{
		Concurrency:     50,
		TaskCount:       50,
		ProgressUpdates: 10,
		UpdateInterval:  5 * time.Millisecond,
	}

	metrics := runStressTest(t, taskService, config)

	assert.Equal(t, int64(50), metrics.SuccessfulTasks)
	assert.Equal(t, int64(0), metrics.FailedTasks)

	t.Logf("✅ 50 Concurrent Tasks - Success: %d, Failed: %d, Avg Latency: %v, Throughput: %.2f tasks/sec",
		metrics.SuccessfulTasks, metrics.FailedTasks, metrics.AverageLatency, metrics.ThroughputPerSec)
}

// TestStress_100ConcurrentTasks 压力测试: 100 个并发任务
func TestStress_100ConcurrentTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	config := StressTestConfig{
		Concurrency:     100,
		TaskCount:       100,
		ProgressUpdates: 5,
		UpdateInterval:  10 * time.Millisecond,
	}

	metrics := runStressTest(t, taskService, config)

	assert.Equal(t, int64(100), metrics.SuccessfulTasks)
	assert.Less(t, metrics.ErrorRate, 0.01) // 错误率 < 1%

	t.Logf("✅ 100 Concurrent Tasks - Success: %d, Failed: %d, Throughput: %.2f tasks/sec",
		metrics.SuccessfulTasks, metrics.FailedTasks, metrics.ThroughputPerSec)
}

// TestStress_SustainedLoad 压力测试: 持续负载 (200 任务, 10 并发)
func TestStress_SustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	config := StressTestConfig{
		Concurrency:     10,
		TaskCount:       200,
		ProgressUpdates: 10,
		UpdateInterval:  5 * time.Millisecond,
	}

	metrics := runStressTest(t, taskService, config)

	assert.Equal(t, int64(200), metrics.SuccessfulTasks)
	assert.Less(t, metrics.AverageLatency, 2*time.Second)

	t.Logf("✅ Sustained Load - Success: %d, Total Duration: %v, Throughput: %.2f tasks/sec",
		metrics.SuccessfulTasks, metrics.TotalDuration, metrics.ThroughputPerSec)
}

// TestStress_HighFrequencyUpdates 压力测试: 高频更新
func TestStress_HighFrequencyUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	config := StressTestConfig{
		Concurrency:     10,
		TaskCount:       10,
		ProgressUpdates: 100, // 每个任务 100 次更新
		UpdateInterval:  1 * time.Millisecond,
	}

	metrics := runStressTest(t, taskService, config)

	assert.Equal(t, int64(10), metrics.SuccessfulTasks)

	t.Logf("✅ High Frequency Updates - Success: %d, Total Updates: %d, Avg Latency: %v",
		metrics.SuccessfulTasks, config.TaskCount*config.ProgressUpdates, metrics.AverageLatency)
}

// TestStress_RapidTaskCreation 压力测试: 快速任务创建
func TestStress_RapidTaskCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	taskCount := 1000
	var successCount int64
	var failCount int64

	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			_, err := taskService.CreateTask(ctx, fmt.Sprintf("rapid_%d.apk", index))
			if err != nil {
				atomic.AddInt64(&failCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)
	throughput := float64(successCount) / duration.Seconds()

	assert.Equal(t, int64(taskCount), successCount)
	assert.Equal(t, int64(0), failCount)

	t.Logf("✅ Rapid Task Creation - Created: %d, Duration: %v, Throughput: %.2f tasks/sec",
		successCount, duration, throughput)
}

// TestStress_MixedOperations 压力测试: 混合操作 (创建/读取/更新/删除)
func TestStress_MixedOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	operationCount := 500
	concurrency := 20

	var createCount, readCount, updateCount, deleteCount int64
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)

	startTime := time.Now()

	for i := 0; i < operationCount; i++ {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(index int) {
			defer wg.Done()
			defer func() { <-semaphore }()

			operation := index % 4

			switch operation {
			case 0: // Create
				_, err := taskService.CreateTask(ctx, fmt.Sprintf("mixed_%d.apk", index))
				if err == nil {
					atomic.AddInt64(&createCount, 1)
				}

			case 1: // Read
				tasks, err := taskService.ListRecentTasks(ctx, 10)
				if err == nil && len(tasks) > 0 {
					atomic.AddInt64(&readCount, 1)
				}

			case 2: // Update
				tasks, err := taskService.ListRecentTasks(ctx, 1)
				if err == nil && len(tasks) > 0 {
					err = taskService.UpdateTaskProgress(ctx, tasks[0].ID, 50, "Testing")
					if err == nil {
						atomic.AddInt64(&updateCount, 1)
					}
				}

			case 3: // Delete
				tasks, err := taskService.ListRecentTasks(ctx, 1)
				if err == nil && len(tasks) > 0 {
					err = taskService.DeleteTask(ctx, tasks[0].ID)
					if err == nil {
						atomic.AddInt64(&deleteCount, 1)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	t.Logf("✅ Mixed Operations - Create: %d, Read: %d, Update: %d, Delete: %d, Duration: %v",
		createCount, readCount, updateCount, deleteCount, duration)
}

// TestStress_MemoryUsage 压力测试: 内存使用测试
func TestStress_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	taskService, cleanup := setupStressTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	taskCount := 1000

	// 创建大量任务
	for i := 0; i < taskCount; i++ {
		_, err := taskService.CreateTask(ctx, fmt.Sprintf("memory_test_%d.apk", i))
		require.NoError(t, err)
	}

	// 读取所有任务
	tasks, err := taskService.ListRecentTasks(ctx, taskCount)
	require.NoError(t, err)
	assert.Len(t, tasks, taskCount)

	t.Logf("✅ Memory Usage Test - Created and retrieved %d tasks", taskCount)
}

// runStressTest 运行压力测试的通用函数
func runStressTest(t *testing.T, taskService service.TaskService, config StressTestConfig) *StressTestMetrics {
	ctx := context.Background()

	var successCount, failCount int64
	latencies := make([]time.Duration, config.TaskCount)
	var wg sync.WaitGroup

	startTime := time.Now()

	// 限制并发数
	semaphore := make(chan struct{}, config.Concurrency)

	for i := 0; i < config.TaskCount; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(index int) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			taskStart := time.Now()

			// 1. 创建任务
			task, err := taskService.CreateTask(ctx, fmt.Sprintf("stress_%d.apk", index))
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				return
			}

			// 2. 更新状态为 Running
			err = taskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusRunning)
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				return
			}

			// 3. 模拟进度更新
			for progress := 0; progress < config.ProgressUpdates; progress++ {
				percent := (progress + 1) * (100 / config.ProgressUpdates)
				step := fmt.Sprintf("Step %d/%d", progress+1, config.ProgressUpdates)

				err = taskService.UpdateTaskProgress(ctx, task.ID, percent, step)
				if err != nil {
					atomic.AddInt64(&failCount, 1)
					return
				}

				time.Sleep(config.UpdateInterval)
			}

			// 4. 完成任务
			err = taskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusCompleted)
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				return
			}

			latency := time.Since(taskStart)
			latencies[index] = latency
			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// 计算指标
	metrics := calculateMetrics(successCount, failCount, totalDuration, latencies)
	return metrics
}

// calculateMetrics 计算压力测试指标
func calculateMetrics(successCount, failCount int64, totalDuration time.Duration, latencies []time.Duration) *StressTestMetrics {
	totalTasks := successCount + failCount

	var totalLatency time.Duration
	var maxLatency time.Duration
	minLatency := time.Duration(1<<63 - 1) // Max duration

	for _, latency := range latencies {
		if latency > 0 {
			totalLatency += latency
			if latency > maxLatency {
				maxLatency = latency
			}
			if latency < minLatency {
				minLatency = latency
			}
		}
	}

	var averageLatency time.Duration
	if successCount > 0 {
		averageLatency = totalLatency / time.Duration(successCount)
	}

	throughput := float64(successCount) / totalDuration.Seconds()
	errorRate := float64(failCount) / float64(totalTasks)

	return &StressTestMetrics{
		TotalTasks:       totalTasks,
		SuccessfulTasks:  successCount,
		FailedTasks:      failCount,
		TotalDuration:    totalDuration,
		AverageLatency:   averageLatency,
		MaxLatency:       maxLatency,
		MinLatency:       minLatency,
		ThroughputPerSec: throughput,
		ErrorRate:        errorRate,
	}
}

// BenchmarkStress_TaskLifecycle 基准测试: 完整任务生命周期
func BenchmarkStress_TaskLifecycle(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.Task{})

	taskRepo := repository.NewTaskRepository(db)
	taskService := service.NewTaskService(taskRepo, logger)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task, _ := taskService.CreateTask(ctx, fmt.Sprintf("bench_%d.apk", i))
		taskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusRunning)
		taskService.UpdateTaskProgress(ctx, task.ID, 50, "Processing")
		taskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusCompleted)
	}
}

// BenchmarkStress_ConcurrentTaskLifecycle 基准测试: 并发任务生命周期
func BenchmarkStress_ConcurrentTaskLifecycle(b *testing.B) {
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
			task, _ := taskService.CreateTask(ctx, fmt.Sprintf("concurrent_bench_%d.apk", i))
			taskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusRunning)
			taskService.UpdateTaskProgress(ctx, task.ID, 50, "Processing")
			taskService.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusCompleted)
			i++
		}
	})
}
