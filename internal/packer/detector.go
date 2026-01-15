package packer

import (
	"archive/zip"
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// Detector 壳检测器
type Detector struct {
	rules  []PackerRule
	logger *logrus.Logger
}

// NewDetector 创建壳检测器
func NewDetector(logger *logrus.Logger) *Detector {
	rules := GetBuiltinRules()
	// 按优先级降序排序
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})

	return &Detector{
		rules:  rules,
		logger: logger,
	}
}

// Detect 检测APK是否加壳
func (d *Detector) Detect(ctx context.Context, apkPath string) *PackerInfo {
	result := &PackerInfo{
		IsPacked:   false,
		Indicators: []string{},
	}

	d.logger.WithField("apk", apkPath).Debug("Starting packer detection")

	// 收集APK统计信息
	stats, err := d.collectAPKStats(apkPath)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect APK stats")
		return result
	}

	d.logger.WithFields(logrus.Fields{
		"native_libs":  len(stats.NativeLibs),
		"dex_size":     stats.DEXSize,
		"native_size":  stats.NativeSize,
		"dex_count":    stats.DEXCount,
		"has_multidex": stats.HasMultiDex,
	}).Debug("APK stats collected")

	// 匹配规则
	for _, rule := range d.rules {
		confidence, indicators := d.matchRule(rule, stats)

		if confidence >= 0.4 {
			result.IsPacked = true
			result.PackerName = rule.Name
			result.PackerType = rule.Type
			result.Confidence = min(confidence, 1.0)
			result.Indicators = indicators
			result.CanUnpack = rule.CanUnpack
			result.UnpackMethod = rule.UnpackMethod

			d.logger.WithFields(logrus.Fields{
				"packer_name": result.PackerName,
				"packer_type": result.PackerType,
				"confidence":  result.Confidence,
				"indicators":  result.Indicators,
				"can_unpack":  result.CanUnpack,
			}).Info("Packer detected")

			return result
		}
	}

	d.logger.Debug("No packer detected")
	return result
}

// DetectWithBasicInfo 使用已有的基础信息检测壳（避免重复解析）
func (d *Detector) DetectWithBasicInfo(ctx context.Context, apkPath string, nativeLibs []string, dexSize int64) *PackerInfo {
	result := &PackerInfo{
		IsPacked:   false,
		Indicators: []string{},
	}

	stats := &APKPackerStats{
		NativeLibs: nativeLibs,
		DEXSize:    dexSize,
	}

	// 计算 Native 库大小需要重新读取
	if reader, err := zip.OpenReader(apkPath); err == nil {
		defer reader.Close()
		for _, file := range reader.File {
			name := file.Name
			if strings.HasPrefix(name, "lib/") && strings.HasSuffix(name, ".so") {
				stats.NativeSize += int64(file.UncompressedSize64)
			}
			if strings.HasSuffix(name, ".dex") {
				stats.DEXCount++
			}
		}
		stats.HasMultiDex = stats.DEXCount > 1
	}

	// 匹配规则
	for _, rule := range d.rules {
		confidence, indicators := d.matchRule(rule, stats)

		if confidence >= 0.4 {
			result.IsPacked = true
			result.PackerName = rule.Name
			result.PackerType = rule.Type
			result.Confidence = min(confidence, 1.0)
			result.Indicators = indicators
			result.CanUnpack = rule.CanUnpack
			result.UnpackMethod = rule.UnpackMethod

			d.logger.WithFields(logrus.Fields{
				"packer_name": result.PackerName,
				"confidence":  result.Confidence,
			}).Info("Packer detected")

			return result
		}
	}

	return result
}

// collectAPKStats 收集APK统计信息
func (d *Detector) collectAPKStats(apkPath string) (*APKPackerStats, error) {
	stats := &APKPackerStats{
		NativeLibs:      []string{},
		SuspiciousFiles: []string{},
	}

	reader, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		name := file.Name

		// 收集Native库
		if strings.HasPrefix(name, "lib/") && strings.HasSuffix(name, ".so") {
			libName := filepath.Base(name)
			stats.NativeLibs = append(stats.NativeLibs, libName)
			stats.NativeSize += int64(file.UncompressedSize64)
		}

		// 收集DEX大小
		if strings.HasSuffix(name, ".dex") {
			stats.DEXSize += int64(file.UncompressedSize64)
			stats.DEXCount++
		}

		// 检测可疑文件
		if d.isSuspiciousFile(name) {
			stats.SuspiciousFiles = append(stats.SuspiciousFiles, name)
		}
	}

	stats.HasMultiDex = stats.DEXCount > 1

	return stats, nil
}

