package engine

import (
	"math"
	"time"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

type compiledPhase struct {
	initialRate   float64
	finalRate     float64
	duration      time.Duration
	start         time.Duration
	acumuladoAtes float64
}

type Plan struct {
	phases        []compiledPhase
	duration      time.Duration
	totalRequests int64
}

// InstantOf derives the scheduled instant by inverting the integral of the
// rate function rather than stepping a counter: a counter accumulates rounding
// error and makes the effective rate drift over a long run.
func CompilePlan(plan scenario.LoadPlan) Plan {
	if plan.Closed() {
		return Plan{duration: plan.For}
	}
	compiled := Plan{}
	var start time.Duration
	var accumulated float64

	for _, phase := range plan.Phases {
		current := compiledPhase{
			initialRate:   phase.InitialRate(),
			finalRate:     phase.FinalRate(),
			duration:      phase.For,
			start:         start,
			acumuladoAtes: accumulated,
		}
		accumulated += countInPhase(current, phase.For)
		start += phase.For
		compiled.phases = append(compiled.phases, current)
	}

	compiled.duration = start
	compiled.totalRequests = int64(math.Floor(accumulated))
	return compiled
}

func countInPhase(phase compiledPhase, until time.Duration) float64 {
	if phase.duration <= 0 {
		return 0
	}
	elapsed := until.Seconds()
	span := phase.duration.Seconds()
	slope := (phase.finalRate - phase.initialRate) / span
	return phase.initialRate*elapsed + slope*elapsed*elapsed/2
}

func (plan Plan) Duration() time.Duration { return plan.duration }

func (plan Plan) TotalRequests() int64 { return plan.totalRequests }

func (plan Plan) RateAt(instant time.Duration) float64 {
	for _, phase := range plan.phases {
		if instant < phase.start || instant >= phase.start+phase.duration {
			continue
		}
		elapsed := (instant - phase.start).Seconds()
		slope := (phase.finalRate - phase.initialRate) / phase.duration.Seconds()
		return phase.initialRate + slope*elapsed
	}
	return 0
}

func (plan Plan) InstantOf(index int64) time.Duration {
	target := float64(index)
	for position, phase := range plan.phases {
		inPhase := target - phase.acumuladoAtes
		last := position == len(plan.phases)-1
		if !last && inPhase >= countInPhase(phase, phase.duration) {
			continue
		}
		instant := phase.start + instantInPhase(phase, inPhase)
		if instant > plan.duration {
			return plan.duration
		}
		return instant
	}
	return plan.duration
}

func instantInPhase(phase compiledPhase, count float64) time.Duration {
	if count <= 0 {
		return 0
	}
	span := phase.duration.Seconds()
	slope := (phase.finalRate - phase.initialRate) / span
	var seconds float64
	if math.Abs(slope) < 1e-12 {
		seconds = count / phase.initialRate
	} else {
		quadratic := slope / 2
		linear := phase.initialRate
		seconds = (-linear + math.Sqrt(linear*linear+4*quadratic*count)) / (2 * quadratic)
	}
	return time.Duration(seconds * float64(time.Second))
}
