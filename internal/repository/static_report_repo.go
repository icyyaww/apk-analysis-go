package repository

import (
	"context"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StaticReportRepository 静态分析报告 Repository
type StaticReportRepository interface {
	Create(ctx context.Context, report *domain.TaskStaticReport) error
	Update(ctx context.Context, report *domain.TaskStaticReport) error
	Upsert(ctx context.Context, report *domain.TaskStaticReport) error
	FindByID(ctx context.Context, id uint) (*domain.TaskStaticReport, error)
	FindByTaskID(ctx context.Context, taskID string) (*domain.TaskStaticReport, error)
	Delete(ctx context.Context, taskID string) error
}

// staticReportRepo 静态分析报告 Repository 实现
type staticReportRepo struct {
	db *gorm.DB
}

// NewStaticReportRepository 创建静态分析报告 Repository
func NewStaticReportRepository(db *gorm.DB) StaticReportRepository {
	return &staticReportRepo{db: db}
}

// Create 创建静态分析报告
func (r *staticReportRepo) Create(ctx context.Context, report *domain.TaskStaticReport) error {
	return r.db.WithContext(ctx).Create(report).Error
}

// Update 更新静态分析报告
func (r *staticReportRepo) Update(ctx context.Context, report *domain.TaskStaticReport) error {
	return r.db.WithContext(ctx).Save(report).Error
}

// Upsert 插入或更新静态分析报告（使用 ON DUPLICATE KEY UPDATE）
func (r *staticReportRepo) Upsert(ctx context.Context, report *domain.TaskStaticReport) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "task_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"analyzer", "analysis_mode", "status",
				"package_name", "version_name", "version_code", "app_name",
				"file_size", "md5", "sha256",
				"activity_count", "service_count", "receiver_count", "provider_count", "permission_count",
				"url_count", "domain_count", // 🔧 修复：添加 URL 和域名计数字段
				"basic_info_json", "deep_analysis_json",
				"analysis_duration_ms", "fast_analysis_duration_ms", "deep_analysis_duration_ms",
				"needs_deep_analysis_reason", "analyzed_at",
			}),
		}).
		Create(report).Error
}

// FindByID 根据 ID 查询静态分析报告
func (r *staticReportRepo) FindByID(ctx context.Context, id uint) (*domain.TaskStaticReport, error) {
	var report domain.TaskStaticReport
	err := r.db.WithContext(ctx).First(&report, id).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// FindByTaskID 根据任务 ID 查询静态分析报告
func (r *staticReportRepo) FindByTaskID(ctx context.Context, taskID string) (*domain.TaskStaticReport, error) {
	var report domain.TaskStaticReport
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&report).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// Delete 删除静态分析报告
func (r *staticReportRepo) Delete(ctx context.Context, taskID string) error {
	return r.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&domain.TaskStaticReport{}).Error
}
