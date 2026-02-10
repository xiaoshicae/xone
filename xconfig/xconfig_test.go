package xconfig

import (
	"os"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/spf13/viper"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestServerConfigMergeDefault(t *testing.T) {
	PatchConvey("TestServerConfigMergeDefault-Nil", t, func() {
		sc := serverConfigMergeDefault(nil)
		So(sc, ShouldResemble, &Server{
			Name:     "",
			Version:  "v0.0.1",
			Profiles: nil,
		})
	})

	PatchConvey("TestServerConfigMergeDefault-NotNil", t, func() {
		sc := serverConfigMergeDefault(&Server{
			Name:    "1",
			Version: "2",
			Profiles: &Profiles{
				Active: "3",
			},
		})
		So(sc, ShouldResemble, &Server{
			Name:    "1",
			Version: "2",
			Profiles: &Profiles{
				Active: "3",
			},
		})
	})
}

func TestGetProfilesActiveFromArg(t *testing.T) {
	PatchConvey("TestGetProfilesActiveFromArg", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("a", nil).Build()
		a := getProfilesActiveFromArg()
		So(a, ShouldEqual, "a")
	})
}

func TestGetProfilesActiveFromENV(t *testing.T) {
	PatchConvey("TestGetProfilesActiveFromENV", t, func() {
		a := getProfilesActiveFromENV()
		So(a, ShouldEqual, "")

		err := os.Setenv("SERVER_PROFILES_ACTIVE", "xxx")
		So(err, ShouldBeNil)

		a = getProfilesActiveFromENV()
		So(a, ShouldEqual, "xxx")
	})
}

func TestGetProfilesActiveFromViperConfig(t *testing.T) {
	PatchConvey("TestGetProfilesActiveFromViperConfig", t, func() {
		a := getProfilesActiveFromViperConfig(nil)
		So(a, ShouldEqual, "")

		vp := viper.New()
		a = getProfilesActiveFromViperConfig(vp)
		So(a, ShouldEqual, "")

		vp.Set("Server.profiles.active", "dev")
		a = getProfilesActiveFromViperConfig(vp)
		So(a, ShouldEqual, "dev")
	})
}

func TestGetProfilesActiveWithEnvPlaceholder(t *testing.T) {
	PatchConvey("TestGetProfilesActiveWithEnvPlaceholder", t, func() {
		// 设置环境变量
		os.Setenv("PROFILES_ACTIVE", "test")
		defer os.Unsetenv("PROFILES_ACTIVE")

		// 模拟配置文件中使用占位符 ${PROFILES_ACTIVE}
		vp := viper.New()
		vp.Set("Server.Profiles.Active", "${PROFILES_ACTIVE}")

		// 展开前，getProfilesActiveFromViperConfig 返回原始占位符字符串
		a := getProfilesActiveFromViperConfig(vp)
		So(a, ShouldEqual, "${PROFILES_ACTIVE}")

		// 调用 expandEnvPlaceholders 展开占位符
		expandEnvPlaceholders(vp)

		// 展开后，getProfilesActiveFromViperConfig 返回环境变量的值
		a = getProfilesActiveFromViperConfig(vp)
		So(a, ShouldEqual, "test")
	})

	PatchConvey("TestGetProfilesActiveWithEnvPlaceholderDefault", t, func() {
		// 不设置环境变量，测试默认值
		os.Unsetenv("PROFILES_ACTIVE_NOT_SET")

		vp := viper.New()
		vp.Set("Server.Profiles.Active", "${PROFILES_ACTIVE_NOT_SET:-dev}")

		// 调用 expandEnvPlaceholders 展开占位符
		expandEnvPlaceholders(vp)

		// 应该返回默认值 dev
		a := getProfilesActiveFromViperConfig(vp)
		So(a, ShouldEqual, "dev")
	})
}

func TestToProfilesActiveConfigLocation(t *testing.T) {
	PatchConvey("TestToProfilesActiveConfigLocation", t, func() {
		location, err := toProfilesActiveConfigLocation("x", "")
		So(err, ShouldNotBeNil)
		So(location, ShouldBeEmpty)

		location, err = toProfilesActiveConfigLocation("x.yml", "a")
		So(err, ShouldBeNil)
		So(location, ShouldEqual, "x-a.yml")

		location, err = toProfilesActiveConfigLocation("./x.yml", "a")
		So(err, ShouldBeNil)
		So(location, ShouldEqual, "./x-a.yml")

		location, err = toProfilesActiveConfigLocation("/a/b/x.yml", "a")
		So(err, ShouldBeNil)
		So(location, ShouldEqual, "/a/b/x-a.yml")
	})
}

