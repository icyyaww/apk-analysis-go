package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/adb"
	"github.com/apk-analysis/apk-analysis-go/internal/ai"
	"github.com/apk-analysis/apk-analysis-go/internal/cert"
	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/device"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/filter"
	"github.com/apk-analysis/apk-analysis-go/internal/flow"
	"github.com/apk-analysis/apk-analysis-go/internal/frida"
	"github.com/apk-analysis/apk-analysis-go/internal/malware"
	"github.com/apk-analysis/apk-analysis-go/internal/packer"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/staticanalysis"
	"github.com/apk-analysis/apk-analysis-go/internal/unpacker"
	"github.com/sirupsen/logrus"
)

// Orchestrator 核心编排器
type Orchestrator struct {
	deviceMgr          *device.DeviceManager // 设备管理器（替代单一adbClient）
	aiAnalyzer         *ai.Analyzer
	taskRepo           repository.TaskRepository
	staticReportRepo   repository.StaticReportRepository
	malwareRepo        repository.MalwareRepository // 恶意检测结果仓库
	hybridAnalyzer     *staticanalysis.HybridAnalyzer
	malwareDetector    *malware.Detector             // 恶意检测器
	packerDetector     *packer.Detector              // 壳检测器
	dynamicUnpacker    *unpacker.DynamicUnpacker     // 动态脱壳器
	logger             *logrus.Logger
	resultsDir         string
	mitmProxyHost      string // mitmproxy容器主机名
	mitmProxyAPIPort   string // mitmproxy API端口
	aiEnabled          bool
	hybridEnabled      bool
	fridaEnabled       bool
	unpackingEnabled   bool // 是否启用动态脱壳
	malwareEnabled     bool // 是否启用恶意检测
	// AI智能交互相关字段
	aiInteractionEnabled bool
	interactionEngine    *ai.InteractionEngine
	smartClicker         *ai.SmartClicker
	// AI交互广播器（用于实时推送到前端）
	aiInteractionBroadcaster AIInteractionBroadcaster
	// 域名分析回调
	domainAnalysisCallback func(taskID string)
}

// AIInteractionBroadcaster AI交互广播接口
type AIInteractionBroadcaster interface {
	BroadcastAction(taskID, activity string, action AIActionData)
	BroadcastScreenshot(taskID string, screenshotURL string)
	BroadcastStatus(taskID string, status string)
}

