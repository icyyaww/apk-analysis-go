package adb

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Client ADB 客户端
type Client struct {
	target    string        // ADB 目标地址 (如 android-emulator:5555)
	timeout   time.Duration // 命令超时时间
	logger    *logrus.Logger
	connMgr   *ConnectionManager // 连接管理器（单例）
}

// NewClient 创建 ADB 客户端
func NewClient(target string, timeout time.Duration, logger *logrus.Logger) *Client {
	return &Client{
		target:  target,
		timeout: timeout,
		logger:  logger,
		connMgr: GetConnectionManager(logger), // 获取全局连接管理器
	}
}

// Connect 连接设备（使用连接管理器，避免并发冲突）
func (c *Client) Connect(ctx context.Context) error {
	// 使用全局连接管理器，自动处理：
	// 1. ADB daemon 启动（全局互斥锁保护）
	// 2. 连接复用（避免重复 connect）
	// 3. 并发安全（所有操作都有锁保护）
	return c.connMgr.Connect(ctx, c.target)
}

// Disconnect 断开设备（使用连接管理器）
func (c *Client) Disconnect(ctx context.Context) error {
	return c.connMgr.Disconnect(ctx, c.target)
}

// IsConnected 检查设备是否连接（使用连接管理器）
func (c *Client) IsConnected(ctx context.Context) bool {
	return c.connMgr.IsConnected(ctx, c.target)
}

// Install 安装 APK
// -r: 替换已存在的应用
// -g: 自动授予所有运行时权限（网络、存储、相机、定位等）
func (c *Client) Install(ctx context.Context, apkPath string) error {
	c.logger.WithField("apk_path", apkPath).Info("Installing APK with auto-grant permissions")

	cmd := exec.CommandContext(ctx, "adb", "-s", c.target, "install", "-r", "-g", apkPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb install failed: %w, output: %s", err, string(output))
	}

	if !strings.Contains(string(output), "Success") {
		return fmt.Errorf("install failed: %s", string(output))
	}

	c.logger.Info("APK installed successfully (all permissions granted)")
	return nil
}

// Uninstall 卸载应用
func (c *Client) Uninstall(ctx context.Context, packageName string) error {
	c.logger.WithField("package", packageName).Info("Uninstalling app")

	cmd := exec.CommandContext(ctx, "adb", "-s", c.target, "uninstall", packageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb uninstall failed: %w, output: %s", err, string(output))
	}

	c.logger.Info("App uninstalled successfully")
	return nil
}

// Shell 执行 shell 命令
func (c *Client) Shell(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "adb", "-s", c.target, "shell", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("shell command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// GetPackages 获取已安装的包列表
func (c *Client) GetPackages(ctx context.Context) ([]string, error) {
	output, err := c.Shell(ctx, "pm list packages")
	if err != nil {
		return nil, err
	}

	var packages []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			pkg := strings.TrimPrefix(line, "package:")
			packages = append(packages, pkg)
		}
	}

	return packages, nil
}

// FindPackageByAPK 通过安装前后对比找到包名
func (c *Client) FindPackageByAPK(ctx context.Context, apkPath string) (string, error) {
	// 获取安装前的包列表
	beforePkgs, err := c.GetPackages(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get packages before install: %w", err)
	}
	beforeSet := make(map[string]bool)
	for _, pkg := range beforePkgs {
		beforeSet[pkg] = true
	}

	// 安装 APK
	if err := c.Install(ctx, apkPath); err != nil {
		return "", err
	}

	// 获取安装后的包列表
	afterPkgs, err := c.GetPackages(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get packages after install: %w", err)
	}

	// 对比找到新安装的包
	for _, pkg := range afterPkgs {
		if !beforeSet[pkg] {
			c.logger.WithField("package", pkg).Info("Found new package")
			return pkg, nil
		}
	}

	// 如果没有找到新包，说明是覆盖安装
	c.logger.Warn("No new package found (likely reinstall), trying alternative methods")

	// 方法1: 使用 aapt dump badging 直接从 APK 读取包名
	packageName, err := c.extractPackageNameWithAapt(ctx, apkPath)
	if err == nil && packageName != "" {
		c.logger.WithField("package", packageName).Info("Found package name using aapt")
		return packageName, nil
	}
	c.logger.WithError(err).Warn("aapt method failed, trying dumpsys")

	// 方法2: 使用 dumpsys package 查询APK路径对应的包名
	packageName, err = c.extractPackageNameWithDumpsys(ctx, apkPath)
	if err == nil && packageName != "" {
		c.logger.WithField("package", packageName).Info("Found package name using dumpsys")
		return packageName, nil
	}
	c.logger.WithError(err).Warn("dumpsys method failed, trying filename matching")

	// 方法3: 检查是否有以.apk 结尾的包名（通常APK文件名包含包名）
	apkFileName := strings.TrimSuffix(strings.TrimPrefix(apkPath, "inbound_apks/"), ".apk")
	for _, pkg := range afterPkgs {
		if strings.Contains(pkg, apkFileName) || strings.Contains(apkFileName, pkg) {
			c.logger.WithField("package", pkg).Info("Found package by filename matching")
			return pkg, nil
		}
	}

	return "", fmt.Errorf("failed to find package name from APK using all methods")
}

