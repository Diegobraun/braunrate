package metrics_test

import (
	"go/build"
	"strings"
	"testing"
)

// metrics may depend on the shared protocol vocabulary — error class, response
// attributes, consumer lag — because none of that belongs to a protocol in
// particular. What it may never depend on is one protocol: that is where
// protocol-specific measurement starts, and from there the numbers of Kafka and
// of HTTP stop being comparable (ADR 0003 §3).
func TestMetricsDoesNotKnowAnyProtocolInParticular(t *testing.T) {
	packageInfo, err := build.Import("github.com/Diegobraun/braunrate/internal/metrics", "", 0)
	if err != nil {
		t.Fatalf("nao consegui ler o pacote: %v", err)
	}
	for _, imported := range packageInfo.Imports {
		if strings.Contains(imported, "internal/protocol/") {
			t.Errorf("metrics importa %q: medicao especifica de protocolo comeca aqui", imported)
		}
	}
}
