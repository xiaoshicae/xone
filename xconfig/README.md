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
    * application{-profiles}.yml 合并到 application.yml 覆盖原则
        * Server配置按二级key覆盖
        * 其它按一级key覆盖

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

### 2. 配置参数

  ```yaml
  Server:
    Name: "a.b.c"      # 服务名(required)，log/trace都会需要这个配置
    Version: "v2.0.0"  # 服务版本号(optional default "v0.0.1")，trace上报，swagger版本显示等需要使用该配置
    
    Profiles:          # 环境相关配置(optional default nil)
      Active: "dev"      # 指定启用的环境(required)
    
    Gin:               # gin相关配置(optional default nil)
      Host: "0.0.0.0"    # 服务监听host(optional default "0.0.0.0")
      Port: 9000         # 服务端口号(optional default 8000)
      UseHttp2: false    # 是否使用http2协议(optional default false)
      
      # 具体参数含义请参考: https://github.com/swaggo/swag/blob/master/README_zh-CN.md#%E9%80%9A%E7%94%A8api%E4%BF%A1%E6%81%AF
      GinSwagger:        # gin-swagger管理后台相关配置(optional default "")
        Host: "https://xxx.xxx"       # 提供api服务的host(optional default "")
        BasePath: "/api/v1"           # api公共前缀(optional default "")
        Title: "API接口文档"           # api管理后台的title(optional default "")
        Description: "xxx"            # api管理后台的描述信息(optional default "")
        Schemes:                      # 支持的协议(optional default ["https", "http"])
          - "https"
          - "http"
  ```

### 3. 使用demo

* 配置
  ```yaml
  MyConfig:
    X: 1
    Y: "zz"
  ```
* 读取
  ```go
  package main
  
  import "github.com/xiaoshicae/xone/xconfig"
  
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

### 4. 其它模块配置参数说明

* 其它块配置参数，参考相应模块的README.md
