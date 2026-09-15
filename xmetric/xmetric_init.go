package xmetric

import (
	"sync"

	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xlog"
	"github.com/xiaoshicae/xone/v3/xutil"

	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// logHookOnce 确保日志观察者只注册一次
//
// xlog 的观察者注册后不可撤销，因此这个 Once 不随 closeMetric 重置：
// 重置会让重新初始化时重复注册，同一条错误日志被计数多次。
var logHookOnce sync.Once

func init() {
	xhook.BeforeStart(initMetric)
	xhook.BeforeStop(closeMetric)
}

func initMetric() error {
	c, err := getConfig()
	if err != nil {
		return xerror.Newf("xmetric", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initMetric got config: %s", xutil.ToJsonString(c))

	// 设置全局配置，必须先于创建 collector：Namespace / ConstLabels 在创建时读取
	registryMu.Lock()
	metricConfig = c
	metricsHandler = promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	registryMu.Unlock()

	// 注册 Go runtime 和进程指标
	//
	// 用 safeRegister 而非 MustRegister：后者在重复初始化时直接 panic
	// （duplicate metrics collector registration attempted）。
	if *c.EnableGoMetrics {
		safeRegister(promcollectors.NewGoCollector())
	}
	if *c.EnableProcessMetrics {
		safeRegister(promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}))
	}

	// 注册日志观察者（Error 自动上报 metric），只注册一次
	if *c.EnableLogErrorMetric {
		logHookOnce.Do(func() {
			xlog.AddObserver(newMetricLogObserver(c.Namespace).observe)
		})
	}

	return nil
}

func closeMetric() error {
	registryMu.Lock()
	metricsHandler = nil
	metricConfig = nil
	registryMu.Unlock()

	// 清空 collector 缓存：留着的话，重新初始化后新的 Namespace / ConstLabels
	// 对已缓存的指标不生效，指标名会一直沿用旧配置
	resetCollectors()
	return nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XMetricConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}
