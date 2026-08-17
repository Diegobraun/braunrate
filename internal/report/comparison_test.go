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
		t.Fatalf("não gerou a comparação: %v", err)
	}
	text := out.String()

	if strings.Contains(text, "%!s(") || strings.Contains(text, "bool=") {
		t.Fatalf("a ressalva saiu como valor de Go em vez de frase:\n%s", text)
	}
	if !strings.Contains(text, "outra-maquina") {
		t.Fatalf("a comparação não disse que o alvo mudou:\n%s", text)
	}
}

// "Nada" was an absolute claim over the five fields the comparison checks.
// Replacing the whole CSV between two runs changed the p95 by 15x and the
// comparison still said nothing but the service could explain it.
func TestNoCaveatSaysWhatItCompared(t *testing.T) {
	before := sampleDocument()
	after := sampleDocument()
	before.Variety, after.Variety = nil, nil
	before.Run.AuthObtains, after.Run.AuthObtains = 0, 0
	compared := comparison.Compare(before, after)
	if len(compared.Caveats) != 0 {
		t.Fatalf("as duas execuções deveriam sair sem ressalva: %+v", compared.Caveats)
	}

	var terminal strings.Builder
	if err := report.Comparison(&terminal, compared); err != nil {
		t.Fatalf("não gerou a comparação: %v", err)
	}
	text := terminal.String()
	if strings.Contains(text, "Nothing: same scenario") {
		t.Error("a comparação afirma que nada explica a diferença, sobre cinco campos que ela checou")
	}
	if !strings.Contains(text, "data") {
		t.Error("a comparação não avisa que o conteúdo dos dados fica de fora do que ela compara")
	}
}
