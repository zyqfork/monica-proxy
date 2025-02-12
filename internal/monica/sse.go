package monica

import (
	"bufio"
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/sashabaranov/go-openai"
	"io"
	"log"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	sseObject         = "chat.completion.chunk"
	sseFinish         = "[DONE]"
	initialBufferSize = 4096
	maxBufferSize     = 1024 * 1024 // 1MB
	flushThreshold    = 10
	heartbeatInterval = 30 * time.Second
	maxRetries        = 3
)

type SSEData struct {
	Text        string      `json:"text"`
	Finished    bool        `json:"finished"`
	AgentStatus AgentStatus `json:"agent_status,omitempty"`
}

type AgentStatus struct {
	UID      string `json:"uid"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	Metadata struct {
		Title           string `json:"title"`
		ReasoningDetail string `json:"reasoning_detail"`
	} `json:"metadata"`
}

type Metrics struct {
	ProcessingTime  time.Duration
	BufferUsage     int64
	ErrorCount      int64
	TotalProcessed  int64
	MaxBufferUsed   int64
	CurrentMessages int64
}

func (m *Metrics) updateBufferUsage(size int64) {
	current := atomic.AddInt64(&m.BufferUsage, size)
	for {
		max := atomic.LoadInt64(&m.MaxBufferUsed)
		if current <= max || atomic.CompareAndSwapInt64(&m.MaxBufferUsed, max, current) {
			break
		}
	}
}

func logMetrics(metrics *Metrics) {
	log.Printf("Metrics - Buffer Usage: %d/%d bytes (%.2f%%), Total Processed: %d, Errors: %d, Messages: %d",
		atomic.LoadInt64(&metrics.BufferUsage),
		maxBufferSize,
		float64(atomic.LoadInt64(&metrics.BufferUsage))/float64(maxBufferSize)*100,
		atomic.LoadInt64(&metrics.TotalProcessed),
		atomic.LoadInt64(&metrics.ErrorCount),
		atomic.LoadInt64(&metrics.CurrentMessages))
}

func StreamMonicaSSEToClient(ctx context.Context, model string, w io.Writer, r io.Reader) error {
	log.Printf("=== Starting SSE Stream Processing for model: %s ===", model)

	metrics := &Metrics{}
	startTime := time.Now()
	defer func() {
		metrics.ProcessingTime = time.Since(startTime)
		logMetrics(metrics)
		log.Printf("Stream processing completed. Total time: %v", metrics.ProcessingTime)
	}()

	reader := bufio.NewReaderSize(r, initialBufferSize)
	writer := bufio.NewWriterSize(w, initialBufferSize)

	chatId := utils.RandStringUsingMathRand(29)
	now := time.Now().Unix()
	fingerprint := utils.RandStringUsingMathRand(10)

	log.Printf("Session initialized - ChatID: %s, Fingerprint: %s", chatId, fingerprint)

	var messageBuffer strings.Builder
	messageCount := 0
	var thinkFlag bool
	var totalBufferSize int64

	// 创建心跳检测器
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	// 创建metrics日志记录器
	metricsLogger := time.NewTicker(5 * time.Second)
	defer metricsLogger.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := sendHeartbeat(writer, w); err != nil {
				log.Printf("Heartbeat error: %v", err)
			}
			continue
		case <-metricsLogger.C:
			logMetrics(metrics)
			continue
		default:
			// 继续正常处理
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Printf("Reached EOF after %d messages", messageCount)
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		lineSize := int64(len(line))
		newBufferSize := atomic.AddInt64(&totalBufferSize, lineSize)

		if newBufferSize > maxBufferSize {
			atomic.AddInt64(&metrics.ErrorCount, 1)
			log.Printf("Buffer overflow: current size %d exceeds max size %d", newBufferSize, maxBufferSize)
			return fmt.Errorf("buffer overflow: exceeded maximum buffer size of %d bytes", maxBufferSize)
		}

		messageBuffer.WriteString(line)
		metrics.updateBufferUsage(lineSize)
		atomic.AddInt64(&metrics.TotalProcessed, lineSize)

		if !strings.HasSuffix(line, "\n") {
			continue
		}

		atomic.AddInt64(&totalBufferSize, -int64(messageBuffer.Len()))
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
			atomic.AddInt64(&metrics.ErrorCount, 1)
			log.Printf("Error unmarshaling SSE data: %v", err)
			continue
		}

		messageCount++
		atomic.AddInt64(&metrics.CurrentMessages, 1)

		if err := retryProcessMessage(ctx, writer, w, sseData, model, chatId, fingerprint, now, &thinkFlag, metrics); err != nil {
			log.Printf("Failed to process message after %d retries: %v", maxRetries, err)
			return err
		}

		if messageCount >= flushThreshold {
			if err := flushWriter(writer, w); err != nil {
				return fmt.Errorf("flush error: %w", err)
			}
			messageCount = 0
		}

		if sseData.Finished {
			if err := sendFinishSignal(writer, w); err != nil {
				return fmt.Errorf("finish signal error: %w", err)
			}
			log.Printf("Stream completed successfully after %d messages", atomic.LoadInt64(&metrics.CurrentMessages))
			return nil
		}
	}
}

func retryProcessMessage(ctx context.Context, writer *bufio.Writer, w io.Writer, sseData SSEData, model, chatId, fingerprint string, now int64, thinkFlag *bool, metrics *Metrics) error {
	for retry := 0; retry < maxRetries; retry++ {
		if err := processMessage(ctx, writer, w, sseData, model, chatId, fingerprint, now, thinkFlag, metrics); err != nil {
			log.Printf("Retry %d: %v", retry, err)
			time.Sleep(time.Duration(retry+1) * 100 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("max retries exceeded")
}

func processMessage(ctx context.Context, writer *bufio.Writer, w io.Writer, sseData SSEData, model, chatId, fingerprint string, now int64, thinkFlag *bool, metrics *Metrics) error {
	estimatedSize := int64(len(sseData.Text) + 256) // 256 bytes for overhead
	newSize := atomic.AddInt64(&metrics.BufferUsage, estimatedSize)

	if newSize > maxBufferSize {
		atomic.AddInt64(&metrics.BufferUsage, -estimatedSize)
		atomic.AddInt64(&metrics.ErrorCount, 1)
		return fmt.Errorf("message size would exceed buffer limit")
	}

	var sseMsg types.ChatCompletionStreamResponse

	if sseData.AgentStatus.Type == "thinking" {
		*thinkFlag = true
		sseMsg = createThinkingMessage(chatId, fingerprint, now, model)
	} else if sseData.AgentStatus.Type == "thinking_detail_stream" {
		sseMsg = createThinkingDetailMessage(chatId, fingerprint, now, model, sseData)
	} else {
		if *thinkFlag {
			sseData.Text = "</think>" + sseData.Text
			*thinkFlag = false
		}
		sseMsg = createNormalMessage(chatId, fingerprint, now, model, sseData)
	}

	if !sseData.Finished {
		sseMsg.SystemFingerprint = fingerprint
	} else {
		sseMsg.Choices[0].FinishReason = openai.FinishReasonStop
	}

	return sendMessage(writer, w, sseMsg)
}

func createThinkingMessage(chatId, fingerprint string, now int64, model string) types.ChatCompletionStreamResponse {
	return types.ChatCompletionStreamResponse{
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
					Content: "<think>\n",
				},
				FinishReason: openai.FinishReasonNull,
			},
		},
	}
}

func createThinkingDetailMessage(chatId, fingerprint string, now int64, model string, sseData SSEData) types.ChatCompletionStreamResponse {
	return types.ChatCompletionStreamResponse{
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
					Content: sseData.AgentStatus.Metadata.ReasoningDetail,
				},
				FinishReason: openai.FinishReasonNull,
			},
		},
	}
}

func createNormalMessage(chatId, fingerprint string, now int64, model string, sseData SSEData) types.ChatCompletionStreamResponse {
	return types.ChatCompletionStreamResponse{
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
}

func sendMessage(writer *bufio.Writer, w io.Writer, sseMsg types.ChatCompletionStreamResponse) error {
	sendLine, err := sonic.MarshalString(sseMsg)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	outputMsg := fmt.Sprintf("data: %s\n\n", sendLine)
	if _, err := writer.WriteString(outputMsg); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return flushWriter(writer, w)
}

func sendHeartbeat(writer *bufio.Writer, w io.Writer) error {
	if _, err := writer.WriteString(": keepalive\n\n"); err != nil {
		return fmt.Errorf("heartbeat write error: %w", err)
	}
	return flushWriter(writer, w)
}

func sendFinishSignal(writer *bufio.Writer, w io.Writer) error {
	finishMsg := fmt.Sprintf("data: %s\n\n", sseFinish)
	if _, err := writer.WriteString(finishMsg); err != nil {
		return fmt.Errorf("write finish signal error: %w", err)
	}
	return flushWriter(writer, w)
}

func flushWriter(writer *bufio.Writer, w io.Writer) error {
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush error: %w", err)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