// Screenshot 截图
func (c *Client) Screenshot(ctx context.Context, outputPath string) error {
	// 1. 截图到设备
	remotePath := "/sdcard/screenshot.png"
	_, err := c.Shell(ctx, fmt.Sprintf("screencap -p %s", remotePath))
	if err != nil {
		return fmt.Errorf("screencap failed: %w", err)
	}

	// 2. 拉取到本地
	cmd := exec.CommandContext(ctx, "adb", "-s", c.target, "pull", remotePath, outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb pull failed: %w, output: %s", err, string(output))
	}

	// 3. 删除设备上的文件
	c.Shell(ctx, fmt.Sprintf("rm %s", remotePath))

	c.logger.WithField("output_path", outputPath).Debug("Screenshot saved")
	return nil
}

// DumpUIHierarchy 提取 UI 层级
func (c *Client) DumpUIHierarchy(ctx context.Context, outputPath string) error {
	// 1. dump 到设备
	remotePath := "/sdcard/window_dump.xml"
	_, err := c.Shell(ctx, fmt.Sprintf("uiautomator dump %s", remotePath))
	if err != nil {
		return fmt.Errorf("uiautomator dump failed: %w", err)
	}

	// 2. 拉取到本地
	cmd := exec.CommandContext(ctx, "adb", "-s", c.target, "pull", remotePath, outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb pull failed: %w, output: %s", err, string(output))
	}

	// 3. 删除设备上的文件
	c.Shell(ctx, fmt.Sprintf("rm %s", remotePath))

	c.logger.WithField("output_path", outputPath).Debug("UI hierarchy saved")
	return nil
}

// StartActivity 启动 Activity
func (c *Client) StartActivity(ctx context.Context, component string) error {
	c.logger.WithField("component", component).Debug("Starting activity")

	cmd := fmt.Sprintf("am start -n %s", component)
	output, err := c.Shell(ctx, cmd)
	if err != nil {
		return err
	}

	if strings.Contains(output, "Error") {
		return fmt.Errorf("start activity failed: %s", output)
	}

	return nil
}

// GetLogcat 获取 logcat 日志
func (c *Client) GetLogcat(ctx context.Context) (string, error) {
	return c.Shell(ctx, "logcat -d")
}

// ClearLogcat 清空 logcat
func (c *Client) ClearLogcat(ctx context.Context) error {
	_, err := c.Shell(ctx, "logcat -c")
	return err
}

// SetProxy 设置全局代理
func (c *Client) SetProxy(ctx context.Context, host string, port int) error {
	c.logger.WithFields(logrus.Fields{
		"host": host,
		"port": port,
	}).Info("Setting global proxy")

	cmd := fmt.Sprintf("settings put global http_proxy %s:%d", host, port)
	_, err := c.Shell(ctx, cmd)
	return err
}

// ClearProxy 清除代理
func (c *Client) ClearProxy(ctx context.Context) error {
	c.logger.Info("Clearing global proxy")

	_, err := c.Shell(ctx, "settings put global http_proxy :0")
	return err
}

// TapScreen 点击屏幕坐标
func (c *Client) TapScreen(ctx context.Context, x, y int) error {
	cmd := fmt.Sprintf("input tap %d %d", x, y)
	_, err := c.Shell(ctx, cmd)
	return err
}

