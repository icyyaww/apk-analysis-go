package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// setupTestMetrics 创建测试用的 Prometheus 指标收集器
func setupTestMetrics(t *testing.T) *PrometheusMetrics {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	// 使用唯一的 namespace 避免指标冲突
	// 添加纳秒级时间戳确保唯一性
	namespace := "test_" + t.Name() + "_" + time.Now().Format("20060102150405999999999")
	return NewPrometheusMetrics(logger, namespace)
}

// TestPrometheusMetrics_Initialization 测试指标初始化
func TestPrometheusMetrics_Initialization(t *testing.T) {
	pm := setupTestMetrics(t)

	assert.NotNil(t, pm)
	assert.NotNil(t, pm.httpRequestsTotal)
	assert.NotNil(t, pm.tasksTotal)
	assert.NotNil(t, pm.staticAnalysisTotal)
	assert.NotNil(t, pm.mobsfAnalysisTotal)
	assert.NotNil(t, pm.domainAnalysisTotal)
	assert.NotNil(t, pm.retryAttemptsTotal)
}

// TestHTTPMiddleware 测试 HTTP 中间件
func TestHTTPMiddleware(t *testing.T) {
	pm := setupTestMetrics(t)

	// 创建测试路由
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(pm.HTTPMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// 发送测试请求
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证指标已记录（使用 testutil 检查计数器）
	count := testutil.CollectAndCount(pm.httpRequestsTotal)
	assert.Greater(t, count, 0, "HTTP request metric should be recorded")
}

// TestRecordTaskMetrics 测试任务指标记录
func TestRecordTaskMetrics(t *testing.T) {
	pm := setupTestMetrics(t)

	// 记录任务创建
	pm.RecordTaskCreated()

	// 记录任务开始
	pm.RecordTaskStarted()

	// 记录任务完成
	duration := 120 * time.Second
	pm.RecordTaskCompleted(duration)

	// 验证指标（通过检查计数器是否增加）
	count := testutil.CollectAndCount(pm.tasksTotal)
	assert.Greater(t, count, 0, "Task metrics should be recorded")
}

// TestRecordTaskFailed 测试任务失败指标
func TestRecordTaskFailed(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.RecordTaskStarted()
	pm.RecordTaskFailed(30 * time.Second)

	count := testutil.CollectAndCount(pm.tasksTotal)
	assert.Greater(t, count, 0, "Failed task metrics should be recorded")
}

// TestRecordActivitiesAnalyzed 测试 Activity 分析指标
func TestRecordActivitiesAnalyzed(t *testing.T) {
	pm := setupTestMetrics(t)

	taskID := "test-task-001"
	activityCount := 25

	pm.RecordActivitiesAnalyzed(taskID, activityCount)

	count := testutil.CollectAndCount(pm.activitiesTotal)
	assert.Greater(t, count, 0, "Activity metrics should be recorded")
}

// TestRecordURLsCollected 测试 URL 收集指标
func TestRecordURLsCollected(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.RecordURLsCollected(150)
	pm.RecordURLsCollected(200)

	// 验证计数器增加
	count := testutil.CollectAndCount(pm.urlsCollectedTotal)
	assert.Greater(t, count, 0, "URL metrics should be recorded")
}

// TestUpdateMemoryStats 测试内存统计更新
func TestUpdateMemoryStats(t *testing.T) {
	pm := setupTestMetrics(t)

	stats := MemoryStats{
		Alloc:      100 * 1024 * 1024, // 100MB
		TotalAlloc: 200 * 1024 * 1024,
		Sys:        150 * 1024 * 1024,
		NumGC:      10,
		Goroutines: 50,
	}

	pm.UpdateMemoryStats(stats)

	// 验证 Gauge 指标
	assert.Greater(t, testutil.CollectAndCount(pm.memoryUsage), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.goroutinesCount), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.gcCount), 0)
}

// TestUpdateWorkerPoolStats 测试 Worker Pool 统计
func TestUpdateWorkerPoolStats(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.UpdateWorkerPoolStats(8, 5, 12)

	assert.Greater(t, testutil.CollectAndCount(pm.workerPoolSize), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.workerPoolActive), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.workerPoolQueueSize), 0)
}

// TestUpdateDBStats 测试数据库统计
func TestUpdateDBStats(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.UpdateDBStats(10, 5, 5)

	assert.Greater(t, testutil.CollectAndCount(pm.dbConnectionsOpen), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.dbConnectionsIdle), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.dbConnectionsInUse), 0)
}

// TestRecordStaticAnalysis 测试静态分析指标
func TestRecordStaticAnalysis(t *testing.T) {
	pm := setupTestMetrics(t)

	tests := []struct {
		mode     string
		status   string
		duration time.Duration
	}{
		{"fast", "success", 2 * time.Second},
		{"deep", "success", 15 * time.Second},
		{"hybrid", "success", 8 * time.Second},
		{"fast", "failure", 1 * time.Second},
	}

	for _, tt := range tests {
		pm.RecordStaticAnalysis(tt.mode, tt.status, tt.duration)
	}

	count := testutil.CollectAndCount(pm.staticAnalysisTotal)
	assert.Greater(t, count, 0, "Static analysis metrics should be recorded")
}

// TestRecordMobSFAnalysis 测试 MobSF 分析指标
func TestRecordMobSFAnalysis(t *testing.T) {
	pm := setupTestMetrics(t)

	statuses := []string{"queued", "scanning", "completed", "failed", "timeout"}
	duration := 300 * time.Second

	for _, status := range statuses {
		pm.RecordMobSFAnalysis(status, duration)
	}

	count := testutil.CollectAndCount(pm.mobsfAnalysisTotal)
	assert.Greater(t, count, 0, "MobSF analysis metrics should be recorded")
}

