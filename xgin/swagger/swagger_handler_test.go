package swagger

import "testing"

func TestSwaggerUrl(t *testing.T) {
	expected := "/swagger/*any"
	if SwaggerUrl != expected {
		t.Errorf("SwaggerUrl should be %s, got %s", expected, SwaggerUrl)
	}
}

func TestSwaggerHandler(t *testing.T) {
	if SwaggerHandler == nil {
		t.Error("SwaggerHandler should not be nil")
	}
}
