package xconfig

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"

	"github.com/spf13/viper"
	"github.com/xiaoshicae/xone/v3/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== 测试辅助 ====================

// writeFile 在 dir 下写一个配置文件并返回完整路径
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s failed: %v", p, err)
	}
	return p
}

// resetStore 清空全局存储，避免用例之间相互影响
func resetStore() {
	storeMu.Lock()
	store, server, sources = nil, serverMergeDefault(Server{}), nil
	storeMu.Unlock()
}

// loadFrom 走一遍完整加载流程，返回合并展开后的配置树与来源列表
func loadFrom(t *testing.T, location string) (map[string]any, []string) {
	t.Helper()
	base, err := decodeFile(location)
	if err != nil {
		t.Fatalf("decodeFile failed: %v", err)
	}
	active, err := detectProfilesActive(base)
	if err != nil {
		t.Fatalf("detectProfilesActive failed: %v", err)
	}
	layers, imports, err := buildLayers(location, base, active)
	if err != nil {
		t.Fatalf("buildLayers failed: %v", err)
	}
	settings := mergeLayers(layers)
	annotate(settings, active, imports)
	settings, err = expandPlaceholders(settings)
	if err != nil {
		t.Fatalf("expandPlaceholders failed: %v", err)
	}

	from := make([]string, 0, len(layers))
	for _, l := range layers {
		from = append(from, l.from)
	}
	return settings, from
}

// ==================== merge.go ====================

func TestDeepMerge_NestedOverride(t *testing.T) {
	PatchConvey("TestDeepMerge-嵌套逐层覆盖", t, func() {
		base := map[string]any{"xlog": map[string]any{"level": "info", "path": "./log"}}
		override := map[string]any{"xlog": map[string]any{"level": "debug"}}

		merged := deepMerge(base, override)
		So(merged["xlog"], ShouldResemble, map[string]any{"level": "debug", "path": "./log"})
	})
}

func TestDeepMerge_ListReplaced(t *testing.T) {
	PatchConvey("TestDeepMerge-列表整体替换", t, func() {
		base := map[string]any{"headers": []any{"a", "b", "c"}}
		override := map[string]any{"headers": []any{"x"}}

		merged := deepMerge(base, override)
		So(merged["headers"], ShouldResemble, []any{"x"})
	})
}

func TestDeepMerge_TypeMismatchReplaced(t *testing.T) {
	PatchConvey("TestDeepMerge-两侧类型不同时整体替换", t, func() {
		So(deepMerge(map[string]any{"k": "scalar"},
			map[string]any{"k": map[string]any{"a": 1}})["k"],
			ShouldResemble, map[string]any{"a": 1})

		So(deepMerge(map[string]any{"k": map[string]any{"a": 1}},
			map[string]any{"k": "scalar"})["k"], ShouldEqual, "scalar")
	})
}

func TestDeepMerge_NotMutateInput(t *testing.T) {
	PatchConvey("TestDeepMerge-不改动入参", t, func() {
		base := map[string]any{"a": map[string]any{"b": 1}}
		deepMerge(base, map[string]any{"a": map[string]any{"b": 2}, "c": 3})

		So(base["a"], ShouldResemble, map[string]any{"b": 1})
		So(base, ShouldNotContainKey, "c")
	})
}

func TestLookupKeyPath_Found(t *testing.T) {
	PatchConvey("TestLookupKeyPath-命中", t, func() {
		s := map[string]any{"server": map[string]any{"profiles": map[string]any{"active": "dev"}}}
		So(lookupKeyPath(s, "server", "profiles", "active"), ShouldEqual, "dev")
	})
}

func TestLookupKeyPath_Missing(t *testing.T) {
	PatchConvey("TestLookupKeyPath-路径不存在", t, func() {
		s := map[string]any{"server": map[string]any{"name": "x"}}
		So(lookupKeyPath(s, "server", "profiles", "active"), ShouldBeNil)
		So(lookupKeyPath(s, "nope"), ShouldBeNil)
		// 中间节点不是 map
		So(lookupKeyPath(s, "server", "name", "deeper"), ShouldBeNil)
	})
}

func TestLookupKeyPath_EmptyPath(t *testing.T) {
	PatchConvey("TestLookupKeyPath-空路径", t, func() {
		So(lookupKeyPath(map[string]any{"a": 1}), ShouldBeNil)
	})
}

func TestSetKeyPath_CreateIntermediate(t *testing.T) {
	PatchConvey("TestSetKeyPath-自动创建中间节点", t, func() {
		s := map[string]any{}
		setKeyPath(s, "dev", "server", "profiles", "active")
		So(s, ShouldResemble, map[string]any{
			"server": map[string]any{"profiles": map[string]any{"active": "dev"}},
		})
	})
}

func TestSetKeyPath_ReplaceNonMap(t *testing.T) {
	PatchConvey("TestSetKeyPath-中间节点类型不符时整体替换", t, func() {
		s := map[string]any{"server": "scalar"}
		setKeyPath(s, "dev", "server", "profiles", "active")
		So(lookupKeyPath(s, "server", "profiles", "active"), ShouldEqual, "dev")
	})
}

func TestSetKeyPath_EmptyPath(t *testing.T) {
	PatchConvey("TestSetKeyPath-空路径不做任何事", t, func() {
		s := map[string]any{"a": 1}
		setKeyPath(s, "v")
		So(s, ShouldResemble, map[string]any{"a": 1})
	})
}

func TestDeleteKeyPath_CleanEmptyParents(t *testing.T) {
	PatchConvey("TestDeleteKeyPath-清理因此变空的父节点", t, func() {
		s := map[string]any{"server": map[string]any{"config": map[string]any{"import": []any{"db.yml"}}}}
		deleteKeyPath(s, "server", "config", "import")
		So(s, ShouldBeEmpty)
	})
}

func TestDeleteKeyPath_KeepNonEmptyParent(t *testing.T) {
	PatchConvey("TestDeleteKeyPath-父节点还有别的内容则保留", t, func() {
		s := map[string]any{"server": map[string]any{
			"name":     "x",
			"profiles": map[string]any{"active": "dev"},
		}}
		deleteKeyPath(s, "server", "profiles")
		So(s, ShouldResemble, map[string]any{"server": map[string]any{"name": "x"}})
	})
}

func TestDeleteKeyPath_MissingPath(t *testing.T) {
	PatchConvey("TestDeleteKeyPath-路径不存在时保持原样", t, func() {
		s := map[string]any{"xlog": map[string]any{"level": "info"}}
		deleteKeyPath(s, "server", "profiles")
		deleteKeyPath(s)
		So(s, ShouldResemble, map[string]any{"xlog": map[string]any{"level": "info"}})
	})
}

