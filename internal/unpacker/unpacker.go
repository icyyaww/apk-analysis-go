package unpacker

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

// DynamicUnpacker 动态脱壳器
type DynamicUnpacker struct {
	logger         *logrus.Logger
	scriptsDir     string
	defaultTimeout time.Duration
}

// NewDynamicUnpacker 创建动态脱壳器
func NewDynamicUnpacker(logger *logrus.Logger, scriptsDir string) *DynamicUnpacker {
	if scriptsDir == "" {
		scriptsDir = "./scripts/unpacker"
	}
	return &DynamicUnpacker{
		logger:         logger,
		scriptsDir:     scriptsDir,
		defaultTimeout: 60 * time.Second,
	}
}

// Unpack 执行动态脱壳
func (u *DynamicUnpacker) Unpack(ctx context.Context, req UnpackRequest) (*UnpackResult, error) {
	startTime := time.Now()
	result := &UnpackResult{
		Method:    req.PackerInfo.UnpackMethod,
		StartedAt: startTime,
		Status:    UnpackStatusRunning,
	}

	u.logger.WithFields(logrus.Fields{
		"task_id":     req.TaskID,
		"package":     req.PackageName,
		"packer":      req.PackerInfo.PackerName,
		"method":      req.PackerInfo.UnpackMethod,
		"output_dir":  req.OutputDir,
	}).Info("Starting dynamic unpacking")

	// 检查是否支持脱壳
	if !req.PackerInfo.CanUnpack {
		result.Status = UnpackStatusSkipped
		result.Error = "Packer does not support automatic unpacking"
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Milliseconds()
		return result, fmt.Errorf(result.Error)
	}

	// 创建输出目录
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		result.Status = UnpackStatusFailed
		result.Error = fmt.Sprintf("Failed to create output dir: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Milliseconds()
		return result, err
	}

	// 设置超时
	timeout := req.Timeout
	if timeout == 0 {
		timeout = u.defaultTimeout
	}

	// 根据脱壳方法选择脚本
	var scriptPath string
	switch req.PackerInfo.UnpackMethod {
	case "frida_dex_dump":
		scriptPath = filepath.Join(u.scriptsDir, "dex_dump.js")
	case "frida_class_loader":
		scriptPath = filepath.Join(u.scriptsDir, "class_loader.js")
	default:
		scriptPath = filepath.Join(u.scriptsDir, "dex_dump.js")
	}

	// 检查脚本是否存在
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		result.Status = UnpackStatusFailed
		result.Error = fmt.Sprintf("Unpacking script not found: %s", scriptPath)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Milliseconds()
		return result, err
	}

	// 执行Frida脱壳
	dumpedFiles, err := u.executeFridaUnpack(ctx, req, scriptPath, timeout)
	if err != nil {
		result.Status = UnpackStatusFailed
		result.Error = err.Error()
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Milliseconds()

		// 即使Frida执行出错，也尝试拉取已经dump的文件
		if pulledFiles, _ := u.pullDumpedFiles(ctx, req.ADBTarget, req.OutputDir); len(pulledFiles) > 0 {
			result.DumpedDEXs = pulledFiles
			result.DEXCount = len(pulledFiles)
			result.Success = true
			result.Status = UnpackStatusSuccess
			result.Error = ""
		}
	} else {
		result.DumpedDEXs = dumpedFiles
		result.DEXCount = len(dumpedFiles)
		result.Success = len(dumpedFiles) > 0
		if result.Success {
			result.Status = UnpackStatusSuccess
		}
	}

	// 计算总大小
	for _, dexPath := range result.DumpedDEXs {
		if info, err := os.Stat(dexPath); err == nil {
			result.TotalSize += info.Size()
		}
	}

	// 合并多个DEX文件
	if len(result.DumpedDEXs) > 0 {
		if len(result.DumpedDEXs) > 1 {
			mergedPath := filepath.Join(req.OutputDir, "merged.dex")
			if err := MergeDEXFiles(result.DumpedDEXs, mergedPath); err != nil {
				u.logger.WithError(err).Warn("Failed to merge DEX files, using first DEX")
				result.MergedDEXPath = result.DumpedDEXs[0]
			} else {
				result.MergedDEXPath = mergedPath
			}
		} else {
			result.MergedDEXPath = result.DumpedDEXs[0]
		}
	}

	result.CompletedAt = time.Now()
	result.Duration = time.Since(startTime).Milliseconds()

	u.logger.WithFields(logrus.Fields{
		"success":      result.Success,
		"dex_count":    result.DEXCount,
		"total_size":   result.TotalSize,
		"duration_ms":  result.Duration,
		"merged_path":  result.MergedDEXPath,
	}).Info("Dynamic unpacking completed")

	return result, nil
}

