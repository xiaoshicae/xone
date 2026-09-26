// Package coreonly 只 import 根包的程序：单独一个测试二进制，登记表里只有框架自己带来的钩子
package coreonly

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoshicae/xone"
)

func TestRun_XAppWorksWithCoreOnly(t *testing.T) {
	// XApp 块跟着框架一起来：没 import xapp 的程序写了 XApp.Name，也不该被当成没人读的 key
	cfg := filepath.Join(t.TempDir(), "application.yml")
	if err := os.WriteFile(cfg, []byte("XApp:\n  Name: core.only\n  Version: v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := xone.Run(xone.Func(func(context.Context) error { return nil }),
		xone.WithConfigPath(cfg), xone.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatalf("只 import 根包、写了 XApp 也该正常启动：%v", err)
	}
}