func TestLowerKeys_Nested(t *testing.T) {
	PatchConvey("TestLowerKeys-递归小写化", t, func() {
		s := lowerKeys(map[string]any{"XLog": map[string]any{"Level": "Info"}})
		So(s, ShouldResemble, map[string]any{"xlog": map[string]any{"level": "Info"}})
	})
}

func TestLowerKeys_InsideList(t *testing.T) {
	PatchConvey("TestLowerKeys-穿过列表", t, func() {
		s := lowerKeys(map[string]any{"XRedis": []any{
			map[string]any{"Name": "cache", "Addr": "127.0.0.1:6379"},
		}})
		So(s["xredis"], ShouldResemble, []any{
			map[string]any{"name": "cache", "addr": "127.0.0.1:6379"},
		})
	})
}

func TestDistinct_KeepOrder(t *testing.T) {
	PatchConvey("TestDistinct-去重并保持顺序", t, func() {
		So(distinct([]string{"b", "a", "b", "c", "a"}), ShouldResemble, []string{"b", "a", "c"})
		So(distinct(nil), ShouldBeEmpty)
	})
}

// ==================== placeholder.go ====================

func TestResolverExpand_EnvSet(t *testing.T) {
	PatchConvey("TestResolverExpand-环境变量已设置", t, func() {
		os.Setenv("XCONFIG_T1", "hello")
		defer os.Unsetenv("XCONFIG_T1")

		r := &resolver{}
		So(r.expand("v=${XCONFIG_T1}"), ShouldEqual, "v=hello")
		So(r.err("op"), ShouldBeNil)
	})
}

func TestResolverExpand_Default(t *testing.T) {
	PatchConvey("TestResolverExpand-用默认值兜底", t, func() {
		os.Unsetenv("XCONFIG_T2")

		r := &resolver{}
		So(r.expand("${XCONFIG_T2:fallback}"), ShouldEqual, "fallback")
		So(r.err("op"), ShouldBeNil)
	})
}

func TestResolverExpand_EmptyDefault(t *testing.T) {
	PatchConvey("TestResolverExpand-默认值为空串", t, func() {
		os.Unsetenv("XCONFIG_T3")

		r := &resolver{}
		So(r.expand("${XCONFIG_T3:}"), ShouldBeEmpty)
		So(r.err("op"), ShouldBeNil)
	})
}

func TestResolverExpand_EmptyEnvOverridesDefault(t *testing.T) {
	PatchConvey("TestResolverExpand-显式空串覆盖默认值", t, func() {
		os.Setenv("XCONFIG_T4", "")
		defer os.Unsetenv("XCONFIG_T4")

		r := &resolver{}
		So(r.expand("${XCONFIG_T4:fallback}"), ShouldBeEmpty)
	})
}

func TestResolverExpand_DefaultWithColon(t *testing.T) {
	PatchConvey("TestResolverExpand-默认值里含冒号", t, func() {
		os.Unsetenv("XCONFIG_T5")

		r := &resolver{}
		So(r.expand("${XCONFIG_T5:127.0.0.1:6379}"), ShouldEqual, "127.0.0.1:6379")
		So(r.expand("${XCONFIG_T5:-1}"), ShouldEqual, "-1")
	})
}

func TestResolverExpand_MissingRequired(t *testing.T) {
	PatchConvey("TestResolverExpand-必填变量未设置", t, func() {
		os.Unsetenv("XCONFIG_T6")

		r := &resolver{}
		So(r.expand("${XCONFIG_T6}"), ShouldEqual, "${XCONFIG_T6}")

		err := r.err("op")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "required env placeholder not set")
		So(err.Error(), ShouldContainSubstring, "XCONFIG_T6")
	})
}

func TestResolverExpand_UnsupportedSyntax(t *testing.T) {
	PatchConvey("TestResolverExpand-写法不被支持", t, func() {
		r := &resolver{}
		r.expand("${}")

		err := r.err("op")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "unsupported placeholder syntax")
	})
}

func TestResolverExpand_EmptyString(t *testing.T) {
	PatchConvey("TestResolverExpand-空串原样返回", t, func() {
		r := &resolver{}
		So(r.expand(""), ShouldBeEmpty)
		So(r.err("op"), ShouldBeNil)
	})
}

func TestResolverExpandValue_ListOfMaps(t *testing.T) {
	PatchConvey("TestResolverExpandValue-穿过列表里的 map", t, func() {
		os.Setenv("XCONFIG_T7", "pw")
		defer os.Unsetenv("XCONFIG_T7")

		r := &resolver{}
		got := r.expandValue([]any{map[string]any{"password": "${XCONFIG_T7}"}})
		So(got, ShouldResemble, []any{map[string]any{"password": "pw"}})
	})
}

func TestResolverExpandValue_NonString(t *testing.T) {
	PatchConvey("TestResolverExpandValue-非字符串原样返回", t, func() {
		r := &resolver{}
		So(r.expandValue(42), ShouldEqual, 42)
		So(r.expandValue(true), ShouldEqual, true)
		So(r.expandValue(nil), ShouldBeNil)
	})
}

func TestResolverErr_UnsupportedBeatsMissing(t *testing.T) {
	PatchConvey("TestResolverErr-写法错误优先于变量缺失", t, func() {
		os.Unsetenv("XCONFIG_T8")

		r := &resolver{}
		r.expand("${XCONFIG_T8}")
		r.expand("${}")

		So(r.err("op").Error(), ShouldContainSubstring, "unsupported placeholder syntax")
	})
}

func TestExpandPlaceholders_Tree(t *testing.T) {
	PatchConvey("TestExpandPlaceholders-递归展开整棵树", t, func() {
		os.Setenv("XCONFIG_T9", "prod")
		defer os.Unsetenv("XCONFIG_T9")

		got, err := expandPlaceholders(map[string]any{
			"xmetric": map[string]any{"constlabels": map[string]any{"env": "${XCONFIG_T9}"}},
			"xtrace":  map[string]any{"headers": []any{"${XCONFIG_T9}"}},
			"port":    8080,
		})
		So(err, ShouldBeNil)
		So(lookupKeyPath(got, "xmetric", "constlabels", "env"), ShouldEqual, "prod")
		So(lookupKeyPath(got, "xtrace", "headers"), ShouldResemble, []any{"prod"})
		So(got["port"], ShouldEqual, 8080)
	})
}

func TestExpandPlaceholders_MissingInsideList(t *testing.T) {
	PatchConvey("TestExpandPlaceholders-列表里的 map 缺变量同样报错", t, func() {
		os.Unsetenv("XCONFIG_T10")

		_, err := expandPlaceholders(map[string]any{
			"xredis": []any{map[string]any{"password": "${XCONFIG_T10}"}},
		})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "XCONFIG_T10")
	})
}

