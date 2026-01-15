package repository

import (
	"fmt"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/malware"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB 初始化数据库连接
func InitDB(cfg *config.DatabaseConfig, log *logrus.Logger) (*gorm.DB, error) {
	var dialector gorm.Dialector

	if cfg.Type == "mysql" {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
		dialector = mysql.Open(dsn)
	} else {
		// SQLite (fallback)
		dialector = sqlite.Open("./data/tasks.db")
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 关闭 SQL 日志
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: true, // 预编译 SQL
	})

	if err != nil {
		return nil, err
	}

	// 设置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(100)              // 最大连接数
	sqlDB.SetMaxIdleConns(10)               // 最大空闲连接
	sqlDB.SetConnMaxLifetime(time.Hour)     // 连接最长生命周期

	// 自动迁移
	if err := autoMigrate(db, log); err != nil {
		return nil, err
	}

	return db, nil
}

// autoMigrate 自动迁移数据库表结构
func autoMigrate(db *gorm.DB, log *logrus.Logger) error {
	log.Info("Running database migrations...")

	err := db.AutoMigrate(
		&domain.Task{},
		&domain.TaskActivity{},
		&domain.TaskStaticReport{},
		&domain.TaskDomainAnalysis{},
		&domain.TaskAppDomain{},
		&domain.TaskAILog{},
		&domain.ThirdPartySDKRule{},
		&malware.TaskMalwareResult{},
	)

	if err != nil {
		return err
	}

	log.Info("Database migrations completed")
	return nil
}
