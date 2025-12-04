package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// PrometheusMetrics Prometheus 指标收集器
type PrometheusMetrics struct {
	logger *logrus.Logger

	// HTTP 请求指标
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec

	// 业务指标
	tasksTotal        *prometheus.CounterVec
	tasksInProgress   prometheus.Gauge
	taskDuration      *prometheus.HistogramVec
	activitiesTotal   *prometheus.CounterVec
	urlsCollectedTotal prometheus.Counter

	// 系统指标
	memoryUsage    prometheus.Gauge
	goroutinesCount prometheus.Gauge
	gcCount        prometheus.Gauge

	// Worker Pool 指标
	workerPoolSize      prometheus.Gauge
	workerPoolActive    prometheus.Gauge
	workerPoolQueueSize prometheus.Gauge

	// 数据库指标
	dbConnectionsOpen  prometheus.Gauge
	dbConnectionsIdle  prometheus.Gauge
	dbConnectionsInUse prometheus.Gauge

	// 静态分析指标
	staticAnalysisTotal    *prometheus.CounterVec
	staticAnalysisDuration *prometheus.HistogramVec

	// 域名分析指标
	domainAnalysisTotal    prometheus.Counter
	domainAnalysisDuration prometheus.Histogram
	sdkRulesTotal          prometheus.Gauge
	appDomainsTotal        *prometheus.CounterVec

	// 重试指标
	retryAttemptsTotal *prometheus.CounterVec
	retrySuccessTotal  *prometheus.CounterVec
}

// NewPrometheusMetrics 创建 Prometheus 指标收集器
func NewPrometheusMetrics(logger *logrus.Logger, namespace string) *PrometheusMetrics {
	if namespace == "" {
		namespace = "apk_analysis"
	}

	pm := &PrometheusMetrics{
		logger: logger,

		// HTTP 请求指标
		httpRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		httpRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latencies in seconds",
				Buckets:   []float64{0.001, 0.01, 0.05, 0.1, 0.5, 1, 5, 10},
			},
			[]string{"method", "path"},
		),

		// 业务指标
		tasksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tasks_total",
				Help:      "Total number of analysis tasks",
			},
			[]string{"status"}, // queued, running, completed, failed
		),
		tasksInProgress: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "tasks_in_progress",
				Help:      "Number of tasks currently in progress",
			},
		),
		taskDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "task_duration_seconds",
				Help:      "Task execution duration in seconds",
				Buckets:   []float64{10, 30, 60, 120, 300, 600, 1200, 1800},
			},
			[]string{"status"},
		),
		activitiesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "activities_total",
				Help:      "Total number of activities analyzed",
			},
			[]string{"task_id"},
		),
		urlsCollectedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "urls_collected_total",
				Help:      "Total number of URLs collected",
			},
		),

		// 系统指标
		memoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_usage_bytes",
				Help:      "Current memory usage in bytes",
			},
		),
		goroutinesCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "goroutines_count",
				Help:      "Current number of goroutines",
			},
		),
		gcCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "gc_count",
				Help:      "Number of completed GC cycles",
			},
		),

		// Worker Pool 指标
		workerPoolSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "worker_pool_size",
				Help:      "Total number of workers in the pool",
			},
		),
		workerPoolActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "worker_pool_active",
				Help:      "Number of active workers",
			},
		),
		workerPoolQueueSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "worker_pool_queue_size",
				Help:      "Number of tasks waiting in queue",
			},
		),

		// 数据库指标
		dbConnectionsOpen: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_open",
				Help:      "Number of open database connections",
			},
		),
		dbConnectionsIdle: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_idle",
				Help:      "Number of idle database connections",
			},
		),
		dbConnectionsInUse: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_in_use",
				Help:      "Number of database connections in use",
			},
		),

		// 静态分析指标
		staticAnalysisTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "static_analysis_total",
				Help:      "Total number of static analyses performed",
			},
			[]string{"mode", "status"}, // mode: fast/deep/hybrid, status: success/failure
		),
		staticAnalysisDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "static_analysis_duration_seconds",
				Help:      "Static analysis duration in seconds",
				Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"mode"},
		),

		// 域名分析指标
		domainAnalysisTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "domain_analysis_total",
				Help:      "Total number of domain analyses performed",
			},
		),
		domainAnalysisDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "domain_analysis_duration_seconds",
				Help:      "Domain analysis duration in seconds",
				Buckets:   []float64{0.5, 1, 2, 5, 10, 20, 30},
			},
		),
		sdkRulesTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "sdk_rules_total",
				Help:      "Total number of active SDK rules",
			},
		),
		appDomainsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "app_domains_total",
				Help:      "Total number of domains analyzed per task",
			},
			[]string{"task_id"},
		),

		// 重试指标
		retryAttemptsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "retry_attempts_total",
				Help:      "Total number of retry attempts",
			},
			[]string{"operation", "attempt"}, // operation: adb/mobsf/db, attempt: 1/2/3
		),
		retrySuccessTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "retry_success_total",
				Help:      "Total number of successful retries",
			},
			[]string{"operation"},
		),
	}

	logger.Info("Prometheus metrics initialized")
	return pm
}