func TestExpandPlaceholders_NoDoubleExpand(t *testing.T) {
	PatchConvey("TestExpandPlaceholders-展开结果不再被解释", t, func() {
		os.Setenv("XCONFIG_T11", "${XCONFIG_T12}")
		os.Setenv("XCONFIG_T12", "never")
		defer func() {
			os.Unsetenv("XCONFIG_T11")
			os.Unsetenv("XCONFIG_T12")
		}()

		got, err := expandPlaceholders(map[string]any{"k": "${XCONFIG_T11}"})
		So(err, ShouldBeNil)
		So(got["k"], ShouldEqual, "${XCONFIG_T12}")
	})
}

func TestExpandPlaceholders_NotMutateInput(t *testing.T) {
	PatchConvey("TestExpandPlaceholders-不改动入参", t, func() {
		os.Setenv("XCONFIG_T13", "v")
		defer os.Unsetenv("XCONFIG_T13")

		in := map[string]any{"a": map[string]any{"b": "${XCONFIG_T13}"}}
		got, err := expandPlaceholders(in)
		So(err, ShouldBeNil)
		So(lookupKeyPath(in, "a", "b"), ShouldEqual, "${XCONFIG_T13}")
		So(lookupKeyPath(got, "a", "b"), ShouldEqual, "v")
	})
}

// ==================== source.go：定位与环境探测 ====================

func TestDetectConfigLocation_FromArg(t *testing.T) {
	PatchConvey("TestDetectConfigLocation-来自启动参数", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("/a/b/application.yml", nil).Build()
		So(detectConfigLocation(), ShouldEqual, "/a/b/application.yml")
	})
}

func TestDetectConfigLocation_FromEnv(t *testing.T) {
	PatchConvey("TestDetectConfigLocation-来自环境变量", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Setenv(configLocationEnvKey, "/from/env/application.yml")
		defer os.Unsetenv(configLocationEnvKey)

		So(detectConfigLocation(), ShouldEqual, "/from/env/application.yml")
	})
}

func TestDetectConfigLocation_FromCurrentDir(t *testing.T) {
	PatchConvey("TestDetectConfigLocation-来自约定路径", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(configLocationEnvKey)
		Mock(xutil.FileExist).To(func(p string) bool { return p == "./conf/application.yml" }).Build()

		So(detectConfigLocation(), ShouldEqual, "./conf/application.yml")
	})
}

func TestDetectConfigLocation_NotFound(t *testing.T) {
	PatchConvey("TestDetectConfigLocation-都找不到", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(configLocationEnvKey)
		Mock(xutil.FileExist).Return(false).Build()

		So(detectConfigLocation(), ShouldBeEmpty)
	})
}

func TestDetectProfilesActive_FromArg(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-来自启动参数", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("dev", nil).Build()

		active, err := detectProfilesActive(nil)
		So(err, ShouldBeNil)
		So(active, ShouldEqual, "dev")
	})
}

func TestDetectProfilesActive_FromEnv(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-来自环境变量", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Setenv(profilesActiveEnvKey, "prod")
		defer os.Unsetenv(profilesActiveEnvKey)

		active, err := detectProfilesActive(nil)
		So(err, ShouldBeNil)
		So(active, ShouldEqual, "prod")
	})
}

func TestDetectProfilesActive_FromBaseConfig(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-来自主配置文件", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		base := map[string]any{"server": map[string]any{"profiles": map[string]any{"active": "test"}}}
		active, err := detectProfilesActive(base)
		So(err, ShouldBeNil)
		So(active, ShouldEqual, "test")
	})
}

func TestDetectProfilesActive_ArgBeatsEnv(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-启动参数优先于环境变量", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("fromArg", nil).Build()
		os.Setenv(profilesActiveEnvKey, "fromEnv")
		defer os.Unsetenv(profilesActiveEnvKey)

		active, err := detectProfilesActive(map[string]any{
			"server": map[string]any{"profiles": map[string]any{"active": "fromFile"}},
		})
		So(err, ShouldBeNil)
		So(active, ShouldEqual, "fromArg")
	})
}

func TestDetectProfilesActive_Placeholder(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-值本身是占位符", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		PatchConvey("环境变量已设置", func() {
			os.Setenv("XCONFIG_PA", "staging")
			defer os.Unsetenv("XCONFIG_PA")

			active, err := detectProfilesActive(map[string]any{
				"server": map[string]any{"profiles": map[string]any{"active": "${XCONFIG_PA}"}},
			})
			So(err, ShouldBeNil)
			So(active, ShouldEqual, "staging")
		})

		PatchConvey("走默认值", func() {
			os.Unsetenv("XCONFIG_PA_UNSET")

			active, err := detectProfilesActive(map[string]any{
				"server": map[string]any{"profiles": map[string]any{"active": "${XCONFIG_PA_UNSET:dev}"}},
			})
			So(err, ShouldBeNil)
			So(active, ShouldEqual, "dev")
		})

		PatchConvey("必填变量缺失时报错", func() {
			os.Unsetenv("XCONFIG_PA_MISSING")

			_, err := detectProfilesActive(map[string]any{
				"server": map[string]any{"profiles": map[string]any{"active": "${XCONFIG_PA_MISSING}"}},
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "XCONFIG_PA_MISSING")
		})
	})
}

func TestDetectProfilesActive_InvalidChars(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-非法字符被拦下", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		defer os.Unsetenv(profilesActiveEnvKey)

		for _, bad := range []string{"../../etc/x", "dev/prod", "dev prod", "dev.yml"} {
			os.Setenv(profilesActiveEnvKey, bad)
			_, err := detectProfilesActive(nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "profiles active is invalid")
		}
	})
}

func TestDetectProfilesActive_NotFound(t *testing.T) {
	PatchConvey("TestDetectProfilesActive-未指定任何环境", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		active, err := detectProfilesActive(map[string]any{"xlog": map[string]any{"level": "info"}})
		So(err, ShouldBeNil)
		So(active, ShouldBeEmpty)
	})
}

// ==================== source.go：文件解析 ====================

func TestDecodeFile_Yaml(t *testing.T) {
	PatchConvey("TestDecodeFile-解析 YAML 并小写化 key", t, func() {
		loc := writeFile(t, t.TempDir(), "application.yml", "Server:\n  Name: \"xone.demo.app\"\n")

		got, err := decodeFile(loc)
		So(err, ShouldBeNil)
		So(got, ShouldResemble, map[string]any{"server": map[string]any{"name": "xone.demo.app"}})
	})
}

func TestDecodeFile_Json(t *testing.T) {
	PatchConvey("TestDecodeFile-解析 JSON", t, func() {
		loc := writeFile(t, t.TempDir(), "application.json", `{"Server":{"Name":"xone.demo.app"}}`)

		got, err := decodeFile(loc)
		So(err, ShouldBeNil)
		So(lookupKeyPath(got, "server", "name"), ShouldEqual, "xone.demo.app")
	})
}

func TestDecodeFile_Empty(t *testing.T) {
	PatchConvey("TestDecodeFile-空文件得到空配置", t, func() {
		loc := writeFile(t, t.TempDir(), "application.yml", "")

		got, err := decodeFile(loc)
		So(err, ShouldBeNil)
		So(got, ShouldBeEmpty)
	})
}

