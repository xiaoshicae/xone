package xconfig

import (
	"maps"
	"strings"
)

// deepMerge 递归合并两个配置树，override 覆盖 base，返回新 map 不改动入参
//
// 只有两侧都是 map 时才往下走，其余情况整体替换 —— 半个列表没有意义。
//
// 不做环检测：入参只来自 YAML/JSON 解析结果，两者都表达不出自引用
// （YAML 的递归锚点会被解析器拒绝：anchor 'x' value contains itself），
// 嵌套深度也被解析器限制。若改由别处传入任意 map，自引用会撑爆栈。
func deepMerge(base, override map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(override))
	maps.Copy(merged, base)

	for k, v := range override {
		subOverride, isMap := v.(map[string]any)
		if !isMap {
			merged[k] = v
			continue
		}
		subBase, baseIsMap := merged[k].(map[string]any)
		if !baseIsMap {
			merged[k] = v
			continue
		}
		merged[k] = deepMerge(subBase, subOverride)
	}
	return merged
}

// lookupKeyPath 按路径段读取嵌套 map 中的值，路径不存在时返回 nil
func lookupKeyPath(settings map[string]any, path ...string) any {
	if len(path) == 0 {
		return nil
	}

	current := settings
	for _, k := range path[:len(path)-1] {
		next, ok := current[k].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current[path[len(path)-1]]
}

// setKeyPath 按路径段写入嵌套 map，缺失的中间节点会被创建
//
// 中间节点存在但不是 map 时会被整体替换：调用方写的是一条确定的路径，
// 保留一个类型对不上的旧值只会让后续读取拿到似是而非的结果。
func setKeyPath(settings map[string]any, value any, path ...string) {
	if len(path) == 0 {
		return
	}

	current := settings
	for _, k := range path[:len(path)-1] {
		next, ok := current[k].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[k] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

// deleteKeyPath 按路径段删除嵌套 map 中的值，并清理掉因此变空的父节点
//
// 清理空父节点是必要的：留下一个空的 Server 块会让 ContainKey("Server") 依然为真，
// 也会让打印出来的配置多出一层没有内容的壳。
func deleteKeyPath(settings map[string]any, path ...string) {
	if len(path) == 0 {
		return
	}

	// 记录沿途的父节点，删除后自底向上清理
	parents := make([]map[string]any, 0, len(path))
	current := settings
	for _, k := range path[:len(path)-1] {
		parents = append(parents, current)
		next, ok := current[k].(map[string]any)
		if !ok {
			return // 路径不存在，无需删除
		}
		current = next
	}
	delete(current, path[len(path)-1])

	for i := len(parents) - 1; i >= 0; i-- {
		if len(current) > 0 {
			return
		}
		current = parents[i]
		delete(current, path[i])
	}
}

// lowerKeys 递归把所有 map key 转成小写，返回新 map 不改动入参
//
// 与 viper 的大小写不敏感存储对齐（viper 内部同样递归下沉到列表元素），
// 这样 application.yml 里的 XLog 与 application-dev.yml 里的 xlog 能合并到一起。
func lowerKeys(settings map[string]any) map[string]any {
	out := make(map[string]any, len(settings))
	for k, v := range settings {
		out[strings.ToLower(k)] = lowerKeysValue(v)
	}
	return out
}

// lowerKeysValue 递归处理任意配置值
//
// 必须穿过列表：xgorm / xredis 的多实例形态就是一个 map 列表，
// 只认 map 的话列表里的 key 不会被小写化，合并与读取都会对不上。
func lowerKeysValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return lowerKeys(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = lowerKeysValue(item)
		}
		return out
	default:
		return v
	}
}

// distinct 去重并保持出现顺序
func distinct(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}
