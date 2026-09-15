package xflow

import (
	"sync/atomic"

	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xutil"
)

// 运行时生效的配置，原子读写
//
// 在 BeforeStart 阶段一次性写入，而非首次 Execute 时惰性读取：
// 惰性读取一旦发生在 xconfig 就绪之前，读到的零值会被永久缓存，
// 用户配置从此不再生效。
var (
	monitorEnabled       atomic.Bool
	rollbackTimeoutNanos atomic.Int64
)

func init() {
	applyConfig(configMergeDefault(nil))
	xhook.BeforeStart(initXFlow)
}

func initXFlow() error {
	c, err := getConfig()
	if err != nil {
		return xerror.Newf("xflow", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initXFlow got config: %s", xutil.ToJsonString(c))

	applyConfig(c)
	return nil
}

// applyConfig 把配置写入运行时变量
func applyConfig(c *Config) {
	monitorEnabled.Store(*c.EnableMonitor)
	rollbackTimeoutNanos.Store(int64(xutil.ToDuration(c.RollbackTimeout)))
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XFlowConfigKey, c); err != nil {
		return nil, err
	}
	return configMergeDefault(c), nil
}
