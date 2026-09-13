package xconfig

import (
	"os"
	"testing"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xserver"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestXConfig(t *testing.T) {
	t.Skip("集成测试，需手动运行")
	PatchConvey("TestXConfig", t, func() {
		if err := xserver.Init(); err != nil {
			panic(err)
		}

		So(os.Getenv("x"), ShouldEqual, "123")
		So(os.Getenv("y"), ShouldEqual, "456")
		So(xconfig.GetConfig("A.B.C"), ShouldEqual, "a.b.c")
		So(xconfig.GetConfig("A.B.D"), ShouldEqual, "123")
		So(xconfig.GetConfig("A.B.E"), ShouldEqual, "456")
		So(xconfig.GetConfig("A.B.F"), ShouldEqual, "789")
	})
}

const configKey = "MyApp"

// RouteConfig 路由配置，用于验证 map 类型字段的反序列化
// 注意：mapstructure tag 必须使用小写，因为 viper 内部将所有 key 转为小写存储
type RouteConfig struct {
	Endpoint         string   `mapstructure:"endpoint"`
	Aliases          []string `mapstructure:"aliases"`
	StreamingEnabled bool     `mapstructure:"streamingenabled"`
}

// Config 业务自定义配置示例（使用嵌套匿名结构体）
type Config struct {
	Backend struct {
		URL            string `mapstructure:"url"`
		APIKey         string `mapstructure:"apikey"`
		RequestTimeout string `mapstructure:"requesttimeout"`
	} `mapstructure:"backend"`
	Syncer struct {
		Interval       string `mapstructure:"interval"`
		RequestTimeout string `mapstructure:"requesttimeout"`
		ItemInfoPath   string `mapstructure:"iteminfopath"`
		UserInfoPath   string `mapstructure:"userinfopath"`
	} `mapstructure:"syncer"`
	Routes map[string]RouteConfig `mapstructure:"routes"`
}

var cfg Config

func TestLoadConfig(t *testing.T) {
	t.Skip("集成测试，需手动运行")
	if err := xserver.Init(); err != nil {
		t.Fatal(err)
	}

	// 检查原始配置结构
	t.Log("=== 原始配置 ===")
	raw := xconfig.GetConfig(configKey)
	t.Logf("Raw type: %T", raw)
	t.Logf("Raw value: %+v", raw)

	// 检查 Backend 子配置
	backend := xconfig.GetConfig(configKey + ".Backend")
	t.Logf("Backend type: %T", backend)
	t.Logf("Backend value: %+v", backend)

	// 单独获取各字段（APIKey 属敏感配置，不打印内容，仅确认能读到）
	t.Log("=== 单独字段 ===")
	t.Logf("Backend.URL: %v", xconfig.GetString(configKey+".Backend.URL"))
	t.Logf("Backend.APIKey is set: %v", xconfig.GetString(configKey+".Backend.APIKey") != "")
	t.Logf("Backend.RequestTimeout: %v", xconfig.GetString(configKey+".Backend.RequestTimeout"))

	// UnmarshalConfig
	t.Log("=== UnmarshalConfig ===")
	err := xconfig.UnmarshalConfig(configKey, &cfg)
	t.Log("err:", err)
	t.Logf("cfg: %+v", cfg)
}
