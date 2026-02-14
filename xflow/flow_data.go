package xflow

// FlowData 贯穿所有 Processor 的流程数据容器
// Request 为入参（语义上不可变），Response 为出参（Processor 填充），extra 为 Processor 间临时数据
type FlowData[Req, Resp any] struct {
	// Request 入参，语义上不可变
	Request Req
	// Response 出参，由 Processor 逐步填充
	Response Resp
	// extra Processor 间临时数据（惰性初始化）
	extra map[string]any
}

// Set 存储临时数据到 extra
func (d *FlowData[Req, Resp]) Set(key string, val any) {
	if d.extra == nil {
		d.extra = make(map[string]any)
	}
	d.extra[key] = val
}

// Get 获取临时数据
func (d *FlowData[Req, Resp]) Get(key string) (any, bool) {
	if d.extra == nil {
		return nil, false
	}
	v, ok := d.extra[key]
	return v, ok
}

// Key 类型安全的临时数据键
type Key[V any] struct {
	name string
}

// NewKey 创建类型安全的临时数据键
func NewKey[V any](name string) Key[V] {
	return Key[V]{name: name}
}

// SetExtra 类型安全地存储临时数据
func SetExtra[V, Req, Resp any](d *FlowData[Req, Resp], key Key[V], val V) {
	d.Set(key.name, val)
}

// GetExtra 类型安全地获取临时数据
func GetExtra[V, Req, Resp any](d *FlowData[Req, Resp], key Key[V]) (V, bool) {
	v, ok := d.Get(key.name)
	if !ok {
		var zero V
		return zero, false
	}
	typed, ok := v.(V)
	if !ok {
		var zero V
		return zero, false
	}
	return typed, true
}
