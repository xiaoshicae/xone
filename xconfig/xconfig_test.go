package xconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/spf13/viper"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== util.go ====================

func TestCheckParam(t *testing.T) {
	PatchConvey("TestCheckParam", t, func() {
		PatchConvey("EmptyKey", func() {
			err := checkParam("", nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "key is empty")
		})

		PatchConvey("NilConf", func() {
			err := checkParam("key", nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "conf is nil")
		})

		PatchConvey("NotPtrConf", func() {
			err := checkParam("key", struct{}{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "conf is not ptr")
		})

		PatchConvey("Valid", func() {
			err := checkParam("key", &struct{}{})
			So(err, ShouldBeNil)
		})
	})
}

func TestGetViperConfig(t *testing.T) {
	PatchConvey("TestGetViperConfig", t, func() {
		origVip := vip
		defer func() { vip = origVip }()

		PatchConvey("Nil", func() {
			vip = nil
			config := getViperConfig()
			So(config, ShouldNotBeNil)
		})

		PatchConvey("NotNil", func() {
			vip = viper.New()
			vip.Set("test", "value")
			config := getViperConfig()
			So(config, ShouldNotBeNil)
			So(config.GetString("test"), ShouldEqual, "value")
		})
	})
}

func TestUnmarshalConfig(t *testing.T) {
	PatchConvey("TestUnmarshalConfig", t, func() {
		origVip := vip
		defer func() { vip = origVip }()

		PatchConvey("InvalidKey", func() {
			err := UnmarshalConfig("", &struct{}{})
			So(err, ShouldNotBeNil)
		})

		PatchConvey("UnmarshalKeyError", func() {
			vip = viper.New()
			vip.Set("test", "not_a_map")
			// 尝试反序列化到不兼容的类型
			conf := &struct {
				Sub struct {
					Key int `mapstructure:"key"`
				} `mapstructure:"sub"`
			}{}
			// 传入一个存在的 key，但类型不匹配会走到 UnmarshalKey error 分支
			Mock((*viper.Viper).UnmarshalKey).Return(errors.New("unmarshal error")).Build()
			err := UnmarshalConfig("test", conf)
			So(err, ShouldNotBeNil)
		})

		PatchConvey("Valid", func() {
			vip = viper.New()
			vip.Set("test.key", "value")
			conf := struct {
				Key string `mapstructure:"key"`
			}{}
			err := UnmarshalConfig("test", &conf)
			So(err, ShouldBeNil)
			So(conf.Key, ShouldEqual, "value")
		})
	})
}

func TestUtilFunctions(t *testing.T) {
	PatchConvey("TestUtilFunctions", t, func() {
		origVip := vip
		defer func() { vip = origVip }()

		vip = viper.New()
		vip.Set("string_key", "string_value")
		vip.Set("bool_key", true)
		vip.Set("int_key", 42)
		vip.Set("int32_key", int32(32))
		vip.Set("int64_key", int64(64))
		vip.Set("float64_key", 3.14)
		vip.Set("duration_key", "1s")
		vip.Set("string_slice_key", []string{"a", "b"})
		vip.Set("int_slice_key", []int{1, 2, 3})

		PatchConvey("GetConfig", func() {
			So(GetConfig("string_key"), ShouldEqual, "string_value")
		})

		PatchConvey("ContainKey", func() {
			So(ContainKey("string_key"), ShouldBeTrue)
			So(ContainKey("nonexistent"), ShouldBeFalse)
		})

		PatchConvey("GetString", func() {
			So(GetString("string_key"), ShouldEqual, "string_value")
		})

		PatchConvey("GetBool", func() {
			So(GetBool("bool_key"), ShouldBeTrue)
		})

		PatchConvey("GetInt", func() {
			So(GetInt("int_key"), ShouldEqual, 42)
		})

		PatchConvey("GetInt32", func() {
			So(GetInt32("int32_key"), ShouldEqual, int32(32))
		})

		PatchConvey("GetInt64", func() {
			So(GetInt64("int64_key"), ShouldEqual, int64(64))
		})

		PatchConvey("GetFloat64", func() {
			So(GetFloat64("float64_key"), ShouldEqual, 3.14)
		})

		PatchConvey("GetDuration", func() {
			So(GetDuration("duration_key"), ShouldEqual, time.Second)
		})

		PatchConvey("GetStringSlice", func() {
			So(GetStringSlice("string_slice_key"), ShouldResemble, []string{"a", "b"})
		})

		PatchConvey("GetIntSlice", func() {
			So(GetIntSlice("int_slice_key"), ShouldResemble, []int{1, 2, 3})
		})
	})
}

func TestServerConfigFunctions(t *testing.T) {
	PatchConvey("TestServerConfigFunctions", t, func() {
		origVip := vip
		defer func() { vip = origVip }()

		PatchConvey("GetServerName-Default", func() {
			vip = viper.New()
			So(GetServerName(), ShouldEqual, defaultServerName)
		})

		PatchConvey("GetServerName-Custom", func() {
			vip = viper.New()
			vip.Set("Server.Name", "custom-server")
			So(GetServerName(), ShouldEqual, "custom-server")
		})

		PatchConvey("GetRawServerName", func() {
			vip = viper.New()
			vip.Set("Server.Name", "raw-server")
			So(GetRawServerName(), ShouldEqual, "raw-server")
		})

		PatchConvey("GetServerVersion-Default", func() {
			vip = viper.New()
			So(GetServerVersion(), ShouldEqual, defaultServerVersion)
		})

		PatchConvey("GetServerVersion-Custom", func() {
			vip = viper.New()
			vip.Set("Server.Version", "v1.0.0")
			So(GetServerVersion(), ShouldEqual, "v1.0.0")
		})
	})
}

// ==================== xconfig_location.go ====================

func TestGetLocationFromArg(t *testing.T) {
	PatchConvey("TestGetLocationFromArg", t, func() {
		PatchConvey("NoArg", func() {
			location := getLocationFromArg()
			So(location, ShouldBeEmpty)
		})

		PatchConvey("WithArg", func() {
			Mock(xutil.GetConfigFromArgs).Return("/from/arg.yml", nil).Build()
			location := getLocationFromArg()
			So(location, ShouldEqual, "/from/arg.yml")
		})
	})
}

// go test -run ^TestGetLocationFromArgWithArg$ -args server.config.location=/a/b/application.yml
func TestGetLocationFromArgWithArg(t *testing.T) {
	t.Skipf("如果需要测试，通过启动参数指定配置文件位置，请注释后，手动运行上面参数")

	PatchConvey("TestGetLocationFromArgWithArg", t, func() {
		location := getLocationFromArg()
		So(location, ShouldEqual, "/a/b/application.yml")
	})
}

func TestGetLocationFromENV(t *testing.T) {
	PatchConvey("TestGetLocationFromENV", t, func() {
		PatchConvey("Empty", func() {
			os.Unsetenv(configLocationEnvKey)
			So(getLocationFromENV(), ShouldBeEmpty)
		})

		PatchConvey("Set", func() {
			os.Setenv(configLocationEnvKey, "/a/b/c/application.yml")
			defer os.Unsetenv(configLocationEnvKey)
			So(getLocationFromENV(), ShouldEqual, "/a/b/c/application.yml")
		})
	})
}

func TestGetLocationFromCurrentDir(t *testing.T) {
	PatchConvey("TestGetLocationFromCurrentDir", t, func() {
		PatchConvey("NotFound", func() {
			location := getLocationFromCurrentDir()
			So(location, ShouldBeEmpty)
		})

		PatchConvey("FoundInCurrentDir", func() {
			filePath := "application.yml"
			file, err := os.Create(filePath)
			defer func() {
				_ = file.Close()
				_ = os.Remove(filePath)
			}()
			So(err, ShouldBeNil)

			location := getLocationFromCurrentDir()
			So(location, ShouldEqual, "./application.yml")
		})

		PatchConvey("FoundInConfDir", func() {
			err := os.MkdirAll("./conf", 0755)
			So(err, ShouldBeNil)

			filePath := "./conf/application.yml"
			file, err := os.Create(filePath)
			defer func() {
				_ = file.Close()
				_ = os.Remove(filePath)
				_ = os.RemoveAll("./conf")
			}()
			So(err, ShouldBeNil)

			location := getLocationFromCurrentDir()
			So(location, ShouldEqual, "./conf/application.yml")
		})

		PatchConvey("FoundInConfigDir", func() {
			err := os.MkdirAll("./config", 0755)
			So(err, ShouldBeNil)

			filePath := "./config/application.yml"
			file, err := os.Create(filePath)
			defer func() {
				_ = file.Close()
				_ = os.Remove(filePath)
				_ = os.RemoveAll("./config")
			}()
			So(err, ShouldBeNil)

			location := getLocationFromCurrentDir()
			So(location, ShouldEqual, "./config/application.yml")
		})
	})
}

func TestDetectConfigLocation(t *testing.T) {
	PatchConvey("TestDetectConfigLocation", t, func() {
		PatchConvey("FromArg", func() {
			Mock(getLocationFromArg).Return("/from/arg.yml").Build()
			So(detectConfigLocation(), ShouldEqual, "/from/arg.yml")
		})

		PatchConvey("FromENV", func() {
			Mock(getLocationFromArg).Return("").Build()
			Mock(getLocationFromENV).Return("/from/env.yml").Build()
			So(detectConfigLocation(), ShouldEqual, "/from/env.yml")
		})

		PatchConvey("FromCurrentDir", func() {
			Mock(getLocationFromArg).Return("").Build()
			Mock(getLocationFromENV).Return("").Build()
			Mock(getLocationFromCurrentDir).Return("./application.yml").Build()
			So(detectConfigLocation(), ShouldEqual, "./application.yml")
		})

		PatchConvey("NotFound", func() {
			Mock(getLocationFromArg).Return("").Build()
			Mock(getLocationFromENV).Return("").Build()
			Mock(getLocationFromCurrentDir).Return("").Build()
			So(detectConfigLocation(), ShouldEqual, "")
		})
	})
}

// ==================== xconfig_profiles.go ====================

func TestGetProfilesActiveFromArg(t *testing.T) {
	PatchConvey("TestGetProfilesActiveFromArg", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("a", nil).Build()
		So(getProfilesActiveFromArg(), ShouldEqual, "a")
	})
}

