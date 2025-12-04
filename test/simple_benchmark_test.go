package test

import (
	"strings"
	"testing"
	"time"
)

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
		"com.example.app.feature.home.HomeActivity",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range fullNames {
			// Find last dot
			lastDot := strings.LastIndex(name, ".")
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
			if strings.HasPrefix(activity, packageName) {
				count++
			}
		}
		_ = count
	}
}

// BenchmarkString_ContainsCheck 测试字符串包含检查性能
func BenchmarkString_ContainsCheck(b *testing.B) {
	activity := "com.example.app.ui.MainActivity"
	sdkKeywords := []string{".umeng.", ".jpush.", ".sensorsdata.", ".talkingdata."}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		found := false
		for _, keyword := range sdkKeywords {
			if strings.Contains(activity, keyword) {
				found = true
				break
			}
		}
		_ = found
	}
}

// BenchmarkString_Split 测试字符串分割性能
func BenchmarkString_Split(b *testing.B) {
	packageName := "com.example.app.android.feature"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parts := strings.Split(packageName, ".")
		_ = len(parts)
	}
}

// ============================================
// Time Operations Benchmarks
// ============================================

// BenchmarkTime_Now 测试获取当前时间性能
func BenchmarkTime_Now(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = time.Now()
	}
}

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

// BenchmarkTime_UnixTimestamp 测试 Unix 时间戳转换性能
func BenchmarkTime_UnixTimestamp(b *testing.B) {
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = now.Unix()
	}
}

// ============================================
// Memory Allocation Benchmarks
// ============================================

// BenchmarkMemory_StringSliceAllocation 测试字符串切片分配性能
func BenchmarkMemory_StringSliceAllocation(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		activities := make([]string, 0, 100)
		for j := 0; j < 100; j++ {
			activities = append(activities, "com.example.app.Activity"+string(rune('A'+j%26)))
		}
	}
}

// BenchmarkMemory_MapAllocation 测试 Map 分配性能
func BenchmarkMemory_MapAllocation(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filterReasons := make(map[string]string, 50)
		for j := 0; j < 50; j++ {
			key := "activity" + string(rune('A'+j%26))
			filterReasons[key] = "filtered_reason"
		}
	}
}

// BenchmarkMemory_StructAllocation 测试结构体分配性能
func BenchmarkMemory_StructAllocation(b *testing.B) {
	b.ReportAllocs()

	type TaskInfo struct {
		ID         string
		Name       string
		Status     string
		Progress   int
		CreatedAt  time.Time
		StartedAt  *time.Time
	}

	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &TaskInfo{
			ID:        "task-001",
			Name:      "test.apk",
			Status:    "running",
			Progress:  50,
			CreatedAt: now,
			StartedAt: &now,
		}
	}
}

// ============================================
// Algorithm Performance Benchmarks
// ============================================

// BenchmarkAlgorithm_LinearSearch 测试线性搜索性能
func BenchmarkAlgorithm_LinearSearch(b *testing.B) {
	items := make([]string, 100)
	for i := 0; i < 100; i++ {
		items[i] = "item" + string(rune('A'+i%26))
	}
	target := "itemZ"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		found := false
		for _, item := range items {
			if item == target {
				found = true
				break
			}
		}
		_ = found
	}
}

// BenchmarkAlgorithm_MapLookup 测试 Map 查找性能
func BenchmarkAlgorithm_MapLookup(b *testing.B) {
	items := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		items["item"+string(rune('A'+i%26))] = true
	}
	target := "itemZ"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, found := items[target]
		_ = found
	}
}

// BenchmarkAlgorithm_FilterLoop 测试过滤循环性能
func BenchmarkAlgorithm_FilterLoop(b *testing.B) {
	activities := make([]string, 100)
	for i := 0; i < 100; i++ {
		if i%3 == 0 {
			activities[i] = "com.android.internal.Activity" + string(rune('A'+i%26))
		} else {
			activities[i] = "com.example.app.Activity" + string(rune('A'+i%26))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filtered := make([]string, 0, 100)
		for _, activity := range activities {
			if !strings.HasPrefix(activity, "com.android.") {
				filtered = append(filtered, activity)
			}
		}
		_ = len(filtered)
	}
}

// ============================================
// Concurrent Operations Benchmarks
// ============================================

// BenchmarkConcurrent_StringOperations 测试并发字符串操作性能
func BenchmarkConcurrent_StringOperations(b *testing.B) {
	activities := make([]string, 50)
	for i := 0; i < 50; i++ {
		activities[i] = "com.example.app.Activity" + string(rune('A'+i%26))
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for _, activity := range activities {
				lastDot := strings.LastIndex(activity, ".")
				if lastDot >= 0 {
					_ = activity[lastDot+1:]
				}
			}
		}
	})
}

// BenchmarkConcurrent_MapAccess 测试并发 Map 访问性能
func BenchmarkConcurrent_MapAccess(b *testing.B) {
	cache := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		cache["key"+string(rune('A'+i%26))] = true
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, exists := cache["keyA"]
			_ = exists
		}
	})
}
