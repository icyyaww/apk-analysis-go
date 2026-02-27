package handlers

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/apk-analysis/apk-analysis-go/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"
)

// TaskHandler 任务处理器
type TaskHandler struct {
	taskService service.TaskService
	logger      *logrus.Logger
}

// NewTaskHandler 创建任务处理器实例
func NewTaskHandler(taskService service.TaskService, logger *logrus.Logger) *TaskHandler {
	return &TaskHandler{
		taskService: taskService,
		logger:      logger,
	}
}

// ListTasks 获取任务列表
// GET /api/tasks?page=1&page_size=20&status=completed&exclude_status=queued&province=广东&isp=阿里云&beian_status=已备案&search=关键词
// GET /api/tasks?completed_after=2025-01-01&completed_before=2025-12-31  // 完成时间范围
// GET /api/tasks?min_confidence=0.8&max_confidence=1.0  // 置信度范围 (0.0-1.0)
// 支持分页参数，默认每页20条
// 支持状态过滤：status=completed 或 exclude_status=queued
// 支持域名归属地过滤：province=广东&isp=阿里云
// 支持备案状态过滤：beian_status=已备案/未备案/查询失败
// 支持搜索：search=关键词（搜索APK名称、应用名称、包名）
// 支持完成时间筛选：completed_after=2025-01-01&completed_before=2025-12-31
// 支持置信度筛选：min_confidence=0.8&max_confidence=1.0
func (h *TaskHandler) ListTasks(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")
	statusFilter := c.Query("status")              // 例如: status=completed
	excludeStatus := c.Query("exclude_status")     // 例如: exclude_status=queued
	provinceFilter := c.Query("province")          // 例如: province=广东
	ispFilter := c.Query("isp")                    // 例如: isp=阿里云
	beianStatusFilter := c.Query("beian_status")   // 例如: beian_status=已备案
	searchQuery := c.Query("search")               // 例如: search=微信（搜索APK名称、应用名称、包名）
	completedAfterStr := c.Query("completed_after")   // 例如: completed_after=2025-01-01
	completedBeforeStr := c.Query("completed_before") // 例如: completed_before=2025-12-31
	minConfidenceStr := c.Query("min_confidence")     // 例如: min_confidence=0.8
	maxConfidenceStr := c.Query("max_confidence")     // 例如: max_confidence=1.0

	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize <= 0 {
		pageSize = 20
	}

	// 限制最大每页数量，防止过大的查询
	if pageSize > 100 {
		pageSize = 100
	}

	// 解析完成时间参数（使用CST时区，与用户所在时区一致）
	cst, _ := time.LoadLocation("Asia/Shanghai")
	var completedAfter, completedBefore *time.Time
	if completedAfterStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", completedAfterStr, cst); err == nil {
			// 转换为UTC以便与数据库比较
			utcTime := t.UTC()
			completedAfter = &utcTime
		} else if t, err := time.ParseInLocation(time.RFC3339, completedAfterStr, cst); err == nil {
			utcTime := t.UTC()
			completedAfter = &utcTime
		}
	}
	if completedBeforeStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", completedBeforeStr, cst); err == nil {
			// 设置为当天CST的最后一秒 (23:59:59 CST)，然后转换为UTC
			endOfDay := t.Add(24*time.Hour - time.Second).UTC()
			completedBefore = &endOfDay
		} else if t, err := time.ParseInLocation(time.RFC3339, completedBeforeStr, cst); err == nil {
			utcTime := t.UTC()
			completedBefore = &utcTime
		}
	}

	// 解析置信度参数
	var minConfidence, maxConfidence *float64
	if minConfidenceStr != "" {
		if v, err := strconv.ParseFloat(minConfidenceStr, 64); err == nil && v >= 0 && v <= 1 {
			minConfidence = &v
		}
	}
	if maxConfidenceStr != "" {
		if v, err := strconv.ParseFloat(maxConfidenceStr, 64); err == nil && v >= 0 && v <= 1 {
			maxConfidence = &v
		}
	}

	// 判断是否有高级筛选条件
	hasAdvancedFilter := provinceFilter != "" || ispFilter != "" || beianStatusFilter != "" ||
		completedAfter != nil || completedBefore != nil ||
		minConfidence != nil || maxConfidence != nil

	var tasks []*domain.Task
	var total int64

	if hasAdvancedFilter {
		// 使用新的 FilterOptions 方法
		opts := &repository.TaskFilterOptions{
			ExcludeStatus:   excludeStatus,
			StatusFilter:    statusFilter,
			Search:          searchQuery,
			Province:        provinceFilter,
			ISP:             ispFilter,
			BeianStatus:     beianStatusFilter,
			CompletedAfter:  completedAfter,
			CompletedBefore: completedBefore,
			MinConfidence:   minConfidence,
			MaxConfidence:   maxConfidence,
		}
		tasks, total, err = h.taskService.ListTasksWithFilterOptions(c.Request.Context(), page, pageSize, opts)
	} else {
		// 仅有 status 和 exclude_status 时，使用数据库分页（支持搜索）
		tasks, total, err = h.taskService.ListTasksWithSearch(c.Request.Context(), page, pageSize, excludeStatus, statusFilter, searchQuery)
	}

	if err != nil {
		h.logger.WithError(err).Error("Failed to list tasks")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取任务列表失败",
		})
		return
	}

	// 转换为响应格式
	taskList := make([]map[string]interface{}, len(tasks))
	for i, task := range tasks {
		taskList[i] = h.taskToResponse(task)
	}

	// 计算总页数
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)

	c.JSON(http.StatusOK, gin.H{
		"tasks":       taskList,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
}

// ExportTasks 导出任务列表（不分页，用于导出功能）
// GET /api/tasks/export?status=completed&province=广东&isp=阿里云&beian_status=已备案&search=关键词
// 最大返回 10000 条
func (h *TaskHandler) ExportTasks(c *gin.Context) {
	statusFilter := c.Query("status")            // 例如: status=completed
	excludeStatus := c.Query("exclude_status")   // 例如: exclude_status=queued
	provinceFilter := c.Query("province")        // 例如: province=广东
	ispFilter := c.Query("isp")                  // 例如: isp=阿里云
	beianStatusFilter := c.Query("beian_status") // 例如: beian_status=已备案
	searchQuery := c.Query("search")             // 例如: search=微信
	format := strings.ToLower(c.DefaultQuery("format", "json"))

	if format == "csv" {
		h.exportTasksCSV(c, excludeStatus, statusFilter, searchQuery, provinceFilter, ispFilter, beianStatusFilter)
		return
	}

	if format == "xlsx" {
		h.exportTasksXLSX(c, excludeStatus, statusFilter, searchQuery, provinceFilter, ispFilter, beianStatusFilter)
		return
	}

	// 转换为响应格式
	var taskList []map[string]interface{}
	err := h.forEachExportTask(c.Request.Context(), excludeStatus, statusFilter, searchQuery, provinceFilter, ispFilter, beianStatusFilter, func(task *domain.Task) error {
		taskList = append(taskList, h.taskToResponse(task))
		return nil
	})
	if err != nil {
		h.logger.WithError(err).Error("Failed to export tasks")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "导出任务列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": taskList,
		"total": len(taskList),
	})
}

