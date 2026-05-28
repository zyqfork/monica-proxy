package types

import (
	"encoding/json"
	"fmt"
)

// --- OpenAI Responses API 类型（子集，兼容官方 SDK）---

type CreateResponseRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Instructions       string          `json:"instructions,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Stream             bool            `json:"stream,omitempty"`
	Store              *bool           `json:"store,omitempty"`
}

type ResponseInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ResponseInputContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ResponseObject struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	CreatedAt  int64          `json:"created_at"`
	Model      string         `json:"model"`
	Status     string         `json:"status"`
	Output     []OutputItem   `json:"output"`
	OutputText string         `json:"output_text,omitempty"`
	Usage      *ResponseUsage `json:"usage,omitempty"`
}

type OutputItem struct {
	Type    string              `json:"type"`
	ID      string              `json:"id"`
	Role    string              `json:"role,omitempty"`
	Status  string              `json:"status,omitempty"`
	Content []OutputContentPart `json:"content,omitempty"`
}

type OutputContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ResponseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponseStreamEvent SSE 事件（兼容 OpenAI Responses streaming）
type ResponseStreamEvent struct {
	Type           string          `json:"type"`
	SequenceNumber int             `json:"sequence_number,omitempty"`
	Response       *ResponseObject `json:"response,omitempty"`
	Delta          string          `json:"delta,omitempty"`
	OutputIndex    int             `json:"output_index,omitempty"`
	ContentIndex   int             `json:"content_index,omitempty"`
	ItemID         string          `json:"item_id,omitempty"`
}

// ParsedResponseInput 从 Responses API input 解析出的文本与指令。
type ParsedResponseInput struct {
	UserText     string
	Instructions string
}

// ParseResponseInput 解析 input（string 或 message 数组），兼容官方 SDK 的结构化 content。
func ParseResponseInput(raw json.RawMessage) (*ParsedResponseInput, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return &ParsedResponseInput{UserText: s}, nil
	}

	var msgs []ResponseInputMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, fmt.Errorf("invalid input: must be string or message array")
	}

	result := &ParsedResponseInput{}
	var instructionParts []string
	for _, msg := range msgs {
		text, err := parseResponseMessageContent(msg.Content)
		if err != nil {
			return nil, err
		}
		if text == "" {
			continue
		}
		switch msg.Role {
		case "developer", "system":
			instructionParts = append(instructionParts, text)
		}
	}
	if len(instructionParts) > 0 {
		result.Instructions = joinNonEmpty(instructionParts, "\n")
	}

	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		text, err := parseResponseMessageContent(msgs[i].Content)
		if err != nil {
			return nil, err
		}
		if text != "" {
			result.UserText = text
			return result, nil
		}
	}
	return nil, fmt.Errorf("no user message in input")
}

func parseResponseMessageContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	var parts []ResponseInputContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("invalid message content: must be string or content part array")
	}

	var texts []string
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text", "":
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
	}
	return joinNonEmpty(texts, "\n"), nil
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += part
	}
	return out
}

// NewResponseSnapshot 流式早期事件用的 response 快照（空 output，pi-ai 只依赖增量事件）。
func NewResponseSnapshot(id string, createdAt int64, model, status string) ResponseObject {
	return ResponseObject{
		ID:        id,
		Object:    "response",
		CreatedAt: createdAt,
		Model:     model,
		Status:    status,
		Output:    []OutputItem{},
	}
}

func NewResponseObject(id string, createdAt int64, model, status, outputText string, usage *ResponseUsage) ResponseObject {
	msgID := "msg_" + id[5:]
	if len(id) < 5 {
		msgID = "msg_" + id
	}
	return ResponseObject{
		ID:         id,
		Object:     "response",
		CreatedAt:  createdAt,
		Model:      model,
		Status:     status,
		OutputText: outputText,
		Output: []OutputItem{
			{
				Type:   "message",
				ID:     msgID,
				Role:   "assistant",
				Status: status,
				Content: []OutputContentPart{
					{Type: "output_text", Text: outputText},
				},
			},
		},
		Usage: usage,
	}
}
