package domainanalysis

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	appDomain "github.com/apk-analysis/apk-analysis-go/internal/domain"
	"github.com/apk-analysis/apk-analysis-go/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 强制重新编译标记
const forceRebuild = "v2024-11-20-FINAL-TEST-WITH-CHINESE-LOGS-12345"

// AnalysisService 域名分析服务
type AnalysisService struct {
	analyzer      *DomainAnalyzer
	beian         *BeianChecker
	ipLocation    *IPLocationClient
	sdkManager    *SDKManager
	urlClassifier *URLClassifier // 新增：URL 分类器
	taskRepo      repository.TaskRepository
	db            *gorm.DB
	logger        *logrus.Logger
	resultsDir    string // 🔧 添加 resultsDir 字段用于正确读取 flows.jsonl
}

// NewAnalysisService 创建域名分析服务
func NewAnalysisService(db *gorm.DB, taskRepo repository.TaskRepository, logger *logrus.Logger) *AnalysisService {
	sdkManager := NewSDKManager(db, logger)
	return &AnalysisService{
		analyzer:      NewDomainAnalyzer(logger, sdkManager),
		beian:         NewBeianChecker(logger), // 使用默认配置(禁用)
		ipLocation:    NewIPLocationClient(logger),
		sdkManager:    sdkManager,
		urlClassifier: NewURLClassifier(logger, sdkManager), // 新增
		taskRepo:      taskRepo,
		db:            db,
		logger:        logger,
	}
}

// NewAnalysisServiceWithConfig 使用配置创建域名分析服务
func NewAnalysisServiceWithConfig(db *gorm.DB, taskRepo repository.TaskRepository, logger *logrus.Logger, beianConfig *BeianCheckerConfig, resultsDir string) *AnalysisService {
	sdkManager := NewSDKManager(db, logger)
	return &AnalysisService{
		analyzer:      NewDomainAnalyzer(logger, sdkManager),
		beian:         NewBeianCheckerWithConfig(logger, beianConfig),
		ipLocation:    NewIPLocationClient(logger),
		sdkManager:    sdkManager,
		urlClassifier: NewURLClassifier(logger, sdkManager), // 新增
		taskRepo:      taskRepo,
		db:            db,
		logger:        logger,
		resultsDir:    resultsDir, // 🔧 传入 resultsDir 用于读取 flows.jsonl
	}
}

