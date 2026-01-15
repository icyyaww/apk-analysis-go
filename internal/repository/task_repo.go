package repository

import (
	"context"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
	Update(ctx context.Context, task *domain.Task) error
	FindByID(ctx context.Context, id string) (*domain.Task, error)
	List(ctx context.Context, limit int) ([]*domain.Task, error)
	ListWithPagination(ctx context.Context, page int, pageSize int) ([]*domain.Task, int64, error)
	Delete(ctx context.Context, id string) error
	BatchDelete(ctx context.Context, taskIDs []string, status string, beforeDays int) (int64, error)
	UpdateStatus(ctx context.Context, id string, status domain.TaskStatus) error
	UpdateProgress(ctx context.Context, id string, step string, percent int) error
	ShouldStop(ctx context.Context, id string) (bool, error)
	MarkShouldStop(ctx context.Context, id string) error
	SaveActivities(ctx context.Context, activities *domain.TaskActivity) error
	SaveDomainAnalysis(ctx context.Context, domainAnalysis *domain.TaskDomainAnalysis) error
	// 原子更新分析完成标志（解决并发竞态问题）
	MarkStaticAnalysisCompleted(ctx context.Context, id string) error
	MarkDynamicAnalysisCompleted(ctx context.Context, id string) error
	// 获取分析完成状态
	GetAnalysisStatus(ctx context.Context, id string) (staticCompleted, dynamicCompleted bool, err error)
	// 标记任务真正完成（域名分析完成后调用）
	MarkTaskFullyCompleted(ctx context.Context, id string) error
	// 原子更新 app_name（避免被并发操作覆盖）
	UpdateAppName(ctx context.Context, id string, appName string) error
	// 检查是否存在最近创建的同名 APK 任务（防止重复创建）
	HasRecentTaskForAPK(ctx context.Context, apkName string, withinSeconds int) (bool, error)
	// 更新任务失败信息（包含失败类型）
	UpdateFailure(ctx context.Context, id string, failureType domain.FailureType, errorMessage string) error
	// 重试相关方法
	IncrementRetryCount(ctx context.Context, id string) (int, error)
	ResetForRetry(ctx context.Context, id string) error
	GetRetryCount(ctx context.Context, id string) (int, error)
	// 更新应用特征标记
	UpdateLoginRequired(ctx context.Context, id string, loginRequired bool) error
	// 获取各状态任务数量统计（使用数据库聚合查询）
	GetStatusCounts(ctx context.Context) (map[string]int64, int64, error)
	// 获取任务列表（支持排除指定状态）
	ListWithExcludeStatus(ctx context.Context, page int, pageSize int, excludeStatus string) ([]*domain.Task, int64, error)
	// 获取任务列表（支持状态过滤和排除）
	ListWithStatusFilter(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string) ([]*domain.Task, int64, error)
	// 获取任务列表（支持状态过滤、排除和搜索）
	ListWithSearch(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string, search string) ([]*domain.Task, int64, error)
	// 获取所有排队中的任务（不分页）
	ListQueuedTasks(ctx context.Context) ([]*domain.Task, error)
}

type taskRepo struct {
	db     *gorm.DB
	logger *logrus.Logger
}

func NewTaskRepository(db *gorm.DB, logger *logrus.Logger) TaskRepository {
	// Test logger is working
	logger.WithFields(logrus.Fields{
		"component": "task_repository",
		"test":      "logger_injection",
	}).Info("TaskRepository initialized with logger successfully")

	return &taskRepo{
		db:     db,
		logger: logger,
	}
}

