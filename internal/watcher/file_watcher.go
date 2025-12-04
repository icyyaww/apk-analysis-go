package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// FileHandler 文件处理函数
type FileHandler func(ctx context.Context, filePath string) error

// FileWatcher 文件监控器
type FileWatcher struct {
	watcher    *fsnotify.Watcher
	watchDir   string
	pattern    string // 文件匹配模式 (如 "*.apk")
	handler    FileHandler
	logger     *logrus.Logger
	debounce   time.Duration // 防抖时间
	processing map[string]bool
	stopChan   chan struct{}
}

// NewFileWatcher 创建文件监控器
func NewFileWatcher(watchDir, pattern string, handler FileHandler, logger *logrus.Logger) (*FileWatcher, error) {
	// 创建 fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	// 确保监控目录存在
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to create watch directory: %w", err)
	}

	// 添加监控目录
	if err := watcher.Add(watchDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to add watch directory: %w", err)
	}

	fw := &FileWatcher{
		watcher:    watcher,
		watchDir:   watchDir,
		pattern:    pattern,
		handler:    handler,
		logger:     logger,
		debounce:   2 * time.Second, // 2秒防抖
		processing: make(map[string]bool),
		stopChan:   make(chan struct{}),
	}

	logger.WithFields(logrus.Fields{
		"watch_dir": watchDir,
		"pattern":   pattern,
	}).Info("File watcher created")

	return fw, nil
}

// Start 启动文件监控
func (fw *FileWatcher) Start(ctx context.Context) error {
	fw.logger.Info("Starting file watcher")

	// 禁用启动时扫描现有文件的功能
	// 这样重启服务时不会重复处理已存在的APK文件
	// 如果需要重新分析已有的APK,请手动删除后重新上传
	// if err := fw.scanExistingFiles(ctx); err != nil {
	// 	fw.logger.WithError(err).Warn("Failed to scan existing files")
	// }

	fw.logger.Info("Skipping scan of existing files on startup")

	// 启动事件循环
	go fw.eventLoop(ctx)

	fw.logger.Info("File watcher started successfully")
	return nil
}

// scanExistingFiles 扫描现有文件
func (fw *FileWatcher) scanExistingFiles(ctx context.Context) error {
	entries, err := os.ReadDir(fw.watchDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 检查文件名是否匹配模式
		if fw.matchPattern(entry.Name()) {
			filePath := filepath.Join(fw.watchDir, entry.Name())
			fw.logger.WithField("file", entry.Name()).Info("Found existing file")

			// 处理文件
			go fw.handleFile(ctx, filePath)
		}
	}

	return nil
}

// eventLoop 事件循环
func (fw *FileWatcher) eventLoop(ctx context.Context) {
	debounceTimer := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			fw.logger.Info("File watcher context done")
			return
		case <-fw.stopChan:
			fw.logger.Info("File watcher stopped")
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				fw.logger.Warn("Watcher events channel closed")
				return
			}

			// 只处理创建和写入事件
			if event.Op&fsnotify.Create != fsnotify.Create &&
				event.Op&fsnotify.Write != fsnotify.Write {
				continue
			}

			// 检查文件名是否匹配模式
			fileName := filepath.Base(event.Name)
			if !fw.matchPattern(fileName) {
				continue
			}

			fw.logger.WithFields(logrus.Fields{
				"event": event.Op.String(),
				"file":  fileName,
			}).Debug("File event detected")

			// 防抖处理: 同一文件在短时间内多次触发只处理一次
			if timer, exists := debounceTimer[event.Name]; exists {
				timer.Stop()
			}

			debounceTimer[event.Name] = time.AfterFunc(fw.debounce, func() {
				delete(debounceTimer, event.Name)
				fw.handleFile(ctx, event.Name)
			})

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				fw.logger.Warn("Watcher errors channel closed")
				return
			}
			fw.logger.WithError(err).Error("Watcher error")
		}
	}
}

// handleFile 处理文件
func (fw *FileWatcher) handleFile(ctx context.Context, filePath string) {
	// 检查是否正在处理
	if fw.processing[filePath] {
		fw.logger.WithField("file", filePath).Debug("File is already being processed")
		return
	}
	fw.processing[filePath] = true
	defer delete(fw.processing, filePath)

	// 等待文件写入完成
	if err := fw.waitForFileReady(filePath); err != nil {
		fw.logger.WithError(err).WithField("file", filePath).Error("File not ready")
		return
	}

	// 调用处理函数
	fw.logger.WithField("file", filePath).Info("Processing file")

	if err := fw.handler(ctx, filePath); err != nil {
		fw.logger.WithError(err).WithField("file", filePath).Error("Failed to process file")
		return
	}

	fw.logger.WithField("file", filePath).Info("File processed successfully")
}

// waitForFileReady 等待文件准备就绪 (写入完成)
func (fw *FileWatcher) waitForFileReady(filePath string) error {
	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		// 尝试打开文件
		file, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file does not exist")
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 检查文件大小是否稳定
		info1, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}

		time.Sleep(500 * time.Millisecond)

		info2, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}

		file.Close()

		// 文件大小稳定, 说明写入完成
		if info1.Size() == info2.Size() && info1.Size() > 0 {
			return nil
		}
	}

	return fmt.Errorf("file not ready after %d attempts", maxAttempts)
}

// matchPattern 检查文件名是否匹配模式
func (fw *FileWatcher) matchPattern(fileName string) bool {
	// 简单的通配符匹配 (*.apk)
	if fw.pattern == "*" {
		return true
	}

	if strings.HasPrefix(fw.pattern, "*.") {
		ext := strings.TrimPrefix(fw.pattern, "*")
		return strings.HasSuffix(strings.ToLower(fileName), strings.ToLower(ext))
	}

	return fileName == fw.pattern
}

// Stop 停止文件监控
func (fw *FileWatcher) Stop() error {
	fw.logger.Info("Stopping file watcher")
	close(fw.stopChan)

	if fw.watcher != nil {
		return fw.watcher.Close()
	}

	return nil
}

// GetWatchDir 获取监控目录
func (fw *FileWatcher) GetWatchDir() string {
	return fw.watchDir
}
