package xlog

// KV 为日志附加一个自定义字段
// 作为参数传入 Debug/Info/Warn/Error/RawLog，会被提取为日志的 JSON 字段
func KV(k string, v any) Option {
	return func(o *options) {
		o.ensureKV(kvSmallHint)
		o.KV[k] = v
	}
}

// KVMap 为日志批量附加自定义字段，用法同 KV
func KVMap(m map[string]any) Option {
	return func(o *options) {
		// 按真实字段数建 map。由 RawLog 统一按 Option 个数预分配是不够的：
		// 一个 KVMap 就可能带十几个字段（access log 就是），
		// 而 Go 的 map 没法在创建后再扩容，只能一路 rehash 上去
		o.ensureKV(len(m))
		for k, v := range m {
			o.KV[k] = v
		}
	}
}

// Option 日志的可选参数，由 KV / KVMap 构造
type Option func(*options)

// kvSmallHint 逐个 KV 添加时 map 的初始容量
const kvSmallHint = 4

// options 收集一次日志调用的自定义字段
// 使用 map 而非切片，使同名字段后者覆盖前者
type options struct {
	KV map[string]any
}

// ensureKV 按需创建 KV map，容量由第一个写入方给出
//
// 交给 Option 自己建而不是 RawLog 预建：只有 Option 知道自己要塞几个字段。
func (o *options) ensureKV(hint int) {
	if o.KV != nil {
		return
	}
	if hint < kvSmallHint {
		hint = kvSmallHint
	}
	o.KV = make(map[string]any, hint)
}