// AnalyzeTask 分析任务的域名信息
func (s *AnalysisService) AnalyzeTask(ctx context.Context, taskID string) error {
	// 🎯 第一行日志：确认方法被调用
	s.logger.WithField("task_id", taskID).Info("🎯🎯🎯 AnalyzeTask 方法被调用！！！")

	s.logger.WithFields(logrus.Fields{
		"task_id": taskID,
		"step":    "开始",
	}).Info("🚀🚀🚀 ========== [域名分析] 开始执行 ==========")

	// 步骤1: 从数据库加载任务
	s.logger.WithFields(logrus.Fields{
		"task_id": taskID,
		"step":    "步骤1",
	}).Info("📖 [步骤1] 从数据库加载任务数据...")

	var task appDomain.Task
	err := s.db.WithContext(ctx).
		Preload("Activities").
		Preload("StaticReport").
		First(&task, "id = ?", taskID).Error
	if err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("❌ [步骤1] 加载任务失败")
		return err
	}
	s.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"package_name": task.PackageName,
		"apk_name":     task.APKName,
	}).Info("✅ [步骤1] 任务加载成功")

	// 步骤2: 提取动态 URL
	s.logger.WithField("task_id", taskID).Info("📡 [步骤2] 提取动态分析URL（从Activity流量记录）...")
	dynamicURLs := s.extractDynamicURLs(&task)
	s.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"dynamic_urls": len(dynamicURLs),
	}).Info("✅ [步骤2] 动态URL提取完成")

	// 步骤3: 提取静态 URL
	s.logger.WithField("task_id", taskID).Info("📊 [步骤3] 提取静态分析URL（从静态分析报告）...")
	staticURLs := s.extractStaticURLs(&task)
	s.logger.WithFields(logrus.Fields{
		"task_id":     taskID,
		"static_urls": len(staticURLs),
		"has_static":  task.StaticReport != nil,
	}).Info("✅ [步骤3] 静态URL提取完成")

	// 步骤4: 分析主域名
	s.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"total_urls":   len(dynamicURLs) + len(staticURLs),
		"dynamic_urls": len(dynamicURLs),
		"static_urls":  len(staticURLs),
	}).Info("🔍 [步骤4] 开始分析主域名（合并动态+静态URL）...")

	// 提取应用名称（用于拼音匹配）
	appName := s.extractAppName(&task)

	primaryResult := s.analyzer.AnalyzePrimaryDomain(
		ctx,
		task.PackageName,
		task.APKName,
		appName, // 新增：传入应用名称用于拼音匹配
		dynamicURLs,
		staticURLs,
	)

	s.logger.WithFields(logrus.Fields{
		"task_id":          taskID,
		"primary_domain":   primaryResult.PrimaryDomain,
		"confidence":       primaryResult.Confidence,
		"candidates_count": len(primaryResult.Candidates),
	}).Info("✅ [步骤4] 主域名分析完成")

	// 步骤4.5: URL 分类分析（新规则）
	s.logger.WithField("task_id", taskID).Info("🏷️ [步骤4.5] 开始 URL 分类分析（6条规则匹配）...")

	// 构建应用信息（从静态分析报告获取开发者信息）
	developer := s.getDeveloperFromStaticReport(ctx, taskID)
	appInfo := &AppInfo{
		AppName:     s.extractAppName(&task),
		PackageName: task.PackageName,
		Developer:   developer,
	}

	// 合并所有 URL 进行分类
	allURLsForClassify := make([]string, 0, len(dynamicURLs)+len(staticURLs))
	allURLsForClassify = append(allURLsForClassify, dynamicURLs...)
	allURLsForClassify = append(allURLsForClassify, staticURLs...)

	// 执行 URL 分类
	urlClassifications := s.urlClassifier.ClassifyURLs(ctx, allURLsForClassify, appInfo)
	classificationSummary := SummarizeClassifications(urlClassifications)

	s.logger.WithFields(logrus.Fields{
		"task_id":           taskID,
		"total_urls":        classificationSummary.TotalCount,
		"app_server_count":  classificationSummary.AppServerCount,
		"third_party_count": classificationSummary.ThirdPartyCount,
		"unknown_count":     classificationSummary.UnknownCount,
	}).Info("✅ [步骤4.5] URL 分类分析完成")

	// 步骤4.6: 解析应用服务器URL的IP归属地
	s.logger.WithField("task_id", taskID).Info("🌐 [步骤4.6] 开始解析应用服务器URL的IP归属地...")
	if classificationSummary != nil && len(classificationSummary.AppServerURLs) > 0 {
		appServerDomains := s.extractDomainsFromClassifications(classificationSummary.AppServerURLs)
		s.logger.WithFields(logrus.Fields{
			"task_id":              taskID,
			"app_server_urls":      len(classificationSummary.AppServerURLs),
			"unique_domains":       len(appServerDomains),
		}).Info("📋 [步骤4.6] 提取应用服务器域名完成")

		if len(appServerDomains) > 0 {
			s.resolveAndSaveAppServerDomains(ctx, taskID, appServerDomains)
		}
	} else {
		s.logger.WithField("task_id", taskID).Info("⚠️ [步骤4.6] 无应用服务器URL，跳过IP解析")
	}

	// 步骤5: 查询应用备案信息
	s.logger.WithField("task_id", taskID).Info("🏢 [步骤5] 查询应用备案信息...")
	var beianResults []*BeianResult

	// appName 已在步骤4提取，直接使用
	if appName != "" {
		s.logger.WithFields(logrus.Fields{
			"task_id":  taskID,
			"app_name": appName,
		}).Info("📝 [步骤5] 使用应用名称查询备案（站长工具API）")
		beianResult := s.beian.CheckBeianByAppName(ctx, appName)
		beianResults = append(beianResults, beianResult)
		s.logger.WithFields(logrus.Fields{
			"task_id": taskID,
			"status":  beianResult.Status,
			"error":   beianResult.Error,
		}).Info("✅ [步骤5] 备案查询完成")
	} else {
		s.logger.WithField("task_id", taskID).Warn("⚠️ [步骤5] 未找到应用名称，跳过备案查询")
	}

	// 步骤6: 提取子域名（过滤第三方域名）
	s.logger.WithField("task_id", taskID).Info("🌐 [步骤6] 开始提取子域名和IP地址...")

	domainsToQuery := []string{}
	ipsToQuery := []string{}

	// 步骤6.1: 获取主域名
	mainDomain := primaryResult.PrimaryDomain
	if mainDomain == "" {
		s.logger.WithField("task_id", taskID).Warn("⚠️ [步骤6.1] 未识别到主域名，将仅保留应用服务器域名")
		mainDomain = "" // 严格模式：仅保留应用服务器域名
	} else {
		s.logger.WithFields(logrus.Fields{
			"task_id":     taskID,
			"main_domain": mainDomain,
		}).Info("✅ [步骤6.1] 主域名已确定，将只提取相关子域名")
	}

	// 步骤6.2: 从所有 URLs 中提取域名
	s.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"total_urls":   len(dynamicURLs) + len(staticURLs),
		"dynamic_urls": len(dynamicURLs),
		"static_urls":  len(staticURLs),
	}).Info("🔧 [步骤6.2] 从URL中提取域名...")

	allURLs := make([]string, 0, len(dynamicURLs)+len(staticURLs))
	allURLs = append(allURLs, dynamicURLs...)
	allURLs = append(allURLs, staticURLs...)

	allDomains := s.analyzer.ExtractAllDomains(allURLs)
	domainSet := make(map[string]bool)

	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"total_domains": len(allDomains),
	}).Info("✅ [步骤6.2] 域名提取完成")

	// 步骤6.3: 过滤域名（保留主域名子域名 + URL分类中的应用服务器域名）
	s.logger.WithField("task_id", taskID).Info("🔍 [步骤6.3] 过滤域名（主域名子域名 + 应用服务器域名）...")

	// 收集 URL 分类中的应用服务器域名
	appServerDomainSet := make(map[string]bool)
	if classificationSummary != nil && len(classificationSummary.AppServerURLs) > 0 {
		for _, u := range classificationSummary.AppServerURLs {
			if u.Domain != "" {
				appServerDomainSet[u.Domain] = true
			}
		}
		s.logger.WithFields(logrus.Fields{
			"task_id":                  taskID,
			"app_server_domain_count":  len(appServerDomainSet),
		}).Info("📋 [步骤6.3] 收集到应用服务器域名")
	}

	filteredCount := 0
	skippedCount := 0
	appServerCount := 0

	for _, domain := range allDomains {
		if domain == "" {
			continue
		}

		// 去重
		if domainSet[domain] {
			continue
		}

		// 检查是否与主域名相关
		isRelated := false
		if mainDomain != "" {
			if domain == mainDomain {
				isRelated = true
			} else if strings.HasSuffix(domain, "."+mainDomain) {
				isRelated = true
			}
		}

		// 新增：URL分类中的应用服务器域名也保留
		if appServerDomainSet[domain] {
			isRelated = true
			appServerCount++
		}

		if !isRelated {
			skippedCount++
			continue
		}

		domainSet[domain] = true
		filteredCount++

		// 区分域名和 IP
		if s.isIPAddress(domain) {
			ipsToQuery = append(ipsToQuery, domain)
		} else {
			domainsToQuery = append(domainsToQuery, domain)
		}
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":          taskID,
		"filtered_count":   filteredCount,
		"skipped_count":    skippedCount,
		"app_server_added": appServerCount,
		"domains":          len(domainsToQuery),
		"ips":              len(ipsToQuery),
	}).Info("✅ [步骤6.3] 域名过滤完成")

	// 步骤6.4: 确保主域名被包含
	if mainDomain != "" && !domainSet[mainDomain] {
		if s.isIPAddress(mainDomain) {
			ipsToQuery = append(ipsToQuery, mainDomain)
		} else {
			domainsToQuery = append(domainsToQuery, mainDomain)
		}
		domainSet[mainDomain] = true
		s.logger.WithFields(logrus.Fields{
			"task_id":     taskID,
			"main_domain": mainDomain,
		}).Info("✅ [步骤6.4] 主域名已添加到查询列表")
	}

	// 步骤6.5: 从主域名分析结果中提取子域名
	s.logger.WithFields(logrus.Fields{
		"task_id":          taskID,
		"candidates_count": len(primaryResult.Candidates),
		"main_domain":      mainDomain,
	}).Info("🔎 [步骤6.5] 从主域名分析结果中提取子域名...")

	if len(primaryResult.Candidates) > 0 {
		totalSubdomainsAdded := 0

		for i, candidate := range primaryResult.Candidates {
			s.logger.WithFields(logrus.Fields{
				"task_id":         taskID,
				"candidate_index": i + 1,
				"domain":          candidate.Domain,
				"subdomain_count": len(candidate.Subdomains),
				"is_main_domain":  candidate.Domain == mainDomain,
			}).Info("📋 [步骤6.5] 检查候选域名的子域名...")

			if len(candidate.Subdomains) > 0 {
				s.logger.WithFields(logrus.Fields{
					"task_id":    taskID,
					"candidate":  candidate.Domain,
					"subdomains": candidate.Subdomains,
				}).Info("🔍 [步骤6.5] 发现子域名列表，开始逐个检查...")

				addedCount := 0
				for _, subdomain := range candidate.Subdomains {
					if subdomain == "" {
						continue
					}

					// 检查子域名是否属于主域名范围
					belongsToMainDomain := false
					if mainDomain != "" {
						if subdomain == mainDomain {
							belongsToMainDomain = true
						} else if strings.HasSuffix(subdomain, "."+mainDomain) {
							belongsToMainDomain = true
						}
					} else {
						belongsToMainDomain = true
					}

					if belongsToMainDomain && !domainSet[subdomain] {
						domainSet[subdomain] = true

						// 区分域名和IP地址
						if s.isIPAddress(subdomain) {
							ipsToQuery = append(ipsToQuery, subdomain)
							s.logger.WithFields(logrus.Fields{
								"task_id":   taskID,
								"subdomain": subdomain,
								"type":      "IP地址",
							}).Info("✅ [步骤6.5] 添加IP地址到查询列表")
						} else {
							domainsToQuery = append(domainsToQuery, subdomain)
							s.logger.WithFields(logrus.Fields{
								"task_id":   taskID,
								"subdomain": subdomain,
								"type":      "域名",
							}).Info("✅ [步骤6.5] 添加子域名到查询列表")
						}

						addedCount++
						totalSubdomainsAdded++
					}
				}

				s.logger.WithFields(logrus.Fields{
					"task_id":          taskID,
					"candidate_domain": candidate.Domain,
					"added_count":      addedCount,
				}).Info("✅ [步骤6.5] 候选域名处理完成")
			}
		}

		// 统计子域名数量
		subdomainCount := len(domainsToQuery) + len(ipsToQuery)
		if mainDomain != "" && domainSet[mainDomain] {
			subdomainCount-- // 减去主域名本身
		}

		s.logger.WithFields(logrus.Fields{
			"task_id":          taskID,
			"main_domain":      mainDomain,
			"total_domains":    len(domainsToQuery),
			"total_ips":        len(ipsToQuery),
			"subdomain_count":  subdomainCount,
			"added_from_candidates": totalSubdomainsAdded,
		}).Info("✅ [步骤6.5] 子域名提取完成")
	} else {
		s.logger.WithField("task_id", taskID).Warn("⚠️ [步骤6.5] 主域名分析结果中无候选域名")
	}

	// 步骤7: 提取直连IP地址
	s.logger.WithField("task_id", taskID).Info("🔗 [步骤7] 提取直连IP地址（URL中直接使用IP）...")
	directIPs := s.extractDirectIPs(&task)
	directIPCount := 0
	for _, ip := range directIPs {
		// 去重
		if !domainSet[ip] {
			ipsToQuery = append(ipsToQuery, ip)
			domainSet[ip] = true
			directIPCount++
		}
	}
	s.logger.WithFields(logrus.Fields{
		"task_id":        taskID,
		"direct_ips":     directIPCount,
		"total_ips_now":  len(ipsToQuery),
	}).Info("✅ [步骤7] 直连IP提取完成")

	// 步骤8: 准备查询列表
	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"domains_count": len(domainsToQuery),
		"ips_count":     len(ipsToQuery),
		"domains_list":  domainsToQuery,
		"ips_list":      ipsToQuery,
	}).Info("📋 [步骤8] 准备查询IP归属地...")

	if len(domainsToQuery) > 200 {
		s.logger.WithFields(logrus.Fields{
			"task_id":       taskID,
			"total_domains": len(domainsToQuery),
		}).Warn("⚠️ [步骤8] 域名数量较多，但不限制（均为主域名相关）")
	}

	if len(ipsToQuery) > 100 {
		s.logger.WithFields(logrus.Fields{
			"task_id":    taskID,
			"total_ips":  len(ipsToQuery),
			"limited_to": 100,
		}).Warn("⚠️ [步骤8] IP数量过多，限制为100个")
		ipsToQuery = ipsToQuery[:100]
	}

	// 步骤9: 批量查询IP归属地（多源 DNS：电信+移动）
	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"domains_query": len(domainsToQuery),
		"ips_query":     len(ipsToQuery),
	}).Info("🌍 [步骤9] 开始批量查询IP归属地（多源DNS: 电信+移动 -> IP138 API）...")

	// 使用多源 DNS 解析（电信+移动）
	multiResults := s.ipLocation.BatchQueryDomainsMulti(ctx, domainsToQuery)

	// 🔧 修复：保存所有多源 IP 结果，而不是只保存第一个
	// 使用 "domain:ip" 作为 key，这样同一个域名可以有多条记录（不同 IP/不同来源）
	ipResults := make(map[string]*IPLocationResult)
	for domain, multiResult := range multiResults {
		if len(multiResult.Results) > 0 {
			for _, result := range multiResult.Results {
				// 使用 domain:ip 作为 key，确保不同 IP 都能保存
				key := domain + ":" + result.IP
				ipResults[key] = result

				// 确保 dns_source 被保存到 Source 字段
				if result.Info != nil {
					if dnsSource, ok := result.Info["dns_source"]; ok {
						result.Source = dnsSource
					}
				}
			}

			// 记录多源 DNS 结果
			s.logger.WithFields(logrus.Fields{
				"task_id":    taskID,
				"domain":     domain,
				"ip_count":   len(multiResult.Results),
				"ip_sources": getIPSources(multiResult.Results),
			}).Info("🔀 [多源DNS] 域名解析到多个 IP")
		}
	}

	directIPResults := s.ipLocation.BatchQueryIPs(ctx, ipsToQuery)

	// 合并查询结果
	for ip, result := range directIPResults {
		ipResults[ip] = result
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"success_count": len(ipResults),
	}).Info("✅ [步骤9] IP归属地查询完成（多源DNS）")

	// 步骤10: 确保所有域名都有记录
	s.logger.WithField("task_id", taskID).Info("💾 [步骤10] 为未查询成功的域名创建空记录...")
	emptyRecordCount := 0
	// 收集已有结果的域名
	existingDomains := make(map[string]bool)
	for _, result := range ipResults {
		existingDomains[result.Domain] = true
	}
	// 为没有结果的域名创建空记录
	for _, domain := range domainsToQuery {
		if !existingDomains[domain] {
			key := domain + ":"
			ipResults[key] = &IPLocationResult{
				Domain: domain,
				IP:     "",
				Source: "unknown",
			}
			emptyRecordCount++
		}
	}
	if emptyRecordCount > 0 {
		s.logger.WithFields(logrus.Fields{
			"task_id":      taskID,
			"empty_records": emptyRecordCount,
		}).Info("✅ [步骤10] 空记录创建完成")
	}

	// 步骤11: 保存到数据库
	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"total_records": len(ipResults),
	}).Info("💾 [步骤11] 保存域名分析结果到数据库...")

	if err := s.saveToDB(ctx, taskID, primaryResult, beianResults, ipResults, classificationSummary); err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("❌ [步骤11] 保存失败")
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":           taskID,
		"primary_domain":    primaryResult.PrimaryDomain,
		"confidence":        primaryResult.Confidence,
		"saved_records":     len(ipResults),
		"url_app_server":    classificationSummary.AppServerCount,
		"url_third_party":   classificationSummary.ThirdPartyCount,
		"url_unknown":       classificationSummary.UnknownCount,
	}).Info("✅✅✅ ========== [域名分析] 全部完成 ==========")

	return nil
}

