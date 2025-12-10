package device

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/adb"
	"github.com/apk-analysis/apk-analysis-go/internal/cert"
	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/sirupsen/logrus"
)

// DeviceAcquireError 设备获取失败错误（包含失败类型）
type DeviceAcquireError struct {
	FailureType domain.FailureType
	Message     string
}

func (e *DeviceAcquireError) Error() string {
	return e.Message
}

// NewDeviceAcquireError 创建设备获取失败错误
func NewDeviceAcquireError(failureType domain.FailureType, message string) *DeviceAcquireError {
	return &DeviceAcquireError{
		FailureType: failureType,
		Message:     message,
	}
}

// DeviceArch 设备 CPU 架构类型
type DeviceArch string

const (
	ArchARM   DeviceArch = "arm"   // ARM 架构（真机）
	ArchX86   DeviceArch = "x86"   // x86 架构（模拟器）
	ArchAny   DeviceArch = "any"   // 任意架构（通用 APK）
)

// Device 代表一个 Android 设备
type Device struct {
	ID                 string      // 设备ID，如 "device-1", "device-2"
	ADBTarget          string      // ADB连接目标，如 "localhost:5554"
	ProxyHost          string      // 代理主机地址（从设备角度），如 "10.0.3.1"
	ProxyPort          int         // 代理端口，如 8082
	MitmproxyContainer string      // Mitmproxy 容器名称，如 "apk-analysis-mitmproxy-1"
	MitmproxyAPIPort   int         // Mitmproxy API 端口，如 8083, 8085
	FridaHost          string      // Frida 网络连接地址，如 "192.168.2.34:27042"（WiFi 模式）
	Arch               DeviceArch  // 设备 CPU 架构（arm/x86）
	mutex              sync.Mutex  // 设备级互斥锁
	inUse              bool        // 是否正在使用
	currentTaskID      string      // 当前正在执行的任务ID

	// 任务计数和休息控制
	tasksCompleted     int           // 当前已完成任务数
	restInterval       int           // 休息触发阈值（每N个任务休息一次，默认10）
	restDuration       time.Duration // 休息时长（默认30秒）
	isResting          bool          // 是否正在休息
	lastRestTime       time.Time     // 上次休息开始时间
}

// Lock 锁定设备
func (d *Device) Lock(taskID string) {
	d.mutex.Lock()
	d.inUse = true
	d.currentTaskID = taskID
}

// Unlock 释放设备
func (d *Device) Unlock() {
	d.currentTaskID = ""
	d.inUse = false
	d.mutex.Unlock()
}

// IsInUse 检查设备是否正在使用
func (d *Device) IsInUse() bool {
	return d.inUse
}

// DeviceManager 设备管理器
type DeviceManager struct {
	devices      []*Device
	mu           sync.Mutex
	logger       *logrus.Logger
	waitTimeout  time.Duration  // 等待设备可用的超时时间
}

// NewDeviceManager 创建设备管理器
func NewDeviceManager(logger *logrus.Logger) *DeviceManager {
	return &DeviceManager{
		devices:     make([]*Device, 0),
		logger:      logger,
		waitTimeout: 0, // 0 表示无限等待，任务会一直等待直到设备可用
	}
}

// AddDevice 添加设备到设备池
func (m *DeviceManager) AddDevice(device *Device) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devices = append(m.devices, device)
	m.logger.WithFields(logrus.Fields{
		"device_id":   device.ID,
		"adb_target":  device.ADBTarget,
		"proxy":       device.ProxyHost + ":" + string(rune(device.ProxyPort)),
		"total_devices": len(m.devices),
	}).Info("Device added to pool")
}

// ConfigureDeviceRest 配置所有设备的休息参数
func (m *DeviceManager) ConfigureDeviceRest(interval int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, device := range m.devices {
		device.restInterval = interval
		device.restDuration = duration
	}

	m.logger.WithFields(logrus.Fields{
		"rest_interval": interval,
		"rest_duration": duration.String(),
		"devices":       len(m.devices),
	}).Info("Device rest configuration applied to all devices")
}

