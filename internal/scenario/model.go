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

func (p Step) AggregationKey() string {
	if p.Config == nil {
		return p.Name
	}
	return p.Config.AggregationKey()
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
	Model  ArrivalModel
	Phases []Phase
}

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

func (f Phase) InitialRate() float64 {
	if f.Kind == PhaseRamp {
		return f.From
	}
	return f.To
}

func (f Phase) FinalRate() float64 {
	return f.To
}

func (c Spec) Duration() time.Duration {
	var total time.Duration
	for _, phase := range c.Load.Phases {
		total += phase.For
	}
	return total
}

func (c Spec) FindStep(name string) (Step, error) {
	for _, step := range c.Steps {
		if step.Name == name {
			return step, nil
		}
	}
	return Step{}, fmt.Errorf("passo nao encontrado: %q", name)
}
