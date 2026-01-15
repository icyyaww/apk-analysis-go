package packer

// PackerInfo 壳检测结果
type PackerInfo struct {
	IsPacked     bool     `json:"is_packed"`      // 是否加壳
	PackerName   string   `json:"packer_name"`    // 壳名称
	PackerType   string   `json:"packer_type"`    // 壳类型: dex_encrypt/native/vmp
	Confidence   float64  `json:"confidence"`     // 置信度 0-1
	Indicators   []string `json:"indicators"`     // 检测到的特征
	CanUnpack    bool     `json:"can_unpack"`     // 是否支持脱壳
	UnpackMethod string   `json:"unpack_method"`  // 脱壳方法: frida_dex_dump/frida_class_loader/manual
}

// PackerType 壳类型枚举
const (
	PackerTypeNative     = "native"      // 原生库加密
	PackerTypeDexEncrypt = "dex_encrypt" // DEX加密
	PackerTypeVMP        = "vmp"         // 虚拟机保护
	PackerTypeUnknown    = "unknown"     // 未知类型
)

// UnpackMethod 脱壳方法枚举
const (
	UnpackMethodFridaDEXDump     = "frida_dex_dump"     // Frida DEX Dump
	UnpackMethodFridaClassLoader = "frida_class_loader" // Frida ClassLoader Hook
	UnpackMethodManual           = "manual"             // 手动脱壳
	UnpackMethodNone             = "none"               // 不支持脱壳
)

// PackerRule 壳检测规则
type PackerRule struct {
	Name         string           // 壳名称
	Type         string           // 壳类型
	NativeLibs   []string         // 特征Native库
	Strings      []string         // 特征字符串
	ClassNames   []string         // 特征类名
	FileSize     FileSizeRule     // DEX/Native大小异常规则
	CanUnpack    bool             // 是否支持脱壳
	UnpackMethod string           // 脱壳方法
	Priority     int              // 优先级 (越大越优先匹配)
}

// FileSizeRule 文件大小规则
type FileSizeRule struct {
	DEXMaxKB    int64 // DEX最大KB（小于此值可疑）
	NativeMinMB int64 // Native库最小MB（大于此值可疑）
}

// APKPackerStats APK壳相关统计信息
type APKPackerStats struct {
	NativeLibs       []string // 发现的Native库
	DEXSize          int64    // DEX总大小 (bytes)
	NativeSize       int64    // Native库总大小 (bytes)
	DEXCount         int      // DEX文件数量
	HasMultiDex      bool     // 是否为MultiDex
	SuspiciousFiles  []string // 可疑文件
}