// AcquireDevice 获取可用设备（阻塞等待直到有设备可用或超时）
func (m *DeviceManager) AcquireDevice(ctx context.Context, taskID string) (*Device, error) {
	m.logger.WithField("task_id", taskID).Info("Acquiring device from pool...")

	// 使用ticker定期检查设备可用性
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(m.waitTimeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-timeout:
			return nil, NewDeviceAcquireError(
				domain.FailureTypeDeviceTimeout,
				"timeout waiting for available device",
			)

		case <-ticker.C:
			// 尝试获取空闲设备
			device := m.tryAcquireDevice(taskID)
			if device != nil {
				m.logger.WithFields(logrus.Fields{
					"task_id":   taskID,
					"device_id": device.ID,
					"adb_target": device.ADBTarget,
				}).Info("Device acquired successfully")
				return device, nil
			}

			// 没有可用设备，继续等待
			m.logger.WithField("task_id", taskID).Debug("No device available, waiting...")
		}
	}
}

// tryAcquireDevice 尝试获取一个空闲设备（非阻塞）
func (m *DeviceManager) tryAcquireDevice(taskID string) *Device {
	return m.tryAcquireDeviceWithArch(taskID, ArchAny)
}

// tryAcquireDeviceWithArch 尝试获取符合架构要求的空闲设备（非阻塞）
// requiredArch: 需要的架构类型
//   - ArchARM: 只选择 ARM 设备（真机）
//   - ArchX86: 只选择 x86 设备（模拟器）
//   - ArchAny: 任意设备（优先 ARM，因为真机兼容性更好）
func (m *DeviceManager) tryAcquireDeviceWithArch(taskID string, requiredArch DeviceArch) *Device {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 两轮遍历：
	// 第一轮：如果 requiredArch == ArchARM 或 ArchAny，优先选择 ARM 设备
	// 第二轮：如果 requiredArch == ArchAny 且没找到 ARM，选择任意可用设备

	// 第一轮：寻找匹配架构的设备
	for _, device := range m.devices {
		// 架构过滤
		if requiredArch == ArchARM && device.Arch != ArchARM {
			continue
		}
		if requiredArch == ArchX86 && device.Arch != ArchX86 {
			continue
		}
		// ArchAny 第一轮优先选 ARM
		if requiredArch == ArchAny && device.Arch != ArchARM {
			continue
		}

		// 跳过正在休息的设备
		if device.isResting {
			m.logger.WithFields(logrus.Fields{
				"task_id":        taskID,
				"device_id":      device.ID,
				"rest_remaining": device.restDuration - time.Since(device.lastRestTime),
			}).Debug("Device is resting, skipping")
			continue
		}

		if device.mutex.TryLock() {
			// 在分配前检查设备健康状态
			if !m.isDeviceHealthy(device) {
				device.mutex.Unlock()
				m.logger.WithFields(logrus.Fields{
					"task_id":   taskID,
					"device_id": device.ID,
				}).Warn("Device is offline or unhealthy, skipping")
				continue
			}

			device.inUse = true
			device.currentTaskID = taskID
			m.logger.WithFields(logrus.Fields{
				"task_id":       taskID,
				"device_id":     device.ID,
				"device_arch":   device.Arch,
				"required_arch": requiredArch,
			}).Info("Device matched by architecture")
			return device
		}
	}

	// 第二轮：如果是 ArchAny 且没找到 ARM 设备，选择任意可用设备（包括 x86）
	if requiredArch == ArchAny {
		for _, device := range m.devices {
			// 跳过 ARM 设备（已在第一轮检查过）
			if device.Arch == ArchARM {
				continue
			}

			// 跳过正在休息的设备
			if device.isResting {
				continue
			}

			if device.mutex.TryLock() {
				if !m.isDeviceHealthy(device) {
					device.mutex.Unlock()
					continue
				}

				device.inUse = true
				device.currentTaskID = taskID
				m.logger.WithFields(logrus.Fields{
					"task_id":       taskID,
					"device_id":     device.ID,
					"device_arch":   device.Arch,
					"required_arch": requiredArch,
				}).Info("Device matched (fallback to x86)")
				return device
			}
		}
	}

	return nil
}

