package types

import "encoding/json"

// Chat Completions API 入站/出站 wire 类型（OpenAI 兼容 JSON 格式）。
// 官方 SDK github.com/openai/openai-go/v3 面向客户端调用，proxy 侧保留 wire 类型以便 Bind 与序列化。

const (
	ChatMessageRoleAssistant = "assistant"
)

type FinishReason string

const (
	FinishReasonStop FinishReason = "stop"
	FinishReasonNull FinishReason = "null"
)

func (r FinishReason) MarshalJSON() ([]byte, error) {
	if r == FinishReasonNull || r == "" {
		return []byte("null"), nil
	}
	return json.Marshal(string(r))
}

type ContentFilterResults struct{}

type ChatMessageImageURL struct {
	URL    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type ChatMessagePart struct {
	Type     string               `json:"type,omitempty"`
	Text     string               `json:"text,omitempty"`
	ImageURL *ChatMessageImageURL `json:"image_url,omitempty"`
}

type ChatCompletionMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content,omitempty"`
	MultiContent     []ChatMessagePart `json:"-"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
}

func (m *ChatCompletionMessage) UnmarshalJSON(bs []byte) error {
	msg := struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
		MultiContent     []ChatMessagePart
	}{}
	if err := json.Unmarshal(bs, &msg); err == nil {
		m.Role = msg.Role
		m.Content = msg.Content
		m.ReasoningContent = msg.ReasoningContent
		m.MultiContent = msg.MultiContent
		return nil
	}
	multiMsg := struct {
		Role             string            `json:"role"`
		Content          string            `json:"-"`
		ReasoningContent string            `json:"reasoning_content,omitempty"`
		MultiContent     []ChatMessagePart `json:"content"`
	}{}
	if err := json.Unmarshal(bs, &multiMsg); err != nil {
		return err
	}
	m.Role = multiMsg.Role
	m.Content = multiMsg.Content
	m.ReasoningContent = multiMsg.ReasoningContent
	m.MultiContent = multiMsg.MultiContent
	return nil
}

type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason FinishReason          `json:"finish_reason"`
}

type ChatCompletionResponse struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"`
	Created           int64                  `json:"created"`
	Model             string                 `json:"model"`
	Choices           []ChatCompletionChoice `json:"choices"`
	Usage             Usage                  `json:"usage"`
	SystemFingerprint string                 `json:"system_fingerprint"`
}

type ChatCompletionStreamChoiceDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type ChatCompletionStreamChoice struct {
	Index                int                             `json:"index"`
	Delta                ChatCompletionStreamChoiceDelta `json:"delta"`
	FinishReason         FinishReason                    `json:"finish_reason"`
	ContentFilterResults ContentFilterResults            `json:"content_filter_results,omitempty"`
}

type ChatCompletionStreamResponse struct {
	ID                string                       `json:"id"`
	Object            string                       `json:"object"`
	Created           int64                        `json:"created"`
	Model             string                       `json:"model"`
	Choices           []ChatCompletionStreamChoice `json:"choices"`
	SystemFingerprint string                       `json:"system_fingerprint"`
	Usage             *Usage                       `json:"usage,omitempty"`
}

func (req ChatCompletionRequest) PromptTokens(tokenCounter func(string) int) int {
	promptTokens := 0
	for _, message := range req.Messages {
		promptTokens += tokenCounter(message.Role)
		promptTokens += tokenCounter(message.Content)
	}
	return promptTokens
}

func (req ChatCompletionRequest) BuildUsage(completionText string, tokenCounter func(string) int) Usage {
	promptTokens := req.PromptTokens(tokenCounter)
	completionTokens := tokenCounter(completionText)
	return Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}
