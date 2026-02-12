package xutil

import (
	"context"
	"errors"
	"net"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
	"go.opentelemetry.io/otel/trace"
)

// ==================== convert.go ====================

func TestToPtr(t *testing.T) {
	mockey.PatchConvey("TestToPtr", t, func() {
		intVal := 42
		ptrVal := ToPtr(intVal)
		c.So(*ptrVal, c.ShouldEqual, 42)

		strVal := "hello"
		ptrStr := ToPtr(strVal)
		c.So(*ptrStr, c.ShouldEqual, "hello")
	})
}

func TestGetOrDefault(t *testing.T) {
	mockey.PatchConvey("TestGetOrDefault", t, func() {
		mockey.PatchConvey("TestGetOrDefault-ZeroValue", func() {
			c.So(GetOrDefault(0, 100), c.ShouldEqual, 100)
			c.So(GetOrDefault("", "default"), c.ShouldEqual, "default")
		})

		mockey.PatchConvey("TestGetOrDefault-NonZeroValue", func() {
			c.So(GetOrDefault(42, 100), c.ShouldEqual, 42)
			c.So(GetOrDefault("hello", "default"), c.ShouldEqual, "hello")
		})

		mockey.PatchConvey("TestGetOrDefault-NilInterface", func() {
			// nil interface → 零值比较，any 的零值为 nil
			c.So(GetOrDefault[any](nil, "default"), c.ShouldEqual, "default")
		})
	})
}

func TestToDuration(t *testing.T) {
	mockey.PatchConvey("TestToDuration", t, func() {
		mockey.PatchConvey("TestToDuration-Nil", func() {
			c.So(ToDuration(nil), c.ShouldEqual, 0)
		})

		mockey.PatchConvey("TestToDuration-String", func() {
			c.So(ToDuration("1s"), c.ShouldEqual, time.Second)
			c.So(ToDuration("100ms"), c.ShouldEqual, 100*time.Millisecond)
		})

		mockey.PatchConvey("TestToDuration-StringPointer", func() {
			s := "2s"
			c.So(ToDuration(&s), c.ShouldEqual, 2*time.Second)
		})

		mockey.PatchConvey("TestToDuration-WithDay", func() {
			c.So(ToDuration("1d"), c.ShouldEqual, 24*time.Hour)
			c.So(ToDuration("2d12h"), c.ShouldEqual, 60*time.Hour)
		})

		mockey.PatchConvey("TestToDuration-InvalidDay", func() {
			// "abc" 无法解析为天数，fallback 解析剩余 "12h"
			c.So(ToDuration("abcd12h"), c.ShouldEqual, 12*time.Hour)
		})

		mockey.PatchConvey("TestToDuration-Int", func() {
			c.So(ToDuration(1000000000), c.ShouldEqual, time.Second)
		})
	})
}

// ==================== json.go ====================

func TestToJsonString(t *testing.T) {
	mockey.PatchConvey("TestToJsonString", t, func() {
		mockey.PatchConvey("TestToJsonString-Map", func() {
			c.So(ToJsonString(map[string]string{"key": "value"}), c.ShouldEqual, `{"key":"value"}`)
		})

		mockey.PatchConvey("TestToJsonString-Struct", func() {
			type Person struct {
				Name string `json:"name"`
				Age  int    `json:"age"`
			}
			c.So(ToJsonString(Person{Name: "Alice", Age: 30}), c.ShouldEqual, `{"name":"Alice","age":30}`)
		})

		mockey.PatchConvey("TestToJsonString-InvalidValue", func() {
			c.So(ToJsonString(make(chan int)), c.ShouldEqual, "")
		})
	})
}

func TestToJsonStringIndent(t *testing.T) {
	mockey.PatchConvey("TestToJsonStringIndent", t, func() {
		mockey.PatchConvey("TestToJsonStringIndent-Map", func() {
			result := ToJsonStringIndent(map[string]string{"key": "value"})
			c.So(result, c.ShouldContainSubstring, "key")
			c.So(result, c.ShouldContainSubstring, "value")
		})

		mockey.PatchConvey("TestToJsonStringIndent-InvalidValue", func() {
			c.So(ToJsonStringIndent(make(chan int)), c.ShouldEqual, "")
		})
	})
}