func TestDecodeFile_UnsupportedExt(t *testing.T) {
	PatchConvey("TestDecodeFile-不支持的格式", t, func() {
		loc := writeFile(t, t.TempDir(), "application.toml", "a = 1")

		_, err := decodeFile(loc)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "unsupported config file format")
	})
}

func TestDecodeFile_ParseError(t *testing.T) {
	PatchConvey("TestDecodeFile-内容无法解析", t, func() {
		loc := writeFile(t, t.TempDir(), "application.yml", "a:\n\t- broken\n  b: :")

		_, err := decodeFile(loc)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "parse config file failed")
	})
}

func TestDecodeFile_ReadError(t *testing.T) {
	PatchConvey("TestDecodeFile-文件读不到", t, func() {
		_, err := decodeFile(filepath.Join(t.TempDir(), "nope.yml"))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "read config file failed")
	})
}

func TestDecodeFile_DottedKeyPreserved(t *testing.T) {
	PatchConvey("TestDecodeFile-含点的 key 不被拆成多层", t, func() {
		loc := writeFile(t, t.TempDir(), "application.yml",
			"XMetric:\n  ConstLabels:\n    service.name: \"demo\"\n")

		got, err := decodeFile(loc)
		So(err, ShouldBeNil)
		So(lookupKeyPath(got, "xmetric", "constlabels"),
			ShouldResemble, map[string]any{"service.name": "demo"})
	})
}

func TestProfileVariantPath(t *testing.T) {
	PatchConvey("TestProfileVariantPath-拼接环境变体路径", t, func() {
		So(profileVariantPath("./conf/application.yml", "dev"), ShouldEqual, "./conf/application-dev.yml")
		So(profileVariantPath("/a/b/db.yaml", "prod"), ShouldEqual, "/a/b/db-prod.yaml")
	})
}

// ==================== source.go：导入列表 ====================

func TestImportsOf_List(t *testing.T) {
	PatchConvey("TestImportsOf-列表形态", t, func() {
		s := map[string]any{"server": map[string]any{"config": map[string]any{
			"import": []any{"db.yml", "redis.yml", "", 42},
		}}}
		So(importsOf(s), ShouldResemble, []string{"db.yml", "redis.yml"})
	})
}

func TestImportsOf_SingleString(t *testing.T) {
	PatchConvey("TestImportsOf-只写一个文件时不强求列表", t, func() {
		s := map[string]any{"server": map[string]any{"config": map[string]any{"import": "db.yml"}}}
		So(importsOf(s), ShouldResemble, []string{"db.yml"})
	})
}

func TestImportsOf_NotDeclared(t *testing.T) {
	PatchConvey("TestImportsOf-未声明导入", t, func() {
		So(importsOf(nil), ShouldBeEmpty)
		So(importsOf(map[string]any{"xlog": map[string]any{"level": "info"}}), ShouldBeEmpty)
		So(importsOf(map[string]any{"server": map[string]any{"config": map[string]any{"import": ""}}}), ShouldBeEmpty)
	})
}

func TestCollectImports_Union(t *testing.T) {
	PatchConvey("TestCollectImports-主配置与环境变体取并集并去重", t, func() {
		base := map[string]any{"server": map[string]any{"config": map[string]any{
			"import": []any{"db.yml", "redis.yml"},
		}}}
		profile := map[string]any{"server": map[string]any{"config": map[string]any{
			"import": []any{"redis.yml", "mock.yml"},
		}}}

		got, err := collectImports(base, profile)
		So(err, ShouldBeNil)
		So(got, ShouldResemble, []string{"db.yml", "redis.yml", "mock.yml"})
	})
}

func TestCollectImports_NoExtension(t *testing.T) {
	PatchConvey("TestCollectImports-路径没有扩展名", t, func() {
		base := map[string]any{"server": map[string]any{"config": map[string]any{"import": []any{"db"}}}}

		_, err := collectImports(base, nil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "has no extension")
	})
}

func TestCollectImports_PlaceholderInPath(t *testing.T) {
	PatchConvey("TestCollectImports-路径里的占位符先展开", t, func() {
		os.Setenv("XCONFIG_DIR", "/etc/app")
		defer os.Unsetenv("XCONFIG_DIR")

		base := map[string]any{"server": map[string]any{"config": map[string]any{
			"import": []any{"${XCONFIG_DIR}/db.yml", "${XCONFIG_NOT_SET:.}/redis.yml"},
		}}}

		got, err := collectImports(base, nil)
		So(err, ShouldBeNil)
		So(got, ShouldResemble, []string{"/etc/app/db.yml", "./redis.yml"})
	})
}

func TestCollectImports_PlaceholderMissing(t *testing.T) {
	PatchConvey("TestCollectImports-必填变量缺失优先于扩展名报错", t, func() {
		os.Unsetenv("XCONFIG_DIR_MISSING")

		base := map[string]any{"server": map[string]any{"config": map[string]any{
			"import": []any{"${XCONFIG_DIR_MISSING}"},
		}}}

		_, err := collectImports(base, nil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "XCONFIG_DIR_MISSING")
	})
}

func TestDropLoadDirectives(t *testing.T) {
	PatchConvey("TestDropLoadDirectives-剔除加载指令并保留其余配置", t, func() {
		s := map[string]any{
			"server": map[string]any{
				"name":     "x",
				"profiles": map[string]any{"active": "prod"},
				"config":   map[string]any{"import": []any{"db.yml"}},
			},
			"xlog": map[string]any{"level": "info"},
		}

		got := dropLoadDirectives(s)
		So(got, ShouldResemble, map[string]any{
			"server": map[string]any{"name": "x"},
			"xlog":   map[string]any{"level": "info"},
		})
	})
}

func TestDropLoadDirectives_OnlyDirectives(t *testing.T) {
	PatchConvey("TestDropLoadDirectives-只有加载指令时整个 Server 块被清掉", t, func() {
		s := map[string]any{"server": map[string]any{
			"profiles": map[string]any{"active": "prod"},
			"config":   map[string]any{"import": []any{"db.yml"}},
		}}
		So(dropLoadDirectives(s), ShouldBeEmpty)
	})
}

func TestMergeLayers_Order(t *testing.T) {
	PatchConvey("TestMergeLayers-后面的层覆盖前面的", t, func() {
		got := mergeLayers([]layer{
			{from: "a.yml", settings: map[string]any{"xlog": map[string]any{"level": "info", "path": "./log"}}},
			{from: "b.yml", settings: map[string]any{"xlog": map[string]any{"level": "debug"}}},
		})
		So(got["xlog"], ShouldResemble, map[string]any{"level": "debug", "path": "./log"})
	})
}

