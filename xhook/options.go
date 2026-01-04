package xhook

func Order(order int) Option {
	return func(o *options) {
		o.Order = order
	}
}

func MustInvokeSuccess(success bool) Option {
	return func(o *options) {
		o.MustInvokeSuccess = success
	}
}

type Option func(*options)

type options struct {
	Order             int
	MustInvokeSuccess bool
}

func defaultOptions() *options {
	return &options{
		Order:             100,
		MustInvokeSuccess: true,
	}
}
