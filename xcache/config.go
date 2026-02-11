package xcache

const XCacheConfigKey = "XCache"

const (
	defaultNumCounters = 1000000
	defaultMaxCost     = 100000
	defaultBufferItems = 64
	defaultTTL         = "5m"
)

type Config struct {
	// NumCounters 用于跟踪频率的键数量，建议设置为期望缓存条目数量的 10 倍
	// optional default 1000000
	NumCounters int64 `mapstructure:"NumCounters"`

	// MaxCost 缓存的最大成本（当每个条目 cost=1 时，等价于最大缓存条目数）
	// optional default 100000
	MaxCost int64 `mapstructure:"MaxCost"`

	// BufferItems Get 操作的内部缓冲区大小
	// optional default 64
	BufferItems int64 `mapstructure:"BufferItems"`

	// DefaultTTL 默认的缓存过期时间
	// optional default "5m"
	DefaultTTL string `mapstructure:"DefaultTTL"`

	// Name 用于区分多 cache 配置时的唯一身份
	// optional default ""
	Name string `mapstructure:"Name"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.NumCounters <= 0 {
		c.NumCounters = defaultNumCounters
	}
	if c.MaxCost <= 0 {
		c.MaxCost = defaultMaxCost
	}
	if c.BufferItems <= 0 {
		c.BufferItems = defaultBufferItems
	}
	if c.DefaultTTL == "" {
		c.DefaultTTL = defaultTTL
	}
	return c
}
