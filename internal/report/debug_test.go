package report_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/report"
)

// A class the report has no phrase for printed "problem:" and nothing after
// it, which is worse than printing the raw name of the class.
func TestEveryErrorClassPrintsAProblemInDebug(t *testing.T) {
	for _, class := range protocol.ErrorClasses {
		var out strings.Builder
		observation := engine.Observation{Step: "publicar pedido", Class: class}
		if err := report.Debug(&out, 1, observation, false); err != nil {
			t.Fatalf("depuracao não escreveu: %v", err)
		}

		line := problemLine(out.String())
		if line == "" {
			t.Fatalf("a classe %q não produziu linha de problema:\n%s", class, out.String())
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "problem:")) == "" {
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
		Response: protocol.Response{Detail: "o broker recusou a credencial (scram_sha512, usuário ana + TLS com CA própria)"},
	}
	if err := report.Debug(&out, 1, observation, false); err != nil {
		t.Fatalf("depuracao não escreveu: %v", err)
	}

	if !strings.Contains(out.String(), "usuário ana") {
		t.Fatalf("a depuracao não diz quem foi recusado:\n%s", out.String())
	}
}

func problemLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "problem:") {
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
		t.Fatalf("resumo não escreveu: %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "passo que nunca rodou") {
		t.Fatalf("o passo que nunca rodou sumiu do relatório:\n%s", text)
	}
	if !strings.Contains(text, "never got to run") {
		t.Fatalf("nada explica o traço na linha do passo:\n%s", text)
	}
	if !strings.Contains(text, "status 401") {
		t.Fatalf("a linha de erro não diz qual status:\n%s", text)
	}
	if !strings.Contains(text, document.Steps[0].Name) {
		t.Fatalf("a linha de erro não diz em qual passo:\n%s", text)
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
		t.Fatalf("resumo não escreveu: %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "No step recorded a sample") {
		t.Fatalf("a tabela vazia saiu sem explicacao:\n%s", text)
	}
	if strings.Contains(text, "requests      half") {
		t.Fatalf("o cabecalho da tabela foi impresso sem linha nenhuma:\n%s", text)
	}
	if !strings.Contains(text, "braunrate debug") {
		t.Fatalf("nada diz qual e o próximo passo:\n%s", text)
	}
}

// A captura de um token e uma credencial como o cabecalho que ela alimenta, e o
// depurar sai colado em ticket.
func TestTheDebugVariableListCutsACapturedCredential(t *testing.T) {
	var out bytes.Buffer
	err := report.IterationVars(&out, map[string]string{
		"token":     "eyJhbGciOiJIUzI1NiJ9.super-secret",
		"orders.id": "2ad3f3c5",
	})
	if err != nil {
		t.Fatalf("não consegui escrever a lista: %v", err)
	}
	printed := out.String()
	if strings.Contains(printed, "super-secret") {
		t.Errorf("o token saiu inteiro na depuração:\n%s", printed)
	}
	if !strings.Contains(printed, "orders.id = 2ad3f3c5") {
		t.Errorf("a variável comum foi cortada junto:\n%s", printed)
	}
}