func TestGetProfilesActiveFromENV(t *testing.T) {
	PatchConvey("TestGetProfilesActiveFromENV", t, func() {
		PatchConvey("Empty", func() {
			os.Unsetenv(profilesActiveEnvKey)
			So(getProfilesActiveFromENV(), ShouldEqual, "")
		})

		PatchConvey("Set", func() {
			os.Setenv(profilesActiveEnvKey, "xxx")
			defer os.Unsetenv(profilesActiveEnvKey)
			So(getProfilesActiveFromENV(), ShouldEqual, "xxx")
		})
	})
}

func TestGetProfilesActiveFromViperConfig(t *testing.T) {
	PatchConvey("TestGetProfilesActiveFromViperConfig", t, func() {
		PatchConvey("NilViper", func() {
			So(getProfilesActiveFromViperConfig(nil), ShouldEqual, "")
		})

		PatchConvey("EmptyViper", func() {
			So(getProfilesActiveFromViperConfig(viper.New()), ShouldEqual, "")
		})

		PatchConvey("WithValue", func() {
			vp := viper.New()
			vp.Set("Server.profiles.active", "dev")
			So(getProfilesActiveFromViperConfig(vp), ShouldEqual, "dev")
		})
	})
}

func TestDetectProfilesActive(t *testing.T) {
	PatchConvey("TestDetectProfilesActive", t, func() {
		PatchConvey("FromArg", func() {
			Mock(getProfilesActiveFromArg).Return("dev").Build()
			So(detectProfilesActive(nil), ShouldEqual, "dev")
		})

		PatchConvey("FromENV", func() {
			Mock(getProfilesActiveFromArg).Return("").Build()
			Mock(getProfilesActiveFromENV).Return("prod").Build()
			So(detectProfilesActive(nil), ShouldEqual, "prod")
		})

		PatchConvey("FromViperConfig", func() {
			Mock(getProfilesActiveFromArg).Return("").Build()
			Mock(getProfilesActiveFromENV).Return("").Build()

			vp := viper.New()
			vp.Set("Server.profiles.active", "test")
			So(detectProfilesActive(vp), ShouldEqual, "test")
		})

		PatchConvey("NotFound", func() {
			Mock(getProfilesActiveFromArg).Return("").Build()
			Mock(getProfilesActiveFromENV).Return("").Build()
			So(detectProfilesActive(viper.New()), ShouldEqual, "")
		})
	})
}

func TestToProfilesActiveConfigLocation(t *testing.T) {
	PatchConvey("TestToProfilesActiveConfigLocation", t, func() {
		PatchConvey("NoExtension", func() {
			location, err := toProfilesActiveConfigLocation("x", "")
			So(err, ShouldNotBeNil)
			So(location, ShouldBeEmpty)
		})

		PatchConvey("Simple", func() {
			location, err := toProfilesActiveConfigLocation("x.yml", "a")
			So(err, ShouldBeNil)
			So(location, ShouldEqual, "x-a.yml")
		})

		PatchConvey("RelativePath", func() {
			location, err := toProfilesActiveConfigLocation("./x.yml", "a")
			So(err, ShouldBeNil)
			So(location, ShouldEqual, "./x-a.yml")
		})

		PatchConvey("AbsolutePath", func() {
			location, err := toProfilesActiveConfigLocation("/a/b/x.yml", "dev")
			So(err, ShouldBeNil)
			So(location, ShouldEqual, "/a/b/x-dev.yml")
		})
	})
}

