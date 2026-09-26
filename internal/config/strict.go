package config

import (
	"fmt"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DecodeStrict 严格解码：认不出的字段是错误，不是忽略。
//
// 集成包经 xconfig.DecodeStrict 用的是这同一个实现：集合元素的默认值要靠元素
// 自己的 UnmarshalYAML 铺，那里必须能拿到同样的严格检查，
// 否则「拼错就失败」在集合里会悄悄失效。
//
// yaml.v3 的严格检查只有 Decoder 上有，而 Decoder 只收文本。所以不借它，
// 先拿 target 的类型把节点走一遍，查出认不出的字段和类型不对的值，再直接
// node.Decode。走的是节点本身，于是每条报错都知道是哪个节点：行号就是它的
// 行号，文件是 load 记下的它来自的那个，占位符展开出来的值按节点遮掉。
//
// 返回的错误要么是 nil，要么是 yaml 自己的错误；字段和类型层面的问题一律是
// *yaml.TypeError——嵌套在 UnmarshalYAML 里的那一次也是，于是 yaml 把它并进
// 外层的报错列表。那一次拿到的就是配置文件里的节点，报错在那里已经说清楚了。
func DecodeStrict(node *yaml.Node, target any) error {
	c := newChecker()
	if err := c.walk(node, reflect.TypeOf(target)); err != nil {
		return err
	}
	if len(c.errs) > 0 {
		return &yaml.TypeError{Errors: c.errs}
	}
	return node.Decode(target)
}

// checker 拿目标类型走一遍节点，收集字段和类型层面的问题
type checker struct {
	files map[*yaml.Node]string // 节点 → 它来自哪个文件
	raw   map[*yaml.Node]string // 占位符展开过的节点 → 配置里写的原文
	errs  []string
}

// newChecker 带上最近一次加载记下的节点来历
func newChecker() *checker {
	c := &checker{}
	if m := origins.Load(); m != nil {
		c.files = *m
	}
	if m := expanded.Load(); m != nil {
		c.raw = *m
	}
	return c
}

var (
	nodeType        = reflect.TypeOf(yaml.Node{})
	unmarshalerType = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()
	// v2 风格的 UnmarshalYAML，yaml.v3 仍然认
	obsoleteType = reflect.TypeOf((*interface {
		UnmarshalYAML(func(any) error) error
	})(nil)).Elem()
)

// walk 按 t 的形状往下走，和 yaml.v3 解码时的走法一致：mapping 进结构体和 map，
// 序列进切片和数组，形状对不上、或者走到了标量，就地试解一次看 yaml 怎么说。
//
// 自己会解的类型（实现了 yaml.Unmarshaler）不往里走，也是试解：严格与否由它的
// UnmarshalYAML 自己决定——文档要求那里也用 DecodeStrict，那一次报的已经是
// 配置文件里的位置，原样收下。在这里就试一次而不是等 node.Decode，是为了
// 集合元素里的问题和外面的一起报，不必改一处、重启一次才看见下一处。
// encoding.TextUnmarshaler（time.Time、netip.Addr）配的是标量，落在试解那一支。
func (c *checker) walk(n *yaml.Node, t reflect.Type) error {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		return c.walk(n.Content[0], t)
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if n.ShortTag() == "!!null" || t == nodeType || t.Kind() == reflect.Interface {
		return nil // null 保持原值；Node、any 什么都收
	}
	switch {
	case reflect.PointerTo(t).Implements(unmarshalerType) || reflect.PointerTo(t).Implements(obsoleteType):
		return c.try(n, t)
	case n.Kind == yaml.MappingNode && t.Kind() == reflect.Struct:
		return c.mapping(n, t, fieldsOf(t))
	case n.Kind == yaml.MappingNode && t.Kind() == reflect.Map:
		return c.mapping(n, t, nil)
	case n.Kind == yaml.SequenceNode && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array):
		for _, e := range n.Content {
			if err := c.walk(e, t.Elem()); err != nil {
				return err
			}
		}
		return nil
	}
	return c.try(n, t)
}

// mapping 逐个 key 往下走。s 为 nil 时 t 是 map，否则 t 是结构体、s 是它的字段
func (c *checker) mapping(n *yaml.Node, t reflect.Type, s *structFields) error {
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == "<<" && k.ShortTag() == "!!merge" {
			// <<: *base 并进来的字段按同一个类型查；一次并好几个时写成序列
			if err := c.merged(v, t); err != nil {
				return err
			}
			continue
		}
		if s == nil {
			if err := c.walk(k, t.Key()); err != nil {
				return err
			}
			if err := c.walk(v, t.Elem()); err != nil {
				return err
			}
			continue
		}
		ft, ok := s.byKey[k.Value]
		if !ok {
			ft = s.inline
		}
		if ft == nil {
			c.errs = append(c.errs, c.at(k)+": "+s.notFound(k.Value, t))
			continue
		}
		if err := c.walk(v, ft); err != nil {
			return err
		}
	}
	return nil
}

