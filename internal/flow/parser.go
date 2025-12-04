package flow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// FlowRecord 流量记录
type FlowRecord struct {
	Timestamp   float64 `json:"ts"`
	Method      string  `json:"method"`
	Scheme      string  `json:"scheme"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Path        string  `json:"path"`
	URL         string  `json:"url"`
	PackageName string  `json:"package_name,omitempty"` // 新增: 支持并发任务隔离
	TaskID      string  `json:"task_id,omitempty"`      // 新增: 任务ID
}

// Parser JSONL 流式解析器
type Parser struct {
	logger *logrus.Logger
}

// NewParser 创建解析器
func NewParser(logger *logrus.Logger) *Parser {
	return &Parser{
		logger: logger,
	}
}

// ParseFile 解析整个文件
func (p *Parser) ParseFile(filePath string) ([]*FlowRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var records []*FlowRecord
	scanner := bufio.NewScanner(file)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		var record FlowRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			p.logger.WithError(err).WithField("line", lineNum).Warn("Failed to parse line")
			continue
		}

		records = append(records, &record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"file":   filePath,
		"total":  lineNum,
		"parsed": len(records),
	}).Info("JSONL file parsed")

	return records, nil
}

// ReadIncremental 增量读取 (从指定行开始)
func (p *Parser) ReadIncremental(filePath string, startLine int) ([]*FlowRecord, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var records []*FlowRecord
	scanner := bufio.NewScanner(file)

	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine <= startLine {
			continue
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var record FlowRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			p.logger.WithError(err).WithField("line", currentLine).Warn("Failed to parse line")
			continue
		}

		records = append(records, &record)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("scanner error: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"file":       filePath,
		"start_line": startLine,
		"end_line":   currentLine,
		"new_count":  len(records),
	}).Debug("Incremental read completed")

	return records, currentLine, nil
}

// CountLines 统计文件行数
func (p *Parser) CountLines(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scanner error: %w", err)
	}

	return count, nil
}

// ExtractDomains 从记录中提取域名
func (p *Parser) ExtractDomains(records []*FlowRecord) []string {
	domainSet := make(map[string]bool)
	for _, record := range records {
		if record.Host != "" {
			domainSet[record.Host] = true
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}

	return domains
}

// FilterByTime 按时间范围过滤
func (p *Parser) FilterByTime(records []*FlowRecord, startTime, endTime float64) []*FlowRecord {
	var filtered []*FlowRecord
	for _, record := range records {
		if record.Timestamp >= startTime && record.Timestamp <= endTime {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

// FilterByPackageName 按包名过滤（用于并发任务隔离）
func (p *Parser) FilterByPackageName(records []*FlowRecord, packageName string) []*FlowRecord {
	var filtered []*FlowRecord
	for _, record := range records {
		if record.PackageName == packageName {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

// FilterByTimeAndPackage 按时间范围和包名过滤（推荐用于并发场景）
func (p *Parser) FilterByTimeAndPackage(records []*FlowRecord, startTime, endTime float64, packageName string) []*FlowRecord {
	var filtered []*FlowRecord
	for _, record := range records {
		// 如果有包名字段，优先使用包名过滤（新版本mitmproxy）
		if record.PackageName != "" {
			if record.PackageName == packageName &&
				record.Timestamp >= startTime &&
				record.Timestamp <= endTime {
				filtered = append(filtered, record)
			}
		} else {
			// 向后兼容：没有包名字段时使用时间范围过滤（旧版本）
			if record.Timestamp >= startTime && record.Timestamp <= endTime {
				filtered = append(filtered, record)
			}
		}
	}
	return filtered
}

// GroupByHost 按 Host 分组
func (p *Parser) GroupByHost(records []*FlowRecord) map[string][]*FlowRecord {
	groups := make(map[string][]*FlowRecord)
	for _, record := range records {
		groups[record.Host] = append(groups[record.Host], record)
	}
	return groups
}
