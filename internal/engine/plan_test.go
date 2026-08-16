package engine_test

import (
	"math"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func planOf(phases ...scenario.Phase) engine.Plan {
	return engine.CompilePlan(scenario.LoadPlan{Model: scenario.OpenArrival, Phases: phases})
}

func TestConstantRateSchedulesAtFixedInterval(t *testing.T) {
	plan := planOf(scenario.Phase{Kind: scenario.PhaseSteady, To: 100, For: 2 * time.Second})

	if total := plan.TotalRequests(); total != 200 {
		t.Fatalf("total = %d, esperado 200", total)
	}
	for _, index := range []int64{0, 1, 50, 199} {
		expected := time.Duration(index) * 10 * time.Millisecond
		if obtained := plan.InstantOf(index); obtained != expected {
			t.Errorf("InstanteDe(%d) = %v, esperado %v", index, obtained, expected)
		}
	}
}

func TestRampSchedulesByRateIntegral(t *testing.T) {
	plan := planOf(scenario.Phase{Kind: scenario.PhaseRamp, From: 0, To: 100, For: 10 * time.Second})

	if total := plan.TotalRequests(); total != 500 {
		t.Fatalf("total = %d, esperado 500 (area do triangulo)", total)
	}
	if rate := plan.RateAt(5 * time.Second); math.Abs(rate-50) > 0.001 {
		t.Errorf("TaxaEm(5s) = %v, esperado 50", rate)
	}

	half := plan.InstantOf(250)
	expected := 7071 * time.Millisecond
	if difference := half - expected; difference > 5*time.Millisecond || difference < -5*time.Millisecond {
		t.Errorf("metade das requisições em %v, esperado ~%v", half, expected)
	}
}

func TestSequentialPhasesSumDurationAndCount(t *testing.T) {
	plan := planOf(
		scenario.Phase{Kind: scenario.PhaseRamp, From: 0, To: 100, For: 2 * time.Second},
		scenario.Phase{Kind: scenario.PhaseSteady, To: 100, For: 3 * time.Second},
		scenario.Phase{Kind: scenario.PhaseSpike, To: 500, For: 1 * time.Second},
	)

	if duration := plan.Duration(); duration != 6*time.Second {
		t.Errorf("duração = %v, esperado 6s", duration)
	}
	if total := plan.TotalRequests(); total != 100+300+500 {
		t.Errorf("total = %d, esperado 900", total)
	}
	if rate := plan.RateAt(5*time.Second + 500*time.Millisecond); rate != 500 {
		t.Errorf("taxa no pico = %v, esperado 500", rate)
	}
}

func TestInstantsAreMonotonic(t *testing.T) {
	plan := planOf(
		scenario.Phase{Kind: scenario.PhaseRamp, From: 10, To: 200, For: 3 * time.Second},
		scenario.Phase{Kind: scenario.PhaseSteady, To: 200, For: 2 * time.Second},
	)

	previous := time.Duration(-1)
	for index := int64(0); index < plan.TotalRequests(); index++ {
		instant := plan.InstantOf(index)
		if instant < previous {
			t.Fatalf("instante %d (%v) veio antes do anterior (%v)", index, instant, previous)
		}
		if instant > plan.Duration() {
			t.Fatalf("instante %d (%v) passou da duração do plano (%v)", index, instant, plan.Duration())
		}
		previous = instant
	}
}
