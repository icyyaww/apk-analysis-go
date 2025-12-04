package domainanalysis

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// DomainAnalyzer 域名分析器
type DomainAnalyzer struct {
	logger     *logrus.Logger
	sdkManager *SDKManager
}

// NewDomainAnalyzer 创建域名分析器
func NewDomainAnalyzer(logger *logrus.Logger, sdkManager *SDKManager) *DomainAnalyzer {
	return &DomainAnalyzer{
		logger:     logger,
		sdkManager: sdkManager,
	}
}

// AnalyzePrimaryDomain 分析主域名
func (da *DomainAnalyzer) AnalyzePrimaryDomain(
	ctx context.Context,
	packageName string,
	apkName string,
	dynamicURLs []string,
	staticURLs []string,
) *PrimaryDomainResult {
	da.logger.WithFields(logrus.Fields{
		"package_name":  packageName,
		"apk_name":      apkName,
		"dynamic_urls":  len(dynamicURLs),
		"static_urls":   len(staticURLs),
	}).Info("🔍🔍🔍 DomainAnalyzer.AnalyzePrimaryDomain 开始执行")

	da.logger.WithFields(logrus.Fields{
		"package_name":  packageName,
		"apk_name":      apkName,
		"dynamic_urls":  len(dynamicURLs),
		"static_urls":   len(staticURLs),
	}).Info("Analyzing primary domain")

	// 1. 构建域名详细信息映射（包含所有URL）
	da.logger.Info("📋 [Analyzer步骤1] 构建域名详细信息映射...")
	domainDetails := da.buildDomainDetails(dynamicURLs, staticURLs)
	da.logger.WithField("domain_count", len(domainDetails)).Info("✅ [Analyzer步骤1] 域名详情构建完成")

	// 2. 计算每个域名的得分（传入 packageName 和 apkName）
	da.logger.Info("🎯 [Analyzer步骤2] 计算每个域名的得分...")
	candidates := da.scoreDomains(ctx, domainDetails, packageName, apkName)
	da.logger.WithField("candidates_count", len(candidates)).Info("✅ [Analyzer步骤2] 域名评分完成")

	// 3. 选择最高分域名
	da.logger.Info("🏆 [Analyzer步骤3] 选择最高分域名...")
	if len(candidates) == 0 {
		da.logger.Warn("⚠️ [Analyzer步骤3] 无候选域名，返回空结果")
		return &PrimaryDomainResult{
			PrimaryDomain: "",
			Confidence:    0,
			Candidates:    []DomainCandidate{},
		}
	}

	// 按分数降序排序
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	primary := candidates[0]
	da.logger.WithFields(logrus.Fields{
		"primary_domain": primary.Domain,
		"score":          primary.Score,
		"subdomains":     len(primary.Subdomains),
	}).Info("✅ [Analyzer步骤3] 主域名选择完成")

	// 🔧 动态归一化：根据是否有包名匹配使用不同的归一化基数
	// 新分数组成：包名15 + 频率8 + 路径5 + 子域名4 + API 5 + 认证3 + CDN惩罚-2 = 最大40分（不含CDN惩罚）
	// 有包名匹配：归一化基数 = 40（完整评分体系）
	// 无包名匹配：归一化基数 = 25（排除包名分后的最大分：8+5+4+5+3=25）
	var normalizeBase float64
	if primary.PackageScore > 0 {
		normalizeBase = 40.0 // 有包名匹配：完整评分体系
	} else {
		normalizeBase = 25.0 // 无包名匹配：排除包名分后的最大分
	}
	confidence := primary.Score / normalizeBase
	if confidence > 1.0 {
		confidence = 1.0
	}

	da.logger.WithFields(logrus.Fields{
		"primary_domain": primary.Domain,
		"confidence":     confidence,
		"score":          primary.Score,
		"subdomains":     primary.Subdomains,
	}).Info("🎉🎉🎉 DomainAnalyzer.AnalyzePrimaryDomain 执行完成！主域名已识别")

	da.logger.WithFields(logrus.Fields{
		"primary_domain": primary.Domain,
		"confidence":     confidence,
		"score":          primary.Score,
	}).Info("Primary domain identified")

	return &PrimaryDomainResult{
		PrimaryDomain: primary.Domain,
		Confidence:    confidence,
		Candidates:    candidates,
		Evidence: map[string]interface{}{
			"package_match":   primary.PackageMatch,
			"request_count":   primary.RequestCount,
			"path_count":      primary.PathCount,
			"subdomain_count": primary.SubdomainCount,
			"is_api":          primary.IsAPI,
			"has_auth":        primary.HasAuth,
			"is_cdn":          primary.IsCDN,
			"is_sdk":          primary.IsSDK,
		},
	}
}

