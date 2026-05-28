package utils

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
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