// extractDynamicURLs 从多个来源提取动态 URL
// 🔧 修复：同时读取 flows.jsonl 文件和 activity_details_json，确保全量流量被分析
func (s *AnalysisService) extractDynamicURLs(task *appDomain.Task) []string {
	urlSet := make(map[string]bool) // 用于去重
	urls := []string{}

	// 方法1: 从 activity_details_json 提取（已归因流量）
	if task.Activities != nil && task.Activities.ActivityDetailsJSON != "" {
		var details []map[string]interface{}
		if err := json.Unmarshal([]byte(task.Activities.ActivityDetailsJSON), &details); err == nil {
			for _, detail := range details {
				if flows, ok := detail["flows"].([]interface{}); ok {
					for _, flow := range flows {
						if flowMap, ok := flow.(map[string]interface{}); ok {
							if urlStr, ok := flowMap["url"].(string); ok && urlStr != "" {
								if !urlSet[urlStr] {
									urlSet[urlStr] = true
									urls = append(urls, urlStr)
								}
							}
						}
					}
				}
			}
			s.logger.WithFields(logrus.Fields{
				"task_id":            task.ID,
				"urls_from_activity": len(urls),
			}).Info("📋 Extracted URLs from activity_details_json")
		}
	}

	// 方法2: 从 flows.jsonl 文件提取（全量流量，包括时间窗口外的请求）
	// 这是关键修复：确保所有流量都被域名分析使用
	// 🔧 修复：使用配置的 resultsDir 而非硬编码路径
	flowsPath := filepath.Join(s.resultsDir, task.ID, "flows.jsonl")

	s.logger.WithFields(logrus.Fields{
		"task_id":     task.ID,
		"flows_path":  flowsPath,
		"results_dir": s.resultsDir,
	}).Info("📂 [DEBUG] Attempting to read flows.jsonl file...")

	if file, err := os.Open(flowsPath); err == nil {
		defer file.Close()

		scanner := bufio.NewScanner(file)
		flowsFileCount := 0
		for scanner.Scan() {
			var flow map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &flow); err == nil {
				if urlStr, ok := flow["url"].(string); ok && urlStr != "" {
					if !urlSet[urlStr] {
						urlSet[urlStr] = true
						urls = append(urls, urlStr)
						flowsFileCount++
					}
				}
			}
		}

		s.logger.WithFields(logrus.Fields{
			"task_id":           task.ID,
			"urls_from_flows":   flowsFileCount,
			"total_unique_urls": len(urls),
		}).Info("✅ Extracted URLs from flows.jsonl file")
	} else {
		s.logger.WithFields(logrus.Fields{
			"task_id": task.ID,
			"path":    flowsPath,
			"error":   err.Error(),
		}).Warn("⚠️ flows.jsonl file not found or cannot be opened, using only activity_details")
	}

	return urls
}

