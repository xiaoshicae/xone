package xlog

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// setupBenchLogger 将日志输出重定向到 io.Discard，排除终端 I/O 对测量的干扰
func setupBenchLogger(consoleRaw bool, withFile bool) {
	var fw io.Writer
	if withFile {
		fw = io.Discard
	}
	logger.ReplaceHooks(logrus.LevelHooks{})
	logger.SetOutput(io.Discard)
	logger.SetFormatter(nopFormatter{})
	logger.SetLevel(logrus.InfoLevel)
	logger.AddHook(&xLogHook{
		ServerName:     "bench",
		IP:             "10.0.0.1",
		PidStr:         "1",
		SuffixToIgnore: findFrameIgnoreFileNames,
		jsonFormatter:  &logrus.JSONFormatter{TimestampFormat: consoleTimeLayout},
		location:       time.UTC,
		consoleWriter:  io.Discard,
		consoleRaw:     consoleRaw,
		fileWriter:     fw,
	})
}

// BenchmarkInfo_ConsoleOnly 默认配置：仅控制台、可读格式（无需 JSON 序列化）
func BenchmarkInfo_ConsoleOnly(b *testing.B) {
	setupBenchLogger(false, false)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info(ctx, "user login success, userID=[%d]", 12345)
	}
}

// BenchmarkInfo_ConsoleAndFile 控制台 + 文件，验证 JSON 只序列化一次
func BenchmarkInfo_ConsoleAndFile(b *testing.B) {
	setupBenchLogger(true, true)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info(ctx, "user login success, userID=[%d]", 12345)
	}
}

// BenchmarkInfo_WithKV 带自定义 KV 字段
func BenchmarkInfo_WithKV(b *testing.B) {
	setupBenchLogger(false, false)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info(ctx, "order created", KV("orderId", "12345"), KV("amount", 99.9))
	}
}

// BenchmarkDebug_Disabled 级别未开启时应尽早返回，几乎无开销
func BenchmarkDebug_Disabled(b *testing.B) {
	setupBenchLogger(false, false)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Debug(ctx, "user login success, userID=[%d]", 12345)
	}
}
