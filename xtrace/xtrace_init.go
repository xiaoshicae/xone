package xtrace

import (
	"context"
	"fmt"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const defaultShutdownTimeout = 5 * time.Second

var (
	xTraceShutdownFunc func() error
)

func init() {
	xhook.BeforeStart(initXTrace)
	xhook.BeforeStop(shutdownXTrace)
}

func initXTrace() error {
	c, err := getConfig()
	if err != nil {
		return fmt.Errorf("XOne initXTrace getConfig failed, err=[%v]", err)
	}

	if c.Enable != nil && !*c.Enable {
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		xutil.InfoIfEnableDebug("XOne initXTrace ignored, because of config XTrace.Enable=false")
		return nil
	}

	serviceName := xconfig.GetServerName()
	serviceVersion := xconfig.GetServerVersion()

	xutil.InfoIfEnableDebug("XOne initXTrace got param: ServiceName:%s, ServiceVersion:%s", serviceName, serviceVersion)

	return initXTraceByConfig(c, serviceName, serviceVersion)
}

func initXTraceByConfig(c *Config, serviceName, serviceVersion string) error {
	r, err := resource.New(
		context.Background(),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			attribute.String("serviceName", serviceName),
			attribute.String("serviceVersion", serviceVersion),
		),
	)
	if err != nil {
		return fmt.Errorf("XOne initXTraceByConfig invoke resource.New failed, err=[%v]", err)
	}

	tpOpts := []trace.TracerProviderOption{
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(r),
	}
	if c.Console {
		exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return fmt.Errorf("XOne initXTraceByConfig init exporter failed, err=[%v]", err)
		}
		tpOpts = append(tpOpts, trace.WithBatcher(exporter))
	}

	tp := trace.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)

	xTraceShutdownFunc = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()
		return tp.Shutdown(ctx)
	}

	return nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XTraceConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}

func shutdownXTrace() error {
	fn := xTraceShutdownFunc
	xTraceShutdownFunc = nil
	if fn == nil {
		return nil
	}
	return fn()
}