// DomainDetails 域名详细信息
type DomainDetails struct {
	Domain     string
	URLs       []string
	Paths      map[string]bool
	Subdomains map[string]bool
	Count      int
}

// buildDomainDetails 构建域名详细信息
// 🔧 重构：合并静态URL和动态URL，统一处理，避免代码重复
func (da *DomainAnalyzer) buildDomainDetails(dynamicURLs []string, staticURLs []string) map[string]*DomainDetails {
	details := make(map[string]*DomainDetails)

	// 🔧 合并所有URL（静态 + 动态），统一处理
	allURLs := make([]string, 0, len(dynamicURLs)+len(staticURLs))
	allURLs = append(allURLs, dynamicURLs...)
	allURLs = append(allURLs, staticURLs...)

	// 统一处理所有URL
	for _, rawURL := range allURLs {
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		host := parsedURL.Hostname()
		if host == "" {
			continue
		}

		mainDomain := da.extractMainDomain(host)
		if mainDomain == "" {
			continue
		}

		// 如果主域名不存在，创建新记录
		if _, exists := details[mainDomain]; !exists {
			details[mainDomain] = &DomainDetails{
				Domain:     mainDomain,
				URLs:       []string{},
				Paths:      make(map[string]bool),
				Subdomains: make(map[string]bool),
				Count:      0,
			}
		}

		detail := details[mainDomain]
		detail.URLs = append(detail.URLs, rawURL)
		detail.Count++

		// 记录路径
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			detail.Paths[parsedURL.Path] = true
		}

		// 🔧 记录完整的子域名（host），而不只是前缀
		// 如果 host 不等于 mainDomain，说明有子域名
		if host != mainDomain {
			detail.Subdomains[host] = true // 存储完整域名：img.gngkgoods.com
		}
	}

	return details
}

// extractSubdomain 提取子域名部分
func (da *DomainAnalyzer) extractSubdomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return ""
	}

	// 处理二级域名后缀
	secondLevelTLDs := map[string]bool{
		"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true,
		"co.uk": true, "co.jp": true, "co.kr": true,
	}

	if len(parts) >= 3 {
		suffix := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if secondLevelTLDs[suffix] {
			if len(parts) == 3 {
				return ""
			}
			// 返回子域名部分（排除主域名和TLD）
			return strings.Join(parts[:len(parts)-3], ".")
		}
	}

	// 返回子域名部分
	return strings.Join(parts[:len(parts)-2], ".")
}

// extractDomainsFromURLs 从 URL 列表中提取域名
func (da *DomainAnalyzer) extractDomainsFromURLs(urls []string) map[string]int {
	domainCount := make(map[string]int)

	for _, rawURL := range urls {
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		host := parsedURL.Hostname()
		if host == "" {
			continue
		}

		// 提取主域名 (去除子域名)
		mainDomain := da.extractMainDomain(host)
		if mainDomain != "" {
			domainCount[mainDomain]++
		}
	}

	return domainCount
}