func TestGetProfilesActiveWithEnvPlaceholder(t *testing.T) {
	PatchConvey("TestGetProfilesActiveWithEnvPlaceholder", t, func() {
		PatchConvey("WithEnvVar", func() {
			os.Setenv("PROFILES_ACTIVE", "test")
			defer os.Unsetenv("PROFILES_ACTIVE")

			vp := viper.New()
			vp.Set("Server.Profiles.Active", "${PROFILES_ACTIVE}")

			// 配置尚未整体展开，该函数自己展开这一个值
			So(getProfilesActiveFromViperConfig(vp), ShouldEqual, "test")
		})

		PatchConvey("WithDefault", func() {
			os.Unsetenv("PROFILES_ACTIVE_NOT_SET")

			vp := viper.New()
			vp.Set("Server.Profiles.Active", "${PROFILES_ACTIVE_NOT_SET:dev}")

			So(getProfilesActiveFromViperConfig(vp), ShouldEqual, "dev")
		})
	})
}

// ==================== xconfig_init.go ====================

func TestInitXConfig(t *testing.T) {
	PatchConvey("TestInitXConfig", t, func() {
		origVip := vip
		defer func() { vip = origVip }()

		PatchConvey("ConfigLocationNotFound", func() {
			Mock(detectConfigLocation).Return("").Build()
			err := initXConfig()
			So(err, ShouldBeNil)
		})

		PatchConvey("LoadDotEnvError", func() {
			Mock(detectConfigLocation).Return("/a/b.yaml").Build()
			Mock(loadDotEnvIfExist).Return(errors.New("dotenv error")).Build()
			err := initXConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "loadDotEnvIfExist")
		})

		PatchConvey("ParseConfigError", func() {
			Mock(detectConfigLocation).Return("/a/b.yaml").Build()
			Mock(loadDotEnvIfExist).Return(nil).Build()
			Mock(parseConfig).Return(nil, errors.New("parse error")).Build()
			err := initXConfig()
			So(err, ShouldNotBeNil)
		})

		PatchConvey("Success", func() {
			Mock(detectConfigLocation).Return("/a/b.yaml").Build()
			Mock(loadDotEnvIfExist).Return(nil).Build()
			Mock(parseConfig).Return(viper.New(), nil).Build()
			err := initXConfig()
			So(err, ShouldBeNil)
		})
	})
}

func TestLoadDotEnvIfExist(t *testing.T) {
	PatchConvey("TestLoadDotEnvIfExist", t, func() {
		PatchConvey("FileNotExist", func() {
			err := loadDotEnvIfExist("/nonexistent/b.yaml")
			So(err, ShouldBeNil)
		})

		PatchConvey("FileExist", func() {
			Mock(xutil.FileExist).Return(true).Build()
			Mock(godotenv.Load).Return(nil).Build()
			err := loadDotEnvIfExist("/a/b.yaml")
			So(err, ShouldBeNil)
		})

		PatchConvey("FileExistLoadError", func() {
			Mock(xutil.FileExist).Return(true).Build()
			Mock(godotenv.Load).Return(errors.New("load error")).Build()
			err := loadDotEnvIfExist("/a/b.yaml")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestParseConfig(t *testing.T) {
	PatchConvey("TestParseConfig", t, func() {
		PatchConvey("LoadLocalConfigError", func() {
			Mock(loadLocalConfig).Return(nil, errors.New("load error")).Build()
			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "load viper config failed")
			So(vp, ShouldBeNil)
		})

		PatchConvey("NoProfilesActive", func() {
			vpConfig := viper.New()
			vpConfig.Set("server.name", "test-svc")
			Mock(loadLocalConfig).Return(vpConfig, nil).Build()
			Mock(detectProfilesActive).Return("").Build()

			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldBeNil)
			So(vp, ShouldNotBeNil)
		})

		PatchConvey("WithProfilesActive", func() {
			vpConfig := viper.New()
			vpConfig.Set("server", map[string]any{
				"s1": 1,
				"s2": "2",
				"s3": []string{"3", "33"},
				"s4": map[string]any{
					"s41": 41,
					"s51": "51",
				},
				"profiles": map[string]any{
					"active": "xxx",
				},
			})
			vpConfig.Set("x", "x1")
			vpConfig.Set("y", "y1")

			Mock(loadLocalConfig).Return(vpConfig, nil).Build()
			Mock(detectProfilesActive).Return("xx").Build()

			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldBeNil)
			So(vp.AllSettings(), ShouldResemble, map[string]any{
				"server": map[string]any{
					"s1": 1,
					"s2": "2",
					"s3": []string{"3", "33"},
					"s4": map[string]any{
						"s41": 41,
						"s51": "51",
					},
					"profiles": map[string]any{
						"active": "xxx",
					},
				},
				"x": "x1",
				"y": "y1",
			})
		})

		PatchConvey("ProfilesActiveFileMissing", func() {
			vpConfig := viper.New()
			vpConfig.Set("server.name", "test-svc")
			Mock(loadLocalConfig).Return(vpConfig, nil).Build()
			Mock(detectProfilesActive).Return("dev").Build()
			Mock(xutil.FileExist).Return(false).Build()

			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldBeNil)
			So(vp, ShouldNotBeNil)
		})

		PatchConvey("ProfilesActiveFileLoadError", func() {
			vpConfig := viper.New()
			vpConfig.Set("server.name", "test-svc")
			callCount := 0
			Mock(loadLocalConfig).To(func(loc string) (*viper.Viper, error) {
				callCount++
				if callCount == 1 {
					return vpConfig, nil
				}
				return nil, errors.New("env config load error")
			}).Build()
			Mock(detectProfilesActive).Return("dev").Build()
			Mock(xutil.FileExist).Return(true).Build()

			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "load config file failed")
			So(vp, ShouldBeNil)
		})

		PatchConvey("ProfilesActiveFileLoadSuccess", func() {
			vpBase := viper.New()
			vpBase.Set("server", map[string]any{
				"name": "test-svc",
				"s1":   1,
			})
			vpBase.Set("x", "x1")

			vpEnv := viper.New()
			vpEnv.Set("server", map[string]any{
				"s1": 11,
			})
			vpEnv.Set("x", "x2")
			vpEnv.Set("z", "z2")

			callCount := 0
			Mock(loadLocalConfig).To(func(loc string) (*viper.Viper, error) {
				callCount++
				if callCount == 1 {
					return vpBase, nil
				}
				return vpEnv, nil
			}).Build()
			Mock(detectProfilesActive).Return("dev").Build()
			Mock(xutil.FileExist).Return(true).Build()

			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldBeNil)
			So(vp, ShouldNotBeNil)
			// 环境配置覆盖了基础配置
			So(vp.Get("x"), ShouldEqual, "x2")
			So(vp.Get("z"), ShouldEqual, "z2")
		})

		PatchConvey("ToProfilesActiveConfigLocationError", func() {
			vpConfig := viper.New()
			vpConfig.Set("server.name", "test-svc")
			// 配置文件无扩展名，导致 toProfilesActiveConfigLocation 失败
			Mock(loadLocalConfig).Return(vpConfig, nil).Build()
			Mock(detectProfilesActive).Return("dev").Build()
			Mock(toProfilesActiveConfigLocation).Return("", errors.New("no extension")).Build()

			vp, err := parseConfig("/a/b")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "parse profiles active config file failed")
			So(vp, ShouldBeNil)
		})

		PatchConvey("ServerNameEmpty", func() {
			vpConfig := viper.New()
			// 不设置 server.name，触发警告
			Mock(loadLocalConfig).Return(vpConfig, nil).Build()
			Mock(detectProfilesActive).Return("").Build()

			vp, err := parseConfig("/a/b.yml")
			So(err, ShouldBeNil)
			So(vp, ShouldNotBeNil)
		})
	})
}

