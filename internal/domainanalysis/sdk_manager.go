package domainanalysis

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SDKManager 第三方SDK管理器
// 实现两级缓存:
// Level 1: 本地内存缓存 (5分钟TTL)
// Level 2: MySQL数据库 (权威数据源)
type SDKManager struct {
	db     *gorm.DB
	logger *logrus.Logger

	// 内存缓存
	cache      map[string]*SDKRuleInfo // domain -> rule info
	cacheTime  time.Time                // 缓存更新时间
	cacheTTL   time.Duration            // 缓存过期时间
	cacheMutex sync.RWMutex             // 缓存读写锁
}

// SDKRuleInfo SDK规则信息
type SDKRuleInfo struct {
	Domain      string
	Category    string
	SubCategory string
	Provider    string
	Confidence  float64
	Priority    int
}

// NewSDKManager 创建SDK管理器
func NewSDKManager(db *gorm.DB, logger *logrus.Logger) *SDKManager {
	return &SDKManager{
		db:       db,
		logger:   logger,
		cache:    make(map[string]*SDKRuleInfo),
		cacheTTL: 5 * time.Minute,
	}
}

// IsThirdPartyDomain 判断域名是否为第三方SDK
// 返回: (是否第三方, 分类, 服务商)
func (m *SDKManager) IsThirdPartyDomain(ctx context.Context, domain string) (bool, string, string) {
	// 标准化域名
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false, "", ""
	}

	// 先检查内存缓存
	if info := m.getFromCache(domain); info != nil {
		return true, info.Category, info.Provider
	}

	// 缓存未命中，从数据库加载所有规则到缓存
	if err := m.refreshCache(ctx); err != nil {
		m.logger.WithError(err).Warn("Failed to refresh SDK rules cache")
		return false, "", ""
	}

	// 再次从缓存查询
	if info := m.getFromCache(domain); info != nil {
		return true, info.Category, info.Provider
	}

	return false, "", ""
}

// getFromCache 从内存缓存中获取规则
func (m *SDKManager) getFromCache(domain string) *SDKRuleInfo {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()

	// 检查缓存是否过期
	if time.Since(m.cacheTime) > m.cacheTTL {
		return nil
	}

	// 精确匹配
	if info, ok := m.cache[domain]; ok {
		return info
	}

	// 后缀匹配 (例如 api.qq.com 匹配 qq.com)
	for cachedDomain, info := range m.cache {
		if strings.HasSuffix(domain, "."+cachedDomain) {
			return info
		}
	}

	return nil
}

// refreshCache 从数据库刷新缓存
func (m *SDKManager) refreshCache(ctx context.Context) error {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	// 检查是否需要刷新 (避免并发多次刷新)
	if time.Since(m.cacheTime) < m.cacheTTL {
		return nil
	}

	// 从数据库加载所有启用的规则
	var rules []domain.ThirdPartySDKRule
	err := m.db.WithContext(ctx).
		Where("status = ?", "active").
		Order("priority DESC, confidence DESC").
		Find(&rules).Error

	if err != nil {
		return fmt.Errorf("failed to load SDK rules from database: %w", err)
	}

	// 更新缓存
	newCache := make(map[string]*SDKRuleInfo, len(rules))
	for _, rule := range rules {
		newCache[rule.Domain] = &SDKRuleInfo{
			Domain:      rule.Domain,
			Category:    rule.Category,
			SubCategory: rule.SubCategory,
			Provider:    rule.Provider,
			Confidence:  rule.Confidence,
			Priority:    rule.Priority,
		}
	}

	m.cache = newCache
	m.cacheTime = time.Now()

	m.logger.WithFields(logrus.Fields{
		"rules_count": len(rules),
		"cache_time":  m.cacheTime.Format(time.RFC3339),
	}).Info("SDK rules cache refreshed")

	return nil
}

