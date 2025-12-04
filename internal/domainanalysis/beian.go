package domainanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BeianChecker 备案查询器 (使用站长之家API)
type BeianChecker struct {
	httpClient *http.Client
	logger     *logrus.Logger
	apiURL     string
	apiKey     string
	apiVersion string
	enabled    bool
}

// BeianCheckerConfig 备案查询器配置
type BeianCheckerConfig struct {
	Enabled    bool
	APIKey     string
	APIURL     string
	APIVersion string
	Timeout    int // seconds
}

// NewBeianChecker 创建备案查询器
func NewBeianChecker(logger *logrus.Logger) *BeianChecker {
	return NewBeianCheckerWithConfig(logger, &BeianCheckerConfig{
		Enabled:    false, // 默认禁用
		APIURL:     "http://openapiu67.chinaz.net/v1/1001/icpappunit",
		APIVersion: "1.0",
		Timeout:    70, // 站长之家API建议不低于60秒
	})
}

// NewBeianCheckerWithConfig 使用配置创建备案查询器
func NewBeianCheckerWithConfig(logger *logrus.Logger, config *BeianCheckerConfig) *BeianChecker {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 70
	}

	return &BeianChecker{
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		logger:     logger,
		apiURL:     config.APIURL,
		apiKey:     config.APIKey,
		apiVersion: config.APIVersion,
		enabled:    config.Enabled,
	}
}

// CheckBeianByAppName 通过应用名称查询备案信息 (站长之家API)
func (bc *BeianChecker) CheckBeianByAppName(ctx context.Context, appName string) *BeianResult {
	bc.logger.WithField("app_name", appName).Info("Checking app beian by name")

	// 检查是否启用
	if !bc.enabled {
		bc.logger.Debug("Beian check disabled in configuration")
		return &BeianResult{
			Domain:    appName,
			Status:    BeianStatusDisabled,
			CheckedAt: time.Now().Format(time.RFC3339),
			API:       "chinaznet",
			Info: map[string]string{
				"message": "备案查询功能未启用",
			},
		}
	}

	// 检查API Key
	if bc.apiKey == "" {
		bc.logger.Warn("Beian API key not configured")
		return &BeianResult{
			Domain:    appName,
			Status:    BeianStatusError,
			CheckedAt: time.Now().Format(time.RFC3339),
			API:       "chinaznet",
			Error:     "站长之家API Key未配置",
		}
	}

	// 清理应用名称
	cleanAppName := strings.TrimSpace(appName)
	if cleanAppName == "" {
		return &BeianResult{
			Domain:    appName,
			Status:    BeianStatusError,
			CheckedAt: time.Now().Format(time.RFC3339),
			API:       "chinaznet",
			Error:     "应用名称为空",
		}
	}

	// 调用站长之家API
	result, err := bc.queryChinazNetAPI(ctx, cleanAppName)
	if err != nil {
		bc.logger.WithError(err).Warn("Beian query by app name failed")
		return &BeianResult{
			Domain:    cleanAppName,
			Status:    BeianStatusError,
			CheckedAt: time.Now().Format(time.RFC3339),
			API:       "chinaznet",
			Error:     err.Error(),
		}
	}

	return result
}

