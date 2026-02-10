package options

import "testing"

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if !opts.EnableLogMiddleware {
		t.Error("EnableLogMiddleware should be true by default")
	}
	if !opts.EnableTraceMiddleware {
		t.Error("EnableTraceMiddleware should be true by default")
	}
	if opts.EnableZHTranslations {
		t.Error("EnableZHTranslations should be false by default")
	}
	if opts.Addr != "0.0.0.0:8080" {
		t.Errorf("Addr should be '0.0.0.0:8080' by default, got %s", opts.Addr)
	}
}

func TestEnableLogMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected bool
	}{
		{"enable", true, true},
		{"disable", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultOptions()
			opt := EnableLogMiddleware(tt.input)
			opt(opts)

			if opts.EnableLogMiddleware != tt.expected {
				t.Errorf("expected EnableLogMiddleware to be %v, got %v", tt.expected, opts.EnableLogMiddleware)
			}
		})
	}
}

func TestEnableTraceMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected bool
	}{
		{"enable", true, true},
		{"disable", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultOptions()
			opt := EnableTraceMiddleware(tt.input)
			opt(opts)

			if opts.EnableTraceMiddleware != tt.expected {
				t.Errorf("expected EnableTraceMiddleware to be %v, got %v", tt.expected, opts.EnableTraceMiddleware)
			}
		})
	}
}

func TestEnableZHTranslations(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected bool
	}{
		{"enable", true, true},
		{"disable", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultOptions()
			opt := EnableZHTranslations(tt.input)
			opt(opts)

			if opts.EnableZHTranslations != tt.expected {
				t.Errorf("expected EnableZHTranslations to be %v, got %v", tt.expected, opts.EnableZHTranslations)
			}
		})
	}
}

func TestAddr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"custom addr", "127.0.0.1:9000", "127.0.0.1:9000"},
		{"empty addr", "", ""},
		{"localhost", "localhost:8080", "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultOptions()
			opt := Addr(tt.input)
			opt(opts)

			if opts.Addr != tt.expected {
				t.Errorf("expected Addr to be %s, got %s", tt.expected, opts.Addr)
			}
		})
	}
}

func TestMultipleOptions(t *testing.T) {
	opts := DefaultOptions()

	options := []Option{
		EnableLogMiddleware(false),
		EnableTraceMiddleware(false),
		EnableZHTranslations(true),
		Addr("192.168.1.1:3000"),
	}

	for _, opt := range options {
		opt(opts)
	}

	if opts.EnableLogMiddleware {
		t.Error("EnableLogMiddleware should be false")
	}
	if opts.EnableTraceMiddleware {
		t.Error("EnableTraceMiddleware should be false")
	}
	if !opts.EnableZHTranslations {
		t.Error("EnableZHTranslations should be true")
	}
	if opts.Addr != "192.168.1.1:3000" {
		t.Errorf("Addr should be '192.168.1.1:3000', got %s", opts.Addr)
	}
}
