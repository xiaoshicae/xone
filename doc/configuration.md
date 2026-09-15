# 配置参考

XOne 的全部配置集中在一个 YAML 文件里，各模块按自己的顶层 key 读取。
本文覆盖配置文件的位置与加载规则、多环境、环境变量注入、IDE 补全，以及全部配置项。

各模块配置项的完整说明（含默认值与取舍原因）见对应模块的 README。

## 配置文件位置

优先级从高到低：启动参数 `--server.config.location` → 环境变量 `SERVER_CONFIG_LOCATION` → 按下列路径依次查找：

```
./application.yml            ./application.yaml
./conf/application.yml       ./conf/application.yaml
./config/application.yml     ./config/application.yaml
../conf/application.yml      ../conf/application.yaml
../config/application.yml    ../config/application.yaml
```

`../` 那几条是为 `go test` 在子目录下运行时仍能找到项目根的配置准备的。

**配置文件不存在不会导致启动失败** —— 各模块会退回默认值，只有显式声明了必填项（如 `XGorm.DSN`）
且对应的顶层 key 存在时才会报错。

## 多环境配置（Profile）

通过 Profile 加载不同环境的配置文件：

```yaml
# application.yml（公共配置）
Server:
  Name: "my-service"
  Profiles:
    Active: "dev"    # 指定环境
```

框架会按顺序加载：`application.yml` → `application-dev.yml`，后者覆盖前者同名配置。

也可通过环境变量或启动参数指定：

```bash
# 环境变量
export SERVER_PROFILES_ACTIVE=prod

# 启动参数
go run main.go --server.profiles.active=prod
```

## 环境变量

| 环境变量                     | 说明                             | 示例                |
|--------------------------|--------------------------------|-------------------|
| `XONE_ENABLE_DEBUG`      | 启用框架自身的调试日志（初始化过程、配置解析结果等）     | `true`            |
| `SERVER_PROFILES_ACTIVE` | 指定激活的配置环境                      | `dev`、`prod`      |
| `SERVER_CONFIG_LOCATION` | 指定配置文件绝对路径                     | `/app/config.yml` |

`SERVER_ENABLE_DEBUG` 是 `XONE_ENABLE_DEBUG` 的旧名，仍然生效，新项目用前者。

## 拆分配置文件

配置都堆在 `application.yml` 里难以维护时，用 `Server.Config.Import` 拆开：

```yaml
# conf/application.yml
Server:
  Name: "my-service"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml       # 路径相对本文件所在目录
      - redis.yml
```

加载顺序（后者覆盖前者）：

```
第一轮（无环境后缀）：application.yml → db.yml → redis.yml
第二轮（带环境后缀）：application-dev.yml → db-dev.yml → redis-dev.yml
第二轮整体覆盖第一轮
```

* 被导入的文件**同样支持** `{name}-{env}.yml` 环境变体
* 被导入的文件**覆盖主配置**（与 Spring Boot 的 `spring.config.import` 同向）
* **带环境后缀的文件整体压过不带的**（同 Spring：profile-specific files always overriding the non-specific ones）
* 被导入的文件**不存在时启动失败**（与 `application-{env}.yml` 不存在只告警不同）
* `application.yml` 与 `application-{env}.yml` 的 `Import` 列表**取并集**，环境配置只需写它额外要加的
* **不支持嵌套导入**，被导入文件里的 `Import` 会被忽略