// extractMainDomain 提取主域名
func (da *DomainAnalyzer) extractMainDomain(host string) string {
	// 🔧 修复：排除无效 host（空字符串、以.开头的文件扩展名等）
	if host == "" || strings.HasPrefix(host, ".") {
		da.logger.WithFields(logrus.Fields{
			"host":   host,
			"reason": "Invalid host (empty or starts with dot)",
		}).Debug("Host skipped in extractMainDomain")
		return ""
	}

	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return ""
	}

	// 🔧 修复：检查第一个部分是否为空（如 .jpg 分割后为 ["", "jpg"]）
	if parts[0] == "" {
		da.logger.WithFields(logrus.Fields{
			"host":   host,
			"reason": "Invalid host (first part is empty, likely file extension)",
		}).Debug("Host skipped in extractMainDomain")
		return ""
	}

	// 检查是否为IP地址（IPv4格式：所有部分都是数字）
	isIPv4 := true
	for _, part := range parts {
		// 检查是否全是数字
		for _, char := range part {
			if char < '0' || char > '9' {
				isIPv4 = false
				break
			}
		}
		if !isIPv4 {
			break
		}
	}

	// 如果是IP地址，检查是否为私有IP（RFC 1918）
	if isIPv4 && len(parts) == 4 {
		// 🔧 修复：过滤私有IP地址（10.x.x.x, 172.16-31.x.x, 192.168.x.x）
		// 这些通常是内网/开发环境的IP，不应该作为应用的主域名
		firstOctet := 0
		secondOctet := 0
		fmt.Sscanf(parts[0], "%d", &firstOctet)
		fmt.Sscanf(parts[1], "%d", &secondOctet)

		// 检查是否为私有IP
		isPrivate := false
		if firstOctet == 10 {
			isPrivate = true // 10.0.0.0/8
		} else if firstOctet == 172 && secondOctet >= 16 && secondOctet <= 31 {
			isPrivate = true // 172.16.0.0/12
		} else if firstOctet == 192 && secondOctet == 168 {
			isPrivate = true // 192.168.0.0/16
		} else if firstOctet == 127 {
			isPrivate = true // 127.0.0.0/8 (localhost)
		}

		if isPrivate {
			da.logger.WithFields(logrus.Fields{
				"host":   host,
				"reason": "Private IP address (RFC 1918 or localhost)",
			}).Debug("Host skipped in extractMainDomain")
			return ""
		}

		// 公网IP，返回完整IP
		return host
	}

	// 处理常见的二级域名后缀 (如 .com.cn, .co.uk)
	secondLevelTLDs := map[string]bool{
		"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true,
		"co.uk": true, "co.jp": true, "co.kr": true,
	}

	if len(parts) >= 3 {
		suffix := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if secondLevelTLDs[suffix] {
			// 返回三级域名 (如 example.com.cn)
			if len(parts) >= 3 {
				return parts[len(parts)-3] + "." + suffix
			}
		}
	}

	// 🔧 修复：检查TLD（顶级域名）是否为文件扩展名
	tld := parts[len(parts)-1]
	invalidTLDs := map[string]bool{
		"css": true, "js": true, "json": true, "xml": true, "html": true,
		"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
		"svg": true, "ico": true, "woff": true, "ttf": true, "eot": true,
		"mp3": true, "mp4": true, "avi": true, "mkv": true,
		"zip": true, "rar": true, "tar": true, "gz": true,
		"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	}

	if invalidTLDs[tld] {
		da.logger.WithFields(logrus.Fields{
			"host":   host,
			"tld":    tld,
			"reason": "Invalid TLD (file extension)",
		}).Debug("Host skipped in extractMainDomain")
		return ""
	}

	// 返回二级域名 (如 example.com)
	return parts[len(parts)-2] + "." + parts[len(parts)-1]
}

// mergeDomains 合并动态和静态域名
func (da *DomainAnalyzer) mergeDomains(dynamic map[string]int, static []string) map[string]int {
	merged := make(map[string]int)

	// 复制动态域名
	for domain, count := range dynamic {
		merged[domain] = count
	}

	// 添加静态域名 (权重较低)
	for _, domain := range static {
		mainDomain := da.extractMainDomain(domain)
		if mainDomain != "" {
			if _, exists := merged[mainDomain]; !exists {
				merged[mainDomain] = 1 // 静态域名初始计数为 1
			}
		}
	}

	return merged
}

