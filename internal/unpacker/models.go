package unpacker

import (
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/packer"
)

// UnpackRequest 脱壳请求
type UnpackRequest struct {
	TaskID      string            // 任务ID
	PackageName string            // 应用包名
	ADBTarget   string            // ADB目标设备
	FridaHost   string            // Frida服务地址（WiFi模式）
	PackerInfo  *packer.PackerInfo // 壳信息
	OutputDir   string            // 输出目录
	Timeout     time.Duration     // 超时时间
}

// UnpackResult 脱壳结果
type UnpackResult struct {
	Success       bool     `json:"success"`
	Status        string   `json:"status"` // success/failed/timeout/skipped
	Method        string   `json:"method"` // frida_dex_dump/frida_class_loader
	DumpedDEXs    []string `json:"dumped_dexs"`
	MergedDEXPath string   `json:"merged_dex_path"`
	DEXCount      int      `json:"dex_count"`
	TotalSize     int64    `json:"total_size"` // 总大小（字节）
	Duration      int64    `json:"duration_ms"`
	Error         string   `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
}

// UnpackStatus 脱壳状态枚举
const (
	UnpackStatusPending  = "pending"  // 等待执行
	UnpackStatusRunning  = "running"  // 执行中
	UnpackStatusSuccess  = "success"  // 成功
	UnpackStatusFailed   = "failed"   // 失败
	UnpackStatusTimeout  = "timeout"  // 超时
	UnpackStatusSkipped  = "skipped"  // 跳过（无需脱壳或不支持）
)

// DEXDumpInfo 单个DEX Dump信息
type DEXDumpInfo struct {
	FileName    string `json:"file_name"`
	FilePath    string `json:"file_path"`
	FileSize    int64  `json:"file_size"`
	DumpSource  string `json:"dump_source"` // dex_class_loader/in_memory/native
	IsValid     bool   `json:"is_valid"`    // DEX文件是否有效
	ClassCount  int    `json:"class_count"` // 类数量（如果解析成功）
}

// UnpackingRecord 脱壳记录（用于数据库存储）
type UnpackingRecord struct {
	ID              int64     `json:"id" gorm:"primaryKey"`
	TaskID          string    `json:"task_id" gorm:"type:varchar(36);not null;uniqueIndex"`
	Status          string    `json:"status" gorm:"type:varchar(50);not null"`
	Method          string    `json:"method" gorm:"type:varchar(50)"`
	DumpedDEXCount  int       `json:"dumped_dex_count" gorm:"default:0"`
	DumpedDEXPaths  string    `json:"dumped_dex_paths" gorm:"type:json"`
	MergedDEXPath   string    `json:"merged_dex_path" gorm:"type:varchar(500)"`
	TotalSize       int64     `json:"total_size" gorm:"default:0"`
	DurationMS      int       `json:"duration_ms"`
	ErrorMessage    string    `json:"error_message" gorm:"type:text"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定表名
func (UnpackingRecord) TableName() string {
	return "task_unpacking_results"
}
