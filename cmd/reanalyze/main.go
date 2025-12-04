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

	// 重新分析淘宝任务（验证SDK修复）
	tasks := map[string]string{
		"4fd95c48-dbc7-4876-9b22-162ee44a3a09": "com.taobao.taobao",
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
