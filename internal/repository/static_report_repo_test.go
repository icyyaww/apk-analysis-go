package repository

import (
	"context"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupStaticReportTestDB 创建静态分析报告测试数据库
func setupStaticReportTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to open test database")

	// 自动迁移
	err = db.AutoMigrate(&domain.TaskStaticReport{})
	require.NoError(t, err, "Failed to migrate test database")

	return db
}

// TestStaticReportRepository_Create 测试创建静态分析报告
func TestStaticReportRepository_Create(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	now := time.Now()
	report := &domain.TaskStaticReport{
		TaskID:       "test-task-001",
		Analyzer:     "hybrid",
		AnalysisMode: "fast",
		Status:       domain.StaticStatusCompleted,
		PackageName:  "com.example.app",
		VersionName:  "1.0.0",
		VersionCode:  "1",
		AppName:      "Test App",
		FileSize:     1024000,
		MD5:          "abc123def456",
		SHA256:       "abc123def456789",
		AnalyzedAt:   &now,
		CreatedAt:    now,
	}

	err := repo.Create(ctx, report)
	assert.NoError(t, err, "Create should not return error")
	assert.NotZero(t, report.ID, "ID should be assigned after creation")

	// 验证报告已创建
	found, err := repo.FindByTaskID(ctx, report.TaskID)
	assert.NoError(t, err)
	assert.Equal(t, report.TaskID, found.TaskID)
	assert.Equal(t, report.PackageName, found.PackageName)
	assert.Equal(t, report.Analyzer, found.Analyzer)
}

// TestStaticReportRepository_Create_Duplicate 测试创建重复报告
func TestStaticReportRepository_Create_Duplicate(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	report := &domain.TaskStaticReport{
		TaskID:      "test-task-002",
		Analyzer:    "hybrid",
		Status:      domain.StaticStatusQueued,
		PackageName: "com.example.app",
	}

	// 第一次创建
	err := repo.Create(ctx, report)
	assert.NoError(t, err)

	// 第二次创建 (应该失败 - task_id 有唯一索引)
	duplicateReport := &domain.TaskStaticReport{
		TaskID:      "test-task-002",
		Analyzer:    "hybrid",
		Status:      domain.StaticStatusQueued,
		PackageName: "com.example.app2",
	}
	err = repo.Create(ctx, duplicateReport)
	assert.Error(t, err, "Creating duplicate task_id should return error")
}

// TestStaticReportRepository_Update 测试更新报告
func TestStaticReportRepository_Update(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	// 创建报告
	report := &domain.TaskStaticReport{
		TaskID:      "test-task-003",
		Analyzer:    "hybrid",
		Status:      domain.StaticStatusAnalyzing,
		PackageName: "com.example.app",
	}
	err := repo.Create(ctx, report)
	require.NoError(t, err)

	// 更新报告
	report.Status = domain.StaticStatusCompleted
	report.AnalysisMode = "deep"
	report.ActivityCount = 25
	report.ServiceCount = 5
	now := time.Now()
	report.AnalyzedAt = &now
	err = repo.Update(ctx, report)
	assert.NoError(t, err)

	// 验证更新
	updated, err := repo.FindByTaskID(ctx, report.TaskID)
	assert.NoError(t, err)
	assert.Equal(t, domain.StaticStatusCompleted, updated.Status)
	assert.Equal(t, domain.StaticAnalysisMode("deep"), updated.AnalysisMode)
	assert.Equal(t, 25, updated.ActivityCount)
	assert.Equal(t, 5, updated.ServiceCount)
	assert.NotNil(t, updated.AnalyzedAt)
}

