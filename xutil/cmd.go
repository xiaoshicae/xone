package xutil

import (
	"os"
	"regexp"
	"strings"

	"github.com/xiaoshicae/xone/v3/xerror"
)

var argKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]*$`)

// GetConfigFromArgs 从启动命令获取指定参数
//
// 支持 --key value 与 --key=value 两种写法。
// 只有以 - 开头的参数才会被当作 key 比对：否则上一个参数的值若恰好
// 等于本次要找的 key，会被误判成 key 而返回错误结果。
func GetConfigFromArgs(key string) (string, error) {
	if !argKeyPattern.MatchString(key) {
		return "", xerror.Newf("xutil", "GetConfigFromArgs", "key must match regexp: %s", argKeyPattern.String())
	}

	args := GetOsArgs()
	if len(args) == 0 {
		return "", xerror.Newf("xutil", "GetConfigFromArgs", "arg not found, there is no arg")
	}

	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue // 不是 key，跳过（它可能是上一个 key 的值）
		}
		name := strings.TrimLeft(arg, "-")

		// 空格配置方式 --config c
		if name == key {
			if i+1 == len(args) { // 没有后续参数
				return "", xerror.Newf("xutil", "GetConfigFromArgs", "arg not found, arg not set")
			}
			return args[i+1], nil
		}

		// 等号配置方式 --config=c
		if after, found := strings.CutPrefix(name, key+"="); found {
			return after, nil
		}
	}

	return "", xerror.Newf("xutil", "GetConfigFromArgs", "arg not found")
}

// GetOsArgs 获取启动命令参数（排除程序名）
func GetOsArgs() []string {
	return os.Args[1:]
}
