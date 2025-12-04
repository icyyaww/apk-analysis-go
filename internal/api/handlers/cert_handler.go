package handlers

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CertHandler 证书状态处理器
type CertHandler struct {
	adbTarget string
	logger    *logrus.Logger
}

// NewCertHandler 创建证书状态处理器
func NewCertHandler(adbTarget string, logger *logrus.Logger) *CertHandler {
	return &CertHandler{
		adbTarget: adbTarget,
		logger:    logger,
	}
}

// CertStatus 证书状态响应
type CertStatus struct {
	Installed        bool      `json:"installed"`
	UserCertInstalled bool     `json:"user_cert_installed"`
	SystemCertInstalled bool   `json:"system_cert_installed"`
	CertHash         string    `json:"cert_hash"`
	LastChecked      time.Time `json:"last_checked"`
	DeviceConnected  bool      `json:"device_connected"`
	Error            string    `json:"error,omitempty"`
}

// GetCertStatus 获取证书状态
func (h *CertHandler) GetCertStatus(c *gin.Context) {
	ctx := context.Background()
	certHash := "c8750f0d" // mitmproxy 默认证书哈希

	// 使用实际的模拟器设备地址
	deviceTarget := "android-emulator-1:5555"

	status := &CertStatus{
		CertHash:    certHash,
		LastChecked: time.Now(),
	}

	// 检查设备连接
	deviceConnected := h.checkDeviceConnection(ctx, deviceTarget)
	status.DeviceConnected = deviceConnected

	if !deviceConnected {
		status.Error = "设备未连接"
		c.JSON(200, status)
		return
	}

	// 检查用户证书
	userCertInstalled := h.checkUserCert(ctx, deviceTarget, certHash)
	status.UserCertInstalled = userCertInstalled

	// 检查系统证书
	systemCertInstalled := h.checkSystemCert(ctx, deviceTarget, certHash)
	status.SystemCertInstalled = systemCertInstalled

	// 只要有一个证书安装了就算安装成功
	status.Installed = userCertInstalled || systemCertInstalled

	c.JSON(200, status)
}

// checkDeviceConnection 检查设备连接
func (h *CertHandler) checkDeviceConnection(ctx context.Context, deviceTarget string) bool {
	cmd := exec.CommandContext(ctx, "adb", "-s", deviceTarget, "shell", "echo 'ping'")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return false
	}

	return strings.Contains(string(output), "ping")
}

// checkUserCert 检查用户证书
func (h *CertHandler) checkUserCert(ctx context.Context, deviceTarget string, certHash string) bool {
	certFile := certHash + ".0"
	userCertPath := "/data/misc/user/0/cacerts-added/" + certFile

	cmd := exec.CommandContext(ctx, "adb", "-s", deviceTarget, "shell", "ls "+userCertPath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return false
	}

	return strings.Contains(string(output), certHash)
}

// checkSystemCert 检查系统证书
func (h *CertHandler) checkSystemCert(ctx context.Context, deviceTarget string, certHash string) bool {
	certFile := certHash + ".0"
	systemCertPath := "/system/etc/security/cacerts/" + certFile

	cmd := exec.CommandContext(ctx, "adb", "-s", deviceTarget, "shell", "ls "+systemCertPath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return false
	}

	return strings.Contains(string(output), certHash)
}
