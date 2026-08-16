package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Diegobraun/braunrate/internal/auth"
	"github.com/Diegobraun/braunrate/internal/correlation"
	"github.com/Diegobraun/braunrate/internal/data"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/runtime"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Options struct {
	Version          string
	MaxInflight      int64
	LateThreshold    time.Duration
	Clock            Clock
	DataRoot         string
	OnProgress       func(metrics.Instantaneo, float64, time.Duration)
	ProgressInterval time.Duration
	OnStep           func(Observation)
}

func DefaultOptions() Options {
	return Options{
		Version:          "0.2.0",
		MaxInflight:      20000,
		LateThreshold:    10 * time.Millisecond,
		Clock:            SystemClock{},
		ProgressInterval: time.Second,
	}
}

// Observacao existe para o modo de depuracao: um usuario, uma iteracao, tudo
// visivel. O caminho de execucao e o mesmo da carga.
type Observation struct {
	Step     string
	Key      string
	Config   protocol.Config
	Response protocol.Response
	Captured map[string]string
	Vars     map[string]string
	Failures []string
	Class    protocol.ErrorClass
	Duration time.Duration
}

type Motor struct {
	scenario      scenario.Scenario
	plan          Plan
	opts          Options
	sources       []data.Source
	authenticator *auth.Manager
	collector     atomic.Pointer[metrics.Collector]
}

func New(c scenario.Scenario, opts Options) (*Motor, error) {
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.LateThreshold <= 0 {
		opts.LateThreshold = 10 * time.Millisecond
	}
	if opts.MaxInflight <= 0 {
		opts.MaxInflight = 20000
	}

	m := &Motor{scenario: c, plan: CompilePlan(c.Load), opts: opts}

	for _, source := range c.Data {
		open, err := data.Open(source, opts.DataRoot)
		if err != nil {
			return nil, err
		}
		m.sources = append(m.sources, open)
	}

	if c.Auth != nil {
		m.authenticator = auth.New(*c.Auth, m.runAuthStep, opts.Clock)
	}
	return m, nil
}

func (m *Motor) Plan() Plan { return m.plan }

// Depurar roda uma unica iteracao pelo mesmo caminho da carga: mesmo motor,
// mesma resolucao de variavel, mesma captura. So a carga muda.
func (m *Motor) Debug(ctx context.Context) ([]Observation, map[string]string, error) {
	values := runtime.New(0, 0, m.scenario.Vars)

	for _, source := range m.sources {
		record, err := source.Next(0)
		if err != nil {
			return nil, values.Values(), err
		}
		values.SetAll(record)
	}

	var authHeader [2]string
	if m.authenticator != nil {
		name, value, err := m.authenticator.Header(ctx, values)
		if err != nil {
			return nil, values.Values(), err
		}
		authHeader = [2]string{name, value}
	}

	var observations []Observation
	instant := m.opts.Clock.Now()
	for _, step := range m.scenario.Steps {
		sample, observation := m.runStep(ctx, step, instant, values, authHeader)
		observations = append(observations, observation)
		instant = sample.FinishedAt
		if sample.Class != protocol.Success {
			break
		}
	}
	return observations, values.Values(), nil
}

func (m *Motor) Scenario() scenario.Scenario { return m.scenario }

func (m *Motor) DataRoot() string { return filepath.Clean(m.opts.DataRoot) }

