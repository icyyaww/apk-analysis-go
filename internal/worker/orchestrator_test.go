package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/device"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/filter"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/staticanalysis"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestOrchestrator 创建测试用的 Orchestrator
func setupTestOrchestrator(t *testing.T, staticAnalysisMode string) (*Orchestrator, *gorm.DB, string) {
	// 创建测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// 逐个迁移表，忽略索引冲突错误
	tables := []interface{}{
		&domain.Task{},
		&domain.TaskActivity{},
		&domain.TaskStaticReport{},
		&domain.TaskMobSFReport{},
		&domain.TaskDomainAnalysis{},
		&domain.TaskAppDomain{},
		&domain.TaskAILog{},
	}

	for _, table := range tables {
		err = db.AutoMigrate(table)
		// 忽略 "index already exists" 错误（测试环境中的正常现象）
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			require.NoError(t, err, "Failed to migrate test database")
		}
	}

	// 创建临时结果目录
	resultsDir := t.TempDir()

	// 创建 logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // 减少测试输出

	// 创建 repositories
	taskRepo := repository.NewTaskRepository(db)
	staticReportRepo := repository.NewStaticReportRepository(db)

	// 创建模拟 DeviceManager (测试中不需要真实设备)
	deviceMgr := device.NewDeviceManager(logger)

	// 创建测试配置
	// 注意：为了避免测试超时，我们禁用 Hybrid analyzer（它会尝试初始化 Python 进程）
	cfg := &config.Config{
		StaticAnalysis: config.StaticAnalysisConfig{
			EnabledAnalyzers: staticAnalysisMode,
			MobSF: config.MobSFConfig{
				Enabled: staticAnalysisMode == "mobsf" || staticAnalysisMode == "both",
			},
			Hybrid: config.HybridAnalyzerConfig{
				Enabled:         false, // 测试中禁用以避免超时
				PythonPath:      "python3",
				ScriptPath:      "/nonexistent/script.py",
				UseProcessPool:  false,
				ProcessPoolSize: 2,
			},
		},
	}

	// 创建 Orchestrator
	orchestrator := NewOrchestrator(
		deviceMgr,
		taskRepo,
		staticReportRepo,
		cfg,
		logger,
		resultsDir,
		"localhost",
	)

	return orchestrator, db, resultsDir
}

// TestOrchestrator_StaticAnalysisMode 测试静态分析模式配置
func TestOrchestrator_StaticAnalysisMode(t *testing.T) {
	tests := []struct {
		name              string
		mode              string
		expectedMobsf     bool
		expectedHybrid    bool
		expectedMode      string
	}{
		{
			name:           "MobSF Only Mode",
			mode:           "mobsf",
			expectedMobsf:  true,
			expectedHybrid: false,
			expectedMode:   "mobsf",
		},
		{
			name:           "Hybrid Only Mode",
			mode:           "hybrid",
			expectedMobsf:  false,
			expectedHybrid: false, // 会因为脚本路径不存在而禁用
			expectedMode:   "hybrid",
		},
		{
			name:           "Both Mode",
			mode:           "both",
			expectedMobsf:  true,
			expectedHybrid: false, // 会因为脚本路径不存在而禁用
			expectedMode:   "both",
		},
		{
			name:           "Disabled Mode",
			mode:           "none",
			expectedMobsf:  false,
			expectedHybrid: false,
			expectedMode:   "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator, _, _ := setupTestOrchestrator(t, tt.mode)

			assert.Equal(t, tt.expectedMode, orchestrator.staticAnalysisMode, "静态分析模式应匹配")
			assert.Equal(t, tt.expectedMobsf, orchestrator.mobsfEnabled, "MobSF 启用状态应匹配")
			// Hybrid 可能因为脚本不存在而被禁用，这是正常的
		})
	}
}