func TestMergeLayers_Empty(t *testing.T) {
	PatchConvey("TestMergeLayers-没有任何层", t, func() {
		So(mergeLayers(nil), ShouldBeEmpty)
	})
}

// ==================== source.go：层序构建 ====================

func TestLoadProfileBase_NotFound(t *testing.T) {
	PatchConvey("TestLoadProfileBase-环境配置文件不存在时只告警", t, func() {
		loc := writeFile(t, t.TempDir(), "application.yml", "Server:\n  Name: x\n")

		settings, from, err := loadProfileBase(loc, "dev")
		So(err, ShouldBeNil)
		So(settings, ShouldBeNil)
		So(from, ShouldBeEmpty)
	})
}

func TestLoadProfileBase_NoActive(t *testing.T) {
	PatchConvey("TestLoadProfileBase-未激活任何环境", t, func() {
		settings, from, err := loadProfileBase("/a/application.yml", "")
		So(err, ShouldBeNil)
		So(settings, ShouldBeNil)
		So(from, ShouldBeEmpty)
	})
}

func TestLoadProfileBase_DecodeError(t *testing.T) {
	PatchConvey("TestLoadProfileBase-环境配置文件解析失败", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", "Server:\n  Name: x\n")
		writeFile(t, dir, "application-dev.yml", "a:\n\t- broken\n  b: :")

		_, _, err := loadProfileBase(loc, "dev")
		So(err, ShouldNotBeNil)
	})
}

func TestBuildLayers_PlainAndProfileBands(t *testing.T) {
	PatchConvey("TestBuildLayers-无后缀层在前带后缀层在后", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - db.yml\n      - redis.yml\n")
		writeFile(t, dir, "db.yml", "XGorm:\n  Driver: postgres\n")
		writeFile(t, dir, "redis.yml", "XRedis:\n  Addr: 127.0.0.1:6379\n")
		writeFile(t, dir, "application-dev.yml", "XLog:\n  Level: debug\n")
		writeFile(t, dir, "db-dev.yml", "XGorm:\n  Driver: mysql\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		layers, imports, err := buildLayers(loc, base, "dev")
		So(err, ShouldBeNil)
		So(imports, ShouldResemble, []string{"db.yml", "redis.yml"})

		got := make([]string, 0, len(layers))
		for _, l := range layers {
			got = append(got, filepath.Base(l.from))
		}
		So(got, ShouldResemble, []string{
			"application.yml", "db.yml", "redis.yml", // 无后缀层
			"application-dev.yml", "db-dev.yml", // 带后缀层
		})
	})
}

func TestBuildLayers_NoImport(t *testing.T) {
	PatchConvey("TestBuildLayers-未声明导入时只有主配置一层", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", "Server:\n  Name: x\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		layers, imports, err := buildLayers(loc, base, "")
		So(err, ShouldBeNil)
		So(imports, ShouldBeEmpty)
		So(len(layers), ShouldEqual, 1)
		So(layers[0].from, ShouldEqual, loc)
	})
}

func TestBuildLayers_ImportMissingFile(t *testing.T) {
	PatchConvey("TestBuildLayers-点名导入的文件不存在直接失败", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - missing.yml\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		_, _, err = buildLayers(loc, base, "")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "imported config file not found")
	})
}

func TestBuildLayers_ImportDecodeError(t *testing.T) {
	PatchConvey("TestBuildLayers-导入的文件解析失败", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - db.yml\n")
		writeFile(t, dir, "db.yml", "a:\n\t- broken\n  b: :")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		_, _, err = buildLayers(loc, base, "")
		So(err, ShouldNotBeNil)
	})
}

func TestBuildLayers_NestedImportIgnored(t *testing.T) {
	PatchConvey("TestBuildLayers-被导入文件里的 Import 被忽略", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - db.yml\n")
		writeFile(t, dir, "db.yml",
			"Server:\n  Config:\n    Import:\n      - nested.yml\nXGorm:\n  Driver: postgres\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		layers, imports, err := buildLayers(loc, base, "")
		So(err, ShouldBeNil)
		So(imports, ShouldResemble, []string{"db.yml"}) // nested.yml 没有被加载
		So(len(layers), ShouldEqual, 2)
		So(lookupKeyPath(layers[1].settings, "server", "config"), ShouldBeNil)
	})
}

func TestBuildLayers_ImportCannotChangeProfilesActive(t *testing.T) {
	PatchConvey("TestBuildLayers-被导入文件不能改写激活环境", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - db.yml\n")
		writeFile(t, dir, "db.yml", "Server:\n  Profiles:\n    Active: hacked\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		layers, _, err := buildLayers(loc, base, "")
		So(err, ShouldBeNil)
		So(lookupKeyPath(layers[1].settings, "server", "profiles"), ShouldBeNil)
	})
}

func TestBuildLayers_ImportProfileVariant(t *testing.T) {
	PatchConvey("TestBuildLayers-被导入文件同样有环境变体", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - db.yml\n")
		writeFile(t, dir, "db.yml", "XGorm:\n  Driver: postgres\n")
		writeFile(t, dir, "db-dev.yml", "XGorm:\n  Driver: mysql\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		layers, _, err := buildLayers(loc, base, "dev")
		So(err, ShouldBeNil)

		last := layers[len(layers)-1]
		So(filepath.Base(last.from), ShouldEqual, "db-dev.yml")
		So(lookupKeyPath(last.settings, "xgorm", "driver"), ShouldEqual, "mysql")
	})
}

func TestBuildLayers_AbsoluteImportPath(t *testing.T) {
	PatchConvey("TestBuildLayers-导入绝对路径", t, func() {
		dir := t.TempDir()
		other := t.TempDir()
		abs := writeFile(t, other, "db.yml", "XGorm:\n  Driver: postgres\n")
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - "+abs+"\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		layers, _, err := buildLayers(loc, base, "")
		So(err, ShouldBeNil)
		So(layers[1].from, ShouldEqual, abs)
	})
}

func TestBuildLayers_ImportUnionFromProfile(t *testing.T) {
	PatchConvey("TestBuildLayers-环境配置可以追加导入文件", t, func() {
		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Config:\n    Import:\n      - db.yml\n")
		writeFile(t, dir, "db.yml", "XGorm:\n  Driver: postgres\n")
		writeFile(t, dir, "application-dev.yml",
			"Server:\n  Config:\n    Import:\n      - mock.yml\n")
		writeFile(t, dir, "mock.yml", "XHttp:\n  Timeout: 1s\n")

		base, err := decodeFile(loc)
		So(err, ShouldBeNil)
		_, imports, err := buildLayers(loc, base, "dev")
		So(err, ShouldBeNil)
		So(imports, ShouldResemble, []string{"db.yml", "mock.yml"})
	})
}

// ==================== xconfig.go：对外 API ====================

