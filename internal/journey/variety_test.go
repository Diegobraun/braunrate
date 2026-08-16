package journey

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// A scenario that exercises every place a varying value leaves through: path,
// body, header and GraphQL variable. The frozen-data bug was not an isolated
// auth defect, it was a class of failure the suite never checked, because
// count, latency and errors all stay pretty when the whole load uses the same
// value.
const scenarioWithVariety = `
nome: Variedade
alvo: %s

autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
    captura: { token: $.access_token }

dados:
  assinantes:
    arquivo: assinantes.csv
    consumo: circular

carga:
  perfis:
    - constante: { taxa: 60/s, durante: 1s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: caminho

  - nome: corpo
    http:
      metodo: POST
      caminho: /faturas/pagar
      corpo: { assinante: "${assinantes.id}", regiao: "${assinantes.regiao}" }

  - nome: cabecalho
    http:
      metodo: GET
      caminho: /eco
      cabecalhos: { X-Assinante: "${assinantes.id}" }

  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }
      variaveis: { id: "${assinantes.id}" }
`

type valueCollector struct {
	mu   sync.Mutex
	seen map[string]map[string]struct{}
}

func (valueCollector *valueCollector) note(where, value string) {
	valueCollector.mu.Lock()
	defer valueCollector.mu.Unlock()
	if valueCollector.seen[where] == nil {
		valueCollector.seen[where] = map[string]struct{}{}
	}
	valueCollector.seen[where][value] = struct{}{}
}

func (valueCollector *valueCollector) distinct(where string) []string {
	valueCollector.mu.Lock()
	defer valueCollector.mu.Unlock()
	values := make([]string, 0, len(valueCollector.seen[where]))
	for value := range valueCollector.seen[where] {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func TestEveryDeclaredValueReachesTarget(t *testing.T) {
	received := &valueCollector{seen: map[string]map[string]struct{}{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)

		switch {
		case r.URL.Path == "/auth/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token-de-teste"}`)
			return
		case strings.HasPrefix(r.URL.Path, "/pedidos/"):
			received.note("caminho", strings.TrimPrefix(r.URL.Path, "/pedidos/"))
		case r.URL.Path == "/faturas/pagar":
			var sent struct {
				Subscriber string `json:"assinante"`
				Region     string `json:"regiao"`
			}
			_ = json.Unmarshal(body, &sent)
			received.note("corpo", sent.Subscriber)
			received.note("corpo.regiao", sent.Region)
		case r.URL.Path == "/eco":
			received.note("cabecalho", r.Header.Get("X-Assinante"))
		case r.URL.Path == "/graphql":
			var sent struct {
				Vars map[string]any `json:"variables"`
			}
			_ = json.Unmarshal(body, &sent)
			received.note("graphql", fmt.Sprint(sent.Vars["id"]))
		}
		_, _ = fmt.Fprint(w, `{"data":{"pedido":{"status":"ABERTO"}},"status":"ABERTO"}`)
	}))
	t.Cleanup(server.Close)

	document := runVarietyScenario(t, server.URL,
		"id,regiao\n1001,sul\n1002,norte\n1003,leste\n")

	if document.Overall.Errors != 0 {
		t.Fatalf("esperava zero erro, obtive %d: %+v", document.Overall.Errors, document.Steps)
	}

	expected := []string{"1001", "1002", "1003"}
	for _, where := range []string{"caminho", "corpo", "cabecalho", "graphql"} {
		if got := received.distinct(where); !sameValues(got, expected) {
			t.Errorf("%s recebeu %v, esperava %v: algum valor declarado nunca chegou ao alvo", where, got, expected)
		}
	}
	if got := received.distinct("corpo.regiao"); len(got) != 3 {
		t.Errorf("região recebeu %v, esperava três valores distintos", got)
	}

	byName := map[string]metrics.Variety{}
	for _, variety := range document.Variety {
		byName[variety.Name] = variety
	}
	if variety := byName["assinantes.id"]; variety.Distinct != 3 {
		t.Errorf("o relatório declarou %d valores distintos de assinantes.id, esperava 3", variety.Distinct)
	}
	if variety := byName["assinantes.regiao"]; variety.Distinct != 3 {
		t.Errorf("o relatório declarou %d valores distintos de assinantes.regiao, esperava 3", variety.Distinct)
	}
	for _, warning := range document.Warnings {
		if warning.Kind == "variedade_ausente" {
			t.Errorf("execução com variedade não pode gerar aviso de variedade ausente: %s", warning.Evidence)
		}
	}
}

// The warning that would have caught the frozen-data bug. The source offers
// three values; a run that uses one is a defect, not a choice.
func TestSingleObservedValueBecomesHighSeverityWarning(t *testing.T) {
	document := metrics.Document{
		Variety: []metrics.Variety{
			{Name: "assinantes.id", Distinct: 1, Uses: 2375, Available: 3},
		},
	}
	warnings := metrics.VarietyWarnings(document.Variety)
	if len(warnings) != 1 {
		t.Fatalf("esperava um aviso, obtive %+v", warnings)
	}
	if warnings[0].Severity != metrics.SeverityHigh {
		t.Errorf("gravidade = %q, esperava alta: o resultado não representa a carga declarada", warnings[0].Severity)
	}
	if !strings.Contains(warnings[0].Message, "cache") {
		t.Errorf("a mensagem precisa dizer por que isso engana: %q", warnings[0].Message)
	}
	if !strings.Contains(warnings[0].Evidence, "3 valores disponíveis") {
		t.Errorf("a evidencia precisa comparar o disponível com o usado: %q", warnings[0].Evidence)
	}
}

func TestFixedValueDeclaredInScenarioIsReadingWarningNotDefect(t *testing.T) {
	warnings := metrics.VarietyWarnings([]metrics.Variety{
		{Name: "pedidoFixo", Distinct: 1, Uses: 500, Available: 0},
	})
	if len(warnings) != 1 || warnings[0].Severity != metrics.SeverityMedium {
		t.Fatalf("valor fixo declarado e aviso de leitura, não resultado inválido: %+v", warnings)
	}
}

func TestSingleValueSourceRaisesNoWarning(t *testing.T) {
	warnings := metrics.VarietyWarnings([]metrics.Variety{
		{Name: "assinantes.id", Distinct: 1, Uses: 500, Available: 1},
	})
	if len(warnings) != 0 {
		t.Errorf("quem declarou um valor só não precisa ser avisado disso: %+v", warnings)
	}
}

func runVarietyScenario(t *testing.T, address, csv string) metrics.Document {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "assinantes.csv"), []byte(csv), 0o644); err != nil {
		t.Fatalf("não consegui escrever o csv: %v", err)
	}
	path := filepath.Join(root, "cenario.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(scenarioWithVariety, address)), 0o644); err != nil {
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
	return m.Execute(context.Background())
}

func sameValues(obtained, expected []string) bool {
	if len(obtained) != len(expected) {
		return false
	}
	for index, value := range obtained {
		if value != expected[index] {
			return false
		}
	}
	return true
}
