package journey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// Every id is different and every one of them belongs to the same customer.
// The count of distinct values reads as full coverage, and the run exercised
// one slice of the target — the gap ADR 0007 left open.
const scenarioWithinOneRange = `
nome: Faixa
alvo: %s

dados:
  pedidos:
    arquivo: pedidos.csv
    consumo: circular

carga:
  perfis:
    - constante: { taxa: 60/s, durante: 1s }

cenario:
  - nome: criar pedido
    http:
      metodo: POST
      caminho: /pedidos
      corpo: { id: "${pedidos.id}", total: "${pedidos.total}", cupom: "${pedidos.cupom}" }
`

func TestRangeSaysWhereTheValuesLanded(t *testing.T) {
	document := runRangeScenario(t,
		"id,total,cupom\nCLI-A-001,10,X1\nCLI-A-002,12,X2\nCLI-A-003,11,X3\n")

	byName := map[string]metrics.Variety{}
	for _, variety := range document.Variety {
		byName[variety.Name] = variety
	}

	ids := byName["pedidos.id"]
	if ids.Range == nil || ids.Range.Kind != metrics.PrefixRange {
		t.Fatalf("tres ids diferentes do mesmo cliente precisavam declarar o prefixo comum: %+v", ids.Range)
	}
	if ids.Range.Prefix != "CLI-A-00" {
		t.Fatalf("prefixo comum saiu %q", ids.Range.Prefix)
	}
	if !strings.Contains(ids.Sentence, "CLI-A-00") {
		t.Fatalf("a frase nao contou onde os valores caem: %q", ids.Sentence)
	}

	totals := byName["pedidos.total"]
	if totals.Range == nil || totals.Range.Kind != metrics.NumericRange {
		t.Fatalf("valores numericos precisavam declarar a faixa: %+v", totals.Range)
	}
	if totals.Range.Min != 10 || totals.Range.Max != 12 {
		t.Fatalf("faixa saiu de %v a %v, esperava de 10 a 12", totals.Range.Min, totals.Range.Max)
	}
}

// A body whose fields came empty is a code path production does not see. Every
// number in the report stays healthy while it happens, which is why it needs a
// sentence of its own.
func TestEmptyFieldInTheBodyIsSaidOutLoud(t *testing.T) {
	document := runRangeScenario(t,
		"id,total,cupom\nCLI-A-001,10,\nCLI-A-002,12,\nCLI-A-003,11,\n")

	shape, found := "", false
	for _, variety := range document.Variety {
		if strings.HasPrefix(variety.Name, metrics.BodyShapeName) {
			found = true
			shape = variety.Sentence
			if len(variety.Shapes) != 1 {
				t.Fatalf("esperava uma forma de corpo, obtive %v", variety.Shapes)
			}
			if !variety.Notable() {
				t.Fatalf("forma com campo vazio precisa aparecer no relatorio: %q", shape)
			}
		}
	}
	if !found {
		t.Fatalf("a forma do corpo nao foi medida: %+v", document.Variety)
	}
	if !strings.Contains(shape, "cupom: vazio") {
		t.Fatalf("a forma nao disse qual campo saiu vazio: %q", shape)
	}

	warned := false
	for _, warning := range document.Warnings {
		if warning.Kind == "corpo_com_campo_vazio" {
			warned = true
			if warning.Severity != metrics.SeverityMedium {
				t.Fatalf("campo vazio pode ser proposital; gravidade saiu %q", warning.Severity)
			}
		}
	}
	if !warned {
		t.Fatalf("corpo com campo vazio passou calado: %+v", document.Warnings)
	}
}

// One shape is the normal case. Repeating it for every step would bury the
// lines that do say something, which is the same reason ADR 0007 gives for not
// warning about a source that has a single value.
func TestASingleWholeBodyShapeIsNotWorthALine(t *testing.T) {
	document := runRangeScenario(t,
		"id,total,cupom\nCLI-A-001,10,X1\nCLI-A-002,12,X2\n")

	for _, variety := range document.Variety {
		if strings.HasPrefix(variety.Name, metrics.BodyShapeName) && variety.Notable() {
			t.Fatalf("forma unica e sem campo vazio nao devia render linha: %q", variety.Sentence)
		}
	}
}

func runRangeScenario(t *testing.T, csv string) metrics.Document {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pedidos.csv"), []byte(csv), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}
	path := filepath.Join(root, "cenario.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(scenarioWithinOneRange, server.URL)), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	specification, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	if err := specification.Validate(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	options := engine.DefaultOptions()
	options.DataRoot = root
	executor, err := engine.New(specification, options)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	return executor.Execute(context.Background())
}
