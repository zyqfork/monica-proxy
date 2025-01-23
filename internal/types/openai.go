package types

import "github.com/sashabaranov/go-openai"

type ChatCompletionStreamResponse struct {
	ID                  string                       `json:"id"`
	Object              string                       `json:"object"`
	Created             int64                        `json:"created"`
	Model               string                       `json:"model"`
	Choices             []ChatCompletionStreamChoice `json:"choices"`
	SystemFingerprint   string                       `json:"system_fingerprint"`
	PromptAnnotations   []openai.PromptAnnotation    `json:"prompt_annotations,omitempty"`
	PromptFilterResults []openai.PromptFilterResult  `json:"prompt_filter_results,omitempty"`
	Usage               *openai.Usage                `json:"usage,omitempty"`
}

type ChatCompletionStreamChoice struct {
	Index        int                                        `json:"index"`
	Delta        openai.ChatCompletionStreamChoiceDelta     `json:"delta"`
	Logprobs     *openai.ChatCompletionStreamChoiceLogprobs `json:"logprobs,omitempty"`
	FinishReason openai.FinishReason                        `json:"finish_reason"`
}