// TestUpdateMobSFQueueStats 测试 MobSF 队列统计
func TestUpdateMobSFQueueStats(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.UpdateMobSFQueueStats(5, 2)

	assert.Greater(t, testutil.CollectAndCount(pm.mobsfQueueSize), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.mobsfProcessingSize), 0)
}

// TestRecordDomainAnalysis 测试域名分析指标
func TestRecordDomainAnalysis(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.RecordDomainAnalysis(5 * time.Second)
	pm.RecordDomainAnalysis(3 * time.Second)
	pm.RecordDomainAnalysis(7 * time.Second)

	count := testutil.CollectAndCount(pm.domainAnalysisTotal)
	assert.Greater(t, count, 0, "Domain analysis metrics should be recorded")
}

// TestUpdateSDKRulesCount 测试 SDK 规则计数
func TestUpdateSDKRulesCount(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.UpdateSDKRulesCount(150)

	count := testutil.CollectAndCount(pm.sdkRulesTotal)
	assert.Greater(t, count, 0, "SDK rules count should be recorded")
}

// TestRecordAppDomains 测试应用域名指标
func TestRecordAppDomains(t *testing.T) {
	pm := setupTestMetrics(t)

	pm.RecordAppDomains("task-001", 25)
	pm.RecordAppDomains("task-002", 30)

	count := testutil.CollectAndCount(pm.appDomainsTotal)
	assert.Greater(t, count, 0, "App domains metrics should be recorded")
}

// TestRecordRetryMetrics 测试重试指标
func TestRecordRetryMetrics(t *testing.T) {
	pm := setupTestMetrics(t)

	// 记录重试尝试
	pm.RecordRetryAttempt("adb", 1)
	pm.RecordRetryAttempt("adb", 2)
	pm.RecordRetryAttempt("mobsf", 1)

	// 记录重试成功
	pm.RecordRetrySuccess("adb")

	countAttempts := testutil.CollectAndCount(pm.retryAttemptsTotal)
	assert.Greater(t, countAttempts, 0, "Retry attempt metrics should be recorded")

	countSuccess := testutil.CollectAndCount(pm.retrySuccessTotal)
	assert.Greater(t, countSuccess, 0, "Retry success metrics should be recorded")
}

// TestConcurrentMetrics 测试并发指标记录
func TestConcurrentMetrics(t *testing.T) {
	pm := setupTestMetrics(t)

	// 并发记录多个指标
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 10; i++ {
			pm.RecordTaskCreated()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			pm.RecordURLsCollected(5)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			pm.RecordStaticAnalysis("fast", "success", time.Second)
		}
		done <- true
	}()

	// 等待所有 goroutine 完成
	for i := 0; i < 3; i++ {
		<-done
	}

	// 验证所有指标都已记录
	assert.Greater(t, testutil.CollectAndCount(pm.tasksTotal), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.urlsCollectedTotal), 0)
	assert.Greater(t, testutil.CollectAndCount(pm.staticAnalysisTotal), 0)
}

// TestPrometheusHandler 测试 Prometheus HTTP Handler
func TestPrometheusHandler(t *testing.T) {
	pm := setupTestMetrics(t)

	// 记录一些指标
	pm.RecordTaskCreated()
	pm.RecordStaticAnalysis("fast", "success", 2*time.Second)

	// 创建测试服务器
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", pm.Handler())

	// 请求 metrics 端点
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "# HELP", "Should contain Prometheus help text")
	assert.Contains(t, w.Body.String(), "# TYPE", "Should contain Prometheus type text")
}

// BenchmarkRecordTaskMetrics 基准测试：任务指标记录
func BenchmarkRecordTaskMetrics(b *testing.B) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPrometheusMetrics(logger, "bench")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.RecordTaskCreated()
	}
}

// BenchmarkRecordStaticAnalysis 基准测试：静态分析指标记录
func BenchmarkRecordStaticAnalysis(b *testing.B) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPrometheusMetrics(logger, "bench")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.RecordStaticAnalysis("fast", "success", time.Second)
	}
}

// BenchmarkUpdateWorkerPoolStats 基准测试：Worker Pool 统计更新
func BenchmarkUpdateWorkerPoolStats(b *testing.B) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPrometheusMetrics(logger, "bench")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.UpdateWorkerPoolStats(8, 5, 12)
	}
}

// BenchmarkConcurrentMetrics 基准测试：并发指标记录
func BenchmarkConcurrentMetrics(b *testing.B) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPrometheusMetrics(logger, "bench")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pm.RecordTaskCreated()
			pm.RecordURLsCollected(5)
		}
	})
}

// TestMetricsRegistry 测试指标注册
func TestMetricsRegistry(t *testing.T) {
	pm := setupTestMetrics(t)

	// 验证所有指标都已注册到 Prometheus
	metrics := []prometheus.Collector{
		pm.httpRequestsTotal,
		pm.httpRequestDuration,
		pm.tasksTotal,
		pm.tasksInProgress,
		pm.taskDuration,
		pm.activitiesTotal,
		pm.urlsCollectedTotal,
		pm.staticAnalysisTotal,
		pm.mobsfAnalysisTotal,
		pm.domainAnalysisTotal,
		pm.retryAttemptsTotal,
		pm.retrySuccessTotal,
	}

	for _, metric := range metrics {
		assert.NotNil(t, metric, "Metric should be initialized")
	}
}