// scoreDomains 计算每个域名的得分（参考Python MVP版本）
func (da *DomainAnalyzer) scoreDomains(
	ctx context.Context,
	domainDetails map[string]*DomainDetails,
	packageName string,
	apkName string,
) []DomainCandidate {
	candidates := make([]DomainCandidate, 0, len(domainDetails))

	// 计算全局最大值（用于归一化）
	maxCount := 0
	maxPaths := 0
	maxSubdomains := 0
	for _, detail := range domainDetails {
		if detail.Count > maxCount {
			maxCount = detail.Count
		}
		if len(detail.Paths) > maxPaths {
			maxPaths = len(detail.Paths)
		}
		if len(detail.Subdomains) > maxSubdomains {
			maxSubdomains = len(detail.Subdomains)
		}
	}

	// 避免除以0
	if maxCount == 0 {
		maxCount = 1
	}
	if maxPaths == 0 {
		maxPaths = 1
	}
	if maxSubdomains == 0 {
		maxSubdomains = 1
	}

	for _, detail := range domainDetails {
		// 🔧 修复：先检查域名是否与应用包名匹配
		// 如果匹配，即使它在第三方SDK列表中，也不应该被排除
		// 例如：分析淘宝APK(com.taobao.taobao)时，taobao.com应该被视为主域名而非第三方SDK
		isAppOwnDomain := da.isDomainMatchingPackage(detail.Domain, packageName, apkName)

		// 🚫 检查是否为SDK域名
		isSDK, sdkPenalty := da.checkSDK(ctx, detail.Domain)

		// 🔧 关键修复：只有当域名是SDK且不是应用自己的域名时才排除
		if isSDK && !isAppOwnDomain {
			da.logger.WithFields(logrus.Fields{
				"domain": detail.Domain,
				"reason": "SDK domain excluded from primary domain competition",
			}).Debug("Domain excluded: SDK")
			continue // 跳过SDK域名，不加入候选列表
		}

		// 如果域名是应用自己的域名但在SDK列表中，记录日志
		if isSDK && isAppOwnDomain {
			da.logger.WithFields(logrus.Fields{
				"domain":       detail.Domain,
				"package_name": packageName,
				"apk_name":     apkName,
				"reason":       "Domain matches app package name, NOT excluded despite being in SDK list",
			}).Info("App's own domain found in SDK list - including in candidates")
			// 重置SDK状态，因为这是应用自己的域名
			isSDK = false
			sdkPenalty = 0
		}

		// 🚫 检查是否为公共TLD域名（.org/.edu/.gov等），如果是则直接跳过
		if da.isPublicTLD(detail.Domain) {
			da.logger.WithFields(logrus.Fields{
				"domain": detail.Domain,
				"reason": "Public TLD domain excluded (org/edu/gov/int/mil)",
			}).Debug("Domain excluded: Public TLD")
			continue // 跳过公共TLD域名，不加入候选列表
		}

		// 🔧 提取子域名列表（转换为字符串切片）
		subdomains := make([]string, 0, len(detail.Subdomains))
		for subdomain := range detail.Subdomains {
			subdomains = append(subdomains, subdomain)
		}

		candidate := DomainCandidate{
			Domain:         detail.Domain,
			RequestCount:   detail.Count,
			PathCount:      len(detail.Paths),
			SubdomainCount: len(detail.Subdomains),
			Subdomains:     subdomains, // 🔧 新增：保存子域名列表
			DynamicCount:   0,          // 🔧 已移除：不再区分动态/静态
			IsSDK:          false,      // 已排除SDK域名
			SDKPenalty:     sdkPenalty, // 保持为0
		}

		// 1. 包名/APK名匹配 (0-15分，优先APK文件名，其次包名)
		// 🔧 评分优化：降低包名权重，提升其他特征权重，使无包名匹配的主域名也能获得高置信度
		packageScore := da.calculatePackageMatchScore(detail.Domain, packageName, apkName)
		// 将原25分缩放到15分
		packageScore = packageScore * 15.0 / 25.0
		candidate.PackageScore = packageScore
		if packageScore > 0 {
			candidate.PackageMatch = true
		}

		// 2. URL频率 (0-8分，归一化) - 原5分提升到8分
		frequencyScore := (float64(detail.Count) / float64(maxCount)) * 8.0
		candidate.FrequencyScore = frequencyScore

		// 3. 路径多样性 (0-5分，归一化) - 原3分提升到5分
		pathScore := (float64(len(detail.Paths)) / float64(maxPaths)) * 5.0
		candidate.PathScore = pathScore

		// 4. 子域名数量 (0-4分，归一化) - 原2分提升到4分
		subdomainScore := (float64(len(detail.Subdomains)) / float64(maxSubdomains)) * 4.0
		candidate.SubdomainScore = subdomainScore

		// 5. 🔧 已移除：动态流量分数（不再区分动态/静态，统一处理）
		dynamicScore := 0.0
		candidate.DynamicScore = dynamicScore

		// 6. API特征 (0-5分) - 原3分提升到5分
		isAPI, apiScore := da.checkAPIFeatures(detail.URLs)
		candidate.IsAPI = isAPI
		candidate.APIScore = apiScore

		// 7. 认证特征 (0-3分) - 原1分提升到3分
		hasAuth, authScore := da.checkAuthFeatures(detail.URLs)
		candidate.HasAuth = hasAuth
		candidate.AuthScore = authScore

		// 8. CDN惩罚 (0或-2分)
		isCDN, cdnPenalty := da.checkCDN(detail.Domain)
		candidate.IsCDN = isCDN
		candidate.CDNPenalty = cdnPenalty

		// 总分计算（SDK和公共TLD已排除，所以不包含这两种惩罚）
		candidate.Score = packageScore + frequencyScore + pathScore +
			subdomainScore + dynamicScore + apiScore + authScore +
			cdnPenalty

		// 确保分数非负
		if candidate.Score < 0 {
			candidate.Score = 0
		}

		da.logger.WithFields(logrus.Fields{
			"domain":      detail.Domain,
			"total_score": candidate.Score,
			"package":     packageScore,
			"frequency":   frequencyScore,
			"path":        pathScore,
			"subdomain":   subdomainScore,
			"dynamic":     dynamicScore,
			"api":         apiScore,
			"auth":        authScore,
			"cdn_penalty": cdnPenalty,
		}).Debug("Domain scored")

		candidates = append(candidates, candidate)
	}

	return candidates
}