// TestStaticReportRepository_Upsert_Insert 测试 Upsert - 插入新记录
func TestStaticReportRepository_Upsert_Insert(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	report := &domain.TaskStaticReport{
		TaskID:      "test-task-004",
		Analyzer:    "hybrid",
		Status:      domain.StaticStatusCompleted,
		PackageName: "com.example.app",
		VersionName: "1.0.0",
	}

	// Upsert 新记录（实际是插入）
	err := repo.Upsert(ctx, report)
	assert.NoError(t, err)

	// 验证插入成功
	found, err := repo.FindByTaskID(ctx, report.TaskID)
	assert.NoError(t, err)
	assert.Equal(t, report.PackageName, found.PackageName)
	assert.Equal(t, "1.0.0", found.VersionName)
}

// TestStaticReportRepository_Upsert_Update 测试 Upsert - 更新已存在记录
func TestStaticReportRepository_Upsert_Update(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	// 先创建一条记录
	initialReport := &domain.TaskStaticReport{
		TaskID:      "test-task-005",
		Analyzer:    "hybrid",
		Status:      domain.StaticStatusAnalyzing,
		PackageName: "com.example.app",
		VersionName: "1.0.0",
	}
	err := repo.Create(ctx, initialReport)
	require.NoError(t, err)

	// Upsert 更新（task_id 相同）
	updatedReport := &domain.TaskStaticReport{
		TaskID:       "test-task-005",
		Analyzer:     "hybrid",
		Status:       domain.StaticStatusCompleted,
		PackageName:  "com.example.app",
		VersionName:  "2.0.0",
		VersionCode:  "2",
		ActivityCount: 30,
	}
	err = repo.Upsert(ctx, updatedReport)
	assert.NoError(t, err)

	// 验证更新成功
	found, err := repo.FindByTaskID(ctx, "test-task-005")
	assert.NoError(t, err)
	assert.Equal(t, domain.StaticStatusCompleted, found.Status)
	assert.Equal(t, "2.0.0", found.VersionName)
	assert.Equal(t, "2", found.VersionCode)
	assert.Equal(t, 30, found.ActivityCount)

	// 验证只有一条记录（没有重复插入）
	var count int64
	db.Model(&domain.TaskStaticReport{}).Where("task_id = ?", "test-task-005").Count(&count)
	assert.Equal(t, int64(1), count, "Should only have one record")
}

// TestStaticReportRepository_Upsert_MultipleUpdates 测试 Upsert - 多次更新
func TestStaticReportRepository_Upsert_MultipleUpdates(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	taskID := "test-task-006"

	// 第一次 Upsert（插入）
	report1 := &domain.TaskStaticReport{
		TaskID:      taskID,
		Status:      domain.StaticStatusQueued,
		PackageName: "com.example.app",
	}
	err := repo.Upsert(ctx, report1)
	require.NoError(t, err)

	// 第二次 Upsert（更新为 analyzing）
	report2 := &domain.TaskStaticReport{
		TaskID:        taskID,
		Status:        domain.StaticStatusAnalyzing,
		PackageName:   "com.example.app",
		ActivityCount: 10,
	}
	err = repo.Upsert(ctx, report2)
	require.NoError(t, err)

	// 第三次 Upsert（更新为 completed）
	now := time.Now()
	report3 := &domain.TaskStaticReport{
		TaskID:        taskID,
		Status:        domain.StaticStatusCompleted,
		PackageName:   "com.example.app",
		ActivityCount: 25,
		AnalysisMode:  "deep",
		AnalyzedAt:    &now,
	}
	err = repo.Upsert(ctx, report3)
	require.NoError(t, err)

	// 验证最终状态
	found, err := repo.FindByTaskID(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, domain.StaticStatusCompleted, found.Status)
	assert.Equal(t, domain.StaticAnalysisMode("deep"), found.AnalysisMode)
	assert.Equal(t, 25, found.ActivityCount)
	assert.NotNil(t, found.AnalyzedAt)

	// 验证只有一条记录
	var count int64
	db.Model(&domain.TaskStaticReport{}).Where("task_id = ?", taskID).Count(&count)
	assert.Equal(t, int64(1), count, "Should only have one record after multiple upserts")
}