func (h *TaskHandler) exportTasksCSV(
	c *gin.Context,
	excludeStatus string,
	statusFilter string,
	searchQuery string,
	provinceFilter string,
	ispFilter string,
	beianStatusFilter string,
) {
	filterDesc := buildExportFilterDescription(provinceFilter, ispFilter, beianStatusFilter, searchQuery)
	fileName := fmt.Sprintf("APP分析_%s_%s.csv", filterDesc, time.Now().Format("2006-01-02"))
	escapedFileName := url.PathEscape(fileName)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+escapedFileName)
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		h.logger.WithError(err).Warn("Failed to write CSV BOM")
		return
	}

	writer := csv.NewWriter(c.Writer)
	if err := writer.Write([]string{"APP名称", "包名", "接入域名", "接入IP", "归属地", "运营商", "备案状态", "发现时间"}); err != nil {
		h.logger.WithError(err).Warn("Failed to write CSV header")
		return
	}

	today := time.Now().Format("2006-01-02")

	err := h.forEachExportTask(c.Request.Context(), excludeStatus, statusFilter, searchQuery, provinceFilter, ispFilter, beianStatusFilter, func(task *domain.Task) error {
		appName := resolveExportAppName(task)
		packageName := task.PackageName
		beianStatus := getBeianStatusForExport(task)

		appDomains := resolveExportAppDomains(task)
		hasMatchedDomain := false
		if len(appDomains) > 0 {
			for _, domainInfo := range appDomains {
				if provinceFilter != "" && !strings.Contains(domainInfo.Province, provinceFilter) {
					continue
				}
				if ispFilter != "" && !strings.Contains(domainInfo.ISP, ispFilter) {
					continue
				}

				if err := writer.Write([]string{
					appName,
					packageName,
					domainInfo.Domain,
					domainInfo.IP,
					domainInfo.Province,
					domainInfo.ISP,
					beianStatus,
					today,
				}); err != nil {
					return err
				}
				hasMatchedDomain = true
			}
		}

		if !hasMatchedDomain {
			if err := writer.Write([]string{
				appName,
				packageName,
				"",
				"",
				"",
				"",
				beianStatus,
				today,
			}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		h.logger.WithError(err).Warn("Failed to export CSV data")
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		h.logger.WithError(err).Warn("Failed to flush CSV response")
	}
}

func (h *TaskHandler) exportTasksXLSX(
	c *gin.Context,
	excludeStatus string,
	statusFilter string,
	searchQuery string,
	provinceFilter string,
	ispFilter string,
	beianStatusFilter string,
) {
	filterDesc := buildExportFilterDescription(provinceFilter, ispFilter, beianStatusFilter, searchQuery)
	fileName := fmt.Sprintf("APP分析_%s_%s.xlsx", filterDesc, time.Now().Format("2006-01-02"))
	escapedFileName := url.PathEscape(fileName)

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+escapedFileName)
	c.Header("Cache-Control", "no-store")

	file := excelize.NewFile()
	sheetName := "Sheet1"
	displaySheetName := filterDesc
	if len(displaySheetName) > 31 {
		displaySheetName = displaySheetName[:31]
	}
	if displaySheetName == "" {
		displaySheetName = sheetName
	}
	file.SetSheetName(sheetName, displaySheetName)
	sheetName = displaySheetName

	streamWriter, err := file.NewStreamWriter(sheetName)
	if err != nil {
		h.logger.WithError(err).Warn("Failed to create XLSX stream writer")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "导出任务列表失败",
		})
		return
	}

	rowIndex := 1
	headerCell, _ := excelize.CoordinatesToCellName(1, rowIndex)
	if err := streamWriter.SetRow(headerCell, []interface{}{"APP名称", "包名", "接入域名", "接入IP", "归属地", "运营商", "备案状态", "发现时间"}); err != nil {
		h.logger.WithError(err).Warn("Failed to write XLSX header")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "导出任务列表失败",
		})
		return
	}
	rowIndex++

	today := time.Now().Format("2006-01-02")
	err = h.forEachExportTask(c.Request.Context(), excludeStatus, statusFilter, searchQuery, provinceFilter, ispFilter, beianStatusFilter, func(task *domain.Task) error {
		appName := resolveExportAppName(task)
		packageName := task.PackageName
		beianStatus := getBeianStatusForExport(task)

		appDomains := resolveExportAppDomains(task)
		hasMatchedDomain := false
		if len(appDomains) > 0 {
			for _, domainInfo := range appDomains {
				if provinceFilter != "" && !strings.Contains(domainInfo.Province, provinceFilter) {
					continue
				}
				if ispFilter != "" && !strings.Contains(domainInfo.ISP, ispFilter) {
					continue
				}

				cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
				if err := streamWriter.SetRow(cell, []interface{}{
					appName,
					packageName,
					domainInfo.Domain,
					domainInfo.IP,
					domainInfo.Province,
					domainInfo.ISP,
					beianStatus,
					today,
				}); err != nil {
					return err
				}
				rowIndex++
				hasMatchedDomain = true
			}
		}

		if !hasMatchedDomain {
			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := streamWriter.SetRow(cell, []interface{}{
				appName,
				packageName,
				"",
				"",
				"",
				"",
				beianStatus,
				today,
			}); err != nil {
				return err
			}
			rowIndex++
		}

		return nil
	})
	if err != nil {
		h.logger.WithError(err).Warn("Failed to export XLSX data")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "导出任务列表失败",
		})
		return
	}

	if err := streamWriter.Flush(); err != nil {
		h.logger.WithError(err).Warn("Failed to flush XLSX stream")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "导出任务列表失败",
		})
		return
	}

	if _, err := file.WriteTo(c.Writer); err != nil {
		h.logger.WithError(err).Warn("Failed to write XLSX response")
	}
}

func (h *TaskHandler) forEachExportTask(
	ctx context.Context,
	excludeStatus string,
	statusFilter string,
	searchQuery string,
	provinceFilter string,
	ispFilter string,
	beianStatusFilter string,
	handle func(task *domain.Task) error,
) error {
	const exportBatchSize = 1000
	page := 1
	for {
		tasks, _, err := h.taskService.ListTasksWithAdvancedFilters(ctx, page, exportBatchSize, excludeStatus, statusFilter, searchQuery, provinceFilter, ispFilter, beianStatusFilter)
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			break
		}
		for _, task := range tasks {
			if err := handle(task); err != nil {
				return err
			}
		}
		if len(tasks) < exportBatchSize {
			break
		}
		page++
	}
	return nil
}

func resolveExportAppName(task *domain.Task) string {
	if task.AppName != "" {
		return task.AppName
	}
	return strings.TrimSuffix(task.APKName, ".apk")
}

func resolveExportAppDomains(task *domain.Task) []domain.TaskAppDomain {
	appDomains := task.AppDomains
	if len(appDomains) == 0 && task.DomainAnalysis != nil && task.DomainAnalysis.AppDomainsJSON != "" {
		var appDomainsFromJSON []domain.TaskAppDomain
		if err := json.Unmarshal([]byte(task.DomainAnalysis.AppDomainsJSON), &appDomainsFromJSON); err == nil {
			appDomains = appDomainsFromJSON
		}
	}
	return appDomains
}

func buildExportFilterDescription(province string, isp string, beian string, search string) string {
	parts := []string{}
	if province != "" {
		parts = append(parts, province)
	}
	if isp != "" {
		parts = append(parts, isp)
	}
	if beian != "" {
		parts = append(parts, beian)
	}
	if search != "" {
		parts = append(parts, "搜索"+search)
	}
	if len(parts) == 0 {
		return "全部"
	}
	return strings.Join(parts, "_")
}

func getBeianStatusForExport(task *domain.Task) string {
	if task.DomainAnalysis == nil {
		return "未查询"
	}
	if task.DomainAnalysis.DomainBeianStatus != "" {
		return task.DomainAnalysis.DomainBeianStatus
	}
	if task.DomainAnalysis.DomainBeianJSON == "" {
		return "未查询"
	}

	var beianList []map[string]interface{}
	var beianSingle map[string]interface{}

	if err := json.Unmarshal([]byte(task.DomainAnalysis.DomainBeianJSON), &beianList); err != nil {
		if err := json.Unmarshal([]byte(task.DomainAnalysis.DomainBeianJSON), &beianSingle); err != nil {
			return "解析错误"
		}
		beianList = []map[string]interface{}{beianSingle}
	}

	if len(beianList) == 0 {
		return "未查询"
	}

	status := ""
	if statusVal, ok := beianList[0]["status"].(string); ok {
		status = statusVal
	}

	reason := ""
	if info, ok := beianList[0]["info"].(map[string]interface{}); ok {
		if reasonVal, ok := info["reason"].(string); ok {
			reason = reasonVal
		}
	}

	switch status {
	case "registered", "ok", "已备案":
		return "已备案"
	case "not_found", "not_registered", "未备案":
		return "未备案"
	case "error":
		if strings.Contains(reason, "暂无数据") {
			return "未备案"
		}
		return "查询失败"
	case "查询失败":
		return "查询失败"
	default:
		if status != "" {
			return status
		}
	}

	return "未查询"
}

// GetTask 获取单个任务详情
// GET /api/tasks/:id
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		h.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get task")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	c.JSON(http.StatusOK, h.taskToResponse(task))
}

// DeleteTask 删除任务
// DELETE /api/tasks/:id
func (h *TaskHandler) DeleteTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.taskService.DeleteTask(c.Request.Context(), taskID); err != nil {
		h.logger.WithError(err).WithField("task_id", taskID).Error("Failed to delete task")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除任务失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "任务删除成功",
	})
}

