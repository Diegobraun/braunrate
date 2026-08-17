package scenario

import (
	"fmt"
	"github.com/Diegobraun/braunrate/internal/messaging"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
)

const FormatVersion = "2"

type Spec struct {
	FormatVersion string
	Name          string
	Target        string
	Requires      []string
	// Names referenced as ${NOME} that only the environment can supply and that
	// were not in it when the file was read. Not part of what the file says —
	// part of what the machine had — so the DSL does not carry it.
	MissingEnvironment []string
	Vars               map[string]string
	Auth               *Auth
	Messaging          *messaging.Settings
	// TLS of the HTTP target. Kafka and AMQP declare theirs inside 'messaging';
	// HTTP had nowhere to say it, and homologation behind a private CA could not
	// be tested at all.
	TLS   *messaging.TLS
	Data  []DataSource
	Load  LoadPlan
	Steps []Step
	SLO   []SLORule
}

type Step struct {
	Name     string
	Protocol string
	Config   protocol.Config
	// Weight of the alternative in the mix. Zero when the scenario declares no
	// mix, and then every step runs on every iteration. See ADR 0016.
	Weight     int
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
	CheckBody   CheckKind = "bodyContains"
)

type ArrivalModel string

const (
	OpenArrival   ArrivalModel = "open"
	ClosedArrival ArrivalModel = "closed"
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
	PhaseRamp   PhaseKind = "ramp"
	PhaseSteady PhaseKind = "steady"
	PhaseSpike  PhaseKind = "spike"
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
	return Step{}, fmt.Errorf("step not found: %q", name)
}
