package monica

import (
	"bufio"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/sashabaranov/go-openai"
	"io"
	"log"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
	"net/http"
	"strings"
	"time"
)

const (
	sseObject  = "chat.completion.chunk"
	sseFinish  = "[DONE]"
	bufferSize = 4096
)

type SSEData struct {
	Text     string `json:"text"`
	Finished bool   `json:"finished"`
}

func StreamMonicaSSEToClient(model string, w io.Writer, r io.Reader) error {
	log.Printf("=== Starting SSE Stream Processing for model: %s ===", model)

	reader := bufio.NewReaderSize(r, bufferSize)
	writer := bufio.NewWriterSize(w, bufferSize)

	chatId := utils.RandStringUsingMathRand(29)
	now := time.Now().Unix()
	fingerprint := utils.RandStringUsingMathRand(10)

	log.Printf("Session initialized - ChatID: %s, Fingerprint: %s", chatId, fingerprint)

	var messageBuffer strings.Builder
	messageCount := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Printf("Reached EOF after %d messages", messageCount)
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		messageBuffer.WriteString(line)

		if !strings.HasSuffix(line, "\n") {
			continue
		}

		message := messageBuffer.String()
		messageBuffer.Reset()

		if !strings.HasPrefix(message, "data: ") {
			continue
		}

		jsonStr := strings.TrimSpace(strings.TrimPrefix(message, "data: "))
		if jsonStr == "" {
			continue
		}

		var sseData SSEData
		if err := sonic.UnmarshalString(jsonStr, &sseData); err != nil {
			log.Printf("Error unmarshaling SSE data: %v", err)
			continue
		}

		messageCount++

		// 创建响应消息
		sseMsg := types.ChatCompletionStreamResponse{
			ID:      "chatcmpl-" + chatId,
			Object:  sseObject,
			Created: now,
			Model:   model,
			Choices: []types.ChatCompletionStreamChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Role:    openai.ChatMessageRoleAssistant,
						Content: sseData.Text,
					},
					FinishReason: openai.FinishReasonNull,
				},
			},
		}

		if !sseData.Finished {
			sseMsg.SystemFingerprint = fingerprint
		} else {
			sseMsg.Choices[0].FinishReason = openai.FinishReasonStop
		}

		sendLine, err := sonic.MarshalString(sseMsg)
		if err != nil {
			return fmt.Errorf("marshal error: %w", err)
		}

		outputMsg := fmt.Sprintf("data: %s\n\n", sendLine)
		if _, err := writer.WriteString(outputMsg); err != nil {
			return fmt.Errorf("write error: %w", err)
		}

		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush error: %w", err)
		}

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		if sseData.Finished {
			finishMsg := fmt.Sprintf("data: %s\n\n", sseFinish)
			if _, err := writer.WriteString(finishMsg); err != nil {
				return fmt.Errorf("write finish signal error: %w", err)
			}
			if err := writer.Flush(); err != nil {
				return fmt.Errorf("final flush error: %w", err)
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			log.Printf("Stream completed successfully after %d messages", messageCount)
			return nil
		}
	}
}
