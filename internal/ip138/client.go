package ip138

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// IPLocationInfo IP归属地信息
type IPLocationInfo struct {
	IP         string `json:"ip"`
	Country    string `json:"country"`
	Province   string `json:"province"`
	City       string `json:"city"`
	District   string `json:"district"`
	ISP        string `json:"isp"`
	PostalCode string `json:"postal_code,omitempty"`
	AreaCode   string `json:"area_code,omitempty"`
	Source     string `json:"source"`
}

// IP138Response IP138 API响应结构
type IP138Response struct {
	Ret  string   `json:"ret"`
	Data []string `json:"data"`
}

// Client IP138客户端
type Client struct {
	apiURL     string
	token      string
	httpClient *http.Client
	logger     *logrus.Logger
	cache      map[string]*IPLocationInfo
	cacheMu    sync.RWMutex
}

// NewClient 创建IP138客户端
func NewClient(apiURL, token string, logger *logrus.Logger) *Client {
	if apiURL == "" {
		apiURL = "https://api.ip138.com/ip/?datatype=jsonp&ip="
	}
	if token == "" {
		token = "912d1e809877cad833f2a45d919622c7" // 默认token
	}

	return &Client{
		apiURL: apiURL,
		token:  token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
		cache:  make(map[string]*IPLocationInfo),
	}
}

// GetIPLocation 查询IP归属地（带缓存）
func (c *Client) GetIPLocation(ip string) (*IPLocationInfo, error) {
	// 1. 先查缓存
	c.cacheMu.RLock()
	if cached, ok := c.cache[ip]; ok {
		c.cacheMu.RUnlock()
		c.logger.WithField("ip", ip).Debug("Using cached IP location")
		cached.Source = "cache"
		return cached, nil
	}
	c.cacheMu.RUnlock()

	// 2. 调用API查询
	info, err := c.callAPI(ip)
	if err != nil {
		return nil, err
	}

	// 3. 保存到缓存
	c.cacheMu.Lock()
	c.cache[ip] = info
	c.cacheMu.Unlock()

	return info, nil
}

// callAPI 调用IP138 API
func (c *Client) callAPI(ip string) (*IPLocationInfo, error) {
	url := fmt.Sprintf("%s%s", c.apiURL, ip)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("token", c.token)
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
			body, _ := io.ReadAll(resp.Body)
			c.logger.Warnf("IP138 API returned status %d: %s", resp.StatusCode, string(body))
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response failed: %w", err)
		}

		respText := string(body)

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
		info := &IPLocationInfo{
			IP:       ip,
			Country:  getField(apiResp.Data, 0),
			Province: getField(apiResp.Data, 1),
			City:     getField(apiResp.Data, 2),
			District: getField(apiResp.Data, 3),
			ISP:      getField(apiResp.Data, 4),
			Source:   "ip138",
		}

		if len(apiResp.Data) > 5 {
			info.PostalCode = getField(apiResp.Data, 5)
		}
		if len(apiResp.Data) > 6 {
			info.AreaCode = getField(apiResp.Data, 6)
		}

		c.logger.WithFields(logrus.Fields{
			"ip":       ip,
			"province": info.Province,
			"city":     info.City,
			"isp":      info.ISP,
		}).Info("IP location queried successfully")

		return info, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// BatchGetIPLocations 批量查询IP归属地
func (c *Client) BatchGetIPLocations(ips []string) map[string]*IPLocationInfo {
	results := make(map[string]*IPLocationInfo)
	var mu sync.Mutex

	// 使用有限并发避免触发频率限制
	maxConcurrent := 3
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}        // 获取信号量
			defer func() { <-sem }() // 释放信号量

			// 批量查询时增加延迟避免频率限制
			time.Sleep(500 * time.Millisecond)

			info, err := c.GetIPLocation(ip)
			if err != nil {
				c.logger.WithError(err).Warnf("Failed to query IP location: %s", ip)
				return
			}

			mu.Lock()
			results[ip] = info
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	return results
}

// getField 安全获取数组字段
func getField(data []string, index int) string {
	if index < len(data) {
		return data[index]
	}
	return ""
}

// min 返回两个整数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
