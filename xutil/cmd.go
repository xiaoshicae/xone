package xutil

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var argKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]*$`)

// GetConfigFromArgs 从启动命令获取指定参数
func GetConfigFromArgs(key string) (string, error) {
	if !argKeyPattern.MatchString(key) {
		return "", fmt.Errorf("key must match regexp: %s", argKeyPattern.String())
	}

	args := GetOsArgs()
	if len(args) == 0 {
		return "", fmt.Errorf("arg not found, there is no arg")
	}

	for i, arg := range args {
		arg = strings.TrimLeft(arg, "-")

		// 空格配置方式 --config c
		if arg == key {
			if i+1 == len(args) { // 没有后续参数
				return "", fmt.Errorf("arg not found, arg not set")
			}
			return args[i+1], nil
		}

		// 等号配置方式 --config=c
		if strings.HasPrefix(arg, key+"=") {
			return arg[len(key)+1:], nil
		}
	}

	return "", fmt.Errorf("arg not found")
}

// GetOsArgs 获取启动命令参数（排除程序名）
func GetOsArgs() []string {
	return os.Args[1:]
}