// calculatePackageMatchScore 计算包名/APK名匹配分数 (0-25分)
// 🔧 修复Bug：优先匹配包名核心关键词（倒数第一个部分），避免.css等误匹配
func (da *DomainAnalyzer) calculatePackageMatchScore(domain, packageName, apkName string) float64 {
	// 通用词汇黑名单（这些词在域名中很常见，不应该匹配）
	commonWords := map[string]bool{
		"com": true, "cn": true, "net": true, "org": true,
		"android": true, "app": true, "mobile": true,
		"activity": true, "service": true, "application": true,
		"main": true, "core": true, "common": true, "base": true,
		"www": true, "api": true, "sdk": true,
		"phone": true, "androidphone": true, "pc": true,
		"downloadpage": true, "download": true, "wbqd": true,
		// 🔧 新增：文件扩展名黑名单
		"css": true, "js": true, "json": true, "xml": true, "html": true,
		"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
		"svg": true, "ico": true, "woff": true, "ttf": true, "eot": true,
	}

	bestMatch := 0.0
	domainLower := strings.ToLower(domain)

	// 🔧 修复1：排除无效域名（文件扩展名、空域名等）
	if domainLower == "" || strings.HasPrefix(domainLower, ".") {
		da.logger.WithFields(logrus.Fields{
			"domain": domain,
			"reason": "Invalid domain (empty or file extension)",
		}).Debug("Domain skipped")
		return 0.0
	}

	// 🔧 修复2：提取域名主要部分（去除TLD，正确处理.com.cn等）
	domainParts := strings.Split(domainLower, ".")
	domainMain := ""

	if len(domainParts) >= 2 {
		// 检查是否为二级域名后缀（.com.cn等）
		secondLevelTLDs := map[string]bool{
			"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true,
			"co.uk": true, "co.jp": true, "co.kr": true,
		}

		if len(domainParts) >= 3 {
			suffix := domainParts[len(domainParts)-2] + "." + domainParts[len(domainParts)-1]
			if secondLevelTLDs[suffix] {
				// 二级域名后缀：取倒数第三部分（如 example.com.cn -> example）
				domainMain = domainParts[len(domainParts)-3]
			} else {
				// 普通域名：取倒数第二部分（如 example.com -> example）
				domainMain = domainParts[len(domainParts)-2]
			}
		} else {
			// 只有两部分：取第一部分（如 example.com -> example）
			domainMain = domainParts[0]
		}
	} else {
		// 单部分域名（如localhost）
		domainMain = domainLower
	}

	// 检查提取的主域名是否在黑名单中
	if commonWords[domainMain] {
		da.logger.WithFields(logrus.Fields{
			"domain":      domain,
			"domain_main": domainMain,
			"reason":      "Common word domain",
		}).Debug("Domain skipped")
		return 0.0
	}

	// 1. 优先检查 APK 文件名匹配（更准确）
	if apkName != "" {
		apkNameLower := strings.ToLower(apkName)
		apkNameLower = strings.TrimSuffix(apkNameLower, ".apk")

		// 使用下划线、横线、点号分割文件名
		apkParts := strings.FieldsFunc(apkNameLower, func(r rune) bool {
			return r == '_' || r == '-' || r == '.'
		})

		for _, part := range apkParts {
			// 跳过长度太短的部分（< 3）或通用词汇
			if len(part) < 3 || commonWords[part] {
				continue
			}

			// 🔧 优化：完全匹配域名主要部分
			if domainMain == part || strings.Contains(domainMain, part) || strings.Contains(part, domainMain) {
				da.logger.WithFields(logrus.Fields{
					"domain":      domain,
					"domain_main": domainMain,
					"apk_part":    part,
					"match_type":  "APK name exact match",
				}).Debug("Domain matched via APK name")
				return 25.0
			}

			// 部分匹配
			if strings.Contains(domainLower, part) {
				bestMatch = 22.5
			}
		}
	}

	// 2. 如果 APK 名匹配成功，直接返回
	if bestMatch >= 22.5 {
		return bestMatch
	}

	// 🔧 修复3：包名匹配策略优化 - 优先匹配包名最后一部分（最核心）
	// 例如: com.gngnetwork.kgoods -> 优先匹配 kgoods，其次 gngnetwork
	packageParts := strings.Split(packageName, ".")
	if len(packageParts) < 2 {
		return bestMatch
	}

	// 🔧 关键修复：按优先级排序包名部分（倒序遍历，最后的部分优先级最高）
	// 例如: com.gngnetwork.kgoods -> [kgoods, gngnetwork, com]
	priorityParts := make([]string, 0, len(packageParts))
	for i := len(packageParts) - 1; i >= 0; i-- {
		part := packageParts[i]
		// 跳过长度太短的部分和通用词汇
		if len(part) < 3 || commonWords[strings.ToLower(part)] {
			continue
		}
		priorityParts = append(priorityParts, part)
	}

	// 遍历优先级排序后的包名部分
	for idx, part := range priorityParts {
		partLower := strings.ToLower(part)

		// 🔧 核心修复：完全匹配域名主要部分（最高优先级）
		// 例如: kgoods 匹配 gngkgoods.com 的 gngkgoods
		if domainMain == partLower {
			score := 25.0 // 完全匹配，最高分
			da.logger.WithFields(logrus.Fields{
				"domain":        domain,
				"domain_main":   domainMain,
				"package_part":  part,
				"package_index": len(packageParts) - 1 - idx,
				"match_type":    "Package name exact match (main domain)",
			}).Debug("Domain matched via package name (exact)")
			return score
		}

		// 🔧 核心修复：域名主要部分包含包名关键词
		// 例如: kgoods 匹配 gngkgoods 的 kgoods 部分
		if strings.Contains(domainMain, partLower) {
			// 根据优先级给分：
			// - 包名最后一部分（idx=0）: 25分
			// - 包名倒数第二部分（idx=1）: 20分
			// - 其他部分：17.5分
			score := 25.0 - float64(idx)*5.0
			if score < 17.5 {
				score = 17.5
			}

			if score > bestMatch {
				bestMatch = score
				da.logger.WithFields(logrus.Fields{
					"domain":        domain,
					"domain_main":   domainMain,
					"package_part":  part,
					"package_index": len(packageParts) - 1 - idx,
					"score":         score,
					"match_type":    "Package name substring match (main domain)",
				}).Debug("Domain matched via package name (substring)")
			}
		}

		// 检查整个域名是否包含包名部分
		if strings.Contains(domainLower, partLower) && bestMatch < 17.5 {
			score := 17.5 - float64(idx)*2.5
			if score < 12.5 {
				score = 12.5
			}
			if score > bestMatch {
				bestMatch = score
			}
		}

		// 部分匹配（前缀或后缀）
		if strings.HasPrefix(domainMain, partLower) || strings.HasSuffix(domainMain, partLower) {
			score := 15.0 - float64(idx)*2.5
			if score < 10.0 {
				score = 10.0
			}
			if score > bestMatch {
				bestMatch = score
			}
		}
	}

	return bestMatch
}
// checkAPIFeatures 检查API特征 (0-5分) - 原3分提升到5分
func (da *DomainAnalyzer) checkAPIFeatures(urls []string) (bool, float64) {
	apiPatterns := []string{"/api/", "/v1/", "/v2/", "/v3/", "/rest/", "/graphql", "/json"}

	for _, rawURL := range urls {
		urlLower := strings.ToLower(rawURL)
		for _, pattern := range apiPatterns {
			if strings.Contains(urlLower, pattern) {
				return true, 5.0
			}
		}
	}

	return false, 0.0
}