// BatchDeleteTasks 批量删除任务
// DELETE /api/tasks/batch
// 支持三种删除方式：
// 1. 按任务ID列表删除: {"task_ids": ["id1", "id2"]}
// 2. 按状态删除: {"status": "completed"} 或 {"status": "failed"}
// 3. 删除指定天数之前的任务: {"before_days": 7}
// 4. 删除所有任务: {"status": "all"}
// 可以组合使用状态和天数: {"status": "completed", "before_days": 7}
func (h *TaskHandler) BatchDeleteTasks(c *gin.Context) {
	var req struct {
		TaskIDs    []string `json:"task_ids"`
		Status     string   `json:"status"`
		BeforeDays int      `json:"before_days"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数格式错误",
		})
		return
	}

	// 验证参数
	if len(req.TaskIDs) == 0 && req.Status == "" && req.BeforeDays == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请至少提供一个删除条件: task_ids, status 或 before_days",
		})
		return
	}

	deletedCount, err := h.taskService.BatchDeleteTasks(c.Request.Context(), req.TaskIDs, req.Status, req.BeforeDays)
	if err != nil {
		h.logger.WithError(err).Error("Failed to batch delete tasks")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "批量删除任务失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "批量删除成功",
		"deleted_count": deletedCount,
	})
}

// ListQueuedTasks 获取所有排队中的任务（不分页）
// GET /api/tasks/queued
func (h *TaskHandler) ListQueuedTasks(c *gin.Context) {
	tasks, err := h.taskService.ListQueuedTasks(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to list queued tasks")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取排队任务列表失败",
		})
		return
	}

	// 转换为响应格式
	var taskResponses []gin.H
	for _, task := range tasks {
		taskResponses = append(taskResponses, h.taskToResponse(task))
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": taskResponses,
		"total": len(tasks),
	})
}

// StopTask 停止任务
// POST /api/tasks/:id/stop
func (h *TaskHandler) StopTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.taskService.StopTask(c.Request.Context(), taskID); err != nil {
		h.logger.WithError(err).WithField("task_id", taskID).Error("Failed to stop task")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "停止任务失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "任务已标记为停止",
	})
}

// GetTaskURLs 获取任务的所有 URL
// GET /api/tasks/:id/urls
func (h *TaskHandler) GetTaskURLs(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 1. 获取静态 URL
	staticURLs := []map[string]interface{}{}
	urlSet := make(map[string]bool) // 用于去重

	// 1.1 优先从 StaticReport（Hybrid Analyzer 深度分析）获取
	if task.StaticReport != nil && task.StaticReport.DeepAnalysisJSON != "" {
		var deepAnalysis map[string]interface{}
		if err := json.Unmarshal([]byte(task.StaticReport.DeepAnalysisJSON), &deepAnalysis); err == nil {
			// 从 deep_analysis.urls 提取
			if urls, ok := deepAnalysis["urls"].([]interface{}); ok {
				for _, u := range urls {
					if urlStr, ok := u.(string); ok && urlStr != "" && !urlSet[urlStr] {
						urlSet[urlStr] = true
						staticURLs = append(staticURLs, map[string]interface{}{
							"url":    urlStr,
							"source": "Hybrid Static Analysis (DEX)",
						})
					}
				}
			}
		}
	}

	// 静态 URL 已经从 StaticReport 获取，无需额外处理

	// 2. 获取动态 URL（从 flows.jsonl）
	dynamicURLs := []map[string]interface{}{}
	flowsPath := fmt.Sprintf("./results/%s/flows.jsonl", taskID)
	if file, err := os.Open(flowsPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		urlSet := make(map[string]bool) // 去重

		for scanner.Scan() {
			var flow map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &flow); err == nil {
				if url, ok := flow["url"].(string); ok && url != "" {
					// 去重
					if !urlSet[url] {
						urlSet[url] = true
						dynamicURLs = append(dynamicURLs, map[string]interface{}{
							"url":         url,
							"method":      flow["method"],
							"status_code": flow["status_code"],
							"timestamp":   flow["timestamp"],
						})
					}
				}
			}
		}
	}

	// 3. 获取 URL 分类结果
	var urlClassification map[string]interface{}
	if task.DomainAnalysis != nil && task.DomainAnalysis.URLClassificationJSON != "" {
		if err := json.Unmarshal([]byte(task.DomainAnalysis.URLClassificationJSON), &urlClassification); err != nil {
			h.logger.WithError(err).Warn("Failed to parse URL classification JSON")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"static_urls":        staticURLs,
		"dynamic_urls":       dynamicURLs,
		"static_count":       len(staticURLs),
		"dynamic_count":      len(dynamicURLs),
		"url_classification": urlClassification,
	})
}

// GetActivityURLs 获取特定 Activity 的 URL
// GET /api/tasks/:id/activities/:name/urls
func (h *TaskHandler) GetActivityURLs(c *gin.Context) {
	taskID := c.Param("id")
	activityName := c.Param("name")

	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 从 ActivityDetails 中提取指定 Activity 的 URL
	var urls []map[string]interface{}

	if task.Activities != nil {
		// TODO: 解析 ActivityDetailsJSON 并过滤特定 Activity
		h.logger.WithField("activity_name", activityName).Info("Getting activity URLs")
	}

	c.JSON(http.StatusOK, urls)
}

// GetSystemStats 获取系统统计信息
// GET /api/stats
// 使用数据库聚合查询统计各状态任务数量，避免只统计部分数据的问题
func (h *TaskHandler) GetSystemStats(c *gin.Context) {
	statusCounts, total, err := h.taskService.GetStatusCounts(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get status counts")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计信息失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_tasks":      total,
		"status_breakdown": statusCounts,
	})
}

// taskToResponse 将 Task 模型转换为响应格式
func (h *TaskHandler) taskToResponse(task *domain.Task) map[string]interface{} {
	response := map[string]interface{}{
		"id":               task.ID,
		"apk_name":         task.APKName,
		"app_name":         task.AppName, // 应用名称（从静态分析获取）
		"package_name":     task.PackageName,
		"status":           task.Status,
		"created_at":       task.CreatedAt,
		"started_at":       task.StartedAt,
		"completed_at":     task.CompletedAt,
		"current_step":     task.CurrentStep,
		"progress_percent": task.ProgressPercent,
		"error_message":    task.ErrorMessage,
		"should_stop":      task.ShouldStop,
		"failure_type":     task.FailureType,
	}

	// 添加失败类型的显示名称和严重程度
	if task.FailureType != "" {
		response["failure_type_display"] = task.FailureType.GetDisplayName()
		response["failure_severity"] = task.FailureType.GetSeverity()
	}

	// 添加 CST 时间格式
	if !task.CreatedAt.IsZero() {
		response["created_at_cst"] = task.CreatedAt.Add(8 * 60 * 60 * 1000000000).Format("2006/01/02 15:04:05")
	}
	if task.StartedAt != nil && !task.StartedAt.IsZero() {
		response["started_at_cst"] = task.StartedAt.Add(8 * 60 * 60 * 1000000000).Format("2006/01/02 15:04:05")
	}
	if task.CompletedAt != nil && !task.CompletedAt.IsZero() {
		response["completed_at_cst"] = task.CompletedAt.Add(8 * 60 * 60 * 1000000000).Format("2006/01/02 15:04:05")
	}

	// 添加关联数据 - Activities (动态分析数据)
	if task.Activities != nil {
		response["activities"] = map[string]interface{}{
			"activities_json":       task.Activities.ActivitiesJSON,
			"activity_details_json": task.Activities.ActivityDetailsJSON,
			"launcher_activity":     task.Activities.LauncherActivity,
		}
	}

	// 静态分析状态（Hybrid 模式）
	if task.StaticReport != nil {
		response["static_status"] = task.StaticReport.Status
		response["static_url_count"] = task.StaticReport.URLCount
		response["static_domain_count"] = task.StaticReport.DomainCount

		// 壳检测信息
		if task.StaticReport.IsPacked {
			response["is_packed"] = true
			response["packer_name"] = task.StaticReport.PackerName
			response["packer_type"] = task.StaticReport.PackerType
			response["packer_confidence"] = task.StaticReport.PackerConfidence
			response["needs_dynamic_unpacking"] = task.StaticReport.NeedsDynamicUnpacking
		} else {
			response["is_packed"] = false
		}
	}

	if task.DomainAnalysis != nil {
		response["primary_domain"] = task.DomainAnalysis.PrimaryDomainJSON
		response["domain_beian_status"] = task.DomainAnalysis.DomainBeianJSON
	}

	// 恶意检测结果（用于任务列表展示结论）
	if task.MalwareResult != nil {
		response["malware_result"] = map[string]interface{}{
			"status":     task.MalwareResult.Status,
			"is_malware": task.MalwareResult.IsMalware,
		}
	}

	// 添加 IP 归属地信息（从 task_app_domains 表）
	if len(task.AppDomains) > 0 {
		appDomains := make([]map[string]interface{}, len(task.AppDomains))
		for i, appDomain := range task.AppDomains {
			appDomains[i] = map[string]interface{}{
				"domain":   appDomain.Domain,
				"ip":       appDomain.IP,
				"province": appDomain.Province,
				"city":     appDomain.City,
				"isp":      appDomain.ISP,
				"source":   appDomain.Source,
			}
		}
		response["app_domains"] = appDomains
	} else if task.DomainAnalysis != nil && task.DomainAnalysis.AppDomainsJSON != "" {
		// 兼容旧数据源：从 AppDomainsJSON 填充 app_domains
		var appDomainsFromJSON []domain.TaskAppDomain
		if err := json.Unmarshal([]byte(task.DomainAnalysis.AppDomainsJSON), &appDomainsFromJSON); err == nil && len(appDomainsFromJSON) > 0 {
			appDomains := make([]map[string]interface{}, len(appDomainsFromJSON))
			for i, appDomain := range appDomainsFromJSON {
				appDomains[i] = map[string]interface{}{
					"domain":   appDomain.Domain,
					"ip":       appDomain.IP,
					"province": appDomain.Province,
					"city":     appDomain.City,
					"isp":      appDomain.ISP,
					"source":   appDomain.Source,
				}
			}
			response["app_domains"] = appDomains
		}
	}

	// 添加 IP 列表 - 区分两种来源
	// 1. url_direct_ips: URL中直接使用的IP（显示在域名列中，带归属地）
	// 2. top_domains_with_ips: 所有候选域名及其对应的DNS解析IP（带归属地）
	urlDirectIPs := []map[string]interface{}{}
	urlIPSet := make(map[string]bool)

	// 构建IP->归属地映射（从task_app_domains表）
	ipLocationMap := make(map[string]map[string]interface{})
	if len(task.AppDomains) > 0 {
		for _, appDomain := range task.AppDomains {
			if appDomain.IP != "" {
				ipLocationMap[appDomain.IP] = map[string]interface{}{
					"province": appDomain.Province,
					"city":     appDomain.City,
					"isp":      appDomain.ISP,
				}
			}
		}
	}

	// 方法1: 从 Activities 动态分析URL中提取直接使用的IP地址
	if task.Activities != nil && task.Activities.ActivityDetailsJSON != "" {
		var activities []map[string]interface{}
		if err := json.Unmarshal([]byte(task.Activities.ActivityDetailsJSON), &activities); err == nil {
			for _, activity := range activities {
				if flows, ok := activity["flows"].([]interface{}); ok {
					for _, flowInterface := range flows {
						if flow, ok := flowInterface.(map[string]interface{}); ok {
							if host, ok := flow["host"].(string); ok && host != "" {
								// 检查host是否是IP地址
								if h.isIPAddress(host) && !urlIPSet[host] {
									urlIPSet[host] = true
									ipInfo := map[string]interface{}{
										"ip": host,
									}
									// 添加归属地信息
									if location, exists := ipLocationMap[host]; exists {
										if province, ok := location["province"].(string); ok && province != "" {
											ipInfo["province"] = province
										}
										if city, ok := location["city"].(string); ok && city != "" {
											ipInfo["city"] = city
										}
										if isp, ok := location["isp"].(string); ok && isp != "" {
											ipInfo["isp"] = isp
										}
									}
									urlDirectIPs = append(urlDirectIPs, ipInfo)
								}
							}
						}
					}
				}
			}
		}
	}

	// 方法2: 从 PrimaryDomain 中获取前3个域名，并从 AppDomainsJSON 或 AppDomains关联表 获取其对应的IP
	topDomainsWithIPs := []map[string]interface{}{}
	if task.DomainAnalysis != nil && task.DomainAnalysis.PrimaryDomainJSON != "" {
		var primaryDomain map[string]interface{}
		if err := json.Unmarshal([]byte(task.DomainAnalysis.PrimaryDomainJSON), &primaryDomain); err == nil {
			// 获取候选域名列表
			if candidates, ok := primaryDomain["candidates"].([]interface{}); ok && len(candidates) > 0 {
				// 取前3个候选域名
				maxDomains := 3
				if len(candidates) < 3 {
					maxDomains = len(candidates)
				}

				// 从 AppDomainsJSON 构建域名->IP+归属地映射
				domainInfoMap := make(map[string]map[string]interface{})

				// 优先从 AppDomains 关联表获取 (新数据源，包含归属地信息)
				if len(task.AppDomains) > 0 {
					for _, appDomain := range task.AppDomains {
						if appDomain.Domain != "" && appDomain.IP != "" {
							// 提取主域名
							mainDomain := h.extractMainDomain(appDomain.Domain)
							// 只保留第一个IP及其归属地
							if _, exists := domainInfoMap[mainDomain]; !exists {
								domainInfoMap[mainDomain] = map[string]interface{}{
									"ip":       appDomain.IP,
									"province": appDomain.Province,
									"city":     appDomain.City,
									"isp":      appDomain.ISP,
								}
							}
						}
					}
				}

				// 如果 AppDomains 表为空,从 AppDomainsJSON 获取 (旧数据源)
				if len(domainInfoMap) == 0 && task.DomainAnalysis.AppDomainsJSON != "" {
					var ipLocations []map[string]interface{}
					if err := json.Unmarshal([]byte(task.DomainAnalysis.AppDomainsJSON), &ipLocations); err == nil {
						for _, loc := range ipLocations {
							if fullDomain, ok := loc["domain"].(string); ok {
								if ip, ok := loc["ip"].(string); ok && ip != "" {
									// 提取主域名 (例如: sgm-m.jd.com -> jd.com)
									mainDomain := h.extractMainDomain(fullDomain)
									// 只保留第一个IP及其归属地
									if _, exists := domainInfoMap[mainDomain]; !exists {
										domainInfoMap[mainDomain] = map[string]interface{}{
											"ip":       ip,
											"province": loc["province"],
											"city":     loc["city"],
											"isp":      loc["isp"],
										}
									}
								}
							}
						}
					}
				}

				// 构建前3个域名及其IP和归属地
				for i := 0; i < maxDomains; i++ {
					if candidate, ok := candidates[i].(map[string]interface{}); ok {
						if domain, ok := candidate["domain"].(string); ok && domain != "" {
							domainInfo := map[string]interface{}{
								"domain": domain,
							}
							// 查找对应的IP和归属地
							if ipInfo, exists := domainInfoMap[domain]; exists {
								if ip, ok := ipInfo["ip"].(string); ok && !urlIPSet[ip] {
									domainInfo["ip"] = ip
									// 添加归属地信息
									if province, ok := ipInfo["province"].(string); ok && province != "" {
										domainInfo["province"] = province
									}
									if city, ok := ipInfo["city"].(string); ok && city != "" {
										domainInfo["city"] = city
									}
									if isp, ok := ipInfo["isp"].(string); ok && isp != "" {
										domainInfo["isp"] = isp
									}
								}
							}
							topDomainsWithIPs = append(topDomainsWithIPs, domainInfo)
						}
					}
				}
			}
		}
	}

	// 添加到响应中
	if len(urlDirectIPs) > 0 {
		response["url_direct_ips"] = urlDirectIPs
	}
	if len(topDomainsWithIPs) > 0 {
		response["top_domains_with_ips"] = topDomainsWithIPs
	}

	return response
}

// GetActivitiesReport 获取任务的 Activity 执行报告 (HTML)
// GET /api/tasks/:id/activities/report
func (h *TaskHandler) GetActivitiesReport(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	if task.Activities == nil || task.Activities.ActivityDetailsJSON == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Activity 数据不存在",
		})
		return
	}

	// 解析 Activity 详情
	var activities []map[string]interface{}
	if err := json.Unmarshal([]byte(task.Activities.ActivityDetailsJSON), &activities); err != nil {
		h.logger.WithError(err).Error("Failed to parse activity details")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "解析 Activity 数据失败",
		})
		return
	}

	// 返回 HTML 报告
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, h.generateActivitiesReportHTML(task, activities))
}

// generateActivitiesReportHTML 生成 Activity 执行报告 HTML
func (h *TaskHandler) generateActivitiesReportHTML(task *domain.Task, activities []map[string]interface{}) string {
	// 统计信息
	totalCount := len(activities)
	successCount := 0
	failedCount := 0
	totalURLs := 0
	totalExecutionTime := 0.0

	for _, act := range activities {
		if status, ok := act["status"].(string); ok && status == "completed" {
			successCount++
		} else {
			failedCount++
		}
		if urls, ok := act["urls_collected"].(float64); ok {
			totalURLs += int(urls)
		}
		if execTime, ok := act["execution_time"].(float64); ok {
			totalExecutionTime += execTime
		}
	}

	// Debug logging
	h.logger.WithFields(map[string]interface{}{
		"totalCount":         totalCount,
		"successCount":       successCount,
		"failedCount":        failedCount,
		"totalURLs":          totalURLs,
		"totalExecutionTime": totalExecutionTime,
	}).Info("Activity statistics calculated")

	// 生成 Activity 列表 HTML
	activitiesHTML := ""
	for i, act := range activities {
		activityName := h.getStringValue(act, "activity")
		shortName := h.getShortActivityName(activityName)
		status := h.getStringValue(act, "status")
		statusClass := ""
		statusIcon := ""
		if status == "completed" {
			statusClass = "success"
			statusIcon = "✓"
		} else {
			statusClass = "failed"
			statusIcon = "✗"
		}

		execTime := h.getFloatValue(act, "execution_time")
		urlsCount := int(h.getFloatValue(act, "urls_collected"))
		screenshot := h.getStringValue(act, "screenshot_file")
		uiHierarchy := h.getStringValue(act, "ui_hierarchy_file")

		activitiesHTML += fmt.Sprintf(`
		<div class="activity-card" onclick="showActivityDetail(%d)">
			<div class="activity-header">
				<div class="activity-title">
					<span class="activity-icon">📱</span>
					<div>
						<div class="activity-name">%s</div>
						<div class="activity-full-name">%s</div>
					</div>
				</div>
				<span class="status-badge status-%s">%s %s</span>
			</div>
			<div class="activity-stats">
				<div class="stat-item">
					<span class="stat-label">执行时间</span>
					<span class="stat-value">%.2fs</span>
				</div>
				<div class="stat-item">
					<span class="stat-label">URL数量</span>
					<span class="stat-value">%d</span>
				</div>
				<div class="stat-item">
					<span class="stat-label">截图</span>
					<span class="stat-value">%s</span>
				</div>
				<div class="stat-item">
					<span class="stat-label">UI层级</span>
					<span class="stat-value">%s</span>
				</div>
			</div>
		</div>
		`, i, html.EscapeString(shortName), html.EscapeString(activityName), statusClass, statusIcon, status,
			execTime, urlsCount,
			h.boolIcon(screenshot != ""), h.boolIcon(uiHierarchy != ""))
	}

	// 生成详细信息的 JavaScript 数据
	activitiesJSON, _ := json.Marshal(activities)

	h.logger.WithFields(map[string]interface{}{
		"apkName":            task.APKName,
		"totalCount":         totalCount,
		"successCount":       successCount,
		"failedCount":        failedCount,
		"totalURLs":          totalURLs,
		"totalExecutionTime": totalExecutionTime,
		"activitiesHTMLLen":  len(activitiesHTML),
		"activitiesJSONLen":  len(activitiesJSON),
		"taskID":             task.ID,
	}).Info("About to call fmt.Sprintf")

	result := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Activity 执行报告 - %s</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #F5F7FA;
            min-height: 100vh;
        }

        .sidebar {
            position: fixed;
            left: 0;
            top: 0;
            bottom: 0;
            width: 70px;
            background: #1E293B;
            display: flex;
            flex-direction: column;
            align-items: center;
            padding: 20px 0;
            z-index: 100;
        }

        .sidebar-icon {
            width: 40px;
            height: 40px;
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 15px;
            cursor: pointer;
            transition: all 0.3s;
            font-size: 20px;
            text-decoration: none;
            color: #94A3B8;
        }

        .sidebar-icon:hover {
            background: #334155;
        }

        .main-container {
            margin-left: 70px;
            padding: 30px;
        }

        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 30px;
        }

        .header h1 {
            font-size: 28px;
            color: #1E293B;
            font-weight: 600;
        }

        .back-btn {
            background: #5B68FF;
            color: white;
            padding: 12px 24px;
            border-radius: 10px;
            border: none;
            font-weight: 500;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
            transition: all 0.3s;
            box-shadow: 0 2px 8px rgba(91, 104, 255, 0.3);
            text-decoration: none;
        }

        .back-btn:hover {
            background: #4A56E8;
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(91, 104, 255, 0.4);
        }

        .content-section {
            background: white;
            border-radius: 16px;
            padding: 24px;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
            margin-bottom: 20px;
        }

        .section-title {
            font-size: 20px;
            font-weight: 600;
            color: #1E293B;
            margin-bottom: 20px;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px;
            margin-bottom: 30px;
        }

        .stat-card {
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            border-radius: 12px;
            color: white;
        }

        .stat-card:nth-child(2) {
            background: linear-gradient(135deg, #f093fb 0%%, #f5576c 100%%);
        }

        .stat-card:nth-child(3) {
            background: linear-gradient(135deg, #4facfe 0%%, #00f2fe 100%%);
        }

        .stat-card:nth-child(4) {
            background: linear-gradient(135deg, #43e97b 0%%, #38f9d7 100%%);
        }

        .stat-card:nth-child(5) {
            background: linear-gradient(135deg, #fa709a 0%%, #fee140 100%%);
        }

        .stat-label {
            font-size: 13px;
            opacity: 0.9;
            margin-bottom: 8px;
        }

        .stat-number {
            font-size: 32px;
            font-weight: 700;
        }

        .activities-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
            gap: 16px;
        }

        .activity-card {
            background: #F8FAFC;
            border-radius: 12px;
            padding: 16px;
            cursor: pointer;
            transition: all 0.3s;
            border: 2px solid transparent;
        }

        .activity-card:hover {
            border-color: #5B68FF;
            box-shadow: 0 4px 12px rgba(91, 104, 255, 0.15);
            transform: translateY(-2px);
        }

        .activity-header {
            display: flex;
            justify-content: space-between;
            align-items: start;
            margin-bottom: 12px;
        }

        .activity-title {
            display: flex;
            gap: 12px;
            align-items: start;
            flex: 1;
        }

        .activity-icon {
            font-size: 24px;
        }

        .activity-name {
            font-size: 16px;
            font-weight: 600;
            color: #1E293B;
            margin-bottom: 4px;
        }

        .activity-full-name {
            font-size: 11px;
            color: #64748B;
            word-break: break-all;
        }

        .status-badge {
            padding: 4px 12px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 500;
            white-space: nowrap;
        }

        .status-success {
            background: #DCFCE7;
            color: #16A34A;
        }

        .status-failed {
            background: #FEE2E2;
            color: #DC2626;
        }

        .activity-stats {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 8px;
        }

        .stat-item {
            display: flex;
            justify-content: space-between;
            font-size: 13px;
        }

        .stat-label {
            color: #64748B;
        }

        .stat-value {
            font-weight: 600;
            color: #1E293B;
        }

        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.5);
            z-index: 1000;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }

        .modal.active {
            display: flex;
        }

        .modal-content {
            background: white;
            border-radius: 16px;
            max-width: 900px;
            max-height: 90vh;
            overflow-y: auto;
            width: 100%%;
            padding: 30px;
        }

        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
        }

        .modal-title {
            font-size: 24px;
            font-weight: 600;
            color: #1E293B;
        }

        .close-btn {
            background: #F1F5F9;
            border: none;
            width: 36px;
            height: 36px;
            border-radius: 8px;
            cursor: pointer;
            font-size: 20px;
            color: #64748B;
            transition: all 0.3s;
        }

        .close-btn:hover {
            background: #E2E8F0;
        }

        .detail-section {
            margin-bottom: 24px;
        }

        .detail-title {
            font-size: 16px;
            font-weight: 600;
            color: #1E293B;
            margin-bottom: 12px;
        }

        .detail-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 12px;
            background: #F8FAFC;
            padding: 16px;
            border-radius: 8px;
        }

        .detail-item {
            display: flex;
            justify-content: space-between;
        }

        .detail-label {
            color: #64748B;
            font-size: 14px;
        }

        .detail-value {
            color: #1E293B;
            font-weight: 600;
            font-size: 14px;
        }

        .url-list {
            background: #F8FAFC;
            border-radius: 8px;
            padding: 16px;
            max-height: 300px;
            overflow-y: auto;
        }

        .url-item {
            padding: 8px;
            background: white;
            border-radius: 6px;
            margin-bottom: 8px;
            font-size: 13px;
            word-break: break-all;
        }

        .url-item a {
            color: #5B68FF;
            text-decoration: none;
        }

        .url-item a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="sidebar">
        <a href="/" class="sidebar-icon" title="返回首页">📊</a>
    </div>

    <div class="main-container">
        <div class="header">
            <h1>📱 Activity 执行报告</h1>
            <a href="/" class="back-btn">
                <span>←</span>
                <span>返回任务列表</span>
            </a>
        </div>

        <div class="content-section">
            <div class="section-title">📊 执行统计</div>
            <div class="stats-grid">
                <div class="stat-card">
                    <div class="stat-label">Activity 总数</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">执行成功</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">执行失败</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">总URL数量</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">总执行时间</div>
                    <div class="stat-number">%.1fs</div>
                </div>
            </div>
        </div>

        <div class="content-section">
            <div class="section-title">🔍 Activity 列表</div>
            <div class="activities-grid">
                %s
            </div>
        </div>
    </div>

    <!-- Modal -->
    <div class="modal" id="activityModal">
        <div class="modal-content">
            <div class="modal-header">
                <div class="modal-title" id="modalTitle">Activity 详情</div>
                <button class="close-btn" onclick="closeModal()">×</button>
            </div>
            <div id="modalBody"></div>
        </div>
    </div>

    <script>
        const activities = %s;
        const taskId = '%s';

        function showActivityDetail(index) {
            const activity = activities[index];
            const modal = document.getElementById('activityModal');
            const title = document.getElementById('modalTitle');
            const body = document.getElementById('modalBody');

            title.textContent = activity.activity || 'Unknown Activity';

            let detailHTML = '<div class="detail-section">';
            detailHTML += '<div class="detail-title">基本信息</div>';
            detailHTML += '<div class="detail-grid">';
            detailHTML += '<div class="detail-item"><span class="detail-label">组件名</span><span class="detail-value">' + (activity.component || 'N/A') + '</span></div>';
            detailHTML += '<div class="detail-item"><span class="detail-label">状态</span><span class="detail-value">' + (activity.status || 'N/A') + '</span></div>';
            detailHTML += '<div class="detail-item"><span class="detail-label">开始时间</span><span class="detail-value">' + (activity.start_time || 'N/A') + '</span></div>';
            detailHTML += '<div class="detail-item"><span class="detail-label">结束时间</span><span class="detail-value">' + (activity.end_time || 'N/A') + '</span></div>';
            detailHTML += '<div class="detail-item"><span class="detail-label">执行时间</span><span class="detail-value">' + (activity.execution_time ? activity.execution_time.toFixed(2) + 's' : 'N/A') + '</span></div>';
            detailHTML += '<div class="detail-item"><span class="detail-label">URL数量</span><span class="detail-value">' + (activity.urls_collected || 0) + '</span></div>';
            detailHTML += '</div></div>';

            if (activity.screenshot_file) {
                detailHTML += '<div class="detail-section">';
                detailHTML += '<div class="detail-title">📸 截图</div>';
                detailHTML += '<a href="/api/tasks/' + taskId + '/screenshot/' + activity.screenshot_file + '" target="_blank">查看截图</a>';
                detailHTML += '</div>';
            }

            if (activity.ui_hierarchy_file) {
                detailHTML += '<div class="detail-section">';
                detailHTML += '<div class="detail-title">🌳 UI层级</div>';
                detailHTML += '<a href="/api/tasks/' + taskId + '/ui_hierarchy/' + activity.ui_hierarchy_file + '" target="_blank">查看UI层级</a>';
                detailHTML += '</div>';
            }

            if (activity.flows && activity.flows.length > 0) {
                detailHTML += '<div class="detail-section">';
                detailHTML += '<div class="detail-title">🔗 收集的URL (' + activity.flows.length + ')</div>';
                detailHTML += '<div class="url-list">';
                activity.flows.forEach(function(flow) {
                    detailHTML += '<div class="url-item"><a href="' + flow.url + '" target="_blank">' + flow.url + '</a></div>';
                });
                detailHTML += '</div></div>';
            }

            if (activity.error) {
                detailHTML += '<div class="detail-section">';
                detailHTML += '<div class="detail-title">❌ 错误信息</div>';
                detailHTML += '<div style="color: #DC2626; padding: 12px; background: #FEE2E2; border-radius: 8px;">' + activity.error + '</div>';
                detailHTML += '</div>';
            }

            body.innerHTML = detailHTML;
            modal.classList.add('active');
        }

        function closeModal() {
            document.getElementById('activityModal').classList.remove('active');
        }

        // Close modal on outside click
        document.getElementById('activityModal').addEventListener('click', function(e) {
            if (e.target === this) {
                closeModal();
            }
        });
    </script>
</body>
</html>
	`, html.EscapeString(task.APKName),
		totalCount, successCount, failedCount, totalURLs, totalExecutionTime,
		activitiesHTML,
		string(activitiesJSON), html.EscapeString(task.ID))

	// Debug: log first 500 chars of result
	if len(result) > 500 {
		h.logger.WithField("result_preview", result[:500]).Info("fmt.Sprintf result preview")
	} else {
		h.logger.WithField("result_preview", result).Info("fmt.Sprintf result preview")
	}

	return result
}

