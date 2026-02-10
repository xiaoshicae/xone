package xlog

import (
	"context"
	"testing"

	"github.com/xiaoshicae/xone/xlog"
	"github.com/xiaoshicae/xone/xserver"
)

func TestXLog(t *testing.T) {
	t.Skip("真实环境测试，如果client能连通，可以注释掉该Skip进行测试")
	if err := xserver.R(); err != nil {
		panic(err)
	}

	myKV := map[string]interface{}{"key": "value", "key2": "value2"}
	xlog.Info(context.TODO(), "hello world %s", "hhh", xlog.KV("kkk", "vvv"), xlog.KVMap(myKV))

	xlog.Debug(context.Background(), "hello world some thing ....", xlog.KV("key", "value"), xlog.KV("k2", 3))
	xlog.Info(context.Background(), "info ....", xlog.KV("key", "value"), xlog.KV("k2", 3))
	xlog.Warn(context.Background(), "warn ....", xlog.KV("key", "value"), xlog.KV("k2", 3))

	xlog.Error(context.Background(), "err ....", xlog.KV("key", "value"), xlog.KV("k2", 3))
}
