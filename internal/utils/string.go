package utils

import (
	"math/rand/v2"
)

// RandStringUsingMathRand 生成指定长度的随机字符串
// 使用 math/rand/v2 减少锁竞争，提升高并发下的性能
func RandStringUsingMathRand(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	result := make([]byte, n)
	for i := range result {
		result[i] = letters[rand.IntN(len(letters))]
	}
	return string(result)
}