// TestStaticReportRepository_FindByID 测试按主键 ID 查找
func TestStaticReportRepository_FindByID(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	// 创建测试报告
	report := &domain.TaskStaticReport{
		TaskID:      "test-task-007",
		Analyzer:    "hybrid",
		PackageName: "com.example.app",
	}
	err := repo.Create(ctx, report)
	require.NoError(t, err)

	// 查找存在的报告
	found, err := repo.FindByID(ctx, report.ID)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, report.ID, found.ID)
	assert.Equal(t, report.TaskID, found.TaskID)

	// 查找不存在的报告
	notFound, err := repo.FindByID(ctx, 99999)
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

// TestStaticReportRepository_FindByTaskID 测试按 TaskID 查找
func TestStaticReportRepository_FindByTaskID(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	// 创建测试报告
	report := &domain.TaskStaticReport{
		TaskID:      "test-task-008",
		Analyzer:    "hybrid",
		PackageName: "com.example.app",
	}
	err := repo.Create(ctx, report)
	require.NoError(t, err)

	// 查找存在的报告
	found, err := repo.FindByTaskID(ctx, "test-task-008")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "test-task-008", found.TaskID)

	// 查找不存在的报告
	notFound, err := repo.FindByTaskID(ctx, "non-existent-task")
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

// TestStaticReportRepository_Delete 测试删除报告
func TestStaticReportRepository_Delete(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	report := &domain.TaskStaticReport{
		TaskID:      "test-task-009",
		Analyzer:    "hybrid",
		PackageName: "com.example.app",
	}
	err := repo.Create(ctx, report)
	require.NoError(t, err)

	// 删除报告
	err = repo.Delete(ctx, report.TaskID)
	assert.NoError(t, err)

	// 验证已删除
	_, err = repo.FindByTaskID(ctx, report.TaskID)
	assert.Error(t, err)
}

// TestStaticReportRepository_Delete_NonExistent 测试删除不存在的报告
func TestStaticReportRepository_Delete_NonExistent(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	// 删除不存在的报告（不应该报错）
	err := repo.Delete(ctx, "non-existent-task")
	assert.NoError(t, err, "Deleting non-existent report should not error")
}

// TestStaticReportRepository_CompleteWorkflow 测试完整工作流
func TestStaticReportRepository_CompleteWorkflow(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	taskID := "test-task-workflow"

	// 步骤 1: 创建初始报告（queued 状态）
	t.Run("Create initial report", func(t *testing.T) {
		report := &domain.TaskStaticReport{
			TaskID:   taskID,
			Analyzer: "hybrid",
			Status:   domain.StaticStatusQueued,
		}
		err := repo.Upsert(ctx, report)
		assert.NoError(t, err)

		found, err := repo.FindByTaskID(ctx, taskID)
		assert.NoError(t, err)
		assert.Equal(t, domain.StaticStatusQueued, found.Status)
	})

	// 步骤 2: 开始分析（analyzing 状态）
	t.Run("Update to analyzing", func(t *testing.T) {
		report := &domain.TaskStaticReport{
			TaskID:      taskID,
			Analyzer:    "hybrid",
			Status:      domain.StaticStatusAnalyzing,
			PackageName: "com.example.app",
		}
		err := repo.Upsert(ctx, report)
		assert.NoError(t, err)

		found, err := repo.FindByTaskID(ctx, taskID)
		assert.NoError(t, err)
		assert.Equal(t, domain.StaticStatusAnalyzing, found.Status)
		assert.Equal(t, "com.example.app", found.PackageName)
	})

	// 步骤 3: 完成分析（completed 状态）
	t.Run("Complete analysis", func(t *testing.T) {
		now := time.Now()
		report := &domain.TaskStaticReport{
			TaskID:        taskID,
			Analyzer:      "hybrid",
			AnalysisMode:  "deep",
			Status:        domain.StaticStatusCompleted,
			PackageName:   "com.example.app",
			VersionName:   "1.0.0",
			VersionCode:   "1",
			ActivityCount: 25,
			ServiceCount:  5,
			BasicInfoJSON: `{"package":"com.example.app"}`,
			AnalyzedAt:    &now,
		}
		err := repo.Upsert(ctx, report)
		assert.NoError(t, err)

		found, err := repo.FindByTaskID(ctx, taskID)
		assert.NoError(t, err)
		assert.Equal(t, domain.StaticStatusCompleted, found.Status)
		assert.Equal(t, domain.StaticAnalysisMode("deep"), found.AnalysisMode)
		assert.Equal(t, 25, found.ActivityCount)
		assert.NotEmpty(t, found.BasicInfoJSON)
		assert.NotNil(t, found.AnalyzedAt)
	})

	// 步骤 4: 删除报告
	t.Run("Delete report", func(t *testing.T) {
		err := repo.Delete(ctx, taskID)
		assert.NoError(t, err)

		_, err = repo.FindByTaskID(ctx, taskID)
		assert.Error(t, err)
	})
}

