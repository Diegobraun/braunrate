package dsl_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/dsl"
	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

const gemeoEmYAML = `
name: Consulta de pedido
target: %s

load:
  profiles:
    - steady: { rate: 100/s, duration: 1s }

scenario:
  - http: GET /pedidos/1
    name: consultar pedido
    expect: { status: 200 }

slo:
  - consultar pedido: { p95: < 1s }
`

// Structural equivalence is already locked; this test closes the other half of
// the promise: the scenario written in Go runs on the same engine and comes out
// with the same aggregation keys and the same verdict as its YAML twin.
func TestGoScenarioRunsOnSameEngineWithSameKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ABERTO"}`)
	}))
	t.Cleanup(server.Close)

	doYAML, err := scenario.Parse([]byte(fmt.Sprintf(gemeoEmYAML, server.URL)))
	if err != nil {
		t.Fatalf("yaml não carregou: %v", err)
	}
	daDSL, err := dsl.New("Consulta de pedido").
		Target(server.URL).
		Steady(dsl.PerSecond(100), time.Second).
		Step(dsl.GET("/pedidos/1"), dsl.Name("consultar pedido"), dsl.CheckStatus(200)).
		SLO("consultar pedido", "p95", "< 1s").
		Build()
	if err != nil {
		t.Fatalf("dsl não montou: %v", err)
	}

	pelaYAML := execute(t, doYAML)
	pelaDSL := execute(t, daDSL)

	if len(pelaYAML.Steps) != len(pelaDSL.Steps) {
		t.Fatalf("quantidade de passos: yaml %d, dsl %d", len(pelaYAML.Steps), len(pelaDSL.Steps))
	}
	for index := range pelaYAML.Steps {
		if pelaYAML.Steps[index].Key != pelaDSL.Steps[index].Key {
			t.Errorf("chave de agregação: yaml %q, dsl %q", pelaYAML.Steps[index].Key, pelaDSL.Steps[index].Key)
		}
	}
	if pelaYAML.SLO.Passed != pelaDSL.SLO.Passed {
		t.Errorf("veredito de slo: yaml %v, dsl %v", pelaYAML.SLO.Passed, pelaDSL.SLO.Passed)
	}
	if pelaDSL.Overall.Count == 0 {
		t.Error("o cenário em Go não executou nenhuma requisição")
	}
}

func execute(t *testing.T, c scenario.Spec) metrics.Document {
	t.Helper()
	options := engine.DefaultOptions()
	options.DataRoot = t.TempDir()
	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	return m.Execute(context.Background())
}
