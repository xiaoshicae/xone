package xutil

import (
	"os"
	"strings"
)

const (
	DebugKey = "SERVER_ENABLE_DEBUG"
)

// EnableDebug 是否启用debug模式，用于xone启动过程中的日志记录
func EnableDebug() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(DebugKey))) {
	case "true", "1", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}
