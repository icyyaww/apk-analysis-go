package config

import (
	"fmt"
	"os"
	"runtime"

	"github.com/sirupsen/logrus"
)

func InitLogger(cfg *LogConfig) *logrus.Logger {
	logger := logrus.New()

	// 设置日志级别
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// 启用调用者信息（文件名和行号）
	logger.SetReportCaller(true)

	// 设置日志格式
	if cfg.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				// 自定义文件路径显示格式
				filename := fmt.Sprintf("%s:%d", f.File, f.Line)
				return "", filename
			},
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006/01/02 15:04:05",
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				// 自定义文件路径显示格式，类似 GORM 的输出
				filename := fmt.Sprintf("%s:%d", f.File, f.Line)
				return "", filename
			},
		})
	}

	// 输出到标准输出
	logger.SetOutput(os.Stdout)

	return logger
}