// extractStaticURLs 从静态分析报告中提取 URL
func (s *AnalysisService) extractStaticURLs(task *appDomain.Task) []string {
	if task.StaticReport == nil || task.StaticReport.DeepAnalysisJSON == "" {
		s.logger.WithField("task_id", task.ID).Info("⚠️ 无静态分析报告，跳过静态URL提取")
		return []string{}
	}

	// 解析静态分析深度分析 JSON
	var deepAnalysis map[string]interface{}
	if err := json.Unmarshal([]byte(task.StaticReport.DeepAnalysisJSON), &deepAnalysis); err != nil {
		s.logger.WithError(err).Warn("Failed to parse static analysis deep report")
		return []string{}
	}

	urls := []string{}

	// 从深度分析报告中提取 URLs
	if urlsList, ok := deepAnalysis["urls"].([]interface{}); ok {
		for _, urlInterface := range urlsList {
			if urlStr, ok := urlInterface.(string); ok && urlStr != "" {
				urls = append(urls, urlStr)
			}
		}
	}

	// 从域名列表提取（转换为 URL 格式）
	if domainsList, ok := deepAnalysis["domains"].([]interface{}); ok {
		for _, domainInterface := range domainsList {
			if domain, ok := domainInterface.(string); ok && domain != "" {
				urls = append(urls, "https://"+domain)
			}
		}
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":     task.ID,
		"static_urls": len(urls),
	}).Info("Extracted static URLs from Hybrid analysis report")

	return urls
}

