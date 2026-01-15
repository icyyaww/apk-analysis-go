package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// PackerHandler 壳检测和脱壳处理器
type PackerHandler struct {
	taskService   service.TaskService
	unpackingRepo repository.UnpackingRepository
	staticRepo    repository.StaticReportRepository
	logger        *logrus.Logger
}

// NewPackerHandler 创建壳检测处理器
func NewPackerHandler(
	taskService service.TaskService,
	unpackingRepo repository.UnpackingRepository,
	staticRepo repository.StaticReportRepository,
	logger *logrus.Logger,
) *PackerHandler {
	return &PackerHandler{
		taskService:   taskService,
		unpackingRepo: unpackingRepo,
		staticRepo:    staticRepo,
		logger:        logger,
	}
}

// GetPackerDetection 获取任务的壳检测结果
// GET /api/tasks/:id/packer-detection
func (h *PackerHandler) GetPackerDetection(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	if task.StaticReport == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "静态分析报告不存在",
		})
		return
	}

	// 解析壳检测指标
	var indicators []string
	if task.StaticReport.PackerIndicators != "" {
		if err := json.Unmarshal([]byte(task.StaticReport.PackerIndicators), &indicators); err != nil {
			h.logger.WithError(err).Warn("Failed to parse packer indicators")
		}
	}

	response := gin.H{
		"task_id":               taskID,
		"is_packed":             task.StaticReport.IsPacked,
		"packer_name":           task.StaticReport.PackerName,
		"packer_type":           task.StaticReport.PackerType,
		"packer_confidence":     task.StaticReport.PackerConfidence,
		"packer_indicators":     indicators,
		"needs_dynamic_unpacking": task.StaticReport.NeedsDynamicUnpacking,
		"detection_duration_ms": task.StaticReport.PackerDetectionDurationMs,
	}

	c.JSON(http.StatusOK, response)
}

// GetUnpackingResult 获取任务的脱壳结果
// GET /api/tasks/:id/unpacking-result
func (h *PackerHandler) GetUnpackingResult(c *gin.Context) {
	taskID := c.Param("id")

	result, err := h.unpackingRepo.GetByTaskID(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "脱壳结果不存在",
		})
		return
	}

	// 解析DEX路径列表
	var dexPaths []string
	if result.DumpedDexPaths != "" {
		if err := json.Unmarshal([]byte(result.DumpedDexPaths), &dexPaths); err != nil {
			h.logger.WithError(err).Warn("Failed to parse dumped dex paths")
		}
	}

	response := gin.H{
		"task_id":          result.TaskID,
		"status":           result.Status,
		"method":           result.Method,
		"dumped_dex_count": result.DumpedDexCount,
		"dumped_dex_paths": dexPaths,
		"merged_dex_path":  result.MergedDexPath,
		"total_size":       result.TotalSize,
		"duration_ms":      result.DurationMs,
		"error_message":    result.ErrorMessage,
		"started_at":       result.StartedAt,
		"completed_at":     result.CompletedAt,
		"created_at":       result.CreatedAt,
	}

	c.JSON(http.StatusOK, response)
}

// GetPackerStatistics 获取壳检测统计信息
// GET /api/packer/statistics
func (h *PackerHandler) GetPackerStatistics(c *gin.Context) {
	stats, err := h.staticRepo.GetPackerStatistics(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get packer statistics")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取壳检测统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetUnpackingStatistics 获取脱壳统计信息
// GET /api/unpacking/statistics
func (h *PackerHandler) GetUnpackingStatistics(c *gin.Context) {
	stats, err := h.unpackingRepo.GetStatistics(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get unpacking statistics")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取脱壳统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetPackerAndUnpackingOverview 获取壳检测和脱壳综合概览
// GET /api/security/overview
func (h *PackerHandler) GetPackerAndUnpackingOverview(c *gin.Context) {
	// 获取壳检测统计
	packerStats, err := h.staticRepo.GetPackerStatistics(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Warn("Failed to get packer statistics")
		packerStats = &repository.PackerStatistics{}
	}

	// 获取脱壳统计
	unpackStats, err := h.unpackingRepo.GetStatistics(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Warn("Failed to get unpacking statistics")
		unpackStats = &repository.UnpackingStatistics{}
	}

	c.JSON(http.StatusOK, gin.H{
		"packer_statistics":    packerStats,
		"unpacking_statistics": unpackStats,
	})
}