// ==================== net.go ====================

func TestGetLocalIP(t *testing.T) {
	mockey.PatchConvey("TestGetLocalIP", t, func() {
		mockey.PatchConvey("TestGetLocalIP-PublicIPv4", func() {
			mockey.Mock(collectLocalIPs).Return(
				[]net.IP{net.ParseIP("8.8.8.8")}, nil, nil, nil, nil,
			).Build()
			ip, err := GetLocalIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "8.8.8.8")
		})

		mockey.PatchConvey("TestGetLocalIP-PublicIPv6Only", func() {
			mockey.Mock(collectLocalIPs).Return(
				nil, []net.IP{net.ParseIP("2001:db8::1")}, nil, nil, nil,
			).Build()
			ip, err := GetLocalIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "2001:db8::1")
		})

		mockey.PatchConvey("TestGetLocalIP-FallbackToPrivateIPv4", func() {
			mockey.Mock(collectLocalIPs).Return(
				nil, nil, []net.IP{net.ParseIP("192.168.1.1")}, nil, nil,
			).Build()
			ip, err := GetLocalIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "192.168.1.1")
		})

		mockey.PatchConvey("TestGetLocalIP-FallbackToPrivateIPv6", func() {
			mockey.Mock(collectLocalIPs).Return(
				nil, nil, nil, []net.IP{net.ParseIP("fd00::1")}, nil,
			).Build()
			ip, err := GetLocalIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "fd00::1")
		})

		mockey.PatchConvey("TestGetLocalIP-Error", func() {
			mockey.Mock(collectLocalIPs).Return(nil, nil, nil, nil, errors.New("mock error")).Build()
			_, err := GetLocalIP()
			c.So(err, c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestGetLocalIP-NoIPFound", func() {
			mockey.Mock(collectLocalIPs).Return(nil, nil, nil, nil, nil).Build()
			_, err := GetLocalIP()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldEqual, "no IP address found")
		})
	})
}

func TestGetLocalPublicIP(t *testing.T) {
	mockey.PatchConvey("TestGetLocalPublicIP", t, func() {
		mockey.PatchConvey("TestGetLocalPublicIP-IPv4", func() {
			mockey.Mock(collectLocalIPs).Return(
				[]net.IP{net.ParseIP("1.2.3.4")}, nil, nil, nil, nil,
			).Build()
			ip, err := GetLocalPublicIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "1.2.3.4")
		})

		mockey.PatchConvey("TestGetLocalPublicIP-IPv6", func() {
			mockey.Mock(collectLocalIPs).Return(
				nil, []net.IP{net.ParseIP("2001:db8::1")}, nil, nil, nil,
			).Build()
			ip, err := GetLocalPublicIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "2001:db8::1")
		})

		mockey.PatchConvey("TestGetLocalPublicIP-NotFound", func() {
			mockey.Mock(collectLocalIPs).Return(nil, nil, nil, nil, nil).Build()
			_, err := GetLocalPublicIP()
			c.So(err, c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestGetLocalPublicIP-Error", func() {
			mockey.Mock(collectLocalIPs).Return(nil, nil, nil, nil, errors.New("mock error")).Build()
			_, err := GetLocalPublicIP()
			c.So(err, c.ShouldNotBeNil)
		})
	})
}

