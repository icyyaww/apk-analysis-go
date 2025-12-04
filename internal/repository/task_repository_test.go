package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to open test database")

	// 迁移所有相关表（因为 FindByID 会 Preload 这些关联表）
	// 逐个迁移避免索引冲突导致后续表没有创建
	tables := []interface{}{
		&domain.Task{},
		&domain.TaskActivity{},
		&domain.TaskMobSFReport{},
		&domain.TaskStaticReport{},
		&domain.TaskDomainAnalysis{},
		&domain.TaskAppDomain{},
		&domain.TaskAILog{},
	}

	for _, table := range tables {
		err = db.AutoMigrate(table)
		// Ignore "index already exists" errors (happens in test environment)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			require.NoError(t, err, "Failed to migrate test database")
		}
	}

	return db
}

// TestTaskRepository_Create 测试创建任务
func TestTaskRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &domain.Task{
		ID:              "test-task-001",
		APKName:         "test.apk",
		Status:          domain.TaskStatusQueued,
		CreatedAt:       time.Now(),
		ProgressPercent: 0,
	}

	err := repo.Create(ctx, task)
	assert.NoError(t, err, "Create should not return error")

	// 验证任务已创建
	found, err := repo.FindByID(ctx, task.ID)
	assert.NoError(t, err)
	assert.Equal(t, task.ID, found.ID)
	assert.Equal(t, task.APKName, found.APKName)
	assert.Equal(t, domain.TaskStatusQueued, found.Status)
}

// TestTaskRepository_Create_Duplicate 测试创建重复任务
func TestTaskRepository_Create_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &domain.Task{
		ID:      "test-task-002",
		APKName: "test.apk",
		Status:  domain.TaskStatusQueued,
	}

	// 第一次创建
	err := repo.Create(ctx, task)
	assert.NoError(t, err)

	// 第二次创建 (应该失败)
	err = repo.Create(ctx, task)
	assert.Error(t, err, "Creating duplicate task should return error")
}

// TestTaskRepository_FindByID 测试按ID查找
func TestTaskRepository_FindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	// 创建测试任务
	task := &domain.Task{
		ID:      "test-task-003",
		APKName: "test.apk",
		Status:  domain.TaskStatusQueued,
	}
	err := repo.Create(ctx, task)
	require.NoError(t, err)

	// 查找存在的任务
	found, err := repo.FindByID(ctx, task.ID)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, task.ID, found.ID)

	// 查找不存在的任务
	notFound, err := repo.FindByID(ctx, "non-existent-id")
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

// TestTaskRepository_Update 测试更新任务
func TestTaskRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	// 创建任务
	task := &domain.Task{
		ID:              "test-task-004",
		APKName:         "test.apk",
		Status:          domain.TaskStatusQueued,
		ProgressPercent: 0,
	}
	err := repo.Create(ctx, task)
	require.NoError(t, err)

	// 更新任务
	task.Status = domain.TaskStatusRunning
	task.ProgressPercent = 50
	task.CurrentStep = "正在分析"
	err = repo.Update(ctx, task)
	assert.NoError(t, err)

	// 验证更新
	updated, err := repo.FindByID(ctx, task.ID)
	assert.NoError(t, err)
	assert.Equal(t, domain.TaskStatusRunning, updated.Status)
	assert.Equal(t, 50, updated.ProgressPercent)
	assert.Equal(t, "正在分析", updated.CurrentStep)
}

// TestTaskRepository_UpdateStatus 测试更新状态
func TestTaskRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &domain.Task{
		ID:      "test-task-005",
		APKName: "test.apk",
		Status:  domain.TaskStatusQueued,
	}
	err := repo.Create(ctx, task)
	require.NoError(t, err)

	// 更新状态
	err = repo.UpdateStatus(ctx, task.ID, domain.TaskStatusCompleted)
	assert.NoError(t, err)

	// 验证状态
	updated, err := repo.FindByID(ctx, task.ID)
	assert.NoError(t, err)
	assert.Equal(t, domain.TaskStatusCompleted, updated.Status)
}

