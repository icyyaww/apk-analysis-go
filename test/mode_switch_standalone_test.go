package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 模拟深度分析决策逻辑
func needsDeepAnalysis(fileSize int64, activityCount, serviceCount, permissionCount int) (bool, string) {
	// 阈值配置（从实际代码中提取）
	const (
		maxFileSizeMB     = 10  // 10MB
		maxActivityCount  = 20
		maxServiceCount   = 10
		maxPermissionCount = 30
	)

	fileSizeMB := fileSize / (1024 * 1024)

	// 检查文件大小
	if fileSizeMB > maxFileSizeMB {
		return true, "APK 文件较大"
	}

	// 检查 Activity 数量
	if activityCount > maxActivityCount {
		return true, "Activity 数量较多"
	}

	// 检查 Service 数量
	if serviceCount > maxServiceCount {
		return true, "Service 数量较多"
	}

	// 检查权限数量
	if permissionCount > maxPermissionCount {
		return true, "权限数量较多"
	}

	return false, "应用较小，使用快速模式"
}

// TestModeSwitch_SmallApp 测试小应用使用快速模式
func TestModeSwitch_SmallApp(t *testing.T) {
	fileSize := int64(500 * 1024) // 500KB
	activityCount := 3
	serviceCount := 1
	permissionCount := 5

	needDeep, reason := needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)

	assert.False(t, needDeep, "小应用应使用快速模式")
	assert.Equal(t, "应用较小，使用快速模式", reason)
}

// TestModeSwitch_LargeFile 测试大文件触发深度模式
func TestModeSwitch_LargeFile(t *testing.T) {
	fileSize := int64(50 * 1024 * 1024) // 50MB
	activityCount := 5
	serviceCount := 2
	permissionCount := 10

	needDeep, reason := needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)

	assert.True(t, needDeep, "大文件应触发深度模式")
	assert.Contains(t, reason, "APK 文件较大")
}

// TestModeSwitch_ManyActivities 测试过多Activity触发深度模式
func TestModeSwitch_ManyActivities(t *testing.T) {
	fileSize := int64(5 * 1024 * 1024) // 5MB
	activityCount := 25                // 超过阈值20
	serviceCount := 3
	permissionCount := 15

	needDeep, reason := needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)

	assert.True(t, needDeep, "过多Activity应触发深度模式")
	assert.Contains(t, reason, "Activity 数量较多")
}

// TestModeSwitch_ManyServices 测试过多Service触发深度模式
func TestModeSwitch_ManyServices(t *testing.T) {
	fileSize := int64(4 * 1024 * 1024) // 4MB
	activityCount := 8
	serviceCount := 12 // 超过阈值10
	permissionCount := 20

	needDeep, reason := needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)

	assert.True(t, needDeep, "过多Service应触发深度模式")
	assert.Contains(t, reason, "Service 数量较多")
}

// TestModeSwitch_ManyPermissions 测试过多权限触发深度模式
func TestModeSwitch_ManyPermissions(t *testing.T) {
	fileSize := int64(3 * 1024 * 1024) // 3MB
	activityCount := 10
	serviceCount := 5
	permissionCount := 35 // 超过阈值30

	needDeep, reason := needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)

	assert.True(t, needDeep, "过多权限应触发深度模式")
	assert.Contains(t, reason, "权限数量较多")
}

// TestModeSwitch_BoundaryValues 测试边界值
func TestModeSwitch_BoundaryValues(t *testing.T) {
	tests := []struct {
		name            string
		fileSize        int64
		activityCount   int
		serviceCount    int
		permissionCount int
		expectedDeep    bool
	}{
		{
			name:            "Exactly at file size threshold",
			fileSize:        10 * 1024 * 1024, // 正好10MB
			activityCount:   10,
			serviceCount:    5,
			permissionCount: 15,
			expectedDeep:    false, // 不超过阈值
		},
		{
			name:            "Just over file size threshold",
			fileSize:        11 * 1024 * 1024, // 11MB
			activityCount:   10,
			serviceCount:    5,
			permissionCount: 15,
			expectedDeep:    true, // 超过阈值
		},
		{
			name:            "Exactly at activity threshold",
			fileSize:        5 * 1024 * 1024,
			activityCount:   20, // 正好20
			serviceCount:    5,
			permissionCount: 15,
			expectedDeep:    false, // 不超过阈值
		},
		{
			name:            "Just over activity threshold",
			fileSize:        5 * 1024 * 1024,
			activityCount:   21, // 21
			serviceCount:    5,
			permissionCount: 15,
			expectedDeep:    true, // 超过阈值
		},
		{
			name:            "All thresholds maxed but not exceeded",
			fileSize:        10 * 1024 * 1024,
			activityCount:   20,
			serviceCount:    10,
			permissionCount: 30,
			expectedDeep:    false, // 都不超过
		},
		{
			name:            "One threshold exceeded",
			fileSize:        10 * 1024 * 1024,
			activityCount:   20,
			serviceCount:    11, // 超过
			permissionCount: 30,
			expectedDeep:    true, // 有一个超过就触发
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needDeep, _ := needsDeepAnalysis(tt.fileSize, tt.activityCount, tt.serviceCount, tt.permissionCount)
			assert.Equal(t, tt.expectedDeep, needDeep, "边界值判断应该正确")
		})
	}
}

// TestModeSwitch_Priority 测试多个条件同时满足时的优先级
func TestModeSwitch_Priority(t *testing.T) {
	// 当多个条件都满足时，应该返回最先匹配的原因
	fileSize := int64(50 * 1024 * 1024) // 超过文件大小阈值（第一个检查）
	activityCount := 30                 // 也超过Activity阈值
	serviceCount := 15                  // 也超过Service阈值
	permissionCount := 40               // 也超过权限阈值

	needDeep, reason := needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)

	assert.True(t, needDeep, "应触发深度模式")
	assert.Contains(t, reason, "APK 文件较大", "应返回第一个匹配的原因")
}

// BenchmarkModeDecision 基准测试：模式决策性能
func BenchmarkModeDecision(b *testing.B) {
	fileSize := int64(5 * 1024 * 1024)
	activityCount := 15
	serviceCount := 5
	permissionCount := 20

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		needsDeepAnalysis(fileSize, activityCount, serviceCount, permissionCount)
	}
}
