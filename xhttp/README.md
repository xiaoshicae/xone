## XHttp模块

### 1. 模块简介

* 对go-resty(https://github.com/go-resty/resty)进行了封装，版本参见go.mod文件。

### 2. 配置参数

```yaml
XHttp:
  Timeout: "3s" # http请求超时时间(optional default "60s")
```

### 3. 使用demo

* 配置:

```yaml
XHttp:
    Timeout: "10s"
```

* 获取client并使用，详细请参考[go-resty](https://github.com/go-resty/resty):

```go
package main

import (
  "fmt"
  "github.com/xiaoshicae/xone/xhttp"
)
  
func main() {
  // 通过C()获取client，不推荐该方法，推荐使用 xhttp.RWithCtx(ctx) 保证traceId传递到下游
  client := xhttp.C()
  resp, err := client.R().Get("https://httpbin.org/get")
  
  // 处理response
  fmt.Println("Response Info:")
  fmt.Println("  Error      :", err)
  fmt.Println("  Status Code:", resp.StatusCode())
  fmt.Println("  Status     :", resp.Status())
  fmt.Println("  Proto      :", resp.Proto())
  fmt.Println("  Time       :", resp.Time())
  fmt.Println("  Received At:", resp.ReceivedAt())
  fmt.Println("  Body       :\n", resp)
 
  // 推荐使用该方法保证traceId传递到下游
  resp, err := xhttp.RWithCtx(ctx).Get("https://httpbin.org/get")
}
```