// TestStaticReportRepository_JSONFields 测试 JSON 字段存储
func TestStaticReportRepository_JSONFields(t *testing.T) {
	db := setupStaticReportTestDB(t)
	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	basicInfoJSON := `{
		"package_name": "com.example.app",
		"version_name": "1.0.0",
		"activities": ["MainActivity", "LoginActivity"]
	}`

	deepAnalysisJSON := `{
		"urls": ["https://api.example.com", "https://cdn.example.com"],
		"domains": ["api.example.com", "cdn.example.com"]
	}`

	report := &domain.TaskStaticReport{
		TaskID:           "test-task-json",
		Analyzer:         "hybrid",
		BasicInfoJSON:    basicInfoJSON,
		DeepAnalysisJSON: deepAnalysisJSON,
	}

	err := repo.Create(ctx, report)
	require.NoError(t, err)

	// 验证 JSON 字段正确存储和读取
	found, err := repo.FindByTaskID(ctx, "test-task-json")
	assert.NoError(t, err)
	assert.JSONEq(t, basicInfoJSON, found.BasicInfoJSON)
	assert.JSONEq(t, deepAnalysisJSON, found.DeepAnalysisJSON)
}

// BenchmarkStaticReportRepository_Create 性能测试 - 创建报告
func BenchmarkStaticReportRepository_Create(b *testing.B) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.TaskStaticReport{})

	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report := &domain.TaskStaticReport{
			TaskID:      generateTaskID(i),
			Analyzer:    "hybrid",
			PackageName: "com.example.app",
		}
		repo.Create(ctx, report)
	}
}

// BenchmarkStaticReportRepository_Upsert 性能测试 - Upsert
func BenchmarkStaticReportRepository_Upsert(b *testing.B) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.TaskStaticReport{})

	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report := &domain.TaskStaticReport{
			TaskID:      generateTaskID(i),
			Analyzer:    "hybrid",
			Status:      domain.StaticStatusCompleted,
			PackageName: "com.example.app",
		}
		repo.Upsert(ctx, report)
	}
}

// BenchmarkStaticReportRepository_FindByTaskID 性能测试 - 查找
func BenchmarkStaticReportRepository_FindByTaskID(b *testing.B) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.TaskStaticReport{})

	repo := NewStaticReportRepository(db)
	ctx := context.Background()

	// 预先创建报告
	report := &domain.TaskStaticReport{
		TaskID:      "bench-task",
		Analyzer:    "hybrid",
		PackageName: "com.example.app",
	}
	repo.Create(ctx, report)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.FindByTaskID(ctx, "bench-task")
	}
}

// generateTaskID 生成测试用的 task ID
func generateTaskID(i int) string {
	return string(rune('A'+i%26)) + string(rune('A'+(i/26)%26)) + "-bench-task"
}
