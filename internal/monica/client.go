package monica

import (
	"context"
	"github.com/go-resty/resty/v2"
	"log"
	"monica-proxy/internal/config"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
)

func SendMonicaRequest(ctx context.Context, mReq *types.MonicaRequest) (*resty.Response, error) {
	resp, err := utils.RestySSEClient.R().
		SetContext(ctx).
		SetHeader("cookie", config.MonicaConfig.MonicaCookie).
		SetHeader("Accept", "text/event-stream").
		SetDoNotParseResponse(true). // 不自动解析响应
		SetBody(mReq).
		Post(types.BotChatURL)

	if err != nil {
		log.Printf("Monica API error: %v", err)
		return nil, err
	}

	return resp, nil
}
