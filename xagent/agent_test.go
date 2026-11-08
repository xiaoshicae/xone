package xagent

import (
	"testing"

	"github.com/xiaoshicae/xone/xagent/tool"
	"github.com/xiaoshicae/xone/xerror"
)

func TestNew_RejectsInvalidArguments(t *testing.T) {
	m := &scriptedModel{}
	cases := map[string]func() (*Agent, error){
		"nil model":      func() (*Agent, error) { return New(nil, nil) },
		"zero max steps": func() (*Agent, error) { return New(m, nil, WithMaxSteps(0)) },
		"duplicate tool": func() (*Agent, error) { return New(m, []tool.Tool{echo("a"), echo("a")}) },
	}
	for name, fn := range cases {
		if _, err := fn(); err == nil || !xerror.Is(err, "xagent") {
			t.Errorf("%s: want xagent error, got %v", name, err)
		}
	}
}
