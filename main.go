package main

import (
	"errors"
	"log"
	"monica-proxy/internal/apiserver"
	"monica-proxy/internal/config"
	"net/http"

	"github.com/labstack/echo/v4/middleware"

	"github.com/labstack/echo/v4"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()
	if cfg.MonicaCookie == "" {
		log.Fatal("MONICA_COOKIE environment variable is required")
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	// 注册路由
	apiserver.RegisterRoutes(e)
	// 启动服务
	if err := e.Start("0.0.0.0:8080"); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("start server error: %v", err)
	}
}
