package xlog

import (
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xutil"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	logwriter "github.com/sirupsen/logrus/hooks/writer"
)

var (
	findFrameIgnoreFileNames = []string{
		"/xlog/util.go",
		"/xlog/xlog_hook.go",
	}
)

func init() {
	xhook.BeforeStart(initXLog)
}

func initXLog() error {
	c, err := getConfig()
	if err != nil {
		return fmt.Errorf("XOne initXLog getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initXLog got config: %s", xutil.ToJsonString(c))

	return initXLogByConfig(c)
}

func initXLogByConfig(c *Config) error {
	if !xutil.DirExist(c.Path) { // 日志所在文件夹不存在则创建
		if err := os.MkdirAll(c.Path, os.ModePerm); err != nil {
			return fmt.Errorf("XOne initXLogByConfig invoke os.MkdirAll failed, path=[%s], err=[%v]", c.Path, err)
		}
	}

	// 创建 file writer
	logFilePath := path.Join(c.Path, c.Name+".log")
	fileWriter, err := rotatelogs.New(
		logFilePath+".%Y%m%d",
		rotatelogs.WithLinkName(logFilePath),
		rotatelogs.WithMaxAge(xutil.ToDuration(c.MaxAge)),
		rotatelogs.WithRotationTime(xutil.ToDuration(c.RotateTime)),
	)
	if err != nil {
		return fmt.Errorf("XOne initXLogByConfig invoke rotatelogs.New failed, err=[%v]", err)
	}

	// 注册关闭钩子
	xhook.BeforeStop(func() error {
		return fileWriter.Close()
	})

	// 加载时区
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		xutil.WarnIfEnableDebug("XOne initXLogByConfig load timezone [%s] failed, using Local timezone, err=[%v]", c.Timezone, err)
		loc = time.Local
	}

	logrus.SetOutput(io.Discard)

	// 设置日志输出格式
	logrus.SetFormatter(timeFormatter{
		Formatter: &logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.999",
			CallerPrettyfier: func(*runtime.Frame) (function string, file string) {
				return "", "" // 去掉自带的file和func字段
			},
		},
		Location: loc,
	})

	localIP, _ := xutil.GetLocalIp()
	localIP = xutil.GetOrDefault(localIP, "0.0.0.0")

	// 自定义hook，进行日志format和打印到屏幕
	logrus.AddHook(&xLogHook{
		SuffixToIgnore:     findFrameIgnoreFileNames,
		ServerName:         xconfig.GetServerName(),
		IP:                 localIP,
		PidStr:             strconv.Itoa(os.Getpid()), // 初始化时转换，避免每次日志都转换
		Console:            c.Console,
		ConsoleFormatIsRaw: c.ConsoleFormatIsRaw,
		Writer:             os.Stdout,
	})

	// file writer hook
	logrus.AddHook(&logwriter.Hook{
		Writer:    fileWriter,
		LogLevels: resolveLevels(c.Level),
	})

	l, err := logrus.ParseLevel(c.Level)
	if err != nil {
		l = logrus.InfoLevel
	}
	logrus.SetLevel(l)

	return nil
}

func getConfig() (*Config, error) {
	// 获取配置
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XLogConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}

// levelMapping 日志级别映射，避免使用魔术数字
var levelMapping = map[string][]logrus.Level{
	"debug": {logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel, logrus.InfoLevel, logrus.DebugLevel},
	"info":  {logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel, logrus.InfoLevel},
	"warn":  {logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel},
	"error": {logrus.FatalLevel, logrus.ErrorLevel},
	"fatal": {logrus.FatalLevel},
}

func resolveLevels(l string) []logrus.Level {
	if levels, ok := levelMapping[strings.ToLower(l)]; ok {
		return levels
	}
	// 默认 info 级别
	return levelMapping["info"]
}
