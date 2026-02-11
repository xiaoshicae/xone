package xutil

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

// ==================== convert.go ====================

func TestToPrt(t *testing.T) {
	mockey.PatchConvey("TestToPrt", t, func() {
		intVal := 42
		ptrVal := ToPrt(intVal)
		c.So(*ptrVal, c.ShouldEqual, 42)

		strVal := "hello"
		ptrStr := ToPrt(strVal)
		c.So(*ptrStr, c.ShouldEqual, "hello")
	})
}

func TestGetOrDefault(t *testing.T) {
	mockey.PatchConvey("TestGetOrDefault", t, func() {
		mockey.PatchConvey("TestGetOrDefault-ZeroValue", func() {
			result := GetOrDefault(0, 100)
			c.So(result, c.ShouldEqual, 100)

			result2 := GetOrDefault("", "default")
			c.So(result2, c.ShouldEqual, "default")
		})

		mockey.PatchConvey("TestGetOrDefault-NonZeroValue", func() {
			result := GetOrDefault(42, 100)
			c.So(result, c.ShouldEqual, 42)

			result2 := GetOrDefault("hello", "default")
			c.So(result2, c.ShouldEqual, "hello")
		})
	})
}

func TestToDuration(t *testing.T) {
	mockey.PatchConvey("TestToDuration", t, func() {
		mockey.PatchConvey("TestToDuration-Nil", func() {
			result := ToDuration(nil)
			c.So(result, c.ShouldEqual, 0)
		})

		mockey.PatchConvey("TestToDuration-String", func() {
			result := ToDuration("1s")
			c.So(result, c.ShouldEqual, time.Second)

			result2 := ToDuration("100ms")
			c.So(result2, c.ShouldEqual, 100*time.Millisecond)
		})

		mockey.PatchConvey("TestToDuration-StringPointer", func() {
			s := "2s"
			result := ToDuration(&s)
			c.So(result, c.ShouldEqual, 2*time.Second)
		})

		mockey.PatchConvey("TestToDuration-WithDay", func() {
			result := ToDuration("1d")
			c.So(result, c.ShouldEqual, 24*time.Hour)

			result2 := ToDuration("2d12h")
			c.So(result2, c.ShouldEqual, 60*time.Hour)
		})

		mockey.PatchConvey("TestToDuration-InvalidDay", func() {
			result := ToDuration("abcd12h")
			c.So(result, c.ShouldEqual, 12*time.Hour) // invalid day part, fallback to remaining
		})

		mockey.PatchConvey("TestToDuration-Int", func() {
			result := ToDuration(1000000000) // 1 second in nanoseconds
			c.So(result, c.ShouldEqual, time.Second)
		})
	})
}

// ==================== json.go ====================

func TestToJsonString(t *testing.T) {
	mockey.PatchConvey("TestToJsonString", t, func() {
		mockey.PatchConvey("TestToJsonString-Map", func() {
			m := map[string]string{"key": "value"}
			result := ToJsonString(m)
			c.So(result, c.ShouldEqual, `{"key":"value"}`)
		})

		mockey.PatchConvey("TestToJsonString-Struct", func() {
			type Person struct {
				Name string `json:"name"`
				Age  int    `json:"age"`
			}
			p := Person{Name: "Alice", Age: 30}
			result := ToJsonString(p)
			c.So(result, c.ShouldEqual, `{"name":"Alice","age":30}`)
		})

		mockey.PatchConvey("TestToJsonString-InvalidValue", func() {
			// channel cannot be marshaled to JSON
			ch := make(chan int)
			result := ToJsonString(ch)
			c.So(result, c.ShouldEqual, "")
		})
	})
}

func TestToJsonStringIndent(t *testing.T) {
	mockey.PatchConvey("TestToJsonStringIndent", t, func() {
		mockey.PatchConvey("TestToJsonStringIndent-Map", func() {
			m := map[string]string{"key": "value"}
			result := ToJsonStringIndent(m)
			c.So(result, c.ShouldContainSubstring, "key")
			c.So(result, c.ShouldContainSubstring, "value")
		})

		mockey.PatchConvey("TestToJsonStringIndent-InvalidValue", func() {
			ch := make(chan int)
			result := ToJsonStringIndent(ch)
			c.So(result, c.ShouldEqual, "")
		})
	})
}

// ==================== net.go ====================

func TestGetLocalIp(t *testing.T) {
	mockey.PatchConvey("TestGetLocalIp", t, func() {
		ip, err := GetLocalIp()
		c.So(err, c.ShouldBeNil)
		c.So(ip, c.ShouldNotBeEmpty)
	})
}