func TestLoadLocalConfig(t *testing.T) {
	PatchConvey("TestLoadLocalConfig", t, func() {
		PatchConvey("FileNotFound", func() {
			vp, err := loadLocalConfig("/nonexistent/path.yml")
			So(err, ShouldNotBeNil)
			So(vp, ShouldBeNil)
		})

		PatchConvey("Success", func() {
			// 创建临时配置文件
			tmpFile, err := os.CreateTemp("", "test-*.yml")
			So(err, ShouldBeNil)
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString("key: value\n")
			So(err, ShouldBeNil)
			tmpFile.Close()

			vp, err := loadLocalConfig(tmpFile.Name())
			So(err, ShouldBeNil)
			So(vp, ShouldNotBeNil)
			So(vp.GetString("key"), ShouldEqual, "value")
		})
	})
}

func TestPrintFinalConfig(t *testing.T) {
	PatchConvey("TestPrintFinalConfig", t, func() {
		PatchConvey("DebugDisabled", func() {
			Mock(xutil.EnableXOneDebug).Return(false).Build()
			vp := viper.New()
			vp.Set("k", "v")
			printFinalConfig(vp) // 不输出
		})

		PatchConvey("DebugEnabled", func() {
			Mock(xutil.EnableXOneDebug).Return(true).Build()
			vp := viper.New()
			vp.Set("k", "v")
			printFinalConfig(vp) // 输出配置信息
		})
	})
}

func TestMergeProfilesViperConfig(t *testing.T) {
	PatchConvey("TestMergeProfilesViperConfig", t, func() {
		vp1 := viper.New()
		vp1.Set("server", map[string]any{
			"s1": 1,
			"s2": "2",
			"s3": []string{"3", "33"},
			"s4": map[string]any{
				"s41": 41,
				"s51": "51",
			},
			"profiles": "p1",
		})
		vp1.Set("x", "x1")
		vp1.Set("y", "y1")

		vp2 := viper.New()
		vp2.Set("server", map[string]any{
			"s1": 11,
			"s3": []string{"33", "44"},
			"s4": map[string]any{
				"s41": 441,
				"651": "651",
			},
			"profiles": "p2",
		})
		vp2.Set("x", "x2")
		vp2.Set("z", "z2")

		merged := deepMerge(vp1.AllSettings(), profileSettingsOf(vp2))
		vp := viper.New()
		for k, v := range merged {
			vp.Set(k, v)
		}
		So(vp.AllSettings(), ShouldResemble, map[string]any{
			"server": map[string]any{
				"s1": 11,
				"s2": "2",
				"s3": []string{"33", "44"}, // 列表整体替换
				"s4": map[string]any{ // 嵌套块逐层合并，vp1 的 s51 得以保留
					"s41": 441,
					"s51": "51",
					"651": "651",
				},
				"profiles": "p1", // 环境配置文件不能改写激活环境
			},
			"x": "x2",
			"y": "y1",
			"z": "z2",
		})
	})
}

