package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

var MonicaConfig *Config

// Config 存储应用配置
type Config struct {
	MonicaCookie string
	BearerToken  string
	IsIncognito  bool
	Debug        bool // 为 true 时输出 metrics 等调试日志
}

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	// 尝试加载 .env 文件，但不强制要求文件存在
	_ = godotenv.Load()

	MonicaConfig = &Config{
		MonicaCookie: os.Getenv("MONICA_COOKIE"),
		BearerToken:  os.Getenv("BEARER_TOKEN"),
		IsIncognito:  os.Getenv("IS_INCOGNITO") == "true",
		Debug:        strings.ToLower(os.Getenv("DEBUG")) == "true" || os.Getenv("DEBUG") == "1",
	}
	return MonicaConfig
}
