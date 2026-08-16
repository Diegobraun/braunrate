package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	OnProgress       ProgressFunc
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

// Observation is what debug mode shows: one user, one iteration, everything
// visible. The execution path is the same one the load uses.
// ProgressFunc is named so the CLI and the server can pass the same thing
// without each spelling the signature out.
type ProgressFunc = func(metrics.Snapshot, float64, time.Duration)

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

type Executor struct {
	scenario      scenario.Spec
	plan          Plan
	options       Options
	sources       []data.Source
	authenticator *auth.Manager
	collector     atomic.Pointer[metrics.Collector]
}

func New(spec scenario.Spec, options Options) (*Executor, error) {
	if options.Clock == nil {
		options.Clock = SystemClock{}
	}
	if options.LateThreshold <= 0 {
		options.LateThreshold = 10 * time.Millisecond
	}
	if options.MaxInflight <= 0 {
		options.MaxInflight = 20000
	}

	executor := &Executor{scenario: spec, plan: CompilePlan(spec.Load), options: options}

	for _, source := range spec.Data {
		open, err := data.Open(source, options.DataRoot)
		if err != nil {
			return nil, err
		}
		executor.sources = append(executor.sources, open)
	}

	if spec.Auth != nil {
		executor.authenticator = auth.New(*spec.Auth, executor.runAuthStep, options.Clock)
	}
	return executor, nil
}

func (executor *Executor) Plan() Plan { return executor.plan }

// Debug runs a single iteration through the load path: same engine, same
// variable resolution, same capture. Only the load is missing.
func (executor *Executor) Debug(runContext context.Context) ([]Observation, map[string]string, error) {
	values := runtime.New(0, 0, executor.scenario.Vars)

	for _, source := range executor.sources {
		record, err := source.Next(0)
		if err != nil {
			return nil, values.Values(), err
		}
		values.SetAll(record)
		if perUse, has := source.(interface {
			PerUse() map[string]func() (string, error)
		}); has {
			for name, generate := range perUse.PerUse() {
				values.SetPerUse(name, generate)
			}
		}
	}

	var authHeader [2]string
	if executor.authenticator != nil {
		name, value, err := executor.authenticator.Header(runContext, values)
		if err != nil {
			return nil, values.Values(), err
		}
		authHeader = [2]string{name, value}
	}

	var observations []Observation
	instant := executor.options.Clock.Now()
	for _, step := range executor.scenario.Steps {
		sample, observation := executor.runStep(runContext, step, instant, values, authHeader)
		observations = append(observations, observation)
		instant = sample.FinishedAt
		if sample.Class != protocol.Success {
			break
		}
	}
	return observations, values.Values(), nil
}

func (executor *Executor) Spec() scenario.Spec { return executor.scenario }

func (executor *Executor) DataRoot() string { return filepath.Clean(executor.options.DataRoot) }

func (executor *Executor) Execute(runContext context.Context) metrics.Document {
	clock := executor.options.Clock
	// Preparation is setup, not load: opening a subscription or paying a TLS and
	// SASL handshake takes time, and starting the clock before it would push the
	// first scheduled instants into the past — the run then invalidates itself
	// for saturation that the generator never caused.
	if err := executor.prepareProtocols(runContext); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	start := clock.Now()

	collector := metrics.NewCollector(start, executor.options.LateThreshold)
	executor.collector.Store(collector)

	stopProgress := make(chan struct{})
	if executor.options.OnProgress != nil {
		go executor.follow(collector, start, stopProgress)
	}

	if executor.closed() {
		executor.driveClosed(runContext, collector, start)
	} else {
		executor.driveOpen(runContext, collector, start)
	}

	close(stopProgress)
	end := clock.Now()
	collector.Close()

	load := executor.scenario.Load
	return metrics.BuildDocument(collector, metrics.DocumentInput{
		Version:          executor.options.Version,
		Spec:             executor.scenario.Name,
		Target:           executor.scenario.Target,
		Model:            string(load.Model),
		Start:            start,
		End:              end,
		Phases:           executor.appliedPhases(),
		Users:            load.Users,
		ThinkTime:        load.ThinkTime,
		MaxInflight:      executor.options.MaxInflight,
		Seeds:            executor.seeds(),
		Availability:     executor.availability(),
		AuthObtains:      executor.authObtains(),
		ScenarioWarnings: executor.scenarioWarnings(),
		Brokers:          scenario.DescribeMessaging(executor.scenario.Messaging),
		DeclaredSteps:    executor.declaredSteps(),
		ConsumerLag:      consumerLag(),
		PlannedDuration:  executor.plan.Duration(),
		PlannedRequests:  executor.plan.TotalRequests(),
	})
}

func (executor *Executor) closed() bool { return executor.scenario.Load.Closed() }

