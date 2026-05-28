package monica

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
)

// MonicaStreamResult 从 Monica SSE 解析的完整回复。
type MonicaStreamResult struct {
	Content           string
	Meta              types.MonicaReplyMeta
	LastThinkingUID   string
}

// CollectMonicaStream 读取 Monica SSE 直至 finished。
func CollectMonicaStream(ctx context.Context, r io.Reader) (*MonicaStreamResult, error) {
	reader := bufio.NewReader(r)
	var fullContent strings.Builder
	meta := types.MonicaReplyMeta{
		Artifacts: []any{},
	}
	var lastThinkingUID string
	inThink := false

	type readResult struct {
		line string
		err  error
	}
	lineChan := make(chan readResult, 1)
	go func() {
		defer close(lineChan)
		for {
			line, err := reader.ReadString('\n')
			select {
			case lineChan <- readResult{line: line, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result, ok := <-lineChan:
			if !ok {
				return nil, ctx.Err()
			}
			line, err := result.line, result.err
			if err != nil {
				if err == io.EOF {
					return &MonicaStreamResult{Content: fullContent.String(), Meta: meta, LastThinkingUID: lastThinkingUID}, nil
				}
				return nil, fmt.Errorf("read monica sse: %w", err)
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
				return nil, fmt.Errorf("parse monica sse: %w", err)
			}

			if sseData.AgentStatus.Type == "thinking" {
				inThink = true
				lastThinkingUID = sseData.AgentStatus.UID
				meta.AgentStatus = append(meta.AgentStatus, types.AgentStatusEntry{
					UID:  sseData.AgentStatus.UID,
					Type: "thinking",
					Text: "",
				})
			} else if sseData.AgentStatus.Type == "thinking_detail_stream" {
				if inThink && len(meta.AgentStatus) > 0 {
					meta.AgentStatus[len(meta.AgentStatus)-1].Text += sseData.AgentStatus.Metadata.ReasoningDetail
				}
			} else if sseData.AgentStatus.Type == "draw_img_result" && sseData.AgentStatus.Metadata.ImageURL != "" {
				fullContent.WriteString("\n![image](" + sseData.AgentStatus.Metadata.ImageURL + ")\n")
				fullContent.WriteString(sseData.Text)
			} else {
				if inThink {
					inThink = false
				}
				fullContent.WriteString(sseData.Text)
			}

			if len(sseData.FollowSuggestions) > 0 {
				meta.FollowSuggestions = sseData.FollowSuggestions
			}

			if sseData.Finished {
				return &MonicaStreamResult{
					Content:         fullContent.String(),
					Meta:            meta,
					LastThinkingUID: lastThinkingUID,
				}, nil
			}
		}
	}
}

func outputTextPart(text string) map[string]any {
	return map[string]any{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
	}
}

func outputMessageItem(id, status, text string) map[string]any {
	content := []any{}
	if text != "" {
		content = append(content, outputTextPart(text))
	}
	return map[string]any{
		"type":    "message",
		"id":      id,
		"status":  status,
		"role":    "assistant",
		"content": content,
	}
}

// StreamMonicaToResponsesSSE 将 Monica SSE 转为 OpenAI Responses API 流式事件。
func StreamMonicaToResponsesSSE(
	ctx context.Context,
	w io.Writer,
	r io.Reader,
	respID, model string,
	createdAt int64,
) (*MonicaStreamResult, error) {
	writer := bufio.NewWriter(w)
	seq := 0
	msgItemID := "msg_" + respID[5:]
	if len(respID) < 5 {
		msgItemID = "msg_" + respID
	}
	const outputIndex = 0
	const contentIndex = 0

	writeEvent := func(eventType string, payload map[string]any) error {
		payload["type"] = eventType
		payload["sequence_number"] = seq
		seq++
		line, err := sonic.MarshalString(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", eventType, line); err != nil {
			return err
		}
		if f, ok := w.(http.Flusher); ok {
			writer.Flush()
			f.Flush()
		}
		return nil
	}

	if err := writeEvent(string(types.ResponseEventCreated), map[string]any{
		"response": types.NewResponseSnapshot(respID, createdAt, model, "queued"),
	}); err != nil {
		return nil, err
	}
	if err := writeEvent(string(types.ResponseEventInProgress), map[string]any{
		"response": types.NewResponseSnapshot(respID, createdAt, model, "in_progress"),
	}); err != nil {
		return nil, err
	}
	if err := writeEvent(string(types.ResponseEventOutputItemAdded), map[string]any{
		"output_index": outputIndex,
		"item":         outputMessageItem(msgItemID, "in_progress", ""),
	}); err != nil {
		return nil, err
	}
	if err := writeEvent(string(types.ResponseEventContentPartAdded), map[string]any{
		"item_id":        msgItemID,
		"output_index":   outputIndex,
		"content_index":  contentIndex,
		"part":           outputTextPart(""),
	}); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(r)
	var fullContent strings.Builder
	meta := types.MonicaReplyMeta{Artifacts: []any{}}
	var lastThinkingUID string
	inThink := false

	type readResult struct {
		line string
		err  error
	}
	lineChan := make(chan readResult, 1)
	go func() {
		defer close(lineChan)
		for {
			line, err := reader.ReadString('\n')
			select {
			case lineChan <- readResult{line: line, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	emitDelta := func(chunk string) error {
		if chunk == "" {
			return nil
		}
		return writeEvent(string(types.ResponseEventOutputTextDelta), map[string]any{
			"delta":          chunk,
			"item_id":        msgItemID,
			"output_index":   outputIndex,
			"content_index":  contentIndex,
			"logprobs":       []any{},
		})
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result, ok := <-lineChan:
			if !ok {
				return nil, ctx.Err()
			}
			line, err := result.line, result.err
			if err != nil {
				if err == io.EOF {
					goto done
				}
				return nil, fmt.Errorf("read monica sse: %w", err)
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
				continue
			}

			if sseData.AgentStatus.Type == "thinking" {
				inThink = true
				lastThinkingUID = sseData.AgentStatus.UID
				meta.AgentStatus = append(meta.AgentStatus, types.AgentStatusEntry{
					UID: sseData.AgentStatus.UID, Type: "thinking", Text: "",
				})
			} else if sseData.AgentStatus.Type == "thinking_detail_stream" {
				// thinking 内容不写入 output_text delta
			} else if sseData.AgentStatus.Type == "draw_img_result" && sseData.AgentStatus.Metadata.ImageURL != "" {
				chunk := "\n![image](" + sseData.AgentStatus.Metadata.ImageURL + ")\n" + sseData.Text
				fullContent.WriteString(chunk)
				if err := emitDelta(chunk); err != nil {
					return nil, err
				}
			} else if sseData.Text != "" {
				if inThink {
					inThink = false
				}
				fullContent.WriteString(sseData.Text)
				if err := emitDelta(sseData.Text); err != nil {
					return nil, err
				}
			}

			if len(sseData.FollowSuggestions) > 0 {
				meta.FollowSuggestions = sseData.FollowSuggestions
			}

			if sseData.Finished {
				goto done
			}
		}
	}

done:
	text := fullContent.String()
	if err := writeEvent(string(types.ResponseEventOutputTextDone), map[string]any{
		"text":           text,
		"item_id":        msgItemID,
		"output_index":   outputIndex,
		"content_index":  contentIndex,
		"logprobs":       []any{},
	}); err != nil {
		return nil, err
	}
	if err := writeEvent(string(types.ResponseEventContentPartDone), map[string]any{
		"item_id":       msgItemID,
		"output_index":  outputIndex,
		"content_index": contentIndex,
		"part":          outputTextPart(text),
	}); err != nil {
		return nil, err
	}
	if err := writeEvent(string(types.ResponseEventOutputItemDone), map[string]any{
		"output_index": outputIndex,
		"item":         outputMessageItem(msgItemID, "completed", text),
	}); err != nil {
		return nil, err
	}

	usage := &types.ResponseUsage{
		InputTokens:  0,
		OutputTokens: utils.CalculateTokens(text),
		TotalTokens:  utils.CalculateTokens(text),
	}
	completed := types.NewResponseObject(respID, createdAt, model, "completed", text, usage)
	if err := writeEvent(string(types.ResponseEventCompleted), map[string]any{
		"response": completed,
	}); err != nil {
		return nil, err
	}
	writer.Flush()
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return &MonicaStreamResult{Content: text, Meta: meta, LastThinkingUID: lastThinkingUID}, nil
}

// ResponseID 生成 OpenAI 风格的 response id。
func ResponseID() string {
	return "resp_" + utils.RandStringUsingMathRand(48)
}

func ResponseCreatedAt() int64 {
	return time.Now().Unix()
}
