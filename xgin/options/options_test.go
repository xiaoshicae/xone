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

func TestLogSkipPaths(t *testing.T) {
	opts := DefaultOptions()
	opt := LogSkipPaths("/health", "/ready")
	opt(opts)

	if len(opts.LogSkipPaths) != 2 {
		t.Errorf("expected 2 skip paths, got %d", len(opts.LogSkipPaths))
	}
	if opts.LogSkipPaths[0] != "/health" {
		t.Errorf("expected first path '/health', got %s", opts.LogSkipPaths[0])
	}
	if opts.LogSkipPaths[1] != "/ready" {
		t.Errorf("expected second path '/ready', got %s", opts.LogSkipPaths[1])
	}
}

func TestMultipleOptions(t *testing.T) {
	opts := DefaultOptions()

	options := []Option{
		EnableLogMiddleware(false),
		EnableTraceMiddleware(false),
		EnableZHTranslations(true),
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
}
