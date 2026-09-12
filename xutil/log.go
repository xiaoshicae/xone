package xutil

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 日志相关的方法，注意这里的日志主要用来做XOne debug用，因此只会打印在屏幕上

// debugLevel XOne 框架调试日志的级别
type debugLevel struct {
	name  string
	color int
}

var (
	debugLevelError = debugLevel{name: "ERROR", color: 31}
	debugLevelWarn  = debugLevel{name: "WARN", color: 33}
	debugLevelInfo  = debugLevel{name: "INFO", color: 36}
)

// ErrorIfEnableDebug 当开启 debug 模式时输出 Error 级别日志
func ErrorIfEnableDebug(msg string, args ...any) {
	logIfEnableDebug(debugLevelError, msg, args...)
}

// InfoIfEnableDebug 当开启 debug 模式时输出 Info 级别日志
func InfoIfEnableDebug(msg string, args ...any) {
	logIfEnableDebug(debugLevelInfo, msg, args...)
}

// WarnIfEnableDebug 当开启 debug 模式时输出 Warn 级别日志
func WarnIfEnableDebug(msg string, args ...any) {
	logIfEnableDebug(debugLevelWarn, msg, args...)
}

// xoneDebugPrefix XOne 框架调试日志的醒目前缀（紫色高亮）
const xoneDebugPrefix = "\x1b[35m[XOne-Debug]\x1b[0m "

// debugTimeLayout 调试日志的时间格式
const debugTimeLayout = "2006-01-02 15:04:05.999"

// debugOut 调试日志输出目标，便于测试替换
var debugOut = os.Stdout

// debugCallerResolver 调试日志的调用方解析器，忽略规则固定故可复用缓存
var debugCallerResolver = NewCallerResolver([]string{currentFilePath})

// logIfEnableDebug 当开启 debug 模式时按指定级别输出日志
//
// 框架调试日志量极小且只输出到屏幕，无需引入日志库，直接格式化写出即可
func logIfEnableDebug(level debugLevel, msg string, args ...any) {
	if !EnableXOneDebug() {
		return
	}

	caller := debugCallerResolver.Caller(0)
	location := unknownCaller
	if caller != nil {
		location = fmt.Sprintf("%s:%d", path.Base(caller.File), caller.Line)
	}

	line := fmt.Sprintf("\x1b[%dm%s\x1b[0m[%s] \x1b[34m%s\x1b[0m %s%s\n",
		level.color, level.name, time.Now().Format(debugTimeLayout),
		location, xoneDebugPrefix, fmt.Sprintf(msg, args...))
	_, _ = debugOut.WriteString(line)
}

// CallerResolver 日志调用方解析器
//
// 解析结果按程序计数器（PC）缓存：调用点数量有限且每个 PC 对应的源码位置固定，
// 缓存后可跳过 runtime.CallersFrames 的符号解析——那是栈回溯中最昂贵的部分。
//
// 忽略规则在构造时固定，因此缓存无需区分规则集合。
type CallerResolver struct {
	suffixToIgnore []string

	// cache 映射 uintptr -> callerCacheEntry
	// 读多写少（进程稳定后不再新增调用点），用 sync.Map 避免读路径加锁
	cache sync.Map
}

// callerCacheEntry 单个 PC 的解析结果
// 一个 PC 经内联可能展开为多个栈帧，此处记录该 PC 的最终结论
type callerCacheEntry struct {
	file string
	line int
	// ignored 为 true 表示该 PC 展开出的所有帧都命中忽略规则，
	// 此时 file/line 保留最后一帧，供全部被忽略时兜底
	ignored bool
}

// NewCallerResolver 创建调用方解析器，suffixToIgnore 为需跳过的文件路径后缀
func NewCallerResolver(suffixToIgnore []string) *CallerResolver {
	return &CallerResolver{suffixToIgnore: suffixToIgnore}
}

// Caller 获取日志调用方的栈帧，callDepth 为额外跳过的帧数
// 全部栈帧都被忽略时返回最后一帧兜底，避免调用方拿到 nil
func (r *CallerResolver) Caller(callDepth int) *runtime.Frame {
	var pcs [maximumCallerDepth]uintptr
	depth := runtime.Callers(minimumCallerDepth+callDepth, pcs[:])
	if depth == 0 {
		return nil
	}

	var last *callerCacheEntry
	for i := 0; i < depth; i++ {
		entry := r.entryFor(pcs[i])
		if !entry.ignored {
			return &runtime.Frame{File: entry.file, Line: entry.line}
		}
		last = entry
	}

	if last == nil {
		return nil
	}
	return &runtime.Frame{File: last.file, Line: last.line}
}

