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

// B8 and B9 of the audit: a step that never ran vanished from the table, and
// the error section said "status HTTP inesperado 60" without saying which
// status or in which step — both were already in the JSON.
func TestReportShowsTheStepThatNeverRanAndNamesTheError(t *testing.T) {
	document := sampleDocument()
	document.Run.DeclaredSteps = []string{"consultar pedido", "pagar fatura", "passo que nunca rodou"}
	document.Steps[0].Errors = 40
	document.Steps[0].ErrorsByClass = map[string]int64{"status": 40}
	document.Steps[0].Details = map[string]int64{"status 401": 40}

	var out strings.Builder
	if err := report.Summary(&out, document, document.SLO); err != nil {
		t.Fatalf("resumo nao escreveu: %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "passo que nunca rodou") {
		t.Fatalf("o passo que nunca rodou sumiu do relatorio:\n%s", text)
	}
	if !strings.Contains(text, "nunca chegou a executar") {
		t.Fatalf("nada explica o traco na linha do passo:\n%s", text)
	}
	if !strings.Contains(text, "status 401") {
		t.Fatalf("a linha de erro nao diz qual status:\n%s", text)
	}
	if !strings.Contains(text, document.Steps[0].Name) {
		t.Fatalf("a linha de erro nao diz em qual passo:\n%s", text)
	}
}

// B15 of the audit: the header printed over an empty table says "there is
// nothing here" in the least useful way there is.
func TestEmptyStepTableSaysWhatHappenedInsteadOfPrintingAHeader(t *testing.T) {
	document := sampleDocument()
	document.Steps = nil
	document.Run.DeclaredSteps = nil

	var out strings.Builder
	if err := report.Summary(&out, document, document.SLO); err != nil {
		t.Fatalf("resumo nao escreveu: %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "Nenhum passo registrou amostra") {
		t.Fatalf("a tabela vazia saiu sem explicacao:\n%s", text)
	}
	if strings.Contains(text, "requisicoes    metade") {
		t.Fatalf("o cabecalho da tabela foi impresso sem linha nenhuma:\n%s", text)
	}
	if !strings.Contains(text, "braunrate debug") {
		t.Fatalf("nada diz qual e o proximo passo:\n%s", text)
	}
}
