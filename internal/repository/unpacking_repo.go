package repository

import (
	"context"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// UnpackingRepository 脱壳结果仓库接口
type UnpackingRepository interface {
	Create(ctx context.Context, result *domain.TaskUnpackingResult) error
	GetByTaskID(ctx context.Context, taskID string) (*domain.TaskUnpackingResult, error)
	Update(ctx context.Context, result *domain.TaskUnpackingResult) error
	GetStatistics(ctx context.Context) (*UnpackingStatistics, error)
}

// UnpackingStatistics 脱壳统计信息
type UnpackingStatistics struct {
	TotalAttempts  int64   `json:"total_attempts"`
	SuccessCount   int64   `json:"success_count"`
	FailedCount    int64   `json:"failed_count"`
	TimeoutCount   int64   `json:"timeout_count"`
	SkippedCount   int64   `json:"skipped_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
	AvgDexCount    float64 `json:"avg_dex_count"`
	MethodStats    []MethodStat `json:"method_stats,omitempty"`
}

// MethodStat 按脱壳方法统计
type MethodStat struct {
	Method        string  `json:"method"`
	Count         int64   `json:"count"`
	SuccessCount  int64   `json:"success_count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// unpackingRepository 脱壳结果仓库实现
type unpackingRepository struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewUnpackingRepository 创建脱壳结果仓库
func NewUnpackingRepository(db *gorm.DB, logger *logrus.Logger) UnpackingRepository {
	return &unpackingRepository{
		db:     db,
		logger: logger,
	}
}

// Create 创建脱壳结果记录
func (r *unpackingRepository) Create(ctx context.Context, result *domain.TaskUnpackingResult) error {
	return r.db.WithContext(ctx).Create(result).Error
}

// GetByTaskID 根据任务ID获取脱壳结果
func (r *unpackingRepository) GetByTaskID(ctx context.Context, taskID string) (*domain.TaskUnpackingResult, error) {
	var result domain.TaskUnpackingResult
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Update 更新脱壳结果
func (r *unpackingRepository) Update(ctx context.Context, result *domain.TaskUnpackingResult) error {
	return r.db.WithContext(ctx).Save(result).Error
}

// GetStatistics 获取脱壳统计信息
func (r *unpackingRepository) GetStatistics(ctx context.Context) (*UnpackingStatistics, error) {
	var stats UnpackingStatistics

	// 总体统计
	err := r.db.WithContext(ctx).Model(&domain.TaskUnpackingResult{}).
		Select(`
			COUNT(*) as total_attempts,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_count,
			SUM(CASE WHEN status = 'timeout' THEN 1 ELSE 0 END) as timeout_count,
			SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END) as skipped_count,
			AVG(duration_ms) as avg_duration_ms,
			AVG(dumped_dex_count) as avg_dex_count
		`).Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	// 计算成功率
	if stats.TotalAttempts > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalAttempts) * 100
	}

	// 按方法统计
	var methodStats []MethodStat
	err = r.db.WithContext(ctx).Model(&domain.TaskUnpackingResult{}).
		Select(`
			method,
			COUNT(*) as count,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
			AVG(duration_ms) as avg_duration_ms
		`).
		Where("method IS NOT NULL AND method != ''").
		Group("method").
		Scan(&methodStats).Error
	if err != nil {
		r.logger.WithError(err).Warn("Failed to get method statistics")
	} else {
		for i := range methodStats {
			if methodStats[i].Count > 0 {
				methodStats[i].SuccessRate = float64(methodStats[i].SuccessCount) / float64(methodStats[i].Count) * 100
			}
		}
		stats.MethodStats = methodStats
	}

	return &stats, nil
}
