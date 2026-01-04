## XTrace模块

### 1. 模块简介

* 对opentelemetry-go(https://github.com/open-telemetry/opentelemetry-go)进行了封装，版本参见go.mod文件。
* 当前只是本地记录trace，暂不支持上报到远程服务端。

### 2. 配置参数

```yaml
XTrace:
  Enable: false   # Trace是否开启(optional default true)
  Console: false  # 是否要在控制台打印trace内容(optional default false)
```

### 3. 使用demo

> 通过XOne运行的应用会自动初始化trace，无需手动调用
