package xhttp

import (
	"testing"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

func TestXHttpConfig(t *testing.T) {
	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Timeout: "60s",
		})
	})

	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-NotNil", t, func() {
		config := &Config{
			Timeout: "1",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Timeout: "1",
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
