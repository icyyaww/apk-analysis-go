package service

import (
	"context"
	"fmt"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// TaskService 任务服务接口
type TaskService interface {
	// 创建任务
	CreateTask(ctx context.Context, apkName string) (*domain.Task, error)

	// 获取任务
	GetTask(ctx context.Context, taskID string) (*domain.Task, error)

	// 获取任务列表
	ListTasks(ctx context.Context, limit int) ([]*domain.Task, error)

	// 获取任务列表（分页）
	ListTasksWithPagination(ctx context.Context, page int, pageSize int) ([]*domain.Task, int64, error)

	// 删除任务
	DeleteTask(ctx context.Context, taskID string) error

	// 停止任务
	StopTask(ctx context.Context, taskID string) error

	// 批量删除任务
	BatchDeleteTasks(ctx context.Context, taskIDs []string, status string, beforeDays int) (int64, error)

	// 更新任务状态
	UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error

	// 更新任务进度
	UpdateTaskProgress(ctx context.Context, taskID string, step string, percent int) error

	// 获取任务状态统计（使用数据库聚合查询）
	GetStatusCounts(ctx context.Context) (map[string]int64, int64, error)

	// 获取任务列表（支持排除指定状态）
	ListTasksWithExcludeStatus(ctx context.Context, page int, pageSize int, excludeStatus string) ([]*domain.Task, int64, error)

	// 获取任务列表（支持状态过滤和排除）
	ListTasksWithStatusFilter(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string) ([]*domain.Task, int64, error)

	// 获取任务列表（支持状态过滤、排除和搜索）
	ListTasksWithSearch(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string, search string) ([]*domain.Task, int64, error)

	// 获取任务列表（支持状态过滤、排除、搜索和高级筛选）
	ListTasksWithAdvancedFilters(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string, search string, province string, isp string, beianStatus string) ([]*domain.Task, int64, error)

	// 获取任务列表（支持所有筛选条件，包括完成时间和置信度）
	ListTasksWithFilterOptions(ctx context.Context, page int, pageSize int, opts *repository.TaskFilterOptions) ([]*domain.Task, int64, error)

	// 获取所有排队中的任务（不分页）
	ListQueuedTasks(ctx context.Context) ([]*domain.Task, error)
}

type taskService struct {
	taskRepo repository.TaskRepository
	logger   *logrus.Logger
}

// NewTaskService 创建任务服务实例
func NewTaskService(taskRepo repository.TaskRepository, logger *logrus.Logger) TaskService {
	return &taskService{
		taskRepo: taskRepo,
		logger:   logger,
	}
}

func (s *taskService) CreateTask(ctx context.Context, apkName string) (*domain.Task, error) {
	// 🔧 防重复：检查是否存在最近创建的同名 APK 任务
	// 解决大文件复制时文件监控器触发多次事件导致重复创建任务的问题
	hasRecent, err := s.taskRepo.HasRecentTaskForAPK(ctx, apkName, 60) // 60秒时间窗口
	if err != nil {
		s.logger.WithError(err).WithField("apk_name", apkName).Warn("Failed to check recent task, continuing anyway")
	} else if hasRecent {
		s.logger.WithField("apk_name", apkName).Warn("⚠️ Duplicate task creation blocked: recent task exists for same APK")
		return nil, fmt.Errorf("任务已存在：最近60秒内已为该APK创建任务")
	}

	task := &domain.Task{
		ID:              uuid.New().String(),
		APKName:         apkName,
		Status:          domain.TaskStatusQueued,
		CreatedAt:       time.Now().UTC(),
		ProgressPercent: 0,
		CurrentStep:     "任务已创建",
		ShouldStop:      false,
	}

	if err := s.taskRepo.Create(ctx, task); err != nil {
		s.logger.WithError(err).Error("Failed to create task")
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}

	s.logger.WithField("task_id", task.ID).Info("Task created successfully")
	return task, nil
}

func (s *taskService) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get task")
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}
	return task, nil
}

func (s *taskService) ListTasks(ctx context.Context, limit int) ([]*domain.Task, error) {
	tasks, err := s.taskRepo.List(ctx, limit)
	if err != nil {
		s.logger.WithError(err).Error("Failed to list tasks")
		return nil, fmt.Errorf("获取任务列表失败: %w", err)
	}
	return tasks, nil
}

