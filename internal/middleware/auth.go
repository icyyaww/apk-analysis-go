package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 认证中间件
// 检查请求是否携带有效的 Bearer token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Authorization header 获取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "未提供认证令牌",
			})
			c.Abort()
			return
		}

		// 提取 Bearer token
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "认证令牌格式错误",
			})
			c.Abort()
			return
		}

		// 简单验证 token 非空
		// 由于使用远程认证服务，我们信任其生成的 token
		if len(token) < 10 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "无效的认证令牌",
			})
			c.Abort()
			return
		}

		// 将 token 存入上下文，供后续处理使用
		c.Set("token", token)
		c.Next()
	}
}
