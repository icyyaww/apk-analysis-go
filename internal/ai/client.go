package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// Client 智谱 AI 客户端
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewClient 创建 AI 客户端
func NewClient(apiKey string, logger *logrus.Logger) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: "https://open.bigmodel.cn/api/paas/v4",
		model:   "GLM-4.5-Flash", // 智谱 GLM-4.5-Flash 模型
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

// ThinkingConfig 深度思考配置
type ThinkingConfig struct {
	Type string `json:"type"` // "enabled" 或 "disabled"
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model    string          `json:"model"`
	Messages []Message       `json:"messages"`
	Thinking *ThinkingConfig `json:"thinking,omitempty"` // 禁用深度思考以加快响应
}

// Message 消息
type Message struct {
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

// ContentPart 内容部分
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片 URL
type ImageURL struct {
	URL string `json:"url"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice 选择
type Choice struct {
	Index   int            `json:"index"`
	Message ResponseMessage `json:"message"`
}

// ResponseMessage 响应消息
type ResponseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage 使用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// AnalyzeScreenshot 分析截图
func (c *Client) AnalyzeScreenshot(ctx context.Context, screenshotPath string) (*UIAnalysisResult, error) {
	c.logger.WithField("screenshot", screenshotPath).Info("Analyzing screenshot with GLM-4V")

	// 1. 读取并编码图片
	imageData, err := os.ReadFile(screenshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read screenshot: %w", err)
	}

	base64Image := base64.StdEncoding.EncodeToString(imageData)
	imageURL := fmt.Sprintf("data:image/png;base64,%s", base64Image)

	// 2. 构建提示词
	prompt := `你是一个 Android UI 分析专家。请分析这个 Android 应用界面截图，识别所有可交互的元素。

请以 JSON 格式返回分析结果，包含以下字段：
{
  "buttons": ["按钮1文本", "按钮2文本", ...],
  "input_fields": ["输入框1提示", "输入框2提示", ...],
  "clickable_items": [
    {"text": "元素文本", "type": "button/link/checkbox", "importance": "high/medium/low"}
  ],
  "suggested_actions": [
    {"action": "click/input/swipe", "target": "目标元素", "reason": "操作原因", "priority": 1-10}
  ]
}

重点关注：
1. 登录/注册相关按钮
2. 权限请求弹窗的允许/拒绝按钮
3. 隐私政策/用户协议的同意按钮
4. 输入框（手机号、验证码、密码等）
5. 底部导航栏
6. 重要功能入口

请只返回 JSON，不要添加其他说明文字。`

	// 3. 构建请求
	reqBody := ChatRequest{
		Model:    c.model,
		Thinking: &ThinkingConfig{Type: "disabled"}, // 禁用深度思考，加快响应速度
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentPart{
					{
						Type:     "image_url",
						ImageURL: &ImageURL{URL: imageURL},
					},
					{
						Type: "text",
						Text: prompt,
					},
				},
			},
		},
	}

	// 4. 发送请求
	resp, err := c.sendChatRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to send chat request: %w", err)
	}

	// 5. 解析响应
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := resp.Choices[0].Message.Content

	// 6. 解析 JSON
	var result UIAnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 如果直接解析失败，尝试提取 JSON 部分
		content = extractJSON(content)
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			c.logger.WithError(err).WithField("content", content).Warn("Failed to parse AI response")
			return nil, fmt.Errorf("failed to parse AI response: %w", err)
		}
	}

	c.logger.WithFields(logrus.Fields{
		"buttons":           len(result.Buttons),
		"input_fields":      len(result.InputFields),
		"suggested_actions": len(result.SuggestedActions),
	}).Info("Screenshot analyzed successfully")

	return &result, nil
}

// sendChatRequest 发送聊天请求
func (c *Client) sendChatRequest(ctx context.Context, reqBody ChatRequest) (*ChatResponse, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// extractJSON 从文本中提取 JSON
func extractJSON(text string) string {
	// 简单实现：查找第一个 { 和最后一个 }
	start := -1
	end := -1

	for i, ch := range text {
		if ch == '{' && start == -1 {
			start = i
		}
		if ch == '}' {
			end = i
		}
	}

	if start >= 0 && end > start {
		return text[start : end+1]
	}

	return text
}

// UIAnalysisResult UI 分析结果
type UIAnalysisResult struct {
	Buttons          []string          `json:"buttons"`
	InputFields      []string          `json:"input_fields"`
	ClickableItems   []ClickableItem   `json:"clickable_items"`
	SuggestedActions []SuggestedAction `json:"suggested_actions"`
}

// ClickableItem 可点击元素
type ClickableItem struct {
	Text       string `json:"text"`
	Type       string `json:"type"`
	Importance string `json:"importance"`
}

// SuggestedAction 建议操作
type SuggestedAction struct {
	Action   string `json:"action"`
	Target   string `json:"target"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

// AnalyzeText 分析纯文本提示词
func (c *Client) AnalyzeText(ctx context.Context, prompt string) (string, error) {
	c.logger.WithField("model", c.model).Debug("Analyzing text with GLM-4-Flash")

	// 构建请求
	reqBody := ChatRequest{
		Model:    c.model,
		Thinking: &ThinkingConfig{Type: "disabled"}, // 禁用深度思考，加快响应速度
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentPart{
					{
						Type: "text",
						Text: prompt,
					},
				},
			},
		},
	}

	// 发送请求
	resp, err := c.sendChatRequest(ctx, reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to send chat request: %w", err)
	}

	// 解析响应
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	content := resp.Choices[0].Message.Content

	c.logger.WithField("response_length", len(content)).Debug("Text analysis completed")

	return content, nil
}
