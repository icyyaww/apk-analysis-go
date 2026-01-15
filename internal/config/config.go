package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Redis          RedisConfig          `mapstructure:"redis"`
	RabbitMQ       RabbitMQConfig       `mapstructure:"rabbitmq"`
	StaticAnalysis StaticAnalysisConfig `mapstructure:"static_analysis"`
	Malware        MalwareConfig        `mapstructure:"malware"`
	AI             AIConfig             `mapstructure:"ai"`
	ADB            ADBConfig            `mapstructure:"adb"`
	Mitmproxy      MitmproxyConfig      `mapstructure:"mitmproxy"`
	Worker         WorkerConfig         `mapstructure:"worker"`
	Log            LogConfig            `mapstructure:"log"`
	Beian          BeianConfig          `mapstructure:"beian"`
	APKDir         string               `mapstructure:"apk_dir"`
	ResultDir      string               `mapstructure:"result_dir"`
	DataDir        string               `mapstructure:"data_dir"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release
}

type DatabaseConfig struct {
	Type     string `mapstructure:"type"` // mysql, sqlite
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"db_name"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RabbitMQConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	VHost    string `mapstructure:"vhost"`
	Queue    string `mapstructure:"queue"`
}

// StaticAnalysisConfig 静态分析配置
type StaticAnalysisConfig struct {
	EnabledAnalyzers string              `mapstructure:"enabled_analyzers"` // mobsf / hybrid / both
	MobSF            MobSFConfig         `mapstructure:"mobsf"`
	Hybrid           HybridAnalyzerConfig `mapstructure:"hybrid"`
}

// MobSFConfig MobSF 配置
type MobSFConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	URL            string `mapstructure:"url"`
	APIKey         string `mapstructure:"api_key"`
	Timeout        int    `mapstructure:"timeout"`      // seconds - 扫描超时
	HTTPTimeout    int    `mapstructure:"http_timeout"` // seconds - HTTP 客户端超时
}

// HybridAnalyzerConfig 混合分析器配置
type HybridAnalyzerConfig struct {
	Enabled           bool                  `mapstructure:"enabled"`
	PythonPath        string                `mapstructure:"python_path"`
	ScriptPath        string                `mapstructure:"script_path"`
	UseProcessPool    bool                  `mapstructure:"use_process_pool"`
	ProcessPoolSize   int                   `mapstructure:"process_pool_size"`
	ForceDeepAnalysis bool                  `mapstructure:"force_deep_analysis"` // 强制所有APK都进行深度分析
	DeepThreshold     DeepAnalysisThreshold `mapstructure:"deep_analysis_threshold"`
}

// DeepAnalysisThreshold 深度分析决策阈值
type DeepAnalysisThreshold struct {
	FileSizeMB                     int  `mapstructure:"file_size_mb"`
	ActivityCount                  int  `mapstructure:"activity_count"`
	EnableForHighPriorityPackages  bool `mapstructure:"enable_for_high_priority_packages"`
}

// MitmproxyConfig Mitmproxy 配置
type MitmproxyConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type AIConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	APIKey     string `mapstructure:"api_key"`
	Model      string `mapstructure:"model"`
	MaxActions int    `mapstructure:"max_actions"`
}

type ADBConfig struct {
	Target  string `mapstructure:"target"`
	Timeout int    `mapstructure:"timeout"` // seconds
}

type WorkerConfig struct {
	Concurrency int `mapstructure:"concurrency"` // Worker 数量
	QueueSize   int `mapstructure:"queue_size"`  // 任务队列大小
}

type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, text
}

type BeianConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	APIKey     string `mapstructure:"api_key"`
	APIURL     string `mapstructure:"api_url"`
	APIVersion string `mapstructure:"api_version"`
	Timeout    int    `mapstructure:"timeout"` // seconds
}

// MalwareConfig 恶意检测配置
type MalwareConfig struct {
	Enabled                 bool     `mapstructure:"enabled"`
	ServerURL               string   `mapstructure:"server_url"`
	Timeout                 int      `mapstructure:"timeout"`                   // seconds
	Models                  []string `mapstructure:"models"`                    // 使用的模型列表: drebin, mh100k, cicmaldroid
	ExtractGraphFeatures    bool     `mapstructure:"extract_graph_features"`    // 是否提取图特征
	ExtractTemporalFeatures bool     `mapstructure:"extract_temporal_features"` // 是否提取时序特征
	UseEnsemble             bool     `mapstructure:"use_ensemble"`              // 是否使用集成预测
	MaxRetries              int      `mapstructure:"max_retries"`               // 最大重试次数
	RetryDelay              int      `mapstructure:"retry_delay"`               // 重试间隔(秒)
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// 环境变量覆盖（支持嵌套配置）
	viper.AutomaticEnv()

	// 绑定环境变量到嵌套配置路径
	// RabbitMQ
	viper.BindEnv("rabbitmq.host", "RABBITMQ_HOST")
	viper.BindEnv("rabbitmq.port", "RABBITMQ_PORT")
	viper.BindEnv("rabbitmq.user", "RABBITMQ_USER")
	viper.BindEnv("rabbitmq.password", "RABBITMQ_PASS")

	// Redis
	viper.BindEnv("redis.host", "REDIS_HOST")
	viper.BindEnv("redis.port", "REDIS_PORT")

	// Database
	viper.BindEnv("database.host", "MYSQL_HOST")
	viper.BindEnv("database.port", "MYSQL_PORT")
	viper.BindEnv("database.user", "MYSQL_USER")
	viper.BindEnv("database.password", "MYSQL_PASS")
	viper.BindEnv("database.db_name", "MYSQL_DB")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
