package staticanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/packer"
	"github.com/sirupsen/logrus"
)

// HybridAnalyzer 混合分析器（Go + Python）
type HybridAnalyzer struct {
	fastAnalyzer      *FastAnalyzer
	packerDetector    *packer.Detector // 壳检测器
	pythonPath        string
	scriptPath        string
	processPool       *ProcessPool
	logger            *logrus.Logger
	usePool           bool // 是否使用进程池
	forceDeepAnalysis bool // 强制所有APK都进行深度分析
}

// HybridConfig 混合分析器配置
type HybridConfig struct {
	PythonPath        string
	ScriptPath        string
	UseProcessPool    bool
	ProcessPoolSize   int
	ForceDeepAnalysis bool // 强制所有APK都进行深度分析
}

// NewHybridAnalyzer 创建混合分析器
func NewHybridAnalyzer(config *HybridConfig, logger *logrus.Logger) (*HybridAnalyzer, error) {
	ha := &HybridAnalyzer{
		fastAnalyzer:      NewFastAnalyzer(logger),
		packerDetector:    packer.NewDetector(logger), // 初始化壳检测器
		pythonPath:        config.PythonPath,
		scriptPath:        config.ScriptPath,
		logger:            logger,
		usePool:           config.UseProcessPool,
		forceDeepAnalysis: config.ForceDeepAnalysis,
	}

	// 如果启用进程池，则创建进程池
	if config.UseProcessPool {
		pool, err := NewProcessPool(config.PythonPath, config.ScriptPath, config.ProcessPoolSize, logger)
		if err != nil {
			logger.WithError(err).Warn("Failed to create process pool, will use direct mode")
			ha.usePool = false
		} else {
			ha.processPool = pool
			logger.Info("Hybrid analyzer initialized with process pool")
		}
	}

	if config.ForceDeepAnalysis {
		logger.Info("Force deep analysis enabled - all APKs will undergo deep analysis")
	}

	return ha, nil
}

// Analyze 智能分析（自动选择快速/深度模式）
func (ha *HybridAnalyzer) Analyze(ctx context.Context, apkPath string) (*AnalysisResult, error) {
	startTime := time.Now()

	result := &AnalysisResult{
		AnalyzedAt: startTime,
	}

	// Step 1: Go 快速预分析（必须执行）
	ha.logger.Info("Starting fast analysis (Go)...")
	fastStart := time.Now()

	basicInfo, err := ha.fastAnalyzer.AnalyzeFast(ctx, apkPath)
	if err != nil {
		return nil, fmt.Errorf("fast analysis failed: %w", err)
	}

	result.BasicInfo = basicInfo
	result.FastAnalysisDuration = time.Since(fastStart).Milliseconds()

	// Step 1.5: 壳检测
	ha.logger.Info("Starting packer detection...")
	packerStart := time.Now()

	packerInfo := ha.packerDetector.Detect(ctx, apkPath)
	result.PackerInfo = packerInfo
	result.PackerDetectionDuration = time.Since(packerStart).Milliseconds()

	if packerInfo.IsPacked {
		ha.logger.WithFields(logrus.Fields{
			"packer_name": packerInfo.PackerName,
			"packer_type": packerInfo.PackerType,
			"confidence":  packerInfo.Confidence,
			"can_unpack":  packerInfo.CanUnpack,
		}).Warn("⚠️ Packer detected! Dynamic unpacking may be required")
		result.NeedsDynamicUnpacking = packerInfo.CanUnpack
	} else {
		ha.logger.Info("No packer detected")
	}

	// Step 2: 判断是否需要深度分析
	var needDeep bool
	var reason string

	if ha.forceDeepAnalysis {
		// 强制深度分析模式：所有APK都进行深度分析
		needDeep = true
		reason = "force_deep_analysis_enabled"
		ha.logger.Info("Force deep analysis mode: all APKs undergo deep analysis")
	} else {
		// 智能决策模式：根据阈值判断
		needDeep, reason = ha.fastAnalyzer.NeedsDeepAnalysis(basicInfo)
	}

	result.NeedsDeepAnalysisReason = reason

	if needDeep {
		ha.logger.WithField("reason", reason).Info("Deep analysis required, calling Python Androguard...")
		result.AnalysisMode = "deep"

		// 调用 Python 深度分析
		deepStart := time.Now()
		deepResult, err := ha.analyzeDeep(ctx, apkPath)
		if err != nil {
			ha.logger.WithError(err).Warn("Deep analysis failed, using fast mode only")
			result.AnalysisMode = "fast_fallback"
		} else {
			result.DeepAnalysis = deepResult
			result.DeepAnalysisDuration = time.Since(deepStart).Milliseconds()
		}
	} else {
		ha.logger.WithField("reason", reason).Info("Fast analysis sufficient, skipping deep analysis")
		result.AnalysisMode = "fast"
	}

	// Step 3: 记录总耗时
	result.AnalysisDuration = time.Since(startTime).Milliseconds()

	logFields := logrus.Fields{
		"package_name": basicInfo.PackageName,
		"mode":         result.AnalysisMode,
		"duration_ms":  result.AnalysisDuration,
		"fast_ms":      result.FastAnalysisDuration,
		"deep_ms":      result.DeepAnalysisDuration,
		"packer_ms":    result.PackerDetectionDuration,
	}

	if result.DeepAnalysis != nil {
		logFields["urls_found"] = len(result.DeepAnalysis.URLs)
		logFields["domains_found"] = len(result.DeepAnalysis.Domains)
	} else {
		logFields["urls_found"] = 0
		logFields["domains_found"] = 0
	}

	// 添加壳检测结果
	if result.PackerInfo != nil && result.PackerInfo.IsPacked {
		logFields["is_packed"] = true
		logFields["packer_name"] = result.PackerInfo.PackerName
		logFields["needs_unpack"] = result.NeedsDynamicUnpacking
	} else {
		logFields["is_packed"] = false
	}

	ha.logger.WithFields(logFields).Info("Analysis completed")

	return result, nil
}

