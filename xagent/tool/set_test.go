package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xiaoshicae/xone/xagent/llm"
)

type named string

func (n named) Def() llm.ToolDef { return llm.ToolDef{Name: string(n)} }
func (n named) Call(context.Context, json.RawMessage) (string, error) {
	return string(n), nil
}

func TestNewSet_KeepsOrderAndFindsByName(t *testing.T) {
	s, err := NewSet(named("b"), named("a"))
	if err != nil {
		t.Fatal(err)
	}
	if defs := s.Defs(); len(defs) != 2 || defs[0].Name != "b" || defs[1].Name != "a" {
		t.Fatalf("defs not in input order: %+v", defs)
	}
	if got, ok := s.Get("a"); !ok || got != named("a") {
		t.Fatalf("Get(a) = %v, %v", got, ok)
	}
	if _, ok := s.Get("c"); ok {
		t.Fatal("Get(c) should miss")
	}
}

func TestNewSet_RejectsNilEmptyAndDuplicateNames(t *testing.T) {
	cases := map[string][]Tool{
		"nil":       {nil},
		"empty":     {named("")},
		"duplicate": {named("a"), named("a")},
	}
	for name, tools := range cases {
		if _, err := NewSet(tools...); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
