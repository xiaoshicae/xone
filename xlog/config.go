package xlog

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoshicae/xone/v3/xutil"
)

const (
	XLogConfigKey = "XLog"

	// 各配置项的默认值
	defaultLevel      = "info"
	defaultTimezone   = "Asia/Shanghai"
	defaultFileName   = "app"
	defaultFilePath   = "./log"
	defaultMaxAge     = "7d"
	defaultRotateTime = "1d"
	defaultFilePerm   = "0644"
)

// minRotateTime 轮转周期下限
//
// 日志文件名的时间后缀最细到分钟，小于一分钟的周期无法体现在文件名上，
// 配置成这类值只会退化为按分钟切割。无单位的数字会被解析为纳秒
// （如 "5" 即 5ns），因此这类笔误必须一并拦下。
const minRotateTime = time.Minute

// 控制台输出格式
const (
	// FormatText 带颜色的可读格式：level+time+filename+traceid+内容
	FormatText = "text"

	// FormatJSON 结构化 JSON 格式，与写入文件的内容一致
	FormatJSON = "json"
)

// Config 日志配置
//
// Level 与 Timezone 为两路输出共用；控制台与文件各自的配置分别位于
// Console 与 File 子块，两者可独立开关。
type Config struct {
	// Level 日志级别，两路输出共用
	// optional default "info"，可选 debug/info/warn/error/fatal，大小写不敏感
	Level string `mapstructure:"Level"`

	// Timezone 日志时间的时区，两路输出共用
	// optional default "Asia/Shanghai"
	Timezone string `mapstructure:"Timezone"`

	// Console 控制台(标准输出)配置
	// optional
	Console ConsoleConfig `mapstructure:"Console"`

	// File 日志文件配置
	// optional
	File FileConfig `mapstructure:"File"`
}

// ConsoleConfig 控制台输出配置
type ConsoleConfig struct {
	// Enable 是否输出到控制台
	// 与 File.Enable 同时为 false 时会被强制置为 true，避免日志无处输出
	// optional default true
	Enable *bool `mapstructure:"Enable"`

	// Format 输出格式，text 为带颜色的可读格式，json 为结构化格式
	// 容器环境建议使用 json，便于采集器解析
	// optional default "text"
	Format string `mapstructure:"Format"`
}

// FileConfig 日志文件配置
type FileConfig struct {
	// Enable 是否将日志写入文件
	// 默认关闭，日志仅打印到标准输出，适配 K8s 等由采集器收集容器标准输出的环境
	// optional default false
	Enable bool `mapstructure:"Enable"`

	// Path 日志文件夹路径
	// optional default "./log"
	Path string `mapstructure:"Path"`

	// Name 日志文件名称，实际文件形如 {Name}.log.20260912
	// optional default "app"
	Name string `mapstructure:"Name"`

	// MaxAge 日志保存最大时长，超过该时长的历史文件会在轮转时清理
	// 设为 0 表示不清理
	// optional default "7d"
	MaxAge string `mapstructure:"MaxAge"`

	// RotateTime 日志切割周期，支持小于一天的周期如 "6h"
	// optional default "1d"
	RotateTime string `mapstructure:"RotateTime"`

	// Perm 日志文件权限，八进制字符串如 "0644"
	// 日志可能含敏感信息，需要限制同机其他用户读取时配 "0600"
	// optional default "0644"
	Perm string `mapstructure:"Perm"`
}

// FileMode 解析日志文件权限，无法解析时回退到默认值
func (c FileConfig) FileMode() os.FileMode {
	v, err := strconv.ParseUint(strings.TrimSpace(c.Perm), 8, 32)
	if err != nil || v == 0 {
		if c.Perm != "" && c.Perm != defaultFilePerm {
			xutil.WarnIfEnableDebug("XOne xlog invalid File.Perm [%s], fallback to [%s]", c.Perm, defaultFilePerm)
		}
		return defaultLogFilePerm
	}
	return os.FileMode(v)
}

// IsJSON 控制台是否输出 JSON 格式
func (c ConsoleConfig) IsJSON() bool {
	return strings.EqualFold(c.Format, FormatJSON)
}

// configMergeDefault 合并默认配置，入参为 nil 时返回全默认配置
// 该函数幂等，重复调用结果一致
func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}

	if c.Level == "" {
		c.Level = defaultLevel
	}
	if c.Timezone == "" {
		c.Timezone = defaultTimezone
	}

	if c.Console.Enable == nil {
		c.Console.Enable = xutil.ToPtr(true)
	}
	// 两路输出都关闭时日志无处可去，强制打开控制台输出
	if !c.File.Enable && !*c.Console.Enable {
		c.Console.Enable = xutil.ToPtr(true)
	}
	// 无法识别的格式按默认的可读格式处理，不因笔误让控制台变成 JSON 或反之
	if !strings.EqualFold(c.Console.Format, FormatJSON) {
		if c.Console.Format != "" && !strings.EqualFold(c.Console.Format, FormatText) {
			xutil.WarnIfEnableDebug("XOne xlog unknown Console.Format [%s], fallback to [%s]", c.Console.Format, FormatText)
		}
		c.Console.Format = FormatText
	} else {
		c.Console.Format = FormatJSON
	}

	if c.File.Name == "" {
		c.File.Name = defaultFileName
	}
	if c.File.Path == "" {
		c.File.Path = defaultFilePath
	}
	if c.File.MaxAge == "" {
		c.File.MaxAge = defaultMaxAge
	}
	if c.File.RotateTime == "" {
		c.File.RotateTime = defaultRotateTime
	}
	// 无法解析、非正数或过小的轮转周期都会退化为按分钟切割（一天上千个文件），
	// 这类笔误不应静默生效，回退到默认值
	if xutil.ToDuration(c.File.RotateTime) < minRotateTime {
		xutil.WarnIfEnableDebug("XOne xlog invalid File.RotateTime [%s] (min %v), fallback to [%s]", c.File.RotateTime, minRotateTime, defaultRotateTime)
		c.File.RotateTime = defaultRotateTime
	}
	if c.File.Perm == "" {
		c.File.Perm = defaultFilePerm
	}
	// 负数保留时长会让所有历史文件立即过期，同样视为配置错误
	if xutil.ToDuration(c.File.MaxAge) < 0 {
		xutil.WarnIfEnableDebug("XOne xlog invalid File.MaxAge [%s], fallback to [%s]", c.File.MaxAge, defaultMaxAge)
		c.File.MaxAge = defaultMaxAge
	}

	return c
}
