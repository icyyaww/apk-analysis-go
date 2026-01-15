package staticanalysis

import (
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/packer"
)

// BasicInfo Go 快速分析提取的基础信息
type BasicInfo struct {
	// 文件信息
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	MD5      string `json:"md5"`
	SHA256   string `json:"sha256"`

	// 应用基本信息
	PackageName string `json:"package_name"`
	VersionName string `json:"version_name"`
	VersionCode string `json:"version_code"`
	AppName     string `json:"app_name"`

	// SDK 版本
	MinSDK    string `json:"min_sdk"`
	TargetSDK string `json:"target_sdk"`

	// 组件列表
	Activities []string `json:"activities"`
	Services   []string `json:"services"`
	Receivers  []string `json:"receivers"`
	Providers  []string `json:"providers"`

	// 权限列表
	Permissions []string `json:"permissions"`

	// 统计信息
	ActivityCount   int `json:"activity_count"`
	ServiceCount    int `json:"service_count"`
	ReceiverCount   int `json:"receiver_count"`
	ProviderCount   int `json:"provider_count"`
	PermissionCount int `json:"permission_count"`
}

// DeepAnalysisResult Python Androguard 深度分析结果
type DeepAnalysisResult struct {
	// URL 和域名
	URLs    []string `json:"urls"`
	Domains []string `json:"domains"`

	// 敏感字符串
	Strings []string `json:"strings,omitempty"`

	// Native 库
	NativeLibs []string `json:"native_libs,omitempty"`

	// 证书信息
	Certificates map[string]interface{} `json:"certificates,omitempty"`

	// 敏感 API 调用
	APICalls []map[string]string `json:"api_calls,omitempty"`

	// 基本信息（从 Python Androguard 提取）
	BasicInfo *DeepBasicInfo `json:"basic_info,omitempty"`
}

// DeepBasicInfo Python Androguard 提取的基本信息
type DeepBasicInfo struct {
	PackageName  string `json:"package_name"`
	VersionName  string `json:"version_name"`
	VersionCode  string `json:"version_code"`
	AppName      string `json:"app_name"`
	MinSDK       string `json:"min_sdk"`
	TargetSDK    string `json:"target_sdk"`
	MainActivity string `json:"main_activity"`
}

// AnalysisResult 完整的分析结果
type AnalysisResult struct {
	// 基础信息（Go 快速分析）
	BasicInfo *BasicInfo `json:"basic_info"`

	// 深度分析结果（Python，可选）
	DeepAnalysis *DeepAnalysisResult `json:"deep_analysis,omitempty"`

	// 壳检测结果
	PackerInfo            *packer.PackerInfo `json:"packer_info,omitempty"`
	NeedsDynamicUnpacking bool               `json:"needs_dynamic_unpacking,omitempty"`

	// 元数据
	AnalysisMode            string    `json:"analysis_mode"` // "fast" / "deep" / "fast_fallback"
	AnalysisDuration        int64     `json:"analysis_duration_ms"`
	FastAnalysisDuration    int64     `json:"fast_analysis_duration_ms,omitempty"`
	DeepAnalysisDuration    int64     `json:"deep_analysis_duration_ms,omitempty"`
	PackerDetectionDuration int64     `json:"packer_detection_duration_ms,omitempty"`
	AnalyzedAt              time.Time `json:"analyzed_at"`
	NeedsDeepAnalysisReason string    `json:"needs_deep_analysis_reason,omitempty"`
}

// AndroidManifest 解析后的 Manifest 结构
type AndroidManifest struct {
	Package     string `json:"package"`
	VersionName string `json:"version_name"`
	VersionCode string `json:"version_code"`

	UsesSdk struct {
		MinSdkVersion    string `json:"min_sdk_version"`
		TargetSdkVersion string `json:"target_sdk_version"`
	} `json:"uses_sdk"`

	Application struct {
		Label      string     `json:"label"`
		Activities []Activity `json:"activities"`
		Services   []Service  `json:"services"`
		Receivers  []Receiver `json:"receivers"`
		Providers  []Provider `json:"providers"`
	} `json:"application"`

	UsesPermissions []UsesPermission `json:"uses_permissions"`
}

// Activity 组件
type Activity struct {
	Name string `json:"name"`
}

// Service 组件
type Service struct {
	Name string `json:"name"`
}

// Receiver 组件
type Receiver struct {
	Name string `json:"name"`
}

// Provider 组件
type Provider struct {
	Name string `json:"name"`
}

// UsesPermission 权限
type UsesPermission struct {
	Name string `json:"name"`
}
