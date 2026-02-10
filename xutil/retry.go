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

// RetryWithBackoff 指数退避重试
// delay 从 initialDelay 开始，每次翻倍，不超过 maxDelay
func RetryWithBackoff(fn func() error, attempts int, initialDelay, maxDelay time.Duration) (err error) {
	if attempts <= 0 {
		return fn()
	}
	delay := initialDelay
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i+1 < attempts && delay > 0 {
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
	return err
}