// TestTaskRepository_UpdateProgress 测试更新进度
func TestTaskRepository_UpdateProgress(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &domain.Task{
		ID:              "test-task-006",
		APKName:         "test.apk",
		ProgressPercent: 0,
	}
	err := repo.Create(ctx, task)
	require.NoError(t, err)

	// 更新进度
	err = repo.UpdateProgress(ctx, task.ID, "正在收集流量", 75)
	assert.NoError(t, err)

	// 验证进度
	updated, err := repo.FindByID(ctx, task.ID)
	assert.NoError(t, err)
	assert.Equal(t, 75, updated.ProgressPercent)
	assert.Equal(t, "正在收集流量", updated.CurrentStep)
}

// TestTaskRepository_List 测试列出任务
func TestTaskRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	// 创建多个任务
	for i := 1; i <= 5; i++ {
		task := &domain.Task{
			ID:        string(rune('A' + i - 1)) + "-task",
			APKName:   "test.apk",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
		err := repo.Create(ctx, task)
		require.NoError(t, err)
	}

	// 列出最近 3 个任务
	tasks, err := repo.List(ctx, 3)
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)

	// 验证按创建时间倒序
	for i := 0; i < len(tasks)-1; i++ {
		assert.True(t, tasks[i].CreatedAt.After(tasks[i+1].CreatedAt) ||
			tasks[i].CreatedAt.Equal(tasks[i+1].CreatedAt))
	}
}

// TestTaskRepository_Delete 测试删除任务
func TestTaskRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &domain.Task{
		ID:      "test-task-007",
		APKName: "test.apk",
	}
	err := repo.Create(ctx, task)
	require.NoError(t, err)

	// 删除任务
	err = repo.Delete(ctx, task.ID)
	assert.NoError(t, err)

	// 验证已删除
	_, err = repo.FindByID(ctx, task.ID)
	assert.Error(t, err)
}

// 注意：以下方法已被移除，因为它们不再是 TaskRepository 接口的一部分：
// - FindByStatus: 可以使用 List 或 ListWithPagination 配合过滤逻辑实现
// - CountByStatus: 统计信息现在通过 GetSystemStats API 获取
// - UpdateMobSFStatus: MobSF 信息现在存储在独立的 task_mobsf_reports 表中，使用 MobSFReportRepository 管理

// TestTaskRepository_ShouldStop 测试停止标记
func TestTaskRepository_ShouldStop(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &domain.Task{
		ID:         "test-task-008",
		APKName:    "test.apk",
		ShouldStop: false,
	}
	err := repo.Create(ctx, task)
	require.NoError(t, err)

	// 检查初始状态
	shouldStop, err := repo.ShouldStop(ctx, task.ID)
	assert.NoError(t, err)
	assert.False(t, shouldStop)

	// 标记停止
	err = repo.MarkShouldStop(ctx, task.ID)
	assert.NoError(t, err)

	// 验证标记
	shouldStop, err = repo.ShouldStop(ctx, task.ID)
	assert.NoError(t, err)
	assert.True(t, shouldStop)
}

// BenchmarkTaskRepository_Create 性能测试 - 创建任务
func BenchmarkTaskRepository_Create(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.Task{})

	repo := NewTaskRepository(db)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := &domain.Task{
			ID:      string(rune('A' + i%26)) + string(rune('A' + (i/26)%26)) + "-bench",
			APKName: "bench.apk",
		}
		repo.Create(ctx, task)
	}
}

// BenchmarkTaskRepository_FindByID 性能测试 - 查找任务
func BenchmarkTaskRepository_FindByID(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&domain.Task{})

	repo := NewTaskRepository(db)
	ctx := context.Background()

	// 预先创建任务
	task := &domain.Task{
		ID:      "bench-task",
		APKName: "bench.apk",
	}
	repo.Create(ctx, task)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.FindByID(ctx, "bench-task")
	}
}
