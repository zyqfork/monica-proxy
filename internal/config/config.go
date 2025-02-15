package config

import (
	"os"

	"github.com/joho/godotenv"
)

var MonicaConfig *Config

// Config 存储应用配置
type Config struct {
	MonicaCookie string
	BearerToken  string
	IsIncognito  bool
}

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	// 尝试加载 .env 文件，但不强制要求文件存在
	_ = godotenv.Load()

	MonicaConfig = &Config{
		MonicaCookie: os.Getenv("MONICA_COOKIE"),
		BearerToken:  os.Getenv("BEARER_TOKEN"),
		IsIncognito:  os.Getenv("IS_INCOGNITO") == "true",
	}
	return MonicaConfig
}
