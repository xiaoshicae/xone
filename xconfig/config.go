// Package xconfig 提供 Spring 风格的配置加载能力。
//
// 加载过程是一条直线，每一步只做一件事：
//
//	定位 → 解析 → 分层 → 合并 → 展开 → 存储
//
//	定位  找到主配置文件（启动参数 > 环境变量 > 约定路径）
//	解析  把每个文件读成 map[string]any
//	分层  按优先级从低到高排出所有配置层（主配置、导入、环境变体）
//	合并  逐层深合并成一棵树
//	展开  统一展开一次 ${VAR} 占位符
//	存储  交给 viper 作为只读存储，对外提供读取 API
//
// 中间各步都是 map[string]any 上的纯函数，viper 只出现在最后一步。
package xconfig

const (
	// ServerConfigKey 框架级服务配置在配置文件中的根 key
	ServerConfigKey = "Server"

	// importDisplayKey 导入列表的完整 key，仅用于日志与报错，展示给使用者看
	importDisplayKey = ServerConfigKey + ".Config.Import"

	defaultServerName    = "unknown.unknown.unknown"
	defaultServerVersion = "v0.0.1"
)

// 嵌套 map 中的 key 路径，解析时已统一转成小写。
//
// 用「路径段列表」而不是 "Server.Config.Import" 这样的点分字符串：
// 配置 key 本身允许含 "."（如 OpenTelemetry 的 service.name、Prometheus 的多级标签），
// 按 "." 切分会把这样一个 key 误判成两层嵌套结构。
var (
	profilesPath       = []string{"server", "profiles"}
	profilesActivePath = []string{"server", "profiles", "active"}
	importPath         = []string{"server", "config", "import"}
)

// Server 框架级服务配置，对应配置文件中的 Server 块
type Server struct {
	// Name 服务名
	// required
	Name string `mapstructure:"Name"`

	// Version 服务版本号
	// optional default "v0.0.1"
	Version string `mapstructure:"Version"`

	// Profiles 环境相关配置
	// optional default nil
	Profiles *Profiles `mapstructure:"Profiles"`

	// Config 配置文件来源
	// optional default nil
	Config *ConfigSource `mapstructure:"Config"`
}

// Profiles 环境配置
type Profiles struct {
	// Active 指定启用的环境，只允许字母、数字、下划线和短横线
	// required
	Active string `mapstructure:"Active"`
}

// ConfigSource 配置文件来源，对应配置文件中的 Server.Config 块
type ConfigSource struct {
	// Import 额外加载的配置文件，路径相对主配置文件所在目录，也支持绝对路径
	// optional default nil
	Import []string `mapstructure:"Import"`
}

// serverMergeDefault 集中设置 Server 块的默认值
func serverMergeDefault(s Server) Server {
	if s.Name == "" {
		s.Name = defaultServerName
	}
	if s.Version == "" {
		s.Version = defaultServerVersion
	}
	return s
}
