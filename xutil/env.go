package xutil

import (
	"os"
	"strings"
	"sync"
)

const (
	DebugKey       = "XONE_ENABLE_DEBUG"
	legacyDebugKey = "SERVER_ENABLE_DEBUG"
)

var (
	debugOnce  sync.Once
	debugValue bool
)

// EnableXOneDebug 是否启用debug模式，用于xone启动过程中的日志记录
// 结果在首次调用时通过 sync.Once 缓存，后续调用直接返回缓存值，运行期间不可变
// 设计意图：debug 模式仅在启动阶段确定，运行时不支持热更新
func EnableXOneDebug() bool {
	debugOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(DebugKey))
		if raw == "" {
			raw = strings.TrimSpace(os.Getenv(legacyDebugKey))
		}
		switch strings.ToLower(raw) {
		case "true", "1", "t", "yes", "y", "on":
			debugValue = true
		}
	})
	return debugValue
}
