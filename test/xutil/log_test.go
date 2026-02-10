package xutil

import (
	"testing"

	"github.com/xiaoshicae/xone/v2/xutil"
)

func TestLog(t *testing.T) {
	frame := xutil.GetLogCaller(0, nil)
	t.Log(frame)
}