// AcquireDeviceForAPK 根据 APK 架构获取合适的设备
// apkArch: APK 的原生库架构（ArchARM/ArchX86/ArchAny）
// 如果 waitTimeout=0，则无限等待直到设备可用
func (m *DeviceManager) AcquireDeviceForAPK(ctx context.Context, taskID string, apkArch DeviceArch) (*Device, error) {
	m.logger.WithFields(logrus.Fields{
		"task_id":  taskID,
		"apk_arch": apkArch,
	}).Info("Acquiring device for APK architecture...")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// 如果 waitTimeout > 0，则设置超时；否则无限等待
	var timeoutCh <-chan time.Time
	if m.waitTimeout > 0 {
		timeoutCh = time.After(m.waitTimeout)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-timeoutCh:
			// 只有在设置了超时时才会触发
			// 根据设备池状态返回不同的错误类型
			exists, allBusy, _ := m.GetDeviceStatusForArch(apkArch)

			if !exists {
				// 系统中不存在该架构的设备
				if apkArch == ArchARM {
					return nil, NewDeviceAcquireError(
						domain.FailureTypeARMDeviceOnly,
						fmt.Sprintf("no ARM device configured (APK requires ARM architecture)"),
					)
				}
				return nil, NewDeviceAcquireError(
					domain.FailureTypeDeviceTimeout,
					fmt.Sprintf("no %s device configured", apkArch),
				)
			}

			if allBusy {
				// 设备存在但都被占用
				return nil, NewDeviceAcquireError(
					domain.FailureTypeDeviceTimeout,
					fmt.Sprintf("timeout waiting for %s device (all devices busy)", apkArch),
				)
			}

			// 设备存在但可能离线或健康检查失败
			return nil, NewDeviceAcquireError(
				domain.FailureTypeConnectionError,
				fmt.Sprintf("timeout waiting for %s device (devices may be offline or unhealthy)", apkArch),
			)

		case <-ticker.C:
			device := m.tryAcquireDeviceWithArch(taskID, apkArch)
			if device != nil {
				m.logger.WithFields(logrus.Fields{
					"task_id":     taskID,
					"device_id":   device.ID,
					"device_arch": device.Arch,
					"adb_target":  device.ADBTarget,
				}).Info("Device acquired successfully for APK")
				return device, nil
			}

			m.logger.WithFields(logrus.Fields{
				"task_id":  taskID,
				"apk_arch": apkArch,
			}).Debug("No matching device available, waiting...")
		}
	}
}

// isDeviceHealthy 快速检查设备是否健康（非阻塞）
// 注意：调用此方法前必须已经持有 device.mutex.Lock()
func (m *DeviceManager) isDeviceHealthy(dev *Device) bool {
	adbClient := dev.CreateADBClient(m.logger)

	// 快速超时检查（5秒）
	checkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 尝试执行简单的 shell 命令来验证连接
	_, err := adbClient.Shell(checkCtx, "echo 'health_check'")
	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"device_id": dev.ID,
			"error":     err.Error(),
		}).Warn("Device health check failed")
		return false
	}

	return true
}

// ReleaseDevice 释放设备并检查是否需要休息
func (m *DeviceManager) ReleaseDevice(device *Device) {
	if device == nil {
		return
	}

	taskID := device.currentTaskID
	deviceID := device.ID

	// 任务完成计数
	device.tasksCompleted++
	currentCount := device.tasksCompleted

	device.currentTaskID = ""
	device.inUse = false

	m.logger.WithFields(logrus.Fields{
		"task_id":         taskID,
		"device_id":       deviceID,
		"tasks_completed": currentCount,
		"rest_threshold":  device.restInterval,
	}).Info("Device released")

	// 检查是否需要休息（必须在释放互斥锁之前检查，因为休息逻辑需要修改设备状态）
	if device.restInterval > 0 && currentCount >= device.restInterval {
		m.triggerDeviceRest(device)
	}

	device.mutex.Unlock()
}

// triggerDeviceRest 触发设备休息（设备冷却）
func (m *DeviceManager) triggerDeviceRest(device *Device) {
	device.isResting = true
	device.tasksCompleted = 0
	device.lastRestTime = time.Now()

	m.logger.WithFields(logrus.Fields{
		"device_id":     device.ID,
		"rest_duration": device.restDuration.String(),
	}).Info("🛌 Device entering rest period (cooling down)...")

	// 异步休息（不阻塞设备释放）
	go func(dev *Device) {
		time.Sleep(dev.restDuration)

		dev.mutex.Lock()
		dev.isResting = false
		dev.mutex.Unlock()

		m.logger.WithFields(logrus.Fields{
			"device_id":  dev.ID,
			"rested_for": time.Since(dev.lastRestTime).String(),
		}).Info("✅ Device rest completed, ready for new tasks")
	}(device)
}