// HTTPMiddleware HTTP 请求监控中间件
func (pm *PrometheusMetrics) HTTPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 处理请求
		c.Next()

		// 记录指标
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		pm.httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		pm.httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

// Handler 返回 Prometheus HTTP Handler
func (pm *PrometheusMetrics) Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// RecordTaskCreated 记录任务创建
func (pm *PrometheusMetrics) RecordTaskCreated() {
	pm.tasksTotal.WithLabelValues("queued").Inc()
}

// RecordTaskStarted 记录任务开始
func (pm *PrometheusMetrics) RecordTaskStarted() {
	pm.tasksTotal.WithLabelValues("running").Inc()
	pm.tasksInProgress.Inc()
}

// RecordTaskCompleted 记录任务完成
func (pm *PrometheusMetrics) RecordTaskCompleted(duration time.Duration) {
	pm.tasksTotal.WithLabelValues("completed").Inc()
	pm.tasksInProgress.Dec()
	pm.taskDuration.WithLabelValues("completed").Observe(duration.Seconds())
}

// RecordTaskFailed 记录任务失败
func (pm *PrometheusMetrics) RecordTaskFailed(duration time.Duration) {
	pm.tasksTotal.WithLabelValues("failed").Inc()
	pm.tasksInProgress.Dec()
	pm.taskDuration.WithLabelValues("failed").Observe(duration.Seconds())
}

// RecordActivitiesAnalyzed 记录 Activity 分析数量
func (pm *PrometheusMetrics) RecordActivitiesAnalyzed(taskID string, count int) {
	pm.activitiesTotal.WithLabelValues(taskID).Add(float64(count))
}

// RecordURLsCollected 记录收集的 URL 数量
func (pm *PrometheusMetrics) RecordURLsCollected(count int) {
	pm.urlsCollectedTotal.Add(float64(count))
}

// UpdateMemoryStats 更新内存统计
func (pm *PrometheusMetrics) UpdateMemoryStats(stats MemoryStats) {
	pm.memoryUsage.Set(float64(stats.Alloc))
	pm.goroutinesCount.Set(float64(stats.Goroutines))
	pm.gcCount.Set(float64(stats.NumGC))
}

// UpdateWorkerPoolStats 更新 Worker Pool 统计
func (pm *PrometheusMetrics) UpdateWorkerPoolStats(size, active, queueSize int) {
	pm.workerPoolSize.Set(float64(size))
	pm.workerPoolActive.Set(float64(active))
	pm.workerPoolQueueSize.Set(float64(queueSize))
}

// UpdateDBStats 更新数据库连接统计
func (pm *PrometheusMetrics) UpdateDBStats(open, idle, inUse int) {
	pm.dbConnectionsOpen.Set(float64(open))
	pm.dbConnectionsIdle.Set(float64(idle))
	pm.dbConnectionsInUse.Set(float64(inUse))
}

// RecordStaticAnalysis 记录静态分析
func (pm *PrometheusMetrics) RecordStaticAnalysis(mode string, status string, duration time.Duration) {
	pm.staticAnalysisTotal.WithLabelValues(mode, status).Inc()
	pm.staticAnalysisDuration.WithLabelValues(mode).Observe(duration.Seconds())
}

// RecordDomainAnalysis 记录域名分析
func (pm *PrometheusMetrics) RecordDomainAnalysis(duration time.Duration) {
	pm.domainAnalysisTotal.Inc()
	pm.domainAnalysisDuration.Observe(duration.Seconds())
}

// UpdateSDKRulesCount 更新 SDK 规则数量
func (pm *PrometheusMetrics) UpdateSDKRulesCount(count int) {
	pm.sdkRulesTotal.Set(float64(count))
}

// RecordAppDomains 记录应用域名数量
func (pm *PrometheusMetrics) RecordAppDomains(taskID string, count int) {
	pm.appDomainsTotal.WithLabelValues(taskID).Add(float64(count))
}

// RecordRetryAttempt 记录重试尝试
func (pm *PrometheusMetrics) RecordRetryAttempt(operation string, attempt int) {
	pm.retryAttemptsTotal.WithLabelValues(operation, strconv.Itoa(attempt)).Inc()
}

// RecordRetrySuccess 记录重试成功
func (pm *PrometheusMetrics) RecordRetrySuccess(operation string) {
	pm.retrySuccessTotal.WithLabelValues(operation).Inc()
}
