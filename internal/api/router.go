package api

import (
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/api/handlers"
	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/device"
	"github.com/apk-analysis/apk-analysis-go/internal/middleware"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func SetupRouter(cfg *config.Config, logger *logrus.Logger, db *gorm.DB, memMonitor *middleware.MemoryMonitor, promMetrics *middleware.PrometheusMetrics, deviceMgr *device.DeviceManager, aiInteractionHandler *handlers.AIInteractionHandler) *gin.Engine {
	// 设置 Gin 模式
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// 全局中间件
	r.Use(gin.Recovery())
	r.Use(LoggerMiddleware(logger))
	r.Use(CORSMiddleware())

	// Prometheus 监控中间件
	if promMetrics != nil {
		r.Use(promMetrics.HTTPMiddleware())
	}

	// 静态资源
	r.Static("/static", "./web/static")
	r.LoadHTMLGlob("./web/templates/*")

	// 初始化依赖
	taskRepo := repository.NewTaskRepository(db, logger)
	sdkRepo := repository.NewSDKRepository(db)
	taskService := service.NewTaskService(taskRepo, logger)

	// 初始化处理器
	taskHandler := handlers.NewTaskHandler(taskService, logger)
	fileHandler := handlers.NewFileHandler(taskService, logger, "./results", "./inbound_apks")
	sdkHandler := handlers.NewSDKHandler(sdkRepo, logger)
	certHandler := handlers.NewCertHandler(cfg.ADB.Target, logger)
	authHandler := handlers.NewAuthHandler(logger)
	// aiInteractionHandler 已在main.go中创建并传入，直接使用

	// 登录页面（无需认证）
	r.GET("/login", func(c *gin.Context) {
		c.HTML(200, "login.html", gin.H{
			"title": "登录 - APK Analysis Platform",
		})
	})

	// HTML 页面（需要认证，但认证在前端 JS 处理）
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "dashboard.html", gin.H{
			"title": "APK Analysis Dashboard",
		})
	})

	// 任务详情页面
	r.GET("/tasks/:id", func(c *gin.Context) {
		c.HTML(200, "task_detail.html", gin.H{
			"title": "任务详情",
		})
	})

	// URL 分析页面
	r.GET("/tasks/:id/urls", func(c *gin.Context) {
		c.HTML(200, "url_analysis.html", gin.H{
			"title": "URL 分析",
		})
	})

	// AI交互监控页面
	r.GET("/ai-interaction", aiInteractionHandler.GetAIInteractionPage)
	r.GET("/ws/ai-interaction/:task_id", aiInteractionHandler.HandleWebSocket)

	// 性能监控端点 (仅在非生产环境)
	if cfg.Server.Mode != "release" {
		middleware.RegisterPprof(r)
		logger.Info("pprof endpoints registered at /debug/pprof/*")
	}

	// 内存监控端点
	r.GET("/metrics", memMonitor.MetricsEndpoint())
	r.POST("/debug/gc", middleware.ForceGC())

	// Prometheus 指标端点
	if promMetrics != nil {
		r.GET("/metrics/prometheus", promMetrics.Handler())
	}

	// API v1
	v1 := r.Group("/api")
	{
		// 健康检查（无需认证）
		v1.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"version": "1.0.0",
			})
		})

		// 登录接口（无需认证）
		v1.POST("/login", authHandler.Login)

		// Token 验证接口（无需认证，用于前端检查 token 有效性）
		v1.GET("/auth/validate", authHandler.ValidateToken)

		// 系统统计
		v1.GET("/stats", taskHandler.GetSystemStats)

		// 任务管理
		v1.GET("/tasks", taskHandler.ListTasks)
		v1.GET("/tasks/export", taskHandler.ExportTasks)     // 导出任务（不分页，最大10000条）
		v1.GET("/tasks/queued", taskHandler.ListQueuedTasks) // 获取所有排队任务（不分页）
		v1.DELETE("/tasks/batch", taskHandler.BatchDeleteTasks) // 批量删除必须在 :id 之前
		v1.GET("/tasks/:id", taskHandler.GetTask)
		v1.DELETE("/tasks/:id", taskHandler.DeleteTask)
		v1.POST("/tasks/:id/stop", taskHandler.StopTask)

		// 流量分析
		v1.GET("/tasks/:id/urls", taskHandler.GetTaskURLs)
		v1.GET("/tasks/:id/activities/:name/urls", taskHandler.GetActivityURLs)
		v1.GET("/tasks/:id/activities/report", taskHandler.GetActivitiesReport)

		// 文件服务
		v1.POST("/upload", fileHandler.UploadAPK)           // 单个 APK 上传
		v1.POST("/upload/batch", fileHandler.UploadAPKBatch) // 批量 APK 上传
		v1.GET("/tasks/:id/screenshot/:filename", fileHandler.GetScreenshot)
		v1.GET("/tasks/:id/screenshots", fileHandler.ListScreenshots)
		v1.GET("/tasks/:id/ui_hierarchy/:filename", fileHandler.GetUIHierarchy)
		v1.GET("/tasks/:id/flows", fileHandler.DownloadFlows)

		// 静态分析报告（Hybrid Analyzer）
		v1.GET("/tasks/:id/static", taskHandler.GetStaticReport)        // JSON 格式的静态分析报告
		v1.GET("/tasks/:id/static/report", taskHandler.GetStaticReportHTML) // HTML 格式的静态分析报告

		// 设备状态监控
		v1.GET("/devices", func(c *gin.Context) {
			stats := deviceMgr.GetDeviceStats()
			c.JSON(200, stats)
		})

		// 证书状态检查
		v1.GET("/cert/status", certHandler.GetCertStatus)

		// SDK 规则管理
		v1.GET("/sdk_rules", sdkHandler.ListSDKRules)
		v1.POST("/sdk_rules", sdkHandler.CreateSDKRule)
		v1.GET("/sdk_rules/pending", sdkHandler.GetPendingSDKRules)
		v1.GET("/sdk_rules/statistics", sdkHandler.GetSDKStatistics)
		v1.GET("/sdk_rules/categories", sdkHandler.GetSDKCategories)
		v1.POST("/sdk_rules/:id/approve", sdkHandler.ApproveSDKRule)
		v1.POST("/sdk_rules/:id/reject", sdkHandler.RejectSDKRule)
		v1.PUT("/sdk_rules/:id", sdkHandler.UpdateSDKRule)
		v1.DELETE("/sdk_rules/:id", sdkHandler.DeleteSDKRule)
	}

	return r
}

// LoggerMiddleware 日志中间件
func LoggerMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		c.Next()

		latency := time.Since(startTime)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		logger.WithFields(logrus.Fields{
			"status":  statusCode,
			"method":  method,
			"path":    path,
			"latency": latency.Milliseconds(),
		}).Info("HTTP Request")
	}
}

// CORSMiddleware CORS 中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