func TestExpandEnvPlaceholder(t *testing.T) {
	PatchConvey("TestExpandEnvPlaceholder", t, func() {
		PatchConvey("WithEnvVar", func() {
			t.Setenv("TEST_EXPAND", "expanded")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND}")
			So(got, ShouldEqual, "expanded")
			So(iss.missing, ShouldBeEmpty)
			So(iss.unsupported, ShouldBeEmpty)
		})

		PatchConvey("WithDefault", func() {
			os.Unsetenv("NONEXISTENT_VAR")
			got, iss := expandEnvPlaceholder("${NONEXISTENT_VAR:fallback}")
			So(got, ShouldEqual, "fallback")
			So(iss.missing, ShouldBeEmpty)
			So(iss.unsupported, ShouldBeEmpty)
		})

		PatchConvey("NoPlaceholder", func() {
			got, iss := expandEnvPlaceholder("plain_value")
			So(got, ShouldEqual, "plain_value")
			So(iss.missing, ShouldBeEmpty)
			So(iss.unsupported, ShouldBeEmpty)
		})

		PatchConvey("显式设为空串的环境变量应覆盖默认值", func() {
			t.Setenv("TEST_EXPAND_EMPTY", "")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND_EMPTY:fallback}")
			So(got, ShouldEqual, "")
			So(iss.missing, ShouldBeEmpty)
			So(iss.unsupported, ShouldBeEmpty)
		})

		PatchConvey("空默认值写法 ${VAR:} 表示可选且默认为空", func() {
			os.Unsetenv("TEST_EXPAND_OPTIONAL")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND_OPTIONAL:}")
			So(got, ShouldEqual, "")
			So(iss.missing, ShouldBeEmpty)
			So(iss.unsupported, ShouldBeEmpty)
		})

		PatchConvey("无默认值且未设置时报告为缺失，并保留原样", func() {
			os.Unsetenv("TEST_EXPAND_REQUIRED")
			got, iss := expandEnvPlaceholder("prefix-${TEST_EXPAND_REQUIRED}-suffix")
			So(got, ShouldEqual, "prefix-${TEST_EXPAND_REQUIRED}-suffix")
			So(iss.missing, ShouldResemble, []string{"TEST_EXPAND_REQUIRED"})
		})

		PatchConvey("同一个字符串里多个占位符", func() {
			t.Setenv("TEST_EXPAND_A", "a")
			os.Unsetenv("TEST_EXPAND_B")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND_A}/${TEST_EXPAND_B}/${TEST_EXPAND_C:c}")
			So(got, ShouldEqual, "a/${TEST_EXPAND_B}/c")
			So(iss.missing, ShouldResemble, []string{"TEST_EXPAND_B"})
		})

		PatchConvey("默认值里可以含冒号", func() {
			// 判断是否写了默认值靠正则分组而不是查字符串里有没有冒号，
			// 否则 ${ADDR:127.0.0.1:6379} 这种会解析错
			os.Unsetenv("TEST_EXPAND_ADDR")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND_ADDR:127.0.0.1:6379}")
			So(got, ShouldEqual, "127.0.0.1:6379")
			So(iss.empty(), ShouldBeTrue)
		})

		PatchConvey("默认值可以以 - 开头", func() {
			os.Unsetenv("TEST_EXPAND_NEG")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND_NEG:-1}")
			So(got, ShouldEqual, "-1")
			So(iss.empty(), ShouldBeTrue)
		})

		PatchConvey("${VAR:} 表示默认为空", func() {
			os.Unsetenv("TEST_EXPAND_E")
			got, iss := expandEnvPlaceholder("${TEST_EXPAND_E:}")
			So(got, ShouldEqual, "")
			So(iss.empty(), ShouldBeTrue)
		})

		PatchConvey("空占位符 ${} 同样报出来", func() {
			_, iss := expandEnvPlaceholder("${}")
			So(iss.unsupported, ShouldResemble, []string{"${}"})
		})
	})
}
func TestExpandEnvPlaceholders(t *testing.T) {
	PatchConvey("TestExpandEnvPlaceholders", t, func() {
		PatchConvey("WithEnvVar", func() {
			t.Setenv("TEST_VAR", "test_value")
			vp := viper.New()
			vp.Set("key", "${TEST_VAR}")
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.GetString("key"), ShouldEqual, "test_value")
		})

		PatchConvey("WithDefault", func() {
			os.Unsetenv("NONEXISTENT_VAR")
			vp := viper.New()
			vp.Set("key", "${NONEXISTENT_VAR:default_value}")
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.GetString("key"), ShouldEqual, "default_value")
		})

		PatchConvey("NoPlaceholder", func() {
			vp := viper.New()
			vp.Set("key", "plain_value")
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.GetString("key"), ShouldEqual, "plain_value")
		})

		PatchConvey("EmptyValue", func() {
			vp := viper.New()
			vp.Set("key", "")
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.GetString("key"), ShouldEqual, "")
		})

		PatchConvey("NestedKey", func() {
			t.Setenv("NESTED_VAR", "nested_val")
			vp := viper.New()
			vp.Set("a.b.c", "${NESTED_VAR}")
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.GetString("a.b.c"), ShouldEqual, "nested_val")
		})

		PatchConvey("列表元素里的占位符同样展开", func() {
			t.Setenv("LIST_VAR", "X-Real-Header")
			vp := viper.New()
			vp.Set("headers", []any{"${LIST_VAR}", "X-Static", "", 42})
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.Get("headers"), ShouldResemble, []any{"X-Real-Header", "X-Static", "", 42})
		})

		PatchConvey("必填占位符缺失时初始化失败", func() {
			os.Unsetenv("REQUIRED_A")
			os.Unsetenv("REQUIRED_B")
			vp := viper.New()
			vp.Set("a", "${REQUIRED_A}")
			vp.Set("b.c", "${REQUIRED_A}")
			vp.Set("list", []any{"${REQUIRED_B}"})

			err := expandEnvPlaceholders(vp)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "required env placeholder not set")
			// 去重后每个变量只报一次
			So(strings.Count(err.Error(), "REQUIRED_A"), ShouldEqual, 1)
			So(err.Error(), ShouldContainSubstring, "REQUIRED_B")
		})

		PatchConvey("非字符串类型不受影响", func() {
			vp := viper.New()
			vp.Set("num", 42)
			vp.Set("flag", true)
			So(expandEnvPlaceholders(vp), ShouldBeNil)
			So(vp.GetInt("num"), ShouldEqual, 42)
			So(vp.GetBool("flag"), ShouldBeTrue)
		})
	})
}

func TestSetNestedValue(t *testing.T) {
	PatchConvey("TestSetNestedValue", t, func() {
		PatchConvey("SimpleKey", func() {
			m := make(map[string]any)
			setNestedValue(m, "key", "value")
			So(m["key"], ShouldEqual, "value")
		})

		PatchConvey("NestedKey", func() {
			m := make(map[string]any)
			setNestedValue(m, "a.b.c", "value")
			a := m["a"].(map[string]any)
			b := a["b"].(map[string]any)
			So(b["c"], ShouldEqual, "value")
		})

		PatchConvey("ExistingNested", func() {
			m := map[string]any{
				"a": map[string]any{
					"existing": "value",
				},
			}
			setNestedValue(m, "a.new", "newvalue")
			a := m["a"].(map[string]any)
			So(a["existing"], ShouldEqual, "value")
			So(a["new"], ShouldEqual, "newvalue")
		})

		PatchConvey("OverwriteNonMap", func() {
			// 中间路径不是 map，会被覆盖为新 map
			m := map[string]any{
				"a": "not_a_map",
			}
			setNestedValue(m, "a.b", "value")
			a := m["a"].(map[string]any)
			So(a["b"], ShouldEqual, "value")
		})
	})
}

// ==================== 审查回归 ====================

// writeTempConfig 写入临时配置文件并返回路径
func writeTempConfig(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	return p
}

// TestProfileDeepMerge 环境配置文件只写要改的字段，同一块下未提及的字段必须保留
func TestProfileDeepMerge(t *testing.T) {
	PatchConvey("TestProfileDeepMerge", t, func() {
		dir := t.TempDir()
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: demo
  Profiles:
    Active: dev
XLog:
  Level: info
  File:
    Enable: true
    Path: ./log
`)
		writeTempConfig(t, dir, "application-dev.yml", `
XLog:
  Level: debug
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XLog.Level"), ShouldEqual, "debug")
		// 环境文件没提到的兄弟字段不应被抹掉
		So(vp.GetString("XLog.File.Path"), ShouldEqual, "./log")
		So(vp.GetBool("XLog.File.Enable"), ShouldBeTrue)
	})
}

// TestProfilesActiveRejectsPathSeparators 环境名会拼进文件路径，必须限制字符集
func TestProfilesActiveRejectsPathSeparators(t *testing.T) {
	PatchConvey("TestProfilesActiveRejectsPathSeparators", t, func() {
		for _, bad := range []string{"../../../tmp/evil", "a/b", "a\\b", "", "dev prod", "de.v"} {
			loc, err := toProfilesActiveConfigLocation("./conf/application.yml", bad)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "profiles active is invalid")
			So(loc, ShouldBeEmpty)
		}

		for _, ok := range []string{"dev", "pre-prod", "test_1", "PROD"} {
			loc, err := toProfilesActiveConfigLocation("./conf/application.yml", ok)
			So(err, ShouldBeNil)
			So(loc, ShouldEqual, "./conf/application-"+ok+".yml")
		}
	})
}

// TestMaskSensitive 打印配置时凭证必须脱敏
func TestMaskSensitive(t *testing.T) {
	PatchConvey("TestMaskSensitive", t, func() {
		masked := maskSensitive(map[string]any{
			"xgorm":  map[string]any{"password": "super-secret-pw", "host": "127.0.0.1"},
			"xredis": map[string]any{"Password": "redis-pw"},
			"myapp": map[string]any{
				"apikey":     "ak",
				"api_key":    "ak2",
				"accesskey":  "ak3",
				"privatekey": "pk",
				"token":      "tk",
				"secret":     "sc",
				"credential": "cd",
				"dsn":        "user:pw@tcp(host)/db",
				"nested":     map[string]any{"passwd": "p"},
				"plain":      "visible",
			},
		})
		dumped := xutil.ToJsonStringIndent(masked)
		for _, secret := range []string{"super-secret-pw", "redis-pw", "ak", "ak2", "ak3", "pk", "tk", "sc", "cd", "user:pw@tcp(host)/db", `"p"`} {
			So(dumped, ShouldNotContainSubstring, secret)
		}
		So(dumped, ShouldContainSubstring, "visible")
		So(dumped, ShouldContainSubstring, "127.0.0.1")
	})
}

