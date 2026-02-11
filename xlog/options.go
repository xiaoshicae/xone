package xlog

func KV(k string, v any) Option {
	return func(o *options) {
		o.KV[k] = v
	}
}

func KVMap(m map[string]any) Option {
	return func(o *options) {
		for k, v := range m {
			o.KV[k] = v
		}
	}
}

type Option func(*options)

type options struct {
	KV map[string]any
}

func defaultOptions() *options {
	return &options{
		KV: make(map[string]any),
	}
}