func (r *taskRepo) Create(ctx context.Context, task *domain.Task) error {
	task.CreatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *taskRepo) Update(ctx context.Context, task *domain.Task) error {
	// 禁止级联更新关联表,只更新主表 apk_tasks 的字段
	// 这避免了频繁的 task 更新覆盖 MobSFReport 等关联表的数据
	//
	// 🔧 重要修复：
	// 1. 不更新 static_analysis_completed 和 dynamic_analysis_completed（使用原子方法）
	// 2. 不更新 app_name（使用 UpdateAppName 原子方法，避免被动态分析覆盖）
	// 避免并发竞态问题（静态分析和动态分析并行执行时互相覆盖）

	err := r.db.WithContext(ctx).
		Model(task).
		Select("apk_name", "package_name", "status", "should_stop", "error_message",
			"started_at", "completed_at", "current_step", "progress_percent",
			"device_connected", "install_result").
		// 注意：不包含 app_name, static_analysis_completed, dynamic_analysis_completed
		Updates(task).Error

	if err != nil {
		r.logger.WithError(err).WithField("task_id", task.ID).Error("Task update failed")
	}

	return err
}

func (r *taskRepo) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	var task domain.Task
	err := r.db.WithContext(ctx).
		Preload("Activities").
		Preload("StaticReport").
		Preload("DomainAnalysis").
		Preload("AppDomains").
		Preload("AILogs").
		Preload("MalwareResult").
		First(&task, "id = ?", id).Error

	if err != nil {
		return nil, err
	}

	// 调试日志
	r.logger.WithFields(logrus.Fields{
		"task_id":             id,
		"has_static_report":   task.StaticReport != nil,
		"has_domain_analysis": task.DomainAnalysis != nil,
	}).Info("FindByID loaded associations")

	return &task, nil
}

func (r *taskRepo) List(ctx context.Context, limit int) ([]*domain.Task, error) {
	var tasks []*domain.Task
	// 优化: 列表查询只加载必要的轻量级关联数据
	err := r.db.WithContext(ctx).
		Preload("StaticReport", func(db *gorm.DB) *gorm.DB {
			// 静态分析报告：只选择状态和统计信息
			return db.Select("id", "task_id", "status", "url_count", "domain_count", "analysis_mode")
		}).
		Preload("DomainAnalysis", func(db *gorm.DB) *gorm.DB {
			// 只选择备案状态、主域名和IP归属地数据
			return db.Select("id", "task_id", "primary_domain_json", "domain_beian_json", "app_domains_json")
		}).
		Preload("AppDomains", func(db *gorm.DB) *gorm.DB {
			// 加载IP和归属地信息,用于在任务列表中显示
			return db.Select("id", "task_id", "domain", "ip", "province", "city", "isp", "source")
		}).
		Preload("Activities", func(db *gorm.DB) *gorm.DB {
			// 加载Activity详情用于提取动态分析URL中的IP
			return db.Select("task_id", "activity_details_json")
		}).
		Preload("MalwareResult", func(db *gorm.DB) *gorm.DB {
			// 恶意检测结果：只选择状态和核心字段
			return db.Select("id", "task_id", "status", "is_malware", "confidence", "predicted_family")
		}).
		// 不加载大数据量的关联表:
		// - AILogs: 数量可能很多
		Order("created_at DESC").
		Limit(limit).
		Find(&tasks).Error

	return tasks, err
}

