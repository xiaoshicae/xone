package xconfig

import (
	"os"
	"testing"

	"github.com/xiaoshicae/xone/xutil"

	"github.com/spf13/viper"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGinSwaggerConfigMergeDefault(t *testing.T) {
	PatchConvey("TestGinSwaggerConfigMergeDefault-Nil", t, func() {
		gc := ginSwaggerConfigMergeDefault(nil)
		So(gc, ShouldResemble, &GinSwagger{
			Schemes: []string{"https", "http"},
		})
	})

	PatchConvey("TestGinSwaggerConfigMergeDefault-NotNil", t, func() {
		gc := ginSwaggerConfigMergeDefault(&GinSwagger{
			Host:        "1",
			BasePath:    "2",
			Title:       "3",
			Description: "4",
			Schemes:     []string{"5", "6"},
		})
		So(gc, ShouldResemble, &GinSwagger{
			Host:        "1",
			BasePath:    "2",
			Title:       "3",
			Description: "4",
			Schemes:     []string{"5", "6"},
		})
	})
}

func TestGinConfigMergeDefault(t *testing.T) {
	PatchConvey("TestGinConfigMergeDefault-Nil", t, func() {
		gc := ginConfigMergeDefault(nil)
		So(gc, ShouldResemble, &Gin{
			Host:       "0.0.0.0",
			Port:       8000,
			UseHttp2:   false,
			GinSwagger: nil,
		})
	})

	PatchConvey("TestGinConfigMergeDefault-NotNil", t, func() {
		gc := ginConfigMergeDefault(&Gin{
			Host:     "1",
			Port:     2,
			UseHttp2: true,
			GinSwagger: &GinSwagger{
				Host:        "1",
				BasePath:    "2",
				Title:       "3",
				Description: "4",
				Schemes:     []string{"5", "6"},
			},
		})
		So(gc, ShouldResemble, &Gin{
			Host:     "1",
			Port:     2,
			UseHttp2: true,
			GinSwagger: &GinSwagger{
				Host:        "1",
				BasePath:    "2",
				Title:       "3",
				Description: "4",
				Schemes:     []string{"5", "6"},
			},
		})
	})
}

func TestServerConfigMergeDefault(t *testing.T) {
	PatchConvey("TestServerConfigMergeDefault-Nil", t, func() {
		sc := serverConfigMergeDefault(nil)
		So(sc, ShouldResemble, &Server{
			Name:     "",
			Version:  "v0.0.1",
			Profiles: nil,
			Gin:      nil,
		})
	})

	PatchConvey("TestServerConfigMergeDefault-NotNil", t, func() {
		sc := serverConfigMergeDefault(&Server{
			Name:    "1",
			Version: "2",
			Profiles: &Profiles{
				Active: "3",
			},
			Gin: nil,
		})
		So(sc, ShouldResemble, &Server{
			Name:    "1",
			Version: "2",
			Profiles: &Profiles{
				Active: "3",
			},
			Gin: nil,
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
