package domainanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// IPLocationClient IP 归属地查询客户端
type IPLocationClient struct {
	httpClient  *http.Client
	logger      *logrus.Logger
	ip138URL    string
	ip138Token  string
	vvhanURL    string
	cache       map[string]*IPLocationResult // 简单内存缓存
}

// NewIPLocationClient 创建 IP 归属地查询客户端
func NewIPLocationClient(logger *logrus.Logger) *IPLocationClient {
	return &IPLocationClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:     logger,
		ip138URL:   "https://api.ip138.com/ip/?datatype=jsonp&ip=",
		ip138Token: "912d1e809877cad833f2a45d919622c7",
		vvhanURL:   "https://api.vvhan.com/api/getIpInfo", // 免费 IP 查询 API（备用）
		cache:      make(map[string]*IPLocationResult),
	}
}

// QueryDomainLocation 查询域名的 IP 归属地
func (c *IPLocationClient) QueryDomainLocation(ctx context.Context, domain string) *IPLocationResult {
	c.logger.WithField("domain", domain).Debug("Querying domain IP location")

	// 1. DNS 解析获取 IP
	ip, err := c.resolveDomain(domain)
	if err != nil {
		c.logger.WithError(err).Warn("DNS resolution failed")
		return &IPLocationResult{
			Domain: domain,
			Error:  err.Error(),
		}
	}

	// 2. 查询 IP 归属地
	return c.QueryIPLocation(ctx, ip, domain)
}

// QueryIPLocation 查询 IP 归属地（优先IP138，失败则使用vvhan）
func (c *IPLocationClient) QueryIPLocation(ctx context.Context, ip, domain string) *IPLocationResult {
	// 检查缓存
	if cached, exists := c.cache[ip]; exists {
		c.logger.WithField("ip", ip).Debug("Using cached IP location")
		result := *cached // 复制
		result.Domain = domain
		return &result
	}

	c.logger.WithField("ip", ip).Info("Querying IP location with IP138 (primary)")

	// 优先尝试 IP138 API
	result, err := c.queryIP138API(ctx, ip)
	if err != nil {
		c.logger.WithError(err).Warn("IP138 query failed, falling back to vvhan API")

		// IP138 失败，尝试 vvhan API
		result, err = c.queryVvhanAPI(ctx, ip)
		if err != nil {
			c.logger.WithError(err).Error("Both IP138 and vvhan queries failed")
			return &IPLocationResult{
				Domain: domain,
				IP:     ip,
				Error:  "All IP location APIs failed: " + err.Error(),
			}
		}
	}

	result.Domain = domain

	// 缓存结果
	c.cache[ip] = result

	return result
}

// BatchQueryDomains 批量查询域名归属地
func (c *IPLocationClient) BatchQueryDomains(ctx context.Context, domains []string) map[string]*IPLocationResult {
	results := make(map[string]*IPLocationResult)

	for _, domain := range domains {
		// 限流: 每次查询间隔 500ms
		time.Sleep(500 * time.Millisecond)

		result := c.QueryDomainLocation(ctx, domain)
		results[domain] = result
	}

	return results
}

// BatchQueryIPs 批量查询IP归属地（直接查询IP，不需要DNS解析）
func (c *IPLocationClient) BatchQueryIPs(ctx context.Context, ips []string) map[string]*IPLocationResult {
	results := make(map[string]*IPLocationResult)

	for _, ip := range ips {
		// 限流: 每次查询间隔 500ms
		time.Sleep(500 * time.Millisecond)

		// 直接查询IP归属地，domain参数留空（因为是直连IP）
		result := c.QueryIPLocation(ctx, ip, "")
		results[ip] = result
	}

	c.logger.WithFields(logrus.Fields{
		"total_ips":      len(ips),
		"success_count":  len(results),
	}).Info("Batch query IPs completed")

	return results
}

// resolveDomain DNS 解析域名
func (c *IPLocationClient) resolveDomain(domain string) (string, error) {
	// 清理域名
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	if idx := strings.Index(domain, ":"); idx > 0 {
		domain = domain[:idx]
	}
	if idx := strings.Index(domain, "/"); idx > 0 {
		domain = domain[:idx]
	}

	// DNS 查询
	ips, err := net.LookupIP(domain)
	if err != nil {
		return "", fmt.Errorf("DNS lookup failed: %w", err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for domain")
	}

	// 返回第一个 IPv4 地址
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}

	// 如果没有 IPv4,返回第一个 IP
	return ips[0].String(), nil
}