func TestGetLocationFromArgWithNoArg(t *testing.T) {
	PatchConvey("TestGetLocationFromArgWithNoArg", t, func() {
		location := getLocationFromArg()
		t.Log("1location: ", location)
		So(location, ShouldBeEmpty)
	})
}

func TestGetLocationFromENV(t *testing.T) {
	PatchConvey("getLocationFromENV", t, func() {
		location := getLocationFromENV()
		So(location, ShouldBeEmpty)

		err := os.Setenv("SERVER_CONFIG_LOCATION", "/a/b/c/application.yml")
		So(err, ShouldBeNil)

		location = getLocationFromENV()
		So(location, ShouldEqual, "/a/b/c/application.yml")
	})
}

func TestGetLocationFromCurrentDir(t *testing.T) {
	PatchConvey("TestGetLocationFromCurrentDir", t, func() {
		location := getLocationFromCurrentDir()
		So(location, ShouldBeEmpty)

		PatchConvey("TestGetLocationFromCurrentDir-FoundInCurrentDir", func() {
			filePath := "application.yml"

			file, err := os.Create(filePath)
			defer func() {
				_ = file.Close()
				_ = os.Remove(filePath)
			}()
			So(err, ShouldBeNil)

			location = getLocationFromCurrentDir()
			So(location, ShouldEqual, "./application.yml")
		})

		PatchConvey("TestGetLocationFromCurrentDir-FoundInCurrentConfDir", func() {
			err := os.MkdirAll("./conf", 0755) // 0755 是目录权限
			So(err, ShouldBeNil)

			filePath := "./conf/application.yml"
			file, err := os.Create(filePath)
			defer func() {
				_ = file.Close()
				_ = os.Remove(filePath)
				_ = os.RemoveAll("./conf")
			}()
			So(err, ShouldBeNil)

			location = getLocationFromCurrentDir()
			So(location, ShouldEqual, "./conf/application.yml")
		})

		PatchConvey("TestGetLocationFromCurrentDir-FoundInCurrentConfigDir", func() {
			err := os.MkdirAll("./config", 0755) // 0755 是目录权限
			So(err, ShouldBeNil)

			filePath := "./config/application.yml"
			file, err := os.Create(filePath)
			defer func() {
				_ = file.Close()
				_ = os.Remove(filePath)
				_ = os.RemoveAll("./config")
			}()
			So(err, ShouldBeNil)

			location = getLocationFromCurrentDir()
			So(location, ShouldEqual, "./config/application.yml")
		})

	})
}

func TestInitConfig(t *testing.T) {
	PatchConvey("TestInitConfig", t, func() {
		Mock(detectConfigLocation).Return("/a/b.yaml").Build()
		Mock(loadDotEnvIfExist).Return(nil).Build()
		Mock(parseConfig).Return(viper.New(), nil).Build()
		err := initXConfig()
		So(err, ShouldBeNil)
	})
}

func TestLoadDotEnvIfExist(t *testing.T) {
	PatchConvey("TestLoadDotEnvIfExist", t, func() {
		err := loadDotEnvIfExist("/a/b.yaml")
		So(err, ShouldBeNil)
	})
}

func TestParseConfig(t *testing.T) {
	PatchConvey("TestParseConfig", t, func() {
		vpConfig := viper.New()
		vpConfig.Set("server", map[string]interface{}{
			"s1": 1,
			"s2": "2",
			"s3": []string{"3", "33"},
			"s4": map[string]interface{}{
				"s41": 41,
				"s51": "51",
			},
			"profiles": map[string]interface{}{
				"active": "xxx",
			},
		})
		vpConfig.Set("x", "x1")
		vpConfig.Set("y", "y1")

		Mock(loadLocalConfig).Return(vpConfig, nil).Build()
		Mock(detectProfilesActive).Return("xx").Build()

		parseConfig, err := parseConfig("/a/b.yml")
		So(err, ShouldBeNil)
		So(parseConfig.AllSettings(), ShouldResemble, map[string]interface{}{
			"server": map[string]interface{}{
				"s1": 1,
				"s2": "2",
				"s3": []string{"3", "33"},
				"s4": map[string]interface{}{
					"s41": 41,
					"s51": "51",
				},
				"profiles": map[string]interface{}{
					"active": "xxx",
				},
			},
			"x": "x1",
			"y": "y1",
		})
	})
}