// checkAuthFeatures 检查认证特征 (0-3分) - 原1分提升到3分
func (da *DomainAnalyzer) checkAuthFeatures(urls []string) (bool, float64) {
	authPatterns := []string{"/login", "/auth", "/oauth", "/token", "/signin", "/sso"}

	for _, rawURL := range urls {
		urlLower := strings.ToLower(rawURL)
		for _, pattern := range authPatterns {
			if strings.Contains(urlLower, pattern) {
				return true, 3.0
			}
		}
	}

	return false, 0.0
}

// checkCDN 检查CDN域名 (0或-2分)
func (da *DomainAnalyzer) checkCDN(domain string) (bool, float64) {
	cdnPatterns := []string{"cdn", "static", "img", "image", "assets", "cache", "resource"}

	domainLower := strings.ToLower(domain)
	for _, pattern := range cdnPatterns {
		if strings.Contains(domainLower, pattern) {
			return true, -2.0
		}
	}

	return false, 0.0
}

// isPublicTLD 检查是否为公共顶级域名（.org/.edu/.gov/.int/.mil）
// 🔧 新增：直接排除这些公共域名，不参与主域名竞争
// 这些TLD通常用于公共组织、教育机构、政府网站，极少作为商业应用的主域名
func (da *DomainAnalyzer) isPublicTLD(domain string) bool {
	// 公共TLD黑名单
	publicTLDs := []string{
		".org", // 非营利组织 (如 ietf.org, webrtc.org)
		".edu", // 教育机构
		".gov", // 政府网站
		".int", // 国际组织
		".mil", // 军事机构
	}

	domainLower := strings.ToLower(domain)
	for _, tld := range publicTLDs {
		if strings.HasSuffix(domainLower, tld) {
			return true
		}
	}

	return false
}

