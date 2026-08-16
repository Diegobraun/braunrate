package selfcheck

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	protocolohttp "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/testsupport"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const (
	targetLatency  = 2 * time.Millisecond
	freezeInstant  = time.Second
	freezeDuration = time.Second
	runDuration    = 3 * time.Second
	runRate        = 200.0
)

func startFreezingTarget(t *testing.T) *testsupport.Server {
	t.Helper()
	server := testsupport.New(testsupport.Options{
		Latency:     targetLatency,
		FreezeAfter: freezeInstant,
		FreezeFor:   freezeDuration,
	})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func runOpenModel(t *testing.T, address string) metrics.Document {
	t.Helper()
	c := scenario.Spec{
		Name:   "auto-validacao de medicao",
		Target: address,
		Load: scenario.LoadPlan{
			Model:  scenario.OpenArrival,
			Phases: []scenario.Phase{{Kind: scenario.PhaseConstant, To: runRate, For: runDuration}},
		},
		Steps: []scenario.Step{{
			Name:     "consultar pedido",
			Protocol: "http",
			Config:   &protocolohttp.Config{Method: http.MethodGet, Path: "/pedido"},
		}},
	}

	opts := engine.DefaultOptions()
	opts.Version = "teste"
	m, err := engine.New(c, opts)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	return m.Execute(context.Background())
}

// A closed loop only sends the next request after the previous one answers;
// that is how JMeter and Locust measure, and why the pause vanishes from their
// p99.
func runClosedLoop(t *testing.T, address string) *hdrhistogram.Histogram {
	t.Helper()
	histogram := hdrhistogram.New(1, 600_000_000, 3)
	client := &http.Client{Timeout: 30 * time.Second}
	limit := time.Now().Add(runDuration)

	for time.Now().Before(limit) {
		start := time.Now()
		response, err := client.Get(address + "/pedido")
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		_ = histogram.RecordValue(time.Since(start).Microseconds())
	}
	return histogram
}

func TestMeasurementReflectsTargetFreeze(t *testing.T) {
	server := startFreezingTarget(t)
	document := runOpenModel(t, server.Address())

	pisoEsperado := float64(freezeDuration.Milliseconds()) / 2
	if document.Overall.Latency.P99 < pisoEsperado {
		t.Fatalf("p99 corrigida = %.1f ms; o alvo congelou por %s e a medicao precisa refletir isso (piso %.1f ms)",
			document.Overall.Latency.P99, freezeDuration, pisoEsperado)
	}
	if document.Overall.Latency.Max < float64(freezeDuration.Milliseconds())*0.9 {
		t.Errorf("maximo = %.1f ms, esperado proximo de %s", document.Overall.Latency.Max, freezeDuration)
	}
	if document.Overall.Count == 0 {
		t.Fatal("nenhuma requisicao concluida")
	}
	t.Logf("modelo aberto: p50 %.1f ms | p99 %.1f ms | max %.1f ms | n %d",
		document.Overall.Latency.P50, document.Overall.Latency.P99,
		document.Overall.Latency.Max, document.Overall.Count)
}

func TestTargetFreezeIsNotConfusedWithGeneratorSaturation(t *testing.T) {
	server := startFreezingTarget(t)
	document := runOpenModel(t, server.Address())

	for _, warning := range document.Warnings {
		if warning.Kind == "gerador_saturado" {
			t.Fatalf("alvo congelado foi reportado como saturacao do gerador: %s | %s", warning.Message, warning.Evidence)
		}
	}

	found := false
	for _, warning := range document.Warnings {
		if warning.Kind == "alvo_degradado" {
			found = true
			t.Logf("aviso correto: %s | %s", warning.Message, warning.Evidence)
		}
	}
	if !found {
		t.Fatalf("degradacao do alvo nao foi detectada; avisos: %+v", document.Warnings)
	}
}

func TestClosedLoopWouldHideThePauseOpenModelShows(t *testing.T) {
	openModelServer := startFreezingTarget(t)
	document := runOpenModel(t, openModelServer.Address())

	closedModelServer := startFreezingTarget(t)
	histogram := runClosedLoop(t, closedModelServer.Address())

	openP99 := document.Overall.Latency.P99
	closedP99 := float64(histogram.ValueAtQuantile(99)) / 1000

	t.Logf("mesma pausa de %s no mesmo alvo:", freezeDuration)
	t.Logf("  modelo aberto (braunrate): p99 %.1f ms sobre %d amostras", openP99, document.Overall.Count)
	t.Logf("  laco fechado:              p99 %.1f ms sobre %d amostras", closedP99, histogram.TotalCount())
	t.Logf("  omissao coordenada: %.1f ms escondidos pelo laco fechado", openP99-closedP99)

	if closedP99*5 > openP99 {
		t.Fatalf("o experimento nao demonstrou omissao coordenada: aberto %.1f ms, fechado %.1f ms",
			openP99, closedP99)
	}
}