func (m *Motor) Execute(ctx context.Context) metrics.Document {
	clock := m.opts.Clock
	start := clock.Now()
	if err := m.prepareProtocols(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	collector := metrics.NewCollector(start, m.opts.LateThreshold)
	m.collector.Store(collector)

	var inflight atomic.Int64
	var group sync.WaitGroup
	stopProgress := make(chan struct{})

	if m.opts.OnProgress != nil {
		go m.follow(collector, start, stopProgress)
	}

	total := m.plan.TotalRequests()
	for index := int64(0); index < total; index++ {
		if ctx.Err() != nil {
			break
		}
		offset := m.plan.InstantOf(index)
		scheduled := start.Add(offset)
		clock.WaitUntil(scheduled)
		dispatch := clock.Now()

		if inflight.Load() >= m.opts.MaxInflight {
			collector.RecordInflightDrop()
			continue
		}

		current := inflight.Add(1)
		collector.RecordDispatch(scheduled, dispatch, m.plan.RateAt(offset), current)

		group.Add(1)
		go func(virtualUser int64, scheduled time.Time) {
			defer group.Done()
			defer inflight.Add(-1)
			m.runIteration(ctx, virtualUser, scheduled, collector)
		}(index, scheduled)
	}

	group.Wait()
	close(stopProgress)
	end := clock.Now()
	collector.Close()

	return metrics.BuildDocument(collector, metrics.DocumentInput{
		Version:          m.opts.Version,
		Scenario:         m.scenario.Name,
		Target:           m.scenario.Target,
		Model:            string(m.scenario.Load.Model),
		Start:            start,
		End:              end,
		Phases:           m.appliedPhases(),
		MaxInflight:      m.opts.MaxInflight,
		Seeds:            m.seeds(),
		Availability:     m.availability(),
		AuthObtains:      m.authObtains(),
		ScenarioWarnings: m.scenarioWarnings(),
	})
}

// Espera por sondagem mede em degraus do intervalo: o numero e sempre maior ou
// igual ao real, e quem le precisa saber disso antes de comparar com um SLO.
func (m *Motor) scenarioWarnings() []metrics.Warning {
	var warnings []metrics.Warning
	for _, step := range m.scenario.Steps {
		polling, sonda := step.Config.(interface{ PollInterval() time.Duration })
		if !sonda {
			continue
		}
		interval := polling.PollInterval()
		if interval <= 0 {
			continue
		}
		warnings = append(warnings, metrics.Warning{
			Kind:     "espera_por_sondagem",
			Severity: metrics.SeverityLow,
			Message: fmt.Sprintf("o passo %q espera sondando a cada %s: a latencia dele tem essa granularidade e fica maior que a real, nunca menor",
				step.Name, interval),
			Evidence: step.AggregationKey(),
		})
	}
	return warnings
}

// Cada chegada agendada e uma iteracao inteira do cenario: e o que faz o valor
// capturado num passo chegar ao passo seguinte. Se um passo falha, a iteracao
// para — os passos seguintes dependeriam de uma captura que nao aconteceu.
func (m *Motor) runIteration(ctx context.Context, virtualUser int64, scheduled time.Time, collector *metrics.Collector) {
	values := runtime.New(virtualUser, virtualUser, m.scenario.Vars)

	for _, source := range m.sources {
		record, err := source.Next(virtualUser)
		if err != nil {
			collector.Record(metrics.Sample{
				Step: "dados: " + source.Name(), Key: source.Name(), Protocol: "dados",
				ScheduledAt: scheduled, SentAt: scheduled, FinishedAt: m.opts.Clock.Now(),
				Class: protocol.ErrConfig, Detail: err.Error(),
			})
			return
		}
		values.SetAll(record)
	}

	var authHeader [2]string
	if m.authenticator != nil {
		name, value, err := m.authenticator.Header(ctx, values)
		if err != nil {
			collector.Record(metrics.Sample{
				Step: "autenticacao", Key: "autenticacao", Protocol: "http",
				ScheduledAt: scheduled, SentAt: scheduled, FinishedAt: m.opts.Clock.Now(),
				Class: protocol.ErrAuth, Detail: fmt.Sprintf("%v — alvo %s", err, m.scenario.Target),
			})
			return
		}
		authHeader = [2]string{name, value}
	}

	stepInstant := scheduled
	complete := true
	for index, step := range m.scenario.Steps {
		sample, _ := m.runStep(ctx, step, stepInstant, values, authHeader)
		if index == 0 {
			sample.LatencyKind = metrics.CorrectedLatency
		} else {
			sample.LatencyKind = metrics.ServiceLatency
		}
		collector.Record(sample)
		stepInstant = sample.FinishedAt
		if sample.Class != protocol.Success {
			complete = false
			break
		}
	}
	collector.RecordJourney(scheduled, stepInstant, complete)
	collector.RecordUses(values.Uses())
}

func (m *Motor) runStep(ctx context.Context, step scenario.Step, scheduled time.Time,
	values *runtime.Values, authHeader [2]string) (metrics.Sample, Observation) {

	clock := m.opts.Clock
	observation := Observation{Step: step.Name, Key: step.AggregationKey(), Captured: map[string]string{}}
	sample := metrics.Sample{
		Step: step.Name, Key: step.AggregationKey(), Protocol: step.Protocol,
		ScheduledAt: scheduled,
	}

	implementation, exists := protocol.Lookup(step.Protocol)
	if !exists {
		sample.SentAt = clock.Now()
		sample.FinishedAt = sample.SentAt
		sample.Class = protocol.ErrConfig
		sample.Detail = "protocolo nao compilado neste binario"
		return sample, observation
	}

	config := step.Config.Resolve(values.Resolve)
	if authHeader[0] != "" {
		if withHeaders, accepts := config.(protocol.WithHeaders); accepts {
			config = withHeaders.WithHeader(authHeader[0], authHeader[1])
		}
	}
	observation.Config = config

	sample.SentAt = clock.Now()
	response := implementation.Execute(ctx, protocol.Request{
		StepName: step.Name,
		Config:   config,
		URLBase:  m.scenario.Target,
		Vars:     values.Values(),
	})
	sample.FinishedAt = clock.Now()
	sample.Status = response.Status
	sample.Bytes = response.Bytes
	sample.Class = response.Class
	sample.Detail = response.Detail
	observation.Response = response
	observation.Duration = sample.FinishedAt.Sub(sample.SentAt)

	if collector := m.collector.Load(); collector != nil && len(response.Attributes) > 0 {
		collector.RecordUses(response.Attributes)
	}

	if response.Key != "" {
		sample.Key = response.Key
		observation.Key = response.Key
	}

	if sample.Class == protocol.Success {
		if class, detail := m.verificar(step, response, values); class != protocol.Success {
			sample.Class = class
			sample.Detail = detail
			observation.Failures = append(observation.Failures, detail)
		}
	}

	if sample.Class == protocol.Success {
		for _, capture := range step.Captures {
			value, err := correlation.Extract(capture, response)
			if err != nil {
				if capture.Required {
					sample.Class = protocol.ErrCorrelation
					sample.Detail = err.Error()
					observation.Failures = append(observation.Failures, err.Error())
					break
				}
				value = capture.Default
			}
			values.Set(capture.Variable, value)
			observation.Captured[capture.Variable] = value
		}
	}

	observation.Class = sample.Class
	observation.Vars = values.Values()
	return sample, observation
}

func (m *Motor) verificar(step scenario.Step, response protocol.Response, values *runtime.Values) (protocol.ErrorClass, string) {
	for _, check := range step.Checks {
		switch check.Kind {
		case scenario.CheckStatus:
			if response.Status != check.Status {
				return protocol.ErrStatus, fmt.Sprintf("esperava status %d, recebeu %d", check.Status, response.Status)
			}
		case scenario.CheckBody:
			if !bytes.Contains(response.Body, []byte(check.Text)) {
				return protocol.ErrAssertion, fmt.Sprintf("o corpo nao contem %q", check.Text)
			}
		}
	}
	for _, assertion := range step.Assertions {
		if err := correlation.Evaluate(assertion, response, values.Resolve); err != nil {
			return protocol.ErrAssertion, err.Error()
		}
	}
	return protocol.Success, ""
}

func (m *Motor) runAuthStep(ctx context.Context, step scenario.Step, values *runtime.Values) (protocol.Response, error) {
	sample, observation := m.runStep(ctx, step, m.opts.Clock.Now(), values, [2]string{})
	if sample.Class != protocol.Success && sample.Class != protocol.ErrStatus {
		return observation.Response, fmt.Errorf("%s", sample.Detail)
	}
	return observation.Response, nil
}

func (m *Motor) follow(collector *metrics.Collector, start time.Time, stop <-chan struct{}) {
	interval := m.opts.ProgressInterval
	if interval <= 0 {
		interval = time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			elapsed := time.Since(start)
			remaining := m.plan.Duration() - elapsed
			if remaining < 0 {
				remaining = 0
			}
			m.opts.OnProgress(collector.Instantaneo(), m.plan.RateAt(elapsed), remaining)
		}
	}
}

