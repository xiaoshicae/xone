package xlog

// KV 为日志附加一个自定义字段
// 作为参数传入 Debug/Info/Warn/Error/RawLog，会被提取为日志的 JSON 字段
func KV(k string, v any) Option {
	return func(o *options) {
		o.KV[k] = v
	}
}

// KVMap 为日志批量附加自定义字段，用法同 KV
func KVMap(m map[string]any) Option {
	return func(o *options) {
		for k, v := range m {
			o.KV[k] = v
		}
	}
}

// Option 日志的可选参数，由 KV / KVMap 构造
type Option func(*options)

// options 收集一次日志调用的自定义字段
// 使用 map 而非切片，使同名字段后者覆盖前者
type options struct {
	KV map[string]any
}
