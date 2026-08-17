package journey

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

const scenarioModel = `
name: Jornada de cobrança
target: %s

variables:
  user: ${USUARIO_DE_TESTE:-ana}

auth:
  type: token
  obtain:
    http:
      method: POST
      path: /auth/token
      body: { user: "${user}", password: "${SENHA:-segredo}" }
    capture: { token: $.access_token }
  refreshAfter: 25m

data:
  assinantes:
    file: assinantes.csv
    consume: circular

load:
  profiles:
    - steady: { rate: 100/s, duration: 2s }

scenario:
  - http: GET /orders/${assinantes.id}
    name: look up order
    expect:
      status: 200
      json: { $.lastInvoice.status: OPEN }
    capture:
      invoiceId: $.lastInvoice.id

  - name: pay invoice
    http:
      method: POST
      path: /invoices/${invoiceId}/pay
      body: { value: 199.90 }
    expect:
      status: 200
      json: { $.status: PAID }

slo:
  - look up order: { p95: < 2s }
  - global: { errors: < 0.1 }
`

func prepareScenario(t *testing.T, address string) (scenario.Spec, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "assinantes.csv"),
		[]byte("id,nome\n1001,ana\n1002,bruno\n1003,carla\n"), 0o644); err != nil {
		t.Fatalf("não consegui escrever o csv: %v", err)
	}

	content := fmt.Sprintf(scenarioModel, address)
	path := filepath.Join(root, "jornada.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("não consegui escrever o cenário: %v", err)
	}

	c, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenário não carregou: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	return c, root
}

func execute(t *testing.T) (metrics.Document, slo.Verdict) {
	t.Helper()
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	c, root := prepareScenario(t, server.Address())
	options := engine.DefaultOptions()
	options.DataRoot = root
	options.Version = "teste"

	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	document := m.Execute(context.Background())
	return document, slo.Evaluate(c.SLO, document, nil)
}

func TestJourneyWithAuthCorrelationAndDataWorksEndToEnd(t *testing.T) {
	document, verdict := execute(t)

	if document.Overall.Errors != 0 {
		t.Fatalf("esperava zero erro, obtive %d: %+v", document.Overall.Errors, document.Steps)
	}
	if len(document.Steps) != 2 {
		t.Fatalf("esperava dois passos no relatório, obtive %d", len(document.Steps))
	}
	for _, step := range document.Steps {
		if step.Count != 200 {
			t.Errorf("passo %q com %d requisições, esperado 200", step.Name, step.Count)
		}
	}
	if !verdict.Passed {
		t.Errorf("SLO deveria passar: %s", verdict.Sentence)
	}
	if document.Run.AuthObtains != 1 {
		t.Errorf("autenticacoes = %d, esperado 1 para a execução inteira", document.Run.AuthObtains)
	}
}

func TestBrokenCorrelationIsCorrelationErrorNotNetworkError(t *testing.T) {
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	c, root := prepareScenario(t, server.Address())
	c.Steps[0].Captures[0].Expression = "$.campo.que.nao.existe"

	options := engine.DefaultOptions()
	options.DataRoot = root
	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	document := m.Execute(context.Background())

	first := document.Steps[0]
	if first.ErrorsByClass["correlation"] == 0 {
		t.Fatalf("esperava erro de correlação, obtive %+v", first.ErrorsByClass)
	}
	if len(document.Steps) != 1 {
		t.Errorf("o passo seguinte não deveria ter rodado sem a captura; passos: %d", len(document.Steps))
	}
	foundExplanation := false
	for detail := range first.Details {
		if strings.Contains(detail, "campo.que.nao.existe") {
			foundExplanation = true
		}
	}
	if !foundExplanation {
		t.Errorf("o relatório precisa dizer qual captura falhou: %+v", first.Details)
	}
}

func TestFailedAssertionSeparatesFunctionalFromSLOFailure(t *testing.T) {
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	c, root := prepareScenario(t, server.Address())
	c.Steps[0].Assertions = []scenario.Assertion{{
		Kind: scenario.AssertJSON, Target: "$.lastInvoice.status",
		Operator: scenario.OpEqual, Value: "PAID",
	}}

	options := engine.DefaultOptions()
	options.DataRoot = root
	m, _ := engine.New(c, options)
	document := m.Execute(context.Background())
	verdict := slo.Evaluate(c.SLO, document, nil)

	if document.Steps[0].ErrorsByClass["assertion"] == 0 {
		t.Fatalf("esperava falha funcional classificada como assercao: %+v", document.Steps[0].ErrorsByClass)
	}
	if verdict.Passed {
		t.Error("o SLO global de erros deveria falhar junto")
	}
	if !strings.Contains(verdict.Sentence, "error rate") {
		t.Errorf("a frase do veredito deveria falar de taxa de erro: %q", verdict.Sentence)
	}
}
