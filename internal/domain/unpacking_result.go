package domain

import "time"

// UnpackingStatus 脱壳状态
type UnpackingStatus string

const (
	UnpackStatusPending UnpackingStatus = "pending"
	UnpackStatusRunning UnpackingStatus = "running"
	UnpackStatusSuccess UnpackingStatus = "success"
	UnpackStatusFailed  UnpackingStatus = "failed"
	UnpackStatusTimeout UnpackingStatus = "timeout"
	UnpackStatusSkipped UnpackingStatus = "skipped"
)

// TaskUnpackingResult 任务脱壳结果表
type TaskUnpackingResult struct {
	ID     uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID string `gorm:"type:varchar(36);uniqueIndex:uk_task_id;not null" json:"task_id"`

	// 脱壳状态
	Status UnpackingStatus `gorm:"type:varchar(50);not null;index:idx_status" json:"status"`
	Method string          `gorm:"type:varchar(50)" json:"method,omitempty"` // frida_dex_dump/frida_class_loader/manual

	// 脱壳结果
	DumpedDexCount int    `gorm:"default:0" json:"dumped_dex_count"`
	DumpedDexPaths string `gorm:"type:text" json:"dumped_dex_paths,omitempty"`   // JSON 数组
	MergedDexPath  string `gorm:"type:varchar(500)" json:"merged_dex_path,omitempty"`
	TotalSize      int64  `gorm:"default:0" json:"total_size"`
	DurationMs     int    `gorm:"type:int" json:"duration_ms,omitempty"`

	// 错误信息
	ErrorMessage string `gorm:"type:text" json:"error_message,omitempty"`

	// 时间戳
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `gorm:"not null;index:idx_created_at" json:"created_at"`
}

func (TaskUnpackingResult) TableName() string {
	return "task_unpacking_results"
}

// PackerInfo 壳检测信息（用于静态报告扩展）
// 已经在 task_static_reports 表中添加了相关字段，这里定义结构体用于JSON解析
type PackerInfo struct {
	IsPacked     bool     `json:"is_packed"`
	PackerName   string   `json:"packer_name,omitempty"`
	PackerType   string   `json:"packer_type,omitempty"`   // native/dex_encrypt/vmp/unknown
	Confidence   float64  `json:"confidence,omitempty"`    // 0.00-1.00
	Indicators   []string `json:"indicators,omitempty"`    // 壳检测特征列表
	CanUnpack    bool     `json:"can_unpack,omitempty"`
	UnpackMethod string   `json:"unpack_method,omitempty"` // 推荐的脱壳方法
}
