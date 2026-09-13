package xredis

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// C 获取 redis client，支持指定 client name 获取，name 为空则取默认 client
//
// 找不到时 panic。返回 nil 并不会让程序走得更远——*redis.Client 的任何方法在 nil 上
// 都是空指针解引用，只是把同一个 panic 推迟到调用方第一次用它的时候，
// 而那里的栈里只剩 "invalid memory address"，看不出根因是配置没配。
// 这是启动期的配置问题，不是运行期需要处理的错误。
//
// 可选依赖（配了就用、没配就跳过）用 Has() 先判断。
func C(name ...string) *redis.Client {
	client := get(name...)
	if client == nil {
		panic(noClientMsg(name...))
	}
	return client
}

// Has 报告指定 client 是否已配置
//
// 供可选依赖使用：配了就用、没配就跳过，不必用 C 去触发 panic。
func Has(name ...string) bool {
	return get(name...) != nil
}

// noClientMsg 拼装 panic 信息，带上已配置的 client 名称
//
// 只报"没找到"帮助有限：名字写错和整个 XRedis 没配是两个不同的问题，
// 列出实际配了哪些，两者一眼可分。
func noClientMsg(name ...string) string {
	want := "default"
	if len(name) > 0 {
		want = name[0]
	}

	configured := clientNames()
	if len(configured) == 0 {
		return fmt.Sprintf("XOne xredis: no client found for name=[%s], no client configured at all, check the XRedis section of your config", want)
	}
	return fmt.Sprintf("XOne xredis: no client found for name=[%s], configured=[%s]", want, strings.Join(configured, " "))
}

// clientNames 返回已配置的 client 名称，不含内部默认别名
func clientNames() []string {
	clientMu.RLock()
	defer clientMu.RUnlock()

	names := slices.Sorted(maps.Keys(clientMap))
	return slices.DeleteFunc(names, func(n string) bool { return n == defaultClientName })
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

// removeClients 把指定配置对应的 client 从 clientMap 中摘除，用于初始化失败回滚
func removeClients(configs []*Config) {
	clientMu.Lock()
	defer clientMu.Unlock()
	for _, c := range configs {
		delete(clientMap, c.Name)
	}
	delete(clientMap, defaultClientName)
}