// Helper functions for Activity report

func (h *TaskHandler) getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (h *TaskHandler) getFloatValue(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0
}

func (h *TaskHandler) getShortActivityName(fullName string) string {
	parts := strings.Split(fullName, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fullName
}

func (h *TaskHandler) boolIcon(val bool) string {
	if val {
		return "✓"
	}
	return "✗"
}

// isIPAddress 检查字符串是否是IPv4地址
func (h *TaskHandler) isIPAddress(host string) bool {
	// 简单的IPv4正则匹配
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		num := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
			num = num*10 + int(ch-'0')
		}
		if num > 255 {
			return false
		}
	}
	return true
}

// extractMainDomain 提取主域名
// 例如: sgm-m.jd.com -> jd.com, www.google.com -> google.com
func (h *TaskHandler) extractMainDomain(fullDomain string) string {
	parts := strings.Split(fullDomain, ".")
	if len(parts) >= 2 {
		// 返回最后两个部分作为主域名
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return fullDomain
}

// GetStaticReport 获取混合静态分析报告（JSON）
// GET /api/tasks/:id/static
func (h *TaskHandler) GetStaticReport(c *gin.Context) {
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

	// 解析 JSON 数据
	var basicInfo map[string]interface{}
	var deepAnalysis map[string]interface{}

	if task.StaticReport.BasicInfoJSON != "" {
		if err := json.Unmarshal([]byte(task.StaticReport.BasicInfoJSON), &basicInfo); err != nil {
			h.logger.WithError(err).Warn("Failed to parse basic info JSON")
		}
	}

	if task.StaticReport.DeepAnalysisJSON != "" {
		if err := json.Unmarshal([]byte(task.StaticReport.DeepAnalysisJSON), &deepAnalysis); err != nil {
			h.logger.WithError(err).Warn("Failed to parse deep analysis JSON")
		}
	}

	// 解析壳检测指标
	var packerIndicators []string
	if task.StaticReport.PackerIndicators != "" {
		json.Unmarshal([]byte(task.StaticReport.PackerIndicators), &packerIndicators)
	}

	response := map[string]interface{}{
		"task_id":                    task.StaticReport.TaskID,
		"analyzer":                   task.StaticReport.Analyzer,
		"analysis_mode":              task.StaticReport.AnalysisMode,
		"status":                     task.StaticReport.Status,
		"package_name":               task.StaticReport.PackageName,
		"version_name":               task.StaticReport.VersionName,
		"version_code":               task.StaticReport.VersionCode,
		"app_name":                   task.StaticReport.AppName,
		"file_size":                  task.StaticReport.FileSize,
		"md5":                        task.StaticReport.MD5,
		"sha256":                     task.StaticReport.SHA256,
		"developer":                  task.StaticReport.Developer,
		"company_name":               task.StaticReport.CompanyName,
		"activity_count":             task.StaticReport.ActivityCount,
		"service_count":              task.StaticReport.ServiceCount,
		"receiver_count":             task.StaticReport.ReceiverCount,
		"provider_count":             task.StaticReport.ProviderCount,
		"permission_count":           task.StaticReport.PermissionCount,
		"url_count":                  task.StaticReport.URLCount,
		"domain_count":               task.StaticReport.DomainCount,
		"basic_info":                 basicInfo,
		"deep_analysis":              deepAnalysis,
		"analysis_duration_ms":       task.StaticReport.AnalysisDurationMs,
		"fast_analysis_duration_ms":  task.StaticReport.FastAnalysisDurationMs,
		"deep_analysis_duration_ms":  task.StaticReport.DeepAnalysisDurationMs,
		"needs_deep_analysis_reason": task.StaticReport.NeedsDeepAnalysisReason,
		"analyzed_at":                task.StaticReport.AnalyzedAt,
		// 壳检测相关
		"is_packed":                    task.StaticReport.IsPacked,
		"packer_name":                  task.StaticReport.PackerName,
		"packer_type":                  task.StaticReport.PackerType,
		"packer_confidence":            task.StaticReport.PackerConfidence,
		"packer_indicators":            packerIndicators,
		"needs_dynamic_unpacking":      task.StaticReport.NeedsDynamicUnpacking,
		"packer_detection_duration_ms": task.StaticReport.PackerDetectionDurationMs,
	}

	c.JSON(http.StatusOK, response)
}

// GetStaticReportHTML 获取混合静态分析报告（HTML 页面）
// GET /api/tasks/:id/static/report
func (h *TaskHandler) GetStaticReportHTML(c *gin.Context) {
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

	// 返回 HTML 格式的报告
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, h.generateHybridReportHTML(task))
}

