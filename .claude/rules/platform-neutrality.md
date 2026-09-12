# 平台中立规范

## 核心原则

XOne 是通用 Go SDK，**不绑定任何云厂商、部署平台或特定公司的内部环境**。

使用者可能运行在 K8s、虚拟机、物理机或本地开发机上，日志可能被任意采集器收集，
也可能不被收集。框架的行为、默认值、配置和文档都**不得假设某一种部署环境**。

## 禁止事项

### 1. 代码与配置不得依赖特定平台

- **禁止**引入云厂商 SDK 作为框架依赖（AWS SDK、阿里云 SDK、腾讯云 SDK 等）
- **禁止**默认值、初始化逻辑依赖特定平台的环境变量、元数据服务或目录约定
- **禁止**配置项以平台命名（如 `CloudWatchEnable`、`SLSEndpoint`）

平台相关能力应由使用者在业务侧实现，框架只提供通用扩展点。

### 2. 文档与注释不得把某平台当作既定环境

```yaml
# ❌ 断言使用者在特定云上
EnableConsole: true    # 打印到屏幕，aws cloud watch 会自动收集日志

# ✅ 描述框架行为本身，采集方式交给部署环境
EnableConsole: true    # 打印到标准输出，由部署环境的日志采集组件收集
```

判断标准：把句子里的平台名换成另一家厂商，如果语义变得不成立，说明这句话在做平台假设。

### 3. 测试夹具不得混入真实业务配置

`test/` 下的配置和测试代码是仓库的公开内容，**禁止**出现：

- 公司名、产品名、内部服务名（如 `Server.Name: "xxx.llmtrainer.gateway"`）
- 内部域名、内网地址、内部 API 路径（如 `/api/internal/users`）
- 真实或形似真实的凭证（API Key、Token、密码），即使标注了 `debug-` 前缀
- 与 SDK 能力无关的业务配置块（计费、代理、对象存储等）

测试夹具统一使用中立占位符：

| 用途 | 占位符 |
|------|--------|
| 业务配置根 key | `MyApp` |
| 服务名 | `xone.demo.app` |
| 域名 | `example.com` / `httpbin.org` |
| 地址 | `127.0.0.1` / `localhost` |
| 凭证 | 环境变量占位符，如 `${MYAPP_API_KEY:-}`，不写字面值 |

### 4. 不得在日志或测试输出中打印凭证

```go
// ❌ 直接打印密钥内容
t.Logf("APIKey: %v", xconfig.GetString("MyApp.Backend.APIKey"))

// ✅ 只确认能读到，不暴露内容
t.Logf("APIKey is set: %v", xconfig.GetString("MyApp.Backend.APIKey") != "")
```

框架内部日志同理，敏感字段必须脱敏后再输出（参考 `xgorm` 的 DSN 脱敏、`xredis` 的密码脱敏）。

## 允许事项

以下**不算**平台绑定，无需规避：

- **中立地举例生态工具**：`由采集器（Filebeat / Fluent Bit / Loki 等）收集` —— 并列举例且不假设使用者在用哪一个
- **通用技术名词**：容器、K8s、标准输出、IANA 时区等，作为通行概念使用
- **第三方库依赖**：logrus、gorm、resty 等通用库是框架的实现选择，与部署平台无关
- **通用协议与格式**：JSON 日志、OpenTelemetry、Prometheus 指标等跨平台标准

区别在于：**举例说明**可以，**假设成立**不行。

## 提交前自查

新增或修改代码、文档、测试夹具后执行：

```bash
# 1. 云厂商与平台专有名词
# 用 [^a-z] 而非 \b 作边界：\b 不匹配下划线会漏掉 AWS_REGION，
# 裸 aws 又会误伤 GetRawServerName 中的 "awS"
grep -rniE "(^|[^a-z])(aws|cloudwatch|amazon|aliyun|tencent|huawei|azure|gcp|firebase)([^a-z]|\$)|阿里云|腾讯云|华为云" \
  --include=*.go --include=*.md --include=*.yml --include=*.yaml --include=*.json .

# 2. 凭证字面值（排除环境变量占位符写法）
grep -rniE "(apikey|api_key|token|secret|password):" --include=*.yml --include=*.yaml . \
  | grep -v '\${'

# 3. 硬编码外部地址（排除公开示例地址与依赖库官网）
grep -rnoiE "https?://[a-z0-9.:_/-]+" --include=*.go --include=*.md --include=*.yml . \
  | grep -viE "localhost|127\.0\.0\.1|example\.com|httpbin\.org|github\.com|pkg\.go\.dev|gorm\.io|golang\.org"
```

命中结果需逐条确认属于"允许事项"，否则必须改为中立写法。
以上命令在当前仓库跑出的结果应为空（命令 3 仅剩文档中的工具官网链接）。
