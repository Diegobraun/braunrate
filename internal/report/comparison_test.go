package report_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
)

// The caveat is the line that says what could explain the difference without
// the service having changed. Printing the struct instead of the text put a Go
// value on the screen of whoever was reading the comparison.
func TestCaveatIsPrintedAsASentence(t *testing.T) {
	before := sampleDocument()
	after := sampleDocument()
	after.Run.Target = "http://outra-maquina:8080"

	var out strings.Builder
	if err := report.Comparison(&out, comparison.Compare(before, after)); err != nil {
		t.Fatalf("nao gerou a comparacao: %v", err)
	}
	text := out.String()

	if strings.Contains(text, "%!s(") || strings.Contains(text, "bool=") {
		t.Fatalf("a ressalva saiu como valor de Go em vez de frase:\n%s", text)
	}
	if !strings.Contains(text, "outra-maquina") {
		t.Fatalf("a comparacao nao disse que o alvo mudou:\n%s", text)
	}
}
