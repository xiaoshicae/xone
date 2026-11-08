package tool

import (
	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xerror"
)

// Set 一组工具，按名字查找。
type Set struct {
	byName map[string]Tool
	defs   []llm.ToolDef // 保持传入顺序：每次发给模型的工具列表顺序固定
}

// NewSet 把工具收成一组。空名、重名在这里就报错，不留到模型调用时才发现。
func NewSet(tools ...Tool) (*Set, error) {
	s := &Set{byName: make(map[string]Tool, len(tools))}
	for i, t := range tools {
		if t == nil {
			return nil, xerror.Newf("xagent", "new", "tool #%d is nil", i)
		}
		def := t.Def()
		if def.Name == "" {
			return nil, xerror.Newf("xagent", "new", "tool #%d has an empty name", i)
		}
		if _, dup := s.byName[def.Name]; dup {
			return nil, xerror.Newf("xagent", "new", "duplicate tool name %q", def.Name)
		}
		s.byName[def.Name] = t
		s.defs = append(s.defs, def)
	}
	return s, nil
}

// Get 按名字取工具。
func (s *Set) Get(name string) (Tool, bool) {
	t, ok := s.byName[name]
	return t, ok
}

// Defs 全部工具的声明，按传入顺序。
func (s *Set) Defs() []llm.ToolDef {
	return s.defs
}
