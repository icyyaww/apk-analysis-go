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
	GetPackerStatistics(ctx context.Context) (*PackerStatistics, error)
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
				"developer", "company_name", // 开发者和公司信息
				"activity_count", "service_count", "receiver_count", "provider_count", "permission_count",
				"url_count", "domain_count", // URL 和域名计数字段
				"basic_info_json", "deep_analysis_json",
				"analysis_duration_ms", "fast_analysis_duration_ms", "deep_analysis_duration_ms",
				"needs_deep_analysis_reason", "analyzed_at",
				// 壳检测相关字段
				"is_packed", "packer_name", "packer_type", "packer_confidence",
				"packer_indicators", "needs_dynamic_unpacking", "packer_detection_duration_ms",
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

// PackerStatistics 壳检测统计信息
type PackerStatistics struct {
	TotalScanned     int64   `json:"total_scanned"`
	PackedCount      int64   `json:"packed_count"`
	UnpackedCount    int64   `json:"unpacked_count"`
	PackedRate       float64 `json:"packed_rate"`
	NeedsUnpackCount int64   `json:"needs_unpack_count"`
	PackerBreakdown  []PackerBreakdownItem `json:"packer_breakdown,omitempty"`
}

// PackerBreakdownItem 壳类型分布
type PackerBreakdownItem struct {
	PackerName       string  `json:"packer_name"`
	PackerType       string  `json:"packer_type"`
	Count            int64   `json:"count"`
	AvgConfidence    float64 `json:"avg_confidence"`
	NeedsUnpackCount int64   `json:"needs_unpack_count"`
}

// GetPackerStatistics 获取壳检测统计信息
func (r *staticReportRepo) GetPackerStatistics(ctx context.Context) (*PackerStatistics, error) {
	var stats PackerStatistics

	// 总体统计
	err := r.db.WithContext(ctx).Model(&domain.TaskStaticReport{}).
		Select(`
			COUNT(*) as total_scanned,
			SUM(CASE WHEN is_packed = true THEN 1 ELSE 0 END) as packed_count,
			SUM(CASE WHEN is_packed = false THEN 1 ELSE 0 END) as unpacked_count,
			SUM(CASE WHEN needs_dynamic_unpacking = true THEN 1 ELSE 0 END) as needs_unpack_count
		`).Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	// 计算加壳率
	if stats.TotalScanned > 0 {
		stats.PackedRate = float64(stats.PackedCount) / float64(stats.TotalScanned) * 100
	}

	// 按壳类型统计
	var breakdown []PackerBreakdownItem
	err = r.db.WithContext(ctx).Model(&domain.TaskStaticReport{}).
		Select(`
			packer_name,
			packer_type,
			COUNT(*) as count,
			AVG(packer_confidence) as avg_confidence,
			SUM(CASE WHEN needs_dynamic_unpacking = true THEN 1 ELSE 0 END) as needs_unpack_count
		`).
		Where("is_packed = true AND packer_name IS NOT NULL AND packer_name != ''").
		Group("packer_name, packer_type").
		Order("count DESC").
		Scan(&breakdown).Error
	if err == nil {
		stats.PackerBreakdown = breakdown
	}

	return &stats, nil
}
