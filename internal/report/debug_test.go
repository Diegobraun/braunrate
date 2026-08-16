package report_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/report"
)

// A class the report has no phrase for printed "problema:" and nothing after
// it, which is worse than printing the raw name of the class.
func TestEveryErrorClassPrintsAProblemInDebug(t *testing.T) {
	for _, class := range protocol.ErrorClasses {
		var out strings.Builder
		observation := engine.Observation{Step: "publicar pedido", Class: class}
		if err := report.Debug(&out, 1, observation, false); err != nil {
			t.Fatalf("depuracao nao escreveu: %v", err)
		}

		line := problemLine(out.String())
		if line == "" {
			t.Fatalf("a classe %q nao produziu linha de problema:\n%s", class, out.String())
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "problema:")) == "" {
			t.Fatalf("a classe %q produziu problema vazio:\n%s", class, out.String())
		}
	}
}

// The class says what kind of failure it was; the detail says which broker
// refused and why. Without it debug prints "nao consegui autenticar" and stops
// exactly where the person needs to know the user and the kind of credential.
func TestDebugShowsTheDetailOfTheFailure(t *testing.T) {
	var out strings.Builder
	observation := engine.Observation{
		Step:     "publicar pedido",
		Class:    protocol.ErrAuth,
		Response: protocol.Response{Detail: "o broker recusou a credencial (scram_sha512, usuario ana + TLS com CA propria)"},
	}
	if err := report.Debug(&out, 1, observation, false); err != nil {
		t.Fatalf("depuracao nao escreveu: %v", err)
	}

	if !strings.Contains(out.String(), "usuario ana") {
		t.Fatalf("a depuracao nao diz quem foi recusado:\n%s", out.String())
	}
}

func problemLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "problema:") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
