package main

import (
	"context"
	"fmt"
	"log"

	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/domainanalysis"
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

	taskRepo := repository.NewTaskRepository(db, logger)
	analysisService := domainanalysis.NewAnalysisService(db, taskRepo, logger)

	// 重新分析京东金融任务（修复 primary_domain_json 字段问题 - LONGTEXT）
	tasks := map[string]string{
		"eb72b320-d357-4a3c-8201-e0d06e759dd7": "com.jd.jrapp",
	}

	for taskID, pkg := range tasks {
		fmt.Printf("\n🔄 Re-analyzing task: %s (%s)\n", taskID, pkg)
		if err := analysisService.AnalyzeTask(context.Background(), taskID); err != nil {
			log.Printf("❌ Failed to analyze task %s: %v", pkg, err)
		} else {
			fmt.Printf("✅ Analysis completed for %s\n", pkg)
		}
	}

	fmt.Println("\n🎉 All tasks reanalyzed!")
}