// extractAppName 从任务信息中提取应用名称
func (s *AnalysisService) extractAppName(task *appDomain.Task) string {
	// 🔧 优先从静态分析报告获取（最新的静态分析结果）
	if task.StaticReport != nil && task.StaticReport.AppName != "" {
		s.logger.WithFields(logrus.Fields{
			"task_id":  task.ID,
			"app_name": task.StaticReport.AppName,
			"source":   "static_report",
		}).Info("✅ 备案查询使用应用名称（来源：静态分析报告）")
		return task.StaticReport.AppName
	}

	// 兜底：使用 APK 文件名（去除 .apk 后缀）
	if task.APKName != "" {
		appName := strings.TrimSuffix(task.APKName, ".apk")
		s.logger.WithFields(logrus.Fields{
			"task_id":  task.ID,
			"app_name": appName,
			"source":   "apk_filename",
		}).Warn("⚠️ 备案查询使用APK文件名（兜底方案）")
		return appName
	}

	s.logger.WithField("task_id", task.ID).Warn("⚠️ 未找到应用名称，无法查询备案")
	return ""
}

// extractDirectIPs 从 activity_details 中提取直连IP地址
func (s *AnalysisService) extractDirectIPs(task *appDomain.Task) []string {
	if task.Activities == nil || task.Activities.ActivityDetailsJSON == "" {
		return []string{}
	}

	// 解析 activity_details JSON
	var details []map[string]interface{}
	if err := json.Unmarshal([]byte(task.Activities.ActivityDetailsJSON), &details); err != nil {
		s.logger.WithError(err).Warn("Failed to parse activity details for direct IPs")
		return []string{}
	}

	// 提取所有直连IP（host字段为IP地址的情况）
	ipSet := make(map[string]bool)
	ips := []string{}

	for _, detail := range details {
		if flows, ok := detail["flows"].([]interface{}); ok {
			for _, flow := range flows {
				if flowMap, ok := flow.(map[string]interface{}); ok {
					if host, ok := flowMap["host"].(string); ok && host != "" {
						// 检查host是否是IP地址
						if s.isIPAddress(host) && !ipSet[host] {
							ipSet[host] = true
							ips = append(ips, host)
						}
					}
				}
			}
		}
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":    task.ID,
		"direct_ips": len(ips),
	}).Info("Extracted direct IPs from activity details")

	return ips
}

