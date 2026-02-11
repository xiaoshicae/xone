package xconfig

import (
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/spf13/viper"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== config.go ====================

func TestServerConfigMergeDefault(t *testing.T) {
	PatchConvey("TestServerConfigMergeDefault", t, func() {
		PatchConvey("Nil", func() {
			sc := serverConfigMergeDefault(nil)
			So(sc, ShouldResemble, &Server{
				Name:     "",
				Version:  defaultServerVersion,
				Profiles: nil,
			})
		})

		PatchConvey("NotNil", func() {
			sc := serverConfigMergeDefault(&Server{
				Name:    "svc",
				Version: "v1.0.0",
				Profiles: &Profiles{
					Active: "dev",
				},
			})
			So(sc, ShouldResemble, &Server{
				Name:    "svc",
				Version: "v1.0.0",
				Profiles: &Profiles{
					Active: "dev",
				},
			})
		})
	})
}

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

			// 展开前返回原始占位符
			So(getProfilesActiveFromViperConfig(vp), ShouldEqual, "${PROFILES_ACTIVE}")

			// 展开后返回环境变量值
			expandEnvPlaceholders(vp)
			So(getProfilesActiveFromViperConfig(vp), ShouldEqual, "test")
		})

		PatchConvey("WithDefault", func() {
			os.Unsetenv("PROFILES_ACTIVE_NOT_SET")

			vp := viper.New()
			vp.Set("Server.Profiles.Active", "${PROFILES_ACTIVE_NOT_SET:-dev}")

			expandEnvPlaceholders(vp)
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

		vp := mergeProfilesViperConfig(vp1, vp2)
		So(vp.AllSettings(), ShouldResemble, map[string]any{
			"server": map[string]any{
				"s1": 11,
				"s2": "2",
				"s3": []string{"33", "44"},
				"s4": map[string]any{
					"s41": 441,
					"651": "651",
				},
				"profiles": "p1",
			},
			"x": "x2",
			"y": "y1",
			"z": "z2",
		})
	})
}

func TestGetTopLevelConfigs(t *testing.T) {
	PatchConvey("TestGetTopLevelConfigs", t, func() {
		vp := viper.New()
		vp.Set("server", map[string]any{
			"s1":       1,
			"s2":       "2",
			"s3":       []string{"3", "33"},
			"s4":       map[string]any{"s41": 41, "s51": "51"},
			"profiles": "ppp",
		})
		vp.Set("x", "xxx")
		vp.Set("y", "yyy")

		res := getTopLevelConfigs(vp)
		So(res, ShouldResemble, map[string]any{
			"server": map[string]any{
				"s1":       1,
				"s2":       "2",
				"s3":       []string{"3", "33"},
				"s4":       map[string]any{"s41": 41, "s51": "51"},
				"profiles": "ppp",
			},
			"x": "xxx",
			"y": "yyy",
		})
	})
}

func TestIsNestedKey(t *testing.T) {
	PatchConvey("TestIsNestedKey", t, func() {
		So(isNestedKey(""), ShouldBeFalse)
		So(isNestedKey("1"), ShouldBeFalse)
		So(isNestedKey("."), ShouldBeTrue)
		So(isNestedKey(".1"), ShouldBeTrue)
		So(isNestedKey("1.1"), ShouldBeTrue)
	})
}

func TestGetTopLevelAndServerSecondLevelConfigs(t *testing.T) {
	PatchConvey("TestGetTopLevelAndServerSecondLevelConfigs", t, func() {
		vp := viper.New()
		vp.Set("server", map[string]any{
			"s1":       1,
			"s2":       "2",
			"s3":       []string{"3", "33"},
			"s4":       map[string]any{"s41": 41, "s51": "51"},
			"profiles": "ppp",
		})
		vp.Set("x", "xxx")
		vp.Set("y", "yyy")

		res := getTopLevelAndServerSecondLevelConfigs(vp)
		So(res, ShouldResemble, map[string]any{
			"server.s1": 1,
			"server.s2": "2",
			"server.s3": []string{"3", "33"},
			"server.s4": map[string]any{"s41": 41, "s51": "51"},
			"x":         "xxx",
			"y":         "yyy",
		})
	})
}

// ==================== xconfig_init.go (env placeholders) ====================

func TestExpandEnvPlaceholder(t *testing.T) {
	PatchConvey("TestExpandEnvPlaceholder", t, func() {
		PatchConvey("WithEnvVar", func() {
			os.Setenv("TEST_EXPAND", "expanded")
			defer os.Unsetenv("TEST_EXPAND")
			So(expandEnvPlaceholder("${TEST_EXPAND}"), ShouldEqual, "expanded")
		})

		PatchConvey("WithDefault", func() {
			os.Unsetenv("NONEXISTENT_VAR")
			So(expandEnvPlaceholder("${NONEXISTENT_VAR:-fallback}"), ShouldEqual, "fallback")
		})

		PatchConvey("NoPlaceholder", func() {
			So(expandEnvPlaceholder("plain_value"), ShouldEqual, "plain_value")
		})

		PatchConvey("EmptyDefault", func() {
			os.Unsetenv("EMPTY_DEFAULT_VAR")
			So(expandEnvPlaceholder("${EMPTY_DEFAULT_VAR}"), ShouldEqual, "")
		})

		PatchConvey("FindStringSubmatchShort", func() {
			// mock FindStringSubmatch 返回不足 2 个元素，覆盖防御性分支
			Mock((*regexp.Regexp).FindStringSubmatch).Return([]string{"${VAR}"}).Build()
			So(expandEnvPlaceholder("${VAR}"), ShouldEqual, "${VAR}")
		})
	})
}

func TestExpandEnvPlaceholders(t *testing.T) {
	PatchConvey("TestExpandEnvPlaceholders", t, func() {
		PatchConvey("WithEnvVar", func() {
			os.Setenv("TEST_VAR", "test_value")
			defer os.Unsetenv("TEST_VAR")

			vp := viper.New()
			vp.Set("key", "${TEST_VAR}")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "test_value")
		})

		PatchConvey("WithDefault", func() {
			os.Unsetenv("NONEXISTENT_VAR")
			vp := viper.New()
			vp.Set("key", "${NONEXISTENT_VAR:-default_value}")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "default_value")
		})

		PatchConvey("NoPlaceholder", func() {
			vp := viper.New()
			vp.Set("key", "plain_value")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "plain_value")
		})

		PatchConvey("EmptyValue", func() {
			vp := viper.New()
			vp.Set("key", "")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "")
		})

		PatchConvey("NestedKey", func() {
			os.Setenv("NESTED_VAR", "nested_val")
			defer os.Unsetenv("NESTED_VAR")

			vp := viper.New()
			vp.Set("a.b.c", "${NESTED_VAR}")
			expandEnvPlaceholders(vp)
			So(vp.GetString("a.b.c"), ShouldEqual, "nested_val")
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
