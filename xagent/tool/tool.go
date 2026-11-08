// Package tool 定义 agent 可以调用的工具。
//
// 这个包不认识 llm 包：写工具的人只需要关心「收到一段 JSON 参数，返回一段文本」，
// 不需要了解消息格式。
package tool

import (
	"context"
	"encoding/json"

	"github.com/xiaoshicae/xone/xerror"
)

// Tool 一个可被模型调用的工具。
type Tool interface {
	// Spec 描述这个工具，会原样发给模型。描述写得越清楚，模型越会用。
	Spec() Spec

	// Call 执行一次调用。input 是模型给的参数（JSON），不保证合法，要自己校验。
	//
	// 返回的 error 不会中断 agent：它会作为一条错误结果回给模型，
	// 模型通常会据此换个参数重试。所以错误消息要写给模型看：说清楚哪里不对、应该怎么改。
	Call(ctx context.Context, input json.RawMessage) (string, error)
}

// Spec 工具的名字、用途和参数格式。
type Spec struct {
	Name        string          // 模型用它来指定工具，同一个 Set 里不能重复
	Description string          // 这个工具做什么、什么时候该用
	InputSchema json.RawMessage // 参数的 JSON Schema
}

// Set 一组工具，按名字查找。
type Set struct {
	byName map[string]Tool
	specs  []Spec // 保持传入顺序：每次发给模型的工具列表顺序固定
}

// NewSet 把工具收成一组。空名、重名在这里就报错，不留到模型调用时才发现。
func NewSet(tools ...Tool) (*Set, error) {
	s := &Set{byName: make(map[string]Tool, len(tools))}
	for i, t := range tools {
		if t == nil {
			return nil, xerror.Newf("xagent", "new", "tool #%d is nil", i)
		}
		spec := t.Spec()
		if spec.Name == "" {
			return nil, xerror.Newf("xagent", "new", "tool #%d has an empty name", i)
		}
		if _, dup := s.byName[spec.Name]; dup {
			return nil, xerror.Newf("xagent", "new", "duplicate tool name %q", spec.Name)
		}
		s.byName[spec.Name] = t
		s.specs = append(s.specs, spec)
	}
	return s, nil
}

// Get 按名字取工具。
func (s *Set) Get(name string) (Tool, bool) {
	t, ok := s.byName[name]
	return t, ok
}

// Specs 全部工具的描述，按传入顺序。
func (s *Set) Specs() []Spec {
	return s.specs
}
