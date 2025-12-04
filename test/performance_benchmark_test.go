package test

import (
	"context"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/filter"
	"github.com/apk-analysis/apk-analysis-go/internal/staticanalysis"
	"github.com/sirupsen/logrus"
)

// ============================================
// FastAnalyzer Benchmarks
// ============================================

// BenchmarkFastAnalyzer_AnalyzeFast 测试快速分析的性能（跳过实际执行）
func BenchmarkFastAnalyzer_AnalyzeFast(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	analyzer := staticanalysis.NewFastAnalyzer(logger)

	// 模拟 APK 文件路径（测试overhead，实际文件不存在会快速失败）
	apkPath := "/tmp/nonexistent.apk"
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Note: This will fail as file doesn't exist, but we're measuring the overhead
		analyzer.AnalyzeFast(ctx, apkPath)
	}
}

// BenchmarkFastAnalyzer_NeedsDeepAnalysis 测试深度分析决策性能
func BenchmarkFastAnalyzer_NeedsDeepAnalysis(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	analyzer := staticanalysis.NewFastAnalyzer(logger)

	basicInfo := &staticanalysis.BasicInfo{
		PackageName:   "com.example.benchmark",
		FileSize:      5 * 1024 * 1024, // 5MB
		ActivityCount: 15,
		ServiceCount:  5,
		ProviderCount: 2,
		ReceiverCount: 3,
		Permissions:   make([]string, 20),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.NeedsDeepAnalysis(basicInfo)
	}
}

// ============================================
// ActivityFilter Benchmarks
// ============================================

// BenchmarkActivityFilter_Filter 测试 Activity 过滤性能
func BenchmarkActivityFilter_Filter(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	packageName := "com.example.app"
	activityFilter := filter.NewActivityFilter(packageName, logger)

	// 模拟 50 个 Activity
	activities := make([]string, 50)
	for i := 0; i < 50; i++ {
		if i%5 == 0 {
			// 20% 系统 Activity
			activities[i] = "com.android.internal.app.MainActivity"
		} else if i%7 == 0 {
			// ~14% 第三方 SDK Activity
			activities[i] = "com.google.android.gms.ads.AdActivity"
		} else {
			// 其余为应用自身 Activity
			activities[i] = "com.example.app.Activity" + string(rune('A'+i%26))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		activityFilter.Filter(activities)
	}
}

// BenchmarkActivityFilter_FilterWithCoreDetection 测试包含核心Activity检测的过滤性能
func BenchmarkActivityFilter_FilterWithCoreDetection(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	packageName := "com.example.app"
	activityFilter := filter.NewActivityFilter(packageName, logger)

	activities := []string{
		"com.example.app.MainActivity",
		"com.example.app.LoginActivity",
		"com.example.app.SettingsActivity",
		"com.example.app.ProfileActivity",
		"com.example.app.DetailActivity",
		"com.example.app.ui.HomeActivity",
		"com.example.app.feature.WelcomeActivity",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := activityFilter.Filter(activities)
		_ = len(result.CoreActivities) // Use the result
	}
}

// ============================================
// Domain Model Benchmarks
// ============================================

// BenchmarkTask_JSONSerialization 测试 Task 模型序列化性能
func BenchmarkTask_JSONSerialization(b *testing.B) {
	now := time.Now()
	task := &domain.Task{
		ID:              "benchmark-task-001",
		APKName:         "benchmark.apk",
		PackageName:     "com.example.benchmark",
		Status:          domain.TaskStatusCompleted,
		CurrentStep:     "任务完成",
		ProgressPercent: 100,
		CreatedAt:       now,
		StartedAt:       &now,
		CompletedAt:     &now,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate JSON marshaling (using string conversion as proxy)
		_ = task.ID + task.APKName + task.PackageName
	}
}

// BenchmarkTaskActivity_Creation 测试 TaskActivity 创建性能
func BenchmarkTaskActivity_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &domain.TaskActivity{
			TaskID:              "benchmark-task-001",
			LauncherActivity:    "com.example.app.MainActivity",
			ActivitiesJSON:      "com.example.app.MainActivity,com.example.app.LoginActivity",
			ActivityDetailsJSON: `[{"name":"MainActivity"}]`,
			CreatedAt:           time.Now(),
		}
	}
}

