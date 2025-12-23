package domain

import "time"

// StaticAnalysisMode 分析模式
type StaticAnalysisMode string

const (
	ModeFast         StaticAnalysisMode = "fast"          // 仅 Go 快速分析
	ModeDeep         StaticAnalysisMode = "deep"          // Go + Python 深度分析
	ModeFastFallback StaticAnalysisMode = "fast_fallback" // Python 失败，降级为 Go
)

// StaticAnalysisStatus 静态分析状态
type StaticAnalysisStatus string

const (
	StaticStatusQueued    StaticAnalysisStatus = "queued"
	StaticStatusAnalyzing StaticAnalysisStatus = "analyzing"
	StaticStatusCompleted StaticAnalysisStatus = "completed"
	StaticStatusFailed    StaticAnalysisStatus = "failed"
)

// TaskStaticReport 静态分析报告表（混合模式）
type TaskStaticReport struct {
	ID     uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID string `gorm:"type:varchar(36);uniqueIndex:uk_task_id;not null" json:"task_id"`

	// 分析器配置
	Analyzer     string             `gorm:"type:varchar(20);default:'hybrid'" json:"analyzer"` // go_only / hybrid / python_only
	AnalysisMode StaticAnalysisMode `gorm:"type:varchar(20)" json:"analysis_mode"`

	// 状态
	Status StaticAnalysisStatus `gorm:"type:varchar(30);default:'queued'" json:"status"`

	// 基础信息（冗余存储，方便查询）
	PackageName  string `gorm:"type:varchar(255);index:idx_package_name" json:"package_name,omitempty"`
	VersionName  string `gorm:"type:varchar(50)" json:"version_name,omitempty"`
	VersionCode  string `gorm:"type:varchar(20)" json:"version_code,omitempty"`
	AppName      string `gorm:"type:varchar(255)" json:"app_name,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	MD5          string `gorm:"type:varchar(32)" json:"md5,omitempty"`
	SHA256       string `gorm:"type:varchar(64)" json:"sha256,omitempty"`

	// 开发者信息（从签名证书提取）
	Developer   string `gorm:"type:varchar(500)" json:"developer,omitempty"`   // 开发者/签名者 (CN)
	CompanyName string `gorm:"type:varchar(500)" json:"company_name,omitempty"` // 公司/组织名称 (O)

	// 组件统计
	ActivityCount   int `gorm:"default:0" json:"activity_count"`
	ServiceCount    int `gorm:"default:0" json:"service_count"`
	ReceiverCount   int `gorm:"default:0" json:"receiver_count"`
	ProviderCount   int `gorm:"default:0" json:"provider_count"`
	PermissionCount int `gorm:"default:0" json:"permission_count"`

	// URL 和域名统计（从深度分析中提取）
	URLCount    int `gorm:"default:0" json:"url_count"`
	DomainCount int `gorm:"default:0" json:"domain_count"`

	// 完整 JSON 数据
	BasicInfoJSON    string `gorm:"type:text" json:"basic_info_json,omitempty"`
	DeepAnalysisJSON string `gorm:"type:mediumtext" json:"deep_analysis_json,omitempty"`

	// 性能指标
	AnalysisDurationMs     int `gorm:"type:int" json:"analysis_duration_ms,omitempty"`
	FastAnalysisDurationMs int `gorm:"type:int" json:"fast_analysis_duration_ms,omitempty"`
	DeepAnalysisDurationMs int `gorm:"type:int" json:"deep_analysis_duration_ms,omitempty"`

	// 决策信息
	NeedsDeepAnalysisReason string `gorm:"type:varchar(255)" json:"needs_deep_analysis_reason,omitempty"`

	// 时间戳
	AnalyzedAt *time.Time `json:"analyzed_at,omitempty"`
	CreatedAt  time.Time  `gorm:"not null" json:"created_at"`
}

func (TaskStaticReport) TableName() string {
	return "task_static_reports"
}
