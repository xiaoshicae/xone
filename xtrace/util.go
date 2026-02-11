package xtrace

import "github.com/xiaoshicae/xone/v2/xconfig"

// xTraceEnableKey 从 XTraceConfigKey 推导，避免硬编码不同步
var xTraceEnableKey = XTraceConfigKey + ".Enable"

// EnableTrace 检查 Trace 是否开启
// 未配置时默认开启，只有明确配置为 false 才关闭
func EnableTrace() bool {
	if !xconfig.ContainKey(xTraceEnableKey) {
		return true
	}
	return xconfig.GetBool(xTraceEnableKey)
}
