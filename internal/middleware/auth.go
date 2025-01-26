package middleware

import (
	"monica-proxy/internal/config"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// BearerAuth 创建一个Bearer Token认证中间件
func BearerAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 获取Authorization header
			auth := c.Request().Header.Get("Authorization")

			// 检查header格式
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header")
			}

			// 提取token
			token := strings.TrimPrefix(auth, "Bearer ")

			// 验证token
			if token != config.MonicaConfig.BearerToken || token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			return next(c)
		}
	}
}