func (m *Motor) appliedPhases() []metrics.AppliedPhase {
	phases := make([]metrics.AppliedPhase, 0, len(m.scenario.Load.Phases))
	for _, phase := range m.scenario.Load.Phases {
		phases = append(phases, metrics.AppliedPhase{
			Kind:       string(phase.Kind),
			From:       phase.InitialRate(),
			To:         phase.FinalRate(),
			DurationMs: phase.For.Milliseconds(),
		})
	}
	return phases
}

func (m *Motor) prepareProtocols(ctx context.Context) error {
	values := runtime.New(0, 0, m.scenario.Vars)
	for _, step := range m.scenario.Steps {
		implementation, exists := protocol.Lookup(step.Protocol)
		if !exists {
			continue
		}
		preparer, needs := implementation.(protocol.Preparable)
		if !needs {
			continue
		}
		err := preparer.Prepare(ctx, protocol.Request{
			StepName: step.Name,
			Config:   step.Config.Resolve(values.Resolve),
			URLBase:  m.scenario.Target,
		})
		if err != nil {
			return fmt.Errorf("nao consegui preparar o passo %q: %w", step.Name, err)
		}
	}
	return nil
}

func (m *Motor) availability() metrics.Availability {
	availability := metrics.Availability{}
	for _, source := range m.sources {
		for name, howMany := range source.Available() {
			availability[name] = howMany
		}
	}
	for _, name := range protocol.Registered() {
		implementation, _ := protocol.Lookup(name)
		if knows, has := implementation.(protocol.WithAvailability); has {
			for key, howMany := range knows.Available() {
				availability[key] = howMany
			}
		}
	}
	// O token e um so por execucao por decisao declarada (ADR 0005), entao um
	// valor unico aqui nao e defeito: a limitacao ja aparece no bloco de
	// ambiente, e repetir como aviso grave abafaria os avisos que importam.
	if m.scenario.Auth != nil && m.scenario.Auth.Obtain != nil {
		for _, capture := range m.scenario.Auth.Obtain.Captures {
			availability[capture.Variable] = 1
		}
	}
	return availability
}

// Semente so existe para fonte sintetica: anotar semente de um CSV sugeriria
// que o arquivo foi sorteado, e a frase do relatorio sobre variedade e a
// variedade observada ([ADR 0007]), nunca a semente declarada.
func (m *Motor) seeds() map[string]int64 {
	seeds := map[string]int64{}
	for _, source := range m.scenario.Data {
		if !source.Synthetic() {
			continue
		}
		seed := source.Seed
		if seed == 0 {
			seed = 1
		}
		seeds[source.Name] = seed
	}
	return seeds
}

func (m *Motor) authObtains() int64 {
	if m.authenticator == nil {
		return 0
	}
	return m.authenticator.Obtains
}
