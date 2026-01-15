package repository

import (
	"context"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// FlowRepository 流量数据访问接口
type FlowRepository interface {
	// SaveFlows 批量保存流量记录
	SaveFlows(ctx context.Context, flows []*domain.TaskFlow) error
	// GetFlowsByTaskID 获取任务的所有流量记录
	GetFlowsByTaskID(ctx context.Context, taskID string) ([]*domain.TaskFlow, error)
	// GetUniqueURLsByTaskID 获取任务的去重 URL 列表
	GetUniqueURLsByTaskID(ctx context.Context, taskID string) ([]string, error)
	// GetDomainStatsByTaskID 获取任务的域名统计
	GetDomainStatsByTaskID(ctx context.Context, taskID string) (map[string]int, error)
	// GetFlowCountByTaskID 获取任务的流量数量
	GetFlowCountByTaskID(ctx context.Context, taskID string) (int64, error)
	// DeleteFlowsByTaskID 删除任务的所有流量记录
	DeleteFlowsByTaskID(ctx context.Context, taskID string) error
}

type flowRepo struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewFlowRepository 创建流量数据访问实例
func NewFlowRepository(db *gorm.DB, logger *logrus.Logger) FlowRepository {
	return &flowRepo{
		db:     db,
		logger: logger,
	}
}

// SaveFlows 批量保存流量记录
func (r *flowRepo) SaveFlows(ctx context.Context, flows []*domain.TaskFlow) error {
	if len(flows) == 0 {
		return nil
	}

	r.logger.WithFields(logrus.Fields{
		"count": len(flows),
	}).Debug("Saving flows to database")

	// 批量插入，每批 100 条
	err := r.db.WithContext(ctx).CreateInBatches(flows, 100).Error
	if err != nil {
		r.logger.WithError(err).Error("Failed to save flows to database")
		return err
	}

	r.logger.WithFields(logrus.Fields{
		"count": len(flows),
	}).Info("Successfully saved flows to database")

	return nil
}

// GetFlowsByTaskID 获取任务的所有流量记录
func (r *flowRepo) GetFlowsByTaskID(ctx context.Context, taskID string) ([]*domain.TaskFlow, error) {
	var flows []*domain.TaskFlow
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("timestamp ASC").
		Find(&flows).Error

	if err != nil {
		r.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get flows")
		return nil, err
	}

	return flows, nil
}

// GetUniqueURLsByTaskID 获取任务的去重 URL 列表
func (r *flowRepo) GetUniqueURLsByTaskID(ctx context.Context, taskID string) ([]string, error) {
	var urls []string
	err := r.db.WithContext(ctx).
		Model(&domain.TaskFlow{}).
		Where("task_id = ?", taskID).
		Distinct("url").
		Pluck("url", &urls).Error

	if err != nil {
		r.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get unique URLs")
		return nil, err
	}

	r.logger.WithFields(logrus.Fields{
		"task_id":    taskID,
		"url_count":  len(urls),
	}).Debug("Got unique URLs from database")

	return urls, nil
}

// GetDomainStatsByTaskID 获取任务的域名统计
func (r *flowRepo) GetDomainStatsByTaskID(ctx context.Context, taskID string) (map[string]int, error) {
	type DomainCount struct {
		Host  string
		Count int
	}
	var results []DomainCount

	err := r.db.WithContext(ctx).
		Model(&domain.TaskFlow{}).
		Select("host, COUNT(*) as count").
		Where("task_id = ?", taskID).
		Group("host").
		Order("count DESC").
		Scan(&results).Error

	if err != nil {
		r.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get domain stats")
		return nil, err
	}

	stats := make(map[string]int)
	for _, r := range results {
		stats[r.Host] = r.Count
	}

	return stats, nil
}

// GetFlowCountByTaskID 获取任务的流量数量
func (r *flowRepo) GetFlowCountByTaskID(ctx context.Context, taskID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.TaskFlow{}).
		Where("task_id = ?", taskID).
		Count(&count).Error

	if err != nil {
		r.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get flow count")
		return 0, err
	}

	return count, nil
}

// DeleteFlowsByTaskID 删除任务的所有流量记录
func (r *flowRepo) DeleteFlowsByTaskID(ctx context.Context, taskID string) error {
	result := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Delete(&domain.TaskFlow{})

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", taskID).Error("Failed to delete flows")
		return result.Error
	}

	r.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"deleted_count": result.RowsAffected,
	}).Info("Deleted flows from database")

	return nil
}