func TestCheckParam(t *testing.T) {
	PatchConvey("TestCheckParam", t, func() {
		PatchConvey("空 key", func() {
			So(checkParam("", &Server{}).Error(), ShouldContainSubstring, "key is empty")
		})

		PatchConvey("conf 为 nil", func() {
			So(checkParam("k", nil).Error(), ShouldContainSubstring, "conf is nil")
		})

		PatchConvey("conf 不是指针", func() {
			So(checkParam("k", Server{}).Error(), ShouldContainSubstring, "not ptr")
		})

		PatchConvey("类型化的 nil 指针", func() {
			var p *Server
			So(checkParam("k", p).Error(), ShouldContainSubstring, "nil pointer")
		})

		PatchConvey("合法入参", func() {
			So(checkParam("k", &Server{}), ShouldBeNil)
		})
	})
}

func TestGetStore_NotInit(t *testing.T) {
	PatchConvey("TestGetStore-未初始化时返回空配置", t, func() {
		resetStore()
		So(getStore(), ShouldNotBeNil)
		So(getStore().AllSettings(), ShouldBeEmpty)
	})
}

func TestUnmarshalConfig_OK(t *testing.T) {
	PatchConvey("TestUnmarshalConfig-正常反序列化", t, func() {
		defer resetStore()
		vp := viper.New()
		So(vp.MergeConfigMap(map[string]any{"server": map[string]any{"name": "xone.demo.app"}}), ShouldBeNil)
		storeMu.Lock()
		store = vp
		storeMu.Unlock()

		s := &Server{}
		So(UnmarshalConfig(ServerConfigKey, s), ShouldBeNil)
		So(s.Name, ShouldEqual, "xone.demo.app")
	})
}

func TestUnmarshalConfig_InvalidParam(t *testing.T) {
	PatchConvey("TestUnmarshalConfig-入参非法", t, func() {
		So(UnmarshalConfig("", &Server{}), ShouldNotBeNil)
		So(UnmarshalConfig("k", nil), ShouldNotBeNil)
	})
}

func TestUnmarshalConfig_TypedNilPointer(t *testing.T) {
	PatchConvey("TestUnmarshalConfig-类型化 nil 指针", t, func() {
		var p *Server
		err := UnmarshalConfig(ServerConfigKey, p)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "nil pointer")
	})
}

func TestUnmarshalConfig_DecodeError(t *testing.T) {
	PatchConvey("TestUnmarshalConfig-类型对不上时报错", t, func() {
		defer resetStore()
		vp := viper.New()
		So(vp.MergeConfigMap(map[string]any{"server": map[string]any{"name": []any{"a", "b"}}}), ShouldBeNil)
		storeMu.Lock()
		store = vp
		storeMu.Unlock()

		err := UnmarshalConfig(ServerConfigKey, &Server{})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "unmarshal failed")
	})
}

func TestGetters(t *testing.T) {
	PatchConvey("TestGetters-各类型读取", t, func() {
		defer resetStore()
		vp := viper.New()
		So(vp.MergeConfigMap(map[string]any{"myapp": map[string]any{
			"name":  "demo",
			"port":  8080,
			"debug": true,
		}}), ShouldBeNil)
		storeMu.Lock()
		store = vp
		storeMu.Unlock()

		So(GetString("MyApp.Name"), ShouldEqual, "demo")
		So(GetInt("MyApp.Port"), ShouldEqual, 8080)
		So(GetBool("MyApp.Debug"), ShouldBeTrue)
		So(GetConfig("MyApp.Name"), ShouldEqual, "demo")
		So(ContainKey("MyApp.Name"), ShouldBeTrue)
		So(ContainKey("MyApp.Nope"), ShouldBeFalse)
	})
}

func TestGetServerName_Default(t *testing.T) {
	PatchConvey("TestGetServerName-未配置时用默认值", t, func() {
		resetStore()
		So(GetServerName(), ShouldEqual, defaultServerName)
	})
}

func TestGetServerVersion_Default(t *testing.T) {
	PatchConvey("TestGetServerVersion-未配置时用默认值", t, func() {
		resetStore()
		So(GetServerVersion(), ShouldEqual, defaultServerVersion)
	})
}

func TestGetProfilesActive(t *testing.T) {
	PatchConvey("TestGetProfilesActive", t, func() {
		defer resetStore()

		PatchConvey("未激活任何环境", func() {
			resetStore()
			So(GetProfilesActive(), ShouldBeEmpty)
		})

		PatchConvey("已激活", func() {
			storeMu.Lock()
			server = Server{Profiles: &Profiles{Active: "dev"}}
			storeMu.Unlock()
			So(GetProfilesActive(), ShouldEqual, "dev")
		})
	})
}

func TestSources_Clone(t *testing.T) {
	PatchConvey("TestSources-返回副本不暴露内部切片", t, func() {
		defer resetStore()
		storeMu.Lock()
		sources = []string{"a.yml", "b.yml"}
		storeMu.Unlock()

		got := Sources()
		So(got, ShouldResemble, []string{"a.yml", "b.yml"})

		got[0] = "tampered"
		So(Sources()[0], ShouldEqual, "a.yml")
	})
}

func TestServerMergeDefault(t *testing.T) {
	PatchConvey("TestServerMergeDefault-集中设置默认值", t, func() {
		PatchConvey("全部为空", func() {
			s := serverMergeDefault(Server{})
			So(s.Name, ShouldEqual, defaultServerName)
			So(s.Version, ShouldEqual, defaultServerVersion)
		})

		PatchConvey("已配置的值不被覆盖", func() {
			s := serverMergeDefault(Server{Name: "xone.demo.app", Version: "v2.0.0"})
			So(s.Name, ShouldEqual, "xone.demo.app")
			So(s.Version, ShouldEqual, "v2.0.0")
		})
	})
}

// ==================== xconfig_init.go ====================

