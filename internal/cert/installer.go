package cert

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Installer mitmproxy 证书安装器
type Installer struct {
	adbTarget  string // ADB 设备地址
	scriptPath string // 安装脚本路径
	logger     *logrus.Logger
}

// NewInstaller 创建证书安装器
func NewInstaller(adbTarget string, logger *logrus.Logger) *Installer {
	return &Installer{
		adbTarget:  adbTarget,
		scriptPath: "./scripts/wait_and_install_cert.sh",
		logger:     logger,
	}
}

// Install 自动安装 mitmproxy 证书到系统信任存储
func (i *Installer) Install(ctx context.Context) error {
	i.logger.Info("Starting automatic mitmproxy certificate installation")

	// 执行安装脚本
	cmd := exec.CommandContext(ctx, "bash", i.scriptPath, i.adbTarget)
	output, err := cmd.CombinedOutput()

	// 记录输出
	outputStr := string(output)
	if len(outputStr) > 0 {
		for _, line := range strings.Split(outputStr, "\n") {
			if line != "" {
				i.logger.Info(line)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("certificate installation failed: %w, output: %s", err, outputStr)
	}

	i.logger.Info("Certificate installed successfully, HTTPS interception enabled")
	return nil
}

// InstallManual 手动安装证书到用户证书目录 (不等待设备启动)
// 安装到 /data/misc/user/0/cacerts-added/ (推荐方式)
// 包含完整流程：导出证书 -> 计算Hash -> 推送 -> 安装
func (i *Installer) InstallManual(ctx context.Context, certHash string) error {
	i.logger.WithField("cert_hash", certHash).Info("Installing user certificate manually")

	// 0. 从 mitmproxy 容器导出证书并推送到设备
	mitmproxyContainer := "apk-analysis-mitmproxy-1"
	certDir := "/tmp/mitmproxy_certs"
	certFile := fmt.Sprintf("%s.0", certHash)

	// 导出证书
	i.logger.Info("Exporting certificate from mitmproxy container...")
	cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("mkdir -p %s && docker exec %s cat /home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem > %s/mitmproxy-ca-cert.pem", certDir, mitmproxyContainer, certDir))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to export certificate from mitmproxy: %w", err)
	}

	// 创建 Android 格式证书
	cmd = exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("cat %s/mitmproxy-ca-cert.pem > %s/%s", certDir, certDir, certFile))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Android certificate file: %w", err)
	}

	// 1. 获取 root 权限
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "root")
	cmd.Run() // 忽略错误

	// 等待 root 生效
	time.Sleep(2 * time.Second)

	// 重新连接（root 后需要重连）
	cmd = exec.CommandContext(ctx, "adb", "connect", i.adbTarget)
	cmd.Run()
	time.Sleep(1 * time.Second)

	// 2. 创建用户证书目录
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", "mkdir -p /data/misc/user/0/cacerts-added")
	if err := cmd.Run(); err != nil {
		i.logger.WithError(err).Warn("Failed to create user certificate directory")
	}

	// 3. 推送证书到设备
	i.logger.Info("Pushing certificate to device...")
	srcFile := fmt.Sprintf("%s/%s", certDir, certFile)
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "push", srcFile, "/sdcard/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push certificate to device: %w", err)
	}

	// 4. 复制证书到用户证书目录
	srcPath := fmt.Sprintf("/sdcard/%s", certFile)
	destPath := fmt.Sprintf("/data/misc/user/0/cacerts-added/%s", certFile)

	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("cp %s %s", srcPath, destPath))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy certificate to user cert directory: %w", err)
	}

	// 5. 设置权限
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("chmod 644 %s", destPath))
	cmd.Run()

	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("chown system:system %s", destPath))
	cmd.Run()

	// 6. 清理临时文件
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("rm -f %s", srcPath))
	cmd.Run()

	// 7. 验证
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("ls -l %s", destPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("user certificate verification failed: %w", err)
	}

	i.logger.WithField("output", string(output)).Info("User certificate installed successfully")
	return nil
}

