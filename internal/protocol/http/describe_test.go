package http

import (
	"strings"
	"testing"
)

// The wire request keeps the declared header, because Set overwrites. Printing
// both made debug show a Content-Type that never went out, which is the one
// place a person checks what was actually sent.
func TestDebugShowsOneContentTypeAndItIsTheOneThatGoesOut(t *testing.T) {
	config := &Config{
		Method:      "POST",
		Path:        "/auth/token",
		Headers:     map[string]string{"Content-Type": "application/json"},
		Body:        []byte(`{"usuario":"ana"}`),
		ContentType: "text/plain",
	}

	described := strings.Join(config.Describe(), "\n")
	if strings.Count(described, "Content-Type:") != 1 {
		t.Fatalf("a depuracao mostrou mais de um Content-Type:\n%s", described)
	}
	if !strings.Contains(described, "Content-Type: application/json") {
		t.Fatalf("mostrou o inferido no lugar do declarado:\n%s", described)
	}
}
