package scenario

import (
	"fmt"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
)

const FormatVersion = "1"

type Spec struct {
	FormatVersion string
	Name          string
	Target        string
	Requires      []string
	Vars          map[string]string
	Auth          *Auth
	Data          []DataSource
	Load          LoadPlan
	Steps         []Step
	SLO           []SLORule
}

type Step struct {
	Name       string
	Protocol   string
	Config     protocol.Config
	Checks     []Check
	Captures   []Capture
	Assertions []Assertion
	Line       int
}

func (step Step) AggregationKey() string {
	if step.Config == nil {
		return step.Name
	}
	return step.Config.AggregationKey()
}

type Check struct {
	Kind   CheckKind
	Status int
	Text   string
}

type CheckKind string

const (
	CheckStatus CheckKind = "status"
	CheckBody   CheckKind = "corpo_contem"
)

type ArrivalModel string

const (
	OpenArrival   ArrivalModel = "aberto"
	ClosedArrival ArrivalModel = "fechado"
)

type LoadPlan struct {
	Model     ArrivalModel
	Phases    []Phase
	Users     int
	For       time.Duration
	ThinkTime time.Duration
}

func (plan LoadPlan) Closed() bool { return plan.Model == ClosedArrival }

type PhaseKind string

const (
	PhaseRamp     PhaseKind = "rampa"
	PhasePlateau  PhaseKind = "patamar"
	PhaseSpike    PhaseKind = "pico"
	PhaseConstant PhaseKind = "constante"
)

type Phase struct {
	Kind PhaseKind
	From float64
	To   float64
	For  time.Duration
	Line int
}

func (phase Phase) InitialRate() float64 {
	if phase.Kind == PhaseRamp {
		return phase.From
	}
	return phase.To
}

func (phase Phase) FinalRate() float64 {
	return phase.To
}

func (spec Spec) Duration() time.Duration {
	if spec.Load.Closed() {
		return spec.Load.For
	}
	var total time.Duration
	for _, phase := range spec.Load.Phases {
		total += phase.For
	}
	return total
}

func (spec Spec) FindStep(name string) (Step, error) {
	for _, step := range spec.Steps {
		if step.Name == name {
			return step, nil
		}
	}
	return Step{}, fmt.Errorf("passo nao encontrado: %q", name)
}
