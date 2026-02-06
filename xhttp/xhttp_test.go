package xhttp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xtrace"

	"github.com/bytedance/mockey"
	"github.com/go-resty/resty/v2"
	c "github.com/smartystreets/goconvey/convey"
)

type stubRoundTripper struct{}

func (s *stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("stub transport")
}

func TestXHttpConfig(t *testing.T) {
	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			RetryWaitTime:       "100ms",
			RetryMaxWaitTime:    "2s",
		})
	})

	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-NotNil", t, func() {
		config := &Config{
			Timeout: "1",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Timeout:             "1",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			RetryWaitTime:       "100ms",
			RetryMaxWaitTime:    "2s",
		})
	})
}

func TestXHttpClient(t *testing.T) {
	mockey.PatchConvey("TestXHttpClient", t, func() {
		mockey.PatchConvey("TestXHttpClient-NotFound", func() {
			client := C()
			c.So(client, c.ShouldNotBeNil)
		})
	})
}

func TestRWithCtx(t *testing.T) {
	mockey.PatchConvey("TestRWithCtx", t, func() {
		ctx := context.Background()
		req := RWithCtx(ctx)
		c.So(req, c.ShouldNotBeNil)
	})
}

func TestRawClient(t *testing.T) {
	mockey.PatchConvey("TestRawClient-NotSet", t, func() {
		// Reset rawHttpClient to nil
		rawHttpClient = nil
		client := RawClient()
		c.So(client, c.ShouldNotBeNil)
		c.So(client, c.ShouldEqual, http.DefaultClient)
	})

	mockey.PatchConvey("TestRawClient-Set", t, func() {
		customClient := &http.Client{}
		setRawHttpClient(customClient)
		client := RawClient()
		c.So(client, c.ShouldNotBeNil)
		c.So(client, c.ShouldEqual, customClient)
		// Clean up
		rawHttpClient = nil
	})
}

func TestSetDefaultClient(t *testing.T) {
	mockey.PatchConvey("TestSetDefaultClient", t, func() {
		original := defaultClient
		newClient := resty.New()
		setDefaultClient(newClient)
		c.So(C(), c.ShouldEqual, newClient)
		// Restore
		defaultClient = original
	})
}

func TestSetRawHttpClient(t *testing.T) {
	mockey.PatchConvey("TestSetRawHttpClient", t, func() {
		customClient := &http.Client{}
		setRawHttpClient(customClient)
		c.So(rawHttpClient, c.ShouldEqual, customClient)
		// Clean up
		rawHttpClient = nil
	})
}

func TestGetConfigXHttp(t *testing.T) {
	mockey.PatchConvey("TestGetConfig-UnmarshalFail", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		config, err := getConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(config, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetConfig-Success", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()

		config, err := getConfig()
		c.So(err, c.ShouldBeNil)
		c.So(config, c.ShouldNotBeNil)
		c.So(config.Timeout, c.ShouldEqual, "60s") // 默认值
	})
}

func TestInitHttpClient(t *testing.T) {
	mockey.PatchConvey("TestInitHttpClient-GetConfigFail", t, func() {
		mockey.Mock(getConfig).Return(nil, errors.New("config failed")).Build()

		err := initHttpClient()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
	})

	mockey.PatchConvey("TestInitHttpClient-Success-NoTrace", t, func() {
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-Success-WithTrace", t, func() {
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(true).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-Success-WithRetry", t, func() {
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          3,
			RetryWaitTime:       "100ms",
			RetryMaxWaitTime:    "2s",
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-Success-WithCustomDialTimeout", t, func() {
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "5s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient, c.ShouldNotBeNil)
		c.So(rawHttpClient.Transport, c.ShouldNotBeNil)
	})
}

func TestInitHttpClientDefaultTransportFallback(t *testing.T) {
	mockey.PatchConvey("TestInitHttpClient-DefaultTransportFallback", t, func() {
		origTransport := http.DefaultTransport
		stub := &stubRoundTripper{}
		http.DefaultTransport = stub
		prevRawClient := rawHttpClient
		prevDefaultClient := defaultClient
		defer func() {
			http.DefaultTransport = origTransport
			rawHttpClient = prevRawClient
			defaultClient = prevDefaultClient
		}()

		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient.Transport, c.ShouldEqual, stub)
	})
}

// TestDialTimeoutApplied 验证 DialTimeout 和 DialKeepAlive 配置是否正确应用到 Transport
func TestDialTimeoutApplied(t *testing.T) {
	mockey.PatchConvey("TestDialTimeout-Applied", t, func() {
		prevRawClient := rawHttpClient
		prevDefaultClient := defaultClient
		defer func() {
			rawHttpClient = prevRawClient
			defaultClient = prevDefaultClient
		}()

		// 设置自定义 DialTimeout 和 DialKeepAlive
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "5s",
			DialKeepAlive:       "15s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)

		// 验证 rawHttpClient 已创建
		c.So(rawHttpClient, c.ShouldNotBeNil)

		// 验证 Transport 已设置
		transport, ok := rawHttpClient.Transport.(*http.Transport)
		c.So(ok, c.ShouldBeTrue)
		c.So(transport, c.ShouldNotBeNil)

		// 验证 DialContext 已设置（不为 nil）
		c.So(transport.DialContext, c.ShouldNotBeNil)

		// 验证其他 Transport 配置
		c.So(transport.MaxIdleConns, c.ShouldEqual, 100)
		c.So(transport.MaxIdleConnsPerHost, c.ShouldEqual, 10)
		c.So(transport.IdleConnTimeout, c.ShouldEqual, 90*time.Second)
	})
}

// TestDialConfigMerge 验证 DialTimeout 和 DialKeepAlive 配置合并逻辑
func TestDialConfigMerge(t *testing.T) {
	mockey.PatchConvey("TestDialConfig-ConfigMerge-Empty", t, func() {
		config := configMergeDefault(&Config{})
		c.So(config.DialTimeout, c.ShouldEqual, "30s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "30s")
	})

	mockey.PatchConvey("TestDialConfig-ConfigMerge-CustomTimeout", t, func() {
		config := configMergeDefault(&Config{DialTimeout: "5s"})
		c.So(config.DialTimeout, c.ShouldEqual, "5s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "30s")
	})

	mockey.PatchConvey("TestDialConfig-ConfigMerge-CustomKeepAlive", t, func() {
		config := configMergeDefault(&Config{DialKeepAlive: "15s"})
		c.So(config.DialTimeout, c.ShouldEqual, "30s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "15s")
	})

	mockey.PatchConvey("TestDialConfig-ConfigMerge-CustomBoth", t, func() {
		config := configMergeDefault(&Config{DialTimeout: "5s", DialKeepAlive: "10s"})
		c.So(config.DialTimeout, c.ShouldEqual, "5s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "10s")
	})
}
