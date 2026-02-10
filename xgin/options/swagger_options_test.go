package options

import "testing"

func TestDefaultSwaggerOptions(t *testing.T) {
	opts := DefaultSwaggerOptions()

	if opts.UrlPrefix != "" {
		t.Errorf("UrlPrefix should be empty by default, got %s", opts.UrlPrefix)
	}
}

func TestWithSwaggerUrlPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with prefix", "/api/v1", "/api/v1"},
		{"empty prefix", "", ""},
		{"complex prefix", "/openapi-demo/v4", "/openapi-demo/v4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultSwaggerOptions()
			opt := WithSwaggerUrlPrefix(tt.input)
			opt(opts)

			if opts.UrlPrefix != tt.expected {
				t.Errorf("expected UrlPrefix to be %s, got %s", tt.expected, opts.UrlPrefix)
			}
		})
	}
}
