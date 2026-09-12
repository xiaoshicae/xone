package xlog

import (
	"io"
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

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
)

const (
	// defaultLocalIP 获取本机 IP 失败时的兜底值
	defaultLocalIP = "0.0.0.0"

	// closeHookOrder 日志关闭钩子的 Order
	// 取较大值确保日志系统在其他模块关闭之后再关闭，避免关闭阶段的日志丢失
	closeHookOrder = 9999
)

// findFrameIgnoreFileNames 定位调用方时需跳过的本模块文件
var findFrameIgnoreFileNames = []string{
	"/xlog/util.go",
	"/xlog/xlog_hook.go",
}

var (
	// logger xlog 私有的 logrus 实例
	// 不使用 logrus.StandardLogger()，避免本模块的 formatter/hook 影响
	// 使用者及其第三方依赖对全局 logrus 的使用
	logger *logrus.Logger

	// currentLevel 初始化后生效的日志级别，供运行时查询
	// 配置值在初始化时即已确定，运行时不再回查 xconfig
	currentLevel atomic.Uint32

	// fileWriter 当前生效的日志文件写入器，重复初始化时替换并关闭旧实例
	fileWriter   *asyncWriter
	fileWriterMu sync.Mutex

	// stopHookOnce 保证关闭钩子只注册一次
	stopHookOnce sync.Once
)

// localIP 本机 IP，涉及网卡枚举，进程内只计算一次
var localIP = sync.OnceValue(func() string {
	ip, _ := xutil.GetLocalIP()
	return xutil.GetOrDefault(ip, defaultLocalIP)
})

func init() {
	// 初始化前即可使用日志，避免 BeforeStart 执行前的日志丢失
	logger = logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetFormatter(nopFormatter{})
	logger.SetLevel(logrus.InfoLevel)
	logger.AddHook(newHook(configMergeDefault(nil), time.Local, os.Stdout, nil))
	currentLevel.Store(uint32(InfoLevel))

	xhook.BeforeStart(initXLog)
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

	// 按需创建文件写入器，EnableFile 为 false 时不触碰文件系统
	var fw io.Writer
	if c.EnableFile {
		aw, err := newFileWriter(c)
		if err != nil {
			return err
		}
		setFileWriter(aw)
		fw = aw
	} else {
		setFileWriter(nil)
	}

	var cw io.Writer
	if *c.EnableConsole {
		cw = os.Stdout
	}

	level, ok := ParseLevel(c.Level)
	if !ok {
		xutil.WarnIfEnableDebug("XOne initXLogByConfig unknown level [%s], fallback to info", c.Level)
	}

	// logrus 每条日志都会调用一次 Formatter 并写入 Out，
	// 这里置为空实现 + io.Discard，实际序列化与输出全部由 hook 按需完成
	logger.SetOutput(io.Discard)
	logger.SetFormatter(nopFormatter{})
	// ReplaceHooks 而非 AddHook，保证重复初始化不会叠加 hook 导致日志重复输出
	logger.ReplaceHooks(logrus.LevelHooks{})
	logger.AddHook(newHook(c, loc, cw, fw))
	logger.SetLevel(level.toLogrus())
	currentLevel.Store(uint32(level))

	return nil
}

// newHook 构造日志 hook，consoleWriter / fileWriter 为 nil 表示对应输出关闭
func newHook(c *Config, loc *time.Location, consoleWriter, fileWriter io.Writer) *xLogHook {
	return &xLogHook{
		ServerName:     xconfig.GetServerName(),
		IP:             localIP(),
		PidStr:         strconv.Itoa(os.Getpid()), // 初始化时转换，避免每条日志重复转换
		SuffixToIgnore: findFrameIgnoreFileNames,
		jsonFormatter: &logrus.JSONFormatter{
			TimestampFormat: consoleTimeLayout,
		},
		location:      loc,
		consoleWriter: consoleWriter,
		consoleRaw:    c.ConsoleFormatIsRaw,
		fileWriter:    fileWriter,
	}
}

// newFileWriter 创建轮转日志文件并包装为异步写入器
func newFileWriter(c *Config) (*asyncWriter, error) {
	if !xutil.DirExist(c.Path) { // 日志所在文件夹不存在则创建
		if err := os.MkdirAll(c.Path, os.ModePerm); err != nil {
			return nil, xerror.Newf("xlog", "init", "os.MkdirAll failed, path=[%s], err=[%v]", c.Path, err)
		}
	}

	logFilePath := path.Join(c.Path, c.Name+".log")
	w, err := rotatelogs.New(
		logFilePath+".%Y%m%d",
		rotatelogs.WithLinkName(logFilePath),
		rotatelogs.WithMaxAge(xutil.ToDuration(c.MaxAge)),
		rotatelogs.WithRotationTime(xutil.ToDuration(c.RotateTime)),
	)
	if err != nil {
		return nil, xerror.Newf("xlog", "init", "rotatelogs.New failed, err=[%v]", err)
	}

	// 异步写入，避免日志 I/O 阻塞调用方
	return newAsyncWriter(w, defaultAsyncBufferSize), nil
}

// setFileWriter 替换当前文件写入器并关闭旧实例，避免重复初始化泄漏 goroutine
func setFileWriter(aw *asyncWriter) {
	fileWriterMu.Lock()
	old := fileWriter
	fileWriter = aw
	fileWriterMu.Unlock()

	if old != nil {
		if err := old.Close(); err != nil {
			xutil.WarnIfEnableDebug("XOne setFileWriter close previous writer failed, err=[%v]", err)
		}
	}

	// 使用高 Order 值确保日志系统在其他模块关闭之后再关闭，避免关闭阶段日志丢失
	stopHookOnce.Do(func() {
		xhook.BeforeStop(closeFileWriter, xhook.Order(closeHookOrder))
	})
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
