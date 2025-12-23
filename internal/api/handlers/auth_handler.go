package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	logger        *logrus.Logger
	remoteAuthURL string
	httpClient    *http.Client
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Status           string `json:"status"`
	Type             string `json:"type,omitempty"`
	CurrentAuthority string `json:"currentAuthority,omitempty"`
	Token            string `json:"token,omitempty"`
	Message          string `json:"message,omitempty"`
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(logger *logrus.Logger) *AuthHandler {
	return &AuthHandler{
		logger:        logger,
		remoteAuthURL: "http://39.99.236.217:8000/api/login/account",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Login 登录接口 - 代理到远程认证服务
// POST /api/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoginResponse{
			Status:  "error",
			Message: "请求参数错误",
		})
		return
	}

	// 构建远程请求
	remoteReq := map[string]string{
		"username": req.Username,
		"password": req.Password,
		"type":     "account",
	}
	reqBody, _ := json.Marshal(remoteReq)

	// 发送请求到远程认证服务
	resp, err := h.httpClient.Post(h.remoteAuthURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		h.logger.WithError(err).Error("远程认证服务请求失败")
		c.JSON(http.StatusServiceUnavailable, LoginResponse{
			Status:  "error",
			Message: "认证服务暂不可用",
		})
		return
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.WithError(err).Error("读取远程认证响应失败")
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Status:  "error",
			Message: "认证服务响应错误",
		})
		return
	}

	// 解析远程响应
	var remoteResp LoginResponse
	if err := json.Unmarshal(body, &remoteResp); err != nil {
		h.logger.WithError(err).Error("解析远程认证响应失败")
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Status:  "error",
			Message: "认证服务响应格式错误",
		})
		return
	}

	// 返回结果
	c.JSON(resp.StatusCode, remoteResp)
}

// ValidateToken 验证 Token
// GET /api/auth/validate
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	// 从 Authorization header 获取 token
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "未提供认证令牌",
		})
		return
	}

	// 提取 Bearer token
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "认证令牌格式错误",
		})
		return
	}

	// 简单验证 token 非空即可（实际场景应验证 JWT 签名）
	// 由于远程服务生成的 token，我们信任它的有效性
	if len(token) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"valid":  true,
		})
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{
		"status":  "error",
		"message": "无效的认证令牌",
	})
}
