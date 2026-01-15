package domain

import "time"

// TaskFlow 任务流量记录
type TaskFlow struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID       string    `gorm:"type:varchar(36);not null;index:idx_task_id;index:idx_task_host" json:"task_id"`
	URL          string    `gorm:"type:varchar(2048);not null" json:"url"`
	Host         string    `gorm:"type:varchar(255);not null;index:idx_host;index:idx_task_host" json:"host"`
	Port         int       `gorm:"default:443" json:"port"`
	Path         string    `gorm:"type:varchar(1024)" json:"path"`
	Method       string    `gorm:"type:varchar(10);default:'GET'" json:"method"`
	Scheme       string    `gorm:"type:varchar(10);default:'https'" json:"scheme"`
	StatusCode   int       `gorm:"column:status_code" json:"status_code,omitempty"`
	ContentType  string    `gorm:"type:varchar(128)" json:"content_type,omitempty"`
	RequestSize  int       `gorm:"column:request_size" json:"request_size,omitempty"`
	ResponseSize int       `gorm:"column:response_size" json:"response_size,omitempty"`
	Timestamp    float64   `gorm:"type:decimal(16,6);index:idx_task_timestamp" json:"timestamp"`
	Activity     string    `gorm:"type:varchar(255)" json:"activity,omitempty"`
	Source       string    `gorm:"type:varchar(20);default:'mitmproxy'" json:"source"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (TaskFlow) TableName() string {
	return "task_flows"
}

// DynamicURLSummary 动态 URL 汇总结构
type DynamicURLSummary struct {
	URLs              []string          `json:"urls"`
	Domains           map[string]int    `json:"domains"`
	TotalCount        int               `json:"total_count"`
	UniqueURLCount    int               `json:"unique_url_count"`
	UniqueDomainCount int               `json:"unique_domain_count"`
	CapturedAt        string            `json:"captured_at"`
}