func (s *taskService) ListTasksWithPagination(ctx context.Context, page int, pageSize int) ([]*domain.Task, int64, error) {
	tasks, total, err := s.taskRepo.ListWithPagination(ctx, page, pageSize)
	if err != nil {
		s.logger.WithError(err).Error("Failed to list tasks with pagination")
		return nil, 0, fmt.Errorf("获取任务列表失败: %w", err)
	}
	return tasks, total, nil
}

func (s *taskService) DeleteTask(ctx context.Context, taskID string) error {
	if err := s.taskRepo.Delete(ctx, taskID); err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("Failed to delete task")
		return fmt.Errorf("删除任务失败: %w", err)
	}

	s.logger.WithField("task_id", taskID).Info("Task deleted successfully")
	return nil
}

func (s *taskService) StopTask(ctx context.Context, taskID string) error {
	if err := s.taskRepo.MarkShouldStop(ctx, taskID); err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("Failed to stop task")
		return fmt.Errorf("停止任务失败: %w", err)
	}

	s.logger.WithField("task_id", taskID).Info("Task marked for stopping")
	return nil
}

func (s *taskService) UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error {
	if err := s.taskRepo.UpdateStatus(ctx, taskID, status); err != nil {
		s.logger.WithError(err).
			WithField("task_id", taskID).
			WithField("status", status).
			Error("Failed to update task status")
		return fmt.Errorf("更新任务状态失败: %w", err)
	}
	return nil
}

func (s *taskService) BatchDeleteTasks(ctx context.Context, taskIDs []string, status string, beforeDays int) (int64, error) {
	deletedCount, err := s.taskRepo.BatchDelete(ctx, taskIDs, status, beforeDays)
	if err != nil {
		s.logger.WithError(err).
			WithFields(logrus.Fields{
				"task_ids":    taskIDs,
				"status":      status,
				"before_days": beforeDays,
			}).
			Error("Failed to batch delete tasks")
		return 0, fmt.Errorf("批量删除任务失败: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"deleted_count": deletedCount,
		"status":        status,
		"before_days":   beforeDays,
	}).Info("Tasks batch deleted successfully")

	return deletedCount, nil
}

func (s *taskService) UpdateTaskProgress(ctx context.Context, taskID string, step string, percent int) error {
	if err := s.taskRepo.UpdateProgress(ctx, taskID, step, percent); err != nil {
		s.logger.WithError(err).
			WithField("task_id", taskID).
			WithField("step", step).
			WithField("percent", percent).
			Error("Failed to update task progress")
		return fmt.Errorf("更新任务进度失败: %w", err)
	}
	return nil
}

func (s *taskService) GetStatusCounts(ctx context.Context) (map[string]int64, int64, error) {
	return s.taskRepo.GetStatusCounts(ctx)
}

func (s *taskService) ListTasksWithExcludeStatus(ctx context.Context, page int, pageSize int, excludeStatus string) ([]*domain.Task, int64, error) {
	return s.taskRepo.ListWithExcludeStatus(ctx, page, pageSize, excludeStatus)
}

func (s *taskService) ListTasksWithStatusFilter(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string) ([]*domain.Task, int64, error) {
	return s.taskRepo.ListWithStatusFilter(ctx, page, pageSize, excludeStatus, statusFilter)
}

func (s *taskService) ListTasksWithSearch(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string, search string) ([]*domain.Task, int64, error) {
	return s.taskRepo.ListWithSearch(ctx, page, pageSize, excludeStatus, statusFilter, search)
}

func (s *taskService) ListTasksWithAdvancedFilters(ctx context.Context, page int, pageSize int, excludeStatus string, statusFilter string, search string, province string, isp string, beianStatus string) ([]*domain.Task, int64, error) {
	return s.taskRepo.ListWithAdvancedFilters(ctx, page, pageSize, excludeStatus, statusFilter, search, province, isp, beianStatus)
}

func (s *taskService) ListTasksWithFilterOptions(ctx context.Context, page int, pageSize int, opts *repository.TaskFilterOptions) ([]*domain.Task, int64, error) {
	return s.taskRepo.ListWithFilterOptions(ctx, page, pageSize, opts)
}

func (s *taskService) ListQueuedTasks(ctx context.Context) ([]*domain.Task, error) {
	return s.taskRepo.ListQueuedTasks(ctx)
}
