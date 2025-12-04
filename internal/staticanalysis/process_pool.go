package staticanalysis

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ProcessPool Python 进程池（复用 Python 进程，减少启动开销）
type ProcessPool struct {
	pythonPath string
	scriptPath string
	poolSize   int
	processes  []*Process
	taskQueue  chan *AnalysisTask
	wg         sync.WaitGroup
	logger     *logrus.Logger
	ctx        context.Context
	cancel     context.CancelFunc
}

// Process 单个 Python 进程
type Process struct {
	id     int
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser
	mu     sync.Mutex
	active bool
}

// AnalysisTask 分析任务
type AnalysisTask struct {
	APKPath  string
	Callback func(*DeepAnalysisResult, error)
}

// NewProcessPool 创建进程池
func NewProcessPool(pythonPath, scriptPath string, poolSize int, logger *logrus.Logger) (*ProcessPool, error) {
	if poolSize <= 0 {
		poolSize = 3 // 默认 3 个进程
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &ProcessPool{
		pythonPath: pythonPath,
		scriptPath: scriptPath,
		poolSize:   poolSize,
		processes:  make([]*Process, poolSize),
		taskQueue:  make(chan *AnalysisTask, 100),
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
	}

	// 启动进程池
	for i := 0; i < poolSize; i++ {
		process, err := pool.startProcess(i)
		if err != nil {
			// 回滚已启动的进程
			pool.Stop()
			return nil, fmt.Errorf("failed to start process %d: %w", i, err)
		}
		pool.processes[i] = process

		// 启动 worker
		pool.wg.Add(1)
		go pool.worker(i, process)
	}

	pool.logger.WithField("pool_size", poolSize).Info("Python process pool started")

	return pool, nil
}

// startProcess 启动单个 Python 进程（常驻）
func (pp *ProcessPool) startProcess(id int) (*Process, error) {
	// 使用服务模式启动 Python 脚本
	cmd := exec.CommandContext(pp.ctx, pp.pythonPath, "-u", pp.scriptPath, "--server-mode")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start python process: %w", err)
	}

	process := &Process{
		id:     id,
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: stderr,
		active: true,
	}

	// 启动 stderr 日志读取
	go pp.readStderr(id, stderr)

	pp.logger.WithField("worker_id", id).Info("Python worker process started")

	return process, nil
}

// readStderr 读取 Python 进程的 stderr 输出
func (pp *ProcessPool) readStderr(id int, stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		pp.logger.WithFields(logrus.Fields{
			"worker_id": id,
			"stderr":    line,
		}).Debug("Python worker stderr")
	}
}

// worker 处理任务（复用 Python 进程）
func (pp *ProcessPool) worker(id int, process *Process) {
	defer pp.wg.Done()

	pp.logger.WithField("worker_id", id).Info("Worker started")

	for {
		select {
		case <-pp.ctx.Done():
			pp.logger.WithField("worker_id", id).Info("Worker shutting down")
			return

		case task, ok := <-pp.taskQueue:
			if !ok {
				pp.logger.WithField("worker_id", id).Info("Task queue closed")
				return
			}

			pp.processTask(id, process, task)
		}
	}
}

// processTask 处理单个任务
func (pp *ProcessPool) processTask(id int, process *Process, task *AnalysisTask) {
	process.mu.Lock()
	defer process.mu.Unlock()

	if !process.active {
		pp.logger.WithField("worker_id", id).Warn("Process not active, skipping task")
		task.Callback(nil, fmt.Errorf("process not active"))
		return
	}

	pp.logger.WithFields(logrus.Fields{
		"worker_id": id,
		"apk_path":  task.APKPath,
	}).Debug("Processing task")

	// 发送任务到 Python 进程（stdin）
	taskJSON := map[string]string{"apk_path": task.APKPath}
	taskBytes, err := json.Marshal(taskJSON)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to marshal task JSON")
		task.Callback(nil, err)
		return
	}

	// 写入任务（单行 JSON + 换行符）
	if _, err := fmt.Fprintf(process.stdin, "%s\n", string(taskBytes)); err != nil {
		pp.logger.WithError(err).Error("Failed to write task to python process")
		process.active = false
		task.Callback(nil, fmt.Errorf("failed to write task: %w", err))
		return
	}

	// 读取结果（单行 JSON，带超时）
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		line, err := process.stdout.ReadString('\n')
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- line
		}
	}()

	// 超时控制（最长 60 秒）
	timeout := time.After(60 * time.Second)

	select {
	case line := <-resultChan:
		// 解析结果 JSON
		var result DeepAnalysisResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			pp.logger.WithError(err).Error("Failed to parse python result")
			task.Callback(nil, fmt.Errorf("failed to parse result: %w", err))
			return
		}

		task.Callback(&result, nil)

	case err := <-errorChan:
		pp.logger.WithError(err).Error("Failed to read from python process")
		process.active = false
		task.Callback(nil, fmt.Errorf("failed to read result: %w", err))

	case <-timeout:
		pp.logger.Warn("Python analysis timeout")
		process.active = false
		task.Callback(nil, fmt.Errorf("analysis timeout (60s)"))
	}
}

// Submit 提交任务到进程池
func (pp *ProcessPool) Submit(task *AnalysisTask) error {
	select {
	case pp.taskQueue <- task:
		return nil
	default:
		return fmt.Errorf("process pool task queue is full")
	}
}

// Stop 停止进程池
func (pp *ProcessPool) Stop() {
	pp.logger.Info("Stopping Python process pool")

	// 关闭任务队列
	close(pp.taskQueue)

	// 取消 context（触发所有 worker 退出）
	pp.cancel()

	// 等待所有 worker 退出
	pp.wg.Wait()

	// 关闭所有 Python 进程
	for i, process := range pp.processes {
		if process != nil && process.cmd != nil && process.cmd.Process != nil {
			process.stdin.Close()
			process.cmd.Process.Kill()
			pp.logger.WithField("worker_id", i).Info("Python worker process stopped")
		}
	}

	pp.logger.Info("Python process pool stopped")
}

// GetStats 获取进程池统计信息
func (pp *ProcessPool) GetStats() map[string]interface{} {
	activeCount := 0
	for _, process := range pp.processes {
		if process != nil && process.active {
			activeCount++
		}
	}

	return map[string]interface{}{
		"pool_size":    pp.poolSize,
		"active_count": activeCount,
		"queue_size":   len(pp.taskQueue),
	}
}