func TestParseConfigProfilesActiveFileMissing(t *testing.T) {
	PatchConvey("TestParseConfig-ProfilesActiveFileMissing", t, func() {
		vpConfig := viper.New()
		vpConfig.Set("server", map[string]interface{}{
			"s1": 1,
			"s2": "2",
			"s3": []string{"3", "33"},
			"s4": map[string]interface{}{
				"s41": 41,
				"s51": "51",
			},
			"profiles": map[string]interface{}{
				"active": "xxx",
			},
		})
		vpConfig.Set("x", "x1")
		vpConfig.Set("y", "y1")

		Mock(loadLocalConfig).Return(vpConfig, nil).Build()
		Mock(detectProfilesActive).Return("dev").Build()
		Mock(xutil.FileExist).Return(false).Build()

		parseConfig, err := parseConfig("/a/b.yml")
		So(err, ShouldBeNil)
		So(parseConfig.AllSettings(), ShouldResemble, map[string]interface{}{
			"server": map[string]interface{}{
				"s1": 1,
				"s2": "2",
				"s3": []string{"3", "33"},
				"s4": map[string]interface{}{
					"s41": 41,
					"s51": "51",
				},
				"profiles": map[string]interface{}{
					"active": "xxx",
				},
			},
			"x": "x1",
			"y": "y1",
		})
	})
}

func TestPrintFinalConfig(t *testing.T) {
	vp := viper.New()
	vp.Set("k", "v")
	printFinalConfig(vp)
}

