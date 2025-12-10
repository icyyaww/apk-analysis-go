package handlers

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// FileHandler 文件处理器
type FileHandler struct {
	taskService service.TaskService
	logger      *logrus.Logger
	resultsPath string // results 目录路径
	inboundPath string // inbound_apks 目录路径
}

// NewFileHandler 创建文件处理器实例
func NewFileHandler(taskService service.TaskService, logger *logrus.Logger, resultsPath string, inboundPath string) *FileHandler {
	return &FileHandler{
		taskService: taskService,
		logger:      logger,
		resultsPath: resultsPath,
		inboundPath: inboundPath,
	}
}

// GetScreenshot 获取任务截图
// GET /api/tasks/:id/screenshot/:filename
func (h *FileHandler) GetScreenshot(c *gin.Context) {
	taskID := c.Param("id")
	filename := c.Param("filename")

	// 验证任务是否存在
	_, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 构建文件路径
	filePath := filepath.Join(h.resultsPath, taskID, "screenshots", filename)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "截图文件不存在",
		})
		return
	}

	// 返回图片文件
	c.File(filePath)
}

// GetUIHierarchy 获取 UI 层级 XML 并解析
// GET /api/tasks/:id/ui_hierarchy/:filename
func (h *FileHandler) GetUIHierarchy(c *gin.Context) {
	taskID := c.Param("id")
	filename := c.Param("filename")

	// 验证任务是否存在
	_, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 构建文件路径
	filePath := filepath.Join(h.resultsPath, taskID, "ui_hierarchy", filename)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "UI 层级文件不存在",
		})
		return
	}

	// 读取 XML 文件
	xmlData, err := os.ReadFile(filePath)
	if err != nil {
		h.logger.WithError(err).Error("Failed to read UI hierarchy file")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取 UI 层级文件失败",
		})
		return
	}

	// 解析 XML
	var hierarchy UIHierarchy
	if err := xml.Unmarshal(xmlData, &hierarchy); err != nil {
		h.logger.WithError(err).Error("Failed to parse UI hierarchy XML")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "解析 UI 层级 XML 失败",
		})
		return
	}

	// 返回解析后的 JSON
	c.JSON(http.StatusOK, hierarchy)
}

// ListScreenshots 列出任务的所有截图
// GET /api/tasks/:id/screenshots
func (h *FileHandler) ListScreenshots(c *gin.Context) {
	taskID := c.Param("id")

	// 验证任务是否存在
	_, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 截图目录路径
	screenshotDir := filepath.Join(h.resultsPath, taskID, "screenshots")

	// 读取目录
	files, err := os.ReadDir(screenshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, []string{})
			return
		}
		h.logger.WithError(err).Error("Failed to read screenshot directory")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取截图目录失败",
		})
		return
	}

	// 过滤出 PNG 文件
	var screenshots []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".png" {
			screenshots = append(screenshots, file.Name())
		}
	}

	c.JSON(http.StatusOK, screenshots)
}

// UIHierarchy UI 层级结构
type UIHierarchy struct {
	XMLName  xml.Name `xml:"hierarchy" json:"-"`
	Rotation int      `xml:"rotation,attr" json:"rotation"`
	Root     UINode   `xml:"node" json:"root"`
}

// UINode UI 节点
type UINode struct {
	Index       int      `xml:"index,attr" json:"index"`
	Text        string   `xml:"text,attr" json:"text,omitempty"`
	ResourceID  string   `xml:"resource-id,attr" json:"resource_id,omitempty"`
	Class       string   `xml:"class,attr" json:"class"`
	Package     string   `xml:"package,attr" json:"package,omitempty"`
	Bounds      string   `xml:"bounds,attr" json:"bounds"`
	Clickable   bool     `xml:"clickable,attr" json:"clickable"`
	Enabled     bool     `xml:"enabled,attr" json:"enabled"`
	Focusable   bool     `xml:"focusable,attr" json:"focusable"`
	Scrollable  bool     `xml:"scrollable,attr" json:"scrollable"`
	ContentDesc string   `xml:"content-desc,attr" json:"content_desc,omitempty"`
	Children    []UINode `xml:"node" json:"children,omitempty"`
}

// DownloadFlows 下载流量数据文件
// GET /api/tasks/:id/flows
func (h *FileHandler) DownloadFlows(c *gin.Context) {
	taskID := c.Param("id")

	// 验证任务是否存在
	_, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 构建文件路径
	filePath := filepath.Join(h.resultsPath, taskID, "flows.jsonl")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "流量数据文件不存在",
		})
		return
	}

	// 设置下载文件名
	downloadName := fmt.Sprintf("flows_%s.jsonl", taskID[:8])
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", downloadName))
	c.Header("Content-Type", "application/jsonl")

	// 返回文件
	c.File(filePath)
}