func (r *taskRepo) ListWithPagination(ctx context.Context, page int, pageSize int) ([]*domain.Task, int64, error) {
	var tasks []*domain.Task
	var total int64

	// 先统计总数
	if err := r.db.WithContext(ctx).Model(&domain.Task{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 查询当前页数据
	err := r.db.WithContext(ctx).
		Preload("StaticReport", func(db *gorm.DB) *gorm.DB {
			// 静态分析报告：只选择状态和统计信息
			return db.Select("id", "task_id", "status", "url_count", "domain_count", "analysis_mode")
		}).
		Preload("DomainAnalysis", func(db *gorm.DB) *gorm.DB {
			// 只选择备案状态、主域名和IP归属地数据
			return db.Select("id", "task_id", "primary_domain_json", "domain_beian_json", "app_domains_json")
		}).
		Preload("AppDomains", func(db *gorm.DB) *gorm.DB {
			// 加载IP和归属地信息,用于在任务列表中显示
			return db.Select("id", "task_id", "domain", "ip", "province", "city", "isp", "source")
		}).
		Preload("Activities", func(db *gorm.DB) *gorm.DB {
			// 加载Activity详情用于提取动态分析URL中的IP
			return db.Select("task_id", "activity_details_json")
		}).
		Preload("MalwareResult", func(db *gorm.DB) *gorm.DB {
			// 恶意检测结果：只选择状态和核心字段
			return db.Select("id", "task_id", "status", "is_malware", "confidence", "predicted_family")
		}).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

func (r *taskRepo) Delete(ctx context.Context, id string) error {
	// 使用事务和原生 SQL 删除，处理外键约束
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// 删除关联数据（按照外键依赖顺序）
	result := tx.Exec("DELETE FROM task_activities WHERE task_id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	r.logger.WithFields(logrus.Fields{"task_id": id, "deleted": result.RowsAffected}).Info("Deleted task_activities")

	result = tx.Exec("DELETE FROM task_static_reports WHERE task_id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	r.logger.WithFields(logrus.Fields{"task_id": id, "deleted": result.RowsAffected}).Info("Deleted task_static_reports")

	result = tx.Exec("DELETE FROM task_domain_analysis WHERE task_id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	r.logger.WithFields(logrus.Fields{"task_id": id, "deleted": result.RowsAffected}).Info("Deleted task_domain_analysis")

	result = tx.Exec("DELETE FROM task_app_domains WHERE task_id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	r.logger.WithFields(logrus.Fields{"task_id": id, "deleted": result.RowsAffected}).Info("Deleted task_app_domains")

	result = tx.Exec("DELETE FROM task_ai_logs WHERE task_id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	r.logger.WithFields(logrus.Fields{"task_id": id, "deleted": result.RowsAffected}).Info("Deleted task_ai_logs")

	// 删除主表
	result = tx.Exec("DELETE FROM apk_tasks WHERE id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	r.logger.WithFields(logrus.Fields{"task_id": id, "deleted": result.RowsAffected}).Info("Deleted apk_tasks")

	// 提交事务
	return tx.Commit().Error
}

func (r *taskRepo) BatchDelete(ctx context.Context, taskIDs []string, status string, beforeDays int) (int64, error) {
	// 如果指定了任务 ID 列表，则只删除这些任务
	if len(taskIDs) > 0 {
		// 使用事务和原生 SQL 删除，处理外键约束
		tx := r.db.WithContext(ctx).Begin()
		if tx.Error != nil {
			return 0, tx.Error
		}

		// 删除关联数据（按照外键依赖顺序）
		if err := tx.Exec("DELETE FROM task_activities WHERE task_id IN ?", taskIDs).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_static_reports WHERE task_id IN ?", taskIDs).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_domain_analysis WHERE task_id IN ?", taskIDs).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_app_domains WHERE task_id IN ?", taskIDs).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_ai_logs WHERE task_id IN ?", taskIDs).Error; err != nil {
			tx.Rollback()
			return 0, err
		}

		// 删除主表
		result := tx.Exec("DELETE FROM apk_tasks WHERE id IN ?", taskIDs)
		if result.Error != nil {
			tx.Rollback()
			return 0, result.Error
		}

		// 提交事务
		if err := tx.Commit().Error; err != nil {
			return 0, err
		}

		return result.RowsAffected, nil
	}

	// 如果需要删除所有任务（status == "all" 且 beforeDays == 0）
	// 需要先手动删除所有关联数据，然后再删除主表
	if status == "all" && beforeDays == 0 {
		// 开启事务
		tx := r.db.WithContext(ctx).Begin()
		if tx.Error != nil {
			return 0, tx.Error
		}

		// 使用原生 SQL 删除所有关联数据（先删子表，避免外键约束）
		// 临时禁用外键检查
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
			tx.Rollback()
			return 0, err
		}

		// 删除所有关联数据
		if err := tx.Exec("DELETE FROM task_activities").Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_static_reports").Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_domain_analysis").Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_app_domains").Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Exec("DELETE FROM task_ai_logs").Error; err != nil {
			tx.Rollback()
			return 0, err
		}

		// 删除主表
		result := tx.Exec("DELETE FROM apk_tasks")
		if result.Error != nil {
			tx.Rollback()
			return 0, result.Error
		}

		// 重新启用外键检查
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS=1").Error; err != nil {
			tx.Rollback()
			return 0, err
		}

		// 提交事务
		if err := tx.Commit().Error; err != nil {
			return 0, err
		}

		return result.RowsAffected, nil
	}

	// 按状态或时间过滤
	query := r.db.WithContext(ctx).
		Select("Activities", "StaticReport", "DomainAnalysis", "AppDomains", "AILogs") // 级联删除关联数据

	// 按状态过滤
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	// 按时间过滤
	if beforeDays > 0 {
		beforeTime := time.Now().UTC().AddDate(0, 0, -beforeDays)
		query = query.Where("created_at < ?", beforeTime)
	}

	result := query.Delete(&domain.Task{})
	return result.RowsAffected, result.Error
}

func (r *taskRepo) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if status == domain.TaskStatusCompleted || status == domain.TaskStatusFailed || status == domain.TaskStatusCancelled {
		now := time.Now().UTC()
		updates["completed_at"] = &now
	}

	return r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *taskRepo) UpdateProgress(ctx context.Context, id string, step string, percent int) error {
	return r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"current_step":     step,
			"progress_percent": percent,
		}).Error
}

