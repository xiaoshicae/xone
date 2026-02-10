---
globs:
  - "**/*_test.go"
  - "test/**"
alwaysApply: false
---

# Go 测试规范

## 核心原则

1. **依赖必须 Mock**：外部依赖（HTTP/DB/缓存/时间）一律 mock
2. **职责单一**：一个测试函数只验证一个行为/场景
3. **必须禁用内联**：运行测试必须使用 `-gcflags="all=-N -l"` 以支持 Mockey

## 工具链

| 用途 | 库 |
|------|------|
| Mock | `github.com/bytedance/mockey` |
| 断言 | `github.com/smartystreets/goconvey/convey` |

## 测试位置

- **单元测试**：每个模块内 `x{模块名}_test.go`，使用 Mockey + GoConvey
- **集成测试**：`test/` 目录下按模块组织，依赖外部环境（DB/Redis 等），默认 `t.Skip()` 跳过，需手动指定运行，**禁止在自动化流程中执行**
- **判断原则**：需要 Mock 内部函数的放模块内，需要真实环境的放 `test/`

## 命名约定

- 测试函数：`Test{函数名}_{场景}`
- PatchConvey 描述：`"Test{函数名}-{场景}"`

## Mockey 使用规范

### 基本用法：PatchConvey + Mock

`PatchConvey` 是 mockey 的核心入口，退出作用域自动释放所有 mock，无需手动清理。

```go
import (
    . "github.com/bytedance/mockey"
    . "github.com/smartystreets/goconvey/convey"
)

func TestGetConfig(t *testing.T) {
    PatchConvey("TestGetConfig-Default", t, func() {
        // mock 函数
        Mock(xconfig.GetString).Return("test-value").Build()

        result := GetConfig()
        So(result, ShouldNotBeNil)
        So(result.Value, ShouldEqual, "test-value")
    })
    // PatchConvey 外 mock 自动释放
}
```

### Mock 函数

```go
Mock(Foo).Return("mocked").Build()
```

### Mock 结构体方法

```go
// 值接收者
Mock(MyStruct.Method).Return("result", nil).Build()

// 指针接收者
Mock((*MyStruct).Method).Return("result", nil).Build()
```

### Mock 全局变量

```go
MockValue(&config.MaxRetry).To(5)
```

### 条件 Mock（When）

根据入参决定是否走 mock 逻辑：

```go
Mock((*Service).Process).
    When(func(ctx context.Context, key string) bool { return key == "special" }).
    Return("mocked", nil).
    Build()
```

### 序列返回（Sequence）

多次调用返回不同值：

```go
Mock(httpClient.Do).
    Return(Sequence(resp200).Times(2).Then(resp500)).
    Build()
```

### 调用次数断言

```go
mock := Mock(Foo).Return("ok").Build()
Foo("a")
Foo("b")
assert.Equal(t, 2, mock.Times())     // 总调用次数
assert.Equal(t, 2, mock.MockTimes()) // 命中 mock 的次数
```

### 嵌套 PatchConvey

```go
func TestModule(t *testing.T) {
    PatchConvey("TestModule", t, func() {
        // 公共 mock
        Mock(xconfig.ContainKey).Return(true).Build()

        PatchConvey("场景A-正常", func() {
            Mock(initFunc).Return(nil).Build()
            So(DoSomething(), ShouldBeNil)
        })

        PatchConvey("场景B-异常", func() {
            Mock(initFunc).Return(errors.New("fail")).Build()
            So(DoSomething(), ShouldNotBeNil)
        })
    })
}
```

## 断言

- GoConvey 断言：`So(actual, ShouldEqual, expected)`
- 常用断言：`ShouldEqual`、`ShouldNotBeNil`、`ShouldBeNil`、`ShouldResemble`（切片/结构体）、`ShouldBeEmpty`、`ShouldContainSubstring`

## 覆盖要求

- 每个公开函数至少一个正向测试
- 边界条件和错误路径需覆盖
- 增量代码覆盖率 >= 60%
- 重构后测试数量不得减少

## 常用命令

```bash
# 运行所有测试
go test -gcflags="all=-N -l" ./...

# 运行单个模块测试
go test -gcflags="all=-N -l" ./xhttp/... -v

# 覆盖率检查
go test -gcflags="all=-N -l" -coverprofile=coverage.out ./xhttp/...
go tool cover -func=coverage.out

# 竞态检测
go test -gcflags="all=-N -l" -race ./...
```