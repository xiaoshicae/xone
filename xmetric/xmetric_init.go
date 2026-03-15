package xmetric

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
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
	if *c.EnableGoMetrics {
		safeRegister(promcollectors.NewGoCollector())
	}
	if *c.EnableProcessMetrics {
		safeRegister(promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}))
	}

	// 设置全局配置
	registryMu.Lock()
	metricConfig = c
	metricsHandler = promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	registryMu.Unlock()

	// 注册 logrus hook（Error 自动上报 metric），只注册一次
	if *c.EnableLogErrorMetric {
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
	defaultRegistry = prometheus.NewRegistry()
	// 重置 clientMetricOnce，允许重新初始化
	clientMetricOnce = sync.Once{}
	clientRequestsTotal = nil
	clientRequestDuration = nil
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
