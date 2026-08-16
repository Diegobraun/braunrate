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
nome: Jornada de cobranca
alvo: %s

variaveis:
  usuario: ${USUARIO_DE_TESTE:-ana}

autenticacao:
  tipo: token
  obter:
    http:
      metodo: POST
      caminho: /auth/token
      corpo: { usuario: "${usuario}", senha: "${SENHA:-segredo}" }
    captura: { token: $.access_token }
  renovar_apos: 25m

dados:
  assinantes:
    arquivo: assinantes.csv
    consumo: circular

carga:
  perfis:
    - constante: { taxa: 100/s, durante: 2s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    verificar:
      status: 200
      json: { $.ultimaFatura.status: ABERTA }
    captura:
      faturaId: $.ultimaFatura.id

  - nome: pagar fatura
    http:
      metodo: POST
      caminho: /faturas/${faturaId}/pagar
      corpo: { valor: 199.90 }
    verificar:
      status: 200
      json: { $.status: PAGA }

slo:
  - consultar pedido: { p95: < 2s }
  - global: { erros: < 0.1 }
`

func prepareScenario(t *testing.T, address string) (scenario.Spec, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "assinantes.csv"),
		[]byte("id,nome\n1001,ana\n1002,bruno\n1003,carla\n"), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}

	content := fmt.Sprintf(scenarioModel, address)
	path := filepath.Join(root, "jornada.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	c, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	return c, root
}

func execute(t *testing.T) (metrics.Document, slo.Verdict) {
	t.Helper()
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	c, root := prepareScenario(t, server.Address())
	options := engine.DefaultOptions()
	options.DataRoot = root
	options.Version = "teste"

	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
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
		t.Fatalf("esperava dois passos no relatorio, obtive %d", len(document.Steps))
	}
	for _, step := range document.Steps {
		if step.Count != 200 {
			t.Errorf("passo %q com %d requisicoes, esperado 200", step.Name, step.Count)
		}
	}
	if !verdict.Passed {
		t.Errorf("SLO deveria passar: %s", verdict.Sentence)
	}
	if document.Run.AuthObtains != 1 {
		t.Errorf("autenticacoes = %d, esperado 1 para a execucao inteira", document.Run.AuthObtains)
	}
}

func TestBrokenCorrelationIsCorrelationErrorNotNetworkError(t *testing.T) {
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	c, root := prepareScenario(t, server.Address())
	c.Steps[0].Captures[0].Expression = "$.campo.que.nao.existe"

	options := engine.DefaultOptions()
	options.DataRoot = root
	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := m.Execute(context.Background())

	first := document.Steps[0]
	if first.ErrorsByClass["correlacao"] == 0 {
		t.Fatalf("esperava erro de correlacao, obtive %+v", first.ErrorsByClass)
	}
	if len(document.Steps) != 1 {
		t.Errorf("o passo seguinte nao deveria ter rodado sem a captura; passos: %d", len(document.Steps))
	}
	foundExplanation := false
	for detail := range first.Details {
		if strings.Contains(detail, "campo.que.nao.existe") {
			foundExplanation = true
		}
	}
	if !foundExplanation {
		t.Errorf("o relatorio precisa dizer qual captura falhou: %+v", first.Details)
	}
}

func TestFailedAssertionSeparatesFunctionalFromSLOFailure(t *testing.T) {
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	c, root := prepareScenario(t, server.Address())
	c.Steps[0].Assertions = []scenario.Assertion{{
		Kind: scenario.AssertJSON, Target: "$.ultimaFatura.status",
		Operator: scenario.OpEqual, Value: "PAGA",
	}}

	options := engine.DefaultOptions()
	options.DataRoot = root
	m, _ := engine.New(c, options)
	document := m.Execute(context.Background())
	verdict := slo.Evaluate(c.SLO, document, nil)

	if document.Steps[0].ErrorsByClass["assercao"] == 0 {
		t.Fatalf("esperava falha funcional classificada como assercao: %+v", document.Steps[0].ErrorsByClass)
	}
	if verdict.Passed {
		t.Error("o SLO global de erros deveria falhar junto")
	}
	if !strings.Contains(verdict.Sentence, "taxa de erro") {
		t.Errorf("a frase do veredito deveria falar de taxa de erro: %q", verdict.Sentence)
	}
}
