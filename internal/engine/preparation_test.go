package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
)

type slowToOpen struct {
	fakeProtocol
	cost time.Duration
}

func (slow *slowToOpen) Prepare(context.Context, protocol.Request) error {
	time.Sleep(slow.cost)
	return nil
}

// Preparation is setup, not load. If it ran inside the run clock, the first
// scheduled instants would already be in the past and the run would invalidate
// itself for saturation the generator never caused. ADR 0003 §7.
func TestPreparationIsPaidBeforeTheClockStarts(t *testing.T) {
	slow := &slowToOpen{cost: 300 * time.Millisecond}
	slow.name = "falso-caro-de-abrir"
	protocol.Register(slow)

	options := engine.DefaultOptions()
	options.MaxInflight = 1000
	executor, err := engine.New(fakeScenario("falso-caro-de-abrir", 200, time.Second), options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}

	document := executor.Execute(context.Background())

	if !document.Valid() {
		t.Fatalf("a preparacao invalidou a medição: %+v", document.Sanity.Findings)
	}
	for _, warning := range document.Warnings {
		if warning.Kind == "gerador_saturado" {
			t.Fatalf("o custo de abrir virou saturacao do gerador: %s | %s", warning.Message, warning.Evidence)
		}
	}
	if latency := document.Overall.Latency; latency.Max >= float64(slow.cost.Milliseconds()) {
		t.Fatalf("a preparacao caiu dentro da latência: pior caso %.1f ms contra %s de abertura",
			latency.Max, slow.cost)
	}
}
