package domainanalysis

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/mozillazg/go-pinyin"
	"github.com/sirupsen/logrus"
)

// URLClassification URL 分类结果
type URLClassification struct {
	URL         string  `json:"url"`
	Domain      string  `json:"domain"`
	IsAppServer *bool   `json:"is_app_server"` // true=应用服务器, false=第三方, nil=未知
	Confidence  float64 `json:"confidence"`    // 0.0-1.0
	Reason      string  `json:"reason"`        // 匹配原因
	Category    string  `json:"category"`      // 规则类别
	MatchedBy   string  `json:"matched_by"`    // 匹配到的具体值
}

// AppInfo 应用信息（用于 URL 分类）
type AppInfo struct {
	AppName     string `json:"app_name"`     // 应用名称（中文）
	PackageName string `json:"package_name"` // 包名
	Developer   string `json:"developer"`    // 开发者名称（中文）
}

// URLClassifier URL 分类器
type URLClassifier struct {
	logger      *logrus.Logger
	sdkManager  *SDKManager
	pinyinArgs  pinyin.Args
	apiKeywords []string
}

// NewURLClassifier 创建 URL 分类器
func NewURLClassifier(logger *logrus.Logger, sdkManager *SDKManager) *URLClassifier {
	return &URLClassifier{
		logger:     logger,
		sdkManager: sdkManager,
		pinyinArgs: pinyin.NewArgs(),
		apiKeywords: []string{
			"api", "gateway", "interface", "rest", "graphql", "rpc",
		},
	}
}

// ClassifyURLs 对所有 URL 进行分类
func (c *URLClassifier) ClassifyURLs(ctx context.Context, urls []string, appInfo *AppInfo) []URLClassification {
	results := make([]URLClassification, 0, len(urls))

	// 去重
	seen := make(map[string]bool)
	for _, rawURL := range urls {
		if seen[rawURL] {
			continue
		}
		seen[rawURL] = true

		result := c.ClassifyURL(ctx, rawURL, appInfo)
		results = append(results, result)
	}

	return results
}

// ClassifyURL 对单个 URL 进行分类
func (c *URLClassifier) ClassifyURL(ctx context.Context, rawURL string, appInfo *AppInfo) URLClassification {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return URLClassification{
			URL:        rawURL,
			Confidence: 0,
			Reason:     "URL 解析失败",
			Category:   "parse_error",
		}
	}

	domain := strings.ToLower(parsedURL.Hostname())
	path := strings.ToLower(parsedURL.Path)

	result := URLClassification{
		URL:    rawURL,
		Domain: domain,
	}

	// ========== 规则1：包名匹配（强规则，置信度 1.0）==========
	if appInfo.PackageName != "" && strings.Contains(domain, strings.ToLower(appInfo.PackageName)) {
		isApp := true
		result.IsAppServer = &isApp
		result.Confidence = 1.0
		result.Reason = "域名包含包名: " + appInfo.PackageName
		result.Category = "package_match"
		result.MatchedBy = appInfo.PackageName
		return result
	}

	// ========== 规则2：开发者拼音匹配（强规则，置信度 1.0）==========
	if appInfo.Developer != "" {
		developerPinyins := c.getPinyinVariations(appInfo.Developer)
		for _, py := range developerPinyins {
			if len(py) >= 2 && strings.Contains(domain, py) {
				isApp := true
				result.IsAppServer = &isApp
				result.Confidence = 1.0
				result.Reason = "域名包含开发者拼音: " + py
				result.Category = "developer_match"
				result.MatchedBy = py
				return result
			}
		}
	}

	// ========== 规则3：应用名称拼音匹配（强规则，置信度 1.0）==========
	if appInfo.AppName != "" {
		appPinyins := c.getPinyinVariations(appInfo.AppName)
		for _, py := range appPinyins {
			if len(py) >= 2 && strings.Contains(domain, py) {
				isApp := true
				result.IsAppServer = &isApp
				result.Confidence = 1.0
				result.Reason = "域名包含应用名称拼音: " + py
				result.Category = "app_name_match"
				result.MatchedBy = py
				return result
			}
		}
	}

	// ========== 规则4：第三方服务特征匹配（排除规则，置信度 0.9）==========
	if c.sdkManager != nil {
		isThirdParty, category, provider := c.sdkManager.IsThirdPartyDomain(ctx, domain)
		if isThirdParty {
			isApp := false
			result.IsAppServer = &isApp
			result.Confidence = 0.9
			result.Reason = "第三方服务: " + provider
			result.Category = "third_party"
			result.MatchedBy = category
			return result
		}
	}

	// ========== 规则5：IP地址+端口匹配（弱规则，置信度 0.8）==========
	if c.isIPWithPort(domain) {
		isApp := true
		result.IsAppServer = &isApp
		result.Confidence = 0.8
		result.Reason = "使用IP地址和自定义端口"
		result.Category = "ip_port"
		result.MatchedBy = domain
		return result
	}

	// ========== 规则6：API关键词匹配（弱规则，置信度 0.7）==========
	for _, keyword := range c.apiKeywords {
		if strings.Contains(domain, keyword) || strings.Contains(path, "/"+keyword+"/") || strings.Contains(path, "/"+keyword) {
			isApp := true
			result.IsAppServer = &isApp
			result.Confidence = 0.7
			result.Reason = "URL包含API相关关键词: " + keyword
			result.Category = "api_keyword"
			result.MatchedBy = keyword
			return result
		}
	}

	// ========== 默认：无法确定（置信度 0.0）==========
	result.IsAppServer = nil
	result.Confidence = 0.0
	result.Reason = "无法通过现有规则判断"
	result.Category = "unknown"
	result.MatchedBy = ""

	return result
}

