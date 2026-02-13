package xredis

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/xiaoshicae/xone/v2/xlog"
)

// C 获取 redis client，支持指定 client name 获取，name 为空则默认获取第一个 client
func C(name ...string) *redis.Client {
	client := get(name...)
	if client != nil {
		return client
	}

	n := ""
	if len(name) > 0 {
		n = name[0]
	}
	xlog.Error(context.Background(), "no redis client found for name: %s, maybe config not assigned", n)
	return nil
}

var (
	clientMap = make(map[string]*redis.Client)
	clientMu  sync.RWMutex
)

func get(name ...string) *redis.Client {
	n := defaultClientName
	if len(name) > 0 {
		n = name[0]
	}

	clientMu.RLock()
	defer clientMu.RUnlock()
	return clientMap[n]
}

func set(name string, client *redis.Client) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientMap[name] = client
}

func setDefault(client *redis.Client) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientMap[defaultClientName] = client
}
