## XConfig 配置文件解析模块

### 1. 模块简介

* 负责配置文件的解析，是其它模块的基础
* 参考 Spring 的设计方式，用 `application{-profiles}.yml` 区分不同环境
* 支持的文件格式：`.yml` / `.yaml` / `.json`

#### 加载流程

加载过程是一条直线，每一步只做一件事，产物就是下一步的唯一输入：

```
定位 → 解析 → 分层 → 合并 → 展开 → 存储
```

| 步骤 | 做什么 |
|------|--------|
| 定位 | 找到主配置文件（启动参数 > 环境变量 > 约定路径） |
| 解析 | 把每个文件读成 `map[string]any`，key 统一转小写 |
| 分层 | 按优先级从低到高排出所有配置层（主配置、导入的文件、环境变体） |
| 合并 | 逐层深合并成一棵树 |
| 展开 | 统一展开一次 `${VAR}` 占位符 |
| 存储 | 交给 viper 作为只读存储，对外提供读取 API |

中间各步都是 `map[string]any` 上的纯函数，viper 只出现在最后一步。

#### 配置文件路径查找

* 查找优先级: 启动参数 > 环境变量 > `./application.yml` > `./conf/application.yml` > `./config/application.yml` > `./../conf/application.yml` > `./../config/application.yml`
* 各查找方式举例:
    * 启动参数方式：
        ```shell
        --server.config.location=/x/y/z/application.yml
        ```
    * 环境变量方式:
        ```shell
        export SERVER_CONFIG_LOCATION=/x/y/z/application.yml
        ```
* 相对路径基于**进程的工作目录**，容器里取决于镜像的 WORKDIR；路径不确定时请用启动参数或环境变量显式指定
* 同目录下若存在 `.env` 文件会先被加载，其中的变量可供占位符引用

#### 环境（Profiles）启用

* 启用优先级: 启动参数 > 环境变量 > `application.yml` 中的配置
* 各启用方式举例:
    * 启动参数方式：
        ```shell
        --server.profiles.active=dev
        ```
    * 环境变量方式:
        ```shell
        export SERVER_PROFILES_ACTIVE=prod
        ```
    * 配置文件方式:
        ```yaml
        Server:
          Profiles:
            Active: test
        ```
* `Active` 只允许字母、数字、下划线和短横线 —— 它会被拼进配置文件路径，非法字符会导致初始化失败
* `application{-profiles}.yml` 合并到 `application.yml` 的覆盖原则：**逐层深合并**
    * 环境配置文件里只写要改的字段即可，同一块下未提及的字段保留基础配置的值
    * 列表整体替换（半个列表没有意义）
    * `Server.Profiles` 不参与合并：激活的环境由基础配置/启动参数/环境变量决定，环境配置文件不能反过来改写它

    ```yaml
    # application.yml
    XLog: {Level: info, File: {Enable: true, Path: ./log}}
    # application-dev.yml
    XLog: {Level: debug}
    # 合并结果：Level=debug，File.Enable=true，File.Path=./log 都保留
    ```
* 最终配置中的 `Server.Profiles.Active` 是**实际生效的那个值**：用 `--server.profiles.active=prod` 启动时读到的就是 `prod`，而不是配置文件里写的值

#### 空值与空块

* `Timeout: null`（或 `Timeout:` 后面什么都不写）等同于**没写这个字段**，不会把下层同名的值覆盖掉
* 一个块清理完没有任何内容时，视为**没有配置这一块**，`ContainKey` 为 false，对应模块会跳过初始化

    ```yaml
    # application-dev.yml —— 手滑写了个空块
    XLog:
    # 基础配置里的 XLog.Level / XLog.Path 原样保留，不会被抹掉
    ```

    ```yaml
    XRedis:
      Addr: null      # 整块没有有效内容，等同于没配 XRedis，xredis 模块跳过初始化
    ```

* 空列表 `[]` 不在此列，它是一个有意义的取值（「显式置空」），会被保留
* 非字符串的 key（如 `404:`、`true:`）会被转成字符串（`"404"`、`"true"`），之后与普通 key 一视同仁 —— 占位符照常展开，敏感字段照常脱敏