// GetDeviceCount 获取设备总数
func (m *DeviceManager) GetDeviceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.devices)
}

// GetAvailableDeviceCount 获取当前可用设备数
func (m *DeviceManager) GetAvailableDeviceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	available := 0
	for _, device := range m.devices {
		if !device.inUse {
			available++
		}
	}
	return available
}

// HasDeviceWithArch 检查是否存在指定架构的设备
func (m *DeviceManager) HasDeviceWithArch(arch DeviceArch) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, device := range m.devices {
		if arch == ArchAny || device.Arch == arch {
			return true
		}
	}
	return false
}

// GetDeviceStatusForArch 获取指定架构设备的状态
// 返回: exists(是否存在), allBusy(是否全忙), allOffline(是否全离线)
func (m *DeviceManager) GetDeviceStatusForArch(arch DeviceArch) (exists bool, allBusy bool, allOffline bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	matchingDevices := 0
	busyCount := 0

	for _, device := range m.devices {
		if arch == ArchAny || device.Arch == arch {
			matchingDevices++
			if device.inUse {
				busyCount++
			}
			if device.isResting {
				busyCount++ // 休息中也算忙
			}
		}
	}

	exists = matchingDevices > 0
	allBusy = exists && busyCount >= matchingDevices
	allOffline = false // 离线状态在健康检查中判断，这里简化处理
	return
}

// GetDeviceStats 获取设备池统计信息
func (m *DeviceManager) GetDeviceStats() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := map[string]interface{}{
		"total_devices": len(m.devices),
		"in_use":        0,
		"available":     0,
		"resting":       0,
		"devices":       make([]map[string]interface{}, 0),
	}

	for _, device := range m.devices {
		deviceInfo := map[string]interface{}{
			"id":              device.ID,
			"adb_target":      device.ADBTarget,
			"in_use":          device.inUse,
			"is_resting":      device.isResting,
			"tasks_completed": device.tasksCompleted,
			"rest_interval":   device.restInterval,
			"task_id":         device.currentTaskID,
		}

		// 如果设备正在休息，添加剩余休息时间
		if device.isResting {
			remainingRest := device.restDuration - time.Since(device.lastRestTime)
			if remainingRest > 0 {
				deviceInfo["rest_remaining_seconds"] = int(remainingRest.Seconds())
			} else {
				deviceInfo["rest_remaining_seconds"] = 0
			}
		}

		stats["devices"] = append(stats["devices"].([]map[string]interface{}), deviceInfo)

		// 统计设备状态
		if device.isResting {
			stats["resting"] = stats["resting"].(int) + 1
		} else if device.inUse {
			stats["in_use"] = stats["in_use"].(int) + 1
		} else {
			stats["available"] = stats["available"].(int) + 1
		}
	}

	return stats
}

// CreateADBClient 为设备创建 ADB 客户端
func (d *Device) CreateADBClient(logger *logrus.Logger) *adb.Client {
	return adb.NewClient(d.ADBTarget, 30*time.Second, logger)
}

// GetProxyAddress 获取代理地址（从设备角度）
func (d *Device) GetProxyAddress() (string, int) {
	return d.ProxyHost, d.ProxyPort
}

// StartHealthCheck 启动设备健康检查（定期检查并重启异常设备）
func (m *DeviceManager) StartHealthCheck(ctx context.Context, interval time.Duration) {
	m.logger.WithField("interval", interval.String()).Info("Starting device health check")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Device health check stopped")
			return

		case <-ticker.C:
			m.checkAllDevices(ctx)
		}
	}
}