// AIActionData AI动作数据（用于广播）
type AIActionData struct {
	Type     string `json:"type"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

// NewOrchestrator 创建编排器
// deviceMgr: 设备管理器（必须已初始化并添加设备）
// mitmProxyHost: mitmproxy容器主机名（如 "apk-analysis-mitmproxy"）
// malwareRepo: 恶意检测结果仓库（可选，传 nil 则禁用恶意检测存储）
func NewOrchestrator(
	deviceMgr *device.DeviceManager,
	taskRepo repository.TaskRepository,
	staticReportRepo repository.StaticReportRepository,
	malwareRepo repository.MalwareRepository,
	cfg *config.Config,
	logger *logrus.Logger,
	resultsDir string,
	mitmProxyHost string,
) *Orchestrator {
	// AI 分析器从配置文件初始化，如果配置为空则尝试环境变量
	glmAPIKey := cfg.AI.APIKey
	if glmAPIKey == "" {
		glmAPIKey = os.Getenv("GLM_API_KEY")
	}
	aiAnalyzer := ai.NewAnalyzer(glmAPIKey, logger)

	// 检查静态分析配置 (只使用 Hybrid 分析器)
	hybridEnabled := cfg.StaticAnalysis.Hybrid.Enabled

	// 初始化混合分析器
	var hybridAnalyzer *staticanalysis.HybridAnalyzer
	if hybridEnabled {
		hybridCfg := cfg.StaticAnalysis.Hybrid
		hybridConfig := &staticanalysis.HybridConfig{
			PythonPath:        hybridCfg.PythonPath,
			ScriptPath:        hybridCfg.ScriptPath,
			UseProcessPool:    hybridCfg.UseProcessPool,
			ProcessPoolSize:   hybridCfg.ProcessPoolSize,
			ForceDeepAnalysis: hybridCfg.ForceDeepAnalysis,
		}
		var err error
		hybridAnalyzer, err = staticanalysis.NewHybridAnalyzer(hybridConfig, logger)
		if err != nil {
			logger.WithError(err).Warn("Failed to create hybrid analyzer, hybrid analysis will be disabled")
			hybridEnabled = false
		} else {
			logger.Info("✅ Hybrid static analyzer enabled")
		}
	}

	// 初始化壳检测器
	packerDetector := packer.NewDetector(logger)
	logger.Info("✅ Packer detector initialized")

	// 初始化动态脱壳器
	dynamicUnpacker := unpacker.NewDynamicUnpacker(logger, "./scripts/unpacker")
	unpackingEnabled := true // 默认启用动态脱壳
	logger.Info("✅ Dynamic unpacker initialized")

	// 初始化恶意检测器
	var malwareDetector *malware.Detector
	malwareEnabled := cfg.Malware.Enabled
	if malwareEnabled {
		malwareCfg := &malware.DetectorConfig{
			ServerURL:               cfg.Malware.ServerURL,
			Timeout:                 time.Duration(cfg.Malware.Timeout) * time.Second,
			DefaultModels:           cfg.Malware.Models,
			ExtractGraphFeatures:    cfg.Malware.ExtractGraphFeatures,
			ExtractTemporalFeatures: cfg.Malware.ExtractTemporalFeatures,
			UseEnsemble:             cfg.Malware.UseEnsemble,
			MaxRetries:              cfg.Malware.MaxRetries,
			RetryDelay:              time.Duration(cfg.Malware.RetryDelay) * time.Second,
		}
		// 使用默认值填充空配置
		if malwareCfg.ServerURL == "" {
			malwareCfg.ServerURL = "http://localhost:5000"
		}
		if malwareCfg.Timeout == 0 {
			malwareCfg.Timeout = 120 * time.Second
		}
		if len(malwareCfg.DefaultModels) == 0 {
			malwareCfg.DefaultModels = []string{"drebin", "mh100k"}
		}
		if malwareCfg.MaxRetries == 0 {
			malwareCfg.MaxRetries = 3
		}
		if malwareCfg.RetryDelay == 0 {
			malwareCfg.RetryDelay = 1 * time.Second
		}

		malwareDetector = malware.NewDetector(malwareCfg, logger)
		logger.WithFields(logrus.Fields{
			"server_url": malwareCfg.ServerURL,
			"models":     malwareCfg.DefaultModels,
		}).Info("✅ Malware detector initialized")
	} else {
		logger.Info("ℹ️ Malware detection disabled in config (malware.enabled=false)")
	}

	// AI智能交互初始化 - 从配置文件读取
	aiInteractionEnabled := cfg.AI.Enabled
	var interactionEngine *ai.InteractionEngine

	// SmartClicker 始终初始化（不依赖AI，使用UI Automator解析XML）
	// 用于深度探索模式下的智能点击（隐私协议、权限弹窗等）
	smartClicker := ai.NewSmartClicker(logger)
	logger.Info("✅ SmartClicker initialized (UI Automator based)")

	if aiInteractionEnabled {
		if glmAPIKey != "" {
			interactionEngine = ai.NewInteractionEngine(glmAPIKey, logger)
			logger.WithField("model", cfg.AI.Model).Info("✅ AI smart interaction enabled (GLM-4V)")
		} else {
			logger.Warn("⚠️ ai.enabled=true but api_key not set, disabling AI interaction")
			aiInteractionEnabled = false
		}
	} else {
		logger.Info("ℹ️ AI smart interaction disabled in config (ai.enabled=false)")
	}

	logger.WithFields(logrus.Fields{
		"devices":         deviceMgr.GetDeviceCount(),
		"hybrid_enabled":  hybridEnabled,
		"malware_enabled": malwareEnabled,
		"ai_enabled":      aiAnalyzer.IsEnabled(),
		"ai_interaction":  aiInteractionEnabled,
		"frida_enabled":   true,
	}).Info("Orchestrator initialized with device pool")

	return &Orchestrator{
		deviceMgr:            deviceMgr,
		aiAnalyzer:           aiAnalyzer,
		taskRepo:             taskRepo,
		staticReportRepo:     staticReportRepo,
		malwareRepo:          malwareRepo,
		hybridAnalyzer:       hybridAnalyzer,
		malwareDetector:      malwareDetector,
		packerDetector:       packerDetector,
		dynamicUnpacker:      dynamicUnpacker,
		logger:               logger,
		resultsDir:           resultsDir,
		mitmProxyHost:        mitmProxyHost,
		mitmProxyAPIPort:     "8083",
		aiEnabled:            aiAnalyzer.IsEnabled(),
		hybridEnabled:        hybridEnabled,
		fridaEnabled:         true,
		unpackingEnabled:     unpackingEnabled,
		malwareEnabled:       malwareEnabled,
		aiInteractionEnabled: aiInteractionEnabled,
		interactionEngine:    interactionEngine,
		smartClicker:         smartClicker,
	}
}

// SetDomainAnalysisCallback 设置域名分析回调（用于 Hybrid-only 模式）
func (o *Orchestrator) SetDomainAnalysisCallback(callback func(taskID string)) {
	o.domainAnalysisCallback = callback
}

// SetAIInteractionBroadcaster 设置AI交互广播器（用于实时推送到前端）
func (o *Orchestrator) SetAIInteractionBroadcaster(broadcaster AIInteractionBroadcaster) {
	o.aiInteractionBroadcaster = broadcaster
	if broadcaster != nil {
		o.logger.Info("✅ AI interaction broadcaster configured")
	}
}

// ExecuteTask 执行完整任务
func (o *Orchestrator) ExecuteTask(ctx context.Context, taskID, apkPath string) error {
	o.logger.WithField("task_id", taskID).Info("Starting task execution")

	// 1. 检测 APK 架构，智能选择设备
	apkArch := device.DetectAPKArch(apkPath)
	o.logger.WithFields(logrus.Fields{
		"task_id":  taskID,
		"apk_path": apkPath,
		"apk_arch": apkArch,
	}).Info("APK architecture detected")

	// 2. 根据架构获取合适的设备（阻塞等待直到设备可用）
	// ARM-only APK 只能在真机上运行，x86/通用 APK 可以在模拟器上运行
	dev, err := o.deviceMgr.AcquireDeviceForAPK(ctx, taskID, apkArch)
	if err != nil {
		return o.failTaskWithAPKPath(ctx, taskID, apkPath, fmt.Errorf("failed to acquire device for %s APK: %w", apkArch, err))
	}
	defer o.deviceMgr.ReleaseDevice(dev) // 确保设备释放

	// 2. 为该设备创建专属客户端
	adbClient := dev.CreateADBClient(o.logger)
	proxyHost, proxyPort := dev.GetProxyAddress()
	certInstaller := cert.NewInstaller(dev.ADBTarget, o.logger)
	fridaClient := frida.NewClientWithHost(dev.ADBTarget, dev.FridaHost, o.logger)

	o.logger.WithFields(logrus.Fields{
		"task_id":           taskID,
		"device_id":         dev.ID,
		"adb_target":        dev.ADBTarget,
		"proxy":             fmt.Sprintf("%s:%d", proxyHost, proxyPort),
		"mitmproxy_api_port": dev.MitmproxyAPIPort,
	}).Info("Device acquired, clients created")

	// 3. 设置 mitmproxy 输出到该任务专属文件（使用设备特定的 API 端口）
	if err := o.setMitmproxyOutputForDevice(ctx, taskID, dev.MitmproxyContainer, dev.MitmproxyAPIPort); err != nil {
		o.logger.WithError(err).Warn("Failed to set mitmproxy output, flow isolation may not work")
	}
	defer o.clearMitmproxyOutputForDevice(ctx, dev.MitmproxyContainer, dev.MitmproxyAPIPort) // 确保清除输出设置

	// 用于存储包名,确保无论如何都能卸载
	var packageName string

	// 确保无论任务成功还是失败,都会卸载 APK
	defer func() {
		if packageName != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			o.logger.WithField("package", packageName).Info("Executing deferred cleanup (uninstall APK)")
			if err := adbClient.Uninstall(cleanupCtx, packageName); err != nil {
				o.logger.WithError(err).WithField("package", packageName).Error("Failed to uninstall APK in deferred cleanup")
			} else {
				o.logger.WithField("package", packageName).Info("APK uninstalled successfully in deferred cleanup")
			}
		}
	}()

	// 更新任务状态
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusInstalling, "正在连接设备", 10); err != nil {
		return err
	}

	// 1. 连接设备
	if err := adbClient.Connect(ctx); err != nil {
		return o.failTaskWithAPKPath(ctx, taskID, apkPath, fmt.Errorf("failed to connect device: %w", err))
	}

	// 1.5. 自动安装 mitmproxy 证书 (在 APK 安装前)
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusInstalling, "自动安装 mitmproxy 证书", 15); err != nil {
		return err
	}

	// 检查证书是否已安装
	certHash := "c8750f0d" // mitmproxy 默认证书哈希
	if !certInstaller.IsInstalled(ctx, certHash) {
		o.logger.Info("Certificate not installed, installing now...")
		if err := certInstaller.InstallManual(ctx, certHash); err != nil {
			o.logger.WithError(err).Warn("Failed to install certificate, HTTPS traffic may not be captured")
		} else {
			o.logger.Info("Certificate installed successfully")
		}
	} else {
		o.logger.Info("Certificate already installed, skipping")
	}

	// 2. 安装 APK
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusInstalling, "正在安装 APK", 20); err != nil {
		return err
	}

	packageName, err = o.installAndDetectPackage(ctx, taskID, apkPath, adbClient)
	if err != nil {
		return o.failTaskWithAPKPath(ctx, taskID, apkPath, err)
	}

	// 2.1. APK 安装成功后，执行静态分析（异步并行执行）
	// 根据配置启用 MobSF、Hybrid 或两者并行
	if err := o.runStaticAnalysis(ctx, taskID, apkPath, packageName); err != nil {
		o.logger.WithError(err).Warn("Failed to run static analysis, continuing anyway")
	}

	// 2.5. Frida 注入 & SSL Unpinning (高优先级功能)
	if o.fridaEnabled {
		if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusInstalling, "部署 Frida 并注入 SSL Unpinning", 25); err != nil {
			return err
		}

		// 部署 frida-server
		if err := fridaClient.SetupServer(ctx); err != nil {
			o.logger.WithError(err).Warn("Failed to setup Frida server, continuing without SSL unpinning")
		} else {
			// 启动 frida-server
			if err := fridaClient.StartServer(ctx); err != nil {
				o.logger.WithError(err).Warn("Failed to start Frida server")
			} else {
				// 注入 SSL Unpinning 脚本
				if err := fridaClient.InjectSSLUnpinning(ctx, packageName); err != nil {
					o.logger.WithError(err).Warn("Failed to inject SSL unpinning script")
				} else {
					o.logger.WithField("package", packageName).Info("Frida SSL unpinning injected successfully")
				}
			}
		}
	}

	// 2.6. 壳检测与动态脱壳
	if o.unpackingEnabled && o.packerDetector != nil {
		if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusInstalling, "检测应用加壳状态", 27); err != nil {
			return err
		}

		// 执行壳检测
		packerInfo := o.packerDetector.Detect(ctx, apkPath)

		if packerInfo.IsPacked {
			o.logger.WithFields(logrus.Fields{
				"packer_name": packerInfo.PackerName,
				"packer_type": packerInfo.PackerType,
				"confidence":  packerInfo.Confidence,
				"can_unpack":  packerInfo.CanUnpack,
			}).Warn("⚠️ Packer detected! Attempting dynamic unpacking")

			if packerInfo.CanUnpack && o.dynamicUnpacker != nil {
				if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusInstalling, "执行动态脱壳", 28); err != nil {
					return err
				}

				// 创建脱壳输出目录
				unpackDir := filepath.Join(o.resultsDir, taskID, "unpacked")

				// 执行动态脱壳
				unpackResult, err := o.dynamicUnpacker.Unpack(ctx, unpacker.UnpackRequest{
					TaskID:      taskID,
					PackageName: packageName,
					ADBTarget:   dev.ADBTarget,
					FridaHost:   dev.FridaHost,
					PackerInfo:  packerInfo,
					OutputDir:   unpackDir,
				})

				if err != nil {
					o.logger.WithError(err).Warn("Dynamic unpacking failed, continuing with packed APK")
				} else if unpackResult.Success {
					o.logger.WithFields(logrus.Fields{
						"dex_count":   unpackResult.DEXCount,
						"merged_dex":  unpackResult.MergedDEXPath,
						"duration_ms": unpackResult.Duration,
					}).Info("✅ Dynamic unpacking succeeded")

					// 脱壳成功后，重新执行深度静态分析
					if o.hybridEnabled && unpackResult.MergedDEXPath != "" {
						o.logger.Info("Re-analyzing unpacked DEX...")
						// TODO: 实现脱壳后重新分析的逻辑
						// o.reanalyzeUnpackedDEX(ctx, taskID, unpackResult.MergedDEXPath)
					}
				}
			} else {
				o.logger.WithField("packer", packerInfo.PackerName).Warn("Packer detected but automatic unpacking not supported")
			}
		} else {
			o.logger.Info("No packer detected, skipping dynamic unpacking")
		}
	}

	// 3. 跳过代理设置（假设设备已在 WiFi 设置中配置好代理）
	// WiFi 代理比 settings put global http_proxy 更可靠，能捕获所有 APP 流量
	o.logger.Info("Skipping proxy setup - assuming device WiFi proxy is pre-configured")

	// 3.5. 启动应用
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, "启动应用", 30); err != nil {
		return err
	}

	if err := o.launchApp(ctx, packageName, adbClient); err != nil {
		o.logger.WithError(err).Warn("启动应用失败")
	} else {
		o.logger.WithField("package", packageName).Info("应用启动成功")
	}

	// 等待应用启动完成
	time.Sleep(3 * time.Second)

	// 3.6. AI 单步交互循环
	// 由 AI 自动处理协议弹窗、权限请求、登录页面等
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, "AI 智能交互中", 35); err != nil {
		return err
	}

	aiLoopResult := o.runAISingleStepLoop(ctx, taskID, packageName, adbClient)
	o.logger.WithFields(logrus.Fields{
		"total_steps":   aiLoopResult.TotalSteps,
		"success_steps": aiLoopResult.SuccessSteps,
		"exit_reason":   aiLoopResult.ExitReason,
	}).Info("AI 单步交互循环结果")

	// 4. 提取 Activity 列表
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, "提取 Activity 列表", 40); err != nil {
		return err
	}

	activities, err := o.extractActivities(ctx, packageName, adbClient)
	if err != nil {
		return o.failTaskWithAPKPath(ctx, taskID, apkPath, err)
	}

	// 5. 过滤 Activity
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, "智能过滤 Activity", 45); err != nil {
		return err
	}

	activityFilter := filter.NewActivityFilter(packageName, o.logger)
	filterResult := activityFilter.Filter(activities)

	// 保存过滤报告
	taskDir := filepath.Join(o.resultsDir, taskID)
	if err := o.saveFilterReport(taskDir, filterResult); err != nil {
		o.logger.WithError(err).Warn("Failed to save filter report")
	}

	// 6. 遍历 Activity
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, "开始遍历 Activity", 50); err != nil {
		return err
	}

	activityDetails, err := o.traverseActivities(ctx, taskID, packageName, filterResult.SelectedList, adbClient)
	if err != nil {
		o.logger.WithError(err).Warn("Activity traversal had errors, but continuing")
	}

	// 6.5. 后台监控 (保持应用运行，捕获延迟/周期性网络请求)
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, "后台监控: 捕获延迟请求", 85); err != nil {
		return err
	}

	if err := o.runBackgroundMonitoring(ctx, packageName, 30*time.Second, adbClient); err != nil {
		o.logger.WithError(err).Warn("Background monitoring failed, continuing anyway")
	}

	// 7. 收集数据
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusCollecting, "收集分析数据", 90); err != nil {
		return err
	}

	if err := o.collectData(ctx, taskID, packageName, adbClient); err != nil {
		o.logger.WithError(err).Warn("Failed to collect some data")
	}

	// 8. 清理环境
	if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusCollecting, "清理测试环境", 95); err != nil {
		return err
	}

	o.cleanupTask(ctx, taskID, packageName, adbClient, fridaClient)

	// 9. 保存 Activity 详情到数据库
	if err := o.saveActivityDetails(ctx, taskID, packageName, activities, activityDetails); err != nil {
		o.logger.WithError(err).Warn("Failed to save activity details")
	}

	// 10. 完成任务
	return o.completeTask(ctx, taskID)
}

// installAndDetectPackage 安装 APK 并检测包名
// 注意：设备级锁已由 DeviceManager 处理，不再需要全局互斥锁
func (o *Orchestrator) installAndDetectPackage(ctx context.Context, taskID, apkPath string, adbClient *adb.Client) (string, error) {
	o.logger.WithFields(logrus.Fields{
		"task_id":  taskID,
		"apk_path": apkPath,
	}).Info("Starting APK installation")

	// 步骤1: 从 APK 文件提取预期包名
	o.logger.Info("Extracting package name from APK file...")
	expectedPackageName, err := o.extractPackageNameFromAPK(ctx, apkPath, adbClient)
	if err != nil {
		o.logger.WithError(err).Warn("Failed to extract package name from APK, will use detected name")
		expectedPackageName = "" // 继续执行，但没有验证
	} else {
		o.logger.WithField("expected_package", expectedPackageName).Info("Expected package name extracted from APK")

		// 步骤1.5: 预防性卸载已存在的包
		o.logger.WithField("package", expectedPackageName).Info("Attempting pre-installation uninstall...")
		if err := adbClient.Uninstall(ctx, expectedPackageName); err != nil {
			o.logger.WithField("package", expectedPackageName).Debug("Pre-installation uninstall failed (package may not be installed)")
		} else {
			o.logger.WithField("package", expectedPackageName).Info("Successfully uninstalled existing package before installation")
			// 等待一下让系统稳定
			time.Sleep(2 * time.Second)
		}
	}

	// 步骤2: 使用 ADB 安装并检测包名
	detectedPackageName, err := adbClient.FindPackageByAPK(ctx, apkPath)
	if err != nil {
		return "", fmt.Errorf("failed to install and detect package: %w", err)
	}

	// 步骤3: 验证包名是否匹配（如果提取成功）
	if expectedPackageName != "" && detectedPackageName != expectedPackageName {
		o.logger.WithFields(logrus.Fields{
			"expected": expectedPackageName,
			"detected": detectedPackageName,
		}).Error("❌ Package name mismatch detected!")

		// 包名不匹配是严重错误，应该终止任务
		return "", fmt.Errorf("package name mismatch: expected '%s', but detected '%s'. This indicates concurrent installation conflict",
			expectedPackageName, detectedPackageName)
	}

	// 使用验证后的包名
	finalPackageName := detectedPackageName
	if expectedPackageName != "" {
		o.logger.WithFields(logrus.Fields{
			"expected": expectedPackageName,
			"detected": detectedPackageName,
		}).Info("✅ Package name verification passed")
	}

	// 步骤4: 更新数据库
	task, err := o.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return finalPackageName, err
	}

	task.PackageName = finalPackageName
	if err := o.taskRepo.Update(ctx, task); err != nil {
		o.logger.WithError(err).Warn("Failed to update package name")
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"package_name": finalPackageName,
	}).Info("Package installed, detected, and verified successfully")

	return finalPackageName, nil
}

// extractPackageNameFromAPK 从 APK 文件提取包名 (使用 aapt)
func (o *Orchestrator) extractPackageNameFromAPK(ctx context.Context, apkPath string, adbClient *adb.Client) (string, error) {
	// 使用 aapt dump badging 提取包名
	output, err := adbClient.Shell(ctx, fmt.Sprintf("aapt dump badging %s 2>/dev/null | grep package:", apkPath))
	if err != nil {
		return "", fmt.Errorf("aapt command failed: %w", err)
	}

	// 解析输出: package: name='com.example.app' versionCode='1' versionName='1.0'
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "package:") {
			// 提取 name='...' 部分
			start := strings.Index(line, "name='")
			if start == -1 {
				continue
			}
			start += 6 // 跳过 "name='"
			end := strings.Index(line[start:], "'")
			if end == -1 {
				continue
			}
			return line[start : start+end], nil
		}
	}

	return "", fmt.Errorf("package name not found in aapt output")
}

// extractActivities 提取 Activity 列表（增强版：精确识别，过滤非Activity组件）
func (o *Orchestrator) extractActivities(ctx context.Context, packageName string, adbClient *adb.Client) ([]string, error) {
	// 使用 dumpsys package 提取
	output, err := adbClient.Shell(ctx, fmt.Sprintf("dumpsys package %s", packageName))
	if err != nil {
		return nil, fmt.Errorf("failed to dumpsys package: %w", err)
	}

	// 修改正则表达式，支持多种格式：
	// 1. packageName/.ActivityName (简写形式，以.开头)
	// 2. packageName/完整类名 (完整形式)
	// 3. packageName/单个类名 (无点号)
	componentPattern := regexp.MustCompile(regexp.QuoteMeta(packageName) + `/([A-Za-z0-9_.$]+)`)

	// 非 Activity 组件的后缀 (需要过滤)
	nonActivitySuffixes := []string{
		"Provider",      // ContentProvider
		"Receiver",      // BroadcastReceiver
		"Service",       // Service
		"Application",   // Application
		"Initializer",   // ContentProvider Initializer (如 BasePopupInitializer)
	}

	// 非 Activity 组件的关键词（更严格的过滤）
	nonActivityKeywords := []string{
		"ContentProvider",
		"BroadcastReceiver",
		"ServiceConnection",
		"ApplicationDelegate",
		"Initializer",     // 库初始化器
		"Configurator",    // 配置器
		"Installer",       // 安装器
	}

	activitySet := make(map[string]bool)
	lines := strings.Split(output, "\n")

	o.logger.WithField("package_name", packageName).Debug("Starting to extract activities from dumpsys output")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过无关行
		if line == "" || strings.HasPrefix(line, "#") ||
		   strings.HasPrefix(line, "Package [") ||
		   strings.HasPrefix(line, "User ") ||
		   strings.HasPrefix(line, "PackageSetting") {
			continue
		}

		// 查找所有匹配的组件
		matches := componentPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			clsPart := match[1]
			if clsPart == "" {
				continue
			}

			// 构建完整类名
			var fullName string
			if strings.HasPrefix(clsPart, ".") {
				// 格式1: packageName/.ActivityName -> packageName.ActivityName
				fullName = packageName + clsPart
			} else if strings.Contains(clsPart, ".") {
				// 格式2: packageName/com.example.Activity -> com.example.Activity
				fullName = clsPart
			} else {
				// 格式3: packageName/ActivityName -> packageName.ActivityName
				fullName = packageName + "." + clsPart
			}

			// 过滤非 Activity 组件
			simpleName := fullName[strings.LastIndex(fullName, ".")+1:]
			isNonActivity := false

			// 检查后缀
			for _, suffix := range nonActivitySuffixes {
				if strings.HasSuffix(simpleName, suffix) {
					o.logger.WithFields(logrus.Fields{
						"component": fullName,
						"reason":    fmt.Sprintf("suffix: %s", suffix),
					}).Debug("Filtered out non-Activity component")
					isNonActivity = true
					break
				}
			}

			// 检查关键词（如果没有被后缀过滤）
			if !isNonActivity {
				for _, keyword := range nonActivityKeywords {
					if strings.Contains(fullName, keyword) {
						o.logger.WithFields(logrus.Fields{
							"component": fullName,
							"reason":    fmt.Sprintf("keyword: %s", keyword),
						}).Debug("Filtered out non-Activity component")
						isNonActivity = true
						break
					}
				}
			}

			if !isNonActivity {
				if !activitySet[fullName] {
					o.logger.WithFields(logrus.Fields{
						"activity":  fullName,
						"cls_part":  clsPart,
						"line_sample": line[:min(len(line), 100)],
					}).Debug("Activity found")
					activitySet[fullName] = true
				}
			}
		}
	}

	// 转为数组
	activities := make([]string, 0, len(activitySet))
	for activity := range activitySet {
		activities = append(activities, activity)
	}

	o.logger.WithFields(logrus.Fields{
		"package_name":     packageName,
		"total_activities": len(activities),
	}).Info("Activities extracted from dumpsys")

	return activities, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// traverseActivities 遍历 Activity
func (o *Orchestrator) traverseActivities(ctx context.Context, taskID, packageName string, activities []string, adbClient *adb.Client) ([]map[string]interface{}, error) {
	taskDir := filepath.Join(o.resultsDir, taskID)
	screenshotDir := filepath.Join(taskDir, "screenshots")
	uiHierarchyDir := filepath.Join(taskDir, "ui_hierarchy")

	// 创建目录
	os.MkdirAll(screenshotDir, 0755)
	os.MkdirAll(uiHierarchyDir, 0755)

	// 任务专属的 flows.jsonl 文件 (mitmproxy 已切换输出到此文件)
	taskFlowsPath := filepath.Join(taskDir, "flows.jsonl")
	attributor := flow.NewAttributor(o.logger)

	activityDetails := make([]map[string]interface{}, 0, len(activities))

	totalActivities := len(activities)
	for i, activity := range activities {
		// 检查任务是否应该停止
		if i%20 == 0 {
			task, _ := o.taskRepo.FindByID(ctx, taskID)
			if task != nil && task.ShouldStop {
				o.logger.WithField("index", i).Info("Task stopped by user")
				break
			}
		}

		// 更新进度
		progress := 60 + (i * 30 / totalActivities)
		stepDesc := fmt.Sprintf("执行 Activity %d/%d: %s", i+1, totalActivities, o.shortActivityName(activity))
		if err := o.updateTaskStatus(ctx, taskID, domain.TaskStatusRunning, stepDesc, progress); err != nil {
			o.logger.WithError(err).Warn("Failed to update progress")
		}

		detail := o.executeActivity(ctx, taskID, packageName, activity, i, screenshotDir, uiHierarchyDir, taskFlowsPath, attributor, adbClient)
		activityDetails = append(activityDetails, detail)
	}

	return activityDetails, nil
}

// executeActivity 执行单个 Activity
func (o *Orchestrator) executeActivity(
	ctx context.Context,
	taskID, packageName, activity string,
	index int,
	screenshotDir, uiHierarchyDir, taskFlowsPath string,
	attributor *flow.Attributor,
	adbClient *adb.Client,
) map[string]interface{} {
	startTime := time.Now()
	component := fmt.Sprintf("%s/%s", packageName, activity)

	detail := map[string]interface{}{
		"activity":   activity,
		"component":  component,
		"start_time": startTime.Format(time.RFC3339),
		"status":     "failed",
	}

	// 1. 启动 Activity
	if err := adbClient.StartActivity(ctx, component); err != nil {
		detail["error"] = err.Error()
		return detail
	}

	// 等待Activity加载和网络请求
	time.Sleep(3 * time.Second)

	// 1.5 检测是否成功进入目标应用（前台检测）
	currentPkg, err := adbClient.GetForegroundPackage(ctx)
	if err != nil {
		o.logger.WithError(err).WithField("activity", o.shortActivityName(activity)).Warn("⚠️ 无法检测前台应用")
		// 检测失败不阻塞，继续执行
	} else if currentPkg != packageName {
		o.logger.WithFields(logrus.Fields{
			"activity":        o.shortActivityName(activity),
			"target_package":  packageName,
			"current_package": currentPkg,
		}).Warn("⚠️ Activity启动失败，当前不在目标应用内")

		detail["status"] = "launch_failed"
		detail["error"] = fmt.Sprintf("Activity启动失败，当前前台应用: %s", currentPkg)
		detail["current_foreground"] = currentPkg

		// 尝试恢复：重新拉起应用主界面
		o.logger.Info("🔄 尝试恢复：重新拉起应用主界面")
		_, _ = adbClient.Shell(ctx, fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", packageName))
		time.Sleep(2 * time.Second)

		// 返回，跳过该 Activity 的后续操作
		endTime := time.Now()
		detail["end_time"] = endTime.Format(time.RFC3339)
		detail["execution_time"] = endTime.Sub(startTime).Seconds()
		return detail
	}

	// 额外等待以捕获更多网络流量
	time.Sleep(2 * time.Second)

	// 2. 截图 (所有 Activity)
	var screenshotPath string
	screenshotFile := fmt.Sprintf("%03d_%s.png", index+1, o.shortActivityName(activity))
	screenshotPath = filepath.Join(screenshotDir, screenshotFile)
	if err := adbClient.Screenshot(ctx, screenshotPath); err != nil {
		o.logger.WithError(err).Warn("Screenshot failed")
	} else {
		detail["screenshot_file"] = screenshotFile

		// 广播截图更新到前端（如果广播器已配置）
		if o.aiInteractionBroadcaster != nil {
			screenshotURL := fmt.Sprintf("/api/tasks/%s/screenshot/%s", taskID, screenshotFile)
			o.aiInteractionBroadcaster.BroadcastScreenshot(taskID, screenshotURL)
		}

		// AI 分析 (如果启用)
		if o.aiEnabled && screenshotPath != "" {
			aiAnalysis, err := o.aiAnalyzer.AnalyzeActivityUI(ctx, activity, screenshotPath)
			if err != nil {
				o.logger.WithError(err).Warn("AI analysis failed")
			} else {
				// 保存 AI 分析结果
				detail["ai_analysis"] = map[string]interface{}{
					"buttons":          aiAnalysis.UIElements.Buttons,
					"input_fields":     aiAnalysis.UIElements.InputFields,
					"clickable_items":  aiAnalysis.UIElements.ClickableItems,
					"suggested_actions": o.aiAnalyzer.GetTopActions(aiAnalysis, 10),
				}
				o.logger.WithField("activity", activity).Info("AI analysis completed")
			}
		}
	}

	// 3. UI Hierarchy (所有 Activity)
	uiHierarchyFile := fmt.Sprintf("%03d_%s.xml", index+1, o.shortActivityName(activity))
	uiHierarchyPath := filepath.Join(uiHierarchyDir, uiHierarchyFile)
	if err := adbClient.DumpUIHierarchy(ctx, uiHierarchyPath); err != nil {
		o.logger.WithError(err).Warn("UI hierarchy dump failed")
	} else {
		detail["ui_hierarchy_file"] = uiHierarchyFile
	}

	// 4. 交互测试 - AI智能交互优先,失败则降级到传统深度探索
	if o.aiInteractionEnabled {
		o.logger.WithFields(logrus.Fields{
			"activity":     activity,
			"activity_short": o.shortActivityName(activity),
			"index":        index + 1,
		}).Info("🤖 Starting AI smart interaction on Activity")

		aiSuccess, aiActions := o.performAIInteraction(ctx, taskID, packageName, activity, uiHierarchyPath, adbClient)

		// 保存AI交互动作数据到detail
		if len(aiActions) > 0 {
			detail["actions"] = aiActions

			// 打印每个AI动作的详细信息
			for i, action := range aiActions {
				o.logger.WithFields(logrus.Fields{
					"activity":     o.shortActivityName(activity),
					"action_index": i + 1,
					"action_type":  action.Type,
					"action_target": action.Reason,
					"coordinates":  fmt.Sprintf("(%d,%d)", action.X, action.Y),
					"priority":     action.Priority,
				}).Info("✅ AI action executed")
			}

			o.logger.WithFields(logrus.Fields{
				"activity":     o.shortActivityName(activity),
				"action_count": len(aiActions),
			}).Info("📊 AI interaction summary")
		}

		if !aiSuccess {
			o.logger.WithField("activity", o.shortActivityName(activity)).Warn("⚠️ AI interaction failed, falling back to traditional deep exploration")
			o.performDeepExploration(ctx, activity, adbClient)
		} else {
			o.logger.WithField("activity", o.shortActivityName(activity)).Info("✅ AI interaction completed successfully")
		}
	} else {
		// 传统深度探索
		o.logger.WithField("activity", o.shortActivityName(activity)).Info("🔍 Performing deep exploration (AI disabled)")
		o.performDeepExploration(ctx, activity, adbClient)
	}

	// 返回到主界面
	adbClient.PressHome(ctx)

	endTime := time.Now()
	detail["end_time"] = endTime.Format(time.RFC3339)
	detail["execution_time"] = endTime.Sub(startTime).Seconds()

	// 5. 归因流量 - 直接从任务专属 flows.jsonl 读取（mitmproxy已切换输出到该文件）
	// 简化逻辑：只需要根据时间范围过滤，不需要包名过滤
	if _, err := os.Stat(taskFlowsPath); err == nil {
		attributedFlows, err := attributor.AttributeFlows(ctx, taskFlowsPath, startTime, endTime)
		if err != nil {
			o.logger.WithError(err).Warn("Flow attribution failed")
		} else {
			detail["urls_collected"] = len(attributedFlows)
			detail["flows"] = attributedFlows

			o.logger.WithFields(logrus.Fields{
				"activity":    activity,
				"start_time":  startTime.Format(time.RFC3339),
				"end_time":    endTime.Format(time.RFC3339),
				"flows_count": len(attributedFlows),
			}).Debug("Flow attribution completed for activity")
		}
	}

	detail["status"] = "completed"
	return detail
}

// Helper functions

func (o *Orchestrator) updateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus, step string, progress int) error {
	task, err := o.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}

	task.Status = status
	task.CurrentStep = step
	task.ProgressPercent = progress

	if status == domain.TaskStatusRunning && task.StartedAt == nil {
		now := time.Now()
		task.StartedAt = &now
	}

	return o.taskRepo.Update(ctx, task)
}

// RetryableError 可重试错误（用于通知 worker pool 需要重试）
type RetryableError struct {
	TaskID      string
	APKPath     string
	OriginalErr error
	RetryCount  int
	MaxRetry    int
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("task %s failed (retry %d/%d): %v", e.TaskID, e.RetryCount, e.MaxRetry, e.OriginalErr)
}

// IsRetryableError 检查错误是否为可重试错误
func IsRetryableError(err error) (*RetryableError, bool) {
	var retryErr *RetryableError
	if errors.As(err, &retryErr) {
		return retryErr, true
	}
	return nil, false
}

func (o *Orchestrator) failTask(ctx context.Context, taskID string, err error) error {
	return o.failTaskWithAPKPath(ctx, taskID, "", err)
}

func (o *Orchestrator) failTaskWithAPKPath(ctx context.Context, taskID, apkPath string, err error) error {
	// 尝试从错误中提取失败类型
	failureType := o.detectFailureType(err)

	// 获取当前重试次数
	retryCount, getErr := o.taskRepo.GetRetryCount(ctx, taskID)
	if getErr != nil {
		o.logger.WithError(getErr).WithField("task_id", taskID).Warn("Failed to get retry count, assuming 0")
		retryCount = 0
	}

	// 检查是否可以重试
	maxRetry := failureType.GetMaxRetryCount()
	canRetry := failureType.CanRetry() && retryCount < maxRetry

	if canRetry {
		// 增加重试次数
		newRetryCount, incErr := o.taskRepo.IncrementRetryCount(ctx, taskID)
		if incErr != nil {
			o.logger.WithError(incErr).WithField("task_id", taskID).Error("Failed to increment retry count")
		} else {
			retryCount = newRetryCount
		}

		// 重置任务状态以准备重试
		if resetErr := o.taskRepo.ResetForRetry(ctx, taskID); resetErr != nil {
			o.logger.WithError(resetErr).WithField("task_id", taskID).Error("Failed to reset task for retry")
			// 重置失败，不重试，直接标记为失败
			canRetry = false
		}
	}

	if canRetry {
		o.logger.WithFields(logrus.Fields{
			"task_id":      taskID,
			"failure_type": failureType,
			"retry_count":  retryCount,
			"max_retry":    maxRetry,
			"error":        err.Error(),
		}).Warn("🔄 Task will be retried")

		// 返回可重试错误，通知 worker pool 重新入队
		return &RetryableError{
			TaskID:      taskID,
			APKPath:     apkPath,
			OriginalErr: err,
			RetryCount:  retryCount,
			MaxRetry:    maxRetry,
		}
	}

	// 不可重试，标记为最终失败
	if updateErr := o.taskRepo.UpdateFailure(ctx, taskID, failureType, err.Error()); updateErr != nil {
		o.logger.WithError(updateErr).WithField("task_id", taskID).Error("Failed to update task failure")
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":          taskID,
		"failure_type":     failureType,
		"failure_severity": failureType.GetSeverity(),
		"retry_count":      retryCount,
		"max_retry":        maxRetry,
		"error":            err.Error(),
	}).Error("❌ Task failed (no more retries)")

	return err
}

// detectFailureType 根据错误信息检测失败类型
func (o *Orchestrator) detectFailureType(err error) domain.FailureType {
	if err == nil {
		return domain.FailureTypeNone
	}

	// 检查是否为设备获取错误（包含具体失败类型）
	var deviceErr *device.DeviceAcquireError
	if errors.As(err, &deviceErr) {
		return deviceErr.FailureType
	}

	// 根据错误信息关键字判断失败类型
	// 注意：检测顺序很重要！更具体的错误类型要放在前面
	errMsg := err.Error()

	// 1. 安装相关错误（优先级最高，因为包含 "adb" 但实际是安装问题）
	if containsAny(errMsg, "INSTALL_FAILED", "INSTALL_PARSE_FAILED", "pm install failed", "install failed", "failed to install") {
		return domain.FailureTypeInstallFailed
	}

	// 2. ARM 设备相关错误
	if containsAny(errMsg, "timeout waiting for arm device", "ARM device", "arm_device", "APK requires ARM") {
		return domain.FailureTypeARMDeviceOnly
	}

	// 3. 设备超时错误
	if containsAny(errMsg, "timeout waiting for", "device timeout", "no device available") {
		return domain.FailureTypeDeviceTimeout
	}

	// 4. Frida 相关错误
	if containsAny(errMsg, "frida", "inject", "spawn failed", "attach failed") {
		return domain.FailureTypeFridaError
	}

	// 5. 代理相关错误
	if containsAny(errMsg, "proxy", "mitmproxy", "certificate") {
		return domain.FailureTypeProxyError
	}

	// 6. 连接错误（放在后面，避免误判安装错误）
	if containsAny(errMsg, "device offline", "unauthorized", "connection refused", "no devices", "device not found") {
		return domain.FailureTypeConnectionError
	}

	// 7. 超时错误
	if containsAny(errMsg, "context deadline exceeded", "operation timed out") {
		return domain.FailureTypeTimeout
	}

	// 8. 分析相关错误
	if containsAny(errMsg, "analysis failed", "parse error", "extract failed") {
		return domain.FailureTypeAnalysisError
	}

	return domain.FailureTypeUnknown
}

// containsAny 检查字符串是否包含任意一个子串（不区分大小写）
func containsAny(s string, substrs ...string) bool {
	sLower := strings.ToLower(s)
	for _, substr := range substrs {
		if strings.Contains(sLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

// completeTask 完成动态分析阶段，设置进度为95%，等待域名分析完成后才真正完成任务
func (o *Orchestrator) completeTask(ctx context.Context, taskID string) error {
	task, err := o.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}

	// 🔧 修改：动态分析结束后进入 collecting 状态，进度95%
	// 真正的100%完成需要等待域名分析完成
	task.Status = domain.TaskStatusCollecting
	task.CurrentStep = "域名分析中..."
	task.ProgressPercent = 95

	if err := o.taskRepo.Update(ctx, task); err != nil {
		return err
	}

	// 🔧 使用原子更新标记动态分析完成（避免并发竞态）
	if err := o.taskRepo.MarkDynamicAnalysisCompleted(ctx, taskID); err != nil {
		o.logger.WithError(err).Warn("Failed to mark dynamic analysis as completed")
	}

	// 检查是否应该触发域名分析（需要静态+动态都完成）
	o.checkAndTriggerDomainAnalysis(ctx, taskID, nil) // 传 nil 让它从数据库重新加载最新状态

	return nil
}

// checkAndTriggerDomainAnalysis 检查静态+动态是否都完成，如果是则触发域名分析
// 注意：task 参数已废弃，始终从数据库读取最新状态以避免并发问题
func (o *Orchestrator) checkAndTriggerDomainAnalysis(ctx context.Context, taskID string, _ *domain.Task) {
	// 🔧 始终从数据库读取最新状态（避免使用过时的内存对象）
	staticCompleted, dynamicCompleted, err := o.taskRepo.GetAnalysisStatus(ctx, taskID)
	if err != nil {
		o.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get analysis status for domain analysis check")
		return
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":                    taskID,
		"static_analysis_completed":  staticCompleted,
		"dynamic_analysis_completed": dynamicCompleted,
	}).Info("Checking if domain analysis should be triggered")

	// 只有当静态和动态都完成时，才触发域名分析
	if staticCompleted && dynamicCompleted {
		o.logger.WithField("task_id", taskID).Info("✅ Both static and dynamic analysis completed, triggering domain analysis")

		// 触发域名分析回调
		if o.domainAnalysisCallback != nil {
			go o.domainAnalysisCallback(taskID)
		} else {
			o.logger.Warn("No domain analysis callback configured, skipping domain analysis")
		}
	} else {
		o.logger.WithFields(logrus.Fields{
			"task_id":          taskID,
			"static_completed":  staticCompleted,
			"dynamic_completed": dynamicCompleted,
		}).Info("⏳ Waiting for both analyses to complete before domain analysis")
	}
}

func (o *Orchestrator) cleanup(ctx context.Context, packageName string, adbClient *adb.Client, fridaClient *frida.Client) {
	// 停止 Frida server
	if o.fridaEnabled && fridaClient != nil {
		if err := fridaClient.StopServer(ctx); err != nil {
			o.logger.WithError(err).Warn("Failed to stop Frida server")
		}
	}

	// 跳过清除代理（WiFi 代理由用户手动管理）
	// o.logger.Info("Skipping proxy cleanup - WiFi proxy is managed manually")

	// 卸载应用
	if err := adbClient.Uninstall(ctx, packageName); err != nil {
		o.logger.WithError(err).Warn("Failed to uninstall app")
	}
}

// cleanupTask 清理任务
// 注意：mitmproxy 输出切换由 defer clearMitmproxyOutput() 处理
func (o *Orchestrator) cleanupTask(ctx context.Context, taskID, packageName string, adbClient *adb.Client, fridaClient *frida.Client) {
	// 执行常规清理
	o.cleanup(ctx, packageName, adbClient, fridaClient)
}

func (o *Orchestrator) parseProxy(proxy string) (string, int) {
	parts := strings.Split(proxy, ":")
	if len(parts) >= 2 {
		host := parts[0]
		port := 8082 // mitmproxy 默认端口

		// 将 localhost 转换为 Android 模拟器可以访问的地址
		// Genymotion 使用 10.0.3.1 访问宿主机
		// Docker Android 使用 10.0.2.2 访问宿主机
		if host == "localhost" || host == "127.0.0.1" {
			host = "10.0.3.1"  // 使用 Genymotion 网关
		}

		// 解析端口 (如果提供)
		if len(parts) == 2 {
			fmt.Sscanf(parts[1], "%d", &port)
		}

		return host, port
	}
	return "10.0.3.1", 8082  // Genymotion 默认网关
}

func (o *Orchestrator) shortActivityName(fullName string) string {
	parts := strings.Split(fullName, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fullName
}

func (o *Orchestrator) isCoreActivity(activity string) bool {
	corePatterns := []string{
		"MainActivity", "LoginActivity", "HomeActivity",
		"WelcomeActivity", "SplashActivity",
	}
	for _, pattern := range corePatterns {
		if strings.Contains(activity, pattern) {
			return true
		}
	}
	return false
}

func (o *Orchestrator) collectData(ctx context.Context, taskID, packageName string, adbClient *adb.Client) error {
	taskDir := filepath.Join(o.resultsDir, taskID)

	// 收集 logcat
	logcatPath := filepath.Join(taskDir, "logcat.txt")
	logcat, err := adbClient.GetLogcat(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logcat: %w", err)
	}

	return os.WriteFile(logcatPath, []byte(logcat), 0644)
}

func (o *Orchestrator) saveFilterReport(taskDir string, result *filter.FilterResult) error {
	activityFilter := filter.NewActivityFilter("", o.logger)
	report := activityFilter.GetFilterReport(result)

	reportPath := filepath.Join(taskDir, "activity_filter_report.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	os.MkdirAll(taskDir, 0755)
	return os.WriteFile(reportPath, data, 0644)
}

func (o *Orchestrator) saveActivityDetails(ctx context.Context, taskID, packageName string, allActivities []string, details []map[string]interface{}) error {
	// 保存到数据库
	activityData := &domain.TaskActivity{
		TaskID:              taskID,
		ActivitiesJSON:      strings.Join(allActivities, ","),
		ActivityDetailsJSON: o.jsonString(details),
		CreatedAt:           time.Now(),
	}

	// 查找主 Activity (第一个 MainActivity 或第一个 Activity)
	for _, activity := range allActivities {
		if strings.Contains(activity, "MainActivity") {
			activityData.LauncherActivity = activity
			break
		}
	}
	if activityData.LauncherActivity == "" && len(allActivities) > 0 {
		activityData.LauncherActivity = allActivities[0]
	}

	// 直接插入到 task_activities 表 (使用 GORM 的 Create 或 Save)
	// 如果已存在则更新,不存在则插入
	return o.taskRepo.SaveActivities(ctx, activityData)
}

func (o *Orchestrator) jsonString(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// appendFlowsToFile 将流量记录追加到任务专属的 flows.jsonl 文件
// 实现任务流量隔离，避免多任务混淆
func (o *Orchestrator) appendFlowsToFile(filePath string, flows []*flow.FlowRecord) error {
	// 打开文件用于追加写入，如果文件不存在则创建
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open flow file: %w", err)
	}
	defer file.Close()

	// 将每条流量记录写入 JSONL 文件（每行一个 JSON 对象）
	for _, flowRecord := range flows {
		jsonData, err := json.Marshal(flowRecord)
		if err != nil {
			o.logger.WithError(err).Warn("Failed to marshal flow record")
			continue
		}

		if _, err := file.Write(append(jsonData, '\n')); err != nil {
			o.logger.WithError(err).Warn("Failed to write flow record")
			continue
		}
	}

	return nil
}

// performDeepExploration 对核心 Activity 执行深度探索
// 使用智能点击识别UI元素，按优先级点击按钮
// 优化：循环检测页面变化，持续点击高优先级按钮直到无按钮可点
func (o *Orchestrator) performDeepExploration(ctx context.Context, activity string, adbClient *adb.Client) {
	o.logger.WithField("activity", activity).Info("Starting deep exploration with smart click")

	// 等待加载
	time.Sleep(2 * time.Second)

	if o.smartClicker == nil {
		o.logger.Warn("SmartClicker not initialized, skipping deep exploration")
		return
	}

	// 高优先级按钮列表（按优先级从高到低排序）
	// 游客、试用等跳过登录的按钮优先级最高
	highPriorityButtons := []string{
		// 游客/试用/跳过登录（最高优先级 - 快速进入应用）
		"游客登录", "游客模式", "游客", "试用", "体验", "随便看看",
		"跳过", "跳过登录", "稍后", "稍后再说", "暂不登录", "先逛逛", "以后再说",
		// 个人账号登录（教育类应用常见 - 优先于老师/机构账号）
		"个人账号", "个人账号注册", "个人账号登录", "个人注册",
		// 年龄确认/监护人同意（儿童应用常见）
		"已满14周岁", "已满16周岁", "已满18周岁", "已满14岁", "已满16岁", "已满18岁", "我已成年", "我已满",
		"监护人同意", "家长同意", "家长已阅读", "监护人已阅读",
		// 隐私协议/权限相关（次高优先级）
		"同意并继续", "同意并进入", "我同意", "同意",
		"允许", "确定", "确认", "接受", "授权", "继续",
		// 知道了/关闭弹窗
		"我知道了", "知道了", "好的", "好", "关闭", "OK",
		// 开始使用
		"开始体验", "立即体验", "开始使用", "进入",
	}

	// 循环点击：点击 -> 等待1秒 -> 检查页面变化 -> 继续点击
	maxRounds := 5 // 最多5轮点击，防止死循环
	for round := 0; round < maxRounds; round++ {
		o.logger.WithField("round", round+1).Info("🔍 Attempting to click high priority buttons")

		// 尝试点击高优先级按钮
		clicked, err := o.smartClicker.ClickButtonByText(ctx, adbClient, highPriorityButtons, 1)
		if err != nil {
			o.logger.WithError(err).Debug("Smart click failed")
			break
		}

		if !clicked {
			o.logger.Info("No more high priority buttons found")
			break
		}

		o.logger.Info("✅ Clicked high priority button, waiting for page change...")

		// 等待1秒，让页面有时间响应
		time.Sleep(1 * time.Second)

		// 页面可能已变化，继续下一轮检测
		// 下一轮会重新获取UI并查找按钮
	}

	// 滑动探索列表内容
	o.logger.Info("📜 Scrolling to explore content")
	for i := 0; i < 2; i++ {
		// 向下滑动
		adbClient.Shell(ctx, "input swipe 500 1500 500 500 300")
		time.Sleep(1500 * time.Millisecond)

		// 向上滑动
		adbClient.Shell(ctx, "input swipe 500 500 500 1500 300")
		time.Sleep(1500 * time.Millisecond)
	}

	// 左右滑动 (轮播图或多标签页)
	adbClient.Shell(ctx, "input swipe 800 960 200 960 300") // 左滑
	time.Sleep(1 * time.Second)
	adbClient.Shell(ctx, "input swipe 200 960 800 960 300") // 右滑
	time.Sleep(1 * time.Second)

	o.logger.WithField("activity", activity).Info("Deep exploration completed")
}

// runBackgroundMonitoring 后台监控应用，捕获延迟/周期性网络请求
func (o *Orchestrator) runBackgroundMonitoring(ctx context.Context, packageName string, duration time.Duration, adbClient *adb.Client) error {
	o.logger.WithFields(logrus.Fields{
		"package":  packageName,
		"duration": duration.String(),
	}).Info("Starting background monitoring")

	// 1. 启动应用到主 Activity（使用 am start 代替 monkey）
	launchCmd := fmt.Sprintf("am start -n %s/%s", packageName, "$(pm dump %s | grep -A 1 MAIN | grep %s | head -1 | awk '{print $2}')")
	if _, err := adbClient.Shell(ctx, launchCmd); err != nil {
		// 如果 am start 失败，尝试简单的包名启动
		simpleCmd := fmt.Sprintf("am start -a android.intent.action.MAIN -c android.intent.category.LAUNCHER %s", packageName)
		if _, err := adbClient.Shell(ctx, simpleCmd); err != nil {
			return fmt.Errorf("failed to launch app: %w", err)
		}
	}

	o.logger.Info("App launched, monitoring in background...")

	// 2. 保持应用在前台，持续监控指定时长
	startTime := time.Now()
	tickerInterval := 10 * time.Second
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if elapsed >= duration {
				o.logger.WithField("elapsed", elapsed.String()).Info("Background monitoring completed")
				return nil
			}

			// 每隔一段时间执行轻量级交互，保持应用活跃
			// 轻点屏幕中心 (避免误触重要按钮)
			adbClient.TapScreen(ctx, 540, 960)

			remaining := duration - elapsed
			o.logger.WithFields(logrus.Fields{
				"elapsed":   elapsed.String(),
				"remaining": remaining.String(),
			}).Debug("Background monitoring in progress...")
		}
	}
}

// registerPackageToMitmproxy 注册包名到 mitmproxy（用于并发任务流量隔离）
func (o *Orchestrator) registerPackageToMitmproxy(ctx context.Context, taskID, packageName string) error {
	// 构建API URL（mitmproxy容器的主机名通常是 apk-analysis-mitmproxy）
	apiURL := fmt.Sprintf("http://apk-analysis-mitmproxy:%s/register", o.mitmProxyAPIPort)

	// 构建请求体
	payload := map[string]string{
		"task_id":      taskID,
		"package_name": packageName,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 发送POST请求
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"package_name": packageName,
	}).Info("Registered package to mitmproxy for flow isolation")

	return nil
}

// unregisterPackageFromMitmproxy 从 mitmproxy 注销包名
func (o *Orchestrator) unregisterPackageFromMitmproxy(ctx context.Context, taskID string) error {
	// 构建API URL
	apiURL := fmt.Sprintf("http://apk-analysis-mitmproxy:%s/unregister", o.mitmProxyAPIPort)

	// 构建请求体
	payload := map[string]string{
		"task_id": taskID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 发送DELETE请求
	req, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	o.logger.WithFields(logrus.Fields{
		"task_id": taskID,
	}).Info("Unregistered package from mitmproxy")

	return nil
}

// setMitmproxyOutput 设置 mitmproxy 输出到任务专属文件 (已废弃,保留用于向后兼容)
// 调用 mitmproxy API: POST /set_output
func (o *Orchestrator) setMitmproxyOutput(ctx context.Context, taskID string) error {
	// 将 string 类型的端口转换为 int
	port := 8083 // 默认值
	fmt.Sscanf(o.mitmProxyAPIPort, "%d", &port)
	return o.setMitmproxyOutputForDevice(ctx, taskID, o.mitmProxyHost, port)
}

// clearMitmproxyOutput 清除 mitmproxy 输出设置（已废弃,保留用于向后兼容）
// 调用 mitmproxy API: POST /clear_output
func (o *Orchestrator) clearMitmproxyOutput(ctx context.Context) error {
	// 将 string 类型的端口转换为 int
	port := 8083 // 默认值
	fmt.Sscanf(o.mitmProxyAPIPort, "%d", &port)
	return o.clearMitmproxyOutputForDevice(ctx, o.mitmProxyHost, port)
}

// setMitmproxyOutputForDevice 为指定设备的 mitmproxy 实例设置输出到任务专属文件
// 调用 mitmproxy API: POST /set_output
func (o *Orchestrator) setMitmproxyOutputForDevice(ctx context.Context, taskID, mitmproxyHost string, apiPort int) error {
	// 构建 API URL（使用设备特定的 mitmproxy 容器和 API 端口）
	apiURL := fmt.Sprintf("http://%s:%d/set_output", mitmproxyHost, apiPort)

	// 构建请求体
	payload := map[string]string{
		"task_id": taskID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 发送 POST 请求
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":        taskID,
		"mitmproxy_host": mitmproxyHost,
		"api_port":       apiPort,
		"api_url":        apiURL,
	}).Info("Mitmproxy output set to task-specific file")

	return nil
}

// clearMitmproxyOutputForDevice 清除指定设备的 mitmproxy 实例的输出设置（切换回默认文件）
// 调用 mitmproxy API: POST /clear_output
func (o *Orchestrator) clearMitmproxyOutputForDevice(ctx context.Context, mitmproxyHost string, apiPort int) error {
	// 构建 API URL（使用设备特定的 mitmproxy 容器和 API 端口）
	apiURL := fmt.Sprintf("http://%s:%d/clear_output", mitmproxyHost, apiPort)

	// 发送 POST 请求（空 body）
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	o.logger.WithFields(logrus.Fields{
		"mitmproxy_host": mitmproxyHost,
		"api_port":       apiPort,
		"api_url":        apiURL,
	}).Info("Mitmproxy output cleared (back to default)")

	return nil
}

// performAIInteraction 使用AI智能交互引擎执行Activity交互
// 返回: (success bool, actions []ai.Action)
// - success: true表示成功，false表示失败(需要降级到传统方法)
// - actions: 实际执行的AI动作列表(用于保存到数据库)
func (o *Orchestrator) performAIInteraction(
	ctx context.Context,
	taskID, packageName, activity, uiHierarchyPath string,
	adbClient *adb.Client,
) (bool, []ai.Action) {
	// 1. 检查UI hierarchy文件是否存在
	if _, err := os.Stat(uiHierarchyPath); os.IsNotExist(err) {
		o.logger.WithError(err).Warn("UI hierarchy file not found, cannot perform AI interaction")
		return false, nil
	}

	// 2. 解析UI元素
	uiData, err := ai.ParseUIXML(uiHierarchyPath)
	if err != nil {
		o.logger.WithError(err).Warn("Failed to parse UI hierarchy XML")
		return false, nil
	}

	o.logger.WithFields(logrus.Fields{
		"clickable_elements": len(uiData.ClickableElements),
		"input_fields":       len(uiData.InputFields),
		"scrollable_views":   len(uiData.ScrollableViews),
	}).Debug("UI elements parsed successfully")

	// 3. 使用AI生成交互策略 (PlanActions参数: ctx, uiData, activityName, appCategory)
	actions, err := o.interactionEngine.PlanActions(ctx, uiData, activity, "unknown")
	if err != nil {
		o.logger.WithError(err).Warn("Failed to plan AI actions")
		return false, nil
	}

	if len(actions) == 0 {
		o.logger.Warn("AI返回空操作列表,降级到传统方法")
		return false, nil
	}

	o.logger.WithField("action_count", len(actions)).Info("AI interaction plan generated")

	// 4. 读取 UI XML 内容（用于安全检查）
	uiXMLContent := ""
	if xmlBytes, err := os.ReadFile(uiHierarchyPath); err == nil {
		uiXMLContent = string(xmlBytes)
	}

	// 屏幕尺寸（OnePlus 5T: 1080x2160, 标准屏幕: 1080x1920）
	// TODO: 从设备动态获取
	screenWidth := 1080
	screenHeight := 2160

	// 5. 执行AI生成的操作（使用安全执行方法）
	successCount := 0
	skippedCount := 0
	for i, action := range actions {
		o.logger.WithFields(logrus.Fields{
			"action_index":  i + 1,
			"total_actions": len(actions),
			"action_type":   action.Type,
			"priority":      action.Priority,
		}).Debug("Executing AI action")

		// 广播AI动作到前端（如果广播器已配置）
		if o.aiInteractionBroadcaster != nil {
			o.aiInteractionBroadcaster.BroadcastAction(taskID, activity, AIActionData{
				Type:     action.Type,
				X:        action.X,
				Y:        action.Y,
				Reason:   action.Reason,
				Priority: action.Priority,
			})
		}

		// 使用安全执行方法（带前置检查和后置恢复）
		// 构建目标 Activity 完整名称用于恢复
		targetActivity := fmt.Sprintf("%s/%s", packageName, activity)

		if err := o.interactionEngine.ExecuteActionSafe(ctx, action, adbClient, packageName, targetActivity, uiXMLContent, screenWidth, screenHeight); err != nil {
			o.logger.WithError(err).WithField("action_type", action.Type).Warn("Action execution failed")
			continue
		}

		// 检查操作是否被跳过（通过日志判断，或者这里增加返回值）
		successCount++

		// 操作间等待,给应用响应时间
		time.Sleep(2 * time.Second)
	}

	o.logger.WithFields(logrus.Fields{
		"success_count": successCount,
		"skipped_count": skippedCount,
		"total_actions": len(actions),
	}).Debug("AI actions execution summary")

	// 5. 判断成功率
	successRate := float64(successCount) / float64(len(actions))
	o.logger.WithFields(logrus.Fields{
		"success_count": successCount,
		"total_actions": len(actions),
		"success_rate":  fmt.Sprintf("%.1f%%", successRate*100),
	}).Info("AI interaction completed")

	// 如果成功率低于50%,视为失败
	if successRate < 0.5 {
		o.logger.Warn("AI interaction success rate too low, will fallback")
		return false, actions // 即使失败也返回actions数据
	}

	return true, actions // 成功,返回actions数据
}

// runStaticAnalysis 执行静态分析（Hybrid 分析器）和恶意检测
// 异步执行，不阻塞动态分析流程
func (o *Orchestrator) runStaticAnalysis(ctx context.Context, taskID, apkPath, packageName string) error {
	o.logger.WithFields(logrus.Fields{
		"task_id":         taskID,
		"hybrid_enabled":  o.hybridEnabled,
		"malware_enabled": o.malwareEnabled,
	}).Info("Starting static analysis and malware detection (async mode)")

	// 异步执行 Hybrid 分析
	if o.hybridEnabled {
		go func() {
			if err := o.runHybridAnalysis(context.Background(), taskID, apkPath, packageName); err != nil {
				o.logger.WithError(err).Error("❌ Hybrid analysis failed in async mode")
			}
		}()
	} else {
		o.logger.Warn("Hybrid analyzer not enabled, skipping static analysis")
	}

	// 异步执行恶意检测（与 Hybrid 分析并行）
	if o.malwareEnabled {
		go func() {
			if err := o.runMalwareDetection(context.Background(), taskID, apkPath); err != nil {
				o.logger.WithError(err).Error("❌ Malware detection failed in async mode")
			}
		}()
	}

	return nil // 立即返回，不阻塞
}

// runHybridAnalysis 执行混合静态分析（Go Fast + Python Deep）
func (o *Orchestrator) runHybridAnalysis(ctx context.Context, taskID, apkPath, packageName string) error {
	startTime := time.Now()
	o.logger.WithField("task_id", taskID).Info("Starting hybrid static analysis")

	// 创建初始报告记录
	report := &domain.TaskStaticReport{
		TaskID:      taskID,
		Analyzer:    "hybrid",
		Status:      domain.StaticStatusAnalyzing,
		PackageName: packageName,
		CreatedAt:   time.Now(),
	}

	if err := o.staticReportRepo.Upsert(ctx, report); err != nil {
		o.logger.WithError(err).Warn("Failed to create initial static report")
	}

	// 执行分析
	result, err := o.hybridAnalyzer.Analyze(ctx, apkPath)
	if err != nil {
		// 更新失败状态
		report.Status = domain.StaticStatusFailed
		o.staticReportRepo.Upsert(ctx, report)

		// 重要：静态分析失败时，不标记 StaticAnalysisCompleted
		// 这样域名分析不会被触发，任务状态也不会被错误修改
		o.logger.WithError(err).Error("❌ Hybrid static analysis failed, NOT marking as completed")
		return fmt.Errorf("hybrid analysis failed: %w", err)
	}

	// 保存分析结果
	if err := o.saveStaticAnalysisResult(ctx, taskID, result, packageName); err != nil {
		o.logger.WithError(err).Warn("Failed to save hybrid analysis result")
		// 保存失败也不应该标记完成
		return err
	}

	duration := time.Since(startTime)
	o.logger.WithFields(logrus.Fields{
		"task_id":        taskID,
		"analysis_mode":  result.AnalysisMode,
		"duration_ms":    duration.Milliseconds(),
		"package_name":   result.BasicInfo.PackageName,
	}).Info("Hybrid static analysis completed successfully")

	// 🔧 使用原子更新标记静态分析完成（避免并发竞态）
	if err := o.taskRepo.MarkStaticAnalysisCompleted(ctx, taskID); err != nil {
		o.logger.WithError(err).Warn("Failed to mark static analysis as completed")
	} else {
		// 检查是否应该触发域名分析（需要静态+动态都完成）
		o.checkAndTriggerDomainAnalysis(ctx, taskID, nil) // 传 nil 让它从数据库重新加载最新状态
	}

	return nil
}

// runMalwareDetection 执行恶意检测（异步执行，与静态分析并行）
func (o *Orchestrator) runMalwareDetection(ctx context.Context, taskID, apkPath string) error {
	if !o.malwareEnabled || o.malwareDetector == nil {
		o.logger.WithField("task_id", taskID).Debug("Malware detection disabled, skipping")
		return nil
	}

	startTime := time.Now()
	o.logger.WithField("task_id", taskID).Info("Starting malware detection")

	// 检查服务可用性
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := o.malwareDetector.CheckAvailability(checkCtx); err != nil {
		o.logger.WithError(err).Warn("Malware detection service unavailable, skipping")
		// 保存跳过状态
		if o.malwareRepo != nil {
			result := &malware.TaskMalwareResult{
				TaskID:       taskID,
				Status:       malware.DetectionStatusSkipped,
				ErrorMessage: fmt.Sprintf("service unavailable: %v", err),
				CreatedAt:    time.Now(),
			}
			o.malwareRepo.Upsert(ctx, result)
		}
		return nil
	}

	// 创建初始记录（分析中状态）
	if o.malwareRepo != nil {
		initialResult := &malware.TaskMalwareResult{
			TaskID:    taskID,
			Status:    malware.DetectionStatusRunning,
			CreatedAt: time.Now(),
		}
		if err := o.malwareRepo.Upsert(ctx, initialResult); err != nil {
			o.logger.WithError(err).Warn("Failed to create initial malware result record")
		}
	}

	// 执行恶意检测
	result, err := o.malwareDetector.Detect(ctx, apkPath,
		malware.WithTaskID(taskID),
		malware.WithGraphFeatures(true),
	)

	if err != nil {
		o.logger.WithError(err).Error("Malware detection failed")
		// 保存失败状态
		if o.malwareRepo != nil && result != nil {
			result.Status = malware.DetectionStatusFailed
			if result.ErrorMessage == "" {
				result.ErrorMessage = err.Error()
			}
			o.malwareRepo.Upsert(ctx, result)
		}
		return err
	}

	// 保存检测结果
	if o.malwareRepo != nil && result != nil {
		if err := o.malwareRepo.Upsert(ctx, result); err != nil {
			o.logger.WithError(err).Error("Failed to save malware detection result")
			return err
		}
	}

	duration := time.Since(startTime)
	o.logger.WithFields(logrus.Fields{
		"task_id":             taskID,
		"is_malware":          result.IsMalware,
		"confidence":          result.Confidence,
		"malware_probability": result.MalwareProbability,
		"benign_probability":  result.BenignProbability,
		"predicted_family":    result.PredictedFamily,
		"duration_ms":         duration.Milliseconds(),
		"total_time_ms":       result.TotalTimeMs,
	}).Info("✅ Malware detection completed")

	return nil
}

// ============================================
// AI 单步交互循环（新方案）
// ============================================

// ADBUIProvider 实现 ai.UIDataProvider 接口
type ADBUIProvider struct {
	adbClient *adb.Client
	taskDir   string
	logger    *logrus.Logger
}

// DumpUIHierarchy 获取 UI 层级 XML 内容
func (p *ADBUIProvider) DumpUIHierarchy(ctx context.Context) (string, error) {
	// 使用临时文件
	tmpPath := filepath.Join(p.taskDir, "tmp_ui_dump.xml")

	if err := p.adbClient.DumpUIHierarchy(ctx, tmpPath); err != nil {
		return "", err
	}

	// 读取文件内容
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read UI hierarchy file: %w", err)
	}

	// 删除临时文件
	os.Remove(tmpPath)

	return string(content), nil
}

// TakeScreenshot 截图
func (p *ADBUIProvider) TakeScreenshot(ctx context.Context, path string) error {
	return p.adbClient.Screenshot(ctx, path)
}

// runAISingleStepLoop 运行 AI 单步交互循环
// 在智能引导阶段之后执行，用于深度探索应用
func (o *Orchestrator) runAISingleStepLoop(
	ctx context.Context,
	taskID, packageName string,
	adbClient *adb.Client,
) *ai.AILoopResult {
	// 检查 AI 交互是否启用
	if !o.aiInteractionEnabled || o.interactionEngine == nil {
		o.logger.Info("AI 单步交互未启用，跳过")
		return &ai.AILoopResult{
			ExitReason: "AI交互未启用",
		}
	}

	taskDir := filepath.Join(o.resultsDir, taskID)
	os.MkdirAll(taskDir, 0755)

	// 创建 UI 数据提供器
	uiProvider := &ADBUIProvider{
		adbClient: adbClient,
		taskDir:   taskDir,
		logger:    o.logger,
	}

	// 获取当前 Activity 名称
	activityName := "MainActivity" // 默认值
	if currentActivity, err := adbClient.GetForegroundPackage(ctx); err == nil {
		activityName = currentActivity
	}

	// 从配置获取最大步数
	maxSteps := 20 // 默认值
	if o.interactionEngine != nil {
		// 可以从配置读取
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":     taskID,
		"package":     packageName,
		"max_steps":   maxSteps,
	}).Info("开始 AI 单步交互循环")

	// 广播状态到前端
	if o.aiInteractionBroadcaster != nil {
		o.aiInteractionBroadcaster.BroadcastStatus(taskID, "ai_loop_started")
	}

	// 执行 AI 交互循环
	result := o.interactionEngine.RunAIInteractionLoop(
		ctx,
		adbClient,
		uiProvider,
		packageName,
		activityName,
		maxSteps,
	)

	o.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"total_steps":   result.TotalSteps,
		"success_steps": result.SuccessSteps,
		"exit_reason":   result.ExitReason,
		"error_count":   len(result.Errors),
	}).Info("AI 单步交互循环完成")

	// 广播状态到前端
	if o.aiInteractionBroadcaster != nil {
		o.aiInteractionBroadcaster.BroadcastStatus(taskID, "ai_loop_completed")
	}

	return result
}

// saveStaticAnalysisResult 保存静态分析结果到数据库
func (o *Orchestrator) saveStaticAnalysisResult(ctx context.Context, taskID string, result *staticanalysis.AnalysisResult, packageName string) error {
	// 如果 Go 快速分析未能获取基本信息（没有 aapt2），尝试从 Python 深度分析结果中获取
	if result.BasicInfo != nil && result.BasicInfo.PackageName == "" && result.DeepAnalysis != nil && result.DeepAnalysis.BasicInfo != nil {
		deepBasic := result.DeepAnalysis.BasicInfo
		result.BasicInfo.PackageName = deepBasic.PackageName
		result.BasicInfo.VersionName = deepBasic.VersionName
		result.BasicInfo.VersionCode = deepBasic.VersionCode
		result.BasicInfo.AppName = deepBasic.AppName
		result.BasicInfo.MinSDK = deepBasic.MinSDK
		result.BasicInfo.TargetSDK = deepBasic.TargetSDK
		o.logger.WithField("task_id", taskID).Info("Filled basic info from Python Androguard analysis (aapt2 fallback)")
	}

	// 序列化 JSON 数据
	basicInfoJSON, err := json.Marshal(result.BasicInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal basic info: %w", err)
	}

	var deepAnalysisJSON []byte
	if result.DeepAnalysis != nil {
		deepAnalysisJSON, err = json.Marshal(result.DeepAnalysis)
		if err != nil {
			return fmt.Errorf("failed to marshal deep analysis: %w", err)
		}
	}

	// 计算 URL 和域名数量
	urlCount := 0
	domainCount := 0
	if result.DeepAnalysis != nil {
		urlCount = len(result.DeepAnalysis.URLs)
		domainCount = len(result.DeepAnalysis.Domains)
	}

	// 从证书中提取开发者和公司信息
	developer, companyName := o.extractCertificateInfo(result.DeepAnalysis)

	// 构建报告对象
	now := time.Now()
	report := &domain.TaskStaticReport{
		TaskID:                 taskID,
		Analyzer:               "hybrid",
		AnalysisMode:           domain.StaticAnalysisMode(result.AnalysisMode),
		Status:                 domain.StaticStatusCompleted,
		PackageName:            result.BasicInfo.PackageName,
		VersionName:            result.BasicInfo.VersionName,
		VersionCode:            result.BasicInfo.VersionCode,
		AppName:                result.BasicInfo.AppName,
		FileSize:               result.BasicInfo.FileSize,
		MD5:                    result.BasicInfo.MD5,
		SHA256:                 result.BasicInfo.SHA256,
		Developer:              developer,
		CompanyName:            companyName,
		ActivityCount:          result.BasicInfo.ActivityCount,
		ServiceCount:           result.BasicInfo.ServiceCount,
		ReceiverCount:          result.BasicInfo.ReceiverCount,
		ProviderCount:          result.BasicInfo.ProviderCount,
		PermissionCount:        len(result.BasicInfo.Permissions),
		URLCount:               urlCount,
		DomainCount:            domainCount,
		BasicInfoJSON:          string(basicInfoJSON),
		DeepAnalysisJSON:       string(deepAnalysisJSON),
		AnalysisDurationMs:     int(result.AnalysisDuration),
		FastAnalysisDurationMs: int(result.FastAnalysisDuration),
		DeepAnalysisDurationMs: int(result.DeepAnalysisDuration),
		NeedsDeepAnalysisReason: result.NeedsDeepAnalysisReason,
		AnalyzedAt:             &now,
		CreatedAt:              time.Now(),
	}

	// UPSERT 到数据库
	if err := o.staticReportRepo.Upsert(ctx, report); err != nil {
		return fmt.Errorf("failed to save static report: %w", err)
	}

	// 🔧 使用原子更新 app_name（避免被动态分析并发操作覆盖）
	if result.BasicInfo != nil && result.BasicInfo.AppName != "" {
		if err := o.taskRepo.UpdateAppName(ctx, taskID, result.BasicInfo.AppName); err != nil {
			o.logger.WithError(err).Warn("Failed to update task app_name")
		}
	}

	o.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"package_name": packageName,
		"mode":         result.AnalysisMode,
		"duration_ms":  result.AnalysisDuration,
	}).Info("Static analysis result saved to database")

	return nil
}

// launchApp 启动应用
func (o *Orchestrator) launchApp(ctx context.Context, packageName string, adbClient *adb.Client) error {
	o.logger.WithField("package", packageName).Info("启动应用")

	// 方法1: 使用 monkey 命令启动（最可靠）
	cmd := fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", packageName)
	_, err := adbClient.Shell(ctx, cmd)
	if err != nil {
		o.logger.WithError(err).Warn("monkey 启动失败，尝试 am start")

		// 方法2: 尝试使用 am start 启动
		startCmd := fmt.Sprintf("am start -a android.intent.action.MAIN -c android.intent.category.LAUNCHER %s", packageName)
		_, err = adbClient.Shell(ctx, startCmd)
		if err != nil {
			return fmt.Errorf("启动应用失败: %w", err)
		}
	}

	// 等待应用启动
	time.Sleep(2 * time.Second)

	// 验证应用是否在前台
	currentPkg, err := adbClient.GetForegroundPackage(ctx)
	if err != nil {
		o.logger.WithError(err).Warn("无法获取前台应用")
		return nil // 不影响后续流程
	}

	if currentPkg != packageName {
		o.logger.WithFields(logrus.Fields{
			"expected": packageName,
			"actual":   currentPkg,
		}).Warn("应用可能未成功启动到前台")
	} else {
		o.logger.WithField("package", packageName).Info("应用已成功启动到前台")
	}

	return nil
}

// extractCertificateInfo 从深度分析结果中提取开发者和公司信息
// Python 脚本直接返回 developer 和 company 字段
func (o *Orchestrator) extractCertificateInfo(deepAnalysis *staticanalysis.DeepAnalysisResult) (developer, companyName string) {
	if deepAnalysis == nil || deepAnalysis.Certificates == nil {
		return "", ""
	}

	// 优先使用 Python 脚本直接返回的 developer 和 company 字段
	if devVal, ok := deepAnalysis.Certificates["developer"]; ok {
		if dev, ok := devVal.(string); ok && dev != "" {
			developer = dev
		}
	}

	if compVal, ok := deepAnalysis.Certificates["company"]; ok {
		if comp, ok := compVal.(string); ok && comp != "" {
			companyName = comp
		}
	}

	// 如果直接字段为空，回退到解析 subject 字符串
	if developer == "" || companyName == "" {
		if subjectVal, ok := deepAnalysis.Certificates["subject"]; ok {
			if subject, ok := subjectVal.(string); ok && subject != "" {
				if developer == "" {
					developer = o.extractRDNValue(subject, "Common Name")
					if developer == "" {
						developer = o.extractRDNValue(subject, "CN")
					}
				}
				if companyName == "" {
					companyName = o.extractRDNValue(subject, "Organization")
					if companyName == "" {
						companyName = o.extractRDNValue(subject, "O")
					}
				}
			}
		}
	}

	o.logger.WithFields(logrus.Fields{
		"developer":    developer,
		"company_name": companyName,
	}).Debug("Extracted certificate info")

	return developer, companyName
}

// extractRDNValue 从证书 subject 字符串中提取指定字段的值
// 支持格式: "Common Name: value, Organization: value" 或 "CN=value,O=value"
func (o *Orchestrator) extractRDNValue(dn, rdnType string) string {
	// 尝试 "Key: Value" 格式 (asn1crypto human_friendly)
	colonPrefix := rdnType + ": "
	if idx := strings.Index(dn, colonPrefix); idx != -1 {
		start := idx + len(colonPrefix)
		end := strings.Index(dn[start:], ", ")
		if end == -1 {
			return strings.TrimSpace(dn[start:])
		}
		return strings.TrimSpace(dn[start : start+end])
	}

	// 尝试 "Key=Value" 格式 (RFC4514)
	equalPrefix := rdnType + "="
	if idx := strings.Index(dn, equalPrefix); idx != -1 {
		start := idx + len(equalPrefix)
		if start >= len(dn) {
			return ""
		}
		// 找到值的结束位置（未转义的逗号或字符串结尾）
		var value strings.Builder
		escaped := false
		for i := start; i < len(dn); i++ {
			ch := dn[i]
			if escaped {
				value.WriteByte(ch)
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == ',' {
				break
			} else {
				value.WriteByte(ch)
			}
		}
		return strings.TrimSpace(value.String())
	}

	return ""
}
