package xmetric

import (
	"sync"

	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// logHookOnce 确保 logrus hook 只注册一次（logrus.AddHook 不可撤销）
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

	// 注册 Go runtime 和进程指标
	if enableGoMetrics() {
		defaultRegistry.MustRegister(promcollectors.NewGoCollector())
	}
	if enableProcessMetrics() {
		defaultRegistry.MustRegister(promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}))
	}

	// 设置全局配置
	registryMu.Lock()
	namespace = c.Namespace
	histBuckets = c.HistogramBuckets
	metricConfig = c
	metricsHandler = promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	registryMu.Unlock()

	// 注册 logrus hook（Error 自动上报 metric），只注册一次
	if enableLogErrorMetric() {
		logHookOnce.Do(func() {
			logrus.AddHook(newMetricLogHook(c.Namespace))
		})
	}

	return nil
}

func closeMetric() error {
	registryMu.Lock()
	defer registryMu.Unlock()
	metricsHandler = nil
	metricConfig = nil
	namespace = ""
	histBuckets = nil
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

// enableGoMetrics 是否启用 Go runtime 指标，默认 true
func enableGoMetrics() bool {
	key := XMetricConfigKey + ".EnableGoMetrics"
	if !xconfig.ContainKey(key) {
		return true
	}
	return xconfig.GetBool(key)
}

// enableProcessMetrics 是否启用进程指标，默认 true
func enableProcessMetrics() bool {
	key := XMetricConfigKey + ".EnableProcessMetrics"
	if !xconfig.ContainKey(key) {
		return true
	}
	return xconfig.GetBool(key)
}

// enableLogErrorMetric 是否启用 xlog.Error 自动上报 metric，默认 true
func enableLogErrorMetric() bool {
	key := XMetricConfigKey + ".EnableLogErrorMetric"
	if !xconfig.ContainKey(key) {
		return true
	}
	return xconfig.GetBool(key)
}
