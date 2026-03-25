// Package middleware 提供了处理 HTTP 请求的中间件。
package middleware

import (
	"net/http"
	"pai-smart-go/internal/handler"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/code"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/token"
	"strings"
	"time"

	"pai-smart-go/pkg/database"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 创建一个 Gin 中间件，用于 JWT 认证。
// 它会从请求头中提取 token，验证其有效性，并将完整的 User 对象存入 Gin 的上下文中。
func AuthMiddleware(jwtManager *token.JWTManager, userService service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		res := new(handler.Response)

		// 从 Authorization 请求头中获取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 如果请求头为空，则中止请求，返回未授权状态
			c.AbortWithStatusJSON(http.StatusUnauthorized, res.SetCode(code.CodeNotLogin))
			return
		}

		// Token 通常以 "Bearer <token>" 的形式提供，我们需要提取出 token 本身
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			// 如果 token 格式不正确，则返回错误
			c.AbortWithStatusJSON(http.StatusUnauthorized, res.SetCode(code.CodeInvalidToken))
			return
		}
		tokenString := strings.TrimPrefix(authHeader, bearerPrefix)

		// 验证与“宽限期刷新”模拟：若过期则尝试刷新（对齐 Java 的宽限期刷新语义）
		claims, err := jwtManager.VerifyToken(tokenString)
		if err != nil {
			// 简化：尝试用 refreshToken 头部执行刷新（若存在）
			refresh := c.GetHeader("X-Refresh-Token")
			if refresh != "" {
				// 为保证无侵入，尝试解析 refresh 并签发新 access（此处仅日志提示，实际刷新入口仍在 /auth/refreshToken）
				if rclaims, rerr := jwtManager.VerifyToken(refresh); rerr == nil {
					if time.Until(rclaims.ExpiresAt.Time) > 0 {
						// 模拟前置刷新：记录日志并继续后续链路，由前端正式调用刷新接口
						log.Infof("检测到过期 access，存在仍有效的 refresh，可引导刷新")
					}
				}
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, res.SetCode(code.CodeInvalidToken))
			return
		}

		// 检查 token 类型，确保它是一个 access token，而不是 refresh token
		if claims.TokenType != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, res.SetCode(code.CodeInvalidToken))
			return
		}

		// ✅ 新增：检查 token 是否已被登出拉黑
		isBlacklisted, berr := database.RDB.Exists(c.Request.Context(), "blacklist:"+tokenString).Result()
		if berr != nil {
			log.Errorf("AuthMiddleware: 检查 token 黑名单失败: %v", berr)
			c.AbortWithStatusJSON(http.StatusUnauthorized, res.SetCode(code.CodeServerBusy))
			return
		}
		if isBlacklisted == 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, res.SetCode(code.CodeInvalidToken))
			return
		}

		// 使用 claims 中的用户名从数据库获取完整的用户信息
		user, err := userService.GetProfile(claims.Username)
		if err != nil {
			// 如果根据 token 中的用户信息无法找到用户，说明该用户可能已被删除
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			return
		}

		// 将完整的 User 对象存储在 context 中，供后续处理函数使用
		c.Set("user", user)

		// 为了向后兼容或特殊用途，仍然可以存储 claims
		c.Set("claims", claims)

		// 继续处理请求链中的下一个处理器
		c.Next()
	}
}
