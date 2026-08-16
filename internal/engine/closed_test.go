package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
)

func closedRun(t *testing.T, users int, respondIn time.Duration) metrics.Document {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(respondIn)
		_, _ = fmt.Fprint(w, `{"id":"1"}`)
	}))
	t.Cleanup(server.Close)

	spec, err := scenario.Parse([]byte(fmt.Sprintf(`
nome: Laco fechado
alvo: %s

carga:
  modelo: fechado
  usuarios: %d
  duracao: 600ms

cenario:
  - http: GET /pedidos
    nome: consultar
`, server.URL, users)))
	if err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}

	executor, err := engine.New(spec, engine.DefaultOptions())
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	return executor.Execute(context.Background())
}

// The closed loop has no scheduled instant, so it has no corrected latency to
// report. Writing the field with the response time in it would claim a
// correction that never happened, and writing zero would claim there was no
// delay to hide — which is the one thing this model cannot know.
func TestClosedLoopDocumentHasNoCorrectedLatencyAnywhere(t *testing.T) {
	document := closedRun(t, 4, 5*time.Millisecond)

	if document.Overall.Count == 0 {
		t.Fatal("a execucao em laco fechado nao produziu amostra nenhuma")
	}
	if document.Overall.Latency != (metrics.Distribution{}) {
		t.Fatalf("o global veio com latencia corrigida: %+v", document.Overall.Latency)
	}
	if document.Journey.Latency != (metrics.Distribution{}) {
		t.Fatalf("a jornada veio com latencia corrigida: %+v", document.Journey.Latency)
	}
	for _, step := range document.Steps {
		if step.Latency != (metrics.Distribution{}) {
			t.Fatalf("o passo %q veio com latencia corrigida: %+v", step.Name, step.Latency)
		}
		if step.LatencyKind != string(metrics.ServiceLatency) {
			t.Fatalf("o passo %q se declarou %q em laco fechado", step.Name, step.LatencyKind)
		}
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("serializar falhou: %v", err)
	}
	if bytes.Contains(encoded, []byte("latencia_corrigida")) {
		t.Fatal("o JSON de uma execucao em laco fechado ainda tem o campo latencia_corrigida")
	}
	if document.Journey.ServiceLatency.Samples == 0 {
		t.Fatal("sem latencia corrigida e sem latencia de servico, a jornada ficou sem numero nenhum")
	}
}

// The warning is not conditional on anything: it is a property of the model, so
// it has to survive a run where every number looks good.
func TestClosedLoopWarningIsInEveryOutput(t *testing.T) {
	document := closedRun(t, 3, time.Millisecond)

	warning, closed := metrics.ClosedLoopWarning(document)
	if !closed {
		t.Fatal("o documento nao se reconheceu como laco fechado")
	}
	if !strings.Contains(warning, "3 usuarios") {
		t.Fatalf("o aviso nao diz quantos usuarios: %q", warning)
	}

	var terminal bytes.Buffer
	if err := report.Summary(&terminal, document, slo.Verdict{}); err != nil {
		t.Fatalf("resumo falhou: %v", err)
	}
	if !strings.Contains(terminal.String(), warning) {
		t.Fatalf("o terminal nao trouxe o aviso de laco fechado:\n%s", terminal.String())
	}

	var page bytes.Buffer
	if err := report.HTML(&page, document); err != nil {
		t.Fatalf("html falhou: %v", err)
	}
	if !strings.Contains(page.String(), warning) {
		t.Fatal("o HTML nao trouxe o aviso de laco fechado")
	}
}

// Same target, same steps: the only difference is who decides when to ask. The
// open model keeps its correction; without the model check both would be
// labelled the same way.
func TestOpenModelKeepsTheCorrectedLatencyTheClosedOneDrops(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"id":"1"}`)
	}))
	defer server.Close()

	spec, err := scenario.Parse([]byte(fmt.Sprintf(`
nome: Aberto
alvo: %s
carga:
  perfis:
    - patamar: { taxa: 50/s, durante: 400ms }
cenario:
  - http: GET /pedidos
    nome: consultar
`, server.URL)))
	if err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	executor, err := engine.New(spec, engine.DefaultOptions())
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := executor.Execute(context.Background())

	if document.Overall.Latency == (metrics.Distribution{}) {
		t.Fatal("o modelo aberto perdeu a latencia corrigida junto com o fechado")
	}
	if document.Steps[0].LatencyKind != string(metrics.CorrectedLatency) {
		t.Fatalf("o primeiro passo do modelo aberto veio como %q", document.Steps[0].LatencyKind)
	}
}