// GetRuleInfo 获取域名的详细规则信息
func (m *SDKManager) GetRuleInfo(ctx context.Context, domain string) *SDKRuleInfo {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil
	}

	// 先从缓存获取
	if info := m.getFromCache(domain); info != nil {
		return info
	}

	// 刷新缓存后重试
	if err := m.refreshCache(ctx); err != nil {
		m.logger.WithError(err).Warn("Failed to refresh SDK rules cache")
		return nil
	}

	return m.getFromCache(domain)
}

// AddDiscoveredDomain 添加自动发现的域名到待审核列表
// 注意: 这个方法会直接插入数据库，status为pending
func (m *SDKManager) AddDiscoveredDomain(ctx context.Context, taskID, domainName, category string, confidence float64) error {
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// 检查是否已存在
	var existing domain.ThirdPartySDKRule
	err := m.db.WithContext(ctx).
		Where("domain = ?", domainName).
		First(&existing).Error

	if err == nil {
		// 已存在，更新发现次数
		return m.db.WithContext(ctx).
			Model(&domain.ThirdPartySDKRule{}).
			Where("domain = ?", domainName).
			Updates(map[string]interface{}{
				"discover_count": gorm.Expr("discover_count + 1"),
			}).Error
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing rule: %w", err)
	}

	// 不存在，插入新规则
	rule := domain.ThirdPartySDKRule{
		Domain:          domainName,
		Category:        category,
		Source:          "auto_detected",
		Confidence:      confidence,
		Status:          "pending", // 待审核
		DiscoverCount:   1,
		FirstSeenTaskID: taskID,
		CreatedBy:       "auto_discovery",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := m.db.WithContext(ctx).Create(&rule).Error; err != nil {
		return fmt.Errorf("failed to create discovered rule: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"domain":   domainName,
		"category": category,
		"task_id":  taskID,
	}).Info("Discovered new potential third-party domain")

	return nil
}

// ApproveDomain 审核通过待审核的域名
func (m *SDKManager) ApproveDomain(ctx context.Context, domainName string, operator string) error {
	domainName = strings.ToLower(strings.TrimSpace(domainName))

	err := m.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Where("domain = ? AND status = ?", domainName, "pending").
		Updates(map[string]interface{}{
			"status":     "active",
			"updated_by": operator,
			"updated_at": time.Now(),
		}).Error

	if err != nil {
		return fmt.Errorf("failed to approve domain: %w", err)
	}

	// 清空缓存，强制下次查询时重新加载
	m.cacheMutex.Lock()
	m.cacheTime = time.Time{} // 设置为零值，表示缓存已过期
	m.cacheMutex.Unlock()

	m.logger.WithFields(logrus.Fields{
		"domain":   domainName,
		"operator": operator,
	}).Info("Domain rule approved")

	return nil
}

// GetStatistics 获取SDK规则统计信息
func (m *SDKManager) GetStatistics(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 总规则数
	var total int64
	if err := m.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total

	// 启用规则数
	var active int64
	if err := m.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{}).Where("status = ?", "active").Count(&active).Error; err != nil {
		return nil, err
	}
	stats["active"] = active

	// 待审核规则数
	var pending int64
	if err := m.db.WithContext(ctx).Model(&domain.ThirdPartySDKRule{}).Where("status = ?", "pending").Count(&pending).Error; err != nil {
		return nil, err
	}
	stats["pending"] = pending

	// 各分类统计
	type CategoryCount struct {
		Category string
		Count    int64
	}
	var categoryCounts []CategoryCount
	if err := m.db.WithContext(ctx).
		Model(&domain.ThirdPartySDKRule{}).
		Select("category, COUNT(*) as count").
		Where("status = ?", "active").
		Group("category").
		Order("count DESC").
		Limit(10).
		Scan(&categoryCounts).Error; err != nil {
		return nil, err
	}

	categories := make(map[string]int64)
	for _, cc := range categoryCounts {
		categories[cc.Category] = cc.Count
	}
	stats["categories"] = categories

	return stats, nil
}
