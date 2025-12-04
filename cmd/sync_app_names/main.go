package main

import (
	"context"
	"fmt"
	"log"

	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	dbConfig := &config.DatabaseConfig{
		Type:     "mysql",
		Host:     "rm-cn-x0r388op0000ltuo.rwlb.zhangbei.rds.aliyuncs.com",
		Port:     3306,
		User:     "root",
		Password: "Zr@Fs0522%^",
		DBName:   "apk_analysis_go",
	}

	db, err := repository.InitDB(dbConfig, logger)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	ctx := context.Background()

	// 查询所有静态分析报告
	var reports []domain.TaskStaticReport
	if err := db.Find(&reports).Error; err != nil {
		log.Fatalf("Failed to query static reports: %v", err)
	}

	fmt.Printf("Found %d static reports\n", len(reports))

	updated := 0
	for _, report := range reports {
		if report.AppName == "" {
			continue
		}

		// 更新对应任务的 app_name
		result := db.WithContext(ctx).
			Model(&domain.Task{}).
			Where("id = ?", report.TaskID).
			Update("app_name", report.AppName)

		if result.Error != nil {
			log.Printf("Failed to update task %s: %v", report.TaskID, result.Error)
		} else if result.RowsAffected > 0 {
			fmt.Printf("✅ Updated task %s: app_name = '%s'\n", report.TaskID, report.AppName)
			updated++
		}
	}

	fmt.Printf("\n🎉 Updated %d tasks with app_name\n", updated)
}