// isDomainMatchingPackage 检查域名是否与应用包名或APK文件名匹配
// 用于判断域名是否属于应用自己（而非第三方SDK）
func (da *DomainAnalyzer) isDomainMatchingPackage(domain, packageName, apkName string) bool {
	if domain == "" {
		return false
	}

	domainLower := strings.ToLower(domain)

	// 提取域名主要部分（去除TLD）
	domainParts := strings.Split(domainLower, ".")
	if len(domainParts) < 2 {
		return false
	}

	// 提取域名核心（如 taobao.com -> taobao）
	domainCore := domainParts[0]
	if len(domainParts) >= 2 {
		// 处理二级域名后缀
		secondLevelTLDs := map[string]bool{
			"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true,
			"co.uk": true, "co.jp": true, "co.kr": true,
		}
		if len(domainParts) >= 3 {
			suffix := domainParts[len(domainParts)-2] + "." + domainParts[len(domainParts)-1]
			if secondLevelTLDs[suffix] {
				domainCore = domainParts[len(domainParts)-3]
			} else {
				domainCore = domainParts[len(domainParts)-2]
			}
		} else {
			domainCore = domainParts[0]
		}
	}

	// 1. 检查包名是否包含域名核心
	// 例如：com.taobao.taobao 包含 taobao
	if packageName != "" {
		packageLower := strings.ToLower(packageName)
		packageParts := strings.Split(packageLower, ".")

		for _, part := range packageParts {
			if len(part) < 3 {
				continue
			}
			// 精确匹配或包含关系
			if part == domainCore || strings.Contains(part, domainCore) || strings.Contains(domainCore, part) {
				da.logger.WithFields(logrus.Fields{
					"domain":       domain,
					"domain_core":  domainCore,
					"package_name": packageName,
					"package_part": part,
					"match_type":   "package_contains_domain",
				}).Debug("Domain matches package name")
				return true
			}
		}
	}

	// 2. 检查APK文件名是否包含域名核心
	// 例如：com.taobao.taobao10.51.30.apk 包含 taobao
	if apkName != "" {
		apkNameLower := strings.ToLower(apkName)
		apkNameLower = strings.TrimSuffix(apkNameLower, ".apk")

		// 分割APK文件名
		apkParts := strings.FieldsFunc(apkNameLower, func(r rune) bool {
			return r == '_' || r == '-' || r == '.'
		})

		for _, part := range apkParts {
			if len(part) < 3 {
				continue
			}
			if part == domainCore || strings.Contains(part, domainCore) || strings.Contains(domainCore, part) {
				da.logger.WithFields(logrus.Fields{
					"domain":      domain,
					"domain_core": domainCore,
					"apk_name":    apkName,
					"apk_part":    part,
					"match_type":  "apk_contains_domain",
				}).Debug("Domain matches APK name")
				return true
			}
		}
	}

	return false
}

// checkSDK 检查SDK域名 (0或-10分)
func (da *DomainAnalyzer) checkSDK(ctx context.Context, domain string) (bool, float64) {
	if da.sdkManager != nil {
		isThirdParty, category, provider := da.sdkManager.IsThirdPartyDomain(ctx, domain)
		if isThirdParty {
			da.logger.WithFields(logrus.Fields{
				"domain":   domain,
				"category": category,
				"provider": provider,
			}).Debug("Domain identified as third-party SDK")
			return true, -10.0
		}
	} else {
		// Fallback: 使用简单的关键词匹配
		if da.isThirdPartyDomain(domain) {
			return true, -10.0
		}
	}

	return false, 0.0
}