func (r *taskRepo) ShouldStop(ctx context.Context, id string) (bool, error) {
	var task domain.Task
	err := r.db.WithContext(ctx).
		Select("should_stop").
		First(&task, "id = ?", id).Error

	if err != nil {
		return false, err
	}

	return task.ShouldStop, nil
}

func (r *taskRepo) MarkShouldStop(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Update("should_stop", true).Error
}

func (r *taskRepo) SaveActivities(ctx context.Context, activities *domain.TaskActivity) error {
	// 使用 GORM 的 Save 方法,如果记录存在则更新,不存在则插入
	// Save 根据主键判断,这里使用 task_id 作为唯一索引
	// 先尝试查找是否存在
	var existing domain.TaskActivity
	err := r.db.WithContext(ctx).
		Where("task_id = ?", activities.TaskID).
		First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在,直接插入
		return r.db.WithContext(ctx).Create(activities).Error
	} else if err != nil {
		return err
	}

	// 存在,更新记录 (保留主键 ID)
	activities.ID = existing.ID
	return r.db.WithContext(ctx).Save(activities).Error
}

func (r *taskRepo) SaveDomainAnalysis(ctx context.Context, domainAnalysis *domain.TaskDomainAnalysis) error {
	// 使用 GORM 的 Save 方法,如果记录存在则更新,不存在则插入
	// 先尝试查找是否存在
	var existing domain.TaskDomainAnalysis
	err := r.db.WithContext(ctx).
		Where("task_id = ?", domainAnalysis.TaskID).
		First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在,直接插入 (设置创建时间)
		if domainAnalysis.CreatedAt.IsZero() {
			domainAnalysis.CreatedAt = time.Now().UTC()
		}
		return r.db.WithContext(ctx).Create(domainAnalysis).Error
	} else if err != nil {
		return err
	}

	// 存在,更新记录 (保留主键 ID 和创建时间)
	domainAnalysis.ID = existing.ID
	domainAnalysis.CreatedAt = existing.CreatedAt
	return r.db.WithContext(ctx).Save(domainAnalysis).Error
}

// MarkStaticAnalysisCompleted 原子标记静态分析完成
// 使用独立的 SQL UPDATE 语句，避免与其他并发更新冲突
func (r *taskRepo) MarkStaticAnalysisCompleted(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Update("static_analysis_completed", true)

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", id).Error("Failed to mark static analysis completed")
		return result.Error
	}

	r.logger.WithField("task_id", id).Info("✅ Static analysis marked as completed (atomic update)")
	return nil
}

// MarkDynamicAnalysisCompleted 原子标记动态分析完成
// 使用独立的 SQL UPDATE 语句，避免与其他并发更新冲突
func (r *taskRepo) MarkDynamicAnalysisCompleted(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Update("dynamic_analysis_completed", true)

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", id).Error("Failed to mark dynamic analysis completed")
		return result.Error
	}

	r.logger.WithField("task_id", id).Info("✅ Dynamic analysis marked as completed (atomic update)")
	return nil
}