// getPinyinVariations 获取文本的拼音变体
func (c *URLClassifier) getPinyinVariations(text string) []string {
	if text == "" {
		return nil
	}

	// 过滤非中文字符，只保留中文
	var chineseChars []rune
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			chineseChars = append(chineseChars, r)
		}
	}

	if len(chineseChars) == 0 {
		// 没有中文，返回原文（转小写）
		return []string{strings.ToLower(text)}
	}

	chineseText := string(chineseChars)

	// 获取拼音
	pinyinResult := pinyin.Pinyin(chineseText, c.pinyinArgs)

	variations := make(map[string]bool)

	// 1. 完整拼音拼接（如 "北京" -> "beijing"）
	var fullPinyin strings.Builder
	for _, py := range pinyinResult {
		if len(py) > 0 {
			fullPinyin.WriteString(py[0])
		}
	}
	if fullPinyin.Len() > 0 {
		variations[fullPinyin.String()] = true
	}

	// 2. 拼音首字母（如 "北京" -> "bj"）
	var initials strings.Builder
	for _, py := range pinyinResult {
		if len(py) > 0 && len(py[0]) > 0 {
			initials.WriteByte(py[0][0])
		}
	}
	if initials.Len() > 0 {
		variations[initials.String()] = true
	}

	// 转换为切片
	result := make([]string, 0, len(variations))
	for v := range variations {
		result = append(result, v)
	}

	return result
}

// isIPWithPort 判断是否为 IP 地址（带或不带端口）
func (c *URLClassifier) isIPWithPort(domain string) bool {
	// 正则：匹配 IP 地址格式（可选端口）
	ipPortPattern := regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?$`)
	return ipPortPattern.MatchString(domain)
}

// ClassificationSummary URL 分类汇总
type ClassificationSummary struct {
	AppServerURLs   []URLClassification `json:"app_server_urls"`   // 应用服务器 URL
	ThirdPartyURLs  []URLClassification `json:"third_party_urls"`  // 第三方服务 URL
	UnknownURLs     []URLClassification `json:"unknown_urls"`      // 未分类 URL
	TotalCount      int                 `json:"total_count"`       // 总数
	AppServerCount  int                 `json:"app_server_count"`  // 应用服务器数量
	ThirdPartyCount int                 `json:"third_party_count"` // 第三方数量
	UnknownCount    int                 `json:"unknown_count"`     // 未分类数量
}

// SummarizeClassifications 汇总分类结果
func SummarizeClassifications(classifications []URLClassification) *ClassificationSummary {
	summary := &ClassificationSummary{
		AppServerURLs:  make([]URLClassification, 0),
		ThirdPartyURLs: make([]URLClassification, 0),
		UnknownURLs:    make([]URLClassification, 0),
		TotalCount:     len(classifications),
	}

	for _, c := range classifications {
		if c.IsAppServer == nil {
			summary.UnknownURLs = append(summary.UnknownURLs, c)
			summary.UnknownCount++
		} else if *c.IsAppServer {
			summary.AppServerURLs = append(summary.AppServerURLs, c)
			summary.AppServerCount++
		} else {
			summary.ThirdPartyURLs = append(summary.ThirdPartyURLs, c)
			summary.ThirdPartyCount++
		}
	}

	return summary
}
