package monica

import (
	"bufio"
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/sashabaranov/go-openai"
	"io"
	"log"
	"monica-proxy/internal/utils"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	sseObject         = "chat.completion.chunk"
	completionsObject = "chat.completions"
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

func ProcessMonicaResponse(ctx context.Context, req openai.ChatCompletionRequest, r io.Reader, fp string) (openai.ChatCompletionResponse, error) {
	reader := bufio.NewReader(r)
	var fullContent strings.Builder
	var thinkContent strings.Builder
	inThinkBlock := false

	chatId := utils.RandStringUsingMathRand(29)
	now := time.Now().Unix()

	for {
		select {
		case <-ctx.Done():
			return openai.ChatCompletionResponse{}, ctx.Err()
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// 处理完所有数据
					return createMessage(chatId, now, req, utils.CalculateUsage(req, fullContent.String()), fullContent.String(), fp), nil
				}
				return openai.ChatCompletionResponse{}, fmt.Errorf("读取错误: %w", err)
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			if jsonStr == "" || jsonStr == sseFinish {
				continue
			}

			var sseData SSEData
			if err := sonic.UnmarshalString(jsonStr, &sseData); err != nil {
				return openai.ChatCompletionResponse{}, fmt.Errorf("解析SSE数据错误: %w", err)
			}

			// 处理思考块
			if sseData.AgentStatus.Type == "thinking" {
				inThinkBlock = true
				thinkContent.WriteString("<think>\n")
			} else if sseData.AgentStatus.Type == "thinking_detail_stream" {
				if inThinkBlock {
					thinkContent.WriteString(sseData.AgentStatus.Metadata.ReasoningDetail)
				}
			} else {
				// 普通文本内容
				if inThinkBlock {
					thinkContent.WriteString("</think>")
					fullContent.WriteString(thinkContent.String())
					thinkContent.Reset()
					inThinkBlock = false
				}
				fullContent.WriteString(sseData.Text)
			}

			if sseData.Finished {
				return createMessage(chatId, now, req, utils.CalculateUsage(req, fullContent.String()), fullContent.String(), fp), nil
			}
		}
	}
}

func StreamMonicaSSEToClient(ctx context.Context, req openai.ChatCompletionRequest, w io.Writer, r io.Reader, fp string) error {
	log.Printf("=== Starting SSE Stream Processing for model: %s ===", req.Model)
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
	fingerprint := fp

	log.Printf("Session initialized - ChatID: %s, Fingerprint: %s", chatId, fingerprint)

	var completionBuilder strings.Builder
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

		//log.Printf(line)
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

		log.Printf("Received SSE data: %+v", sseData)

		messageCount++
		atomic.AddInt64(&metrics.CurrentMessages, 1)

		if err := retryProcessMessage(writer, w, sseData, chatId, fingerprint, now, &thinkFlag, metrics, &completionBuilder, req); err != nil {
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

func retryProcessMessage(writer *bufio.Writer, w io.Writer, sseData SSEData, chatId, fingerprint string, now int64, thinkFlag *bool, metrics *Metrics, completionBuilder *strings.Builder, req openai.ChatCompletionRequest) error {
	for retry := 0; retry < maxRetries; retry++ {
		if err := processMessage(writer, w, sseData, chatId, fingerprint, now, thinkFlag, metrics, completionBuilder, req); err != nil {
			log.Printf("Retry %d: %v", retry, err)
			time.Sleep(time.Duration(retry+1) * 100 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("max retries exceeded")
}

func processMessage(writer *bufio.Writer, w io.Writer, sseData SSEData, chatId, fingerprint string, now int64, thinkFlag *bool, metrics *Metrics, completionBuilder *strings.Builder, req openai.ChatCompletionRequest) error {
	estimatedSize := int64(len(sseData.Text) + 256) // 256 bytes for overhead
	newSize := atomic.AddInt64(&metrics.BufferUsage, estimatedSize)

	if newSize > maxBufferSize {
		atomic.AddInt64(&metrics.BufferUsage, -estimatedSize)
		atomic.AddInt64(&metrics.ErrorCount, 1)
		return fmt.Errorf("message size would exceed buffer limit")
	}

	var sseMsg openai.ChatCompletionStreamResponse

	if sseData.AgentStatus.Type == "thinking_detail_stream" {
		sseMsg = createStreamMessage(chatId, now, req, fingerprint, "", sseData.AgentStatus.Metadata.ReasoningDetail)
		completionBuilder.WriteString(sseData.Text)
	} else {
		sseMsg = createStreamMessage(chatId, now, req, fingerprint, sseData.Text, "")
		completionBuilder.WriteString(sseData.Text)
	}

	if sseData.Finished {
		sseMsg.Choices[0].FinishReason = openai.FinishReasonStop
		usage := utils.CalculateUsage(req, completionBuilder.String())
		sseMsg.Usage = &usage
	}
	return sendMessage(writer, w, sseMsg)
}

func createStreamMessage(chatId string, now int64, req openai.ChatCompletionRequest, fingerPrint string, conent string, reasoningContent string) openai.ChatCompletionStreamResponse {
	choice := openai.ChatCompletionStreamChoice{
		Index: 0,
		Delta: openai.ChatCompletionStreamChoiceDelta{
			Role:             openai.ChatMessageRoleAssistant,
			Content:          conent,
			ReasoningContent: reasoningContent,
		},
		ContentFilterResults: openai.ContentFilterResults{},
		FinishReason:         openai.FinishReasonNull,
	}

	return openai.ChatCompletionStreamResponse{
		ID:                "chatcmpl-" + chatId,
		Object:            sseObject,
		Created:           now,
		Model:             req.Model,
		Choices:           []openai.ChatCompletionStreamChoice{choice},
		SystemFingerprint: fingerPrint,
	}
}

func createMessage(chatId string, now int64, req openai.ChatCompletionRequest, usage openai.Usage, content string, fp string) openai.ChatCompletionResponse {
	choice := openai.ChatCompletionChoice{
		Index: 0,
		Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		},
		FinishReason: openai.FinishReasonStop,
	}

	return openai.ChatCompletionResponse{
		ID:                "chatcmpl-" + chatId,
		Object:            completionsObject,
		Created:           now,
		Model:             req.Model,
		Choices:           []openai.ChatCompletionChoice{choice},
		SystemFingerprint: fp,
		Usage:             usage,
	}
}

func createThinkMessage(chatId string, now int64, req openai.ChatCompletionRequest, usage openai.Usage, content string, fp string) openai.ChatCompletionResponse {
	choice := openai.ChatCompletionChoice{
		Index: 0,
		Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		},
		FinishReason: openai.FinishReasonStop,
	}

	return openai.ChatCompletionResponse{
		ID:                "chatcmpl-" + chatId,
		Object:            completionsObject,
		Created:           now,
		Model:             req.Model,
		Choices:           []openai.ChatCompletionChoice{choice},
		SystemFingerprint: fp,
		Usage:             usage,
	}
}

func sendMessage(writer *bufio.Writer, w io.Writer, sseMsg openai.ChatCompletionStreamResponse) error {
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
