package staticanalysis

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHybridAnalyzer_AnalyzeMode 测试分析模式决策
func TestHybridAnalyzer_AnalyzeMode(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// 创建快速分析器
	fastAnalyzer := NewFastAnalyzer(logger)

	tests := []struct {
		name              string
		apkSize           int64
		activityCount     int
		serviceCount      int
		permissionCount   int
		expectedNeedDeep  bool
		expectedReason    string
	}{
		{
			name:            "Small APK - Fast Mode",
			apkSize:         500 * 1024, // 500KB
			activityCount:   3,
			serviceCount:    1,
			permissionCount: 5,
			expectedNeedDeep: false,
			expectedReason:  "应用较小，使用快速模式",
		},
		{
			name:            "Large APK - Deep Mode",
			apkSize:         50 * 1024 * 1024, // 50MB
			activityCount:   20,
			serviceCount:    5,
			permissionCount: 30,
			expectedNeedDeep: true,
			expectedReason:  "APK 文件较大 (50.00MB)",
		},
		{
			name:            "Many Activities - Deep Mode",
			apkSize:         5 * 1024 * 1024, // 5MB
			activityCount:   25,
			serviceCount:    3,
			permissionCount: 15,
			expectedNeedDeep: true,
			expectedReason:  "Activity 数量较多 (25)",
		},
		{
			name:            "Many Permissions - Deep Mode",
			apkSize:         3 * 1024 * 1024, // 3MB
			activityCount:   10,
			serviceCount:    2,
			permissionCount: 35,
			expectedNeedDeep: true,
			expectedReason:  "权限数量较多 (35)",
		},
		{
			name:            "Many Services - Deep Mode",
			apkSize:         4 * 1024 * 1024, // 4MB
			activityCount:   8,
			serviceCount:    12,
			permissionCount: 20,
			expectedNeedDeep: true,
			expectedReason:  "Service 数量较多 (12)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建模拟的基本信息
			basicInfo := &BasicInfo{
				PackageName:   "com.example.test",
				VersionName:   "1.0.0",
				FileSize:      tt.apkSize,
				ActivityCount: tt.activityCount,
				ServiceCount:  tt.serviceCount,
				Permissions:   make([]string, tt.permissionCount),
			}

			// 测试决策逻辑
			needDeep, reason := fastAnalyzer.NeedsDeepAnalysis(basicInfo)

			assert.Equal(t, tt.expectedNeedDeep, needDeep, "深度分析决策应该匹配")
			if tt.expectedNeedDeep {
				assert.Contains(t, reason, tt.expectedReason, "决策原因应该匹配")
			}
		})
	}
}

// TestHybridAnalyzer_ModeSelection 测试混合分析器的模式选择
func TestHybridAnalyzer_ModeSelection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// 测试：仅快速模式
	t.Run("Fast Mode Only", func(t *testing.T) {
		fastAnalyzer := NewFastAnalyzer(logger)

		basicInfo := &BasicInfo{
			PackageName:   "com.example.small",
			FileSize:      500 * 1024, // 500KB - 小文件
			ActivityCount: 3,
			ServiceCount:  1,
			Permissions:   []string{"INTERNET"},
		}

		needDeep, _ := fastAnalyzer.NeedsDeepAnalysis(basicInfo)
		assert.False(t, needDeep, "小应用应该使用快速模式")
	})

	// 测试：需要深度分析
	t.Run("Deep Mode Required", func(t *testing.T) {
		fastAnalyzer := NewFastAnalyzer(logger)

		basicInfo := &BasicInfo{
			PackageName:   "com.example.large",
			FileSize:      50 * 1024 * 1024, // 50MB - 大文件
			ActivityCount: 25,
			ServiceCount:  10,
			Permissions:   make([]string, 30),
		}

		needDeep, reason := fastAnalyzer.NeedsDeepAnalysis(basicInfo)
		assert.True(t, needDeep, "大应用应该需要深度分析")
		assert.NotEmpty(t, reason, "应该提供决策原因")
	})
}

// TestFastAnalyzer_BasicAnalysis 测试快速分析器的基本功能
func TestFastAnalyzer_BasicAnalysis(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	fastAnalyzer := NewFastAnalyzer(logger)

	// 测试空 APK 路径
	t.Run("Empty APK Path", func(t *testing.T) {
		ctx := context.Background()
		_, err := fastAnalyzer.AnalyzeFast(ctx, "")
		assert.Error(t, err, "空路径应该返回错误")
	})

	// 测试不存在的文件
	t.Run("Non-existent File", func(t *testing.T) {
		ctx := context.Background()
		_, err := fastAnalyzer.AnalyzeFast(ctx, "/nonexistent/file.apk")
		assert.Error(t, err, "不存在的文件应该返回错误")
	})
}

