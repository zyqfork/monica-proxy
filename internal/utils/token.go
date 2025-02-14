package utils

import (
	"fmt"
	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

func CalculateTokens(text string) int {
	encoding := "cl100k_base"
	tke, err := tiktoken.GetEncoding(encoding)
	if err != nil {
		err = fmt.Errorf("getEncoding: %v", err)
		return 0
	}
	token := tke.Encode(text, nil, nil)
	return len(token)
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