// queryChinazNetAPI 调用站长之家API查询备案信息
//
// API文档:
// URL: http://openapiu67.chinaz.net/v1/1001/icpappunit
// 参数: keyword={appName}&page=1&APIKey={apikey}&ChinazVer={version}
// 超时: 70秒 (建议不低于60秒)
//
// 响应格式:
//
//	{
//	    "StateCode": 1,
//	    "Reason": "成功",
//	    "Result": {
//	        "List": [
//	            {
//	                "ServiceLicence": "京ICP证030173号-209A",  # 备案号
//	                "UnitName": "北京百度网讯科技有限公司"      # 主办单位
//	            }
//	        ]
//	    }
//	}
//
// 错误响应:
//
//	{
//	    "StateCode": -1,
//	    "Reason": "系统异常"
//	}
func (bc *BeianChecker) queryChinazNetAPI(ctx context.Context, appName string) (*BeianResult, error) {
	// 构建请求URL
	params := url.Values{}
	params.Set("keyword", appName)
	params.Set("page", "1")
	params.Set("APIKey", bc.apiKey)
	params.Set("ChinazVer", bc.apiVersion)

	reqURL := fmt.Sprintf("%s?%s", bc.apiURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; APK-Analysis/1.0)")

	bc.logger.WithFields(logrus.Fields{
		"app_name": appName,
		"url":      bc.apiURL,
	}).Info("Sending beian query request to ChinazNet API")

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		bc.logger.WithError(err).Error("Beian API request failed")
		return nil, fmt.Errorf("request failed: %w", err)
	}
	bc.logger.WithField("status_code", resp.StatusCode).Info("Beian API response received")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// 读取响应
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析JSON
	var apiResp ChinazNetAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// 处理响应
	result := &BeianResult{
		Domain:    appName,
		CheckedAt: time.Now().Format(time.RFC3339),
		API:       "chinaznet",
	}

	// 检查系统错误
	if apiResp.StateCode == -1 {
		result.Status = BeianStatusError
		result.Error = fmt.Sprintf("系统异常: %s", apiResp.Reason)
		result.Info = map[string]string{
			"reason":     apiResp.Reason,
			"state_code": fmt.Sprintf("%d", apiResp.StateCode),
		}
		return result, nil
	}

	// 检查成功状态
	if apiResp.StateCode != 1 {
		result.Status = BeianStatusError
		result.Error = fmt.Sprintf("查询失败: %s", apiResp.Reason)
		result.Info = map[string]string{
			"reason":     apiResp.Reason,
			"state_code": fmt.Sprintf("%d", apiResp.StateCode),
		}
		return result, nil
	}

	// 解析Result数据
	if len(apiResp.Result.List) > 0 {
		// 取第一条记录
		firstItem := apiResp.Result.List[0]
		serviceLicence := firstItem.ServiceLicence // 备案号
		unitName := firstItem.UnitName             // 主办单位

		if serviceLicence != "" {
			result.Status = BeianStatusRegistered
			result.BeianNumber = serviceLicence
			result.CompanyName = unitName
			result.Info = map[string]string{
				"service_licence": serviceLicence,
				"unit_name":       unitName,
				"reason":          apiResp.Reason,
			}

			bc.logger.WithFields(logrus.Fields{
				"app_name":      appName,
				"beian_number":  serviceLicence,
				"company_name":  unitName,
			}).Info("Beian found via ChinazNet API")
		} else {
			result.Status = BeianStatusNotRegistered
			result.Info = map[string]string{
				"message": "未查询到备案信息",
				"reason":  apiResp.Reason,
			}
		}
	} else {
		// List为空,表示未备案
		result.Status = BeianStatusNotRegistered
		result.Info = map[string]string{
			"message": "未查询到备案信息",
			"reason":  apiResp.Reason,
		}

		bc.logger.WithField("app_name", appName).Info("No beian found via ChinazNet API")
	}

	return result, nil
}

// BeianResult 备案查询结果
type BeianResult struct {
	Domain      string            `json:"domain"`                // 查询关键词(应用名称)
	Status      BeianStatus       `json:"status"`                // 查询状态
	BeianNumber string            `json:"beian_number,omitempty"` // 备案号
	CompanyName string            `json:"company_name,omitempty"` // 主办单位
	CompanyType string            `json:"company_type,omitempty"` // 公司类型(暂不支持)
	UpdatedAt   string            `json:"updated_at,omitempty"`   // 备案更新时间(暂不支持)
	CheckedAt   string            `json:"checked_at"`             // 查询时间
	API         string            `json:"api,omitempty"`          // API来源
	Info        map[string]string `json:"info,omitempty"`         // 详细信息
	Error       string            `json:"error,omitempty"`        // 错误信息
}

// BeianStatus 备案状态
type BeianStatus string

const (
	BeianStatusRegistered    BeianStatus = "registered"     // 已备案
	BeianStatusNotRegistered BeianStatus = "not_registered" // 未备案
	BeianStatusError         BeianStatus = "error"          // 查询失败
	BeianStatusDisabled      BeianStatus = "disabled"       // 功能未启用
)

// ChinazNetAPIResponse 站长之家API响应结构
type ChinazNetAPIResponse struct {
	StateCode int    `json:"StateCode"` // 1=成功, -1=系统异常
	Reason    string `json:"Reason"`    // 返回说明
	Result    struct {
		List []struct {
			ServiceLicence string `json:"ServiceLicence"` // 备案号 (例: 京ICP证030173号-209A)
			UnitName       string `json:"UnitName"`       // 主办单位 (例: 北京百度网讯科技有限公司)
		} `json:"List"`
	} `json:"Result"`
}

// String 格式化输出
func (r *BeianResult) String() string {
	switch r.Status {
	case BeianStatusRegistered:
		return fmt.Sprintf("%s: %s (%s)", r.Domain, r.BeianNumber, r.CompanyName)
	case BeianStatusNotRegistered:
		return fmt.Sprintf("%s: Not registered", r.Domain)
	case BeianStatusError:
		return fmt.Sprintf("%s: Error - %s", r.Domain, r.Error)
	case BeianStatusDisabled:
		return fmt.Sprintf("%s: Disabled", r.Domain)
	default:
		return fmt.Sprintf("%s: Unknown", r.Domain)
	}
}