完整说明见 [xconfig/README.md](../xconfig/README.md#3-拆分配置文件serverconfigimport)。

### 配置值的环境变量注入

配置文件里所有 string 叶子节点支持占位符（含 `map` 的 value、列表元素，以及列表里嵌套的 map），
在配置合并完成后统一展开一次，展开结果不会被再次解释：

| 写法 | 含义 |
|------|------|
| `${VAR}` | **必填**，环境变量未设置时初始化失败 |
| `${VAR:default}` | 可选，未设置时用 `default` |
| `${VAR:}` | 可选，未设置时为空字符串 |

判断依据是「有没有设置」而不是「是不是空」—— 显式设为空串的环境变量会覆盖默认值（与 Spring 相同）。

分隔符是 `:`，与 Spring 一致，冒号之后的内容一律作为默认值（`${VAR:-1}` 的默认值就是 `-1`）。
写不出来的占位符（如 `${}`）会直接启动失败，不会被当成普通文本留在配置值里。
凭证类配置建议用 `${VAR}` 形式，漏配时直接启动失败，而不是静默变成空串：

```yaml
XGorm:
  DSN: "${DB_DSN}"                                   # 漏配直接启动失败
XRedis:
  Addr: "${REDIS_ADDR:127.0.0.1:6379}"              # 漏配用默认值
XMetric:
  ConstLabels:
    env: "${ENV:dev}"
```

## IDE 配置补全与校验

项目提供了 JSON Schema 文件，配置后 IDE 会自动补全字段并校验配置值。

**VS Code**（需安装 [YAML 扩展](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml)）

在 YAML 文件首行添加：

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/xiaoshicae/xone/main/config_schema.json
```

或在项目 `.vscode/settings.json` 中统一配置：

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/xiaoshicae/xone/main/config_schema.json": [
      "application.yml",
      "application-*.yml"
    ]
  }
}
```

**JetBrains（GoLand / IntelliJ）**

`Settings → Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings`，添加映射：

- Schema URL：`https://raw.githubusercontent.com/xiaoshicae/xone/main/config_schema.json`
- Schema version：JSON Schema version 7
- 文件匹配：`application*.yml`

## 常用配置项

下面是各模块最常用的配置。**完整字段列表以 [`config_schema.json`](../config_schema.json) 为准**
（配好 IDE 后直接自动补全），每个字段的取值含义与默认值原因见对应模块的 README。

```yaml
Server:
  Name: "my-service"          # 服务名（必填）
  Version: "v1.0.0"           # 版本号（默认 v0.0.1）
  Profiles:
    Active: "dev"              # 环境标识
  Config:
    Import: []                 # 额外加载的配置文件，见「拆分配置文件」

XGin:
  Host: "0.0.0.0"              # 监听地址（默认 0.0.0.0）
  Port: 8080                   # 监听端口（默认 8080）
  UseH2C: false                # 非 TLS 下启用 h2c（HTTP/2 Cleartext）
  CertFile: ""                 # TLS 证书路径（配置后自动启用 HTTPS，需与 KeyFile 同时配置）
  KeyFile: ""                  # TLS 私钥路径
  ReadHeaderTimeout: "10s"     # 读请求头超时（默认 10s），slowloris 的主要防线
  IdleTimeout: "60s"           # keep-alive 空闲超时（默认 60s）
  ReadTimeout: ""              # 读整个请求超时（默认不限制，限制会打断大文件上传）
  WriteTimeout: ""             # 写响应超时（默认不限制，限制会打断 SSE / 长轮询）
  GracefulStopTimeout: "25s"   # 优雅退出超时（默认 25s），应小于部署环境的进程终止宽限期

XLog:
  Level: "info"                # 日志级别（默认 info），两路输出共用
  Timezone: "Asia/Shanghai"    # 时区（默认 Asia/Shanghai），两路输出共用
  Console:
    Enable: true               # 控制台输出（默认 true）
    Format: "text"             # 输出格式（默认 text），可选 text / json
  File:
    Enable: false              # 写日志文件（默认 false，仅输出到标准输出）
    Path: "./log"              # 日志文件夹（默认 ./log）
    Name: "app"                # 日志文件名（默认 app）
    MaxAge: "7d"               # 日志保留时长（默认 7d）
    RotateTime: "1d"           # 切割周期（默认 1d，最小 1m）

XTrace:
  Enable: true                 # 启用链路追踪（默认 true）
  EnableConsole: false         # Span 打印到控制台（默认 false，仅调试用）
  SampleRatio: 1               # 采样率（默认 1，即全采样）
  ShutdownTimeout: "5s"        # 关闭时等待 Span 上报完成的超时（默认 5s）
  ForwardHeaders:              # 全局透传的自定义 Header（默认无）
    - X-Request-Id
  ForwardHeaderRules:          # 按域名透传的 Header 规则（默认无）
    - Domains:                 # 域名模式，支持 *.example.com 通配
        - "*.svc.cluster.local"
      Headers:                 # 仅匹配域名时才透传的 Header
        - X-Auth-Token
        - X-Tenant-Id

XMetric:
  Namespace: "myapp"             # 指标命名空间前缀
  ConstLabels:                   # 全局常量标签（区分环境等）
    env: "prod"
  HttpDurationBuckets:           # HTTP 入站/出站请求耗时桶边界（秒，默认 [0.001,0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5,10]）
    - 0.001
    - 0.005
    - 0.01
    - 0.025
    - 0.05
    - 0.1
    - 0.25
    - 0.5
    - 1
    - 2.5
    - 5
    - 10
  HistogramObserveBuckets:       # HistogramObserve() 业务指标桶边界（秒，默认 prometheus.DefBuckets）
    - 0.005
    - 0.01
    - 0.025
    - 0.05
    - 0.1
    - 0.25
    - 0.5
    - 1
    - 2.5
    - 5
    - 10
  EnableGoMetrics: true          # Go runtime 指标（默认 true）
  EnableProcessMetrics: true     # 进程指标（默认 true）
  EnableLogErrorMetric: true     # xlog.Error 自动上报（默认 true）

XHttp:
  Timeout: "60s"               # 请求超时（默认 60s）
  RetryCount: 3                # 重试次数（默认 0，即不重试）
  RetryOnlyIdempotent: true    # 只重试幂等方法（默认 true），关掉后 POST 超时也会重发
  MaxIdleConns: 100            # 最大空闲连接（默认 100）
  EnableMetric: true           # 出站请求指标采集（默认 true）

XGorm:
  Driver: "postgres"           # 驱动（默认 postgres），可选 postgres / mysql
  DSN: ""                      # 连接字符串（必填）
  MaxOpenConns: 50             # 最大连接数（默认 50）
  MaxIdleConns: 50             # 最大空闲连接（默认等于 MaxOpenConns，显式配 0 表示不保留空闲连接）
  EnableLog: false             # 开启 SQL 日志（默认 false）
  SlowThreshold: "3s"          # 慢查询阈值（默认 3s）
  EnableMetric: true           # 连接池指标采集（默认 true）

XRedis:
  Addr: "127.0.0.1:6379"       # 服务器地址（默认 localhost:6379）
  Password: "${REDIS_PASSWORD}"  # 认证密码
  DB: 0                        # 数据库编号（默认 0）
  DialTimeout: "500ms"         # 建连超时（默认 500ms）
  MinIdleConns: 5              # 最小空闲连接（默认 5），多实例时每个实例都会常驻这么多
  EnableMetric: true           # 连接池指标采集（默认 true）

XCache:
  MaxCost: 100000              # 缓存最大成本，每条 cost=1 时等价于最大条目数（默认 100000）
  NumCounters: 1000000         # 频率统计键数量，建议为期望条目数的 10 倍（默认 1000000）
  DefaultTTL: "5m"             # 默认过期时间（默认 5m）

XFlow:
  EnableMonitor: true          # 启用流程监控回调（默认 true）
  RollbackTimeout: "30s"       # 回滚总预算（默认 30s），耗尽后未补偿的 Processor 会逐个记入 RollbackErrors
```

`XGorm` / `XRedis` / `XCache` 都支持多实例：把对象换成数组，每个元素加一个 `Name` 即可，
通过 `xgorm.C("master")`、`xredis.C("cache")` 取用。示例见各模块 README。
