package xtrace

import (
	"strings"

	"github.com/xiaoshicae/xone/v2/xconfig"
)

const (
	XTraceEnableKey = "XTrace.Enable"
)

func EnableTrace() bool {
	enable := strings.TrimSpace(xconfig.GetString(XTraceEnableKey))
	return strings.ToLower(enable) != "false" // 需要明确配置false才会关闭trace
}