// InstallManualSystemCert 手动安装证书到系统证书目录 (已废弃，仅用于兼容)
// 安装到 /system/etc/security/cacerts/ (需要 overlayfs + 重启)
func (i *Installer) InstallManualSystemCert(ctx context.Context, certHash string) error {
	i.logger.WithField("cert_hash", certHash).Warn("Installing system certificate manually (deprecated)")

	// 1. 获取 root 权限
	cmd := exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "root")
	cmd.Run() // 忽略错误

	// 2. 重新挂载 /system 为可写
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", "mount -o rw,remount /")
	if err := cmd.Run(); err != nil {
		i.logger.WithError(err).Warn("Failed to remount /system as rw")
	}

	// 3. 复制证书
	certFile := fmt.Sprintf("%s.0", certHash)
	srcPath := fmt.Sprintf("/sdcard/%s", certFile)
	destPath := fmt.Sprintf("/system/etc/security/cacerts/%s", certFile)

	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("cp %s %s", srcPath, destPath))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy certificate: %w", err)
	}

	// 4. 设置权限
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("chmod 644 %s", destPath))
	cmd.Run()

	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("chown root:root %s", destPath))
	cmd.Run()

	// 5. 验证
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("ls -l %s", destPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	i.logger.WithField("output", string(output)).Info("System certificate installed successfully")
	return nil
}

// IsInstalled 检查证书是否已安装（优先检查用户证书）
func (i *Installer) IsInstalled(ctx context.Context, certHash string) bool {
	certFile := fmt.Sprintf("%s.0", certHash)

	// 优先检查用户证书目录
	userCertPath := fmt.Sprintf("/data/misc/user/0/cacerts-added/%s", certFile)
	cmd := exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("ls %s", userCertPath))
	output, err := cmd.CombinedOutput()

	if err == nil && strings.Contains(string(output), certHash) {
		i.logger.WithField("cert_hash", certHash).Debug("User certificate found")
		return true
	}

	// 回退检查系统证书目录（向后兼容）
	systemCertPath := fmt.Sprintf("/system/etc/security/cacerts/%s", certFile)
	cmd = exec.CommandContext(ctx, "adb", "-s", i.adbTarget, "shell", fmt.Sprintf("ls %s", systemCertPath))
	output, err = cmd.CombinedOutput()

	if err == nil && strings.Contains(string(output), certHash) {
		i.logger.WithField("cert_hash", certHash).Debug("System certificate found")
		return true
	}

	return false
}

// PrepareAndInstall 完整的证书准备和安装流程（用户证书方式）
// 功能：从 mitmproxy 容器导出证书 -> 计算 Hash -> 推送到模拟器 -> 安装到用户证书目录
// 路径：/data/misc/user/0/cacerts-added/
// 优点：不需要修改 /system，不需要重启，配合 Frida 可拦截 80-90% 应用
func (i *Installer) PrepareAndInstall(ctx context.Context, mitmproxyContainer string) error {
	i.logger.WithFields(logrus.Fields{
		"device":              i.adbTarget,
		"mitmproxy_container": mitmproxyContainer,
	}).Info("Starting complete certificate preparation and installation (user certificate)")

	// 执行用户证书安装脚本
	scriptPath := "./scripts/install_user_cert.sh"
	cmd := exec.CommandContext(ctx, "bash", scriptPath, i.adbTarget, mitmproxyContainer)

	output, err := cmd.CombinedOutput()

	// 记录脚本输出
	outputStr := string(output)
	if len(outputStr) > 0 {
		for _, line := range strings.Split(outputStr, "\n") {
			if line != "" {
				i.logger.Info(line)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("user certificate installation failed: %w, output: %s", err, outputStr)
	}

	i.logger.Info("User certificate installed successfully, HTTPS interception enabled with Frida SSL Unpinning")
	return nil
}

// PrepareAndInstallSystemCert 完整的证书准备和安装流程（系统证书方式 - 已废弃）
// 功能：从 mitmproxy 容器导出证书 -> 计算 Hash -> 推送到模拟器 -> 安装到系统证书目录
// 注意：此方法已废弃，因为 Android 11+ 的 dm-verity 保护使得修改 /system 需要 overlayfs + 重启
// 推荐使用 PrepareAndInstall() 方法安装用户证书
func (i *Installer) PrepareAndInstallSystemCert(ctx context.Context, mitmproxyContainer string) error {
	i.logger.WithFields(logrus.Fields{
		"device":              i.adbTarget,
		"mitmproxy_container": mitmproxyContainer,
	}).Warn("Using deprecated system certificate installation method")

	// 执行系统证书安装脚本
	scriptPath := "./scripts/prepare_and_install_cert.sh"
	cmd := exec.CommandContext(ctx, "bash", scriptPath, i.adbTarget, mitmproxyContainer)

	output, err := cmd.CombinedOutput()

	// 记录脚本输出
	outputStr := string(output)
	if len(outputStr) > 0 {
		for _, line := range strings.Split(outputStr, "\n") {
			if line != "" {
				i.logger.Info(line)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("system certificate installation failed: %w, output: %s", err, outputStr)
	}

	i.logger.Info("System certificate installed successfully")
	return nil
}
