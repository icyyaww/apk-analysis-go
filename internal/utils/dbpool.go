package utils

import (
	"time"

	"gorm.io/gorm"
)

// OptimizeDBPool 优化数据库连接池
func OptimizeDBPool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// 设置最大空闲连接数
	// 根据并发量调整,避免频繁创建销毁连接
	sqlDB.SetMaxIdleConns(10)

	// 设置最大打开连接数
	// 避免数据库连接耗尽
	sqlDB.SetMaxOpenConns(50)

	// 设置连接最大生命周期
	// 防止长时间连接导致的问题
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 设置连接最大空闲时间
	// 自动清理闲置连接
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	return nil
}