// AnalyzeDeep 强制深度分析（无论决策结果）
func (ha *HybridAnalyzer) AnalyzeDeep(ctx context.Context, apkPath string) (*AnalysisResult, error) {
	startTime := time.Now()

	// Step 1: 快速分析
	fastStart := time.Now()
	basicInfo, err := ha.fastAnalyzer.AnalyzeFast(ctx, apkPath)
	if err != nil {
		return nil, err
	}
	fastDuration := time.Since(fastStart).Milliseconds()

	// Step 2: 强制深度分析
	deepStart := time.Now()
	deepResult, err := ha.analyzeDeep(ctx, apkPath)
	if err != nil {
		return nil, err
	}
	deepDuration := time.Since(deepStart).Milliseconds()

	return &AnalysisResult{
		BasicInfo:            basicInfo,
		DeepAnalysis:         deepResult,
		AnalysisMode:         "deep",
		AnalysisDuration:     time.Since(startTime).Milliseconds(),
		FastAnalysisDuration: fastDuration,
		DeepAnalysisDuration: deepDuration,
		AnalyzedAt:           startTime,
	}, nil
}

// analyzeDeep 调用 Python 深度分析
func (ha *HybridAnalyzer) analyzeDeep(ctx context.Context, apkPath string) (*DeepAnalysisResult, error) {
	if ha.usePool && ha.processPool != nil {
		// 方案 1: 使用进程池（推荐，复用 Python 进程）
		return ha.analyzeDeepWithPool(ctx, apkPath)
	}

	// 方案 2: 直接调用（简单但可能慢）
	return ha.analyzeDeepDirect(ctx, apkPath)
}

// analyzeDeepDirect 直接调用 Python 脚本
func (ha *HybridAnalyzer) analyzeDeepDirect(ctx context.Context, apkPath string) (*DeepAnalysisResult, error) {
	// 注意：这里使用 androguard_deep_analyzer.py（非服务模式）
	cmd := exec.CommandContext(ctx, ha.pythonPath, ha.scriptPath, apkPath)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("python script failed: %w (output: %s)", err, string(output))
	}

	var result DeepAnalysisResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse python output: %w (output: %s)", err, string(output))
	}

	return &result, nil
}

// analyzeDeepWithPool 使用进程池调用
func (ha *HybridAnalyzer) analyzeDeepWithPool(ctx context.Context, apkPath string) (*DeepAnalysisResult, error) {
	resultChan := make(chan *DeepAnalysisResult, 1)
	errorChan := make(chan error, 1)

	// 创建任务
	task := &AnalysisTask{
		APKPath: apkPath,
		Callback: func(result *DeepAnalysisResult, err error) {
			if err != nil {
				errorChan <- err
			} else {
				resultChan <- result
			}
		},
	}

	// 提交任务到进程池
	if err := ha.processPool.Submit(task); err != nil {
		return nil, fmt.Errorf("failed to submit task to process pool: %w", err)
	}

	// 等待结果
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errorChan:
		return nil, err
	case result := <-resultChan:
		return result, nil
	}
}

// Stop 停止混合分析器（释放资源）
func (ha *HybridAnalyzer) Stop() {
	if ha.processPool != nil {
		ha.processPool.Stop()
	}
	ha.logger.Info("Hybrid analyzer stopped")
}

// GetStats 获取统计信息
func (ha *HybridAnalyzer) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"use_pool": ha.usePool,
	}

	if ha.processPool != nil {
		stats["process_pool"] = ha.processPool.GetStats()
	}

	return stats
}