func (c *checker) merged(v *yaml.Node, t reflect.Type) error {
	for v.Kind == yaml.AliasNode && v.Alias != nil {
		v = v.Alias
	}
	if v.Kind != yaml.SequenceNode {
		return c.walk(v, t)
	}
	for _, e := range v.Content {
		if err := c.walk(e, t); err != nil {
			return err
		}
	}
	return nil
}

// try 把 n 单独解进一个 t 类型的新值，yaml 报在 n 这一行的类型错误记在 n 名下。
// 节点就是这一个，于是行号、文件、要不要遮掉展开出来的值，都照它说。
// 不在这一行的（嵌套的 UnmarshalYAML 报的）原样收下
func (c *checker) try(n *yaml.Node, t reflect.Type) error {
	err := n.Decode(reflect.New(t).Interface())
	te, ok := err.(*yaml.TypeError)
	if !ok {
		if err != nil { // yaml 当成致命错误的（比如 UnmarshalText 失败），照样停在这里
			return fmt.Errorf("%s: %w", c.at(n), err)
		}
		return nil
	}
	for _, e := range te.Errors {
		if msg, ok := strings.CutPrefix(e, fmt.Sprintf("line %d: ", n.Line)); ok {
			e = c.at(n) + ": " + c.redact(n, msg)
		}
		c.errs = append(c.errs, e)
	}
	return nil
}

// at 报错开头的位置：n 是从配置文件里解析出来的（load 记过它）时是「文件:行号」，
// 否则是 yaml 自己的写法「line 行号」
func (c *checker) at(n *yaml.Node) string {
	if file := fileOf(n, c.files); file != "" {
		return fmt.Sprintf("%s:%d", file, n.Line)
	}
	return fmt.Sprintf("line %d", n.Line)
}

// fileOf n 来自哪个文件，不知道时为空。
//
// 合并出来的 map 节点是一份新的副本（见 mergeMapping），load 没记过它；
// 它的行号取自第一个 key，所以文件也跟第一个 key 走
func fileOf(n *yaml.Node, files map[*yaml.Node]string) string {
	for ; n != nil; n = first(n) {
		if f, ok := files[n]; ok {
			return f
		}
	}
	return ""
}

func first(n *yaml.Node) *yaml.Node {
	if len(n.Content) == 0 {
		return nil
	}
	return n.Content[0]
}

// redact 把 n 的类型错误里带出来的值换回配置里写的那段原文。
//
// yaml 的类型错误会带上值的前几个字符：密码填进了 int 字段，
// 报的就是 cannot unmarshal !!str `hunter2...` into int——凭证的一截进了启动日志，
// 而 ${VAR} 恰恰是凭证的推荐写法。换成 `${DB_PASSWORD}` 之后照样看得出是哪一项配错了。
// 只看这一个节点是不是展开出来的，配置里别处恰好同值的字面量不受影响。
func (c *checker) redact(n *yaml.Node, msg string) string {
	raw, ok := c.raw[n]
	if !ok {
		return msg
	}
	i, j := strings.Index(msg, "`"), strings.LastIndex(msg, "`")
	if i == j {
		return msg // 报错里没带值（!!map into string 这一类）
	}
	return msg[:i] + "`" + raw + "` (expanded value redacted)" + msg[j+1:]
}

// structFields 一个结构体在 yaml 眼里有哪些 key，规则照搬 yaml.v3 的 getStructInfo
type structFields struct {
	byKey    map[string]reflect.Type // key → 字段类型，,inline 的结构体已经摊平
	inline   reflect.Type            // ,inline 的 map 的元素类型：认不出的 key 都进它，没有则为 nil
	untagged map[string]bool         // 没写 yaml tag 的字段名，给报错补提示用
}

func fieldsOf(t reflect.Type) *structFields {
	s := &structFields{byKey: map[string]reflect.Type{}, untagged: map[string]bool{}}
	s.add(t)
	return s
}

func (s *structFields) add(t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "" && !strings.Contains(string(f.Tag), ":") {
			tag = string(f.Tag) // yaml.v3 还认整段 tag 就是名字的老写法
		}
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if strings.Contains(","+opts+",", ",inline,") {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			switch {
			case ft.Kind() == reflect.Map:
				s.inline = ft.Elem()
			case !reflect.PointerTo(ft).Implements(unmarshalerType):
				s.add(ft) // 自己会解的内嵌结构体 yaml 不摊平，它的字段在这里也认不出
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		if tag == "" {
			s.untagged[f.Name] = true
		}
		s.byKey[name] = f.Type
	}
}

// notFound 认不出的字段怎么报：和 yaml.v3 的原文一致，字段忘了写 yaml tag 时补一句怎么改。
//
// yaml.v3 对没写 tag 的字段只认全小写的 key：字段 Endpoint 认 endpoint，
// 不认 Endpoint。于是配置里照着字段名写，报的是
// 「field Endpoint not found in type C」——字段明明就叫这个，使用者只会一头雾水。
func (s *structFields) notFound(key string, t reflect.Type) string {
	msg := fmt.Sprintf("field %s not found in type %s", key, t)
	if s.untagged[key] {
		msg += fmt.Sprintf(" (field %s has no yaml tag, so only %q is accepted: add `yaml:\"%s\"`)",
			key, strings.ToLower(key), key)
	}
	return msg
}
