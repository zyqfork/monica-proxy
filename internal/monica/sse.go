package monica

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/sashabaranov/go-openai"
)

const (
	sseObject     = "chat.completion.chunk"
	sseFinish     = "[DONE]"
	flushInterval = 100 * time.Millisecond // 刷新间隔
	bufferSize    = 4096                   // 缓冲区大小
)

// SSEData 用于解析 Monica SSE json
type SSEData struct {
	Text     string `json:"text"`
	Finished bool   `json:"finished"`
}

var sseDataPool = sync.Pool{
	New: func() interface{} {
		return &SSEData{}
	},
}

// StreamMonicaSSEToClient 将 Monica SSE 转成前端可用的流
func StreamMonicaSSEToClient(model string, w io.Writer, r io.Reader) error {
	reader := bufio.NewReaderSize(r, bufferSize)
	writer := bufio.NewWriterSize(w, bufferSize)
	defer writer.Flush()

	chatId := utils.RandStringUsingMathRand(29)
	now := time.Now().Unix()
	fingerprint := utils.RandStringUsingMathRand(10)

	// 创建一个定时刷新的 ticker
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	// 创建一个 done channel 用于清理
	done := make(chan struct{})
	defer close(done)

	// 启动一个 goroutine 定期刷新缓冲区
	go func() {
		for {
			select {
			case <-ticker.C:
				if f, ok := w.(http.Flusher); ok {
					writer.Flush()
					f.Flush()
				}
			case <-done:
				return
			}
		}
	}()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		// Monica SSE 的行前缀一般是 "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "" {
			continue
		}

		// 从对象池获取 SSEData
		sseObj := sseDataPool.Get().(*SSEData)
		if err := sonic.UnmarshalString(jsonStr, sseObj); err != nil {
			sseDataPool.Put(sseObj)
			// 记录错误但继续处理
			log.Printf("Error unmarshaling SSE data: %v", err)
			continue
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

		var sb strings.Builder
		sb.WriteString("data: ")
		sendLine, _ := sonic.MarshalString(sseMsg)
		sb.WriteString(sendLine)
		sb.WriteString("\n\n")

		// 写入缓冲区
		if _, err := writer.WriteString(sb.String()); err != nil {
			sseDataPool.Put(sseObj)
			return fmt.Errorf("write error: %w", err)
		}

		// 如果发现 finished=true，就可以结束
		if sseObj.Finished {
			writer.WriteString(fmt.Sprintf("data: %s\n\n", sseFinish))
			writer.Flush()
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			sseDataPool.Put(sseObj)
			return nil
		}

		// 将对象放回对象池
		sseDataPool.Put(sseObj)
	}
}
