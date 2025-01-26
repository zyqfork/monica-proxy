package monica

import (
	"context"
	"github.com/go-resty/resty/v2"
	"log"
	"monica-proxy/internal/config"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
)

// SendMonicaRequest 发起对 Monica AI 的请求(使用 resty)
func SendMonicaRequest(ctx context.Context, mReq *types.MonicaRequest) (*resty.Response, error) {
	// 发起请求
	resp, err := utils.RestySSEClient.R().
		SetContext(ctx).
		SetHeader("cookie", config.MonicaConfig.MonicaCookie).
		SetBody(mReq).
		Post(types.BotChatURL)

	if err != nil {
		log.Printf("monica API error: %v", err)
		return nil, err
	}

	// 如果需要在这里做更多判断，可自行补充
	return resp, nil
}
