package frida

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Client Frida 客户端
type Client struct {
	adbTarget       string         // ADB 设备地址
	fridaHost       string         // Frida 网络连接地址（WiFi 模式），如 "192.168.2.34:27042"
	fridaServer     string         // frida-server 二进制路径
	devicePath      string         // 设备上 frida-server 路径
	logger          *logrus.Logger
	serverCmd       *exec.Cmd      // frida-server 进程
	isServerRunning bool
}

// NewClient 创建 Frida 客户端
func NewClient(adbTarget string, logger *logrus.Logger) *Client {
	return &Client{
		adbTarget:   adbTarget,
		fridaHost:   "",  // 空表示使用 USB 模式
		fridaServer: "./bin/frida-server",
		devicePath:  "/data/local/tmp/frida-server",
		logger:      logger,
	}
}

// NewClientWithHost 创建支持 WiFi 模式的 Frida 客户端
// fridaHost: Frida 服务器网络地址，如 "192.168.2.34:27042"
func NewClientWithHost(adbTarget, fridaHost string, logger *logrus.Logger) *Client {
	return &Client{
		adbTarget:   adbTarget,
		fridaHost:   fridaHost,
		fridaServer: "./bin/frida-server",
		devicePath:  "/data/local/tmp/frida-server",
		logger:      logger,
	}
}

// SetupServer 部署 frida-server 到设备
func (c *Client) SetupServer(ctx context.Context) error {
	c.logger.Info("Setting up frida-server on device")

	// 1. 检查本地 frida-server 文件
	if _, err := os.Stat(c.fridaServer); os.IsNotExist(err) {
		return fmt.Errorf("frida-server binary not found at %s, please download it first", c.fridaServer)
	}

	// 2. Push frida-server 到设备
	c.logger.Info("Pushing frida-server to device")
	cmd := exec.CommandContext(ctx, "adb", "-s", c.adbTarget, "push", c.fridaServer, c.devicePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push frida-server: %w, output: %s", err, string(output))
	}

	// 3. 设置可执行权限
	c.logger.Info("Setting executable permission")
	cmd = exec.CommandContext(ctx, "adb", "-s", c.adbTarget, "shell", "chmod", "755", c.devicePath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to chmod frida-server: %w, output: %s", err, string(output))
	}

	c.logger.Info("frida-server setup completed")
	return nil
}

// StartServer 启动 frida-server
func (c *Client) StartServer(ctx context.Context) error {
	if c.isServerRunning {
		c.logger.Warn("frida-server is already running")
		return nil
	}

	c.logger.Info("Starting frida-server")

	// 启动 frida-server (后台运行)
	cmd := exec.CommandContext(ctx, "adb", "-s", c.adbTarget, "shell", c.devicePath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start frida-server: %w", err)
	}

	c.serverCmd = cmd
	c.isServerRunning = true

	// 等待 frida-server 启动
	time.Sleep(2 * time.Second)

	// 验证 frida-server 是否运行
	verifyCmd := exec.CommandContext(ctx, "adb", "-s", c.adbTarget, "shell", "ps | grep frida-server")
	output, _ := verifyCmd.CombinedOutput()
	if !strings.Contains(string(output), "frida-server") {
		c.isServerRunning = false
		return fmt.Errorf("frida-server failed to start")
	}

	c.logger.Info("frida-server started successfully")
	return nil
}

// StopServer 停止 frida-server
func (c *Client) StopServer(ctx context.Context) error {
	c.logger.Info("Stopping frida-server")

	// 杀掉设备上的 frida-server 进程
	cmd := exec.CommandContext(ctx, "adb", "-s", c.adbTarget, "shell", "killall frida-server")
	cmd.Run() // 忽略错误

	if c.serverCmd != nil && c.serverCmd.Process != nil {
		c.serverCmd.Process.Kill()
	}

	c.isServerRunning = false
	c.logger.Info("frida-server stopped")
	return nil
}

