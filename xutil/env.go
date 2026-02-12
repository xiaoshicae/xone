package xutil

import (
	"os"
	"strings"
)

const (
	DebugKey       = "XONE_ENABLE_DEBUG"
	legacyDebugKey = "SERVER_ENABLE_DEBUG"
)

// EnableXOneDebug 是否启用debug模式，用于xone启动过程中的日志记录
func EnableXOneDebug() bool {
	raw := strings.TrimSpace(os.Getenv(DebugKey))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(legacyDebugKey))
	}
	switch strings.ToLower(raw) {
	case "true", "1", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}
