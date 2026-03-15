package xtrace

// EnableTrace 检查 Trace 是否开启
// 未配置时默认开启，只有明确配置为 false 才关闭
func EnableTrace() bool {
	traceEnabledMu.RLock()
	defer traceEnabledMu.RUnlock()
	return traceEnabled
}
