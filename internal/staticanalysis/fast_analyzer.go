package staticanalysis

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// FastAnalyzer Go 原生快速分析器
type FastAnalyzer struct {
	logger     *logrus.Logger
	aaptPath   string // aapt2 可执行文件路径
	useAapt    bool   // 是否使用 aapt2（fallback 到简单模式）
}

// NewFastAnalyzer 创建快速分析器
func NewFastAnalyzer(logger *logrus.Logger) *FastAnalyzer {
	fa := &FastAnalyzer{
		logger:   logger,
		aaptPath: "aapt2", // 默认从 PATH 查找
		useAapt:  true,
	}

	// 检查 aapt2 是否可用
	if err := fa.checkAapt(); err != nil {
		fa.logger.Warn("aapt2 not available, will use fallback mode")
		fa.useAapt = false
	}

	return fa
}

// checkAapt 检查 aapt2 是否可用
func (fa *FastAnalyzer) checkAapt() error {
	cmd := exec.Command(fa.aaptPath, "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aapt2 not found: %w", err)
	}
	return nil
}

// AnalyzeFast 快速分析 APK（2-5 秒完成）
func (fa *FastAnalyzer) AnalyzeFast(ctx context.Context, apkPath string) (*BasicInfo, error) {
	startTime := time.Now()

	fa.logger.WithField("apk_path", apkPath).Info("Starting fast analysis")

	info := &BasicInfo{
		FileName: filepath.Base(apkPath),
	}

	// 1. 获取文件大小
	fileInfo, err := os.Stat(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat APK file: %w", err)
	}
	info.FileSize = fileInfo.Size()

	// 2. 并行计算哈希
	hashChan := make(chan map[string]string, 1)
	go func() {
		hashes, err := fa.calculateHashes(apkPath)
		if err != nil {
			fa.logger.WithError(err).Warn("Failed to calculate hashes")
			hashChan <- map[string]string{"md5": "", "sha256": ""}
		} else {
			hashChan <- hashes
		}
	}()

	// 3. 解析 AndroidManifest.xml
	manifest, err := fa.extractManifest(ctx, apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract manifest: %w", err)
	}

	// 4. 填充基础信息
	info.PackageName = manifest.Package
	info.VersionName = manifest.VersionName
	info.VersionCode = manifest.VersionCode
	info.AppName = manifest.Application.Label
	info.MinSDK = manifest.UsesSdk.MinSdkVersion
	info.TargetSDK = manifest.UsesSdk.TargetSdkVersion

	// 5. 提取组件
	info.Activities = fa.extractActivityNames(manifest)
	info.Services = fa.extractServiceNames(manifest)
	info.Receivers = fa.extractReceiverNames(manifest)
	info.Providers = fa.extractProviderNames(manifest)

	// 6. 提取权限
	info.Permissions = fa.extractPermissions(manifest)

	// 7. 统计数量
	info.ActivityCount = len(info.Activities)
	info.ServiceCount = len(info.Services)
	info.ReceiverCount = len(info.Receivers)
	info.ProviderCount = len(info.Providers)
	info.PermissionCount = len(info.Permissions)

	// 8. 等待哈希计算完成
	hashes := <-hashChan
	info.MD5 = hashes["md5"]
	info.SHA256 = hashes["sha256"]

	duration := time.Since(startTime)
	fa.logger.WithFields(logrus.Fields{
		"package_name": info.PackageName,
		"duration_ms":  duration.Milliseconds(),
	}).Info("Fast analysis completed")

	return info, nil
}

