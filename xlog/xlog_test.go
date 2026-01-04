package xlog

import (
	"testing"

	"github.com/xiaoshicae/xone/xconfig"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

func TestXLogConfig(t *testing.T) {
	mockey.PatchConvey("TestXLogConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Level:              "info",
			Name:               "app",
			Path:               "./log",
			Console:            false,
			ConsoleFormatIsRaw: false,
			MaxAge:             "7d",
			RotateTime:         "1d",
			Timezone:           "Asia/Shanghai",
		})
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-NotNil", t, func() {
		mockey.Mock(xconfig.GetServerName).Return("a.b.c").Build()
		config := &Config{
			Level:              "1",
			Name:               "2",
			Path:               "3",
			Console:            true,
			ConsoleFormatIsRaw: true,
			MaxAge:             "4",
			RotateTime:         "5",
			Timezone:           "UTC",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Level:              "1",
			Name:               "2",
			Path:               "3",
			Console:            true,
			ConsoleFormatIsRaw: true,
			MaxAge:             "4",
			RotateTime:         "5",
			Timezone:           "UTC",
		})
	})
}
