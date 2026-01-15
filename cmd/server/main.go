package main
// Force rebuild: 2025-11-25-v1

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/adb"
	"github.com/apk-analysis/apk-analysis-go/internal/api"
	"github.com/apk-analysis/apk-analysis-go/internal/api/handlers"
	"github.com/apk-analysis/apk-analysis-go/internal/cert"
	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/device"
	"github.com/apk-analysis/apk-analysis-go/internal/domainanalysis"
	"github.com/apk-analysis/apk-analysis-go/internal/malware"
	"github.com/apk-analysis/apk-analysis-go/internal/middleware"
	"github.com/apk-analysis/apk-analysis-go/internal/queue"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/apk-analysis/apk-analysis-go/internal/utils"
	"github.com/apk-analysis/apk-analysis-go/internal/watcher"
	"github.com/apk-analysis/apk-analysis-go/internal/worker"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// 1. 打印版本信息
	fmt.Printf("APK Analysis Platform - Go Version\n")
	fmt.Printf("Version: %s\n", Version)
	fmt.Printf("Build Time: %s\n", BuildTime)
	fmt.Printf("Git Commit: %s\n\n", GitCommit)

	// 2. 加载配置
	configPath := "./configs/config.yaml"
	if len(os.Args) > 1 && os.Args[1] == "--config" && len(os.Args) > 2 {
		configPath = os.Args[2]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 3. 初始化日志
	logger := config.InitLogger(&cfg.Log)
	logger.Infof("Starting APK Analysis Platform %s", Version)
	logger.Infof("Config loaded from: %s", configPath)

	// 4. 初始化数据库
	db, err := repository.InitDB(&cfg.Database, logger)
	if err != nil {
		logger.Fatalf("Failed to init database: %v", err)
	}
	logger.Info("Database connected successfully")

	// 优化数据库连接池
	if err := utils.OptimizeDBPool(db); err != nil {
		logger.WithError(err).Warn("Failed to optimize DB pool")
	} else {
		logger.Info("Database connection pool optimized")
	}

	// 清理因服务重启而中断的任务
	if err := cleanupStuckTasks(db, logger); err != nil {
		logger.WithError(err).Warn("Failed to cleanup stuck tasks")
	}

	// 5. 初始化设备管理器 (Device Manager) - 双设备模式
	deviceMgr := device.NewDeviceManager(logger)

	// Device 1: OnePlus 5T (Android 10, Magisk root, 系统证书已安装)
	// ADB 端口固定为 5555 (标准 tcpip 模式)
	device1 := &device.Device{
		ID:                 "oneplus-5t",
		ADBTarget:          "192.168.2.100:5555",              // WiFi ADB地址（固定端口）
		ProxyHost:          "192.168.2.188",                   // 宿主机IP（服务器地址）
		ProxyPort:          8082,                              // mitmproxy-1 代理端口
		MitmproxyContainer: "apk-analysis-mitmproxy-1",        // Mitmproxy容器名称
		MitmproxyAPIPort:   8083,                              // mitmproxy-1 API端口
		FridaHost:          "192.168.2.100:27042",             // Frida WiFi连接地址
		Arch:               device.ArchARM,                    // ARM 架构真机
	}
	deviceMgr.AddDevice(device1)

	logger.Info("BUILD_VERSION_20251215_SINGLE_DEVICE_ONEPLUS") // 编译版本标记

	// 配置设备休息参数（每执行10个任务，休息30秒）
	deviceMgr.ConfigureDeviceRest(10, 30*time.Second)

	logger.WithFields(logrus.Fields{
		"device_count": deviceMgr.GetDeviceCount(),
		"devices": []map[string]interface{}{
			{"id": device1.ID, "adb": device1.ADBTarget, "frida_host": device1.FridaHost, "proxy_port": device1.ProxyPort},
		},
	}).Info("Device manager initialized with 1 device (OnePlus 5T)")

	// 5.5 为所有设备初始化 mitmproxy 证书（带重试机制）
	logger.Info("Initializing mitmproxy certificates for all devices...")
	devices := []*device.Device{device1}

	// 启动异步证书安装任务（避免阻塞服务启动）
	go func() {
		for _, dev := range devices {
			// 每个设备使用独立的 goroutine，支持并发安装
			go func(d *device.Device) {
				maxRetries := 3
				retryDelay := 30 * time.Second

				for attempt := 1; attempt <= maxRetries; attempt++ {
					logger.WithFields(logrus.Fields{
						"device_id":           d.ID,
						"adb_target":          d.ADBTarget,
						"mitmproxy_container": d.MitmproxyContainer,
						"attempt":             attempt,
						"max_retries":         maxRetries,
					}).Info("Installing certificate for device...")

					certInstaller := cert.NewInstaller(d.ADBTarget, logger)
					certCtx, certCancel := context.WithTimeout(context.Background(), 5*time.Minute)

					err := certInstaller.PrepareAndInstall(certCtx, d.MitmproxyContainer)
					certCancel()

					if err == nil {
						logger.WithField("device_id", d.ID).Info("✅ Certificate installed successfully on startup")
						break
					}

					logger.WithError(err).WithFields(logrus.Fields{
						"device_id": d.ID,
						"attempt":   attempt,
					}).Warn("Failed to install certificate")

					// 如果不是最后一次尝试，等待后重试
					if attempt < maxRetries {
						logger.WithFields(logrus.Fields{
							"device_id":   d.ID,
							"retry_delay": retryDelay.String(),
						}).Info("Waiting before retry...")
						time.Sleep(retryDelay)
					} else {
						logger.WithField("device_id", d.ID).Error("❌ Certificate installation failed after all retries, device health check will retry later")
					}
				}
			}(dev)
		}
	}()

	logger.Info("Certificate initialization started in background (will retry if needed)")

	// 6. 初始化 RabbitMQ
	// 使用 NewRabbitMQWithPrefetch，prefetch count = worker concurrency，以支持并行消费
	mqConfig := &queue.RabbitMQConfig{
		Host:     cfg.RabbitMQ.Host,
		Port:     cfg.RabbitMQ.Port,
		User:     cfg.RabbitMQ.User,
		Password: cfg.RabbitMQ.Password,
		VHost:    cfg.RabbitMQ.VHost,
	}
	workerCount := cfg.Worker.Concurrency
	if workerCount <= 0 {
		workerCount = 1
	}
	mq, err := queue.NewRabbitMQWithPrefetch(mqConfig, cfg.RabbitMQ.Queue, workerCount, logger)
	if err != nil {
		logger.Fatalf("Failed to init RabbitMQ: %v", err)
	}
	defer mq.Close()
	logger.WithField("prefetch_count", workerCount).Info("RabbitMQ connected successfully")

	// 6. 初始化 Services
	taskRepo := repository.NewTaskRepository(db, logger)
	staticReportRepo := repository.NewStaticReportRepository(db)
	malwareRepo := repository.NewMalwareRepository(db, logger)
	taskService := service.NewTaskService(taskRepo, logger)

	// 7. 静态分析使用 Hybrid 模式（Go + Androguard）
	hybridEnabled := cfg.StaticAnalysis.Hybrid.Enabled
	logger.WithField("hybrid_enabled", hybridEnabled).Info("Static analysis using Hybrid mode (Go + Androguard)")

	// 注意: 域名分析服务需要在设置回调前初始化
	// 使用配置创建域名分析服务 (包含备案查询配置)
	beianConfig := &domainanalysis.BeianCheckerConfig{
		Enabled:    cfg.Beian.Enabled,
		APIKey:     cfg.Beian.APIKey,
		APIURL:     cfg.Beian.APIURL,
		APIVersion: cfg.Beian.APIVersion,
		Timeout:    cfg.Beian.Timeout,
	}

	// 如果配置为空，使用默认值
	if beianConfig.APIURL == "" {
		beianConfig.APIURL = "http://openapiu67.chinaz.net/v1/1001/icpappunit"
	}
	if beianConfig.APIVersion == "" {
		beianConfig.APIVersion = "1.0"
	}
	if beianConfig.Timeout <= 0 {
		beianConfig.Timeout = 70
	}

	// 🔧 修复：传递 resultsDir 参数给域名分析服务，用于正确读取 flows.jsonl
	domainService := domainanalysis.NewAnalysisServiceWithConfig(db, taskRepo, logger, beianConfig, "./results")
	logger.WithFields(logrus.Fields{
		"beian_enabled": beianConfig.Enabled,
		"beian_api_url": beianConfig.APIURL,
	}).Info("Domain analysis service initialized")

	// 7.5. 初始化AI交互处理器（用于实时展示AI点击过程）
	// 注意：这必须在Orchestrator初始化之前创建
	aiInteractionHandler := handlers.NewAIInteractionHandler(logger)
	aiInteractionHandler.Start() // 启动WebSocket广播服务
	logger.Info("AI interaction handler started for real-time visualization")

	// 8. 初始化核心编排器 (Orchestrator)
	// 注意：双模拟器模式下，mitmproxy的配置已经在Device对象中（每个设备有独立的代理端口）
	// 这里的mitmProxyHost仅用于向后兼容，实际使用设备的ProxyPort进行流量隔离
	mitmProxyHost := "apk-analysis-mitmproxy-1" // 默认使用第一个mitmproxy（仅用于API调用）
	resultsDir := "./results"
	os.MkdirAll(resultsDir, 0755)

	orchestrator := worker.NewOrchestrator(deviceMgr, taskRepo, staticReportRepo, malwareRepo, cfg, logger, resultsDir, mitmProxyHost)

	// 8.2. 设置AI交互广播器（用于实时推送AI动作到前端）
	aiBroadcaster := handlers.NewAIBroadcasterAdapter(aiInteractionHandler)
	orchestrator.SetAIInteractionBroadcaster(aiBroadcaster)
	logger.Info("AI interaction broadcaster connected to orchestrator")

	// 8.1. 为 Orchestrator 设置域名分析回调
	// 任务完成后，回调由 Orchestrator 直接触发域名分析
	orchestrator.SetDomainAnalysisCallback(func(taskID string) {
		// 🔧 添加 panic 恢复机制，防止 goroutine 静默崩溃
		defer func() {
			if r := recover(); r != nil {
				logger.WithFields(logrus.Fields{
					"task_id": taskID,
					"panic":   r,
				}).Error("❌ PANIC in domain analysis callback (goroutine recovered)")
			}
		}()

		logger.WithField("task_id", taskID).Info("Starting domain analysis...")
		if err := domainService.AnalyzeTask(context.Background(), taskID); err != nil {
			logger.WithError(err).WithField("task_id", taskID).Warn("Domain analysis failed")
			// 即使域名分析失败，也标记任务完成（避免任务永远卡在95%）
			if err := taskRepo.MarkTaskFullyCompleted(context.Background(), taskID); err != nil {
				logger.WithError(err).WithField("task_id", taskID).Error("Failed to mark task completed after domain analysis failure")
			}
		} else {
			logger.WithField("task_id", taskID).Info("Domain analysis completed successfully")
			// 🔧 域名分析成功后，标记任务真正完成（100%）
			if err := taskRepo.MarkTaskFullyCompleted(context.Background(), taskID); err != nil {
				logger.WithError(err).WithField("task_id", taskID).Error("Failed to mark task fully completed")
			}
		}
	})
	logger.WithFields(logrus.Fields{
		"device_count":     deviceMgr.GetDeviceCount(),
		"mitm_proxy_host":  mitmProxyHost,
		"results_dir":      resultsDir,
	}).Info("Orchestrator initialized with device pool")

	// 9. 初始化 Worker Pool
	workerPool := worker.NewPool(cfg.Worker.Concurrency, orchestrator, logger)
	workerPool.Start(context.Background())
	defer workerPool.Stop()
	logger.Infof("Worker pool started with %d workers", cfg.Worker.Concurrency)

	// 10. 启动设备健康检查（每5分钟检查一次）
	go deviceMgr.StartHealthCheck(context.Background(), 5*time.Minute)
	logger.Info("Device health check started (interval: 5 minutes)")

	// 10.1 启动 ADB 连接健康检查（每30秒检查一次，自动重连）
	adbTargets := []string{
		device1.ADBTarget, // 192.168.2.34:46791 (Redmi Note 11 Pro WiFi)
	}
	connMgr := adb.GetConnectionManager(logger)
	go connMgr.StartHealthCheck(context.Background(), 30*time.Second, adbTargets)
	logger.Info("ADB connection health check started (interval: 30 seconds, auto-reconnect enabled)")

	// 11. 启动内存监控
	memMonitor := middleware.NewMemoryMonitor(logger, 30*time.Second)
	memMonitor.Start()
	defer memMonitor.Stop()
	logger.Info("Memory monitor started")

	// 12. 初始化 Prometheus 指标
	promMetrics := middleware.NewPrometheusMetrics(logger, "apk_analysis")
	logger.Info("Prometheus metrics initialized")

	// 启动 Prometheus 指标更新协程
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// 更新内存统计
			stats := memMonitor.GetStats()
			promMetrics.UpdateMemoryStats(stats)

			// 更新数据库连接统计
			sqlDB, _ := db.DB()
			dbStats := sqlDB.Stats()
			promMetrics.UpdateDBStats(dbStats.OpenConnections, dbStats.Idle, dbStats.InUse)

			// TODO: 更新 Worker Pool 统计
			// promMetrics.UpdateWorkerPoolStats(size, active, queueSize)
		}
	}()

	// 12. 初始化消息队列 Producer
	producer := queue.NewProducer(mq, logger)

	// 12.1 重新发布排队中的任务（服务重启后以数据库为准重建队列）
	if err := republishQueuedTasks(db, mq, producer, cfg.APKDir, logger); err != nil {
		logger.WithError(err).Warn("Failed to republish queued tasks")
	}

	// 13. 启动任务消费者 (从 RabbitMQ 读取任务并提交到 Worker Pool)
	consumer := queue.NewConsumer(mq, createTaskHandler(workerPool, producer, logger), cfg.Worker.Concurrency, logger)
	if err := consumer.Start(context.Background()); err != nil {
		logger.Fatalf("Failed to start consumer: %v", err)
	}
	defer consumer.Stop()
	logger.Infof("Task consumer started with %d workers", cfg.Worker.Concurrency)

	// 12. 启动文件监控
	fileWatcher, err := watcher.NewFileWatcher(cfg.APKDir, "*.apk", createFileHandler(taskService, producer, logger), logger)
	if err != nil {
		logger.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fileWatcher.Stop()

	if err := fileWatcher.Start(context.Background()); err != nil {
		logger.Fatalf("Failed to start file watcher: %v", err)
	}
	logger.Infof("File watcher started for directory: %s", cfg.APKDir)

	// TODO: 13. 初始化 Redis

	// 13.5 初始化恶意检测器
	var malwareDetector *malware.Detector
	if cfg.Malware.Enabled {
		malwareDetectorCfg := &malware.DetectorConfig{
			ServerURL:               cfg.Malware.ServerURL,
			Timeout:                 time.Duration(cfg.Malware.Timeout) * time.Second,
			DefaultModels:           cfg.Malware.Models,
			ExtractGraphFeatures:    cfg.Malware.ExtractGraphFeatures,
			ExtractTemporalFeatures: cfg.Malware.ExtractTemporalFeatures,
			UseEnsemble:             cfg.Malware.UseEnsemble,
			MaxRetries:              cfg.Malware.MaxRetries,
			RetryDelay:              time.Duration(cfg.Malware.RetryDelay) * time.Second,
		}
		malwareDetector = malware.NewDetector(malwareDetectorCfg, logger)
		logger.Infof("Malware detector initialized with server: %s, models: %v", cfg.Malware.ServerURL, cfg.Malware.Models)
	} else {
		logger.Info("Malware detection disabled")
	}

	// 14. 设置 HTTP Server
	router := api.SetupRouter(cfg, logger, db, memMonitor, promMetrics, deviceMgr, aiInteractionHandler, malwareDetector)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Minute, // 10分钟，支持大文件上传
		WriteTimeout: 5 * time.Minute,  // 5分钟，支持大文件下载
		IdleTimeout:  120 * time.Second,
	}

	// 15. 启动 HTTP Server
	go func() {
		logger.Infof("HTTP server listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 16. 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down gracefully...")

	// 17. 优雅关闭 (30秒超时)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 停止 HTTP Server
	if err := server.Shutdown(ctx); err != nil {
		logger.Errorf("HTTP server shutdown error: %v", err)
	}

	// 关闭数据库连接
	sqlDB, _ := db.DB()
	sqlDB.Close()

	logger.Info("Server stopped")
}