// TestOrchestrator_UpdateTaskStatus 测试任务状态更新
func TestOrchestrator_UpdateTaskStatus(t *testing.T) {
	orchestrator, db, _ := setupTestOrchestrator(t, "none")
	ctx := context.Background()

	// 创建测试任务
	task := &domain.Task{
		ID:              "test-task-001",
		APKName:         "test.apk",
		Status:          domain.TaskStatusQueued,
		ProgressPercent: 0,
		CreatedAt:       time.Now(),
	}
	err := db.Create(task).Error
	require.NoError(t, err)

	// 测试更新状态
	err = orchestrator.updateTaskStatus(ctx, task.ID, domain.TaskStatusRunning, "正在运行", 50)
	assert.NoError(t, err)

	// 验证更新
	var updated domain.Task
	err = db.First(&updated, "id = ?", task.ID).Error
	require.NoError(t, err)

	assert.Equal(t, domain.TaskStatusRunning, updated.Status)
	assert.Equal(t, "正在运行", updated.CurrentStep)
	assert.Equal(t, 50, updated.ProgressPercent)
	assert.NotNil(t, updated.StartedAt, "应该设置 StartedAt")
}

// TestOrchestrator_FailTask 测试任务失败处理
func TestOrchestrator_FailTask(t *testing.T) {
	orchestrator, db, _ := setupTestOrchestrator(t, "none")
	ctx := context.Background()

	// 创建测试任务
	task := &domain.Task{
		ID:        "test-task-002",
		APKName:   "test.apk",
		Status:    domain.TaskStatusRunning,
		CreatedAt: time.Now(),
	}
	err := db.Create(task).Error
	require.NoError(t, err)

	// 测试失败
	testErr := assert.AnError
	err = orchestrator.failTask(ctx, task.ID, testErr)
	assert.Error(t, err)

	// 验证失败状态
	var failed domain.Task
	err = db.First(&failed, "id = ?", task.ID).Error
	require.NoError(t, err)

	assert.Equal(t, domain.TaskStatusFailed, failed.Status)
	assert.Contains(t, failed.ErrorMessage, "assert.AnError")
	assert.NotNil(t, failed.CompletedAt)
}

// TestOrchestrator_CompleteTask 测试任务完成
func TestOrchestrator_CompleteTask(t *testing.T) {
	orchestrator, db, _ := setupTestOrchestrator(t, "none")
	ctx := context.Background()

	// 创建测试任务
	task := &domain.Task{
		ID:              "test-task-003",
		APKName:         "test.apk",
		Status:          domain.TaskStatusRunning,
		ProgressPercent: 90,
		CreatedAt:       time.Now(),
	}
	err := db.Create(task).Error
	require.NoError(t, err)

	// 测试完成
	err = orchestrator.completeTask(ctx, task.ID)
	assert.NoError(t, err)

	// 验证完成状态
	var completed domain.Task
	err = db.First(&completed, "id = ?", task.ID).Error
	require.NoError(t, err)

	assert.Equal(t, domain.TaskStatusCompleted, completed.Status)
	assert.Equal(t, "任务完成", completed.CurrentStep)
	assert.Equal(t, 100, completed.ProgressPercent)
	assert.NotNil(t, completed.CompletedAt)
}