// ============================================
// Concurrent Operations Benchmarks
// ============================================

// BenchmarkConcurrent_Filter 测试并发 Activity 过滤性能
func BenchmarkConcurrent_Filter(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	packageName := "com.example.app"
	activityFilter := filter.NewActivityFilter(packageName, logger)

	activities := make([]string, 100)
	for i := 0; i < 100; i++ {
		activities[i] = "com.example.app.Activity" + string(rune('A'+i%26))
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			activityFilter.Filter(activities)
		}
	})
}

// BenchmarkConcurrent_NeedsDeepAnalysis 测试并发深度分析决策性能
func BenchmarkConcurrent_NeedsDeepAnalysis(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	analyzer := staticanalysis.NewFastAnalyzer(logger)

	basicInfo := &staticanalysis.BasicInfo{
		PackageName:   "com.example.benchmark",
		FileSize:      8 * 1024 * 1024,
		ActivityCount: 18,
		ServiceCount:  8,
		Permissions:   make([]string, 25),
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			analyzer.NeedsDeepAnalysis(basicInfo)
		}
	})
}

// ============================================
// Memory Allocation Benchmarks
// ============================================

// BenchmarkMemory_LargeActivityList 测试大量 TaskActivity 列表的内存分配
func BenchmarkMemory_LargeActivityList(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		activities := make([]*domain.TaskActivity, 100)
		for j := 0; j < 100; j++ {
			activities[j] = &domain.TaskActivity{
				TaskID:           "task-" + string(rune('A'+j%26)),
				LauncherActivity: "com.example.app.Activity" + string(rune('A'+j%26)),
				ActivitiesJSON:   "com.example.app.Activity1,Activity2",
				CreatedAt:        time.Now(),
			}
		}
	}
}

// BenchmarkMemory_TaskStatusUpdates 测试频繁状态更新的内存分配
func BenchmarkMemory_TaskStatusUpdates(b *testing.B) {
	b.ReportAllocs()

	task := &domain.Task{
		ID:              "benchmark-task-002",
		APKName:         "benchmark.apk",
		ProgressPercent: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate 100 progress updates
		for progress := 0; progress <= 100; progress++ {
			task.ProgressPercent = progress
			task.CurrentStep = "处理中..."
		}
	}
}

// ============================================
// String Operations Benchmarks
// ============================================

// BenchmarkString_ActivityNameShortening 测试 Activity 名称缩短性能
func BenchmarkString_ActivityNameShortening(b *testing.B) {
	fullNames := []string{
		"com.example.app.MainActivity",
		"com.example.app.ui.LoginActivity",
		"com.example.app.feature.profile.ProfileActivity",
		"com.example.app.module.settings.SettingsActivity",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range fullNames {
			// Simulate shortening by finding last dot
			lastDot := -1
			for j := len(name) - 1; j >= 0; j-- {
				if name[j] == '.' {
					lastDot = j
					break
				}
			}
			if lastDot >= 0 {
				_ = name[lastDot+1:]
			}
		}
	}
}

// BenchmarkString_PackageNameMatching 测试包名匹配性能
func BenchmarkString_PackageNameMatching(b *testing.B) {
	activities := make([]string, 50)
	for i := 0; i < 50; i++ {
		if i%3 == 0 {
			activities[i] = "com.example.app.Activity" + string(rune('A'+i%26))
		} else {
			activities[i] = "com.thirdparty.sdk.Activity" + string(rune('A'+i%26))
		}
	}

	packageName := "com.example.app"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for _, activity := range activities {
			// Simulate package name prefix matching
			if len(activity) >= len(packageName) {
				match := true
				for j := 0; j < len(packageName); j++ {
					if activity[j] != packageName[j] {
						match = false
						break
					}
				}
				if match {
					count++
				}
			}
		}
	}
}

// ============================================
// Time Operations Benchmarks
// ============================================

// BenchmarkTime_DurationCalculation 测试时长计算性能
func BenchmarkTime_DurationCalculation(b *testing.B) {
	startTime := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		endTime := time.Now()
		_ = endTime.Sub(startTime).Milliseconds()
	}
}

// BenchmarkTime_TimestampFormatting 测试时间戳格式化性能
func BenchmarkTime_TimestampFormatting(b *testing.B) {
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = now.Format("2006/01/02 15:04:05")
	}
}
