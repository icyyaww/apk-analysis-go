package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// Pool Worker 池
type Pool struct {
	workers      int
	taskChan     chan *Task
	orchestrator *Orchestrator
	logger       *logrus.Logger
	wg           sync.WaitGroup
}

// Task 任务
type Task struct {
	ID       string
	APKPath  string
	resultCh chan error // 用于同步等待任务完成
}

// NewPool 创建 Worker 池
func NewPool(workers int, orchestrator *Orchestrator, logger *logrus.Logger) *Pool {
	return &Pool{
		workers:      workers,
		taskChan:     make(chan *Task, 100),
		orchestrator: orchestrator,
		logger:       logger,
	}
}

// Start 启动 Worker 池
func (p *Pool) Start(ctx context.Context) {
	p.logger.WithField("workers", p.workers).Info("Starting worker pool")

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

// worker Worker 协程
func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	p.logger.WithField("worker_id", id).Info("Worker started")

	for {
		select {
		case <-ctx.Done():
			p.logger.WithField("worker_id", id).Info("Worker shutting down")
			return

		case task, ok := <-p.taskChan:
			if !ok {
				p.logger.WithField("worker_id", id).Info("Task channel closed, worker exiting")
				return
			}

			p.logger.WithFields(logrus.Fields{
				"worker_id": id,
				"task_id":   task.ID,
				"apk_path":  task.APKPath,
			}).Info("Processing task")

			err := p.orchestrator.ExecuteTask(ctx, task.ID, task.APKPath)

			if err != nil {
				// 检查是否为可重试错误
				if retryErr, ok := IsRetryableError(err); ok {
					p.logger.WithFields(logrus.Fields{
						"worker_id":   id,
						"task_id":     retryErr.TaskID,
						"retry_count": retryErr.RetryCount,
						"max_retry":   retryErr.MaxRetry,
					}).Warn("🔄 Task failed and reset for retry (will be re-published to queue)")
				} else {
					p.logger.WithError(err).WithFields(logrus.Fields{
						"worker_id": id,
						"task_id":   task.ID,
					}).Error("Task execution failed")
				}
			} else {
				p.logger.WithFields(logrus.Fields{
					"worker_id": id,
					"task_id":   task.ID,
				}).Info("Task completed successfully")
			}

			// 如果有结果通道，发送结果
			if task.resultCh != nil {
				task.resultCh <- err
				close(task.resultCh)
			}
		}
	}
}

// Submit 提交任务（异步，不等待结果）
func (p *Pool) Submit(task *Task) error {
	select {
	case p.taskChan <- task:
		p.logger.WithField("task_id", task.ID).Debug("Task submitted to pool")
		return nil
	default:
		return fmt.Errorf("task queue is full")
	}
}

// SubmitAndWait 提交任务并等待完成
func (p *Pool) SubmitAndWait(ctx context.Context, task *Task) error {
	// 创建结果通道
	task.resultCh = make(chan error, 1)

	// 提交任务
	select {
	case p.taskChan <- task:
		p.logger.WithField("task_id", task.ID).Debug("Task submitted to pool (sync)")
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待结果
	select {
	case err := <-task.resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop 停止 Worker 池
func (p *Pool) Stop() {
	p.logger.Info("Stopping worker pool")
	close(p.taskChan)
	p.wg.Wait()
	p.logger.Info("Worker pool stopped")
}

// GetQueueSize 获取队列中任务数
func (p *Pool) GetQueueSize() int {
	return len(p.taskChan)
}
