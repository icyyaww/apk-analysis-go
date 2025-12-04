package adb

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ConnectionManager ADB 连接管理器（单例模式）
// 功能：
// 1. 全局互斥锁保护 ADB daemon 初始化（解决端口冲突）
// 2. 连接复用（每个设备一个连接，避免重复 connect）
// 3. 心跳检测（定期检查连接状态）
type ConnectionManager struct {
	// 全局互斥锁，保护 ADB daemon 初始化
	daemonMutex sync.Mutex

	// 连接状态缓存（key: target, value: 是否已连接）
	connections map[string]bool
	connMutex   sync.RWMutex

	// ADB daemon 是否已启动
	daemonStarted bool

	logger *logrus.Logger
}

var (
	// 全局单例
	once              sync.Once
	connectionManager *ConnectionManager
)

// GetConnectionManager 获取全局连接管理器（单例）
func GetConnectionManager(logger *logrus.Logger) *ConnectionManager {
	once.Do(func() {
		connectionManager = &ConnectionManager{
			connections:   make(map[string]bool),
			daemonStarted: false,
			logger:        logger,
		}
	})
	return connectionManager
}

// EnsureDaemonStarted 确保 ADB daemon 已启动（线程安全）
// 这是解决并发冲突的核心方法
func (m *ConnectionManager) EnsureDaemonStarted(ctx context.Context) error {
	// 快速路径：已经启动，无需加锁
	if m.daemonStarted {
		return nil
	}

	// 慢速路径：需要启动 daemon，加锁保证只启动一次
	m.daemonMutex.Lock()
	defer m.daemonMutex.Unlock()

	// Double-check：可能其他线程已经启动了
	if m.daemonStarted {
		return nil
	}

	m.logger.Info("ADB daemon not started, initializing...")

	// 检查 daemon 是否已经在运行
	if m.isDaemonRunning(ctx) {
		m.logger.Info("ADB daemon already running")
		m.daemonStarted = true
		return nil
	}

	// 启动 daemon（通过执行任意 adb 命令触发）
	m.logger.Info("Starting ADB daemon...")
	cmd := exec.CommandContext(ctx, "adb", "start-server")
	output, err := cmd.CombinedOutput()

	if err != nil {
		m.logger.WithError(err).WithField("output", string(output)).Warn("Failed to start ADB daemon explicitly, will retry on first connect")
		// 不返回错误，因为 daemon 可能在第一次 connect 时自动启动
	} else {
		m.logger.WithField("output", string(output)).Info("ADB daemon started successfully")
	}

	// 等待 daemon 完全启动
	time.Sleep(2 * time.Second)

	m.daemonStarted = true
	return nil
}

// isDaemonRunning 检查 ADB daemon 是否正在运行
func (m *ConnectionManager) isDaemonRunning(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "adb", "devices")
	err := cmd.Run()
	return err == nil
}

// Connect 连接到设备（带缓存和锁保护）
func (m *ConnectionManager) Connect(ctx context.Context, target string) error {
	// 1. 确保 ADB daemon 已启动
	if err := m.EnsureDaemonStarted(ctx); err != nil {
		return fmt.Errorf("failed to ensure daemon started: %w", err)
	}

	// 2. 检查是否已连接（读锁）
	m.connMutex.RLock()
	if m.connections[target] {
		m.connMutex.RUnlock()
		m.logger.WithField("target", target).Debug("Already connected (cached)")
		return nil
	}
	m.connMutex.RUnlock()

	// 3. 执行连接（写锁）
	m.connMutex.Lock()
	defer m.connMutex.Unlock()

	// Double-check：可能其他线程已经连接了
	if m.connections[target] {
		m.logger.WithField("target", target).Debug("Already connected (race condition avoided)")
		return nil
	}

	m.logger.WithField("target", target).Info("Connecting to ADB device...")

	// 执行 adb connect
	cmd := exec.CommandContext(ctx, "adb", "connect", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb connect failed: %w, output: %s", err, string(output))
	}

	// 标记为已连接
	m.connections[target] = true
	m.logger.WithField("target", target).WithField("output", string(output)).Info("ADB connected successfully")

	return nil
}

// Disconnect 断开设备连接
func (m *ConnectionManager) Disconnect(ctx context.Context, target string) error {
	m.connMutex.Lock()
	defer m.connMutex.Unlock()

	cmd := exec.CommandContext(ctx, "adb", "disconnect", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.WithError(err).WithField("target", target).Warn("ADB disconnect failed")
		return err
	}

	// 从缓存中移除
	delete(m.connections, target)
	m.logger.WithField("target", target).WithField("output", string(output)).Info("ADB disconnected")

	return nil
}

// IsConnected 检查设备是否已连接（优先使用缓存）
func (m *ConnectionManager) IsConnected(ctx context.Context, target string) bool {
	// 先检查缓存
	m.connMutex.RLock()
	cached := m.connections[target]
	m.connMutex.RUnlock()

	if !cached {
		return false
	}

	// 验证实际连接状态
	cmd := exec.CommandContext(ctx, "adb", "devices")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 连接失败，更新缓存
		m.connMutex.Lock()
		m.connections[target] = false
		m.connMutex.Unlock()
		return false
	}

	connected := strings.Contains(string(output), target) &&
		strings.Contains(string(output), "device")

	// 更新缓存
	if !connected {
		m.connMutex.Lock()
		m.connections[target] = false
		m.connMutex.Unlock()
	}

	return connected
}

// StartHealthCheck 启动连接健康检查（定期检查并重连）
func (m *ConnectionManager) StartHealthCheck(ctx context.Context, interval time.Duration, targets []string) {
	m.logger.WithField("interval", interval.String()).Info("Starting ADB connection health check")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("ADB connection health check stopped")
			return

		case <-ticker.C:
			m.checkAndReconnect(ctx, targets)
		}
	}
}

// checkAndReconnect 检查所有设备并重连断开的设备
func (m *ConnectionManager) checkAndReconnect(ctx context.Context, targets []string) {
	for _, target := range targets {
		if !m.IsConnected(ctx, target) {
			m.logger.WithField("target", target).Warn("Device disconnected, attempting to reconnect...")

			if err := m.Connect(ctx, target); err != nil {
				m.logger.WithError(err).WithField("target", target).Error("Failed to reconnect device")
			} else {
				m.logger.WithField("target", target).Info("Device reconnected successfully")
			}
		}
	}
}

// GetConnectionStats 获取连接统计信息
func (m *ConnectionManager) GetConnectionStats() map[string]interface{} {
	m.connMutex.RLock()
	defer m.connMutex.RUnlock()

	connectedDevices := []string{}
	for target, connected := range m.connections {
		if connected {
			connectedDevices = append(connectedDevices, target)
		}
	}

	return map[string]interface{}{
		"daemon_started":     m.daemonStarted,
		"total_connections":  len(m.connections),
		"connected_devices":  connectedDevices,
		"connected_count":    len(connectedDevices),
	}
}