// createTaskHandler 创建任务处理器 (从 RabbitMQ 消息提交到 Worker Pool)
// producer 用于在任务需要重试时重新发布消息
func createTaskHandler(workerPool *worker.Pool, producer *queue.Producer, logger *logrus.Logger) queue.TaskHandler {
	return func(ctx context.Context, msg *queue.TaskMessage) error {
		logger.WithFields(logrus.Fields{
			"task_id":  msg.TaskID,
			"apk_name": msg.APKName,
			"apk_path": msg.APKPath,
		}).Info("Received task from RabbitMQ, submitting to worker pool")

		// 提交任务到 Worker Pool（同步等待任务完成）
		task := &worker.Task{
			ID:      msg.TaskID,
			APKPath: msg.APKPath,
		}

		if err := workerPool.SubmitAndWait(ctx, task); err != nil {
			// 检查是否为可重试错误
			if retryErr, ok := worker.IsRetryableError(err); ok {
				logger.WithFields(logrus.Fields{
					"task_id":     retryErr.TaskID,
					"retry_count": retryErr.RetryCount,
					"max_retry":   retryErr.MaxRetry,
				}).Warn("🔄 Task failed, republishing to RabbitMQ for retry...")

				// 重新发布到 RabbitMQ
				retryMsg := &queue.TaskMessage{
					TaskID:  retryErr.TaskID,
					APKName: msg.APKName,
					APKPath: retryErr.APKPath,
				}
				if pubErr := producer.PublishTask(ctx, retryMsg); pubErr != nil {
					logger.WithError(pubErr).WithField("task_id", retryErr.TaskID).Error("Failed to republish task for retry")
					return pubErr
				}
				logger.WithField("task_id", retryErr.TaskID).Info("✅ Task republished to RabbitMQ for retry")
				return nil // 重试已安排，不返回错误
			}

			logger.WithError(err).Error("Task execution failed")
			return err
		}

		logger.WithField("task_id", msg.TaskID).Info("Task completed successfully")

		// 域名分析现在通过任务完成回调触发,不需要在这里执行

		return nil
	}
}

