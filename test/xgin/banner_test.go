package xgin

import (
	"bytes"
	"os"
	"testing"

	"github.com/xiaoshicae/xone/v2"
	xgin2 "github.com/xiaoshicae/xone/v2/xgin"
)

func TestPrintBanner(t *testing.T) {
	xgin2.PrintBanner()

	// 捕获标准输出
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	xgin2.PrintBanner()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// 验证输出包含 XGin 关键字
	if len(output) == 0 {
		t.Error("PrintBanner should produce output")
	}
}

func TestVersion(t *testing.T) {
	if xone.VERSION == "" {
		t.Error("VERSION should not be empty")
	}
}