func TestMergeProfilesViperConfig(t *testing.T) {
	PatchConvey("TestMergeProfilesViperConfig", t, func() {
		vp1 := viper.New()
		vp1.Set("server", map[string]interface{}{
			"s1": 1,
			"s2": "2",
			"s3": []string{"3", "33"},
			"s4": map[string]interface{}{
				"s41": 41,
				"s51": "51",
			},
			"profiles": "p1",
		})
		vp1.Set("x", "x1")
		vp1.Set("y", "y1")

		vp2 := viper.New()
		vp2.Set("server", map[string]interface{}{
			"s1": 11,
			"s3": []string{"33", "44"},
			"s4": map[string]interface{}{
				"s41": 441,
				"651": "651",
			},
			"profiles": "p2",
		})
		vp2.Set("x", "x2")
		vp2.Set("z", "z2")

		vp := mergeProfilesViperConfig(vp1, vp2)
		So(vp.AllSettings(), ShouldResemble, map[string]interface{}{
			"server": map[string]interface{}{
				"s1": 11,
				"s2": "2",
				"s3": []string{"33", "44"},
				"s4": map[string]interface{}{
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
		vp.Set("server", map[string]interface{}{
			"s1": 1,
			"s2": "2",
			"s3": []string{"3", "33"},
			"s4": map[string]interface{}{
				"s41": 41,
				"s51": "51",
			},
			"profiles": "ppp",
		})

		vp.Set("x", "xxx")
		vp.Set("y", "yyy")

		res := getTopLevelConfigs(vp)
		So(res, ShouldResemble, map[string]interface{}{
			"server": map[string]interface{}{
				"s1": 1,
				"s2": "2",
				"s3": []string{"3", "33"},
				"s4": map[string]interface{}{
					"s41": 41,
					"s51": "51",
				},
				"profiles": "ppp",
			},
			"x": "xxx",
			"y": "yyy",
		})
	})
}

func TestIsNestedKey(t *testing.T) {
	PatchConvey("TestIsNestedKey", t, func() {
		i := isNestedKey("")
		So(i, ShouldBeFalse)

		i = isNestedKey("1")
		So(i, ShouldBeFalse)

		i = isNestedKey(".")
		So(i, ShouldBeTrue)

		i = isNestedKey(".1")
		So(i, ShouldBeTrue)

		i = isNestedKey("1.1")
		So(i, ShouldBeTrue)
	})
}

func TestGetTopLevelAndServerSecondLevelConfigs(t *testing.T) {
	PatchConvey("TestGetTopLevelAndServerSecondLevelConfigs", t, func() {
		vp := viper.New()
		vp.Set("server", map[string]interface{}{
			"s1": 1,
			"s2": "2",
			"s3": []string{"3", "33"},
			"s4": map[string]interface{}{
				"s41": 41,
				"s51": "51",
			},
			"profiles": "ppp",
		})

		vp.Set("x", "xxx")
		vp.Set("y", "yyy")

		res := getTopLevelAndServerSecondLevelConfigs(vp)
		So(res, ShouldResemble, map[string]interface{}{
			"server.s1": 1,
			"server.s2": "2",
			"server.s3": []string{"3", "33"},
			"server.s4": map[string]interface{}{
				"s41": 41,
				"s51": "51",
			},
			"x": "xxx",
			"y": "yyy",
		})
	})
}

// go test -run ^TestGetLocationFromArgWithArg$ -args server.config.location=/a/b/application.yml
func TestGetLocationFromArgWithArg(t *testing.T) {
	t.Skipf("如果需要测试，通过启动参数指定配置文件位置，请注释后，手动运行上面参数")

	PatchConvey("TestGetLocationFromArgWithArg", t, func() {
		location := getLocationFromArg()
		t.Log("location: ", location)
		So(location, ShouldEqual, "/a/b/application.yml")
	})
}

func TestCheckParam(t *testing.T) {
	PatchConvey("TestCheckParam", t, func() {
		PatchConvey("TestCheckParam-EmptyKey", func() {
			err := checkParam("", nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "key is empty")
		})

		PatchConvey("TestCheckParam-NilConf", func() {
			err := checkParam("key", nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "conf is nil")
		})

		PatchConvey("TestCheckParam-NotPtrConf", func() {
			conf := struct{}{}
			err := checkParam("key", conf)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "conf is not ptr")
		})

		PatchConvey("TestCheckParam-Valid", func() {
			conf := &struct{}{}
			err := checkParam("key", conf)
			So(err, ShouldBeNil)
		})
	})
}

func TestGetViperConfig(t *testing.T) {
	PatchConvey("TestGetViperConfig-Nil", t, func() {
		vip = nil
		config := getViperConfig()
		So(config, ShouldNotBeNil)
	})

	PatchConvey("TestGetViperConfig-NotNil", t, func() {
		vip = viper.New()
		vip.Set("test", "value")
		config := getViperConfig()
		So(config, ShouldNotBeNil)
		So(config.GetString("test"), ShouldEqual, "value")
	})
}

func TestUtilFunctions(t *testing.T) {
	PatchConvey("TestUtilFunctions", t, func() {
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

		PatchConvey("TestGetConfig", func() {
			result := GetConfig("string_key")
			So(result, ShouldEqual, "string_value")
		})

		PatchConvey("TestContainKey", func() {
			So(ContainKey("string_key"), ShouldBeTrue)
			So(ContainKey("nonexistent"), ShouldBeFalse)
		})

		PatchConvey("TestGetString", func() {
			So(GetString("string_key"), ShouldEqual, "string_value")
		})

		PatchConvey("TestGetBool", func() {
			So(GetBool("bool_key"), ShouldBeTrue)
		})

		PatchConvey("TestGetInt", func() {
			So(GetInt("int_key"), ShouldEqual, 42)
		})

		PatchConvey("TestGetInt32", func() {
			So(GetInt32("int32_key"), ShouldEqual, int32(32))
		})

		PatchConvey("TestGetInt64", func() {
			So(GetInt64("int64_key"), ShouldEqual, int64(64))
		})

		PatchConvey("TestGetFloat64", func() {
			So(GetFloat64("float64_key"), ShouldEqual, 3.14)
		})

		PatchConvey("TestGetDuration", func() {
			So(GetDuration("duration_key"), ShouldEqual, time.Second)
		})

		PatchConvey("TestGetStringSlice", func() {
			So(GetStringSlice("string_slice_key"), ShouldResemble, []string{"a", "b"})
		})

		PatchConvey("TestGetIntSlice", func() {
			So(GetIntSlice("int_slice_key"), ShouldResemble, []int{1, 2, 3})
		})
	})
}

func TestServerConfigFunctions(t *testing.T) {
	PatchConvey("TestServerConfigFunctions", t, func() {
		PatchConvey("TestGetServerName-Default", func() {
			vip = viper.New()
			name := GetServerName()
			So(name, ShouldEqual, defaultServerName)
		})

		PatchConvey("TestGetServerName-Custom", func() {
			vip = viper.New()
			vip.Set("Server.Name", "custom-server")
			name := GetServerName()
			So(name, ShouldEqual, "custom-server")
		})

		PatchConvey("TestGetRawServerName", func() {
			vip = viper.New()
			vip.Set("Server.Name", "raw-server")
			name := GetRawServerName()
			So(name, ShouldEqual, "raw-server")
		})

		PatchConvey("TestGetServerVersion-Default", func() {
			vip = viper.New()
			version := GetServerVersion()
			So(version, ShouldEqual, defaultServerVersion)
		})

		PatchConvey("TestGetServerVersion-Custom", func() {
			vip = viper.New()
			vip.Set("Server.Version", "v1.0.0")
			version := GetServerVersion()
			So(version, ShouldEqual, "v1.0.0")
		})
	})
}

func TestUnmarshalConfig(t *testing.T) {
	PatchConvey("TestUnmarshalConfig", t, func() {
		PatchConvey("TestUnmarshalConfig-InvalidKey", func() {
			var conf struct{}
			err := UnmarshalConfig("", &conf)
			So(err, ShouldNotBeNil)
		})

		PatchConvey("TestUnmarshalConfig-Valid", func() {
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

func TestDetectConfigLocation(t *testing.T) {
	PatchConvey("TestDetectConfigLocation-FromArg", t, func() {
		Mock(getLocationFromArg).Return("/from/arg.yml").Build()
		location := detectConfigLocation()
		So(location, ShouldEqual, "/from/arg.yml")
	})

	PatchConvey("TestDetectConfigLocation-FromENV", t, func() {
		Mock(getLocationFromArg).Return("").Build()
		Mock(getLocationFromENV).Return("/from/env.yml").Build()
		location := detectConfigLocation()
		So(location, ShouldEqual, "/from/env.yml")
	})

	PatchConvey("TestDetectConfigLocation-FromCurrentDir", t, func() {
		Mock(getLocationFromArg).Return("").Build()
		Mock(getLocationFromENV).Return("").Build()
		Mock(getLocationFromCurrentDir).Return("./application.yml").Build()
		location := detectConfigLocation()
		So(location, ShouldEqual, "./application.yml")
	})
}

func TestDetectProfilesActive(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-FromArg", t, func() {
		Mock(getProfilesActiveFromArg).Return("dev").Build()
		profile := detectProfilesActive(nil)
		So(profile, ShouldEqual, "dev")
	})

	PatchConvey("TestDetectProfilesActive-FromENV", t, func() {
		Mock(getProfilesActiveFromArg).Return("").Build()
		Mock(getProfilesActiveFromENV).Return("prod").Build()
		profile := detectProfilesActive(nil)
		So(profile, ShouldEqual, "prod")
	})

	PatchConvey("TestDetectProfilesActive-FromViperConfig", t, func() {
		Mock(getProfilesActiveFromArg).Return("").Build()
		Mock(getProfilesActiveFromENV).Return("").Build()

		vp := viper.New()
		vp.Set("Server.profiles.active", "test")
		profile := detectProfilesActive(vp)
		So(profile, ShouldEqual, "test")
	})
}

func TestSetNestedValue(t *testing.T) {
	PatchConvey("TestSetNestedValue", t, func() {
		PatchConvey("TestSetNestedValue-SimpleKey", func() {
			m := make(map[string]interface{})
			setNestedValue(m, "key", "value")
			So(m["key"], ShouldEqual, "value")
		})

		PatchConvey("TestSetNestedValue-NestedKey", func() {
			m := make(map[string]interface{})
			setNestedValue(m, "a.b.c", "value")
			a := m["a"].(map[string]interface{})
			b := a["b"].(map[string]interface{})
			So(b["c"], ShouldEqual, "value")
		})

		PatchConvey("TestSetNestedValue-ExistingNested", func() {
			m := map[string]interface{}{
				"a": map[string]interface{}{
					"existing": "value",
				},
			}
			setNestedValue(m, "a.new", "newvalue")
			a := m["a"].(map[string]interface{})
			So(a["existing"], ShouldEqual, "value")
			So(a["new"], ShouldEqual, "newvalue")
		})
	})
}

func TestExpandEnvPlaceholders(t *testing.T) {
	PatchConvey("TestExpandEnvPlaceholders", t, func() {
		PatchConvey("TestExpandEnvPlaceholders-WithEnvVar", func() {
			os.Setenv("TEST_VAR", "test_value")
			defer os.Unsetenv("TEST_VAR")

			vp := viper.New()
			vp.Set("key", "${TEST_VAR}")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "test_value")
		})

		PatchConvey("TestExpandEnvPlaceholders-WithDefault", func() {
			vp := viper.New()
			vp.Set("key", "${NONEXISTENT_VAR:-default_value}")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "default_value")
		})

		PatchConvey("TestExpandEnvPlaceholders-NoPlaceholder", func() {
			vp := viper.New()
			vp.Set("key", "plain_value")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "plain_value")
		})

		PatchConvey("TestExpandEnvPlaceholders-EmptyValue", func() {
			vp := viper.New()
			vp.Set("key", "")
			expandEnvPlaceholders(vp)
			So(vp.GetString("key"), ShouldEqual, "")
		})
	})
}

func TestLoadLocalConfig(t *testing.T) {
	PatchConvey("TestLoadLocalConfig-FileNotFound", t, func() {
		vp, err := loadLocalConfig("/nonexistent/path.yml")
		So(err, ShouldNotBeNil)
		So(vp, ShouldBeNil)
	})
}
