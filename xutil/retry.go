package xutil

import (
	"context"
	"time"
)

// Retry 重试函数
//
// attempts 是总调用次数而非额外重试次数：Retry(fn, 3, d) 最多调用 fn 三次，
// 失败两次后再试一次，不是四次。attempts <= 0 时按 1 处理，即只执行一次。
// 每两次调用之间等待 sleep，最后一次失败后不再等待。
func Retry(fn func() error, attempts int, sleep time.Duration) error {
	return RetryWithContext(context.Background(), func(context.Context) error { return fn() }, attempts, sleep)
}

// RetryWithContext 可取消的重试
//
// ctx 结束时立即返回 ctx.Err()：重试常出现在初始化路径上，
// attempts×sleep 可能长达数十秒，服务关闭时不该被它硬拖住。
func RetryWithContext(ctx context.Context, fn func(context.Context) error, attempts int, sleep time.Duration) error {
	return retry(ctx, fn, attempts, func(time.Duration) time.Duration { return sleep }, sleep)
}

// RetryWithBackoff 指数退避重试
//
// delay 从 initialDelay 开始，每次翻倍，不超过 maxDelay。
// attempts 同 Retry，是总调用次数。
func RetryWithBackoff(fn func() error, attempts int, initialDelay, maxDelay time.Duration) error {
	return RetryWithBackoffContext(context.Background(),
		func(context.Context) error { return fn() }, attempts, initialDelay, maxDelay)
}

// RetryWithBackoffContext 可取消的指数退避重试
func RetryWithBackoffContext(ctx context.Context, fn func(context.Context) error, attempts int, initialDelay, maxDelay time.Duration) error {
	return retry(ctx, fn, attempts, func(d time.Duration) time.Duration {
		return nextBackoff(d, maxDelay)
	}, initialDelay)
}

// retry 重试主循环，next 决定下一次的等待时长
func retry(ctx context.Context, fn func(context.Context) error, attempts int, next func(time.Duration) time.Duration, delay time.Duration) error {
	if attempts <= 0 {
		attempts = 1
	}

	var err error
	for i := 0; i < attempts; i++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if err != nil {
				return err // 已经失败过就保留业务错误，它比"被取消"更有信息量
			}
			return ctxErr
		}

		if err = fn(ctx); err == nil {
			return nil // 成功则立即返回
		}

		if i+1 >= attempts || delay <= 0 {
			continue
		}
		if !sleepWithContext(ctx, delay) {
			return err
		}
		delay = next(delay)
	}
	return err // 重试 attempts 次后仍然失败，返回最后的错误
}

// nextBackoff 计算下一次退避时长
//
// 先与上限比较再翻倍：time.Duration 是 int64 纳秒，
// 先翻倍会在二十来次后溢出成负数，此后 delay > 0 不再成立，退避彻底失效。
func nextBackoff(delay, maxDelay time.Duration) time.Duration {
	if maxDelay > 0 && delay >= maxDelay/2 {
		return maxDelay
	}
	if delay > (1<<62)/2 { // 无上限时防止溢出
		return delay
	}
	return delay * 2
}

// sleepWithContext 等待指定时长，ctx 先结束则返回 false
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
