package xgorm

import (
	"context"

	"github.com/xiaoshicae/xone/v2/xlog"

	"gorm.io/gorm"
)

// C 获取gorm client，支持指定client name获取，name为空则默认获取第一个client，推荐使用 CWithCtx()，保证ctx中内容能传递到下游(trace等)
func C(name ...string) *gorm.DB {
	client := get(name...)
	if client != nil {
		return client
	}

	n := ""
	if len(name) > 0 {
		n = name[0]
	}
	xlog.Error(context.Background(), "no gorm client found for name: %s, maybe config not assigned", n)
	return nil
}

// CWithCtx 可以保证ctx中内容能传递到下游(trace等)
func CWithCtx(ctx context.Context, name ...string) *gorm.DB {
	c := C(name...)
	if c == nil {
		return nil
	}
	return c.WithContext(ctx)
}