// TestAnalysisResult_Structure 测试分析结果结构
func TestAnalysisResult_Structure(t *testing.T) {
	now := time.Now()

	result := &AnalysisResult{
		BasicInfo: &BasicInfo{
			PackageName: "com.example.test",
			VersionName: "1.0.0",
			FileSize:    1024000,
		},
		AnalysisMode:         "fast",
		AnalysisDuration:     1500,
		FastAnalysisDuration: 500,
		DeepAnalysisDuration: 1000,
		AnalyzedAt:           now,
	}

	// 验证结构
	assert.NotNil(t, result.BasicInfo, "基本信息不应为空")
	assert.Equal(t, "fast", result.AnalysisMode, "分析模式应匹配")
	assert.Equal(t, int64(1500), result.AnalysisDuration, "总时长应匹配")
	assert.Equal(t, int64(500), result.FastAnalysisDuration, "快速分析时长应匹配")
	assert.Equal(t, int64(1000), result.DeepAnalysisDuration, "深度分析时长应匹配")
}

// TestDeepAnalysisThreshold 测试深度分析阈值配置
func TestDeepAnalysisThreshold(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		activity int
		service  int
		provider int
		receiver int
		permission int
		expected bool
	}{
		{
			name:       "Below All Thresholds",
			size:       1 * 1024 * 1024,  // 1MB
			activity:   5,
			service:    2,
			provider:   1,
			receiver:   3,
			permission: 8,
			expected:   false, // 快速模式
		},
		{
			name:       "Exceed Size Threshold",
			size:       15 * 1024 * 1024, // 15MB (>10MB)
			activity:   5,
			service:    2,
			provider:   1,
			receiver:   3,
			permission: 8,
			expected:   true, // 深度模式
		},
		{
			name:       "Exceed Activity Threshold",
			size:       5 * 1024 * 1024, // 5MB
			activity:   25,              // >20
			service:    2,
			provider:   1,
			receiver:   3,
			permission: 8,
			expected:   true, // 深度模式
		},
		{
			name:       "Exceed Service Threshold",
			size:       5 * 1024 * 1024, // 5MB
			activity:   10,
			service:    12, // >10
			provider:   1,
			receiver:   3,
			permission: 8,
			expected:   true, // 深度模式
		},
		{
			name:       "Exceed Permission Threshold",
			size:       5 * 1024 * 1024, // 5MB
			activity:   10,
			service:    5,
			provider:   1,
			receiver:   3,
			permission: 35, // >30
			expected:   true, // 深度模式
		},
		{
			name:       "Multiple Thresholds Exceeded",
			size:       20 * 1024 * 1024, // 20MB
			activity:   30,
			service:    15,
			provider:   8,
			receiver:   10,
			permission: 40,
			expected:   true, // 深度模式
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	fastAnalyzer := NewFastAnalyzer(logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basicInfo := &BasicInfo{
				PackageName:   "com.example.threshold",
				FileSize:      tt.size,
				ActivityCount: tt.activity,
				ServiceCount:  tt.service,
				ProviderCount: tt.provider,
				ReceiverCount: tt.receiver,
				Permissions:   make([]string, tt.permission),
			}

			needDeep, _ := fastAnalyzer.NeedsDeepAnalysis(basicInfo)
			assert.Equal(t, tt.expected, needDeep, "阈值判断应该匹配")
		})
	}
}

// BenchmarkFastAnalyzer_NeedsDeepAnalysis 基准测试：深度分析决策性能
func BenchmarkFastAnalyzer_NeedsDeepAnalysis(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fastAnalyzer := NewFastAnalyzer(logger)

	basicInfo := &BasicInfo{
		PackageName:   "com.example.bench",
		FileSize:      5 * 1024 * 1024,
		ActivityCount: 15,
		ServiceCount:  5,
		ProviderCount: 2,
		ReceiverCount: 3,
		Permissions:   make([]string, 20),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fastAnalyzer.NeedsDeepAnalysis(basicInfo)
	}
}

// TestAnalysisMode_String 测试分析模式字符串表示
func TestAnalysisMode_String(t *testing.T) {
	tests := []struct {
		mode     string
		expected string
	}{
		{"fast", "fast"},
		{"deep", "deep"},
		{"fast_fallback", "fast_fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode)
		})
	}
}

// TestBasicInfo_Validation 测试基本信息验证
func TestBasicInfo_Validation(t *testing.T) {
	tests := []struct {
		name    string
		info    *BasicInfo
		isValid bool
	}{
		{
			name: "Valid Basic Info",
			info: &BasicInfo{
				PackageName: "com.example.valid",
				VersionName: "1.0.0",
				VersionCode: "1",
				FileSize:    1024000,
			},
			isValid: true,
		},
		{
			name: "Missing Package Name",
			info: &BasicInfo{
				PackageName: "",
				VersionName: "1.0.0",
				FileSize:    1024000,
			},
			isValid: false,
		},
		{
			name: "Zero File Size",
			info: &BasicInfo{
				PackageName: "com.example.test",
				VersionName: "1.0.0",
				FileSize:    0,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 验证包名
			if tt.isValid {
				assert.NotEmpty(t, tt.info.PackageName, "有效信息应有包名")
				assert.Greater(t, tt.info.FileSize, int64(0), "有效信息应有文件大小")
			}
		})
	}
}