// createFileHandler 创建文件处理器
func createFileHandler(taskService service.TaskService, producer *queue.Producer, logger *logrus.Logger) watcher.FileHandler {
	return func(ctx context.Context, filePath string) error {
		fileName := filepath.Base(filePath)
		logger.WithFields(logrus.Fields{
			"file_path": filePath,
			"file_name": fileName,
		}).Info("New APK file detected")

		// 1. 创建任务
		task, err := taskService.CreateTask(ctx, fileName)
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		// 2. 发布到消息队列
		msg := &queue.TaskMessage{
			TaskID:  task.ID,
			APKName: fileName,
			APKPath: filePath,
		}

		if err := producer.PublishTask(ctx, msg); err != nil {
			return fmt.Errorf("failed to publish task: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"task_id":  task.ID,
			"apk_name": fileName,
		}).Info("Task created and published to queue")

		return nil
	}
}

// cleanupStuckTasks 清理因服务重启而中断的任务
// 将所有 running/installing/collecting 状态的任务标记为 failed
// 注意：queued 状态的任务不需要清理，它们还在队列中等待执行
func cleanupStuckTasks(db *gorm.DB, logger *logrus.Logger) error {
	logger.Info("Checking for stuck tasks from previous service run...")

	// 只清理正在执行的任务（running/installing/collecting）
	// queued 状态的任务仍在队列中，不需要清理
	stuckStatuses := []string{"running", "installing", "collecting"}

	// 查找所有处于执行状态的任务
	var stuckTasks []struct {
		ID     string
		Status string
	}

	err := db.Table("apk_tasks").
		Select("id", "status").
		Where("status IN ?", stuckStatuses).
		Find(&stuckTasks).Error

	if err != nil {
		return fmt.Errorf("failed to query stuck tasks: %w", err)
	}

	if len(stuckTasks) == 0 {
		logger.Info("No stuck tasks found (queued tasks will continue)")
		return nil
	}

	logger.Infof("Found %d stuck tasks (running/installing/collecting), marking as failed...", len(stuckTasks))

	// 批量更新任务状态
	now := time.Now().UTC()
	result := db.Table("apk_tasks").
		Where("status IN ?", stuckStatuses).
		Updates(map[string]interface{}{
			"status":        "failed",
			"error_message": "服务重启，任务中断",
			"completed_at":  now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update stuck tasks: %w", result.Error)
	}

	logger.WithFields(logrus.Fields{
		"count": result.RowsAffected,
		"tasks": func() []string {
			ids := make([]string, len(stuckTasks))
			for i, t := range stuckTasks {
				ids[i] = t.ID
			}
			return ids
		}(),
	}).Warn("Marked stuck tasks as failed due to service restart (queued tasks preserved)")

	return nil
}

// republishQueuedTasks 重新发布排队中的任务到 RabbitMQ
// 服务重启后，以数据库为唯一真实数据源，重建 RabbitMQ 队列
// 步骤：1. 清空队列中的残留消息  2. 从数据库查询 queued 任务  3. 重新投递
func republishQueuedTasks(db *gorm.DB, mq *queue.RabbitMQ, producer *queue.Producer, apkDir string, logger *logrus.Logger) error {
	logger.Info("Rebuilding RabbitMQ queue from database (single source of truth)...")

	// 1. 先清空队列，确保没有残留的重复/过期消息
	purgedCount, err := mq.PurgeQueue()
	if err != nil {
		logger.WithError(err).Warn("Failed to purge queue, continuing with republish...")
	} else if purgedCount > 0 {
		logger.WithField("purged_count", purgedCount).Info("Cleared stale messages from queue")
	}

	// 2. 查找所有 queued 状态的任务
	var queuedTasks []struct {
		ID      string
		APKName string
	}

	err = db.Table("apk_tasks").
		Select("id", "apk_name").
		Where("status = ?", "queued").
		Order("created_at ASC"). // 按创建时间排序，先进先出
		Find(&queuedTasks).Error

	if err != nil {
		return fmt.Errorf("failed to query queued tasks: %w", err)
	}

	if len(queuedTasks) == 0 {
		logger.Info("No queued tasks found, queue is empty and clean")
		return nil
	}

	logger.Infof("Found %d queued tasks in database, republishing to RabbitMQ...", len(queuedTasks))

	// 重新发布每个任务
	successCount := 0
	for _, task := range queuedTasks {
		// 从配置的 APK 目录和文件名构建完整路径
		apkPath := filepath.Join(apkDir, task.APKName)

		msg := &queue.TaskMessage{
			TaskID:  task.ID,
			APKName: task.APKName,
			APKPath: apkPath,
		}

		if err := producer.PublishTask(context.Background(), msg); err != nil {
			logger.WithError(err).WithField("task_id", task.ID).Error("Failed to republish task")
			continue
		}

		successCount++
		logger.WithFields(logrus.Fields{
			"task_id":  task.ID,
			"apk_name": task.APKName,
		}).Debug("Task republished to queue")
	}

	logger.WithFields(logrus.Fields{
		"total":   len(queuedTasks),
		"success": successCount,
		"failed":  len(queuedTasks) - successCount,
	}).Info("Queued tasks republished to RabbitMQ")

	return nil
}