// matchRule 匹配单个规则
func (d *Detector) matchRule(rule PackerRule, stats *APKPackerStats) (float64, []string) {
	confidence := 0.0
	indicators := []string{}

	// 检查Native库匹配
	for _, ruleLib := range rule.NativeLibs {
		for _, apkLib := range stats.NativeLibs {
			if d.matchLibName(ruleLib, apkLib) {
				confidence += 0.4
				indicators = append(indicators, "native_lib:"+apkLib)
			}
		}
	}

	// 检查文件大小异常
	if rule.FileSize.DEXMaxKB > 0 && stats.DEXSize > 0 {
		dexSizeKB := stats.DEXSize / 1024
		if dexSizeKB < rule.FileSize.DEXMaxKB {
			confidence += 0.3
			indicators = append(indicators, "dex_size_anomaly")
		}
	}

	if rule.FileSize.NativeMinMB > 0 && stats.NativeSize > 0 {
		nativeSizeMB := stats.NativeSize / (1024 * 1024)
		if nativeSizeMB > rule.FileSize.NativeMinMB {
			confidence += 0.3
			indicators = append(indicators, "native_size_anomaly")
		}
	}

	// 检查可疑文件
	for _, suspFile := range stats.SuspiciousFiles {
		for _, ruleStr := range rule.Strings {
			if strings.Contains(strings.ToLower(suspFile), strings.ToLower(ruleStr)) {
				confidence += 0.2
				indicators = append(indicators, "suspicious_file:"+suspFile)
			}
		}
	}

	return confidence, indicators
}

// matchLibName 匹配库名（支持模糊匹配）
func (d *Detector) matchLibName(pattern, name string) bool {
	// 精确匹配
	if pattern == name {
		return true
	}

	// 移除版本号后匹配 (如 libshellx-2.10.3.4.so -> libshellx.so)
	patternBase := strings.TrimSuffix(pattern, ".so")
	nameBase := strings.TrimSuffix(name, ".so")

	// 检查是否以相同前缀开始
	if strings.HasPrefix(nameBase, patternBase) || strings.HasPrefix(patternBase, nameBase) {
		return true
	}

	// 检查是否包含核心名称
	patternCore := strings.TrimPrefix(patternBase, "lib")
	nameCore := strings.TrimPrefix(nameBase, "lib")

	// 移除数字和特殊字符后比较
	patternCore = strings.Split(patternCore, "-")[0]
	nameCore = strings.Split(nameCore, "-")[0]

	return patternCore == nameCore
}

// isSuspiciousFile 检查是否为可疑文件
func (d *Detector) isSuspiciousFile(name string) bool {
	suspiciousPatterns := []string{
		// 加固相关
		"stub",
		"shell",
		"protect",
		"guard",
		"jiagu",
		"secneo",
		"ijiami",
		"bangcle",
		"nagapt",
		// 可疑资源
		"assets/classes",
		"assets/dex",
		"assets/jiagu",
		"assets/protect",
	}

	nameLower := strings.ToLower(name)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}

	return false
}

// GetPackerSummary 获取壳检测摘要信息
func (d *Detector) GetPackerSummary(info *PackerInfo) string {
	if !info.IsPacked {
		return "未检测到加壳"
	}

	var summary strings.Builder
	summary.WriteString("检测到加壳: ")
	summary.WriteString(info.PackerName)
	summary.WriteString(" (")
	summary.WriteString(info.PackerType)
	summary.WriteString(")")

	if info.CanUnpack {
		summary.WriteString(" [可自动脱壳]")
	} else {
		summary.WriteString(" [需手动脱壳]")
	}

	return summary.String()
}

// min 返回两个float64中的较小值
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