// InjectScript 注入 Frida 脚本到应用
// 根据 fridaHost 配置自动选择连接模式：
//   - fridaHost 为空: 使用 USB 模式 (-U)
//   - fridaHost 不为空: 使用网络模式 (-H host:port)
func (c *Client) InjectScript(ctx context.Context, packageName, scriptPath string) error {
	c.logger.WithFields(logrus.Fields{
		"package":    packageName,
		"script":     scriptPath,
		"frida_host": c.fridaHost,
	}).Info("Injecting Frida script")

	// 检查脚本文件
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script file not found: %s", scriptPath)
	}

	// 构建 frida 命令参数
	var args []string
	if c.fridaHost != "" {
		// WiFi 模式：使用 -H 参数连接到远程 frida-server
		c.logger.WithField("frida_host", c.fridaHost).Info("Using WiFi mode for Frida connection")
		args = []string{
			"-H", c.fridaHost,   // 网络连接到 frida-server
			"-f", packageName,   // 启动应用
			"-l", scriptPath,    // 加载脚本
			"--no-pause",        // 不暂停
		}
	} else {
		// USB 模式：使用 -U 参数
		c.logger.Info("Using USB mode for Frida connection")
		args = []string{
			"-U",                // USB 设备
			"-f", packageName,   // 启动应用
			"-l", scriptPath,    // 加载脚本
			"--no-pause",        // 不暂停
		}
	}

	cmd := exec.CommandContext(ctx, "frida", args...)

	// 后台运行
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to inject script: %w", err)
	}

	c.logger.Info("Frida script injected successfully")
	return nil
}

// InjectSSLUnpinning 注入 SSL Unpinning 脚本（增强版，支持加固壳）
func (c *Client) InjectSSLUnpinning(ctx context.Context, packageName string) error {
	c.logger.WithField("package", packageName).Info("Injecting Advanced SSL Unpinning script")

	// ✅ 使用增强版 SSL Unpinning 脚本（支持加固壳 + OkHttp Platform.buildCertificateChainCleaner 修复）
	scriptPath := "./scripts/frida_ssl_unpinning_advanced.js"

	// 检查脚本是否存在
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// 回退到旧脚本（兼容性保障）
		c.logger.Warn("Advanced SSL unpinning script not found, falling back to default script")
		scriptPath = "./scripts/ssl_unpinning.js"

		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			// 如果两个脚本都不存在，创建默认脚本
			if err := c.createDefaultSSLUnpinningScript(scriptPath); err != nil {
				return fmt.Errorf("failed to create SSL unpinning script: %w", err)
			}
		}
	} else {
		c.logger.Info("Using advanced SSL unpinning script (packer support enabled)")
	}

	return c.InjectScript(ctx, packageName, scriptPath)
}