// queryIP138API 调用 IP138 API 查询 IP 归属地
func (c *IPLocationClient) queryIP138API(ctx context.Context, ip string) (*IPLocationResult, error) {
	// 构建请求 URL
	reqURL := fmt.Sprintf("%s%s", c.ip138URL, url.QueryEscape(ip))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("token", c.ip138Token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; APK-Analysis/1.0)")

	// 带重试机制
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.WithError(err).Warnf("IP138 API request failed (attempt %d/%d)", attempt, maxRetries)
			if attempt < maxRetries {
				time.Sleep(time.Second)
				continue
			}
			return nil, fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			c.logger.Warnf("IP138 API returned status %d: %s", resp.StatusCode, string(bodyBytes))
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response failed: %w", err)
		}

		respText := string(bodyBytes)

		// 检测频率限制
		if strings.Contains(respText, "访问过于频繁") {
			c.logger.Warnf("IP138 rate limit hit, retrying in 3s (attempt %d/%d)", attempt, maxRetries)
			time.Sleep(3 * time.Second)
			continue
		}

		// 解析JSONP格式
		if strings.HasPrefix(respText, "(") && strings.HasSuffix(respText, ")") {
			respText = respText[1 : len(respText)-1]
		}

		// 解析JSON
		var apiResp IP138Response
		if err := json.Unmarshal([]byte(respText), &apiResp); err != nil {
			c.logger.WithError(err).Warnf("Failed to parse IP138 response: %s", respText[:min(200, len(respText))])
			return nil, fmt.Errorf("parse response failed: %w", err)
		}

		// 检查返回状态
		if apiResp.Ret != "ok" {
			return nil, fmt.Errorf("API returned error: ret=%s", apiResp.Ret)
		}

		// 检查数据格式
		if len(apiResp.Data) < 5 {
			return nil, fmt.Errorf("invalid data format: expected at least 5 fields, got %d", len(apiResp.Data))
		}

		// 构建结果
		result := &IPLocationResult{
			IP:       ip,
			Country:  getField(apiResp.Data, 0),
			Province: getField(apiResp.Data, 1),
			City:     getField(apiResp.Data, 2),
			ISP:      getField(apiResp.Data, 4),
			Source:   "ip138",
			Info: map[string]string{
				"country":  getField(apiResp.Data, 0),
				"province": getField(apiResp.Data, 1),
				"city":     getField(apiResp.Data, 2),
				"district": getField(apiResp.Data, 3),
				"isp":      getField(apiResp.Data, 4),
			},
		}

		c.logger.WithFields(logrus.Fields{
			"ip":       ip,
			"province": result.Province,
			"city":     result.City,
			"isp":      result.ISP,
		}).Info("IP138 location queried successfully")

		return result, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// queryVvhanAPI 调用 vvhan API 查询 IP 归属地（备用）
func (c *IPLocationClient) queryVvhanAPI(ctx context.Context, ip string) (*IPLocationResult, error) {
	// 构建请求 URL
	reqURL := fmt.Sprintf("%s?ip=%s", c.vvhanURL, url.QueryEscape(ip))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; APK-Analysis/1.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// 读取响应
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析 JSON
	var apiResp IPLocationAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// 处理响应
	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	// 构建结果
	result := &IPLocationResult{
		IP:       apiResp.Info.IP,
		Country:  apiResp.Info.Country,
		Province: apiResp.Info.Province,
		City:     apiResp.Info.City,
		ISP:      apiResp.Info.ISP,
		Source:   "vvhan",
		Info: map[string]string{
			"ip":       apiResp.Info.IP,
			"country":  apiResp.Info.Country,
			"province": apiResp.Info.Province,
			"city":     apiResp.Info.City,
			"isp":      apiResp.Info.ISP,
		},
	}

	return result, nil
}

// IPLocationResult IP 归属地查询结果
type IPLocationResult struct {
	Domain   string            `json:"domain,omitempty"`
	IP       string            `json:"ip"`
	Country  string            `json:"country,omitempty"`
	Province string            `json:"province,omitempty"`
	City     string            `json:"city,omitempty"`
	ISP      string            `json:"isp,omitempty"`
	Source   string            `json:"source,omitempty"`
	Info     map[string]string `json:"info,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// IP138Response IP138 API 响应结构
type IP138Response struct {
	Ret  string   `json:"ret"`
	Data []string `json:"data"`
}

// IPLocationAPIResponse API 响应结构 (vvhan API)
type IPLocationAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Info    struct {
		IP       string `json:"ip"`
		Country  string `json:"country"`
		Province string `json:"province"`
		City     string `json:"city"`
		ISP      string `json:"isp"`
	} `json:"info"`
}

// String 格式化输出
func (r *IPLocationResult) String() string {
	if r.Error != "" {
		return fmt.Sprintf("%s: Error - %s", r.Domain, r.Error)
	}

	location := ""
	if r.Province != "" {
		location = r.Province
	}
	if r.City != "" {
		if location != "" {
			location += " " + r.City
		} else {
			location = r.City
		}
	}
	if r.ISP != "" {
		if location != "" {
			location += " (" + r.ISP + ")"
		} else {
			location = r.ISP
		}
	}

	if location == "" {
		location = "Unknown"
	}

	return fmt.Sprintf("%s [%s]: %s", r.Domain, r.IP, location)
}

// getField 安全获取数组字段
func getField(data []string, index int) string {
	if index < len(data) {
		return data[index]
	}
	return ""
}
