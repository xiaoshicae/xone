## XConfig配置文件解析模块

### 1. 模块简介

* 负责配置文件的解析，是其它模块的基础
* XConfig提供的方法请参考util.go
* 配置文件不同环境启用原则:
  > 参考了spring设计方式，采用 _application{-profiles}.yml_ 区分不同环境
    * 启用优先级: 启动参数 > 环境变量 > application.yml中的配置
    * 各启用方式举例:
        * 启动参数方式：
            ```shell
            --server.profiles.active=dev
            ```
        * 环境变量方式:
            ```shell
            export SERVER_PROFILES_ACTIVE=prod
            ```
        * application.yml 配置文件方式:
            ```yaml
            Server:
              Profiles:
                Active: test
            ```
    * `Active` 只允许字母、数字、下划线和短横线 —— 它会被拼进配置文件路径，非法字符会导致初始化失败
    * application{-profiles}.yml 合并到 application.yml 的覆盖原则：**逐层深合并**
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

* 配置文件路径查找原则:
    * 查找优先级: 启动参数 > 环境变量 > ./application.yml > ./conf/application.yml > ./config/application.yml > ./../conf/application.yml > ./../config/application.yml
    * 各查找方式举例:
        * 启动参数方式：
            ```shell
            --server.config.location=/x/y/z/application.yml
            ```
        * 环境变量方式:
            ```shell
            export SERVER_CONFIG_LOCATION=/x/y/z/application.yml
            ```
        * 配置文件application.yml，默认优先级 ./ > ./conf > ./config
    * 相对路径基于**进程的工作目录**，容器里取决于镜像的 WORKDIR；路径不确定时请用启动参数或环境变量显式指定

* 同目录下若存在 `.env` 文件会先被加载，其中的变量可供占位符引用

### 2. 配置参数

  ```yaml
  Server:
    Name: "a.b.c"      # 服务名(required)，log/trace都会需要这个配置
    Version: "v2.0.0"  # 服务版本号(optional default "v0.0.1")，trace上报，swagger版本显示等需要使用该配置

    Profiles:          # 环境相关配置(optional default nil)
      Active: "dev"      # 指定启用的环境(required)
  ```

> Gin 相关配置已迁移至 `xgin` 模块，请参考 [xgin/README.md](../xgin/README.md)

### 3. 环境变量占位符

配置值中的 `${VAR}` / `${VAR:-default}` 会在**合并完成后统一展开一次**，展开结果不会被再次解释。

| 写法 | 含义 |
|------|------|
| `${VAR}` | **必填**。环境变量未设置时初始化直接失败，并列出缺失的变量名 |
| `${VAR:-default}` | 可选。环境变量未设置时使用 `default` |
| `${VAR:-}` | 可选，默认为空字符串 |

环境变量**显式设为空串是一个有效取值**，会覆盖默认值 —— 判断的是"有没有设置"，不是"是不是空"。

覆盖范围为 string 叶子节点、`map` 的 value 以及**字符串列表的元素**：

```yaml
XMetric:
  ConstLabels:
    env: "${ENV:-dev}"          # map value
XTrace:
  ForwardHeaders:
    - "${TRACE_HEADER:-X-Request-Id}"   # 列表元素
XGorm:
  Password: "${DB_PASSWORD}"    # 必填，漏配则启动失败而不是静默变成空串
```

### 4. 配置打印与脱敏

开启 `XONE_ENABLE_DEBUG` 时会打印最终生效的完整配置，其中字段名命中
`password / passwd / secret / token / apikey / accesskey / privatekey / credential / dsn`
（大小写不敏感）的值会被替换为 `******`，不会输出凭证明文。

### 5. 使用demo

* 配置
  ```yaml
  MyConfig:
    X: 1
    Y: "zz"
  ```
* 读取
  ```go
  package main

  import "github.com/xiaoshicae/xone/v2/xconfig"

  // 如果结构体名称, 如果字段名称有特殊命名方式(驼峰映射成下划线等)，需要tag mapstructure 进行映射
  // 具体使用方法请参考: https://github.com/spf13/viper
  type MyConfig  struct {
      X int    `mapstructure:"x"`
      Y string
  }

  func main() {
      x := xconfig.GetInt("MyConfig.X")
      println("get config x: ", x)

      y := xconfig.GetString("MyConfig.Y")
      println("get config y: ", y)

      myConfig := &MyConfig{}
      _ = xconfig.UnmarshalConfig("MyConfig", myConfig)
      println("get config myConfig: ", myConfig)
  }
  ```

### 6. 其它模块配置参数说明

* 其它块配置参数，参考相应模块的README.md