// createDefaultSSLUnpinningScript 创建默认的 SSL Unpinning 脚本
func (c *Client) createDefaultSSLUnpinningScript(scriptPath string) error {
	// 确保目录存在
	dir := filepath.Dir(scriptPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 读取嵌入的脚本内容
	script := getDefaultSSLUnpinningScript()

	// 写入文件
	return os.WriteFile(scriptPath, []byte(script), 0644)
}

// getDefaultSSLUnpinningScript 返回默认的 SSL Unpinning JavaScript 代码
func getDefaultSSLUnpinningScript() string {
	return `/**
 * Universal SSL Unpinning Script for Android
 * Supports multiple SSL pinning implementations
 */

Java.perform(function() {
    console.log("[*] SSL Unpinning script loaded");

    // ============================================
    // 1. OkHttp 3.x CertificatePinner
    // ============================================
    try {
        var CertificatePinner = Java.use("okhttp3.CertificatePinner");
        CertificatePinner.check.overload('java.lang.String', 'java.util.List').implementation = function(hostname, peerCertificates) {
            console.log("[+] OkHttp3 CertificatePinner.check() bypassed for: " + hostname);
            return;
        };
        console.log("[+] OkHttp3 CertificatePinner hooked");
    } catch (e) {
        console.log("[-] OkHttp3 CertificatePinner not found");
    }

    // ============================================
    // 2. TrustManager (javax.net.ssl)
    // ============================================
    try {
        var X509TrustManager = Java.use('javax.net.ssl.X509TrustManager');
        var SSLContext = Java.use('javax.net.ssl.SSLContext');

        // 创建一个接受所有证书的 TrustManager
        var TrustManager = Java.registerClass({
            name: 'com.fake.TrustManager',
            implements: [X509TrustManager],
            methods: {
                checkClientTrusted: function(chain, authType) {},
                checkServerTrusted: function(chain, authType) {},
                getAcceptedIssuers: function() {
                    return [];
                }
            }
        });

        // Hook SSLContext.init()
        SSLContext.init.overload('[Ljavax.net.ssl.KeyManager;', '[Ljavax.net.ssl.TrustManager;', 'java.security.SecureRandom').implementation = function(keyManager, trustManager, secureRandom) {
            console.log("[+] SSLContext.init() called, replacing TrustManager");
            this.init(keyManager, [TrustManager.$new()], secureRandom);
        };

        console.log("[+] TrustManager hooked");
    } catch (e) {
        console.log("[-] TrustManager hook failed: " + e);
    }

    // ============================================
    // 3. WebViewClient (Android WebView)
    // ============================================
    try {
        var WebViewClient = Java.use("android.webkit.WebViewClient");
        WebViewClient.onReceivedSslError.implementation = function(view, handler, error) {
            console.log("[+] WebViewClient.onReceivedSslError() bypassed");
            handler.proceed();
        };
        console.log("[+] WebViewClient hooked");
    } catch (e) {
        console.log("[-] WebViewClient not found");
    }

    // ============================================
    // 4. Apache HttpClient (Legacy)
    // ============================================
    try {
        var AbstractVerifier = Java.use("org.apache.http.conn.ssl.AbstractVerifier");
        AbstractVerifier.verify.overload('java.lang.String', '[Ljava.lang.String', '[Ljava.lang.String', 'boolean').implementation = function(host, cns, subjectAlts, strictWithSubDomains) {
            console.log("[+] Apache HttpClient SSL verification bypassed for: " + host);
            return;
        };
        console.log("[+] Apache HttpClient hooked");
    } catch (e) {
        console.log("[-] Apache HttpClient not found");
    }

    // ============================================
    // 5. HostnameVerifier
    // ============================================
    try {
        var HostnameVerifier = Java.use("javax.net.ssl.HostnameVerifier");
        HostnameVerifier.verify.overload('java.lang.String', 'javax.net.ssl.SSLSession').implementation = function(hostname, session) {
            console.log("[+] HostnameVerifier.verify() bypassed for: " + hostname);
            return true;
        };
        console.log("[+] HostnameVerifier hooked");
    } catch (e) {
        console.log("[-] HostnameVerifier hook failed");
    }

    // ============================================
    // 6. Conscrypt (Android's SSL provider)
    // ============================================
    try {
        var ConscryptFileDescriptorSocket = Java.use("com.android.org.conscrypt.ConscryptFileDescriptorSocket");
        ConscryptFileDescriptorSocket.verifyCertificateChain.implementation = function(certChain, authMethod) {
            console.log("[+] Conscrypt certificate verification bypassed");
        };
        console.log("[+] Conscrypt hooked");
    } catch (e) {
        console.log("[-] Conscrypt not found");
    }

    console.log("[*] SSL Unpinning hooks installed successfully");
});
`
}

// IsServerRunning 检查 frida-server 是否运行
func (c *Client) IsServerRunning(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "adb", "-s", c.adbTarget, "shell", "ps | grep frida-server")
	output, _ := cmd.CombinedOutput()
	return strings.Contains(string(output), "frida-server")
}