// executeFridaUnpack 执行Frida脱壳
func (u *DynamicUnpacker) executeFridaUnpack(ctx context.Context, req UnpackRequest, scriptPath string, timeout time.Duration) ([]string, error) {
	// 设置设备上的Dump输出目录
	deviceDumpDir := "/data/local/tmp/dex_dump"

	// 清理旧的Dump文件
	cleanCmd := exec.CommandContext(ctx, "adb", "-s", req.ADBTarget, "shell",
		fmt.Sprintf("rm -rf %s && mkdir -p %s && chmod 777 %s", deviceDumpDir, deviceDumpDir, deviceDumpDir))
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		u.logger.WithError(err).WithField("output", string(output)).Warn("Failed to clean dump directory")
	}

	// 构建Frida命令
	var fridaArgs []string
	if req.FridaHost != "" {
		// WiFi模式
		fridaArgs = []string{
			"-H", req.FridaHost,
			"-f", req.PackageName,
			"-l", scriptPath,
			"--no-pause",
		}
	} else {
		// USB模式
		fridaArgs = []string{
			"-U",
			"-f", req.PackageName,
			"-l", scriptPath,
			"--no-pause",
		}
	}

	u.logger.WithFields(logrus.Fields{
		"args":        fridaArgs,
		"script_path": scriptPath,
		"timeout":     timeout.String(),
	}).Debug("Executing Frida unpacking script")

	// 创建超时上下文
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fridaCmd := exec.CommandContext(ctxWithTimeout, "frida", fridaArgs...)

	// 执行并等待
	done := make(chan error, 1)
	go func() {
		output, err := fridaCmd.CombinedOutput()
		u.logger.WithField("output", string(output)).Debug("Frida script output")
		done <- err
	}()

	select {
	case <-ctxWithTimeout.Done():
		if fridaCmd.Process != nil {
			fridaCmd.Process.Kill()
		}
		u.logger.Warn("Frida unpacking timed out, attempting to pull dumped files")
	case err := <-done:
		if err != nil {
			u.logger.WithError(err).Warn("Frida script execution completed with error")
		} else {
			u.logger.Debug("Frida script execution completed successfully")
		}
	}

	// 等待一段时间让应用完成DEX加载
	time.Sleep(5 * time.Second)

	// 从设备拉取Dump的DEX文件
	return u.pullDumpedFiles(ctx, req.ADBTarget, req.OutputDir)
}

// pullDumpedFiles 从设备拉取Dump文件
func (u *DynamicUnpacker) pullDumpedFiles(ctx context.Context, adbTarget, localDir string) ([]string, error) {
	deviceDumpDir := "/data/local/tmp/dex_dump"

	// 列出设备上的DEX文件
	listCmd := exec.CommandContext(ctx, "adb", "-s", adbTarget, "shell",
		fmt.Sprintf("ls %s/*.dex 2>/dev/null || ls %s/*dex* 2>/dev/null || true", deviceDumpDir, deviceDumpDir))
	output, _ := listCmd.Output()

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || strings.Contains(outputStr, "No such file") {
		return nil, fmt.Errorf("no DEX files dumped on device")
	}

	// 解析文件列表
	files := strings.Split(outputStr, "\n")
	var pulledFiles []string

	for i, devicePath := range files {
		devicePath = strings.TrimSpace(devicePath)
		if devicePath == "" || strings.Contains(devicePath, "No such file") {
			continue
		}

		localPath := filepath.Join(localDir, fmt.Sprintf("dumped_%d.dex", i))

		pullCmd := exec.CommandContext(ctx, "adb", "-s", adbTarget, "pull", devicePath, localPath)
		if err := pullCmd.Run(); err != nil {
			u.logger.WithError(err).WithField("file", devicePath).Warn("Failed to pull DEX file")
			continue
		}

		// 验证文件是否有效
		if u.isValidDEX(localPath) {
			pulledFiles = append(pulledFiles, localPath)
			u.logger.WithFields(logrus.Fields{
				"device_path": devicePath,
				"local_path":  localPath,
			}).Debug("Successfully pulled DEX file")
		} else {
			u.logger.WithField("path", localPath).Warn("Invalid DEX file, removing")
			os.Remove(localPath)
		}
	}

	if len(pulledFiles) == 0 {
		return nil, fmt.Errorf("no valid DEX files dumped")
	}

	u.logger.WithField("count", len(pulledFiles)).Info("Successfully pulled DEX files from device")
	return pulledFiles, nil
}

// isValidDEX 检查DEX文件是否有效
func (u *DynamicUnpacker) isValidDEX(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// 检查文件大小
	info, err := file.Stat()
	if err != nil || info.Size() < 112 { // DEX header最小size
		return false
	}

	// 检查DEX magic number
	magic := make([]byte, 8)
	if _, err := file.Read(magic); err != nil {
		return false
	}

	// DEX magic: "dex\n035\0" 或 "dex\n036\0" 等
	if string(magic[:4]) == "dex\n" {
		return true
	}

	// ODEX magic: "dey\n036\0"
	if string(magic[:4]) == "dey\n" {
		return true
	}

	return false
}

// CleanupDeviceDumpDir 清理设备上的Dump目录
func (u *DynamicUnpacker) CleanupDeviceDumpDir(ctx context.Context, adbTarget string) error {
	deviceDumpDir := "/data/local/tmp/dex_dump"
	cmd := exec.CommandContext(ctx, "adb", "-s", adbTarget, "shell", "rm", "-rf", deviceDumpDir)
	return cmd.Run()
}
