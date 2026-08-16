package journey

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

const graphqlScenario = `
name: Cobrança via GraphQL
target: %s

auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: ana } }
    capture: { token: $.access_token }

data:
  assinantes:
    file: assinantes.csv
    consume: circular

load:
  profiles:
    - steady: { rate: 50/s, duration: 1s }

scenario:
  - graphql:
      query: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { id status ultimaFatura { id status } } }
      variables: { id: "${assinantes.id}" }
    expect:
      json: { $.data.pedido.status: ABERTO }
    capture:
      faturaId: $.data.pedido.ultimaFatura.id

  - graphql:
      query: |
        mutation PagarFatura($fatura: ID!) { pagarFatura(id: $fatura) { id status } }
      variables: { fatura: "${faturaId}" }
    expect:
      json: { $.data.pagarFatura.status: PAGA }

slo:
  - graphql ConsultarPedido: { p95: < 2s }
  - global: { errors: < 0.1 }
`

func executeGraphQL(t *testing.T, lines string) (metrics.Document, slo.Verdict) {
	t.Helper()
	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "assinantes.csv"), []byte(lines), 0o644); err != nil {
		t.Fatalf("não consegui escrever o csv: %v", err)
	}
	path := filepath.Join(root, "cenario.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(graphqlScenario, server.Address())), 0o644); err != nil {
		t.Fatalf("não consegui escrever o cenário: %v", err)
	}

	c, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenário não carregou: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	options := engine.DefaultOptions()
	options.DataRoot = root
	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	document := m.Execute(context.Background())
	return document, slo.Evaluate(c.SLO, document, nil)
}

func TestGraphQLYieldsOneRowPerOperation(t *testing.T) {
	document, verdict := executeGraphQL(t, "id,nome\n1001,ana\n1002,bruno\n")

	if document.Overall.Errors != 0 {
		t.Fatalf("esperava zero erro, obtive %d: %+v", document.Overall.Errors, document.Steps)
	}
	names := map[string]bool{}
	for _, step := range document.Steps {
		names[step.Name] = true
		if step.Protocol != "graphql" {
			t.Errorf("passo %q com protocolo %q", step.Name, step.Protocol)
		}
	}
	if !names["graphql ConsultarPedido"] || !names["graphql PagarFatura"] {
		t.Fatalf("o relatório precisa de uma linha por operação, obtive %v", names)
	}
	if len(document.Steps) != 2 {
		t.Errorf("consulta e mutation não podem cair na mesma linha: %d passo(s)", len(document.Steps))
	}
	if !verdict.Passed {
		t.Errorf("SLO deveria passar: %s", verdict.Sentence)
	}
}

// The target returns NOT_FOUND with status 200 for identifiers ending in 7,
// which is how a GraphQL error arrives in production.
func TestGraphQLErrorWithStatus200FailsSLO(t *testing.T) {
	document, verdict := executeGraphQL(t, "id,nome\n1007,ana\n")

	query := document.Steps[0]
	if query.ErrorsByClass["graphql"] == 0 {
		t.Fatalf("erro de graphql não foi contado: %+v", query.ErrorsByClass)
	}
	if query.StatusByCode["200"] == 0 {
		t.Errorf("o status HTTP era 200 mesmo: %+v", query.StatusByCode)
	}
	if verdict.Passed {
		t.Error("execução com 100% de erro de graphql não pode passar no SLO")
	}
	if len(document.Steps) != 1 {
		t.Errorf("a mutation não deveria rodar depois do erro: %d passo(s)", len(document.Steps))
	}
}