// isIPAddress 检查字符串是否是IPv4地址
func (s *AnalysisService) isIPAddress(host string) bool {
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

// saveToDB 保存分析结果到数据库
func (s *AnalysisService) saveToDB(
	ctx context.Context,
	taskID string,
	primaryResult *PrimaryDomainResult,
	beianResults []*BeianResult,
	ipResults map[string]*IPLocationResult,
	urlClassification *ClassificationSummary,
) error {
	// 构建 DomainAnalysis 对象
	now := time.Now()
	confidence := primaryResult.Confidence // 提取置信度
	domainAnalysis := &appDomain.TaskDomainAnalysis{
		TaskID:                  taskID,
		PrimaryDomain:           primaryResult.PrimaryDomain, // 🔧 修复：添加 PrimaryDomain 字段
		PrimaryDomainConfidence: &confidence,                 // 新增：保存置信度到独立字段，优化查询
		AnalyzedAt:              &now,                        // 🔧 修复：添加 AnalyzedAt 字段
	}

	// 保存主域名分析结果
	primaryJSON, _ := json.Marshal(primaryResult)
	domainAnalysis.PrimaryDomainJSON = string(primaryJSON)

	// 保存备案信息
	beianJSON, _ := json.Marshal(beianResults)
	domainAnalysis.DomainBeianJSON = string(beianJSON)
	domainAnalysis.DomainBeianStatus = summarizeBeianStatus(beianResults)

	// 转换 IP 结果为数组
	ipResultArray := []IPLocationResult{}
	for _, result := range ipResults {
		ipResultArray = append(ipResultArray, *result)
	}

	// 保存 IP 归属地信息到 JSON 字段（兼容旧版本）
	ipJSON, _ := json.Marshal(ipResultArray)
	domainAnalysis.AppDomainsJSON = string(ipJSON)

	// 保存 URL 分类结果到 JSON 字段（新增）
	if urlClassification != nil {
		urlClassificationJSON, _ := json.Marshal(urlClassification)
		domainAnalysis.URLClassificationJSON = string(urlClassificationJSON)
	}

	// 使用专门的 SaveDomainAnalysis 方法保存
	if err := s.taskRepo.SaveDomainAnalysis(ctx, domainAnalysis); err != nil {
		return err
	}

	// 保存 IP 归属地到 task_app_domains 表（新版本，支持更好的查询）
	return s.saveAppDomainsToTable(ctx, taskID, ipResults)
}

func summarizeBeianStatus(results []*BeianResult) string {
	if len(results) == 0 {
		return ""
	}

	result := results[0]
	status := string(result.Status)

	switch result.Status {
	case BeianStatusRegistered:
		return "已备案"
	case BeianStatusNotRegistered:
		return "未备案"
	case BeianStatusError:
		if reason, ok := result.Info["reason"]; ok && strings.Contains(reason, "暂无数据") {
			return "未备案"
		}
		return "查询失败"
	case BeianStatusDisabled:
		return "查询失败"
	}

	switch status {
	case "ok", "已备案":
		return "已备案"
	case "not_found", "未备案":
		return "未备案"
	case "查询失败":
		return "查询失败"
	default:
		return "查询失败"
	}
}

// saveAppDomainsToTable 保存 IP 归属地到 task_app_domains 表
func (s *AnalysisService) saveAppDomainsToTable(
	ctx context.Context,
	taskID string,
	ipResults map[string]*IPLocationResult,
) error {
	// 先删除该任务的旧数据（如果重新分析）
	if err := s.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&appDomain.TaskAppDomain{}).Error; err != nil {
		s.logger.WithError(err).Warn("Failed to delete old app domains")
		// 不中断流程，继续保存新数据
	}

	// 批量插入新数据
	for _, result := range ipResults {
		// 🔧 修复：即使查询失败，也保存域名记录（IP/归属地为空）
		// 这样可以确保所有子域名都显示在前端，即使没有归属地信息

		taskAppDomain := &appDomain.TaskAppDomain{
			TaskID:   taskID,
			Domain:   result.Domain,
			IP:       result.IP,       // 可能为空（查询失败）
			Province: result.Province, // 可能为空
			City:     result.City,     // 可能为空
			ISP:      result.ISP,      // 可能为空
			Source:   result.Source,   // 可能为 "unknown"
		}

		// 插入到数据库
		if err := s.db.WithContext(ctx).Create(taskAppDomain).Error; err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"task_id": taskID,
				"domain":  result.Domain,
				"ip":      result.IP,
			}).Warn("Failed to save app domain to table")
			// 不中断流程，继续保存其他数据
		}
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":     taskID,
		"domains_saved": len(ipResults),
	}).Info("Saved app domains to table")

	return nil
}