func TestGetLocalPrivateIP(t *testing.T) {
	mockey.PatchConvey("TestGetLocalPrivateIP", t, func() {
		mockey.PatchConvey("TestGetLocalPrivateIP-IPv4", func() {
			mockey.Mock(collectLocalIPs).Return(
				nil, nil, []net.IP{net.ParseIP("192.168.1.1")}, nil, nil,
			).Build()
			ip, err := GetLocalPrivateIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "192.168.1.1")
		})

		mockey.PatchConvey("TestGetLocalPrivateIP-IPv6", func() {
			mockey.Mock(collectLocalIPs).Return(
				nil, nil, nil, []net.IP{net.ParseIP("fd00::1")}, nil,
			).Build()
			ip, err := GetLocalPrivateIP()
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "fd00::1")
		})

		mockey.PatchConvey("TestGetLocalPrivateIP-NotFound", func() {
			mockey.Mock(collectLocalIPs).Return(nil, nil, nil, nil, nil).Build()
			_, err := GetLocalPrivateIP()
			c.So(err, c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestGetLocalPrivateIP-Error", func() {
			mockey.Mock(collectLocalIPs).Return(nil, nil, nil, nil, errors.New("mock error")).Build()
			_, err := GetLocalPrivateIP()
			c.So(err, c.ShouldNotBeNil)
		})
	})
}

func TestCollectLocalIPs(t *testing.T) {
	mockey.PatchConvey("TestCollectLocalIPs", t, func() {
		mockey.PatchConvey("TestCollectLocalIPs-InterfaceError", func() {
			mockey.Mock(net.Interfaces).Return(nil, errors.New("mock error")).Build()
			_, _, _, _, err := collectLocalIPs()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "failed to get interfaces")
		})

		mockey.PatchConvey("TestCollectLocalIPs-SkipLoopback", func() {
			mockey.Mock(net.Interfaces).Return([]net.Interface{
				{Index: 1, Name: "lo0", Flags: net.FlagLoopback | net.FlagUp},
			}, nil).Build()
			pub4, pub6, pri4, pri6, err := collectLocalIPs()
			c.So(err, c.ShouldBeNil)
			c.So(pub4, c.ShouldBeEmpty)
			c.So(pub6, c.ShouldBeEmpty)
			c.So(pri4, c.ShouldBeEmpty)
			c.So(pri6, c.ShouldBeEmpty)
		})

		mockey.PatchConvey("TestCollectLocalIPs-AddrError", func() {
			mockey.Mock(net.Interfaces).Return([]net.Interface{
				{Index: 1, Name: "eth0", Flags: net.FlagUp},
			}, nil).Build()
			mockey.Mock((*net.Interface).Addrs).Return(nil, errors.New("addr error")).Build()
			pub4, pub6, pri4, pri6, err := collectLocalIPs()
			c.So(err, c.ShouldBeNil)
			c.So(pub4, c.ShouldBeEmpty)
			c.So(pub6, c.ShouldBeEmpty)
			c.So(pri4, c.ShouldBeEmpty)
			c.So(pri6, c.ShouldBeEmpty)
		})

		mockey.PatchConvey("TestCollectLocalIPs-ClassifyIPs", func() {
			mockey.Mock(net.Interfaces).Return([]net.Interface{
				{Index: 1, Name: "eth0", Flags: net.FlagUp},
			}, nil).Build()
			mockey.Mock((*net.Interface).Addrs).Return([]net.Addr{
				&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(24, 32)},
				&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPAddr{IP: net.ParseIP("10.0.0.1")},
				&net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 80},                  // default 分支跳过
				&net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},             // unspecified 跳过
				&net.IPNet{IP: net.ParseIP("224.0.0.1"), Mask: net.CIDRMask(4, 32)}, // multicast 跳过
			}, nil).Build()

			pub4, pub6, pri4, pri6, err := collectLocalIPs()
			c.So(err, c.ShouldBeNil)
			c.So(len(pub4), c.ShouldEqual, 1)
			c.So(pub4[0].String(), c.ShouldEqual, "8.8.8.8")
			c.So(len(pub6), c.ShouldEqual, 1)
			c.So(pub6[0].String(), c.ShouldEqual, "2001:db8::1")
			c.So(len(pri4), c.ShouldEqual, 2)
			c.So(len(pri6), c.ShouldEqual, 1)
			c.So(pri6[0].String(), c.ShouldEqual, "fe80::1")
		})
	})
}