// generateHybridReportHTML 生成 Hybrid 静态分析报告 HTML
func (h *TaskHandler) generateHybridReportHTML(task *domain.Task) string {
	report := task.StaticReport

	// 解析 BasicInfo 和 DeepAnalysis JSON
	var basicInfo map[string]interface{}
	var deepAnalysis map[string]interface{}

	if report.BasicInfoJSON != "" {
		json.Unmarshal([]byte(report.BasicInfoJSON), &basicInfo)
	}
	if report.DeepAnalysisJSON != "" {
		json.Unmarshal([]byte(report.DeepAnalysisJSON), &deepAnalysis)
	}

	// 提取数据
	urls := h.extractURLsFromDeepAnalysis(deepAnalysis)
	domains := h.extractDomainsFromDeepAnalysis(deepAnalysis)
	activities := h.extractActivitiesFromBasicInfo(basicInfo)
	permissions := h.extractPermissionsFromBasicInfo(basicInfo)

	// 格式化文件大小
	fileSize := h.formatFileSize(report.FileSize)

	// 格式化分析时间
	analyzedAt := "N/A"
	if report.AnalyzedAt != nil {
		analyzedAt = report.AnalyzedAt.Format("2006-01-02 15:04:05")
	}

	// 生成 URLs 表格
	urlsHTML := h.generateHybridURLsTable(urls)
	domainsHTML := h.generateHybridDomainsTable(domains)
	activitiesHTML := h.generateHybridActivitiesTable(activities)
	permissionsHTML := h.generateHybridPermissionsTable(permissions)

	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>静态分析报告 - %s</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background: #F5F7FA;
            min-height: 100vh;
        }
        .sidebar {
            position: fixed; left: 0; top: 0; bottom: 0; width: 70px;
            background: #1E293B; display: flex; flex-direction: column;
            align-items: center; padding: 20px 0; z-index: 100;
        }
        .sidebar-icon {
            width: 40px; height: 40px; border-radius: 10px;
            display: flex; align-items: center; justify-content: center;
            margin-bottom: 15px; cursor: pointer; transition: all 0.3s;
            font-size: 20px; text-decoration: none; color: #94A3B8;
        }
        .sidebar-icon:hover { background: #334155; }
        .main-container { margin-left: 70px; padding: 30px; }
        .header {
            display: flex; justify-content: space-between;
            align-items: center; margin-bottom: 30px;
        }
        .header h1 { font-size: 28px; color: #1E293B; font-weight: 600; }
        .back-btn {
            background: #5B68FF; color: white; padding: 12px 24px;
            border-radius: 10px; border: none; font-weight: 500;
            cursor: pointer; display: flex; align-items: center; gap: 8px;
            transition: all 0.3s; box-shadow: 0 2px 8px rgba(91, 104, 255, 0.3);
            text-decoration: none;
        }
        .back-btn:hover {
            background: #4A56E8; transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(91, 104, 255, 0.4);
        }
        .content-section {
            background: white; border-radius: 16px; padding: 24px;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05); margin-bottom: 20px;
        }
        .section-title {
            font-size: 20px; font-weight: 600; color: #1E293B; margin-bottom: 20px;
        }
        .info-grid {
            display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
        }
        .info-item {
            padding: 20px; background: #F8FAFC; border-radius: 12px;
            border-left: 4px solid #5B68FF;
        }
        .info-label { font-size: 13px; color: #64748B; margin-bottom: 6px; font-weight: 500; }
        .info-value { font-size: 16px; color: #1E293B; font-weight: 600; word-break: break-all; }
        .stats-grid {
            display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 16px; margin-top: 16px;
        }
        .stat-card {
            padding: 20px; border-radius: 12px; color: white; text-align: center;
        }
        .stat-card:nth-child(1) { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); }
        .stat-card:nth-child(2) { background: linear-gradient(135deg, #f093fb 0%%, #f5576c 100%%); }
        .stat-card:nth-child(3) { background: linear-gradient(135deg, #4facfe 0%%, #00f2fe 100%%); }
        .stat-card:nth-child(4) { background: linear-gradient(135deg, #43e97b 0%%, #38f9d7 100%%); }
        .stat-card:nth-child(5) { background: linear-gradient(135deg, #fa709a 0%%, #fee140 100%%); }
        .stat-card:nth-child(6) { background: linear-gradient(135deg, #a8edea 0%%, #fed6e3 100%%); color: #1E293B; }
        .stat-label { font-size: 13px; opacity: 0.9; margin-bottom: 8px; }
        .stat-number { font-size: 28px; font-weight: 700; }
        .tabs {
            display: flex; gap: 8px; border-bottom: 2px solid #E2E8F0; margin-bottom: 24px;
        }
        .tab {
            padding: 12px 24px; background: transparent; border: none;
            font-size: 15px; font-weight: 500; color: #64748B;
            cursor: pointer; border-bottom: 3px solid transparent;
            transition: all 0.3s; position: relative; bottom: -2px;
        }
        .tab:hover { color: #5B68FF; }
        .tab.active { color: #5B68FF; border-bottom-color: #5B68FF; }
        .tab-content { display: none; }
        .tab-content.active { display: block; }
        .data-table { width: 100%%; border-collapse: collapse; margin-top: 12px; }
        .data-table th {
            background: #F8FAFC; padding: 12px 16px; text-align: left;
            font-size: 13px; font-weight: 600; color: #475569;
            border-bottom: 2px solid #E2E8F0;
        }
        .data-table td {
            padding: 12px 16px; border-bottom: 1px solid #E2E8F0;
            font-size: 14px; color: #1E293B;
        }
        .data-table tr:hover { background: #F8FAFC; }
        .data-table a { color: #5B68FF; text-decoration: none; word-break: break-all; }
        .data-table a:hover { text-decoration: underline; }
        .empty-state { text-align: center; padding: 48px 24px; color: #94A3B8; }
        .empty-state-icon { font-size: 48px; margin-bottom: 16px; }
        .analysis-mode-badge {
            display: inline-block; padding: 6px 12px; border-radius: 8px;
            font-size: 12px; font-weight: 500; background: #DBEAFE; color: #2563EB;
        }
    </style>
</head>
<body>
    <div class="sidebar">
        <a href="/" class="sidebar-icon" title="返回首页">📊</a>
    </div>

    <div class="main-container">
        <div class="header">
            <h1>🔬 静态分析报告</h1>
            <a href="/" class="back-btn">
                <span>←</span>
                <span>返回任务列表</span>
            </a>
        </div>

        <!-- 基本信息 -->
        <div class="content-section">
            <div class="section-title">📋 基本信息</div>
            <div class="info-grid">
                <div class="info-item">
                    <div class="info-label">APK 名称</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">应用名称</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">包名</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">版本</div>
                    <div class="info-value">%s (%s)</div>
                </div>
                <div class="info-item">
                    <div class="info-label">开发者</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">公司/组织</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">文件大小</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">MD5</div>
                    <div class="info-value" style="font-size: 12px; font-family: monospace;">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">分析模式</div>
                    <div class="info-value"><span class="analysis-mode-badge">%s</span></div>
                </div>
                <div class="info-item">
                    <div class="info-label">分析时间</div>
                    <div class="info-value">%s</div>
                </div>
            </div>
        </div>

        <!-- 组件统计 -->
        <div class="content-section">
            <div class="section-title">📊 组件统计</div>
            <div class="stats-grid">
                <div class="stat-card">
                    <div class="stat-label">Activity</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">Service</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">Receiver</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">Provider</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">权限</div>
                    <div class="stat-number">%d</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">URL / 域名</div>
                    <div class="stat-number">%d / %d</div>
                </div>
            </div>
        </div>

        <!-- 详细分析 -->
        <div class="content-section">
            <div class="section-title">🔍 详细分析</div>
            <div class="tabs">
                <button class="tab active" onclick="switchTab('urls')">静态URL (%d)</button>
                <button class="tab" onclick="switchTab('domains')">域名 (%d)</button>
                <button class="tab" onclick="switchTab('activities')">Activity (%d)</button>
                <button class="tab" onclick="switchTab('permissions')">权限 (%d)</button>
            </div>

            <div id="urls" class="tab-content active">%s</div>
            <div id="domains" class="tab-content">%s</div>
            <div id="activities" class="tab-content">%s</div>
            <div id="permissions" class="tab-content">%s</div>
        </div>
    </div>

    <script>
        function switchTab(tabName) {
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });
            document.querySelectorAll('.tab').forEach(tab => {
                tab.classList.remove('active');
            });
            document.getElementById(tabName).classList.add('active');
            event.target.classList.add('active');
        }
    </script>
</body>
</html>
	`,
		html.EscapeString(task.APKName),
		html.EscapeString(task.APKName),
		html.EscapeString(report.AppName),
		html.EscapeString(report.PackageName),
		html.EscapeString(report.VersionName),
		html.EscapeString(report.VersionCode),
		h.formatDeveloperInfo(report.Developer),
		h.formatDeveloperInfo(report.CompanyName),
		fileSize,
		html.EscapeString(report.MD5),
		string(report.AnalysisMode),
		analyzedAt,
		report.ActivityCount,
		report.ServiceCount,
		report.ReceiverCount,
		report.ProviderCount,
		report.PermissionCount,
		report.URLCount, report.DomainCount,
		len(urls), len(domains), len(activities), len(permissions),
		urlsHTML, domainsHTML, activitiesHTML, permissionsHTML,
	)
}

// 辅助函数：从 DeepAnalysis 提取 URLs
func (h *TaskHandler) extractURLsFromDeepAnalysis(deepAnalysis map[string]interface{}) []string {
	if deepAnalysis == nil {
		return nil
	}
	if urls, ok := deepAnalysis["urls"].([]interface{}); ok {
		result := make([]string, 0, len(urls))
		for _, u := range urls {
			if s, ok := u.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// 辅助函数：从 DeepAnalysis 提取域名
func (h *TaskHandler) extractDomainsFromDeepAnalysis(deepAnalysis map[string]interface{}) []string {
	if deepAnalysis == nil {
		return nil
	}
	if domains, ok := deepAnalysis["domains"].([]interface{}); ok {
		result := make([]string, 0, len(domains))
		for _, d := range domains {
			if s, ok := d.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// 辅助函数：从 BasicInfo 提取 Activities
func (h *TaskHandler) extractActivitiesFromBasicInfo(basicInfo map[string]interface{}) []string {
	if basicInfo == nil {
		return nil
	}
	if activities, ok := basicInfo["activities"].([]interface{}); ok {
		result := make([]string, 0, len(activities))
		for _, a := range activities {
			if s, ok := a.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// 辅助函数：从 BasicInfo 提取权限
func (h *TaskHandler) extractPermissionsFromBasicInfo(basicInfo map[string]interface{}) []string {
	if basicInfo == nil {
		return nil
	}
	if permissions, ok := basicInfo["permissions"].([]interface{}); ok {
		result := make([]string, 0, len(permissions))
		for _, p := range permissions {
			if s, ok := p.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// 辅助函数：格式化文件大小
func (h *TaskHandler) formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(size)/(1024*1024*1024))
}

// 生成 URLs 表格
func (h *TaskHandler) generateHybridURLsTable(urls []string) string {
	if len(urls) == 0 {
		return `<div class="empty-state"><div class="empty-state-icon">🔗</div><div>未发现静态 URL</div></div>`
	}
	result := `<table class="data-table"><tr><th style="width: 60px;">#</th><th>URL</th></tr>`
	for i, url := range urls {
		if i >= 200 {
			result += fmt.Sprintf(`<tr><td colspan="2" style="text-align:center; color:#64748B;">... 还有 %d 个 URL</td></tr>`, len(urls)-200)
			break
		}
		result += fmt.Sprintf(`<tr><td>%d</td><td><a href="%s" target="_blank">%s</a></td></tr>`,
			i+1, html.EscapeString(url), html.EscapeString(url))
	}
	result += `</table>`
	return result
}

// 生成域名表格
func (h *TaskHandler) generateHybridDomainsTable(domains []string) string {
	if len(domains) == 0 {
		return `<div class="empty-state"><div class="empty-state-icon">🌐</div><div>未发现域名</div></div>`
	}
	result := `<table class="data-table"><tr><th style="width: 60px;">#</th><th>域名</th></tr>`
	for i, domain := range domains {
		if i >= 200 {
			result += fmt.Sprintf(`<tr><td colspan="2" style="text-align:center; color:#64748B;">... 还有 %d 个域名</td></tr>`, len(domains)-200)
			break
		}
		result += fmt.Sprintf(`<tr><td>%d</td><td>%s</td></tr>`, i+1, html.EscapeString(domain))
	}
	result += `</table>`
	return result
}

// 生成 Activities 表格
func (h *TaskHandler) generateHybridActivitiesTable(activities []string) string {
	if len(activities) == 0 {
		return `<div class="empty-state"><div class="empty-state-icon">📱</div><div>未发现 Activity</div></div>`
	}
	result := `<table class="data-table"><tr><th style="width: 60px;">#</th><th>Activity 名称</th></tr>`
	for i, activity := range activities {
		if i >= 100 {
			result += fmt.Sprintf(`<tr><td colspan="2" style="text-align:center; color:#64748B;">... 还有 %d 个 Activity</td></tr>`, len(activities)-100)
			break
		}
		result += fmt.Sprintf(`<tr><td>%d</td><td>%s</td></tr>`, i+1, html.EscapeString(activity))
	}
	result += `</table>`
	return result
}

// 生成权限表格
func (h *TaskHandler) generateHybridPermissionsTable(permissions []string) string {
	if len(permissions) == 0 {
		return `<div class="empty-state"><div class="empty-state-icon">🔐</div><div>未发现权限</div></div>`
	}
	result := `<table class="data-table"><tr><th style="width: 60px;">#</th><th>权限名称</th></tr>`
	for i, perm := range permissions {
		result += fmt.Sprintf(`<tr><td>%d</td><td>%s</td></tr>`, i+1, html.EscapeString(perm))
	}
	result += `</table>`
	return result
}

// formatDeveloperInfo 格式化开发者信息，如果为空则显示"未知"
func (h *TaskHandler) formatDeveloperInfo(info string) string {
	if info == "" {
		return `<span style="color: #94A3B8;">未知</span>`
	}
	return html.EscapeString(info)
}
