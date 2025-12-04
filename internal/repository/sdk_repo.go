package repository

import (
	"context"
	"fmt"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"gorm.io/gorm"
)

// SDKRepository SDK规则数据访问层
type SDKRepository struct {
	db *gorm.DB
}

// NewSDKRepository 创建SDK规则仓库
func NewSDKRepository(db *gorm.DB) *SDKRepository {
	return &SDKRepository{db: db}
}

// ListSDKRules 查询SDK规则列表（支持分页和过滤）
func (r *SDKRepository) ListSDKRules(ctx context.Context, page, limit int, category, status, search string) ([]domain.ThirdPartySDKRule, int64, error) {
	var rules []domain.ThirdPartySDKRule
	var total int64

	query := r.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{})

	// 应用过滤条件
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		query = query.Where("domain LIKE ? OR provider LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count SDK rules: %w", err)
	}

	// 分页查询
	offset := (page - 1) * limit
	if err := query.Order("priority DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&rules).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query SDK rules: %w", err)
	}

	return rules, total, nil
}

// GetSDKRule 根据ID获取SDK规则
func (r *SDKRepository) GetSDKRule(ctx context.Context, id uint) (*domain.ThirdPartySDKRule, error) {
	var rule domain.ThirdPartySDKRule
	if err := r.db.WithContext(ctx).First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// CreateSDKRule 创建SDK规则
func (r *SDKRepository) CreateSDKRule(ctx context.Context, rule *domain.ThirdPartySDKRule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// UpdateSDKRule 更新SDK规则
func (r *SDKRepository) UpdateSDKRule(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// DeleteSDKRule 删除SDK规则
func (r *SDKRepository) DeleteSDKRule(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.ThirdPartySDKRule{}, id).Error
}

// GetPendingSDKRules 获取待审核的SDK规则
func (r *SDKRepository) GetPendingSDKRules(ctx context.Context) ([]domain.ThirdPartySDKRule, error) {
	var rules []domain.ThirdPartySDKRule
	if err := r.db.WithContext(ctx).
		Where("status = ?", "pending").
		Order("discover_count DESC, created_at DESC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ApproveSDKRule 审核通过SDK规则
func (r *SDKRepository) ApproveSDKRule(ctx context.Context, id uint, operator string) error {
	return r.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     "active",
			"updated_by": operator,
		}).Error
}

// RejectSDKRule 拒绝SDK规则
func (r *SDKRepository) RejectSDKRule(ctx context.Context, id uint, operator string) error {
	return r.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     "disabled",
			"updated_by": operator,
		}).Error
}

// GetSDKStatistics 获取SDK规则统计信息
func (r *SDKRepository) GetSDKStatistics(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 总规则数
	var total int64
	if err := r.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total_rules"] = total

	// 活跃规则数
	var active int64
	if err := r.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{}).
		Where("status = ?", "active").Count(&active).Error; err != nil {
		return nil, err
	}
	stats["active_rules"] = active

	// 待审核规则数
	var pending int64
	if err := r.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{}).
		Where("status = ?", "pending").Count(&pending).Error; err != nil {
		return nil, err
	}
	stats["pending_rules"] = pending

	// 按分类统计
	type CategoryCount struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
	}
	var categoryCounts []CategoryCount
	if err := r.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Select("category, COUNT(*) as count").
		Where("status = ?", "active").
		Group("category").
		Scan(&categoryCounts).Error; err != nil {
		return nil, err
	}

	byCategory := make(map[string]int64)
	for _, cc := range categoryCounts {
		byCategory[cc.Category] = cc.Count
	}
	stats["by_category"] = byCategory

	// 按来源统计
	type SourceCount struct {
		Source string `json:"source"`
		Count  int64  `json:"count"`
	}
	var sourceCounts []SourceCount
	if err := r.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Select("source, COUNT(*) as count").
		Group("source").
		Scan(&sourceCounts).Error; err != nil {
		return nil, err
	}

	bySource := make(map[string]int64)
	for _, sc := range sourceCounts {
		bySource[sc.Source] = sc.Count
	}
	stats["by_source"] = bySource

	return stats, nil
}