// TestPrintFinalConfigMasksSecrets 端到端确认打印路径不泄漏凭证
func TestPrintFinalConfigMasksSecrets(t *testing.T) {
	PatchConvey("TestPrintFinalConfigMasksSecrets", t, func() {
		PatchConvey("debug 关闭时不打印", func() {
			Mock(xutil.EnableXOneDebug).Return(false).Build()
			mocker := Mock(xutil.ToJsonStringIndent).Return("").Build()
			vp := viper.New()
			vp.Set("XGorm", map[string]any{"Password": "super-secret-pw"})
			printFinalConfig(vp)
			So(mocker.Times(), ShouldEqual, 0)
		})

		PatchConvey("debug 开启时打印脱敏后的内容", func() {
			Mock(xutil.EnableXOneDebug).Return(true).Build()
			var dumped any
			Mock(xutil.ToJsonStringIndent).To(func(v any) string {
				dumped = v
				return "dumped"
			}).Build()

			vp := viper.New()
			vp.Set("XGorm", map[string]any{"Password": "super-secret-pw"})
			printFinalConfig(vp)

			settings, ok := dumped.(map[string]any)
			So(ok, ShouldBeTrue)
			gorm, ok := settings["xgorm"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(gorm["password"], ShouldEqual, maskedValue)
		})
	})
}

// TestUnmarshalConfigTypedNilPointer 类型化 nil 指针应被 checkParam 拦下
func TestUnmarshalConfigTypedNilPointer(t *testing.T) {
	PatchConvey("TestUnmarshalConfigTypedNilPointer", t, func() {
		var p *Server
		err := UnmarshalConfig(ServerConfigKey, p)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "param conf is a nil pointer")
	})
}

// TestDeepMerge 深合并的各条规则
func TestDeepMerge(t *testing.T) {
	PatchConvey("TestDeepMerge", t, func() {
		PatchConvey("嵌套 map 逐层合并", func() {
			base := map[string]any{"a": map[string]any{"x": 1, "y": 2}}
			over := map[string]any{"a": map[string]any{"y": 22, "z": 33}}
			So(deepMerge(base, over), ShouldResemble, map[string]any{
				"a": map[string]any{"x": 1, "y": 22, "z": 33},
			})
			// 入参不被修改
			So(base, ShouldResemble, map[string]any{"a": map[string]any{"x": 1, "y": 2}})
		})

		PatchConvey("列表整体替换", func() {
			base := map[string]any{"a": []any{1, 2, 3}}
			over := map[string]any{"a": []any{9}}
			So(deepMerge(base, over), ShouldResemble, map[string]any{"a": []any{9}})
		})

		PatchConvey("类型不一致时以 override 为准", func() {
			So(deepMerge(map[string]any{"a": "str"}, map[string]any{"a": map[string]any{"x": 1}}),
				ShouldResemble, map[string]any{"a": map[string]any{"x": 1}})
			So(deepMerge(map[string]any{"a": map[string]any{"x": 1}}, map[string]any{"a": "str"}),
				ShouldResemble, map[string]any{"a": "str"})
		})
	})
}

// TestDropProfilesActive 环境配置文件不能改写激活环境
func TestDropProfilesActive(t *testing.T) {
	PatchConvey("TestDropProfilesActive", t, func() {
		PatchConvey("移除 profiles 后 server 还有其它字段则保留", func() {
			m := map[string]any{"server": map[string]any{"name": "n", "profiles": map[string]any{"active": "x"}}}
			dropProfilesActive(m)
			So(m, ShouldResemble, map[string]any{"server": map[string]any{"name": "n"}})
		})

		PatchConvey("server 下只有 profiles 时整个 server 移除", func() {
			m := map[string]any{"server": map[string]any{"profiles": map[string]any{"active": "x"}}}
			dropProfilesActive(m)
			So(m, ShouldBeEmpty)
		})

		PatchConvey("没有 server 块时不做任何事", func() {
			m := map[string]any{"x": 1}
			dropProfilesActive(m)
			So(m, ShouldResemble, map[string]any{"x": 1})
		})

		PatchConvey("server 不是 map 时不做任何事", func() {
			m := map[string]any{"server": "not-a-map"}
			dropProfilesActive(m)
			So(m, ShouldResemble, map[string]any{"server": "not-a-map"})
		})
	})
}

// TestDistinct 去重保持顺序
func TestDistinct(t *testing.T) {
	PatchConvey("TestDistinct", t, func() {
		So(distinct([]string{"b", "a", "b", "c", "a"}), ShouldResemble, []string{"b", "a", "c"})
		So(distinct(nil), ShouldBeEmpty)
	})
}

// TestParseConfigRequiredPlaceholderMissing 必填占位符缺失时 parseConfig 直接失败
func TestParseConfigRequiredPlaceholderMissing(t *testing.T) {
	PatchConvey("TestParseConfigRequiredPlaceholderMissing", t, func() {
		os.Unsetenv("XONE_TEST_REQUIRED_DSN")
		dir := t.TempDir()
		base := writeTempConfig(t, dir, "application.yml", "Server:\n  Name: demo\nXGorm:\n  Dsn: \"${XONE_TEST_REQUIRED_DSN}\"\n")

		vp, err := parseConfig(base)
		So(vp, ShouldBeNil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "required env placeholder not set")
		So(err.Error(), ShouldContainSubstring, "XONE_TEST_REQUIRED_DSN")
	})
}

// TestToProfilesActiveConfigLocationNoExtension 配置文件没有扩展名时报错
func TestToProfilesActiveConfigLocationNoExtension(t *testing.T) {
	PatchConvey("TestToProfilesActiveConfigLocationNoExtension", t, func() {
		loc, err := toProfilesActiveConfigLocation("./conf/application", "dev")
		So(loc, ShouldBeEmpty)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "no extension found")
	})
}

// ==================== xconfig_import.go ====================

