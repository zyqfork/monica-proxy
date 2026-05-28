package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"log"
	"monica-proxy/internal/apiserver"
	"monica-proxy/internal/config"
	"monica-proxy/internal/types"
	"net/http"
	"time"
)

func main() {
	// 定义命令行参数
	port := flag.Int("p", 8080, "服务器监听端口")
	host := flag.String("h", "0.0.0.0", "服务器监听地址")
	monicaCookie := flag.String("c", "", "Monica Cookie值 (MONICA_COOKIE)")
	bearerToken := flag.String("k", "", "Bearer Token值 (BEARER_TOKEN)")
	isIncognito := flag.Bool("i", true, "是否启用隐身模式 (IS_INCOGNITO)")
	debug := flag.Bool("d", false, "是否启用调试日志 (DEBUG，输出 metrics 等)")
	cacheDir := flag.String("cache-dir", "", "Responses 有状态会话 SQLite 目录或 .db 文件路径 (SESSION_CACHE_DIR，默认 ./.monica-proxy-cache/sessions.db)")

	flag.Usage = func() {
		fmt.Printf("用法: %s [选项]\n\n", flag.CommandLine.Name())
		fmt.Println("选项:")
		flag.PrintDefaults()
		fmt.Println("\n示例: ./monica-proxy -p 8080 -c \"cookie\" -k \"token\" -cache-dir ./data")
	}

	// 解析命令行参数
	flag.Parse()

	cfg := config.LoadConfig()
	// 如果命令行指定了参数，则覆盖环境变量配置
	if *monicaCookie != "" {
		cfg.MonicaCookie = *monicaCookie
	}
	if *bearerToken != "" {
		cfg.BearerToken = *bearerToken
	}
	// 设置 IsIncognito 和 Debug 值
	cfg.IsIncognito = *isIncognito
	cfg.Debug = *debug
	if *cacheDir != "" {
		cfg.SessionCacheDir = *cacheDir
	}
	if cfg.SessionCacheDir == "" {
		cfg.SessionCacheDir = "./.monica-proxy-cache"
	}

	// 检查必要的配置
	if cfg.MonicaCookie == "" {
		log.Fatal("Monica Cookie is required. Please set it via -c flag or MONICA_COOKIE environment variable")
	}
	if cfg.BearerToken == "" {
		log.Fatal("Bearer Token is required. Please set it via -k flag or BEARER_TOKEN environment variable")
	}

	if err := types.InitSessionStore(cfg.SessionCacheDir, 30*24*time.Hour); err != nil {
		log.Fatalf("init session cache: %v", err)
	}

	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	// 注册路由
	apiserver.RegisterRoutes(e)

	// 启动服务
	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("Server starting on %s", addr)
	log.Printf("Incognito mode: %v", cfg.IsIncognito)
	dbPath, _ := types.ResolveSessionDBPath(cfg.SessionCacheDir)
	log.Printf("Session cache (SQLite): %s (TTL 30 days, store=true enables stateful mode)", dbPath)
	if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("start server error: %v", err)
	}
}