func TestExtractRealIP(t *testing.T) {
	mockey.PatchConvey("TestExtractRealIP", t, func() {
		mockey.PatchConvey("TestExtractRealIP-SpecificAddr", func() {
			ip, err := ExtractRealIP("192.168.1.1")
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "192.168.1.1")
		})

		mockey.PatchConvey("TestExtractRealIP-WithPort", func() {
			ip, err := ExtractRealIP("192.168.1.1:8080")
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "192.168.1.1")
		})

		mockey.PatchConvey("TestExtractRealIP-ZeroAddr", func() {
			ip, err := ExtractRealIP("0.0.0.0")
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldNotBeEmpty)
		})

		mockey.PatchConvey("TestExtractRealIP-IPv6Zero", func() {
			ip, err := ExtractRealIP("[::]")
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldNotBeEmpty)
		})

		mockey.PatchConvey("TestExtractRealIP-InvalidIP", func() {
			_, err := ExtractRealIP("invalid-ip")
			c.So(err, c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestExtractRealIP-IPv6Brackets", func() {
			ip, err := ExtractRealIP("[::1]")
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "::1")
		})
	})
}

func TestValidateIP(t *testing.T) {
	mockey.PatchConvey("TestValidateIP", t, func() {
		mockey.PatchConvey("TestValidateIP-Valid", func() {
			ip, err := validateIP("192.168.1.1", "192.168.1.1")
			c.So(err, c.ShouldBeNil)
			c.So(ip, c.ShouldEqual, "192.168.1.1")
		})

		mockey.PatchConvey("TestValidateIP-Invalid", func() {
			_, err := validateIP("invalid", "invalid")
			c.So(err, c.ShouldNotBeNil)
		})
	})
}

func TestIsPrivateIP(t *testing.T) {
	mockey.PatchConvey("TestIsPrivateIP", t, func() {
		mockey.PatchConvey("TestIsPrivateIP-Nil", func() {
			result := isPrivateIP(nil)
			c.So(result, c.ShouldBeFalse)
		})
	})
}

// ==================== log.go ====================