// min 返回两个整数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max 返回两个整数的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// matchesPackageName 检查域名是否匹配包名
func (da *DomainAnalyzer) matchesPackageName(domain, packageName string) bool {
	// 从包名中提取关键词 (如 com.example.app -> example)
	parts := strings.Split(packageName, ".")
	if len(parts) < 2 {
		return false
	}

	// 通常包名的第二部分是公司/产品名称
	keyword := parts[1]
	if len(keyword) < 3 {
		return false
	}

	// 检查域名是否包含关键词
	return strings.Contains(strings.ToLower(domain), strings.ToLower(keyword))
}

// isThirdPartyDomain 检查是否为常见第三方域名
func (da *DomainAnalyzer) isThirdPartyDomain(domain string) bool {
	thirdPartyKeywords := []string{
		"google", "facebook", "twitter", "doubleclick",
		"googlesyndication", "googletagmanager", "googleadservices",
		"umeng", "baidu", "aliyun", "tencent", "qq.com",
		"analytics", "crashlytics", "firebase",
		"adservice", "adserver", "ads", "advertising",
	}

	domainLower := strings.ToLower(domain)
	for _, keyword := range thirdPartyKeywords {
		if strings.Contains(domainLower, keyword) {
			return true
		}
	}

	return false
}

// isIPAddress 检查字符串是否为IP地址（IPv4格式）
func (da *DomainAnalyzer) isIPAddress(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}

	// 检查每个部分是否为0-255的数字
	for _, part := range parts {
		// 检查是否全是数字
		if len(part) == 0 || len(part) > 3 {
			return false
		}

		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}

		// 转换为整数并检查范围
		var num int
		fmt.Sscanf(part, "%d", &num)
		if num < 0 || num > 255 {
			return false
		}
	}

	return true
}

// ExtractAllDomains 提取所有唯一域名 (用于 IP 查询)
func (da *DomainAnalyzer) ExtractAllDomains(urls []string) []string {
	domainSet := make(map[string]bool)

	for _, rawURL := range urls {
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		host := parsedURL.Hostname()
		if host != "" {
			domainSet[host] = true
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}

	return domains
}

// PrimaryDomainResult 主域名分析结果
type PrimaryDomainResult struct {
	PrimaryDomain string                 `json:"primary_domain"`
	Confidence    float64                `json:"confidence"`
	Candidates    []DomainCandidate      `json:"candidates"`
	Evidence      map[string]interface{} `json:"evidence"`
}

// DomainCandidate 域名候选
type DomainCandidate struct {
	Domain           string   `json:"domain"`
	Score            float64  `json:"score"`
	RequestCount     int      `json:"request_count"`
	PackageMatch     bool     `json:"package_match"`
	PackageScore     float64  `json:"package_score"`      // 包名匹配分数 (0-25)
	FrequencyScore   float64  `json:"frequency_score"`    // 频率分数 (0-5)
	PathScore        float64  `json:"path_score"`         // 路径多样性分数 (0-3)
	SubdomainScore   float64  `json:"subdomain_score"`    // 子域名分数 (0-2)
	DynamicScore     float64  `json:"dynamic_score"`      // 动态流量分数 (0-3)
	APIScore         float64  `json:"api_score"`          // API特征分数 (0-3)
	AuthScore        float64  `json:"auth_score"`         // 认证特征分数 (0-1)
	CDNPenalty       float64  `json:"cdn_penalty"`        // CDN惩罚 (0或-2)
	SDKPenalty       float64  `json:"sdk_penalty"`        // SDK惩罚 (0或-10)
	PathCount        int      `json:"path_count"`         // 路径数量
	SubdomainCount   int      `json:"subdomain_count"`    // 子域名数量
	Subdomains       []string `json:"subdomains"`         // 🔧 新增：子域名列表（完整域名）
	DynamicCount     int      `json:"dynamic_count"`      // 动态请求次数
	IsAPI            bool     `json:"is_api"`             // 是否为API域名
	HasAuth          bool     `json:"has_auth"`           // 是否有认证特征
	IsCDN            bool     `json:"is_cdn"`             // 是否为CDN域名
	IsSDK            bool     `json:"is_sdk"`             // 是否为SDK域名
}

// String 格式化输出
func (r *PrimaryDomainResult) String() string {
	if r.PrimaryDomain == "" {
		return "No primary domain identified"
	}
	return fmt.Sprintf("%s (confidence: %.2f)", r.PrimaryDomain, r.Confidence)
}
