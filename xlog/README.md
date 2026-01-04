## XLog模块

### 1. 模块简介
* XLog提供的方法请参考util.go

### 2. 配置参数
```yaml
# 按如下配置，日志会保存到 /a/b/c/xxx.log
# 如果没有任何配置，日志会默认保存到 ./log/app.log
XLog:
  Level: "debug"            # 日志级别(optional default "info")，支持debug/info/warn/error四个级别，大小写不敏感
  Name: "xxx"               # 日志文件名称(optional default "app")
  Path: "/a/b/c"            # 日志文件夹路径(optional default "./log/")
  Console: true             # 日志内容是否需要在控制台打印(optional default false)
  ConsoleFormatIsRaw: true  # 在控制台打印的日志是否为原始格式(即底层的json格式)，为false时，打印level+time+filename+func+traceid+内容(optional default false)
  MaxAge: "10d"             # 日志保存最大天数(optional default "7d")
  RotateTime: "2d"          # 日志切割天数(optional default "1d")，即完整日志名为: ${Name}.log.%Y%m%d
```

### 3. 使用demo
```go
package main

import (
    "context"

    "github.com/xiaoshicae/xone/xlog"
)

func main() {
    // 支持string参数
    xlog.Info(context.Background(), "some info")

    // 支持string formt方式参数
    xlog.Info(context.Background(), "some info %s", "hahaha")
    
    // 支持string formt + 自定义KV方式参数
    xlog.Info(context.Background(), "some info %s", "hahaha", xlog.KV("k1", "v1"), xlog.KV("k2", 2))
    
    // 支持KVMap方式参数
    myKV := map[string]interface{}{"k3": "v3", "k4": "v4"}
    xlog.Info(context.Background(), "some info", xlog.KV("k1", "v1"), xlog.KVMap(myKV))
}
```

### 4. XLog底层记录的json日志字段说明
```json
{
  "msg": "some info",                 // 日志内容
  "time": "2024-10-15 19:45:05.136",  // 日志时间，格式 yyyy-MM-dd HH:mm:ss.SSS
  "level": "debug",                   // 日志级别
  "filename": "util.go",              // 文件名
  "lineid": "44",                     // 行号
  "ip": "10.10.10.10",                // ip
  "pid": "123",                       // 进程id
  "servername": "my-app",             // 服务名
  "traceid": "xxxxx",                 // traceid
  "spanid": "xxxxx",                  // spanid
  "k1": "v1",                         // 自定义的key
  "k2": "2"                           // 自定义的key
}
```