// GetAnalysisStatus 获取分析完成状态（直接从数据库读取最新值）
func (r *taskRepo) GetAnalysisStatus(ctx context.Context, id string) (staticCompleted, dynamicCompleted bool, err error) {
	var task domain.Task
	err = r.db.WithContext(ctx).
		Select("static_analysis_completed", "dynamic_analysis_completed").
		First(&task, "id = ?", id).Error

	if err != nil {
		return false, false, err
	}

	return task.StaticAnalysisCompleted, task.DynamicAnalysisCompleted, nil
}

// MarkTaskFullyCompleted 标记任务完全完成（域名分析完成后调用）
// 设置状态为 completed，进度为 100%，记录完成时间
func (r *taskRepo) MarkTaskFullyCompleted(ctx context.Context, id string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":           domain.TaskStatusCompleted,
			"current_step":     "任务完成",
			"progress_percent": 100,
			"completed_at":     &now,
		})

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", id).Error("Failed to mark task fully completed")
		return result.Error
	}

	r.logger.WithField("task_id", id).Info("✅ Task marked as fully completed (100%)")
	return nil
}

// UpdateAppName 原子更新 app_name（避免被并发操作覆盖）
func (r *taskRepo) UpdateAppName(ctx context.Context, id string, appName string) error {
	if appName == "" {
		return nil // 空值不更新
	}

	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Update("app_name", appName)

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", id).Error("Failed to update app_name")
		return result.Error
	}

	r.logger.WithFields(logrus.Fields{
		"task_id":  id,
		"app_name": appName,
	}).Info("✅ App name updated (atomic)")
	return nil
}

// HasRecentTaskForAPK 检查是否存在最近创建的同名 APK 任务
// 用于防止文件监控器重复创建任务（大文件复制触发多次事件）
// withinSeconds: 时间窗口（秒），默认建议 60 秒
func (r *taskRepo) HasRecentTaskForAPK(ctx context.Context, apkName string, withinSeconds int) (bool, error) {
	var count int64
	cutoffTime := time.Now().UTC().Add(-time.Duration(withinSeconds) * time.Second)

	err := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("apk_name = ? AND created_at > ?", apkName, cutoffTime).
		Count(&count).Error

	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"apk_name":       apkName,
			"within_seconds": withinSeconds,
		}).Error("Failed to check recent task for APK")
		return false, err
	}

	if count > 0 {
		r.logger.WithFields(logrus.Fields{
			"apk_name":       apkName,
			"recent_count":   count,
			"within_seconds": withinSeconds,
		}).Warn("⚠️ Found recent task for same APK, skipping duplicate creation")
	}

	return count > 0, nil
}

// UpdateFailure 更新任务失败信息（包含失败类型和错误消息）
// 同时将任务状态设置为 failed
func (r *taskRepo) UpdateFailure(ctx context.Context, id string, failureType domain.FailureType, errorMessage string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        domain.TaskStatusFailed,
			"failure_type":  failureType,
			"error_message": errorMessage,
			"completed_at":  &now,
		})

	if result.Error != nil {
		r.logger.WithError(result.Error).WithFields(logrus.Fields{
			"task_id":      id,
			"failure_type": failureType,
		}).Error("Failed to update task failure")
		return result.Error
	}

	r.logger.WithFields(logrus.Fields{
		"task_id":          id,
		"failure_type":     failureType,
		"failure_severity": failureType.GetSeverity(),
		"display_name":     failureType.GetDisplayName(),
	}).Warn("❌ Task marked as failed")

	return nil
}

// IncrementRetryCount 增加重试次数并返回新的计数
func (r *taskRepo) IncrementRetryCount(ctx context.Context, id string) (int, error) {
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1"))

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", id).Error("Failed to increment retry count")
		return 0, result.Error
	}

	// 获取更新后的值
	var task domain.Task
	if err := r.db.WithContext(ctx).Select("retry_count").First(&task, "id = ?", id).Error; err != nil {
		return 0, err
	}

	r.logger.WithFields(logrus.Fields{
		"task_id":     id,
		"retry_count": task.RetryCount,
	}).Info("🔄 Retry count incremented")

	return task.RetryCount, nil
}