func (executor *Executor) driveOpen(runContext context.Context, collector *metrics.Collector, start time.Time) {
	clock := executor.options.Clock
	var inflight atomic.Int64
	var group sync.WaitGroup

	total := executor.plan.TotalRequests()
	for index := int64(0); index < total; index++ {
		if runContext.Err() != nil {
			break
		}
		offset := executor.plan.InstantOf(index)
		scheduled := start.Add(offset)
		clock.WaitUntil(scheduled)
		dispatch := clock.Now()

		if inflight.Load() >= executor.options.MaxInflight {
			collector.RecordInflightDrop()
			continue
		}

		current := inflight.Add(1)
		collector.RecordDispatch(scheduled, dispatch, executor.plan.RateAt(offset), current)

		group.Add(1)
		go func(index int64, scheduled time.Time) {
			defer group.Done()
			defer inflight.Add(-1)
			executor.runIteration(runContext, index, index, scheduled, collector)
		}(index, scheduled)
	}
	group.Wait()
}

// Each user only asks again after the previous answer arrived. Nothing is
// scheduled, so nothing can be late — the rate is whatever the target allows,
// which is the property that makes this model hide a freeze.
func (executor *Executor) driveClosed(runContext context.Context, collector *metrics.Collector, start time.Time) {
	clock := executor.options.Clock
	load := executor.scenario.Load
	deadline := start.Add(load.For)

	var iterations atomic.Int64
	var group sync.WaitGroup
	for user := 0; user < load.Users; user++ {
		group.Add(1)
		go func(user int64) {
			defer group.Done()
			for runContext.Err() == nil && clock.Now().Before(deadline) {
				executor.runIteration(runContext, user, iterations.Add(1)-1, clock.Now(), collector)
				if load.ThinkTime > 0 {
					clock.WaitUntil(clock.Now().Add(load.ThinkTime))
				}
			}
		}(int64(user))
	}
	group.Wait()
}

func (executor *Executor) declaredSteps() []string {
	names := make([]string, 0, len(executor.scenario.Steps))
	for _, step := range executor.scenario.Steps {
		names = append(names, step.Name)
	}
	return names
}

// Polling measures in steps of the poll interval: the number is always greater
// than or equal to the real one, and the reader has to know that before
// comparing it against an SLO.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimPrefix(line, "Atencao: ")
}

