package xutil

import (
	"time"
)

// Retry 重试函数
func Retry(fn func() error, attempts int, sleep time.Duration) (err error) {
	if attempts <= 0 {
		return fn()
	}
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil // 成功则立即返回
		}
		if i+1 < attempts && sleep > 0 {
			time.Sleep(sleep)
		}
	}
	return err // 重试 attempts 次后仍然失败，返回最后的错误
}
