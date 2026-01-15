package unpacker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// MergeDEXFiles 合并多个DEX文件
// 使用Android SDK的d8工具进行合并，如果d8不可用则使用第一个DEX
func MergeDEXFiles(dexFiles []string, outputPath string) error {
	if len(dexFiles) == 0 {
		return fmt.Errorf("no DEX files to merge")
	}

	if len(dexFiles) == 1 {
		return copyFile(dexFiles[0], outputPath)
	}

	// 尝试使用d8工具合并
	if err := mergeWithD8(dexFiles, outputPath); err == nil {
		return nil
	}

	// d8不可用，尝试使用dx工具
	if err := mergeWithDX(dexFiles, outputPath); err == nil {
		return nil
	}

	// 都不可用，直接复制第一个文件（通常是主DEX）
	return copyFile(dexFiles[0], outputPath)
}

// mergeWithD8 使用d8工具合并DEX
func mergeWithD8(dexFiles []string, outputPath string) error {
	// 检查d8是否可用
	d8Path, err := exec.LookPath("d8")
	if err != nil {
		return fmt.Errorf("d8 not found in PATH")
	}

	// 创建临时输出目录
	tempDir := filepath.Dir(outputPath) + "/d8_temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// 构建d8命令
	// d8 --output <dir> <dex1> <dex2> ...
	args := []string{"--output", tempDir}
	args = append(args, dexFiles...)

	cmd := exec.Command(d8Path, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("d8 merge failed: %v, output: %s", err, string(output))
	}

	// d8输出classes.dex，移动到目标路径
	mergedDEX := filepath.Join(tempDir, "classes.dex")
	if _, err := os.Stat(mergedDEX); os.IsNotExist(err) {
		return fmt.Errorf("d8 did not produce output DEX")
	}

	return copyFile(mergedDEX, outputPath)
}

// mergeWithDX 使用dx工具合并DEX
func mergeWithDX(dexFiles []string, outputPath string) error {
	// 检查dx是否可用
	dxPath, err := exec.LookPath("dx")
	if err != nil {
		return fmt.Errorf("dx not found in PATH")
	}

	// dx --dex --output=<file> <input1> <input2> ...
	args := []string{"--dex", "--output=" + outputPath}
	args = append(args, dexFiles...)

	cmd := exec.Command(dxPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dx merge failed: %v, output: %s", err, string(output))
	}

	return nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

// ValidateMergedDEX 验证合并后的DEX是否有效
func ValidateMergedDEX(dexPath string) (bool, error) {
	file, err := os.Open(dexPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// 检查DEX magic
	magic := make([]byte, 8)
	if _, err := file.Read(magic); err != nil {
		return false, err
	}

	if string(magic[:4]) != "dex\n" && string(magic[:4]) != "dey\n" {
		return false, fmt.Errorf("invalid DEX magic: %v", magic[:4])
	}

	// 检查文件大小
	info, err := file.Stat()
	if err != nil {
		return false, err
	}

	if info.Size() < 112 {
		return false, fmt.Errorf("DEX file too small: %d bytes", info.Size())
	}

	return true, nil
}

// GetDEXInfo 获取DEX文件基本信息
type DEXInfo struct {
	FilePath   string
	FileSize   int64
	Version    string
	IsValid    bool
}

// GetDEXInfo 获取DEX文件信息
func GetDEXInfo(dexPath string) (*DEXInfo, error) {
	info := &DEXInfo{
		FilePath: dexPath,
	}

	file, err := os.Open(dexPath)
	if err != nil {
		return info, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return info, err
	}
	info.FileSize = stat.Size()

	// 读取magic和version
	header := make([]byte, 8)
	if _, err := file.Read(header); err != nil {
		return info, err
	}

	if string(header[:4]) == "dex\n" || string(header[:4]) == "dey\n" {
		info.IsValid = true
		info.Version = string(header[4:7])
	}

	return info, nil
}