func TestLogFunctions(t *testing.T) {
	mockey.PatchConvey("TestLogFunctions", t, func() {
		mockey.Mock(EnableDebug).Return(true).Build()

		mockey.PatchConvey("TestInfoIfEnableDebug", func() {
			// Should not panic
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
		mockey.Mock(EnableDebug).Return(false).Build()

		mockey.PatchConvey("TestInfoIfEnableDebug-Disabled", func() {
			InfoIfEnableDebug("test message %s", "arg")
		})
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
		funcName, fileName := callerPretty(nil)
		c.So(funcName, c.ShouldEqual, "")
		c.So(fileName, c.ShouldNotBeEmpty)
	})
}

// ==================== env.go ====================

func TestEnableDebug(t *testing.T) {
	mockey.PatchConvey("TestEnableDebug", t, func() {
		mockey.PatchConvey("TestEnableDebug-True", func() {
			os.Setenv(DebugKey, "true")
			defer os.Unsetenv(DebugKey)
			c.So(EnableDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-1", func() {
			os.Setenv(DebugKey, "1")
			defer os.Unsetenv(DebugKey)
			c.So(EnableDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-Yes", func() {
			os.Setenv(DebugKey, "yes")
			defer os.Unsetenv(DebugKey)
			c.So(EnableDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-On", func() {
			os.Setenv(DebugKey, "on")
			defer os.Unsetenv(DebugKey)
			c.So(EnableDebug(), c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestEnableDebug-False", func() {
			os.Setenv(DebugKey, "false")
			defer os.Unsetenv(DebugKey)
			c.So(EnableDebug(), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestEnableDebug-Empty", func() {
			os.Unsetenv(DebugKey)
			c.So(EnableDebug(), c.ShouldBeFalse)
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
			name := GetFuncName(TestGetFuncName)
			c.So(name, c.ShouldEqual, "TestGetFuncName")
		})

		mockey.PatchConvey("TestGetFuncName-Nil", func() {
			name := GetFuncName(nil)
			c.So(name, c.ShouldEqual, "")
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
	})
}

// ==================== ctx.go ====================

func TestGetTraceIDFromCtx(t *testing.T) {
	mockey.PatchConvey("TestGetTraceIDFromCtx", t, func() {
		mockey.PatchConvey("TestGetTraceIDFromCtx-EmptyCtx", func() {
			traceID := GetTraceIDFromCtx(context.Background())
			c.So(traceID, c.ShouldEqual, "")
		})
	})
}

func TestGetSpanIDFromCtx(t *testing.T) {
	mockey.PatchConvey("TestGetSpanIDFromCtx", t, func() {
		mockey.PatchConvey("TestGetSpanIDFromCtx-EmptyCtx", func() {
			spanID := GetSpanIDFromCtx(context.Background())
			c.So(spanID, c.ShouldEqual, "")
		})
	})
}

// ==================== retry.go ====================

func TestRetry(t *testing.T) {
	mockey.PatchConvey("TestRetry", t, func() {
		mockey.PatchConvey("TestRetry-AllFail", func() {
			err := Retry(func() error {
				return errors.New("for test")
			}, 3, time.Millisecond*100)
			c.So(err.Error(), c.ShouldEqual, "for test")
		})

		mockey.PatchConvey("TestRetry-Success", func() {
			err := Retry(func() error {
				return nil
			}, 3, time.Millisecond*100)
			c.So(err, c.ShouldBeNil)
		})

		mockey.PatchConvey("TestRetry-AttemptsZero", func() {
			calls := 0
			err := Retry(func() error {
				calls++
				return nil
			}, 0, time.Millisecond*100)
			c.So(err, c.ShouldBeNil)
			c.So(calls, c.ShouldEqual, 1)
		})

		mockey.PatchConvey("TestRetry-AttemptsNegative", func() {
			calls := 0
			err := Retry(func() error {
				calls++
				return errors.New("for test")
			}, -1, time.Millisecond*100)
			c.So(err.Error(), c.ShouldEqual, "for test")
			c.So(calls, c.ShouldEqual, 1)
		})
	})
}

func TestRetryWithBackoff(t *testing.T) {
	mockey.PatchConvey("TestRetryWithBackoff", t, func() {
		mockey.PatchConvey("TestRetryWithBackoff-AllFail", func() {
			calls := 0
			err := RetryWithBackoff(func() error {
				calls++
				return errors.New("backoff fail")
			}, 3, 10*time.Millisecond, 100*time.Millisecond)
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldEqual, "backoff fail")
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
			err := RetryWithBackoff(func() error {
				calls++
				return nil
			}, 0, 10*time.Millisecond, 100*time.Millisecond)
			c.So(err, c.ShouldBeNil)
			c.So(calls, c.ShouldEqual, 1)
		})

		mockey.PatchConvey("TestRetryWithBackoff-MaxDelayLimit", func() {
			delays := make([]time.Time, 0)
			err := RetryWithBackoff(func() error {
				delays = append(delays, time.Now())
				return errors.New("fail")
			}, 4, 10*time.Millisecond, 20*time.Millisecond)
			c.So(err, c.ShouldNotBeNil)
			c.So(len(delays), c.ShouldEqual, 4)
		})
	})
}

// ==================== cmd.go ====================

func TestGetConfigFromArgs(t *testing.T) {
	mockey.PatchConvey("TestGetConfigFromArgs", t, func() {
		mockey.PatchConvey("TestGetConfigFromArgs-InvalidKey", func() {
			_, err := GetConfigFromArgs("1a")
			c.So(err.Error(), c.ShouldEqual, "key must match regexp: ^[a-zA-Z_][a-zA-Z0-9_.-]*$")

			_, err = GetConfigFromArgs("#a")
			c.So(err.Error(), c.ShouldEqual, "key must match regexp: ^[a-zA-Z_][a-zA-Z0-9_.-]*$")
		})

		mockey.PatchConvey("TestGetConfigFromArgs-NotFound", func() {
			mockey.Mock(GetOsArgs).Return(make([]string, 0)).Build()
			_, err := GetConfigFromArgs("x")
			c.So(err.Error(), c.ShouldEqual, "arg not found, there is no arg")
		})

		mockey.PatchConvey("TestGetConfigFromArgs-Parse", func() {
			mockey.Mock(GetOsArgs).Return(strings.Split("-x.y.z=a_bc --baaa ww ---b===#123 -z", " ")).Build()
			_, err := GetConfigFromArgs("z")
			c.So(err.Error(), c.ShouldEqual, "arg not found, arg not set")

			v, _ := GetConfigFromArgs("baaa")
			c.So(v, c.ShouldEqual, "ww")

			v, _ = GetConfigFromArgs("b")
			c.So(v, c.ShouldEqual, "#123")

			v, _ = GetConfigFromArgs("x.y.z")
			c.So(v, c.ShouldEqual, "a_bc")

			_, err = GetConfigFromArgs("a")
			c.So(err.Error(), c.ShouldEqual, "arg not found")
		})
	})
}
