package ai

import (
	"context"
	"fmt"
	"sort"

	"github.com/sirupsen/logrus"
)

// Analyzer AI 分析器 (高层封装)
type Analyzer struct {
	client  *Client
	logger  *logrus.Logger
	enabled bool
}

// NewAnalyzer 创建 AI 分析器
func NewAnalyzer(apiKey string, logger *logrus.Logger) *Analyzer {
	return &Analyzer{
		client:  NewClient(apiKey, logger),
		logger:  logger,
		enabled: apiKey != "",
	}
}

// IsEnabled 检查 AI 是否启用
func (a *Analyzer) IsEnabled() bool {
	return a.enabled
}

// AnalyzeActivityUI 分析 Activity UI 并返回优先级操作
func (a *Analyzer) AnalyzeActivityUI(ctx context.Context, activityName, screenshotPath string) (*ActivityAnalysis, error) {
	if !a.enabled {
		return nil, fmt.Errorf("AI analyzer is not enabled (no API key)")
	}

	a.logger.WithFields(logrus.Fields{
		"activity":   activityName,
		"screenshot": screenshotPath,
	}).Info("Analyzing activity UI with AI")

	// 调用 GLM-4V 分析截图
	result, err := a.client.AnalyzeScreenshot(ctx, screenshotPath)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// 构建高层分析结果
	analysis := &ActivityAnalysis{
		ActivityName: activityName,
		Screenshot:   screenshotPath,
		UIElements: UIElements{
			Buttons:        result.Buttons,
			InputFields:    result.InputFields,
			ClickableItems: result.ClickableItems,
		},
		Actions: a.prioritizeActions(result.SuggestedActions),
	}

	a.logger.WithFields(logrus.Fields{
		"activity":       activityName,
		"buttons":        len(analysis.UIElements.Buttons),
		"input_fields":   len(analysis.UIElements.InputFields),
		"actions":        len(analysis.Actions),
		"high_priority":  a.countHighPriority(analysis.Actions),
	}).Info("AI analysis completed")

	return analysis, nil
}

// prioritizeActions 对操作按优先级排序
func (a *Analyzer) prioritizeActions(actions []SuggestedAction) []PrioritizedAction {
	prioritized := make([]PrioritizedAction, 0, len(actions))

	for _, action := range actions {
		prioritized = append(prioritized, PrioritizedAction{
			Action:   action.Action,
			Target:   action.Target,
			Reason:   action.Reason,
			Priority: action.Priority,
		})
	}

	// 按优先级降序排序 (priority 越大越优先)
	sort.Slice(prioritized, func(i, j int) bool {
		return prioritized[i].Priority > prioritized[j].Priority
	})

	return prioritized
}

// countHighPriority 统计高优先级操作数量
func (a *Analyzer) countHighPriority(actions []PrioritizedAction) int {
	count := 0
	for _, action := range actions {
		if action.Priority >= 7 { // 优先级 >= 7 为高优先级
			count++
		}
	}
	return count
}

// GetTopActions 获取前 N 个高优先级操作
func (a *Analyzer) GetTopActions(analysis *ActivityAnalysis, limit int) []PrioritizedAction {
	if len(analysis.Actions) <= limit {
		return analysis.Actions
	}
	return analysis.Actions[:limit]
}

// ActivityAnalysis Activity 分析结果
type ActivityAnalysis struct {
	ActivityName string              `json:"activity_name"`
	Screenshot   string              `json:"screenshot"`
	UIElements   UIElements          `json:"ui_elements"`
	Actions      []PrioritizedAction `json:"actions"`
}

// UIElements UI 元素汇总
type UIElements struct {
	Buttons        []string        `json:"buttons"`
	InputFields    []string        `json:"input_fields"`
	ClickableItems []ClickableItem `json:"clickable_items"`
}

// PrioritizedAction 优先级操作
type PrioritizedAction struct {
	Action   string `json:"action"`   // click/input/swipe
	Target   string `json:"target"`   // 目标元素描述
	Reason   string `json:"reason"`   // 操作原因
	Priority int    `json:"priority"` // 优先级 1-10
}

// BatchAnalyzeResult 批量分析结果
type BatchAnalyzeResult struct {
	TotalActivities int                          `json:"total_activities"`
	SuccessCount    int                          `json:"success_count"`
	FailureCount    int                          `json:"failure_count"`
	Analyses        map[string]*ActivityAnalysis `json:"analyses"` // activityName -> analysis
}

// BatchAnalyze 批量分析多个 Activity
func (a *Analyzer) BatchAnalyze(ctx context.Context, screenshots map[string]string) *BatchAnalyzeResult {
	result := &BatchAnalyzeResult{
		TotalActivities: len(screenshots),
		Analyses:        make(map[string]*ActivityAnalysis),
	}

	for activityName, screenshotPath := range screenshots {
		analysis, err := a.AnalyzeActivityUI(ctx, activityName, screenshotPath)
		if err != nil {
			a.logger.WithError(err).WithField("activity", activityName).Warn("AI analysis failed for activity")
			result.FailureCount++
			continue
		}

		result.Analyses[activityName] = analysis
		result.SuccessCount++
	}

	a.logger.WithFields(logrus.Fields{
		"total":   result.TotalActivities,
		"success": result.SuccessCount,
		"failure": result.FailureCount,
	}).Info("Batch AI analysis completed")

	return result
}
