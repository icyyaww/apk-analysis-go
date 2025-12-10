package domain

import (
	"time"
)

type TaskStatus string

const (
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusInstalling TaskStatus = "installing"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusCollecting TaskStatus = "collecting"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// FailureType 失败类型
type FailureType string

const (
	FailureTypeNone            FailureType = ""                  // 无失败（成功或进行中）
	FailureTypeDeviceTimeout   FailureType = "device_timeout"    // 设备等待超时（正常-设备繁忙）
	FailureTypeARMDeviceOnly   FailureType = "arm_device_only"   // 需要ARM真机但不可用（正常-资源限制）
	FailureTypeInstallFailed   FailureType = "install_failed"    // APK安装失败（警告-APK问题）
	FailureTypeConnectionError FailureType = "connection_error"  // 设备连接错误（异常-系统问题）
	FailureTypeFridaError      FailureType = "frida_error"       // Frida注入失败（异常-环境问题）
	FailureTypeProxyError      FailureType = "proxy_error"       // 代理配置失败（异常-网络问题）
	FailureTypeAnalysisError   FailureType = "analysis_error"    // 分析过程错误（异常-程序问题）
	FailureTypeTimeout         FailureType = "timeout"           // 任务执行超时（警告）
	FailureTypeUnknown         FailureType = "unknown"           // 未知错误（异常）
)

// FailureSeverity 失败严重程度
type FailureSeverity string

const (
	FailureSeverityNormal  FailureSeverity = "normal"  // 正常（资源限制，可重试）
	FailureSeverityWarning FailureSeverity = "warning" // 警告（需要关注）
	FailureSeverityError   FailureSeverity = "error"   // 错误（需要排查）
)

// GetFailureSeverity 获取失败类型对应的严重程度
func (ft FailureType) GetSeverity() FailureSeverity {
	switch ft {
	case FailureTypeNone:
		return FailureSeverityNormal
	case FailureTypeDeviceTimeout, FailureTypeARMDeviceOnly:
		return FailureSeverityNormal // 设备资源问题，正常
	case FailureTypeInstallFailed, FailureTypeTimeout:
		return FailureSeverityWarning // APK或超时问题，需关注
	case FailureTypeConnectionError, FailureTypeFridaError, FailureTypeProxyError, FailureTypeAnalysisError, FailureTypeUnknown:
		return FailureSeverityError // 系统问题，需排查
	default:
		return FailureSeverityError
	}
}

// GetDisplayName 获取失败类型的中文显示名称
func (ft FailureType) GetDisplayName() string {
	switch ft {
	case FailureTypeNone:
		return ""
	case FailureTypeDeviceTimeout:
		return "设备繁忙"
	case FailureTypeARMDeviceOnly:
		return "需要ARM真机"
	case FailureTypeInstallFailed:
		return "安装失败"
	case FailureTypeConnectionError:
		return "连接错误"
	case FailureTypeFridaError:
		return "注入失败"
	case FailureTypeProxyError:
		return "代理错误"
	case FailureTypeAnalysisError:
		return "分析错误"
	case FailureTypeTimeout:
		return "执行超时"
	case FailureTypeUnknown:
		return "未知错误"
	default:
		return "未知错误"
	}
}

// GetMaxRetryCount 获取失败类型对应的最大重试次数
// 返回 0 表示不重试
func (ft FailureType) GetMaxRetryCount() int {
	switch ft {
	case FailureTypeNone:
		return 0 // 成功不需要重试
	case FailureTypeARMDeviceOnly:
		return 0 // ARM设备限制，重试无意义
	case FailureTypeDeviceTimeout, FailureTypeConnectionError, FailureTypeFridaError, FailureTypeProxyError, FailureTypeTimeout:
		return 3 // 环境问题，可重试3次
	case FailureTypeInstallFailed, FailureTypeAnalysisError, FailureTypeUnknown:
		return 1 // APK问题或未知错误，重试1次
	default:
		return 1
	}
}

// CanRetry 检查失败类型是否可以重试
func (ft FailureType) CanRetry() bool {
	return ft.GetMaxRetryCount() > 0
}

// Task 主任务表
type Task struct {
	ID              string      `gorm:"primaryKey;type:varchar(36)" json:"id"`
	APKName         string      `gorm:"type:varchar(255);not null" json:"apk_name"`
	AppName         string      `gorm:"type:varchar(255)" json:"app_name,omitempty"`
	PackageName     string      `gorm:"type:varchar(255)" json:"package_name,omitempty"`
	Status          TaskStatus  `gorm:"type:varchar(20);not null;default:'queued'" json:"status"`
	ShouldStop      bool        `gorm:"default:false" json:"should_stop"`
	FailureType     FailureType `gorm:"type:varchar(30);default:''" json:"failure_type,omitempty"`
	ErrorMessage    string      `gorm:"type:text" json:"error_message,omitempty"`
	RetryCount      int         `gorm:"type:tinyint;default:0" json:"retry_count"`
	CreatedAt       time.Time  `gorm:"not null" json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CurrentStep     string     `gorm:"type:varchar(255)" json:"current_step,omitempty"`
	ProgressPercent int        `gorm:"type:tinyint;default:0" json:"progress_percent"`
	DeviceConnected bool       `gorm:"default:false" json:"device_connected"`
	InstallResult   string     `gorm:"type:text" json:"install_result,omitempty"`

	// 任务完成追踪 (用于域名分析触发)
	StaticAnalysisCompleted  bool `gorm:"default:false" json:"static_analysis_completed"`
	DynamicAnalysisCompleted bool `gorm:"default:false" json:"dynamic_analysis_completed"`

	// 关联 (使用指针避免循环依赖)
	Activities     *TaskActivity       `gorm:"foreignKey:TaskID;references:ID" json:"activities,omitempty"`
	StaticReport   *TaskStaticReport   `gorm:"foreignKey:TaskID;references:ID" json:"static_report,omitempty"`
	DomainAnalysis *TaskDomainAnalysis `gorm:"foreignKey:TaskID;references:ID" json:"domain_analysis,omitempty"`
	AppDomains     []TaskAppDomain     `gorm:"foreignKey:TaskID;references:ID" json:"app_domains,omitempty"`
	AILogs         *TaskAILog          `gorm:"foreignKey:TaskID;references:ID" json:"ai_logs,omitempty"`
}

func (Task) TableName() string {
	return "apk_tasks"
}

// TaskActivity Activity 信息表
type TaskActivity struct {
	ID                  uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID              string    `gorm:"type:varchar(36);uniqueIndex:uk_task_id;not null" json:"task_id"`
	LauncherActivity    string    `gorm:"type:varchar(500)" json:"launcher_activity,omitempty"`
	ActivitiesJSON      string    `gorm:"type:text" json:"activities_json,omitempty"`
	ActivityDetailsJSON string    `gorm:"type:mediumtext" json:"activity_details_json,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

func (TaskActivity) TableName() string {
	return "task_activities"
}

// TaskDomainAnalysis 域名分析表
type TaskDomainAnalysis struct {
	ID                 uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID             string     `gorm:"type:varchar(36);uniqueIndex:uk_task_id;not null" json:"task_id"`
	PrimaryDomain      string     `gorm:"type:varchar(255)" json:"primary_domain,omitempty"`
	PrimaryDomainJSON  string     `gorm:"type:longtext" json:"primary_domain_json,omitempty"`
	DomainBeianStatus  string     `gorm:"type:varchar(50)" json:"domain_beian_status,omitempty"`
	DomainBeianJSON    string     `gorm:"type:longtext" json:"domain_beian_json,omitempty"`
	AppDomainsJSON     string     `gorm:"type:longtext" json:"app_domains_json,omitempty"`
	URLAnalysisStatic  string     `gorm:"type:mediumtext" json:"url_analysis_static,omitempty"`
	URLAnalysisDynamic string     `gorm:"type:mediumtext" json:"url_analysis_dynamic,omitempty"`
	AnalyzedAt         *time.Time `json:"analyzed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

func (TaskDomainAnalysis) TableName() string {
	return "task_domain_analysis"
}

// TaskAppDomain APP 域名归属地表
type TaskAppDomain struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    string    `gorm:"type:varchar(36);index:idx_task_id;not null" json:"task_id"`
	Domain    string    `gorm:"type:varchar(255);index:idx_domain;not null" json:"domain"`
	IP        string    `gorm:"type:varchar(45)" json:"ip,omitempty"`
	Province  string    `gorm:"type:varchar(50)" json:"province,omitempty"`
	City      string    `gorm:"type:varchar(50)" json:"city,omitempty"`
	ISP       string    `gorm:"type:varchar(50)" json:"isp,omitempty"`
	Source    string    `gorm:"type:varchar(50)" json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (TaskAppDomain) TableName() string {
	return "task_app_domains"
}

// TaskAILog AI 交互日志表
type TaskAILog struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID          string    `gorm:"type:varchar(36);uniqueIndex:uk_task_id;not null" json:"task_id"`
	InteractionJSON string    `gorm:"type:mediumtext" json:"interaction_json,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func (TaskAILog) TableName() string {
	return "task_ai_logs"
}

// ThirdPartySDKRule 第三方 SDK 规则表
type ThirdPartySDKRule struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Domain           string    `gorm:"type:varchar(255);uniqueIndex:uk_domain;not null" json:"domain"`
	Category         string    `gorm:"type:varchar(100);index:idx_category" json:"category,omitempty"`
	SubCategory      string    `gorm:"type:varchar(50)" json:"sub_category,omitempty"`
	Provider         string    `gorm:"type:varchar(255)" json:"provider,omitempty"`
	Description      string    `gorm:"type:text" json:"description,omitempty"`
	Source           string    `gorm:"type:varchar(20);default:'builtin'" json:"source"` // builtin, discovered, manual
	Confidence       float64   `gorm:"type:decimal(3,2);default:1.00" json:"confidence"`
	Status           string    `gorm:"type:varchar(20);default:'active';index:idx_status" json:"status"` // active, pending, disabled
	DiscoverCount    int       `gorm:"default:0" json:"discover_count"`
	FirstSeenTaskID  string    `gorm:"type:varchar(36)" json:"first_seen_task_id,omitempty"`
	Priority         int       `gorm:"type:tinyint;default:50" json:"priority"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        string    `gorm:"type:varchar(100);default:'system'" json:"created_by"`
	UpdatedBy        string    `gorm:"type:varchar(100);default:'system'" json:"updated_by"`
}

func (ThirdPartySDKRule) TableName() string {
	return "third_party_sdk_rules"
}
