package monica

import (
	"bufio"
	"fmt"
	"io"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/sashabaranov/go-openai"
)

const (
	sseObject = "chat.completion.chunk"
	sseFinish = "[DONE]"
)

// SSEData 用于解析 Monica SSE json
type SSEData struct {
	Text     string `json:"text"`
	Finished bool   `json:"finished"`
}

// StreamMonicaSSEToClient 将 Monica SSE 转成前端可用的流
func StreamMonicaSSEToClient(model string, w io.Writer, r io.Reader) error {
	reader := bufio.NewReader(r)

	chatId := utils.RandStringUsingMathRand(29)
	now := time.Now().Unix()
	fingerprint := utils.RandStringUsingMathRand(10)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		// Monica SSE 的行前缀一般是 "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "" {
			continue
		}

		var sseObj SSEData
		if err := sonic.UnmarshalString(jsonStr, &sseObj); err != nil {
			// 如果解析失败，忽略该行或返回错误，看需求
			return err
		}
		// 将拆分后的文字写回
		var sseMsg types.ChatCompletionStreamResponse
		if sseObj.Finished {
			sseMsg = types.ChatCompletionStreamResponse{
				ID:      "chatcmpl-" + chatId,
				Object:  sseObject,
				Created: now,
				Model:   model,
				Choices: []types.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role: openai.ChatMessageRoleAssistant,
						},
						FinishReason: openai.FinishReasonStop,
					},
				},
			}
		} else {
			sseMsg = types.ChatCompletionStreamResponse{
				ID:                "chatcmpl-" + chatId,
				Object:            sseObject,
				SystemFingerprint: fingerprint,
				Created:           now,
				Model:             model,
				Choices: []types.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role:    openai.ChatMessageRoleAssistant,
							Content: sseObj.Text,
						},
						FinishReason: openai.FinishReasonNull,
					},
				},
			}
		}
		sendLine, _ := sonic.MarshalString(sseMsg)
		if _, err := w.Write([]byte(fmt.Sprintf("data:  %s\n\n", sendLine))); err != nil {
			return err
		}
		// 强制刷新缓冲区，确保数据立即发送给前端
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// 如果发现 finished=true，就可以结束
		if sseObj.Finished {
			_, _ = w.Write([]byte(sseFinish))
			return nil
		}
	}
}