// UploadAPK 上传 APK 文件
// POST /api/upload
func (h *FileHandler) UploadAPK(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.WithError(err).Error("Failed to get uploaded file")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "获取上传文件失败",
		})
		return
	}

	// 验证文件扩展名
	filename := file.Filename
	if !strings.HasSuffix(strings.ToLower(filename), ".apk") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "只支持 APK 文件格式",
		})
		return
	}

	// 验证文件大小 (最大 500MB)
	maxSize := int64(500 * 1024 * 1024) // 500MB
	if file.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("文件大小超过限制 (最大 %dMB)", maxSize/(1024*1024)),
		})
		return
	}

	// 确保 inbound_apks 目录存在
	if err := os.MkdirAll(h.inboundPath, 0755); err != nil {
		h.logger.WithError(err).Error("Failed to create inbound directory")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建上传目录失败",
		})
		return
	}

	// 构建目标文件路径
	destPath := filepath.Join(h.inboundPath, filename)

	// 检查文件是否已存在
	if _, err := os.Stat(destPath); err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "文件已存在",
			"filename": filename,
		})
		return
	}

	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		h.logger.WithError(err).Error("Failed to open uploaded file")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "打开上传文件失败",
		})
		return
	}
	defer src.Close()

	// 创建目标文件
	dst, err := os.Create(destPath)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create destination file")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建目标文件失败",
		})
		return
	}
	defer dst.Close()

	// 复制文件内容
	written, err := io.Copy(dst, src)
	if err != nil {
		h.logger.WithError(err).Error("Failed to copy file")
		// 删除不完整的文件
		os.Remove(destPath)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "文件上传失败",
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"filename": filename,
		"size":     written,
		"path":     destPath,
	}).Info("APK file uploaded successfully")

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message":  "文件上传成功",
		"filename": filename,
		"size":     written,
		"path":     destPath,
	})
}

// UploadAPKBatch 批量上传 APK 文件
// POST /api/upload/batch
func (h *FileHandler) UploadAPKBatch(c *gin.Context) {
	// 获取上传的多个文件
	form, err := c.MultipartForm()
	if err != nil {
		h.logger.WithError(err).Error("Failed to parse multipart form")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "解析上传表单失败",
		})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请选择要上传的 APK 文件",
		})
		return
	}

	// 最大同时上传数量限制
	maxFiles := 100
	if len(files) > maxFiles {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("最多同时上传 %d 个文件，当前选择了 %d 个", maxFiles, len(files)),
		})
		return
	}

	// 确保 inbound_apks 目录存在
	if err := os.MkdirAll(h.inboundPath, 0755); err != nil {
		h.logger.WithError(err).Error("Failed to create inbound directory")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建上传目录失败",
		})
		return
	}

	// 处理结果
	type UploadResult struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Status   string `json:"status"` // success, error, skipped
		Error    string `json:"error,omitempty"`
	}

	results := make([]UploadResult, 0, len(files))
	successCount := 0
	errorCount := 0
	skippedCount := 0

	maxSize := int64(500 * 1024 * 1024) // 500MB

	for _, file := range files {
		result := UploadResult{
			Filename: file.Filename,
			Size:     file.Size,
		}

		// 验证文件扩展名
		if !strings.HasSuffix(strings.ToLower(file.Filename), ".apk") {
			result.Status = "error"
			result.Error = "只支持 APK 文件格式"
			errorCount++
			results = append(results, result)
			continue
		}

		// 验证文件大小
		if file.Size > maxSize {
			result.Status = "error"
			result.Error = fmt.Sprintf("文件大小超过限制 (最大 %dMB)", maxSize/(1024*1024))
			errorCount++
			results = append(results, result)
			continue
		}

		// 构建目标文件路径
		destPath := filepath.Join(h.inboundPath, file.Filename)

		// 检查文件是否已存在
		if _, err := os.Stat(destPath); err == nil {
			result.Status = "skipped"
			result.Error = "文件已存在"
			skippedCount++
			results = append(results, result)
			continue
		}

		// 打开上传的文件
		src, err := file.Open()
		if err != nil {
			h.logger.WithError(err).WithField("filename", file.Filename).Error("Failed to open uploaded file")
			result.Status = "error"
			result.Error = "打开文件失败"
			errorCount++
			results = append(results, result)
			continue
		}

		// 创建目标文件
		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			h.logger.WithError(err).WithField("filename", file.Filename).Error("Failed to create destination file")
			result.Status = "error"
			result.Error = "创建目标文件失败"
			errorCount++
			results = append(results, result)
			continue
		}

		// 复制文件内容
		written, err := io.Copy(dst, src)
		src.Close()
		dst.Close()

		if err != nil {
			h.logger.WithError(err).WithField("filename", file.Filename).Error("Failed to copy file")
			os.Remove(destPath) // 删除不完整的文件
			result.Status = "error"
			result.Error = "复制文件失败"
			errorCount++
			results = append(results, result)
			continue
		}

		result.Size = written
		result.Status = "success"
		successCount++
		results = append(results, result)

		h.logger.WithFields(logrus.Fields{
			"filename": file.Filename,
			"size":     written,
		}).Info("APK file uploaded successfully (batch)")
	}

	// 返回批量上传结果
	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("批量上传完成: %d 成功, %d 失败, %d 跳过", successCount, errorCount, skippedCount),
		"total":         len(files),
		"success_count": successCount,
		"error_count":   errorCount,
		"skipped_count": skippedCount,
		"results":       results,
	})
}