// GetTaskDomainAnalysis 获取任务的域名分析结果
func (s *AnalysisService) GetTaskDomainAnalysis(ctx context.Context, taskID string) (*TaskDomainAnalysisResult, error) {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if task.DomainAnalysis == nil {
		return &TaskDomainAnalysisResult{}, nil
	}

	result := &TaskDomainAnalysisResult{}

	// 解析主域名
	if task.DomainAnalysis.PrimaryDomainJSON != "" {
		json.Unmarshal([]byte(task.DomainAnalysis.PrimaryDomainJSON), &result.PrimaryDomain)
	}

	// 解析备案信息
	if task.DomainAnalysis.DomainBeianJSON != "" {
		json.Unmarshal([]byte(task.DomainAnalysis.DomainBeianJSON), &result.BeianInfo)
	}

	// 解析 IP 归属地
	if task.DomainAnalysis.AppDomainsJSON != "" {
		json.Unmarshal([]byte(task.DomainAnalysis.AppDomainsJSON), &result.IPLocations)
	}

	return result, nil
}

// TaskDomainAnalysisResult 任务域名分析结果
type TaskDomainAnalysisResult struct {
	PrimaryDomain *PrimaryDomainResult `json:"primary_domain"`
	BeianInfo     []*BeianResult       `json:"beian_info"`
	IPLocations   []IPLocationResult   `json:"ip_locations"`
}

// getIPSources 从多源 IP 结果中提取来源信息
func getIPSources(results []*IPLocationResult) []string {
	sources := make([]string, 0, len(results))
	for _, r := range results {
		if r.Info != nil {
			if source, ok := r.Info["dns_source"]; ok {
				sources = append(sources, r.IP+"("+source+")")
			} else {
				sources = append(sources, r.IP)
			}
		} else {
			sources = append(sources, r.IP)
		}
	}
	return sources
}