// calculateHashes 并行计算文件哈希（MD5 和 SHA256）
func (fa *FastAnalyzer) calculateHashes(apkPath string) (map[string]string, error) {
	file, err := os.Open(apkPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 使用 io.MultiWriter 同时计算两个哈希
	md5Hash := md5.New()
	sha256Hash := sha256.New()
	multiWriter := io.MultiWriter(md5Hash, sha256Hash)

	if _, err := io.Copy(multiWriter, file); err != nil {
		return nil, err
	}

	return map[string]string{
		"md5":    fmt.Sprintf("%x", md5Hash.Sum(nil)),
		"sha256": fmt.Sprintf("%x", sha256Hash.Sum(nil)),
	}, nil
}

// extractManifest 从 APK 中提取 AndroidManifest.xml
func (fa *FastAnalyzer) extractManifest(ctx context.Context, apkPath string) (*AndroidManifest, error) {
	if fa.useAapt {
		// 优先使用 aapt2
		return fa.parseManifestWithAapt2(ctx, apkPath)
	}

	// Fallback: 简单模式（仅提取基本信息）
	return fa.parseManifestFallback(apkPath)
}

// parseManifestWithAapt2 使用 aapt2 解析 Manifest
func (fa *FastAnalyzer) parseManifestWithAapt2(ctx context.Context, apkPath string) (*AndroidManifest, error) {
	// 执行 aapt2 dump xmltree
	cmd := exec.CommandContext(ctx, fa.aaptPath, "dump", "xmltree", apkPath, "AndroidManifest.xml")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("aapt2 command failed: %w", err)
	}

	// 解析 aapt2 输出
	return fa.parseAaptOutput(string(output))
}

// parseAaptOutput 解析 aapt2 输出
func (fa *FastAnalyzer) parseAaptOutput(output string) (*AndroidManifest, error) {
	manifest := &AndroidManifest{}

	// 提取包名
	pkgRe := regexp.MustCompile(`A: package="([^"]+)"`)
	if match := pkgRe.FindStringSubmatch(output); len(match) > 1 {
		manifest.Package = match[1]
	}

	// 提取版本名称
	versionNameRe := regexp.MustCompile(`A: android:versionName\([^)]*\)="([^"]+)"`)
	if match := versionNameRe.FindStringSubmatch(output); len(match) > 1 {
		manifest.VersionName = match[1]
	}

	// 提取版本代码（十六进制转十进制）
	versionCodeRe := regexp.MustCompile(`A: android:versionCode\([^)]*\)=\(type 0x10\)0x([0-9a-f]+)`)
	if match := versionCodeRe.FindStringSubmatch(output); len(match) > 1 {
		manifest.VersionCode = match[1]
	}

	// 提取应用名称（label）
	appLabelRe := regexp.MustCompile(`A: android:label\([^)]*\)="([^"]+)"`)
	if match := appLabelRe.FindStringSubmatch(output); len(match) > 1 {
		manifest.Application.Label = match[1]
	}

	// 提取 minSdkVersion
	minSdkRe := regexp.MustCompile(`A: android:minSdkVersion\([^)]*\)=\(type 0x10\)0x([0-9a-f]+)`)
	if match := minSdkRe.FindStringSubmatch(output); len(match) > 1 {
		manifest.UsesSdk.MinSdkVersion = match[1]
	}

	// 提取 targetSdkVersion
	targetSdkRe := regexp.MustCompile(`A: android:targetSdkVersion\([^)]*\)=\(type 0x10\)0x([0-9a-f]+)`)
	if match := targetSdkRe.FindStringSubmatch(output); len(match) > 1 {
		manifest.UsesSdk.TargetSdkVersion = match[1]
	}

	// 提取 Activity
	activityRe := regexp.MustCompile(`E: activity[^E]*?A: android:name\([^)]*\)="([^"]+)"`)
	for _, match := range activityRe.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			manifest.Application.Activities = append(manifest.Application.Activities, Activity{Name: match[1]})
		}
	}

	// 提取 Service
	serviceRe := regexp.MustCompile(`E: service[^E]*?A: android:name\([^)]*\)="([^"]+)"`)
	for _, match := range serviceRe.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			manifest.Application.Services = append(manifest.Application.Services, Service{Name: match[1]})
		}
	}

	// 提取 Receiver
	receiverRe := regexp.MustCompile(`E: receiver[^E]*?A: android:name\([^)]*\)="([^"]+)"`)
	for _, match := range receiverRe.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			manifest.Application.Receivers = append(manifest.Application.Receivers, Receiver{Name: match[1]})
		}
	}

	// 提取 Provider
	providerRe := regexp.MustCompile(`E: provider[^E]*?A: android:name\([^)]*\)="([^"]+)"`)
	for _, match := range providerRe.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			manifest.Application.Providers = append(manifest.Application.Providers, Provider{Name: match[1]})
		}
	}

	// 提取权限
	permRe := regexp.MustCompile(`E: uses-permission[^E]*?A: android:name\([^)]*\)="([^"]+)"`)
	for _, match := range permRe.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			manifest.UsesPermissions = append(manifest.UsesPermissions, UsesPermission{Name: match[1]})
		}
	}

	return manifest, nil
}

