package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SDKHandler SDK 规则处理器
type SDKHandler struct {
	repo   *repository.SDKRepository
	logger *logrus.Logger
}

// NewSDKHandler 创建 SDK 规则处理器实例
func NewSDKHandler(repo *repository.SDKRepository, logger *logrus.Logger) *SDKHandler {
	return &SDKHandler{
		repo:   repo,
		logger: logger,
	}
}

// ListSDKRules 获取 SDK 规则列表
// GET /api/sdk_rules?page=1&limit=50&category=ad&status=active&search=keyword
func (h *SDKHandler) ListSDKRules(c *gin.Context) {
	// 解析查询参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	category := c.Query("category")
	status := c.Query("status")
	search := c.Query("search")

	h.logger.WithFields(logrus.Fields{
		"page":     page,
		"limit":    limit,
		"category": category,
		"status":   status,
		"search":   search,
	}).Info("Listing SDK rules")

	// 查询数据库
	rules, total, err := h.repo.ListSDKRules(c.Request.Context(), page, limit, category, status, search)
	if err != nil {
		h.logger.WithError(err).Error("Failed to list SDK rules")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	// 计算总页数
	pages := int(total) / limit
	if int(total)%limit != 0 {
		pages++
	}

	response := gin.H{
		"rules": rules,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": pages,
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetPendingSDKRules 获取待审核的 SDK 规则
// GET /api/sdk_rules/pending
func (h *SDKHandler) GetPendingSDKRules(c *gin.Context) {
	rules, err := h.repo.GetPendingSDKRules(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get pending SDK rules")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rules": rules,
		"total": len(rules),
	})
}

// ApproveSDKRule 审核通过 SDK 规则
// POST /api/sdk_rules/:id/approve
func (h *SDKHandler) ApproveSDKRule(c *gin.Context) {
	ruleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的规则ID",
		})
		return
	}

	h.logger.WithField("rule_id", ruleID).Info("Approving SDK rule")

	operator := c.GetString("operator")
	if operator == "" {
		operator = "admin"
	}

	if err := h.repo.ApproveSDKRule(c.Request.Context(), uint(ruleID), operator); err != nil {
		h.logger.WithError(err).Error("Failed to approve SDK rule")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "审核失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "SDK 规则已审核通过",
	})
}

// RejectSDKRule 拒绝 SDK 规则
// POST /api/sdk_rules/:id/reject
func (h *SDKHandler) RejectSDKRule(c *gin.Context) {
	ruleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的规则ID",
		})
		return
	}

	h.logger.WithField("rule_id", ruleID).Info("Rejecting SDK rule")

	operator := c.GetString("operator")
	if operator == "" {
		operator = "admin"
	}

	if err := h.repo.RejectSDKRule(c.Request.Context(), uint(ruleID), operator); err != nil {
		h.logger.WithError(err).Error("Failed to reject SDK rule")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "拒绝失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "SDK 规则已拒绝",
	})
}

// GetSDKStatistics 获取 SDK 规则统计信息
// GET /api/sdk_rules/statistics
func (h *SDKHandler) GetSDKStatistics(c *gin.Context) {
	stats, err := h.repo.GetSDKStatistics(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get SDK statistics")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetSDKCategories 获取 SDK 分类列表
// GET /api/sdk_rules/categories
func (h *SDKHandler) GetSDKCategories(c *gin.Context) {
	categories := []gin.H{
		{"value": "ad", "label": "广告", "color": "#f44336"},
		{"value": "analytics", "label": "统计分析", "color": "#2196f3"},
		{"value": "push", "label": "消息推送", "color": "#4caf50"},
		{"value": "payment", "label": "支付", "color": "#ff9800"},
		{"value": "social", "label": "社交分享", "color": "#9c27b0"},
		{"value": "cdn", "label": "CDN", "color": "#00bcd4"},
		{"value": "cloud", "label": "云服务", "color": "#607d8b"},
		{"value": "other", "label": "其他", "color": "#9e9e9e"},
	}

	c.JSON(http.StatusOK, categories)
}

// CreateSDKRule 创建 SDK 规则
// POST /api/sdk_rules
func (h *SDKHandler) CreateSDKRule(c *gin.Context) {
	var input struct {
		Domain      string  `json:"domain" binding:"required"`
		Category    string  `json:"category" binding:"required"`
		SubCategory string  `json:"sub_category"`
		Provider    string  `json:"provider" binding:"required"`
		Description string  `json:"description"`
		Confidence  float64 `json:"confidence"`
		Priority    int     `json:"priority"`
		Status      string  `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数错误: " + err.Error(),
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"domain":   input.Domain,
		"category": input.Category,
	}).Info("Creating SDK rule")

	// 设置默认值
	if input.Confidence == 0 {
		input.Confidence = 1.0
	}
	if input.Priority == 0 {
		input.Priority = 50
	}
	if input.Status == "" {
		input.Status = "active"
	}

	// 创建规则对象
	rule := &domain.ThirdPartySDKRule{
		Domain:      input.Domain,
		Category:    input.Category,
		SubCategory: input.SubCategory,
		Provider:    input.Provider,
		Description: input.Description,
		Source:      "manual",
		Confidence:  input.Confidence,
		Status:      input.Status,
		Priority:    input.Priority,
		CreatedBy:   "admin",
		UpdatedBy:   "admin",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 保存到数据库
	if err := h.repo.CreateSDKRule(c.Request.Context(), rule); err != nil {
		h.logger.WithError(err).Error("Failed to create SDK rule")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "SDK 规则创建成功",
		"rule":    rule,
	})
}

// UpdateSDKRule 更新 SDK 规则
// PUT /api/sdk_rules/:id
func (h *SDKHandler) UpdateSDKRule(c *gin.Context) {
	ruleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的规则ID",
		})
		return
	}

	var input struct {
		Category    string  `json:"category"`
		SubCategory string  `json:"sub_category"`
		Provider    string  `json:"provider"`
		Description string  `json:"description"`
		Confidence  float64 `json:"confidence"`
		Priority    int     `json:"priority"`
		Status      string  `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数错误: " + err.Error(),
		})
		return
	}

	h.logger.WithField("rule_id", ruleID).Info("Updating SDK rule")

	// 构建更新字段
	updates := make(map[string]interface{})
	if input.Category != "" {
		updates["category"] = input.Category
	}
	if input.SubCategory != "" {
		updates["sub_category"] = input.SubCategory
	}
	if input.Provider != "" {
		updates["provider"] = input.Provider
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Confidence > 0 {
		updates["confidence"] = input.Confidence
	}
	if input.Priority > 0 {
		updates["priority"] = input.Priority
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	updates["updated_by"] = "admin"
	updates["updated_at"] = time.Now()

	// 更新数据库
	if err := h.repo.UpdateSDKRule(c.Request.Context(), uint(ruleID), updates); err != nil {
		h.logger.WithError(err).Error("Failed to update SDK rule")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "SDK 规则更新成功",
	})
}

// DeleteSDKRule 删除 SDK 规则
// DELETE /api/sdk_rules/:id
func (h *SDKHandler) DeleteSDKRule(c *gin.Context) {
	ruleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的规则ID",
		})
		return
	}

	h.logger.WithField("rule_id", ruleID).Info("Deleting SDK rule")

	// 删除规则
	if err := h.repo.DeleteSDKRule(c.Request.Context(), uint(ruleID)); err != nil {
		h.logger.WithError(err).Error("Failed to delete SDK rule")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "SDK 规则删除成功",
	})
}
