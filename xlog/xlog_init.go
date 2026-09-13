package xlog

import (
	"io"
	"log/slog"
	"os"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const (
	// defaultLocalIP 获取本机 IP 失败时的兜底值
	defaultLocalIP = "0.0.0.0"
)

// findFrameIgnoreFileNames 定位调用方时需跳过的本模块文件
var findFrameIgnoreFileNames = []string{
	"/xlog/util.go",
	"/xlog/handler.go",
}

// defaultCallerResolver 调用方解析器
// 置于包级而非 handler 内，使 PC 缓存在重复初始化后依然保留
var defaultCallerResolver = xutil.NewCallerResolver(findFrameIgnoreFileNames)

var (
	// handler 当前生效的日志处理器，热路径以原子读取获取
	handler atomic.Pointer[xHandler]

	// currentLevel 初始化后生效的日志级别，供运行时查询
	// 配置值在初始化时即已确定，运行时不再回查 xconfig
	currentLevel atomic.Uint32

	// fileWriter 当前生效的日志文件写入器，重复初始化时替换并关闭旧实例
	fileWriter   *asyncWriter
	fileWriterMu sync.Mutex
)

// localIP 本机 IP，涉及网卡枚举，进程内只计算一次
var localIP = sync.OnceValue(func() string {
	ip, _ := xutil.GetLocalIP()
	return xutil.GetOrDefault(ip, defaultLocalIP)
})

func init() {
	// 初始化前即可使用日志，避免 BeforeStart 执行前的日志丢失
	handler.Store(newHandler(configMergeDefault(nil), time.Local, os.Stdout, nil, slog.LevelInfo))
	currentLevel.Store(uint32(InfoLevel))

	xhook.BeforeStart(initXLog)
	// 与 BeforeStart 一并注册，使关闭顺序由 import 顺序决定：
	// 日志模块在 xconfig 之后最早注册，因而最后关闭，其他模块关闭时打的日志仍能落盘。
	// 未启用文件日志时 closeFileWriter 直接返回 nil，无条件注册是安全的。
	xhook.BeforeStop(closeFileWriter)
}

func initXLog() error {
	c, err := getConfig()
	if err != nil {
		return xerror.Newf("xlog", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initXLog got config: %s", xutil.ToJsonString(c))

	return initXLogByConfig(c)
}

func initXLogByConfig(c *Config) error {
	// 兜底：configMergeDefault 幂等，这里再合并一次，避免直接传入 nil 或缺省字段的 Config 导致空指针
	c = configMergeDefault(c)

	// 加载时区
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		xutil.WarnIfEnableDebug("XOne initXLogByConfig load timezone [%s] failed, using Local timezone, err=[%v]", c.Timezone, err)
		loc = time.Local
	}

	// 按需创建文件写入器，File.Enable 为 false 时不触碰文件系统
	// 此处只创建不替换：需等新 handler 生效后再关闭旧写入器
	var aw *asyncWriter
	if c.File.Enable {
		var err error
		if aw, err = newFileWriter(c); err != nil {
			return err
		}
	}

	var cw io.Writer
	if *c.Console.Enable {
		cw = os.Stdout
	}

	level, ok := ParseLevel(c.Level)
	if !ok {
		xutil.WarnIfEnableDebug("XOne initXLogByConfig unknown level [%s], fallback to info", c.Level)
	}

	// 整体替换 handler，保证重复初始化不会叠加输出
	handler.Store(newHandler(c, loc, cw, fileWriterOf(aw), level.toSlog()))
	currentLevel.Store(uint32(level))

	// 新 handler 已生效，此时再关闭旧写入器
	// 反过来会让切换窗口内的日志写入已关闭的写入器而丢失
	swapFileWriter(aw)

	return nil
}

// newHandler 构造日志处理器，consoleWriter / fileWriter 为 nil 表示对应输出关闭
func newHandler(c *Config, loc *time.Location, consoleWriter, fileWriter io.Writer, level slog.Level) *xHandler {
	return &xHandler{
		serverName:     xconfig.GetServerName(),
		ip:             localIP(),
		pidStr:         strconv.Itoa(os.Getpid()), // 初始化时转换，避免每条日志重复转换
		callerResolver: defaultCallerResolver,
		location:       loc,
		consoleWriter:  newLockedWriter(consoleWriter),
		consoleJSON:    c.Console.IsJSON(),
		fileWriter:     fileWriter,
		level:          level,
	}
}

// fileWriterOf 将可能为 nil 的 *asyncWriter 转成 io.Writer
// 直接赋值会得到「非 nil 接口包裹 nil 指针」，使 handler 误判文件输出已开启
func fileWriterOf(aw *asyncWriter) io.Writer {
	if aw == nil {
		return nil
	}
	return aw
}

// newFileWriter 创建轮转日志文件并包装为异步写入器
func newFileWriter(c *Config) (*asyncWriter, error) {
	if !xutil.DirExist(c.File.Path) { // 日志所在文件夹不存在则创建
		if err := os.MkdirAll(c.File.Path, os.ModePerm); err != nil {
			return nil, xerror.Newf("xlog", "init", "os.MkdirAll failed, path=[%s], err=[%v]", c.File.Path, err)
		}
	}

	logFilePath := path.Join(c.File.Path, c.File.Name+".log")
	w, err := newRotateWriter(logFilePath, xutil.ToDuration(c.File.MaxAge), xutil.ToDuration(c.File.RotateTime))
	if err != nil {
		return nil, err
	}

	// 异步写入，避免日志 I/O 阻塞调用方
	return newAsyncWriter(w, defaultAsyncBufferSize), nil
}

// swapFileWriter 替换当前文件写入器并关闭旧实例，避免重复初始化泄漏 goroutine
func swapFileWriter(aw *asyncWriter) {
	fileWriterMu.Lock()
	old := fileWriter
	fileWriter = aw
	fileWriterMu.Unlock()

	if old != nil {
		if err := old.Close(); err != nil {
			xutil.WarnIfEnableDebug("XOne swapFileWriter close previous writer failed, err=[%v]", err)
		}
	}

}

// closeFileWriter 关闭文件写入器，等待缓冲区写完
func closeFileWriter() error {
	fileWriterMu.Lock()
	aw := fileWriter
	fileWriter = nil
	fileWriterMu.Unlock()

	if aw == nil {
		return nil
	}
	return aw.Close()
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XLogConfigKey, c); err != nil {
		return nil, err
	}
	return configMergeDefault(c), nil
}