// TestImportPrecedence 被导入文件覆盖主配置，环境配置仍有最终决定权
//
// 与 Spring Boot 的 spring.config.import 同向：db.yml 是数据库配置的权威，
// 不会被 application.yml 里一个忘删的残留字段悄悄压过。
func TestImportPrecedence(t *testing.T) {
	PatchConvey("TestImportPrecedence", t, func() {
		dir := t.TempDir()
		writeTempConfig(t, dir, "db.yml", `
XGorm:
  Driver: "postgres"
  MaxOpenConns: 999
  MaxIdleConns: 7
`)
		writeTempConfig(t, dir, "application-dev.yml", `
XGorm:
  MaxIdleConns: 3
`)
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
XGorm:
  MaxOpenConns: 10
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "postgres") // 只有导入的写了
		So(vp.GetInt("XGorm.MaxOpenConns"), ShouldEqual, 999)     // 导入的压过主配置
		So(vp.GetInt("XGorm.MaxIdleConns"), ShouldEqual, 3)       // 环境配置压过导入的
	})
}

// TestImportOrderWithinList 导入列表内后声明的覆盖先声明的
func TestImportOrderWithinList(t *testing.T) {
	PatchConvey("TestImportOrderWithinList", t, func() {
		dir := t.TempDir()
		writeTempConfig(t, dir, "a.yml", "XGorm:\n  Driver: \"mysql\"\n")
		writeTempConfig(t, dir, "b.yml", "XGorm:\n  Driver: \"postgres\"\n")
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Config:
    Import:
      - a.yml
      - b.yml
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "postgres") // 后声明的赢
	})
}

// TestImportProfileVariant 被导入文件同样支持 {name}-{env}.yml 变体
func TestImportProfileVariant(t *testing.T) {
	PatchConvey("TestImportProfileVariant", t, func() {
		dir := t.TempDir()
		writeTempConfig(t, dir, "db.yml", "XGorm:\n  DSN: \"host=base\"\n  Driver: \"mysql\"\n")
		writeTempConfig(t, dir, "db-dev.yml", "XGorm:\n  DSN: \"host=dev\"\n")
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XGorm.DSN"), ShouldEqual, "host=dev") // 环境变体覆盖
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "mysql") // 未提及的字段保留
	})
}

// TestImportListIsUnion base 与 env 的导入列表取并集而不是替换
func TestImportListIsUnion(t *testing.T) {
	PatchConvey("TestImportListIsUnion", t, func() {
		// Import 是加载指令不是配置数据：按「列表整体替换」处理的话，
		// 环境配置想多加一个文件就得把整张列表抄一遍
		dir := t.TempDir()
		writeTempConfig(t, dir, "db.yml", "XGorm:\n  Driver: \"postgres\"\n")
		writeTempConfig(t, dir, "extra.yml", "XCache:\n  MaxCost: 12345\n")
		writeTempConfig(t, dir, "application-dev.yml", `
Server:
  Config:
    Import:
      - extra.yml
`)
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "postgres") // base 列表里的
		So(vp.GetInt("XCache.MaxCost"), ShouldEqual, 12345)       // env 列表里的
	})
}