func TestLoadDotEnv(t *testing.T) {
	PatchConvey("TestLoadDotEnv", t, func() {
		PatchConvey("存在时加载其中的变量", func() {
			dir := t.TempDir()
			writeFile(t, dir, ".env", "XCONFIG_DOTENV=from-dotenv\n")
			loc := filepath.Join(dir, "application.yml")
			defer os.Unsetenv("XCONFIG_DOTENV")

			So(loadDotEnv(loc), ShouldBeNil)
			So(os.Getenv("XCONFIG_DOTENV"), ShouldEqual, "from-dotenv")
		})

		PatchConvey("不存在时跳过", func() {
			So(loadDotEnv(filepath.Join(t.TempDir(), "application.yml")), ShouldBeNil)
		})

		PatchConvey("加载失败时报错", func() {
			dir := t.TempDir()
			writeFile(t, dir, ".env", "")
			Mock(godotenv.Load).Return(errors.New("boom")).Build()

			err := loadDotEnv(filepath.Join(dir, "application.yml"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "load .env failed")
		})
	})
}

func TestAnnotate(t *testing.T) {
	PatchConvey("TestAnnotate-回填权威加载指令", t, func() {
		PatchConvey("回填激活环境与导入列表", func() {
			s := map[string]any{}
			annotate(s, "dev", []string{"db.yml", "redis.yml"})

			So(lookupKeyPath(s, "server", "profiles", "active"), ShouldEqual, "dev")
			So(lookupKeyPath(s, "server", "config", "import"), ShouldResemble, []any{"db.yml", "redis.yml"})
		})

		PatchConvey("没有激活环境和导入时不写入任何东西", func() {
			s := map[string]any{}
			annotate(s, "", nil)
			So(s, ShouldBeEmpty)
		})

		PatchConvey("覆盖掉配置文件里写的旧值", func() {
			s := map[string]any{"server": map[string]any{"profiles": map[string]any{"active": "stale"}}}
			annotate(s, "dev", nil)
			So(lookupKeyPath(s, "server", "profiles", "active"), ShouldEqual, "dev")
		})
	})
}

func TestPublish(t *testing.T) {
	PatchConvey("TestPublish-写入全局存储并解析 Server 块", t, func() {
		defer resetStore()

		settings := map[string]any{"server": map[string]any{"name": "xone.demo.app", "version": "v2.0.0"}}
		layers := []layer{{from: "application.yml", settings: settings}}

		So(publish(settings, layers), ShouldBeNil)
		So(GetServerName(), ShouldEqual, "xone.demo.app")
		So(GetServerVersion(), ShouldEqual, "v2.0.0")
		So(Sources(), ShouldResemble, []string{"application.yml"})
		So(GetString("Server.Name"), ShouldEqual, "xone.demo.app")
	})
}

func TestPublish_ServerNameEmpty(t *testing.T) {
	PatchConvey("TestPublish-Server.Name 为空时仍可用默认值", t, func() {
		defer resetStore()

		settings := map[string]any{"xlog": map[string]any{"level": "info"}}
		So(publish(settings, []layer{{from: "application.yml", settings: settings}}), ShouldBeNil)
		So(GetServerName(), ShouldEqual, defaultServerName)
		So(GetServerVersion(), ShouldEqual, defaultServerVersion)
	})
}

func TestPublish_UnmarshalServerError(t *testing.T) {
	PatchConvey("TestPublish-Server 块类型不合法时报错", t, func() {
		defer resetStore()

		settings := map[string]any{"server": map[string]any{"name": map[string]any{"a": 1}}}
		err := publish(settings, nil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "unmarshal")
	})
}

func TestInitXConfig_NoLocation(t *testing.T) {
	PatchConvey("TestInitXConfig-找不到配置文件时跳过", t, func() {
		Mock(detectConfigLocation).Return("").Build()
		So(initXConfig(), ShouldBeNil)
	})
}

func TestInitXConfig_DecodeError(t *testing.T) {
	PatchConvey("TestInitXConfig-主配置解析失败", t, func() {
		Mock(detectConfigLocation).Return(filepath.Join(t.TempDir(), "nope.yml")).Build()
		So(initXConfig(), ShouldNotBeNil)
	})
}

func TestInitXConfig_FullPipeline(t *testing.T) {
	PatchConvey("TestInitXConfig-完整走一遍加载流程", t, func() {
		defer resetStore()

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
Server:
  Name: "xone.demo.app"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
XLog:
  Level: info
`)
		writeFile(t, dir, "db.yml", "XGorm:\n  Driver: postgres\n")
		writeFile(t, dir, "db-dev.yml", "XGorm:\n  Driver: mysql\n")
		writeFile(t, dir, "application-dev.yml", "XLog:\n  Level: debug\n")

		Mock(detectConfigLocation).Return(loc).Build()
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		So(initXConfig(), ShouldBeNil)
		So(GetServerName(), ShouldEqual, "xone.demo.app")
		So(GetProfilesActive(), ShouldEqual, "dev")
		So(GetString("XLog.Level"), ShouldEqual, "debug")   // 环境配置覆盖主配置
		So(GetString("XGorm.Driver"), ShouldEqual, "mysql") // 导入文件的环境变体生效
		So(len(Sources()), ShouldEqual, 4)
	})
}

func TestInitXConfig_PlaceholderMissing(t *testing.T) {
	PatchConvey("TestInitXConfig-必填占位符缺失时启动失败", t, func() {
		defer resetStore()
		os.Unsetenv("XCONFIG_REQUIRED_MISSING")

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", "XGorm:\n  DSN: \"${XCONFIG_REQUIRED_MISSING}\"\n")
		Mock(detectConfigLocation).Return(loc).Build()
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		err := initXConfig()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "XCONFIG_REQUIRED_MISSING")
	})
}

func TestPrintFinalConfig_Disabled(t *testing.T) {
	PatchConvey("TestPrintFinalConfig-未开启 debug 时不打印", t, func() {
		Mock(xutil.EnableXOneDebug).Return(false).Build()
		infoMock := Mock(xutil.InfoIfEnableDebug).Return().Build()

		printFinalConfig(map[string]any{"a": 1}, []string{"a.yml"})
		So(infoMock.Times(), ShouldEqual, 0)
	})
}

func TestPrintFinalConfig_MasksSecrets(t *testing.T) {
	PatchConvey("TestPrintFinalConfig-打印时脱敏并列出来源", t, func() {
		Mock(xutil.EnableXOneDebug).Return(true).Build()

		var printed string
		Mock(xutil.InfoIfEnableDebug).To(func(format string, args ...any) {
			if len(args) > 0 {
				printed, _ = args[0].(string)
			}
		}).Build()

		printFinalConfig(map[string]any{
			"xgorm": map[string]any{"dsn": "user:secret@tcp(127.0.0.1)/db"},
		}, []string{"application.yml", "db.yml"})

		So(printed, ShouldContainSubstring, maskedValue)
		So(printed, ShouldNotContainSubstring, "secret@tcp")
		So(printed, ShouldContainSubstring, "1. application.yml")
		So(printed, ShouldContainSubstring, "2. db.yml")
	})
}

func TestMaskSensitive(t *testing.T) {
	PatchConvey("TestMaskSensitive-递归替换敏感字段", t, func() {
		got := maskSensitive(map[string]any{
			"password": "p",
			"level":    "info",
			"nested":   map[string]any{"apikey": "k", "port": 8080},
		})

		So(got["password"], ShouldEqual, maskedValue)
		So(got["level"], ShouldEqual, "info")
		So(got["nested"].(map[string]any)["apikey"], ShouldEqual, maskedValue)
		So(got["nested"].(map[string]any)["port"], ShouldEqual, 8080)
	})
}

func TestMaskSensitive_InsideList(t *testing.T) {
	PatchConvey("TestMaskSensitive-穿过多实例列表", t, func() {
		got := maskSensitive(map[string]any{
			"xredis": []any{
				map[string]any{"name": "cache", "password": "p1"},
				map[string]any{"name": "session", "password": "p2"},
			},
		})

		list := got["xredis"].([]any)
		So(list[0].(map[string]any)["password"], ShouldEqual, maskedValue)
		So(list[0].(map[string]any)["name"], ShouldEqual, "cache")
		So(list[1].(map[string]any)["password"], ShouldEqual, maskedValue)
	})
}

// ==================== 端到端 ====================

func TestPipeline_ProfileOverridesBase(t *testing.T) {
	PatchConvey("TestPipeline-环境配置逐层深合并覆盖主配置", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
Server:
  Profiles:
    Active: dev
XLog:
  Level: info
  File:
    Enable: true
    Path: ./log
`)
		writeFile(t, dir, "application-dev.yml", "XLog:\n  Level: debug\n")

		settings, _ := loadFrom(t, loc)
		So(lookupKeyPath(settings, "xlog", "level"), ShouldEqual, "debug")
		So(lookupKeyPath(settings, "xlog", "file", "enable"), ShouldEqual, true)
		So(lookupKeyPath(settings, "xlog", "file", "path"), ShouldEqual, "./log")
	})
}

