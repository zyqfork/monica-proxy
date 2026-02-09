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
	"net/http"
)

func main() {
	// 定义命令行参数
	port := flag.Int("p", 8080, "服务器监听端口")
	host := flag.String("h", "0.0.0.0", "服务器监听地址")
	monicaCookie := flag.String("c", "", "Monica Cookie值 (MONICA_COOKIE)")
	bearerToken := flag.String("k", "", "Bearer Token值 (BEARER_TOKEN)")
	isIncognito := flag.Bool("i", true, "是否启用隐身模式 (IS_INCOGNITO)")
	debug := flag.Bool("d", false, "是否启用调试日志 (DEBUG，输出 metrics 等)")

	flag.Usage = func() {
		fmt.Printf("用法: %s [选项]\n\n", flag.CommandLine.Name())
		fmt.Println("选项:")
		flag.PrintDefaults()
		fmt.Println("\n示例: ./monica-proxy -p 8080 -c \"cookie\" -k \"token\" -i=false")
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

	// 检查必要的配置
	if cfg.MonicaCookie == "" {
		log.Fatal("Monica Cookie is required. Please set it via -c flag or MONICA_COOKIE environment variable")
	}
	if cfg.BearerToken == "" {
		log.Fatal("Bearer Token is required. Please set it via -k flag or BEARER_TOKEN environment variable")
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	// 注册路由
	apiserver.RegisterRoutes(e)

	// 启动服务
	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("Server starting on %s", addr)
	log.Printf("Incognito mode: %v", cfg.IsIncognito)
	if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("start server error: %v", err)
	}
}
