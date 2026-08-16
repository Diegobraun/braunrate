package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"gopkg.in/yaml.v3"
)

type fakeConfig struct{ key string }

func (c fakeConfig) Protocol() string       { return "falso" }
func (c fakeConfig) AggregationKey() string { return c.key }

func (c fakeConfig) Resolve(func(string) string) protocol.Config { return c }

type fakeProtocol struct {
	name    string
	entrou  chan struct{}
	release chan struct{}
	calls   chan struct{}
}

func (p *fakeProtocol) Name() string { return p.name }

func (p *fakeProtocol) Decode(*yaml.Node) (protocol.Config, error) {
	return fakeConfig{key: "falso"}, nil
}

func (p *fakeProtocol) Close() error { return nil }

func (p *fakeProtocol) Execute(context.Context, protocol.Request) protocol.Response {
	if p.entrou != nil {
		select {
		case p.entrou <- struct{}{}:
		default:
		}
	}
	if p.calls != nil {
		p.calls <- struct{}{}
	}
	if p.release != nil {
		<-p.release
	}
	return protocol.Response{Status: 200, Class: protocol.Success, Bytes: 7}
}

func registerFake(t *testing.T, name string, fake *fakeProtocol) {
	t.Helper()
	fake.name = name
	protocol.Register(fake)
}

func fakeScenario(name string, rate float64, duration time.Duration) scenario.Spec {
	return scenario.Spec{
		Name:   "teste",
		Target: "http://alvo.invalido",
		Load: scenario.LoadPlan{
			Model:  scenario.OpenArrival,
			Phases: []scenario.Phase{{Kind: scenario.PhaseConstant, To: rate, For: duration}},
		},
		Steps: []scenario.Step{{
			Name:     "passo falso",
			Protocol: name,
			Config:   fakeConfig{key: "falso"},
		}},
	}
}

func TestDispatchFollowsScheduledInstantWithInjectedClock(t *testing.T) {
	registerFake(t, "falso-pontual", &fakeProtocol{})
	clock := engine.NewVirtualClock(time.Unix(1_700_000_000, 0))

	opts := engine.DefaultOptions()
	opts.Clock = clock
	opts.MaxInflight = 1000

	m, err := engine.New(fakeScenario("falso-pontual", 100, time.Second), opts)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := m.Execute(context.Background())

	if document.Scheduling.Sent != 100 {
		t.Errorf("enviadas = %d, esperado 100", document.Scheduling.Sent)
	}
	if document.Scheduling.LateDispatches != 0 {
		t.Errorf("despachos atrasados = %d, esperado 0 com relogio virtual", document.Scheduling.LateDispatches)
	}
	if document.Overall.Count != 100 {
		t.Errorf("contagem = %d, esperado 100", document.Overall.Count)
	}
	if !document.Valid() {
		t.Errorf("resultado deveria ser valido, avisos: %+v", document.Warnings)
	}
}

func TestInflightLimitDropsAndInvalidatesResult(t *testing.T) {
	fake := &fakeProtocol{entrou: make(chan struct{}, 1), release: make(chan struct{})}
	registerFake(t, "falso-preso", fake)

	opts := engine.DefaultOptions()
	opts.Clock = engine.NewVirtualClock(time.Unix(1_700_000_000, 0))
	opts.MaxInflight = 1

	finished := make(chan struct{})
	var document = make(chan any, 1)
	go func() {
		m, err := engine.New(fakeScenario("falso-preso", 3, time.Second), opts)
		if err != nil {
			panic(err)
		}
		document <- m.Execute(context.Background())
		close(finished)
	}()

	<-fake.entrou
	close(fake.release)
	<-finished

	result := (<-document).(interface {
		Valid() bool
	})

	if result.Valid() {
		t.Fatal("resultado com descarte por limite de voo nao pode ser valido")
	}
}

func TestStatusCheckClassifiesError(t *testing.T) {
	registerFake(t, "falso-status", &fakeProtocol{})
	c := fakeScenario("falso-status", 10, time.Second)
	c.Steps[0].Checks = []scenario.Check{{Kind: scenario.CheckStatus, Status: 201}}

	opts := engine.DefaultOptions()
	opts.Clock = engine.NewVirtualClock(time.Unix(1_700_000_000, 0))

	m, err := engine.New(c, opts)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := m.Execute(context.Background())

	if document.Overall.Errors != document.Overall.Count {
		t.Fatalf("esperava todas as requisicoes como erro de status, obtido %d de %d",
			document.Overall.Errors, document.Overall.Count)
	}
	if document.Steps[0].ErrorsByClass["status"] == 0 {
		t.Errorf("erro nao foi classificado como status: %+v", document.Steps[0].ErrorsByClass)
	}
}