func (executor *Executor) scenarioWarnings() []metrics.Warning {
	var warnings []metrics.Warning
	for _, warning := range scenario.FixedStepWarnings(executor.scenario) {
		warnings = append(warnings, metrics.Warning{
			Kind:     "passo_sem_variacao",
			Severity: metrics.SeverityLow,
			Message:  firstLine(warning),
			Evidence: "nenhum ${} no passo, entao ele nao entra na variedade observada",
		})
	}
	for _, step := range executor.scenario.Steps {
		polling, polls := step.Config.(interface{ PollInterval() time.Duration })
		if !polls {
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

// Each scheduled arrival is a whole scenario iteration: that is what carries a
// value captured in one step into the next. If a step fails the iteration
// stops, because the following steps would depend on a capture that never
// happened.
func (executor *Executor) runIteration(runContext context.Context, virtualUser, iteration int64, scheduled time.Time, collector *metrics.Collector) {
	values := runtime.New(virtualUser, iteration, executor.scenario.Vars)

	for _, source := range executor.sources {
		record, err := source.Next(iteration)
		if err != nil {
			collector.Record(metrics.Sample{
				Step: "dados: " + source.Name(), Key: source.Name(), Protocol: "dados",
				ScheduledAt: scheduled, SentAt: scheduled, FinishedAt: executor.options.Clock.Now(),
				Class: protocol.ErrConfig, Detail: err.Error(),
			})
			return
		}
		values.SetAll(record)
		if perUse, has := source.(interface {
			PerUse() map[string]func() (string, error)
		}); has {
			for name, generate := range perUse.PerUse() {
				values.SetPerUse(name, generate)
			}
		}
	}

	var authHeader [2]string
	if executor.authenticator != nil {
		name, value, err := executor.authenticator.Header(runContext, values)
		if err != nil {
			collector.Record(metrics.Sample{
				Step: "autenticacao", Key: "autenticacao", Protocol: "http",
				ScheduledAt: scheduled, SentAt: scheduled, FinishedAt: executor.options.Clock.Now(),
				Class: protocol.ErrAuth, Detail: fmt.Sprintf("%v — alvo %s", err, executor.scenario.Target),
			})
			return
		}
		authHeader = [2]string{name, value}
	}

	stepInstant := scheduled
	complete := true
	for index, step := range executor.scenario.Steps {
		sample, _ := executor.runStep(runContext, step, stepInstant, values, authHeader)
		if index == 0 && !executor.closed() {
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

func (executor *Executor) runStep(runContext context.Context, step scenario.Step, scheduled time.Time,
	values *runtime.Values, authHeader [2]string) (metrics.Sample, Observation) {

	clock := executor.options.Clock
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
	response := implementation.Execute(runContext, protocol.Request{
		StepName:  step.Name,
		Config:    config,
		URLBase:   executor.scenario.Target,
		Vars:      values.Values(),
		Messaging: executor.scenario.Messaging,
	})
	sample.FinishedAt = clock.Now()
	sample.Status = response.Status
	sample.Bytes = response.Bytes
	sample.Class = response.Class
	sample.Detail = response.Detail
	observation.Response = response
	observation.Duration = sample.FinishedAt.Sub(sample.SentAt)

	if collector := executor.collector.Load(); collector != nil && len(response.Attributes) > 0 {
		collector.RecordUses(response.Attributes)
	}

	if response.Key != "" {
		sample.Key = response.Key
		observation.Key = response.Key
	}

	if sample.Class == protocol.Success {
		if class, detail := executor.verificar(step, response, values); class != protocol.Success {
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

func (executor *Executor) verificar(step scenario.Step, response protocol.Response, values *runtime.Values) (protocol.ErrorClass, string) {
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

func (executor *Executor) runAuthStep(runContext context.Context, step scenario.Step, values *runtime.Values) (protocol.Response, error) {
	sample, observation := executor.runStep(runContext, step, executor.options.Clock.Now(), values, [2]string{})
	if sample.Class != protocol.Success && sample.Class != protocol.ErrStatus {
		return observation.Response, fmt.Errorf("%s", sample.Detail)
	}
	return observation.Response, nil
}

func (executor *Executor) follow(collector *metrics.Collector, start time.Time, stop <-chan struct{}) {
	interval := executor.options.ProgressInterval
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
			remaining := executor.plan.Duration() - elapsed
			if remaining < 0 {
				remaining = 0
			}
			executor.options.OnProgress(collector.Snapshot(), executor.plan.RateAt(elapsed), remaining)
		}
	}
}

func (executor *Executor) appliedPhases() []metrics.AppliedPhase {
	phases := make([]metrics.AppliedPhase, 0, len(executor.scenario.Load.Phases))
	for _, phase := range executor.scenario.Load.Phases {
		phases = append(phases, metrics.AppliedPhase{
			Kind:       string(phase.Kind),
			From:       phase.InitialRate(),
			To:         phase.FinalRate(),
			DurationMs: phase.For.Milliseconds(),
		})
	}
	return phases
}

func (executor *Executor) prepareProtocols(runContext context.Context) error {
	values := runtime.New(0, 0, executor.scenario.Vars)
	for _, step := range executor.scenario.Steps {
		implementation, exists := protocol.Lookup(step.Protocol)
		if !exists {
			continue
		}
		preparer, needs := implementation.(protocol.Preparable)
		if !needs {
			continue
		}
		err := preparer.Prepare(runContext, protocol.Request{
			StepName:  step.Name,
			Config:    step.Config.Resolve(values.Resolve),
			URLBase:   executor.scenario.Target,
			Messaging: executor.scenario.Messaging,
		})
		if err != nil {
			return fmt.Errorf("nao consegui preparar o passo %q: %w", step.Name, err)
		}
	}
	return nil
}

func (executor *Executor) availability() metrics.Availability {
	availability := metrics.Availability{}
	for _, source := range executor.sources {
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
	// One token per run is a declared decision (ADR 0005), so a single value
	// here is not a defect: the limitation already shows in the environment
	// block, and repeating it as a high warning would drown the ones that
	// matter.
	if executor.scenario.Auth != nil && executor.scenario.Auth.Obtain != nil {
		for _, capture := range executor.scenario.Auth.Obtain.Captures {
			availability[capture.Variable] = 1
		}
	}
	return availability
}

// Only synthetic sources have a seed: recording one for a CSV would suggest
// the file was drawn at random, and what the report says about variety is the
// observed variety (ADR 0007), never the declared seed.
func (executor *Executor) seeds() map[string]int64 {
	seeds := map[string]int64{}
	for _, source := range executor.scenario.Data {
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

func (executor *Executor) authObtains() int64 {
	if executor.authenticator == nil {
		return 0
	}
	return executor.authenticator.Obtains
}

// The engine records; the protocol only declares what it measured (ADR 0003 §3).
func consumerLag() []protocol.ConsumerLag {
	var lags []protocol.ConsumerLag
	for _, name := range protocol.Registered() {
		implementation, found := protocol.Lookup(name)
		if !found {
			continue
		}
		if reporter, reports := implementation.(protocol.WithConsumerLag); reports {
			lags = append(lags, reporter.ConsumerLag()...)
		}
	}
	return lags
}