// parseManifestFallback Fallback 模式（简单解析）
func (fa *FastAnalyzer) parseManifestFallback(apkPath string) (*AndroidManifest, error) {
	// 打开 APK 文件
	reader, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK as zip: %w", err)
	}
	defer reader.Close()

	// 查找 AndroidManifest.xml
	for _, file := range reader.File {
		if file.Name == "AndroidManifest.xml" {
			// APK 中的 AndroidManifest.xml 是二进制格式，简单模式无法解析
			// 返回空 manifest，仅设置文件名
			fa.logger.Warn("Fallback mode: Cannot parse binary AndroidManifest.xml")
			return &AndroidManifest{}, nil
		}
	}

	return nil, fmt.Errorf("AndroidManifest.xml not found in APK")
}

// 提取组件名称的辅助函数

func (fa *FastAnalyzer) extractActivityNames(manifest *AndroidManifest) []string {
	var names []string
	for _, activity := range manifest.Application.Activities {
		names = append(names, activity.Name)
	}
	return names
}

func (fa *FastAnalyzer) extractServiceNames(manifest *AndroidManifest) []string {
	var names []string
	for _, service := range manifest.Application.Services {
		names = append(names, service.Name)
	}
	return names
}

func (fa *FastAnalyzer) extractReceiverNames(manifest *AndroidManifest) []string {
	var names []string
	for _, receiver := range manifest.Application.Receivers {
		names = append(names, receiver.Name)
	}
	return names
}

func (fa *FastAnalyzer) extractProviderNames(manifest *AndroidManifest) []string {
	var names []string
	for _, provider := range manifest.Application.Providers {
		names = append(names, provider.Name)
	}
	return names
}

func (fa *FastAnalyzer) extractPermissions(manifest *AndroidManifest) []string {
	var perms []string
	for _, perm := range manifest.UsesPermissions {
		perms = append(perms, perm.Name)
	}
	return perms
}

// NeedsDeepAnalysis 判断是否需要深度分析
func (fa *FastAnalyzer) NeedsDeepAnalysis(info *BasicInfo) (bool, string) {
	// 决策逻辑 1: 文件大小判断（> 10MB）
	if info.FileSize > 10*1024*1024 {
		return true, fmt.Sprintf("file_size_large (%.2f MB)", float64(info.FileSize)/(1024*1024))
	}

	// 决策逻辑 2: Activity 数量判断（> 20）
	if info.ActivityCount > 20 {
		return true, fmt.Sprintf("activity_count_high (%d activities)", info.ActivityCount)
	}

	// 决策逻辑 3: 权限判断（网络权限 + 敏感权限）
	hasInternet := false
	hasSensitive := false

	sensitivePermissions := []string{
		"android.permission.READ_CONTACTS",
		"android.permission.READ_SMS",
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.CAMERA",
		"android.permission.RECORD_AUDIO",
		"android.permission.READ_PHONE_STATE",
	}

	for _, perm := range info.Permissions {
		if perm == "android.permission.INTERNET" {
			hasInternet = true
		}
		for _, sensitivePerm := range sensitivePermissions {
			if perm == sensitivePerm {
				hasSensitive = true
				break
			}
		}
	}

	if hasInternet && hasSensitive {
		return true, "has_internet_and_sensitive_permissions"
	}

	// 决策逻辑 4: 包名判断（高优先级类别）
	highPriorityPrefixes := []string{
		"com.tencent.",
		"com.alibaba.",
		"com.baidu.",
		"com.jd.",
		"com.bank.",
		"com.wechat.",
		"com.taobao.",
	}

	for _, prefix := range highPriorityPrefixes {
		if strings.HasPrefix(info.PackageName, prefix) {
			return true, fmt.Sprintf("high_priority_package (prefix: %s)", prefix)
		}
	}

	// 默认：不需要深度分析
	return false, "meets_fast_analysis_criteria"
}

// containsString 辅助函数：检查字符串是否在列表中
func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
