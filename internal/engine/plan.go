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

// O instante agendado sai da inversao da integral da taxa, e nao de um
// contador incremental: contador acumula erro e faz a taxa efetiva derivar
// da taxa declarada ao longo de uma execucao longa.
func CompilePlan(plan scenario.LoadPlan) Plan {
	compiled := Plan{}
	var start time.Duration
	var accumulated float64

	for _, phase := range plan.Phases {
		f := compiledPhase{
			initialRate:   phase.InitialRate(),
			finalRate:     phase.FinalRate(),
			duration:      phase.For,
			start:         start,
			acumuladoAtes: accumulated,
		}
		accumulated += countInPhase(f, phase.For)
		start += phase.For
		compiled.phases = append(compiled.phases, f)
	}

	compiled.duration = start
	compiled.totalRequests = int64(math.Floor(accumulated))
	return compiled
}

func countInPhase(f compiledPhase, until time.Duration) float64 {
	if f.duration <= 0 {
		return 0
	}
	t := until.Seconds()
	d := f.duration.Seconds()
	slope := (f.finalRate - f.initialRate) / d
	return f.initialRate*t + slope*t*t/2
}

func (p Plan) Duration() time.Duration { return p.duration }

func (p Plan) TotalRequests() int64 { return p.totalRequests }

func (p Plan) RateAt(instant time.Duration) float64 {
	for _, phase := range p.phases {
		if instant < phase.start || instant >= phase.start+phase.duration {
			continue
		}
		elapsed := (instant - phase.start).Seconds()
		slope := (phase.finalRate - phase.initialRate) / phase.duration.Seconds()
		return phase.initialRate + slope*elapsed
	}
	return 0
}

func (p Plan) InstantOf(index int64) time.Duration {
	target := float64(index)
	for position, phase := range p.phases {
		inPhase := target - phase.acumuladoAtes
		last := position == len(p.phases)-1
		if !last && inPhase >= countInPhase(phase, phase.duration) {
			continue
		}
		instant := phase.start + instantInPhase(phase, inPhase)
		if instant > p.duration {
			return p.duration
		}
		return instant
	}
	return p.duration
}

func instantInPhase(f compiledPhase, count float64) time.Duration {
	if count <= 0 {
		return 0
	}
	d := f.duration.Seconds()
	slope := (f.finalRate - f.initialRate) / d
	var seconds float64
	if math.Abs(slope) < 1e-12 {
		seconds = count / f.initialRate
	} else {
		a := slope / 2
		b := f.initialRate
		seconds = (-b + math.Sqrt(b*b+4*a*count)) / (2 * a)
	}
	return time.Duration(seconds * float64(time.Second))
}
