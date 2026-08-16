package selfcheck

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	protocolohttp "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/testsupport"
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
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func runOpenModel(t *testing.T, address string) metrics.Document {
	t.Helper()
	c := scenario.Spec{
		Name:   "auto-validacao de medição",
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

	options := engine.DefaultOptions()
	options.Version = "teste"
	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	return m.Execute(context.Background())
}

func TestMeasurementReflectsTargetFreeze(t *testing.T) {
	server := startFreezingTarget(t)
	document := runOpenModel(t, server.Address())

	pisoEsperado := float64(freezeDuration.Milliseconds()) / 2
	if document.Overall.Latency.P99 < pisoEsperado {
		t.Fatalf("p99 corrigida = %.1f ms; o alvo congelou por %s e a medição precisa refletir isso (piso %.1f ms)",
			document.Overall.Latency.P99, freezeDuration, pisoEsperado)
	}
	if document.Overall.Latency.Max < float64(freezeDuration.Milliseconds())*0.9 {
		t.Errorf("máximo = %.1f ms, esperado próximo de %s", document.Overall.Latency.Max, freezeDuration)
	}
	if document.Overall.Count == 0 {
		t.Fatal("nenhuma requisição concluida")
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
		t.Fatalf("degradação do alvo não foi detectada; avisos: %+v", document.Warnings)
	}
}

func TestClosedLoopWouldHideThePauseOpenModelShows(t *testing.T) {
	openModelServer := startFreezingTarget(t)
	document := runOpenModel(t, openModelServer.Address())

	closedModelServer := startFreezingTarget(t)
	closed := RunClosedLoop(context.Background(), closedModelServer.Address(), "/pedido", runDuration)

	openP99 := document.Overall.Latency.P99
	closedP99 := closed.P99

	t.Logf("mesma pausa de %s no mesmo alvo:", freezeDuration)
	t.Logf("  modelo aberto (braunrate): p99 %.1f ms sobre %d amostras", openP99, document.Overall.Count)
	t.Logf("  laço fechado:              p99 %.1f ms sobre %d amostras", closedP99, closed.Samples)
	t.Logf("  omissão coordenada: %.1f ms escondidos pelo laço fechado", openP99-closedP99)

	if closedP99*5 > openP99 {
		t.Fatalf("o experimento não demonstrou omissão coordenada: aberto %.1f ms, fechado %.1f ms",
			openP99, closedP99)
	}
}