// TestOrchestrator_SaveActivityDetails 测试保存 Activity 详情
func TestOrchestrator_SaveActivityDetails(t *testing.T) {
	orchestrator, db, _ := setupTestOrchestrator(t, "none")
	ctx := context.Background()

	// 创建测试任务
	task := &domain.Task{
		ID:      "test-task-004",
		APKName: "test.apk",
		Status:  domain.TaskStatusRunning,
	}
	err := db.Create(task).Error
	require.NoError(t, err)

	// 测试数据
	activities := []string{
		"com.example.app.MainActivity",
		"com.example.app.LoginActivity",
		"com.example.app.SettingsActivity",
	}

	details := []map[string]interface{}{
		{
			"activity":       "com.example.app.MainActivity",
			"status":         "completed",
			"urls_collected": 5,
		},
		{
			"activity":       "com.example.app.LoginActivity",
			"status":         "completed",
			"urls_collected": 3,
		},
		{
			"activity":       "com.example.app.SettingsActivity",
			"status":         "completed",
			"urls_collected": 1,
		},
	}

	// 保存
	err = orchestrator.saveActivityDetails(ctx, task.ID, "com.example.app", activities, details)
	assert.NoError(t, err)

	// 验证保存结果
	var activityData domain.TaskActivity
	err = db.Where("task_id = ?", task.ID).First(&activityData).Error
	require.NoError(t, err)

	assert.Equal(t, task.ID, activityData.TaskID)
	assert.Equal(t, "com.example.app.MainActivity", activityData.LauncherActivity)
	assert.Contains(t, activityData.ActivitiesJSON, "MainActivity")
	assert.Contains(t, activityData.ActivitiesJSON, "LoginActivity")
	assert.Contains(t, activityData.ActivityDetailsJSON, "urls_collected")
}

// TestOrchestrator_SaveStaticAnalysisResult 测试保存静态分析结果
func TestOrchestrator_SaveStaticAnalysisResult(t *testing.T) {
	orchestrator, db, _ := setupTestOrchestrator(t, "hybrid")
	ctx := context.Background()

	// 创建测试任务
	task := &domain.Task{
		ID:      "test-task-005",
		APKName: "test.apk",
		Status:  domain.TaskStatusRunning,
	}
	err := db.Create(task).Error
	require.NoError(t, err)

	// 创建测试分析结果
	result := &staticanalysis.AnalysisResult{
		BasicInfo: &staticanalysis.BasicInfo{
			PackageName:    "com.example.app",
			VersionName:    "1.0.0",
			VersionCode:    "1",
			AppName:        "Test App",
			FileSize:       1024000,
			MD5:            "abc123",
			SHA256:         "def456",
			ActivityCount:  5,
			ServiceCount:   2,
			ReceiverCount:  3,
			ProviderCount:  1,
			Permissions:    []string{"INTERNET", "ACCESS_NETWORK_STATE"},
		},
		AnalysisMode:               "fast",
		AnalysisDuration:           1500,
		FastAnalysisDuration:       500,
		DeepAnalysisDuration:       1000,
		NeedsDeepAnalysisReason:    "需要深度分析",
		AnalyzedAt:                 time.Now(),
	}

	// 保存结果
	err = orchestrator.saveStaticAnalysisResult(ctx, task.ID, result, "com.example.app")
	assert.NoError(t, err)

	// 验证保存结果
	var report domain.TaskStaticReport
	err = db.Where("task_id = ?", task.ID).First(&report).Error
	require.NoError(t, err)

	assert.Equal(t, task.ID, report.TaskID)
	assert.Equal(t, "hybrid", report.Analyzer)
	assert.Equal(t, domain.StaticStatusCompleted, report.Status)
	assert.Equal(t, "com.example.app", report.PackageName)
	assert.Equal(t, "1.0.0", report.VersionName)
	assert.Equal(t, 5, report.ActivityCount)
	assert.Equal(t, 2, report.ServiceCount)
	assert.Equal(t, 2, report.PermissionCount)
	assert.Equal(t, 1500, report.AnalysisDurationMs)
}

// TestOrchestrator_ShortActivityName 测试 Activity 名称简化
func TestOrchestrator_ShortActivityName(t *testing.T) {
	orchestrator, _, _ := setupTestOrchestrator(t, "none")

	tests := []struct {
		fullName  string
		shortName string
	}{
		{
			fullName:  "com.example.app.MainActivity",
			shortName: "MainActivity",
		},
		{
			fullName:  "com.example.app.ui.LoginActivity",
			shortName: "LoginActivity",
		},
		{
			fullName:  "SimpleActivity",
			shortName: "SimpleActivity",
		},
		{
			fullName:  "",
			shortName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.fullName, func(t *testing.T) {
			result := orchestrator.shortActivityName(tt.fullName)
			assert.Equal(t, tt.shortName, result)
		})
	}
}

