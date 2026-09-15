package xconfig

import (
	"os"
	"regexp"

	"github.com/xiaoshicae/xone/v3/xerror"
)

// 占位符语法，分隔符是 ":"，与 Spring 一致：
//
//	${VAR}          必填，环境变量未设置时初始化失败
//	${VAR:default}  可选，未设置时用 default（想要空值写 ${VAR:}）
var (
	// placeholderRegex 合法占位符
	placeholderRegex = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

	// anyPlaceholderRegex 任意 ${...} 形态，用来发现写错的占位符
	//
	// 只靠 placeholderRegex 是「匹配不上就当普通文本」：${} 这种不报错也不告警，
	// 值就真的变成了那串字面量，错误要到用它的地方才以另一副面孔出现。
	anyPlaceholderRegex = regexp.MustCompile(`\$\{[^{}]*\}`)
)

// resolver 展开占位符，并累积途中发现的问题
//
// 问题分两类而不是合成一个列表：「变量没设置」是部署环境的问题，
// 「写法不支持」是配置文件写错了，两者的修复动作完全不同。
//
// 加载过程中有三处需要展开占位符 —— 激活环境、导入路径、以及合并后的整棵树 ——
// 它们共用这一个类型：各自 new 一个 resolver，展开若干次，最后问一次 err()。
type resolver struct {
	missing     []string
	unsupported []string
}

// expand 展开单个字符串
//
// 用 os.LookupEnv 而非 os.Getenv：显式设为空串的环境变量是一个有效取值，
// 应当覆盖默认值，而不是被当作未设置。这一点与 Spring 相同。
func (r *resolver) expand(s string) string {
	if s == "" {
		return s
	}

	// 写法不被支持的先挑出来：它们匹配不上 placeholderRegex，
	// 不检查的话会被当成普通文本原样留下
	for _, m := range anyPlaceholderRegex.FindAllString(s, -1) {
		if placeholderRegex.FindString(m) != m {
			r.unsupported = append(r.unsupported, m)
		}
	}

	return placeholderRegex.ReplaceAllStringFunc(s, func(match string) string {
		idx := placeholderRegex.FindStringSubmatchIndex(match)
		name := match[idx[2]:idx[3]]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		// 分组 2 参与匹配即表示写了 ":"，无论默认值是否为空。
		// 用下标判断而不是查字符串里有没有 ":"：变量名里不可能有 ":"，
		// 但默认值里可能有（如 ${ADDR:127.0.0.1:6379}）
		if idx[4] >= 0 {
			return match[idx[4]:idx[5]]
		}
		r.missing = append(r.missing, name)
		return match
	})
}

// expandValue 递归展开任意配置值，返回新值不改动入参
//
// 覆盖所有 string 叶子节点：map 的 value、列表元素，以及列表里嵌套的 map ——
// xgorm / xredis 的多实例形态正是最后这种形状：
//
//	XRedis:
//	  - Name: cache
//	    Password: "${REDIS_PW}"
//
// 数值、布尔等其余类型不含占位符，原样返回。
func (r *resolver) expandValue(v any) any {
	switch t := v.(type) {
	case string:
		return r.expand(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = r.expandValue(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = r.expandValue(item)
		}
		return out
	default:
		return v
	}
}

// err 把累积的问题转成错误，两类各自给出可操作的提示；没有问题时返回 nil
func (r *resolver) err(op string) error {
	if len(r.unsupported) > 0 {
		return xerror.Newf("xconfig", op,
			"unsupported placeholder syntax, supported forms are ${VAR} and ${VAR:default}, got=%v",
			distinct(r.unsupported))
	}
	if len(r.missing) > 0 {
		return xerror.Newf("xconfig", op,
			"required env placeholder not set, use ${VAR:default} to make it optional, missing=%v",
			distinct(r.missing))
	}
	return nil
}

// expandPlaceholders 展开整棵配置树中的占位符，返回新树不改动入参
//
// 在合并完成之后统一展开一次，而不是每个文件各展开各的：
// 一个被后续层覆盖掉的值不该因为它引用的变量没设置就让进程起不来。
// 展开结果不会被再次解释，环境变量的值里若恰好含有 ${...} 会原样保留。
func expandPlaceholders(settings map[string]any) (map[string]any, error) {
	r := &resolver{}
	expanded, _ := r.expandValue(settings).(map[string]any)
	if err := r.err("expandPlaceholders"); err != nil {
		return nil, err
	}
	return expanded, nil
}