func TestIsPrivateIP(t *testing.T) {
	mockey.PatchConvey("TestIsPrivateIP", t, func() {
		mockey.PatchConvey("TestIsPrivateIP-Nil", func() {
			c.So(isPrivateIP(nil), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestIsPrivateIP-Private", func() {
			c.So(isPrivateIP(net.ParseIP("192.168.1.1")), c.ShouldBeTrue)
			c.So(isPrivateIP(net.ParseIP("10.0.0.1")), c.ShouldBeTrue)
			c.So(isPrivateIP(net.ParseIP("172.16.0.1")), c.ShouldBeTrue)
			c.So(isPrivateIP(net.ParseIP("127.0.0.1")), c.ShouldBeTrue)
			c.So(isPrivateIP(net.ParseIP("fe80::1")), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestIsPrivateIP-Public", func() {
			c.So(isPrivateIP(net.ParseIP("8.8.8.8")), c.ShouldBeFalse)
			c.So(isPrivateIP(net.ParseIP("1.1.1.1")), c.ShouldBeFalse)
			c.So(isPrivateIP(net.ParseIP("2001:db8::1")), c.ShouldBeFalse)
		})
	})
}

// ==================== log.go ====================

func TestLogFunctions(t *testing.T) {
	mockey.PatchConvey("TestLogFunctions", t, func() {
		mockey.Mock(EnableXOneDebug).Return(true).Build()

		mockey.PatchConvey("TestInfoIfEnableDebug", func() {
			InfoIfEnableDebug("test message %s", "arg")
		})

		mockey.PatchConvey("TestWarnIfEnableDebug", func() {
			WarnIfEnableDebug("test warning %s", "arg")
		})

		mockey.PatchConvey("TestErrorIfEnableDebug", func() {
			ErrorIfEnableDebug("test error %s", "arg")
		})
	})

	mockey.PatchConvey("TestLogFunctions-DebugDisabled", t, func() {
		mockey.Mock(EnableXOneDebug).Return(false).Build()
		InfoIfEnableDebug("should not log %s", "arg")
	})
}

func TestGetLogCaller(t *testing.T) {
	mockey.PatchConvey("TestGetLogCaller", t, func() {
		frame := GetLogCaller(0, nil)
		c.So(frame, c.ShouldNotBeNil)
	})
}

func TestCallerPretty(t *testing.T) {
	mockey.PatchConvey("TestCallerPretty", t, func() {
		mockey.PatchConvey("TestCallerPretty-Normal", func() {
			funcName, fileName := callerPretty(nil)
			c.So(funcName, c.ShouldEqual, "")
			c.So(fileName, c.ShouldNotBeEmpty)
		})

		mockey.PatchConvey("TestCallerPretty-NilFrame", func() {
			mockey.Mock(GetLogCaller).Return((*runtime.Frame)(nil)).Build()
			funcName, fileName := callerPretty(nil)
			c.So(funcName, c.ShouldEqual, unknownCaller)
			c.So(fileName, c.ShouldEqual, unknownCaller)
		})

		mockey.PatchConvey("TestCallerPretty-EmptyBaseName", func() {
			mockey.Mock(GetLogCaller).Return(&runtime.Frame{File: "test.go", Line: 10}).Build()
			mockey.Mock(path.Base).Return("").Build()
			_, fileName := callerPretty(nil)
			c.So(fileName, c.ShouldContainSubstring, unknownCaller)
		})
	})
}

// ==================== env.go ====================

func TestEnableDebug(t *testing.T) {
	mockey.PatchConvey("TestEnableDebug", t, func() {
		mockey.PatchConvey("TestEnableDebug-True", func() {
			os.Setenv(DebugKey, "true")
			os.Unsetenv(legacyDebugKey)
			defer os.Unsetenv(DebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-1", func() {
			os.Setenv(DebugKey, "1")
			os.Unsetenv(legacyDebugKey)
			defer os.Unsetenv(DebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-Yes", func() {
			os.Setenv(DebugKey, "yes")
			os.Unsetenv(legacyDebugKey)
			defer os.Unsetenv(DebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-On", func() {
			os.Setenv(DebugKey, "on")
			os.Unsetenv(legacyDebugKey)
			defer os.Unsetenv(DebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-False", func() {
			os.Setenv(DebugKey, "false")
			os.Unsetenv(legacyDebugKey)
			defer os.Unsetenv(DebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestEnableDebug-Empty", func() {
			os.Unsetenv(DebugKey)
			os.Unsetenv(legacyDebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestEnableDebug-Legacy", func() {
			os.Unsetenv(DebugKey)
			os.Setenv(legacyDebugKey, "true")
			defer os.Unsetenv(legacyDebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-Precedence", func() {
			os.Setenv(DebugKey, "false")
			os.Setenv(legacyDebugKey, "true")
			defer os.Unsetenv(DebugKey)
			defer os.Unsetenv(legacyDebugKey)
			c.So(EnableXOneDebug(), c.ShouldBeFalse)
		})
	})
}

// ==================== file.go ====================

func TestFileExist(t *testing.T) {
	mockey.PatchConvey("TestFileExist", t, func() {
		mockey.PatchConvey("TestFileExist-Exists", func() {
			c.So(FileExist("xutil_test.go"), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestFileExist-NotExists", func() {
			c.So(FileExist("nonexistent_file.go"), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestFileExist-IsDir", func() {
			c.So(FileExist("."), c.ShouldBeFalse)
		})
	})
}

func TestDirExist(t *testing.T) {
	mockey.PatchConvey("TestDirExist", t, func() {
		mockey.PatchConvey("TestDirExist-Exists", func() {
			c.So(DirExist("."), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestDirExist-NotExists", func() {
			c.So(DirExist("nonexistent_dir"), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestDirExist-IsFile", func() {
			c.So(DirExist("xutil_test.go"), c.ShouldBeFalse)
		})
	})
}

// ==================== reflect.go ====================

func TestIsSlice(t *testing.T) {
	mockey.PatchConvey("TestIsSlice", t, func() {
		mockey.PatchConvey("TestIsSlice-Nil", func() {
			c.So(IsSlice(nil), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestIsSlice-Slice", func() {
			c.So(IsSlice([]int{1, 2, 3}), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestIsSlice-NotSlice", func() {
			c.So(IsSlice("string"), c.ShouldBeFalse)
			c.So(IsSlice(123), c.ShouldBeFalse)
			c.So(IsSlice(map[string]int{}), c.ShouldBeFalse)
		})
	})
}

func TestGetFuncName(t *testing.T) {
	mockey.PatchConvey("TestGetFuncName", t, func() {
		mockey.PatchConvey("TestGetFuncName-Valid", func() {
			c.So(GetFuncName(TestGetFuncName), c.ShouldEqual, "TestGetFuncName")
		})

		mockey.PatchConvey("TestGetFuncName-Nil", func() {
			c.So(GetFuncName(nil), c.ShouldEqual, "")
		})
	})
}

func TestGetFuncInfo(t *testing.T) {
	mockey.PatchConvey("TestGetFuncInfo", t, func() {
		mockey.PatchConvey("TestGetFuncInfo-Valid", func() {
			file, line, name := GetFuncInfo(TestGetFuncInfo)
			c.So(file, c.ShouldNotBeEmpty)
			c.So(line, c.ShouldBeGreaterThan, 0)
			c.So(name, c.ShouldEqual, "TestGetFuncInfo")
		})

		mockey.PatchConvey("TestGetFuncInfo-Nil", func() {
			file, line, name := GetFuncInfo(nil)
			c.So(file, c.ShouldEqual, "")
			c.So(line, c.ShouldEqual, 0)
			c.So(name, c.ShouldEqual, "")
		})

		mockey.PatchConvey("TestGetFuncInfo-NotFunc", func() {
			file, line, name := GetFuncInfo("not a function")
			c.So(file, c.ShouldEqual, "")
			c.So(line, c.ShouldEqual, 0)
			c.So(name, c.ShouldEqual, "")
		})

		mockey.PatchConvey("TestGetFuncInfo-NilFuncValue", func() {
			// typed nil function → f.IsNil() == true
			var nilFunc func()
			file, line, name := GetFuncInfo(nilFunc)
			c.So(file, c.ShouldEqual, "")
			c.So(line, c.ShouldEqual, 0)
			c.So(name, c.ShouldEqual, "")
		})

		mockey.PatchConvey("TestGetFuncInfo-FuncForPCNil", func() {
			mockey.Mock(runtime.FuncForPC).Return((*runtime.Func)(nil)).Build()
			file, line, name := GetFuncInfo(TestGetFuncInfo)
			c.So(file, c.ShouldEqual, "")
			c.So(line, c.ShouldEqual, 0)
			c.So(name, c.ShouldEqual, "")
		})

		mockey.PatchConvey("TestGetFuncInfo-NameNoDot", func() {
			// fn.Name() 返回不含 "." 的字符串，走 !found 分支
			mockey.Mock((*runtime.Func).Name).Return("nodotname").Build()
			file, line, name := GetFuncInfo(TestGetFuncInfo)
			c.So(file, c.ShouldEqual, "")
			c.So(line, c.ShouldEqual, 0)
			c.So(name, c.ShouldEqual, "")
		})
	})
}

// ==================== ctx.go ====================

func TestGetTraceIDFromCtx(t *testing.T) {
	mockey.PatchConvey("TestGetTraceIDFromCtx", t, func() {
		mockey.PatchConvey("TestGetTraceIDFromCtx-EmptyCtx", func() {
			c.So(GetTraceIDFromCtx(context.Background()), c.ShouldEqual, "")
		})

		mockey.PatchConvey("TestGetTraceIDFromCtx-ValidSpan", func() {
			ctx := ctxWithValidSpan()
			result := GetTraceIDFromCtx(ctx)
			c.So(result, c.ShouldEqual, "01020304050607080102030405060708")
		})
	})
}

func TestGetSpanIDFromCtx(t *testing.T) {
	mockey.PatchConvey("TestGetSpanIDFromCtx", t, func() {
		mockey.PatchConvey("TestGetSpanIDFromCtx-EmptyCtx", func() {
			c.So(GetSpanIDFromCtx(context.Background()), c.ShouldEqual, "")
		})

		mockey.PatchConvey("TestGetSpanIDFromCtx-ValidSpan", func() {
			ctx := ctxWithValidSpan()
			result := GetSpanIDFromCtx(ctx)
			c.So(result, c.ShouldEqual, "0102030405060708")
		})
	})
}

// ctxWithValidSpan 创建包含有效 Span 的 context（测试辅助）
func ctxWithValidSpan() context.Context {
	traceID, _ := trace.TraceIDFromHex("01020304050607080102030405060708")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithRemoteSpanContext(context.Background(), sc)
}

// ==================== retry.go ====================

func TestRetry(t *testing.T) {
	mockey.PatchConvey("TestRetry", t, func() {
		mockey.PatchConvey("TestRetry-AllFail", func() {
			err := Retry(func() error {
				return errors.New("for test")
			}, 3, 10*time.Millisecond)
			c.So(err.Error(), c.ShouldEqual, "for test")
		})

		mockey.PatchConvey("TestRetry-Success", func() {
			err := Retry(func() error { return nil }, 3, 10*time.Millisecond)
			c.So(err, c.ShouldBeNil)
		})

		mockey.PatchConvey("TestRetry-AttemptsZero", func() {
			calls := 0
			err := Retry(func() error { calls++; return nil }, 0, 10*time.Millisecond)
			c.So(err, c.ShouldBeNil)
			c.So(calls, c.ShouldEqual, 1)
		})

		mockey.PatchConvey("TestRetry-AttemptsNegative", func() {
			calls := 0
			err := Retry(func() error { calls++; return errors.New("fail") }, -1, 10*time.Millisecond)
			c.So(err, c.ShouldNotBeNil)
			c.So(calls, c.ShouldEqual, 1)
		})
	})
}

func TestRetryWithBackoff(t *testing.T) {
	mockey.PatchConvey("TestRetryWithBackoff", t, func() {
		mockey.PatchConvey("TestRetryWithBackoff-AllFail", func() {
			calls := 0
			err := RetryWithBackoff(func() error { calls++; return errors.New("fail") }, 3, 10*time.Millisecond, 100*time.Millisecond)
			c.So(err, c.ShouldNotBeNil)
			c.So(calls, c.ShouldEqual, 3)
		})

		mockey.PatchConvey("TestRetryWithBackoff-SecondSuccess", func() {
			calls := 0
			err := RetryWithBackoff(func() error {
				calls++
				if calls < 2 {
					return errors.New("not yet")
				}
				return nil
			}, 5, 10*time.Millisecond, 1*time.Second)
			c.So(err, c.ShouldBeNil)
			c.So(calls, c.ShouldEqual, 2)
		})

		mockey.PatchConvey("TestRetryWithBackoff-AttemptsZero", func() {
			calls := 0
			err := RetryWithBackoff(func() error { calls++; return nil }, 0, 10*time.Millisecond, 100*time.Millisecond)
			c.So(err, c.ShouldBeNil)
			c.So(calls, c.ShouldEqual, 1)
		})

		mockey.PatchConvey("TestRetryWithBackoff-MaxDelayLimit", func() {
			calls := 0
			err := RetryWithBackoff(func() error { calls++; return errors.New("fail") }, 4, 10*time.Millisecond, 20*time.Millisecond)
			c.So(err, c.ShouldNotBeNil)
			c.So(calls, c.ShouldEqual, 4)
		})
	})
}

// ==================== cmd.go ====================

func TestGetOsArgs(t *testing.T) {
	mockey.PatchConvey("TestGetOsArgs", t, func() {
		args := GetOsArgs()
		// 测试环境下 os.Args[0] 为测试二进制，os.Args[1:] 不为 nil
		c.So(args, c.ShouldNotBeNil)
	})
}

func TestGetConfigFromArgs(t *testing.T) {
	mockey.PatchConvey("TestGetConfigFromArgs", t, func() {
		mockey.PatchConvey("TestGetConfigFromArgs-InvalidKey", func() {
			_, err := GetConfigFromArgs("1a")
			c.So(err.Error(), c.ShouldContainSubstring, "key must match regexp")

			_, err = GetConfigFromArgs("#a")
			c.So(err, c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestGetConfigFromArgs-NoArgs", func() {
			mockey.Mock(GetOsArgs).Return(make([]string, 0)).Build()
			_, err := GetConfigFromArgs("x")
			c.So(err.Error(), c.ShouldEqual, "arg not found, there is no arg")
		})

		mockey.PatchConvey("TestGetConfigFromArgs-Parse", func() {
			mockey.Mock(GetOsArgs).Return(strings.Split("-x.y.z=a_bc --baaa ww ---b===#123 --token=abc== -z", " ")).Build()

			// 空格方式：-z 后无值
			_, err := GetConfigFromArgs("z")
			c.So(err.Error(), c.ShouldEqual, "arg not found, arg not set")

			// 空格方式：--baaa ww
			v, _ := GetConfigFromArgs("baaa")
			c.So(v, c.ShouldEqual, "ww")

			// 等号方式：第一个 = 为分隔符，保留值中的 =
			v, _ = GetConfigFromArgs("b")
			c.So(v, c.ShouldEqual, "==#123")

			// 等号方式：带点号的 key
			v, _ = GetConfigFromArgs("x.y.z")
			c.So(v, c.ShouldEqual, "a_bc")

			// 等号方式：base64 值尾部 == 不被截断
			v, _ = GetConfigFromArgs("token")
			c.So(v, c.ShouldEqual, "abc==")

			// 不存在的 key
			_, err = GetConfigFromArgs("a")
			c.So(err.Error(), c.ShouldEqual, "arg not found")
		})
	})
}