// InputText 输入文本
func (c *Client) InputText(ctx context.Context, text string) error {
	// 转义特殊字符
	text = strings.ReplaceAll(text, " ", "%s")
	cmd := fmt.Sprintf("input text %s", text)
	_, err := c.Shell(ctx, cmd)
	return err
}

// PressBack 按返回键
func (c *Client) PressBack(ctx context.Context) error {
	_, err := c.Shell(ctx, "input keyevent 4")
	return err
}

// PressHome 按 Home 键
func (c *Client) PressHome(ctx context.Context) error {
	_, err := c.Shell(ctx, "input keyevent 3")
	return err
}

// GetForegroundPackage 获取当前前台应用的包名
// 用于检测操作后应用是否仍在前台，防止误操作导致应用退出
func (c *Client) GetForegroundPackage(ctx context.Context) (string, error) {
	// 方法1：使用 dumpsys window 获取当前焦点窗口
	output, err := c.Shell(ctx, "dumpsys window | grep mCurrentFocus")
	if err == nil && output != "" {
		// 输出格式: mCurrentFocus=Window{xxx u0 com.example.app/com.example.app.MainActivity}
		// 或: mCurrentFocus=Window{xxx u0 com.example.app/com.example.app.MainActivity}
		if idx := strings.Index(output, " u0 "); idx != -1 {
			rest := output[idx+4:]
			if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
				packageName := strings.TrimSpace(rest[:slashIdx])
				if packageName != "" && !strings.Contains(packageName, " ") {
					return packageName, nil
				}
			}
			// 如果没有斜杠，尝试提取到 } 为止
			if endIdx := strings.Index(rest, "}"); endIdx != -1 {
				packageName := strings.TrimSpace(rest[:endIdx])
				if slashIdx := strings.Index(packageName, "/"); slashIdx != -1 {
					packageName = packageName[:slashIdx]
				}
				if packageName != "" && !strings.Contains(packageName, " ") {
					return packageName, nil
				}
			}
		}
	}

	// 方法2：使用 dumpsys activity activities 获取 resumed activity
	output, err = c.Shell(ctx, "dumpsys activity activities | grep mResumedActivity")
	if err == nil && output != "" {
		// 输出格式: mResumedActivity: ActivityRecord{xxx u0 com.example.app/.MainActivity t123}
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "mResumedActivity") {
				// 查找 "u0 " 后面的包名
				if idx := strings.Index(line, " u0 "); idx != -1 {
					rest := line[idx+4:]
					if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
						packageName := strings.TrimSpace(rest[:slashIdx])
						if packageName != "" && !strings.Contains(packageName, " ") {
							return packageName, nil
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("failed to get foreground package")
}

// extractPackageNameWithAapt 使用 aapt 从 APK 文件中提取包名
func (c *Client) extractPackageNameWithAapt(ctx context.Context, apkPath string) (string, error) {
	// 尝试使用 aapt dump badging
	cmd := exec.CommandContext(ctx, "aapt", "dump", "badging", apkPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("aapt command failed: %w", err)
	}

	// 从输出中提取包名： package: name='com.example.app'
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "package:") {
			// 提取 name='xxx' 部分
			if idx := strings.Index(line, "name='"); idx != -1 {
				start := idx + 6 // len("name='")
				if end := strings.Index(line[start:], "'"); end != -1 {
					return line[start : start+end], nil
				}
			}
		}
	}

	return "", fmt.Errorf("package name not found in aapt output")
}

// extractPackageNameWithDumpsys 使用 dumpsys package 查询已安装应用的包名
func (c *Client) extractPackageNameWithDumpsys(ctx context.Context, apkPath string) (string, error) {
	// 从APK文件名提取可能的包名关键词
	apkFileName := strings.TrimSuffix(strings.TrimPrefix(apkPath, "inbound_apks/"), ".apk")

	// 获取所有已安装的包
	packages, err := c.GetPackages(ctx)
	if err != nil {
		return "", err
	}

	// 遍历每个包，使用 dumpsys package 查看详情
	for _, pkg := range packages {
		output, err := c.Shell(ctx, fmt.Sprintf("dumpsys package %s | grep codePath", pkg))
		if err != nil {
			continue
		}

		// 检查 codePath 是否包含APK文件名
		if strings.Contains(output, apkFileName) || strings.Contains(output, pkg) {
			return pkg, nil
		}
	}

	return "", fmt.Errorf("package not found in dumpsys output")
}