### 2. 配置参数

  ```yaml
  Server:
    Name: "a.b.c"      # 服务名(required)，log/trace都会需要这个配置
    Version: "v2.0.0"  # 服务版本号(optional default "v0.0.1")，trace上报，swagger版本显示等需要使用该配置

    Profiles:          # 环境相关配置(optional default nil)
      Active: "dev"      # 指定启用的环境(required)

    Config:            # 配置文件来源(optional default nil)
      Import:            # 额外加载的配置文件，见下一节
        - db.yml
  ```

> Gin 相关配置已迁移至 `xgin` 模块，请参考 [xgin/README.md](../xgin/README.md)

### 3. 拆分配置文件：Server.Config.Import

所有配置都挤在 `application.yml` 里会越来越难维护。用 `Server.Config.Import` 把它拆开：

```yaml
# conf/application.yml
Server:
  Name: "a.b.c"
  Profiles:
    Active: "dev"
  Config:
    Import:
      - db.yml
      - redis.yml
```

```yaml
# conf/db.yml —— 和 application.yml 放在一起
XGorm:
  Driver: "postgres"
  DSN: "${DB_DSN}"
```

四条规则：

* **路径相对主配置文件所在目录**，不是进程工作目录（容器里后者取决于镜像的 WORKDIR）。也支持绝对路径。
* **被导入的文件同样有环境变体**：`db.yml` 之后会紧接着合并 `db-dev.yml`（如果存在），与 `application.yml` / `application-dev.yml` 同构。
* **被导入的文件覆盖主配置**，与 Spring Boot 的 `spring.config.import` 同向
  （官方文档：*"Values from the imported dev.properties will take precedence over the file that triggered the import."*）。
  这样 `db.yml` 就是数据库配置的权威，不会被 `application.yml` 里一个忘删的残留字段悄悄压过。
  列表内后声明的覆盖先声明的。
* **带环境后缀的文件整体压过不带的**，同样与 Spring 一致
  （官方文档：*"profile-specific files always overriding the non-specific ones"*）。
  即先合完所有不带后缀的，再合所有带后缀的：

  ```
  第一轮（无后缀）：application.yml  <  db.yml  <  redis.yml
  第二轮（带后缀）：application-dev.yml  <  db-dev.yml  <  redis-dev.yml
  第二轮整体覆盖第一轮
  ```

  不这么分两轮的话，`db-dev.yml` 会被后面声明的 `redis.yml` 压过 ——
  一个环境专属的值被非环境专属的值覆盖，说不通。
* **文件不存在直接启动失败**。这与 `application-{env}.yml` 不存在只告警不同 —— 后者是约定俗成的可选项，而这个是你点名要的，静默忽略会让配置莫名其妙缺一块。

`application.yml` 和 `application-{env}.yml` 里的 `Import` 列表**取并集**（去重、保持声明顺序），而不是按「列表整体替换」处理 —— 它是加载指令而非配置数据，否则环境配置想多加一个文件就得把整张列表抄一遍：

```yaml
# application-dev.yml：dev 环境额外加载 mock.yml，db.yml / redis.yml 仍然生效
Server:
  Config:
    Import:
      - mock.yml
```

**不支持嵌套导入**：被导入的文件里再写 `Import` 会被忽略（并打一条 debug 告警）。这样免去了环检测，也避免「这个值到底从哪个文件来」难以追查。被导入的文件同样不能改写 `Server.Profiles.Active`。

导入路径支持环境变量占位符（`${CONFIG_DIR:.}/db.yml`），它在加载之前就会展开。

### 4. 环境变量占位符

配置值中的 `${VAR}` / `${VAR:default}` 会在**合并完成后统一展开一次**，展开结果不会被再次解释。

| 写法 | 含义 |
|------|------|
| `${VAR}` | **必填**。环境变量未设置时初始化直接失败，并列出缺失的变量名 |
| `${VAR:default}` | 可选。环境变量未设置时使用 `default` |
| `${VAR:}` | 可选，默认为空字符串 |

写不出来的占位符（如 `${}`、`${VAR::}`）会**直接启动失败**并指出支持的写法。
以前它们匹配不上就被当成普通文本原样留下 —— `DSN` 真的变成字符串 `"${DB_DSN}"`，
不报错也不告警，错误要到连数据库时才以另一副面孔出现。

环境变量**显式设为空串是一个有效取值**，会覆盖默认值 —— 判断的是"有没有设置"，不是"是不是空"。

在合并之后才展开，是为了让一个被后续层覆盖掉的值不必为它引用的变量负责：
`application.yml` 里写 `DSN: ${DB_DSN}`、`application-dev.yml` 里把它改成本地地址时，
dev 环境不需要设置 `DB_DSN` 也能启动。