// ResetForRetry 重置任务状态以准备重试
// 将任务状态改回 queued，清除失败信息，保留重试计数
func (r *taskRepo) ResetForRetry(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":                     domain.TaskStatusQueued,
			"failure_type":               "",
			"error_message":              "",
			"current_step":               "等待重试...",
			"progress_percent":           0,
			"device_connected":           false,
			"started_at":                 nil,
			"completed_at":               nil,
			"static_analysis_completed":  false,
			"dynamic_analysis_completed": false,
		})

	if result.Error != nil {
		r.logger.WithError(result.Error).WithField("task_id", id).Error("Failed to reset task for retry")
		return result.Error
	}

	r.logger.WithField("task_id", id).Info("🔄 Task reset for retry")
	return nil
}

// GetRetryCount 获取当前重试次数
func (r *taskRepo) GetRetryCount(ctx context.Context, id string) (int, error) {
	var task domain.Task
	err := r.db.WithContext(ctx).
		Select("retry_count").
		First(&task, "id = ?", id).Error

	if err != nil {
		return 0, err
	}

	return task.RetryCount, nil
}

// UpdateLoginRequired 更新应用是否需要强制登录的标记
func (r *taskRepo) UpdateLoginRequired(ctx context.Context, id string, loginRequired bool) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Where("id = ?", id).
		Update("login_required", loginRequired)

	if result.Error != nil {
		r.logger.WithError(result.Error).WithFields(logrus.Fields{
			"task_id":        id,
			"login_required": loginRequired,
		}).Error("Failed to update login_required")
		return result.Error
	}

	r.logger.WithFields(logrus.Fields{
		"task_id":        id,
		"login_required": loginRequired,
	}).Info("✅ Login required flag updated")

	return nil
}