// entryFor 取出单个 PC 的解析结果，未命中缓存时解析并写入
func (r *CallerResolver) entryFor(pc uintptr) *callerCacheEntry {
	if v, ok := r.cache.Load(pc); ok {
		return v.(*callerCacheEntry)
	}

	entry := resolvePC(pc, r.suffixToIgnore)
	r.cache.Store(pc, entry)
	return entry
}

// resolvePC 解析单个 PC
// 一个 PC 可能因内联展开为多个栈帧，取其中第一个未被忽略的帧
func resolvePC(pc uintptr, suffixToIgnore []string) *callerCacheEntry {
	frames := runtime.CallersFrames([]uintptr{pc})
	entry := &callerCacheEntry{ignored: true}
	for {
		f, hasMore := frames.Next()
		if !shouldIgnoreCallerFile(f.File, suffixToIgnore) {
			return &callerCacheEntry{file: f.File, line: f.Line}
		}
		entry.file, entry.line = f.File, f.Line
		if !hasMore {
			return entry
		}
	}
}

// GetLogCaller 获取日志调用方的栈帧，跳过 suffixToIgnore 和内置忽略列表中匹配的文件
//
// 不带缓存的一次性版本；固定忽略规则且高频调用的场景应使用 CallerResolver
func GetLogCaller(callDepth int, suffixToIgnore []string) *runtime.Frame {
	var pcs [maximumCallerDepth]uintptr
	depth := runtime.Callers(minimumCallerDepth+callDepth, pcs[:])
	if depth == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs[:depth])
	for {
		f, hasMore := frames.Next()
		if !shouldIgnoreCallerFile(f.File, suffixToIgnore) {
			return &f
		}
		if !hasMore {
			// 所有栈帧都被忽略，返回最后一帧兜底，避免调用方拿到 nil
			return &f
		}
	}
}

// shouldIgnoreCallerFile 判断栈帧文件是否应被跳过
func shouldIgnoreCallerFile(file string, suffixToIgnore []string) bool {
	for _, s := range suffixToIgnore {
		if strings.HasSuffix(file, s) {
			return true
		}
	}
	base := path.Base(file)
	for i := range callerIgnoreRules {
		r := &callerIgnoreRules[i]
		if r.pkg != "" && !strings.Contains(file, r.pkg) {
			continue
		}
		for _, prefix := range r.filePrefixes {
			if strings.HasPrefix(base, prefix) {
				return true
			}
		}
	}
	return false
}

const (
	currentFilePath        = "/xutil/log.go"
	unknownCaller          = "???"
	maximumCallerDepth int = 25

	// minimumCallerDepth 起始跳过的帧数：runtime.Callers 自身与 GetLogCaller
	// 只跳过这两帧、其余交给忽略规则处理，避免写死调用链深度——
	// 该值曾按日志库的栈深度硬编码，调用链或内联一变就会跳过业务帧
	minimumCallerDepth int = 2
)

// callerIgnoreRule 调用栈忽略规则
// pkg 为空表示不限定路径，仅按文件名前缀匹配
type callerIgnoreRule struct {
	pkg          string   // 栈帧文件路径中需包含的库标识
	filePrefixes []string // 文件名（basename）前缀，命中任一即忽略
}

// callerIgnoreRules 需要从调用栈中过滤的第三方库文件
// 原为正则列表，因位于日志热路径改为字符串匹配：栈深 15 层时正则需回溯近百次
var callerIgnoreRules = []callerIgnoreRule{
	{pkg: "go-redis/", filePrefixes: []string{"string_commands.go", "redis.go"}},
	{pkg: "xmysql", filePrefixes: []string{"logger.go"}},
	{pkg: "xredis", filePrefixes: []string{"logger.go"}},
	{pkg: "gorm", filePrefixes: []string{"callbacks.go", "finisher_api.go"}},
	{pkg: "mongo-driver", filePrefixes: []string{"operation", "database", "client", "collection", "cursor"}},
	{pkg: "", filePrefixes: []string{"asm_"}},
}
