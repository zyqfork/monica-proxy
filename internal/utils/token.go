package utils

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

var (
	tiktokenOnce sync.Once
	cachedTke    *tiktoken.Tiktoken
	tiktokenErr  error
)

func getTiktokenEncoding() (*tiktoken.Tiktoken, error) {
	tiktokenOnce.Do(func() {
		tke, err := tiktoken.GetEncoding("cl100k_base")
		cachedTke, tiktokenErr = tke, err
	})
	return cachedTke, tiktokenErr
}

func CalculateTokens(text string) int {
	tke, err := getTiktokenEncoding()
	if err != nil {
		return 0
	}
	tokens := tke.Encode(text, nil, nil)
	return len(tokens)
}

// 简单计算message里面的token数量
func CalculatePromptTokens(req openai.ChatCompletionRequest) int {
	var promptTokens int = 0
	for _, message := range req.Messages {
		promptTokens += CalculateTokens(message.Role)
		promptTokens += CalculateTokens(message.Content)
	}
	return promptTokens
}

func CalculateUsage(req openai.ChatCompletionRequest, completionText string) openai.Usage {
	promptTokens := CalculatePromptTokens(req)
	completionTokens := CalculateTokens(completionText)
	return openai.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}
