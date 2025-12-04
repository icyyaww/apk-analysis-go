package flow

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// AttributionResult 归因结果
type AttributionResult struct {
	Activity      string         `json:"activity"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time"`
	FlowsCollected int           `json:"flows_collected"`
	Flows         []*FlowRecord  `json:"flows"`
}

// Attributor 流量归因器
type Attributor struct {
	parser *Parser
	logger *logrus.Logger
}

// NewAttributor 创建归因器
func NewAttributor(logger *logrus.Logger) *Attributor {
	return &Attributor{
		parser: NewParser(logger),
		logger: logger,
	}
}

// AttributeFlows 基于时间范围归因流量（向后兼容）
func (a *Attributor) AttributeFlows(ctx context.Context, flowsPath string, startTime, endTime time.Time) ([]*FlowRecord, error) {
	// 读取所有流量记录
	allRecords, err := a.parser.ParseFile(flowsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse flows: %w", err)
	}

	// 转换时间为 Unix 时间戳
	startTS := float64(startTime.Unix()) + float64(startTime.Nanosecond())/1e9
	endTS := float64(endTime.Unix()) + float64(endTime.Nanosecond())/1e9

	// 过滤时间范围内的流量
	attributedFlows := a.parser.FilterByTime(allRecords, startTS, endTS)

	a.logger.WithFields(logrus.Fields{
		"total_flows":      len(allRecords),
		"attributed_flows": len(attributedFlows),
		"start_time":       startTime.Format(time.RFC3339),
		"end_time":         endTime.Format(time.RFC3339),
	}).Debug("Flow attribution completed")

	return attributedFlows, nil
}

// AttributeFlowsByPackage 基于时间范围和包名归因流量（推荐用于并发场景）
func (a *Attributor) AttributeFlowsByPackage(ctx context.Context, flowsPath string, startTime, endTime time.Time, packageName string) ([]*FlowRecord, error) {
	// 读取所有流量记录
	allRecords, err := a.parser.ParseFile(flowsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse flows: %w", err)
	}

	// 转换时间为 Unix 时间戳
	startTS := float64(startTime.Unix()) + float64(startTime.Nanosecond())/1e9
	endTS := float64(endTime.Unix()) + float64(endTime.Nanosecond())/1e9

	// 同时过滤时间范围和包名
	attributedFlows := a.parser.FilterByTimeAndPackage(allRecords, startTS, endTS, packageName)

	a.logger.WithFields(logrus.Fields{
		"total_flows":      len(allRecords),
		"attributed_flows": len(attributedFlows),
		"package_name":     packageName,
		"start_time":       startTime.Format(time.RFC3339),
		"end_time":         endTime.Format(time.RFC3339),
	}).Debug("Flow attribution by package completed")

	return attributedFlows, nil
}

// AttributeIncremental 增量归因 (从上次读取位置开始)
func (a *Attributor) AttributeIncremental(ctx context.Context, flowsPath string, lastLineIndex int) ([]*FlowRecord, int, error) {
	return a.parser.ReadIncremental(flowsPath, lastLineIndex)
}

// AttributeToActivities 批量归因到多个 Activity
func (a *Attributor) AttributeToActivities(ctx context.Context, flowsPath string, activities []*ActivityExecution) ([]*AttributionResult, error) {
	results := make([]*AttributionResult, 0, len(activities))

	for _, activity := range activities {
		flows, err := a.AttributeFlows(ctx, flowsPath, activity.StartTime, activity.EndTime)
		if err != nil {
			a.logger.WithError(err).WithField("activity", activity.Name).Warn("Failed to attribute flows")
			continue
		}

		result := &AttributionResult{
			Activity:      activity.Name,
			StartTime:     activity.StartTime,
			EndTime:       activity.EndTime,
			FlowsCollected: len(flows),
			Flows:         flows,
		}

		results = append(results, result)
	}

	return results, nil
}

// ActivityExecution Activity 执行信息
type ActivityExecution struct {
	Name      string
	Component string
	StartTime time.Time
	EndTime   time.Time
}

// GetUniqueHosts 获取所有唯一的主机名
func (a *Attributor) GetUniqueHosts(flows []*FlowRecord) []string {
	return a.parser.ExtractDomains(flows)
}

// GroupFlowsByHost 按主机分组流量
func (a *Attributor) GroupFlowsByHost(flows []*FlowRecord) map[string][]*FlowRecord {
	return a.parser.GroupByHost(flows)
}

// AnalyzeFlowStats 分析流量统计
func (a *Attributor) AnalyzeFlowStats(flows []*FlowRecord) *FlowStats {
	stats := &FlowStats{
		TotalFlows: len(flows),
		Methods:    make(map[string]int),
		Schemes:    make(map[string]int),
		Hosts:      make(map[string]int),
	}

	for _, flow := range flows {
		stats.Methods[flow.Method]++
		stats.Schemes[flow.Scheme]++
		stats.Hosts[flow.Host]++
	}

	stats.UniqueHosts = len(stats.Hosts)
	stats.UniqueDomains = len(a.GetUniqueHosts(flows))

	return stats
}

// FlowStats 流量统计
type FlowStats struct {
	TotalFlows     int            `json:"total_flows"`
	UniqueHosts    int            `json:"unique_hosts"`
	UniqueDomains  int            `json:"unique_domains"`
	Methods        map[string]int `json:"methods"`
	Schemes        map[string]int `json:"schemes"`
	Hosts          map[string]int `json:"hosts"`
}
