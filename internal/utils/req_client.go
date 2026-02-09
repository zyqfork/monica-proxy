package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// newOptimizedTransport 创建 HTTP 传输配置：适度复用连接，控制内存占用
// Go 默认 MaxIdleConnsPerHost=2；过大（如 100）会保留大量空闲连接，每连接约数十 KB，导致内存明显上升
func newOptimizedTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2, // 单主机 10 条空闲连接足够复用，避免占用过多内存
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
}

var (
	RestySSEClient = resty.New().
			SetTimeout(3 * time.Minute).
			SetTransport(newOptimizedTransport()).
			SetDoNotParseResponse(true).
			SetHeaders(map[string]string{
			"Content-Type":    "application/json",
			"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"x-client-locale": "zh_CN",
		}).
		OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
			if resp.StatusCode() != 200 {
				return fmt.Errorf("monica API error: status %d, body: %s",
					resp.StatusCode(), resp.String())
			}
			return nil
		})

	RestyDefaultClient = resty.New().
				SetTimeout(time.Second * 30).
				SetTransport(newOptimizedTransport()).
				SetHeaders(map[string]string{
			"Content-Type": "application/json",
			"User-Agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		}).
		OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
			if resp.StatusCode() != 200 {
				return fmt.Errorf("monica API error: status %d, body: %s",
					resp.StatusCode(), resp.String())
			}
			return nil
		})
)