// extractDomainsFromClassifications 从分类结果中提取唯一域名
func (s *AnalysisService) extractDomainsFromClassifications(urls []URLClassification) []string {
	domainSet := make(map[string]bool)
	domains := []string{}

	for _, u := range urls {
		if u.Domain != "" && !domainSet[u.Domain] {
			domainSet[u.Domain] = true
			domains = append(domains, u.Domain)
		}
	}

	return domains
}

// resolveAndSaveAppServerDomains 解析应用服务器域名的IP并保存到数据库
func (s *AnalysisService) resolveAndSaveAppServerDomains(ctx context.Context, taskID string, domains []string) {
	s.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"domain_count": len(domains),
	}).Info("🔍 [步骤4.6] 开始多源DNS解析应用服务器域名...")

	// 分离域名和IP地址
	domainsToQuery := []string{}
	ipsToQuery := []string{}

	for _, domain := range domains {
		if s.isIPAddress(domain) {
			ipsToQuery = append(ipsToQuery, domain)
		} else {
			domainsToQuery = append(domainsToQuery, domain)
		}
	}

	// 使用多源 DNS 解析域名（电信+移动）
	ipResults := make(map[string]*IPLocationResult)

	if len(domainsToQuery) > 0 {
		multiResults := s.ipLocation.BatchQueryDomainsMulti(ctx, domainsToQuery)

		for domain, multiResult := range multiResults {
			if len(multiResult.Results) > 0 {
				for _, result := range multiResult.Results {
					// 使用 domain:ip 作为 key，确保不同 IP 都能保存
					key := domain + ":" + result.IP
					ipResults[key] = result

					// 确保 dns_source 被保存到 Source 字段
					if result.Info != nil {
						if dnsSource, ok := result.Info["dns_source"]; ok {
							result.Source = dnsSource
						}
					}
				}

				s.logger.WithFields(logrus.Fields{
					"task_id":    taskID,
					"domain":     domain,
					"ip_count":   len(multiResult.Results),
				}).Debug("🔀 [步骤4.6] 应用服务器域名DNS解析完成")
			}
		}
	}

	// 查询直连IP的归属地
	if len(ipsToQuery) > 0 {
		directIPResults := s.ipLocation.BatchQueryIPs(ctx, ipsToQuery)
		for ip, result := range directIPResults {
			ipResults[ip] = result
		}
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"total_results": len(ipResults),
	}).Info("✅ [步骤4.6] 应用服务器域名IP解析完成")

	// 保存到 task_app_domains 表（去重：检查是否已存在）
	savedCount := 0
	skippedCount := 0

	for _, result := range ipResults {
		// 检查是否已存在相同的 task_id + domain + ip 组合
		var existingCount int64
		s.db.WithContext(ctx).Model(&appDomain.TaskAppDomain{}).
			Where("task_id = ? AND domain = ? AND ip = ?", taskID, result.Domain, result.IP).
			Count(&existingCount)

		if existingCount > 0 {
			skippedCount++
			continue
		}

		// 插入新记录，标记来源为 app_server_classification
		taskAppDomain := &appDomain.TaskAppDomain{
			TaskID:   taskID,
			Domain:   result.Domain,
			IP:       result.IP,
			Province: result.Province,
			City:     result.City,
			ISP:      result.ISP,
			Source:   "app_server_classification", // 标记来源
		}

		// 如果有 dns_source，追加到 Source 字段
		if result.Info != nil {
			if dnsSource, ok := result.Info["dns_source"]; ok {
				taskAppDomain.Source = "app_server_" + dnsSource
			}
		}

		if err := s.db.WithContext(ctx).Create(taskAppDomain).Error; err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"task_id": taskID,
				"domain":  result.Domain,
				"ip":      result.IP,
			}).Warn("Failed to save app server domain")
		} else {
			savedCount++
		}
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":       taskID,
		"saved_count":   savedCount,
		"skipped_count": skippedCount,
	}).Info("💾 [步骤4.6] 应用服务器域名保存完成")
}

// getDeveloperFromStaticReport 从静态分析报告获取开发者信息
func (s *AnalysisService) getDeveloperFromStaticReport(ctx context.Context, taskID string) string {
	// 查询静态分析报告
	var staticReport appDomain.TaskStaticReport
	if err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&staticReport).Error; err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Debug("Failed to get static report for developer info")
		return ""
	}

	// 优先返回公司名称，如果没有则返回开发者名称
	if staticReport.CompanyName != "" {
		s.logger.WithFields(logrus.Fields{
			"task_id":      taskID,
			"company_name": staticReport.CompanyName,
		}).Info("🏢 Using company name from certificate for URL classification")
		return staticReport.CompanyName
	}

	if staticReport.Developer != "" {
		s.logger.WithFields(logrus.Fields{
			"task_id":   taskID,
			"developer": staticReport.Developer,
		}).Info("👤 Using developer name from certificate for URL classification")
		return staticReport.Developer
	}

	return ""
}