func TestPipeline_ImportBeatsBase(t *testing.T) {
	PatchConvey("TestPipeline-被导入的文件覆盖主配置", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
Server:
  Config:
    Import:
      - db.yml
XGorm:
  Driver: stale
`)
		writeFile(t, dir, "db.yml", "XGorm:\n  Driver: postgres\n")

		settings, _ := loadFrom(t, loc)
		So(lookupKeyPath(settings, "xgorm", "driver"), ShouldEqual, "postgres")
	})
}

func TestPipeline_FullOrder(t *testing.T) {
	PatchConvey("TestPipeline-完整优先级顺序", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
Server:
  Profiles:
    Active: dev
  Config:
    Import:
      - db.yml
      - redis.yml
K: application
`)
		writeFile(t, dir, "db.yml", "K: db\n")
		writeFile(t, dir, "redis.yml", "K: redis\n")
		writeFile(t, dir, "application-dev.yml", "K: application-dev\n")
		writeFile(t, dir, "db-dev.yml", "K: db-dev\n")
		writeFile(t, dir, "redis-dev.yml", "K: redis-dev\n")

		settings, from := loadFrom(t, loc)
		// application < db < redis < application-dev < db-dev < redis-dev
		So(settings["k"], ShouldEqual, "redis-dev")

		got := make([]string, 0, len(from))
		for _, f := range from {
			got = append(got, filepath.Base(f))
		}
		So(got, ShouldResemble, []string{
			"application.yml", "db.yml", "redis.yml",
			"application-dev.yml", "db-dev.yml", "redis-dev.yml",
		})
	})
}

func TestPipeline_ProfileBandBeatsPlainBand(t *testing.T) {
	PatchConvey("TestPipeline-带后缀层整体压过无后缀层", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
Server:
  Profiles:
    Active: dev
  Config:
    Import:
      - db.yml
      - redis.yml
`)
		writeFile(t, dir, "db.yml", "K: db\n")
		writeFile(t, dir, "db-dev.yml", "K: db-dev\n")
		// redis.yml 声明在 db.yml 之后，但它没有环境变体，
		// 不该压过 db-dev.yml —— 环境专属的值优先
		writeFile(t, dir, "redis.yml", "K: redis\n")

		settings, _ := loadFrom(t, loc)
		So(settings["k"], ShouldEqual, "db-dev")
	})
}

func TestPipeline_AnnotateReflectsRealActive(t *testing.T) {
	PatchConvey("TestPipeline-最终配置里的激活环境是实际生效的那个", t, func() {
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Setenv(profilesActiveEnvKey, "prod")
		defer os.Unsetenv(profilesActiveEnvKey)

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml",
			"Server:\n  Profiles:\n    Active: dev\n")

		settings, _ := loadFrom(t, loc)
		// 配置文件里写的是 dev，但环境变量指定了 prod
		So(lookupKeyPath(settings, "server", "profiles", "active"), ShouldEqual, "prod")
	})
}

func TestPipeline_DottedKeySurvives(t *testing.T) {
	PatchConvey("TestPipeline-含点的 key 走完整流程后仍是一个整体", t, func() {
		defer resetStore()
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
XMetric:
  ConstLabels:
    service.name: "${XCONFIG_SVC:demo}"
    deployment.environment: "prod"
`)
		Mock(detectConfigLocation).Return(loc).Build()
		So(initXConfig(), ShouldBeNil)

		var c struct {
			ConstLabels map[string]string `mapstructure:"ConstLabels"`
		}
		So(UnmarshalConfig("XMetric", &c), ShouldBeNil)
		So(c.ConstLabels, ShouldResemble, map[string]string{
			"service.name":           "demo",
			"deployment.environment": "prod",
		})
	})
}

func TestPipeline_MultiInstancePlaceholder(t *testing.T) {
	PatchConvey("TestPipeline-多实例配置里的占位符被展开", t, func() {
		defer resetStore()
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)
		os.Setenv("XCONFIG_REDIS_PW", "s3cret")
		defer os.Unsetenv("XCONFIG_REDIS_PW")

		dir := t.TempDir()
		loc := writeFile(t, dir, "application.yml", `
XRedis:
  - Name: "cache"
    Password: "${XCONFIG_REDIS_PW}"
  - Name: "session"
    Password: "${XCONFIG_REDIS_PW_UNSET:}"
`)
		Mock(detectConfigLocation).Return(loc).Build()
		So(initXConfig(), ShouldBeNil)

		var multi []struct {
			Name     string `mapstructure:"Name"`
			Password string `mapstructure:"Password"`
		}
		So(UnmarshalConfig("XRedis", &multi), ShouldBeNil)
		So(len(multi), ShouldEqual, 2)
		So(multi[0].Password, ShouldEqual, "s3cret")
		So(multi[1].Password, ShouldBeEmpty)
		So(xutil.IsSlice(GetConfig("XRedis")), ShouldBeTrue)
	})
}

func TestPipeline_DotEnvFeedsPlaceholder(t *testing.T) {
	PatchConvey("TestPipeline-同目录 .env 的变量可被占位符引用", t, func() {
		defer resetStore()
		Mock(xutil.GetConfigFromArgs).Return("", nil).Build()
		os.Unsetenv(profilesActiveEnvKey)
		defer os.Unsetenv("XCONFIG_FROM_DOTENV")

		dir := t.TempDir()
		writeFile(t, dir, ".env", "XCONFIG_FROM_DOTENV=dotenv-value\n")
		loc := writeFile(t, dir, "application.yml", "Server:\n  Name: \"${XCONFIG_FROM_DOTENV}\"\n")

		Mock(detectConfigLocation).Return(loc).Build()
		So(initXConfig(), ShouldBeNil)
		So(GetServerName(), ShouldEqual, "dotenv-value")
	})
}