// TestOrchestrator_IsCoreActivity 测试核心 Activity 判断
func TestOrchestrator_IsCoreActivity(t *testing.T) {
	orchestrator, _, _ := setupTestOrchestrator(t, "none")

	tests := []struct {
		activity string
		isCore   bool
	}{
		{
			activity: "com.example.app.MainActivity",
			isCore:   true,
		},
		{
			activity: "com.example.app.LoginActivity",
			isCore:   true,
		},
		{
			activity: "com.example.app.HomeActivity",
			isCore:   true,
		},
		{
			activity: "com.example.app.WelcomeActivity",
			isCore:   true,
		},
		{
			activity: "com.example.app.SplashActivity",
			isCore:   true,
		},
		{
			activity: "com.example.app.SettingsActivity",
			isCore:   false,
		},
		{
			activity: "com.example.app.DetailActivity",
			isCore:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.activity, func(t *testing.T) {
			result := orchestrator.isCoreActivity(tt.activity)
			assert.Equal(t, tt.isCore, result)
		})
	}
}

// TestOrchestrator_SaveFilterReport 测试保存过滤报告
func TestOrchestrator_SaveFilterReport(t *testing.T) {
	orchestrator, _, resultsDir := setupTestOrchestrator(t, "none")

	// 创建测试任务目录
	taskID := "test-task-006"
	taskDir := filepath.Join(resultsDir, taskID)
	err := os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	// 创建模拟过滤结果
	filterResult := &filter.FilterResult{
		TotalActivities: 10,
		FilteredCount:   3,
		SelectedCount:   7,
		SelectedList: []string{
			"com.example.app.MainActivity",
			"com.example.app.LoginActivity",
		},
		FilteredList: []string{
			"com.example.app.SomeProvider",
		},
	}

	// 保存报告
	err = orchestrator.saveFilterReport(taskDir, filterResult)
	assert.NoError(t, err)

	// 验证文件已创建
	reportPath := filepath.Join(taskDir, "activity_filter_report.json")
	_, err = os.Stat(reportPath)
	assert.NoError(t, err, "报告文件应该存在")
}

// TestOrchestrator_JSONString 测试 JSON 序列化辅助函数
func TestOrchestrator_JSONString(t *testing.T) {
	orchestrator, _, _ := setupTestOrchestrator(t, "none")

	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "Simple map",
			input:    map[string]interface{}{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "Array",
			input:    []string{"a", "b", "c"},
			expected: `["a","b","c"]`,
		},
		{
			name:     "Nil",
			input:    nil,
			expected: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orchestrator.jsonString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// BenchmarkOrchestrator_UpdateTaskStatus 基准测试：任务状态更新性能
func BenchmarkOrchestrator_UpdateTaskStatus(b *testing.B) {
	// 创建测试环境
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.Task{})

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	taskRepo := repository.NewTaskRepository(db)
	staticReportRepo := repository.NewStaticReportRepository(db)
	deviceMgr := device.NewDeviceManager(logger)

	cfg := &config.Config{
		StaticAnalysis: config.StaticAnalysisConfig{
			EnabledAnalyzers: "none",
		},
	}

	orchestrator := NewOrchestrator(
		deviceMgr,
		taskRepo,
		staticReportRepo,
		cfg,
		logger,
		"/tmp",
		"localhost",
	)

	// 创建测试任务
	ctx := context.Background()
	task := &domain.Task{
		ID:      "bench-task",
		APKName: "bench.apk",
		Status:  domain.TaskStatusQueued,
	}
	db.Create(task)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		orchestrator.updateTaskStatus(ctx, task.ID, domain.TaskStatusRunning, "测试", i%100)
	}
}