// checkAllDevices 检查所有设备的健康状态
func (m *DeviceManager) checkAllDevices(ctx context.Context) {
	m.mu.Lock()
	devices := append([]*Device{}, m.devices...)
	m.mu.Unlock()

	for _, dev := range devices {
		// 跳过正在使用的设备（不干扰正在执行的任务）
		if dev.inUse {
			m.logger.WithFields(logrus.Fields{
				"device_id": dev.ID,
				"task_id":   dev.currentTaskID,
			}).Debug("Device is in use, skipping health check")
			continue
		}

		// 检查 package service 是否正常
		if !m.checkPackageService(ctx, dev) {
			m.logger.WithField("device_id", dev.ID).Warn("Package service unhealthy, scheduling restart")
			m.restartDevice(ctx, dev)
		} else {
			m.logger.WithField("device_id", dev.ID).Debug("Device health check passed")
		}
	}
}

// checkPackageService 检查设备的 package service 是否正常
func (m *DeviceManager) checkPackageService(ctx context.Context, dev *Device) bool {
	adbClient := dev.CreateADBClient(m.logger)

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 测试 pm list packages 命令
	_, err := adbClient.Shell(checkCtx, "pm list packages | head -1")
	return err == nil
}

// restartDevice 重启异常设备
func (m *DeviceManager) restartDevice(ctx context.Context, dev *Device) {
	m.logger.WithField("device_id", dev.ID).Info("Restarting device...")

	// 锁定设备（防止新任务分配到重启中的设备）
	dev.mutex.Lock()
	defer dev.mutex.Unlock()

	// 根据设备ID确定容器名称
	containerName := fmt.Sprintf("apk-analysis-android-%s", dev.ID)

	m.logger.WithFields(logrus.Fields{
		"device_id":      dev.ID,
		"container_name": containerName,
	}).Info("Executing docker restart")

	// 重启 Docker 容器
	cmd := exec.Command("docker", "restart", containerName)
	if err := cmd.Run(); err != nil {
		m.logger.WithError(err).WithField("device_id", dev.ID).Error("Failed to restart container")
		return
	}

	m.logger.WithField("device_id", dev.ID).Info("Container restarted, waiting for device to be ready...")

	// 等待设备启动完成（90秒）
	time.Sleep(90 * time.Second)

	// 验证设备是否恢复健康
	if !m.checkPackageService(context.Background(), dev) {
		m.logger.WithField("device_id", dev.ID).Error("❌ Device still unhealthy after restart")
		return
	}

	m.logger.WithField("device_id", dev.ID).Info("✅ Device restarted successfully and is healthy")

	// 自动安装 mitmproxy 证书
	m.logger.WithField("device_id", dev.ID).Info("Installing mitmproxy certificate...")
	certInstaller := cert.NewInstaller(dev.ADBTarget, m.logger)

	installCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := certInstaller.PrepareAndInstall(installCtx, dev.MitmproxyContainer); err != nil {
		m.logger.WithError(err).WithField("device_id", dev.ID).Error("Failed to install certificate after restart")
	} else {
		m.logger.WithField("device_id", dev.ID).Info("✅ Certificate installed successfully after restart")
	}
}

// DetectAPKArch 检测 APK 文件中的原生库架构
// 通过检查 APK 中的 lib/ 目录来判断支持的 CPU 架构
// 返回值：
//   - ArchARM: 只有 ARM 原生库（armeabi-v7a, arm64-v8a）
//   - ArchX86: 只有 x86 原生库（x86, x86_64）
//   - ArchAny: 没有原生库（纯 Java/Kotlin），或同时支持两种架构
func DetectAPKArch(apkPath string) DeviceArch {
	// 使用 unzip -l 列出 APK 内容，查找 lib/ 目录
	cmd := exec.Command("unzip", "-l", apkPath)
	output, err := cmd.Output()
	if err != nil {
		// 检测失败，返回 ArchAny（通用）
		return ArchAny
	}

	outputStr := string(output)

	// 检查是否有原生库
	hasARM := false
	hasX86 := false

	// ARM 架构标识
	if containsAny(outputStr, "lib/armeabi", "lib/arm64") {
		hasARM = true
	}

	// x86 架构标识
	if containsAny(outputStr, "lib/x86", "lib/x86_64") {
		hasX86 = true
	}

	// 判断架构类型
	if hasARM && !hasX86 {
		return ArchARM // 只有 ARM，必须使用真机
	}
	if hasX86 && !hasARM {
		return ArchX86 // 只有 x86，可以使用模拟器
	}
	// 两种都有或都没有，返回 ArchAny
	return ArchAny
}

// containsAny 检查字符串是否包含任意一个子串
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
