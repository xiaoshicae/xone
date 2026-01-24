package xhttp

import (
	"context"
	"net/http"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/go-resty/resty/v2"
	c "github.com/smartystreets/goconvey/convey"
)

func TestXHttpConfig(t *testing.T) {
	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Timeout:             "60s",
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