// TestImportMissingFileFails 显式声明要导入的文件不存在时必须失败
func TestImportMissingFileFails(t *testing.T) {
	PatchConvey("TestImportMissingFileFails", t, func() {
		// 与 application-{env}.yml 不存在只告警不同：那是约定俗成的可选项，
		// 这个是使用者点名要的，静默忽略会让配置莫名其妙地缺一块
		dir := t.TempDir()
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Config:
    Import:
      - nope.yml
`)
		_, err := parseConfig(base)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "imported config file not found")
	})
}

// TestImportNoNesting 被导入文件里的 Import 不生效，且不污染最终配置
func TestImportNoNesting(t *testing.T) {
	PatchConvey("TestImportNoNesting", t, func() {
		dir := t.TempDir()
		writeTempConfig(t, dir, "deep.yml", "XCache:\n  MaxCost: 777\n")
		writeTempConfig(t, dir, "db.yml", `
Server:
  Config:
    Import:
      - deep.yml
XGorm:
  Driver: "postgres"
`)
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Config:
    Import:
      - db.yml
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "postgres")
		So(vp.IsSet("XCache.MaxCost"), ShouldBeFalse) // 嵌套导入未生效
	})
}

// TestImportCannotChangeProfilesActive 被导入文件不能改写激活环境
func TestImportCannotChangeProfilesActive(t *testing.T) {
	PatchConvey("TestImportCannotChangeProfilesActive", t, func() {
		dir := t.TempDir()
		writeTempConfig(t, dir, "db.yml", `
Server:
  Profiles:
    Active: "prod"
XGorm:
  Driver: "postgres"
`)
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString(profilesActiveConfigKey), ShouldEqual, "dev")
	})
}

// TestImportPathPlaceholderAndAbs 导入路径支持占位符与绝对路径
func TestImportPathPlaceholderAndAbs(t *testing.T) {
	PatchConvey("TestImportPathPlaceholderAndAbs", t, func() {
		dir := t.TempDir()
		abs := writeTempConfig(t, dir, "redis.yml", "XRedis:\n  Addr: \"127.0.0.1:6379\"\n")

		PatchConvey("占位符在导入前展开", func() {
			t.Setenv("XONE_TEST_DB_FILE", "db.yml")
			writeTempConfig(t, dir, "db.yml", "XGorm:\n  Driver: \"postgres\"\n")
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - ${XONE_TEST_DB_FILE}\n")

			vp, err := parseConfig(base)
			So(err, ShouldBeNil)
			So(vp.GetString("XGorm.Driver"), ShouldEqual, "postgres")
		})

		PatchConvey("必填占位符缺失时报错", func() {
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - ${XONE_TEST_ABSENT_FILE}\n")

			_, err := parseConfig(base)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "required env placeholder not set")
		})

		PatchConvey("绝对路径不拼接基准目录", func() {
			base := writeTempConfig(t, filepath.Join(dir), "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - "+abs+"\n")

			vp, err := parseConfig(base)
			So(err, ShouldBeNil)
			So(vp.GetString("XRedis.Addr"), ShouldEqual, "127.0.0.1:6379")
		})
	})
}

// TestImportNotDeclared 未声明 Import 时行为完全不变
func TestImportNotDeclared(t *testing.T) {
	PatchConvey("TestImportNotDeclared", t, func() {
		dir := t.TempDir()
		base := writeTempConfig(t, dir, "application.yml", "Server:\n  Name: \"a.b.c\"\nXGorm:\n  Driver: \"mysql\"\n")

		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "mysql")
	})
}

// TestImportErrorPaths 导入过程中的各条失败路径
func TestImportErrorPaths(t *testing.T) {
	PatchConvey("TestImportErrorPaths", t, func() {
		dir := t.TempDir()

		PatchConvey("被导入文件内容非法", func() {
			writeTempConfig(t, dir, "db.yml", "XGorm:\n  : : bad yaml : :\n")
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - db.yml\n")

			_, err := parseConfig(base)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "load imported config file failed")
		})

		PatchConvey("被导入文件的环境变体内容非法", func() {
			writeTempConfig(t, dir, "db2.yml", "XGorm:\n  Driver: \"mysql\"\n")
			writeTempConfig(t, dir, "db2-dev.yml", "XGorm:\n  : : bad yaml : :\n")
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Profiles:\n    Active: \"dev\"\n  Config:\n    Import:\n      - db2.yml\n")

			_, err := parseConfig(base)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "load imported env config file failed")
		})

		PatchConvey("导入路径无扩展名", func() {
			// 报「导入列表里这一项没有扩展名」，而不是 viper 的 Unsupported Config Type
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - noext\n")

			_, err := parseConfig(base)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "imported config file has no extension")
		})
	})
}

// TestDropImport 剔除 Server.Config.Import 时的各种结构形态
func TestDropImport(t *testing.T) {
	PatchConvey("TestDropImport", t, func() {
		PatchConvey("没有 Server 块", func() {
			s := map[string]any{"xgorm": map[string]any{"driver": "mysql"}}
			dropImport(s)
			So(s, ShouldContainKey, "xgorm")
		})

		PatchConvey("Server 下没有 Config 块", func() {
			s := map[string]any{"server": map[string]any{"name": "a.b.c"}}
			dropImport(s)
			So(s["server"].(map[string]any)["name"], ShouldEqual, "a.b.c")
		})

		PatchConvey("Config 下只有 Import 时整块移除", func() {
			s := map[string]any{"server": map[string]any{
				"config": map[string]any{"import": []any{"db.yml"}},
			}}
			dropImport(s)
			So(s, ShouldBeEmpty) // server 与 config 都空了，一并移除
		})

		PatchConvey("Config 下还有别的字段时只删 Import", func() {
			s := map[string]any{"server": map[string]any{
				"name":   "a.b.c",
				"config": map[string]any{"import": []any{"db.yml"}, "other": 1},
			}}
			dropImport(s)
			cfg := s["server"].(map[string]any)["config"].(map[string]any)
			So(cfg, ShouldNotContainKey, "import")
			So(cfg["other"], ShouldEqual, 1)
		})
	})
}

// TestPlaceholderIssuesErr 两类问题各自给出可操作的提示
func TestPlaceholderIssuesErr(t *testing.T) {
	PatchConvey("TestPlaceholderIssuesErr", t, func() {
		PatchConvey("写法不支持时优先报它", func() {
			// 变量没设置往往是写法错了的连带结果，所以写法问题排在前面
			iss := placeholderIssues{missing: []string{"A"}, unsupported: []string{"${}"}}
			err := iss.err("test")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unsupported placeholder syntax")
			So(err.Error(), ShouldContainSubstring, "${VAR:default}")
		})

		PatchConvey("只有缺失变量时报缺失", func() {
			err := placeholderIssues{missing: []string{"A", "A", "B"}}.err("test")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "required env placeholder not set")
			So(err.Error(), ShouldContainSubstring, "[A B]") // 去重
		})

		PatchConvey("没有问题时返回 nil", func() {
			So(placeholderIssues{}.err("test"), ShouldBeNil)
		})
	})
}

// TestImportPlaceholderInPath 导入路径里的占位符两种写法都支持，非法写法要报出来
func TestImportPlaceholderInPath(t *testing.T) {
	PatchConvey("TestImportPlaceholderInPath", t, func() {
		PatchConvey("Spring 写法的默认值可用", func() {
			dir := t.TempDir()
			writeTempConfig(t, dir, "db.yml", "XGorm:\n  Driver: \"postgres\"\n")
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - ${CFG_DB_FILE:db.yml}\n")

			vp, err := parseConfig(base)
			So(err, ShouldBeNil)
			So(vp.GetString("XGorm.Driver"), ShouldEqual, "postgres")
		})

		PatchConvey("非法占位符要报出来", func() {
			dir := t.TempDir()
			base := writeTempConfig(t, dir, "application.yml",
				"Server:\n  Name: \"a.b.c\"\n  Config:\n    Import:\n      - ${}/db.yml\n")

			_, err := parseConfig(base)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unsupported placeholder syntax")
		})
	})
}

// TestImportProfileBandBeatsPlainBand 带环境后缀的文件整体压过不带的
//
// 与 Spring 一致（profile-specific files always overriding the non-specific ones）。
// 若按「每个文件加载完立刻合并自己的环境变体」的写法，db-dev.yml 会被后面声明的
// redis.yml 压过——一个环境专属的值被非环境专属的值覆盖，说不通。
func TestImportProfileBandBeatsPlainBand(t *testing.T) {
	PatchConvey("TestImportProfileBandBeatsPlainBand", t, func() {
		dir := t.TempDir()
		// db 与 redis 都写同一个 key，redis 在导入列表里排在 db 后面
		writeTempConfig(t, dir, "db.yml", "XGorm:\n  Driver: \"from-db\"\n")
		writeTempConfig(t, dir, "db-dev.yml", "XGorm:\n  Driver: \"from-db-dev\"\n")
		writeTempConfig(t, dir, "redis.yml", "XGorm:\n  Driver: \"from-redis\"\n")

		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
      - redis.yml
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)
		// db-dev.yml 在带后缀的一轮，redis.yml 在不带后缀的一轮，前者整体在后
		So(vp.GetString("XGorm.Driver"), ShouldEqual, "from-db-dev")
	})
}

// TestImportFullOrder 六个文件的完整优先级链
func TestImportFullOrder(t *testing.T) {
	PatchConvey("TestImportFullOrder", t, func() {
		dir := t.TempDir()
		// 每个文件都写自己的 key，再各写一个公共 key 用来确认谁最终赢
		writeTempConfig(t, dir, "db.yml", "K:\n  Own: \"db\"\n  Shared: \"db\"\n")
		writeTempConfig(t, dir, "redis.yml", "K:\n  Own2: \"redis\"\n  Shared: \"redis\"\n")
		writeTempConfig(t, dir, "db-dev.yml", "K:\n  Own3: \"db-dev\"\n  Shared: \"db-dev\"\n")
		writeTempConfig(t, dir, "redis-dev.yml", "K:\n  Own4: \"redis-dev\"\n  Shared: \"redis-dev\"\n")
		writeTempConfig(t, dir, "application-dev.yml", "K:\n  Own5: \"app-dev\"\n  Shared: \"app-dev\"\n")
		base := writeTempConfig(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
      - redis.yml
K:
  Own6: "app"
  Shared: "app"
`)
		vp, err := parseConfig(base)
		So(err, ShouldBeNil)

		// 六个文件各自独有的 key 都在：没有任何一份被整体丢弃
		So(vp.GetString("K.Own"), ShouldEqual, "db")
		So(vp.GetString("K.Own2"), ShouldEqual, "redis")
		So(vp.GetString("K.Own3"), ShouldEqual, "db-dev")
		So(vp.GetString("K.Own4"), ShouldEqual, "redis-dev")
		So(vp.GetString("K.Own5"), ShouldEqual, "app-dev")
		So(vp.GetString("K.Own6"), ShouldEqual, "app")

		// 链条末端赢：application.yml < db.yml < redis.yml
		//              < application-dev.yml < db-dev.yml < redis-dev.yml
		So(vp.GetString("K.Shared"), ShouldEqual, "redis-dev")
	})
}