> **分隔符是 `:`，与 Spring 一致。** POSIX shell 的 `${VAR:-default}` 写法不再支持，
> 冒号之后的内容一律作为默认值 —— 即 `${VAR:-1}` 的默认值是 `-1`。
>
> 回退**只在变量未设置时**触发。把变量显式设为空串，拿到的就是空串 ——
> 对配置来说「我就是要空值」是个合理表达，不该被默认值顶掉。这一点与 Spring 相同。

覆盖范围是**递归的**：所有 string 叶子节点都会被展开，包括 `map` 的 value、列表元素，
以及列表里嵌套的 map —— xgorm / xredis 的多实例配置正是最后这种形状。

```yaml
XMetric:
  ConstLabels:
    env: "${ENV:dev}"          # map value
XTrace:
  ForwardHeaders:
    - "${TRACE_HEADER:X-Request-Id}"   # 列表元素
XGorm:
  Password: "${DB_PASSWORD}"    # 必填，漏配则启动失败而不是静默变成空串
XRedis:                          # 多实例：列表里嵌套 map
  - Name: "cache"
    Password: "${REDIS_CACHE_PASSWORD}"
  - Name: "session"
    Password: "${REDIS_SESSION_PASSWORD}"
```

### 5. 配置打印与脱敏

开启 `XONE_ENABLE_DEBUG` 时会打印最终生效的完整配置，以及它的全部来源（按优先级从低到高）：

```
************************************** XOne load config **************************************
sources (low -> high precedence):
  1. conf/application.yml
  2. conf/db.yml
  3. conf/application-dev.yml
----------------------------------------------------------------------------------------------
{ ... }
**********************************************************************************************
```

其中字段名命中 `password / passwd / secret / token / apikey / accesskey / privatekey / credential / dsn`
（大小写不敏感）的值会被替换为 `******`，不会输出凭证明文。

脱敏同样是递归的，会穿过列表：xgorm / xredis 的多实例配置是一个 map 列表，
里面的 `DSN`、`Password` 与单实例形态一样被替换。

排查「这个值到底从哪来」时，也可以在代码里调用 `xconfig.Sources()` 拿到同一份来源列表。

### 6. 对外 API

| 方法 | 说明 |
|------|------|
| `UnmarshalConfig(key string, conf any) error` | 把 key 对应的配置反序列化到结构体，**模块配置一律走这个入口** |
| `GetConfig(key string) any` | 读取原始值，key 不存在时返回 nil |
| `ContainKey(key string) bool` | 判断 key 是否存在 |
| `GetString(key string) string` | 按 string 读取 |
| `GetInt(key string) int` | 按 int 读取 |
| `GetBool(key string) bool` | 按 bool 读取 |
| `GetServerName() string` | `Server.Name`，未配置时返回默认值 |
| `GetServerVersion() string` | `Server.Version`，未配置时返回 `v0.0.1` |
| `GetProfilesActive() string` | 实际激活的环境，未启用时返回空串 |
| `Sources() []string` | 本次实际加载的配置文件，按优先级从低到高 |

> 模块内部**不要**用 `GetXxx()` 散落读取配置：初始化时通过 `UnmarshalConfig` 一次性读进
> Config 结构体，运行时从结构体里取。理由见 `.claude/rules/config-conventions.md`。

### 7. 使用 demo

* 配置
  ```yaml
  MyConfig:
    X: 1
    Y: "zz"
  ```
* 读取
  ```go
  package main

  import "github.com/xiaoshicae/xone/v3/xconfig"

  // 字段名有特殊命名方式（驼峰映射成下划线等）时，需要用 tag mapstructure 映射
  type MyConfig struct {
      X int    `mapstructure:"X"`
      Y string `mapstructure:"Y"`
  }

  func main() {
      myConfig := &MyConfig{}
      _ = xconfig.UnmarshalConfig("MyConfig", myConfig)
      println("get config myConfig: ", myConfig.X, myConfig.Y)
  }
  ```

* 配置 key 里可以含 `.`（如 OpenTelemetry 的 `service.name`），它会被当成一个完整的 key，
  不会被拆成两层嵌套：

  ```yaml
  XMetric:
    ConstLabels:
      service.name: "demo"        # map[string]string 里的一个 key
  ```

### 8. 其它模块配置参数说明

* 其它块配置参数，参考相应模块的 README.md
