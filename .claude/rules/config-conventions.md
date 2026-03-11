# Config 配置规范

## 文件与命名

- Config 结构体定义在 `config.go` 文件中
- 配置 key 常量：`X{Module}ConfigKey = "X{Module}"`（如 `XHttpConfigKey = "XHttp"`）
- 默认值合并函数：`configMergeDefault(c *Config) *Config`

## 字段类型选择

| 场景 | 类型 | 判空方式 | 示例 |
|------|------|---------|------|
| 时间配置 | `string` | `== ""` | `Timeout string` → `"60s"` |
| 默认 true 的开关 | `*bool` | `== nil` | `Enable *bool` → `xutil.ToPtr(true)` |
| 默认 false 的开关 | `bool` | 无需判空，零值即默认 | `Console bool` |
| 数值 | `int` | `<= 0` | `MaxIdleConns int` → `100` |
| 可选列表 | `[]T` | `len() == 0` | `HttpDurationBuckets []float64` |
| 键值对 | `map[string]string` | 无需判空，nil 即无配置 | `ConstLabels map[string]string` |

### `*bool` 指针模式详解

当布尔配置的默认值为 `true` 时，**必须**使用 `*bool` 指针类型，以区分"未配置"和"显式关闭"：

```go
// 正确 - 用户未配置时 Enable 为 nil，configMergeDefault 设为 true
// 用户显式配置 Enable: false 时为 &false，不会被覆盖
Enable *bool `mapstructure:"Enable"`

// configMergeDefault 中
if c.Enable == nil {
    c.Enable = xutil.ToPtr(true)
}

// 使用时
if *c.Enable { ... }
```

默认值为 `false` 的布尔配置直接使用 `bool`，零值即默认值，无需特殊处理。

## configMergeDefault() 规范

```go
func configMergeDefault(c *Config) *Config {
    if c == nil {
        c = &Config{}
    }
    // 按字段类型选择判空方式设置默认值
    if c.Timeout == "" {
        c.Timeout = "60s"
    }
    if c.MaxIdleConns <= 0 {
        c.MaxIdleConns = 100
    }
    if c.Enable == nil {
        c.Enable = xutil.ToPtr(true)
    }
    return c
}
```

关键规则：
- 入参 `nil` 时创建空结构体
- **所有默认值在此函数中集中设置**，不分散到其他位置
- 时间配置使用 `xutil.ToDuration()` 解析，禁止使用 `cast.ToDuration()`
- 默认值使用 `xutil.GetOrDefault()` 或直接赋值

## 配置读取原则

### 统一入口

所有模块配置必须通过 `xconfig.UnmarshalConfig()` 反序列化到 Config 结构体：

```go
func getConfig() (*Config, error) {
    c := &Config{}
    if err := xconfig.UnmarshalConfig(XModuleConfigKey, c); err != nil {
        return nil, err
    }
    c = configMergeDefault(c)
    return c, nil
}
```

### 禁止散落读取

初始化后的配置值**禁止**通过 `xconfig.GetBool()` / `xconfig.GetString()` 等直接从 xconfig 读取。
应在初始化时将配置存储到模块级变量，运行时从存储的配置中读取：

```go
// 错误 - 散落在模块各处直接读取 xconfig
func enableMetric() bool {
    return cast.ToBool(xconfig.GetString("XHttp.EnableMetric"))
}

// 正确 - 初始化时存储到模块变量，运行时读取存储值
var metricConfig *Config

func initModule() error {
    c, _ := getConfig()
    metricConfig = c
    if *c.EnableMetric { ... }
    return nil
}
```

原因：
1. 配置值在初始化时已确定，运行时不会变化
2. 散落读取绕过了 `configMergeDefault()` 的默认值逻辑，可能读到零值
3. 类型转换分散在多处，易出错且难以维护

## 字段注释规范

每个 Config 字段必须有注释，格式：

```go
// FieldName 字段用途描述
// optional default "默认值"  或  required
FieldName string `mapstructure:"FieldName"`
```

## 环境变量注入

xconfig 自动展开配置值中的 `${VAR}` / `${VAR:-default}` 占位符，适用于所有 string 类型叶子节点（包括 `map[string]string` 的 value）：

```yaml
XMetric:
  ConstLabels:
    env: "${ENV:-dev}"           # 自动替换为环境变量 ENV 的值
    cluster: "${CLUSTER:-default}"
```

## config_schema.json 同步

新增、修改、删除 Config 字段时，必须同步更新项目根目录的 `config_schema.json`，保持与 Config 结构体一致。

## 禁止事项

- **禁止**保留无用的 config 字段（dead field），发现即删除
- **禁止**使用 `cast.ToDuration()`，统一使用 `xutil.ToDuration()`
- **禁止**在 `configMergeDefault()` 之外设置默认值
- **禁止**初始化后通过 `xconfig.GetXxx()` 散落读取配置值