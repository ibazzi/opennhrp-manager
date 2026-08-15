package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/auth"
)

func AuthRequired(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""

		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				tokenStr = strings.TrimSpace(parts[1])
			}
		}

		if tokenStr == "" {
			tokenStr = c.Query("token")
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "未提供身份验证凭据，请先登录",
				"code":  "UNAUTHORIZED",
			})
			return
		}

		claims, err := auth.ValidateToken(tokenStr, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "登录凭据无效或已过期，请重新登录",
				"code":  "UNAUTHORIZED",
			})
			return
		}

		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "未登录",
				"code":  "UNAUTHORIZED",
			})
			return
		}

		role, ok := roleVal.(string)
		if !ok || role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "权限不足：当前操作仅限管理员执行，只读用户无权修改",
				"code":  "FORBIDDEN",
			})
			return
		}

		c.Next()
	}
}