// GetStatusCounts 获取各状态任务数量统计（使用数据库聚合查询）
// 返回: statusCounts map, totalCount, error
func (r *taskRepo) GetStatusCounts(ctx context.Context) (map[string]int64, int64, error) {
	type StatusCount struct {
		Status string
		Count  int64
	}

	var results []StatusCount
	err := r.db.WithContext(ctx).
		Model(&domain.Task{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&results).Error

	if err != nil {
		r.logger.WithError(err).Error("Failed to get status counts")
		return nil, 0, err
	}

	// 初始化所有状态计数为 0
	statusCounts := map[string]int64{
		"queued":     0,
		"installing": 0,
		"running":    0,
		"collecting": 0,
		"completed":  0,
		"failed":     0,
		"cancelled":  0,
	}

	var total int64
	for _, r := range results {
		statusCounts[r.Status] = r.Count
		total += r.Count
	}

	return statusCounts, total, nil
}

// ListWithExcludeStatus 获取任务列表（支持排除指定状态）
// 在数据库层面直接排除指定状态，避免内存过滤导致数据不足
func (r *taskRepo) ListWithExcludeStatus(ctx context.Context, page int, pageSize int, excludeStatus string) ([]*domain.Task, int64, error) {
	var tasks []*domain.Task
	var total int64

	// 构建基础查询
	baseQuery := r.db.WithContext(ctx).Model(&domain.Task{})
	if excludeStatus != "" {
		baseQuery = baseQuery.Where("status != ?", excludeStatus)
	}

	// 统计符合条件的总数
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 查询当前页数据（按状态优先级排序：running > installing > collecting > completed > failed > queued）
	query := r.db.WithContext(ctx)
	if excludeStatus != "" {
		query = query.Where("status != ?", excludeStatus)
	}

	err := query.
		Preload("StaticReport", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "status", "url_count", "domain_count", "analysis_mode")
		}).
		Preload("DomainAnalysis", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "primary_domain_json", "domain_beian_json", "app_domains_json")
		}).
		Preload("AppDomains", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "domain", "ip", "province", "city", "isp", "source")
		}).
		Preload("Activities", func(db *gorm.DB) *gorm.DB {
			return db.Select("task_id", "activity_details_json")
		}).
		Preload("MalwareResult", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "status", "is_malware", "confidence", "predicted_family")
		}).
		// 按状态优先级排序，然后按完成时间倒序（最新完成的在前）
		Order("CASE status WHEN 'running' THEN 1 WHEN 'installing' THEN 2 WHEN 'collecting' THEN 3 WHEN 'completed' THEN 4 WHEN 'failed' THEN 5 ELSE 6 END, completed_at DESC, created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// ListWithStatusFilter 获取任务列表（支持状态过滤和排除）
// statusFilter: 只返回指定状态的任务（如 "failed"）
// excludeStatus: 排除指定状态的任务（如 "queued"）
func (r *taskRepo) ListWithStatusFilter(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string) ([]*domain.Task, int64, error) {
	var tasks []*domain.Task
	var total int64

	// 构建基础查询
	baseQuery := r.db.WithContext(ctx).Model(&domain.Task{})
	if excludeStatus != "" {
		baseQuery = baseQuery.Where("status != ?", excludeStatus)
	}
	if statusFilter != "" {
		baseQuery = baseQuery.Where("status = ?", statusFilter)
	}

	// 统计符合条件的总数
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 查询当前页数据
	query := r.db.WithContext(ctx)
	if excludeStatus != "" {
		query = query.Where("status != ?", excludeStatus)
	}
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	err := query.
		Preload("StaticReport", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "status", "url_count", "domain_count", "analysis_mode")
		}).
		Preload("DomainAnalysis", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "primary_domain_json", "domain_beian_json", "app_domains_json")
		}).
		Preload("AppDomains", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "domain", "ip", "province", "city", "isp", "source")
		}).
		Preload("Activities", func(db *gorm.DB) *gorm.DB {
			return db.Select("task_id", "activity_details_json")
		}).
		Preload("MalwareResult", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "status", "is_malware", "confidence", "predicted_family")
		}).
		// 按完成时间倒序（最新的在前）
		Order("completed_at DESC, created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// ListWithSearch 获取任务列表（支持状态过滤、排除和搜索）
// search: 搜索APK名称、应用名称、包名（模糊匹配）
func (r *taskRepo) ListWithSearch(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string, search string) ([]*domain.Task, int64, error) {
	var tasks []*domain.Task
	var total int64

	// 构建基础查询
	baseQuery := r.db.WithContext(ctx).Model(&domain.Task{})
	if excludeStatus != "" {
		baseQuery = baseQuery.Where("status != ?", excludeStatus)
	}
	if statusFilter != "" {
		baseQuery = baseQuery.Where("status = ?", statusFilter)
	}
	// 添加搜索条件
	if search != "" {
		searchPattern := "%" + search + "%"
		baseQuery = baseQuery.Where("apk_name LIKE ? OR app_name LIKE ? OR package_name LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	// 统计符合条件的总数
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 查询当前页数据
	query := r.db.WithContext(ctx)
	if excludeStatus != "" {
		query = query.Where("status != ?", excludeStatus)
	}
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	// 添加搜索条件
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("apk_name LIKE ? OR app_name LIKE ? OR package_name LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	err := query.
		Preload("StaticReport", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "status", "url_count", "domain_count", "analysis_mode")
		}).
		Preload("DomainAnalysis", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "primary_domain_json", "domain_beian_json", "app_domains_json")
		}).
		Preload("AppDomains", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "domain", "ip", "province", "city", "isp", "source")
		}).
		Preload("Activities", func(db *gorm.DB) *gorm.DB {
			return db.Select("task_id", "activity_details_json")
		}).
		Preload("MalwareResult", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "task_id", "status", "is_malware", "confidence", "predicted_family")
		}).
		// 按完成时间倒序（最新的在前）
		Order("completed_at DESC, created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// ListQueuedTasks 获取所有排队中的任务（不分页）
func (r *taskRepo) ListQueuedTasks(ctx context.Context) ([]*domain.Task, error) {
	var tasks []*domain.Task

	err := r.db.WithContext(ctx).
		Where("status = ?", "queued").
		Order("created_at ASC"). // 按创建时间升序，先进先出
		Find(&tasks).Error

	return tasks, err
}
