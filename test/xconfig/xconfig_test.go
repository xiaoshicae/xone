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
	PatchConvey("TestXConfig", t, func() {
		if err := xserver.R(); err != nil {
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

const configKey = "LLMTrainerGateway"

// ModelRouteConfig 模型路由配置
// 注意：mapstructure tag 必须使用小写，因为 viper 内部将所有 key 转为小写存储
type ModelRouteConfig struct {
	Endpoint             string   `mapstructure:"endpoint"`
	WSEndpoint           string   `mapstructure:"wsendpoint"`
	APIKey               string   `mapstructure:"apikey"`
	Aliases              []string `mapstructure:"aliases"`
	StreamingEnabled     bool     `mapstructure:"streamingenabled"`
	CFAccessClientID     string   `mapstructure:"cfaccessclientid"`
	CFAccessClientSecret string   `mapstructure:"cfaccessclientsecret"`
}

// Config 配置（使用嵌套匿名结构体）
type Config struct {
	Backend struct {
		URL            string `mapstructure:"url"`
		APIKey         string `mapstructure:"apikey"`
		RequestTimeout string `mapstructure:"requesttimeout"`
	} `mapstructure:"backend"`
	Syncer struct {
		Interval       string `mapstructure:"interval"`
		RequestTimeout string `mapstructure:"requesttimeout"`
		ModelInfoPath  string `mapstructure:"modelinfopath"`
		UserInfoPath   string `mapstructure:"userinfopath"`
	} `mapstructure:"syncer"`
	ModelRoutes map[string]ModelRouteConfig `mapstructure:"modelroutes"`
}

var cfg Config

func TestLoadConfig(t *testing.T) {
	if err := xserver.R(); err != nil {
		t.Fatal(err)
	}

	// 检查原始配置结构
	t.Log("=== 原始配置 ===")
	raw := xconfig.GetConfig("LLMTrainerGateway")
	t.Logf("Raw type: %T", raw)
	t.Logf("Raw value: %+v", raw)

	// 检查 Backend 子配置
	backend := xconfig.GetConfig("LLMTrainerGateway.Backend")
	t.Logf("Backend type: %T", backend)
	t.Logf("Backend value: %+v", backend)

	// 单独获取各字段
	t.Log("=== 单独字段 ===")
	t.Logf("Backend.URL: %v", xconfig.GetString("LLMTrainerGateway.Backend.URL"))
	t.Logf("Backend.APIKey: %v", xconfig.GetString("LLMTrainerGateway.Backend.APIKey"))
	t.Logf("Backend.RequestTimeout: %v", xconfig.GetString("LLMTrainerGateway.Backend.RequestTimeout"))

	// UnmarshalConfig
	t.Log("=== UnmarshalConfig ===")
	err := xconfig.UnmarshalConfig(configKey, &cfg)
	t.Log("err:", err)
	t.Logf("cfg: %+v", cfg)
}
