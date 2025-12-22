package main

import (
	"context"
	"fmt"
	"log"

	"github.com/apk-analysis/apk-analysis-go/internal/config"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/queue"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// 数据库配置
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

	// RabbitMQ 配置
	mqConfig := &queue.RabbitMQConfig{
		Host:     "localhost",
		Port:     5672,
		User:     "user",
		Password: "pass",
		VHost:    "/",
	}

	mq, err := queue.NewRabbitMQ(mqConfig, "apk_tasks", logger)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer mq.Close()

	producer := queue.NewProducer(mq, logger)

	// 查询所有失败的任务
	var failedTasks []domain.Task
	result := db.Where("status = ?", "failed").Find(&failedTasks)
	if result.Error != nil {
		log.Fatalf("Failed to query failed tasks: %v", result.Error)
	}

	fmt.Printf("找到 %d 个失败任务\n", len(failedTasks))

	// 重置并重新入队
	successCount := 0
	for i, task := range failedTasks {
		// 重置任务状态
		updates := map[string]interface{}{
			"status":                     domain.TaskStatusQueued,
			"failure_type":               "",
			"error_message":              "",
			"current_step":               "重新入队等待执行...",
			"progress_percent":           0,
			"device_connected":           false,
			"started_at":                 nil,
			"completed_at":               nil,
			"static_analysis_completed":  false,
			"dynamic_analysis_completed": false,
			"retry_count":                gorm.Expr("retry_count + 1"),
		}

		if err := db.Model(&domain.Task{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
			log.Printf("❌ Failed to reset task %s: %v", task.ID, err)
			continue
		}

		// 发送到 RabbitMQ
		msg := &queue.TaskMessage{
			TaskID:  task.ID,
			APKName: task.APKName,
			APKPath: fmt.Sprintf("inbound_apks/%s", task.APKName),
		}

		if err := producer.PublishTask(context.Background(), msg); err != nil {
			log.Printf("❌ Failed to publish task %s: %v", task.ID, err)
			continue
		}

		successCount++
		if (i+1)%100 == 0 {
			fmt.Printf("进度: %d/%d\n", i+1, len(failedTasks))
		}
	}

	fmt.Printf("\n✅ 成功重新入队 %d/%d 个任务\n", successCount, len(failedTasks))
}
