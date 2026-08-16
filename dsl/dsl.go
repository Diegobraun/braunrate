// Package dsl builds the same structure the YAML builds and hands it to the
// same engine. That is why it parses nothing of its own: captures, comparisons
// and SLO limits go through the scenario functions the YAML uses, and no
// default is rewritten here. Two readings of the same line would mean a
// different number for each audience, which is exactly what the tool promises
// not to do.
package dsl

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Rate float64

func PerSecond(value float64) Rate { return Rate(value) }
func PerMinute(value float64) Rate { return Rate(value / 60) }
func PerHour(value float64) Rate   { return Rate(value / 3600) }

type Request interface {
	build() (string, protocol.Config, error)
}

type StepOption func(*scenario.Step) error

type Builder struct {
	scenario scenario.Spec
	errors   []error
}

func New(name string) *Builder {
	return &Builder{scenario: scenario.Spec{
		FormatVersion: scenario.FormatVersion,
		Name:          name,
		Vars:          map[string]string{},
		Load:          scenario.LoadPlan{Model: scenario.OpenArrival},
	}}
}

func (builder *Builder) note(err error) {
	if err != nil {
		builder.errors = append(builder.errors, err)
	}
}

func (builder *Builder) Target(target string) *Builder {
	builder.scenario.Target = target
	return builder
}

// Requires declares external infrastructure without which the scenario does not
// run. It changes nothing at run time; it is read by whoever runs the scenario
// and by the CI loop over the published examples.
func (builder *Builder) Requires(requirements ...string) *Builder {
	for _, requirement := range requirements {
		if !slices.Contains(scenario.KnownRequirements, requirement) {
			builder.note(fmt.Errorf("dependencia desconhecida: %q (use %s)", requirement, strings.Join(scenario.KnownRequirements, ", ")))
			continue
		}
		builder.scenario.Requires = append(builder.scenario.Requires, requirement)
	}
	return builder
}

func (builder *Builder) Variable(name, value string) *Builder {
	builder.scenario.Vars[name] = scenario.ExpandFromEnv(value)
	return builder
}

type DataOption func(*scenario.DataSource)

func Consume(policy scenario.ConsumePolicy) DataOption {
	return func(source *scenario.DataSource) { source.Consume = policy }
}

func Seed(seed int64) DataOption {
	return func(source *scenario.DataSource) { source.Seed = seed }
}

func (builder *Builder) DataFromFile(name, file string, options ...DataOption) *Builder {
	source := scenario.DataSource{Name: name, File: file, Consume: scenario.ConsumeCircular}
	for _, option := range options {
		option(&source)
	}
	builder.scenario.Data = append(builder.scenario.Data, source)
	return builder
}

func (builder *Builder) GeneratedData(name string, fields map[string]string, options ...DataOption) *Builder {
	source := scenario.DataSource{Name: name, Consume: scenario.ConsumeCircular, Fields: map[string]scenario.Generator{}}
	for field, recipe := range fields {
		source.Fields[field] = scenario.ParseGenerator(recipe)
	}
	for _, option := range options {
		option(&source)
	}
	if len(source.Fields) == 0 {
		builder.note(fmt.Errorf("a fonte de dados %q precisa de pelo menos um campo em 'gerar'", name))
	}
	builder.scenario.Data = append(builder.scenario.Data, source)
	return builder
}

// Field is the long form of a generated field, for what the short string
// cannot say: a declared format, or a new value at every use.
type Field struct {
	recipe string
	format string
	perUse bool
}

func Generator(recipe string) Field { return Field{recipe: recipe} }

func Pattern(format string) Field { return Field{recipe: "padrao", format: format} }

func (field Field) NewPerUse() Field {
	field.perUse = true
	return field
}

func (builder *Builder) GeneratedFields(name string, fields map[string]Field, options ...DataOption) *Builder {
	source := scenario.DataSource{Name: name, Consume: scenario.ConsumeCircular, Fields: map[string]scenario.Generator{}}
	for field, declared := range fields {
		source.Fields[field] = scenario.Generator{
			Recipe: declared.recipe, Format: declared.format, PerUse: declared.perUse,
		}
	}
	for _, option := range options {
		option(&source)
	}
	if len(source.Fields) == 0 {
		builder.note(fmt.Errorf("a fonte de dados %q precisa de pelo menos um campo em 'gerar'", name))
	}
	builder.scenario.Data = append(builder.scenario.Data, source)
	return builder
}

func (builder *Builder) Ramp(from, to Rate, during time.Duration) *Builder {
	return builder.phase(scenario.Phase{Kind: scenario.PhaseRamp, From: float64(from), To: float64(to), For: during})
}

func (builder *Builder) Plateau(rate Rate, during time.Duration) *Builder {
	return builder.phase(scenario.Phase{Kind: scenario.PhasePlateau, To: float64(rate), For: during})
}

func (builder *Builder) Spike(rate Rate, during time.Duration) *Builder {
	return builder.phase(scenario.Phase{Kind: scenario.PhaseSpike, To: float64(rate), For: during})
}

func (builder *Builder) Constant(rate Rate, during time.Duration) *Builder {
	return builder.phase(scenario.Phase{Kind: scenario.PhaseConstant, To: float64(rate), For: during})
}

// ClosedLoop is the declared exception, never the default: the rate stops being
// something you ask for and becomes whatever the target allows.
func (builder *Builder) ClosedLoop(users int, during, betweenIterations time.Duration) *Builder {
	builder.scenario.Load = scenario.LoadPlan{
		Model: scenario.ClosedArrival, Users: users, For: during, ThinkTime: betweenIterations,
	}
	return builder
}

func (builder *Builder) phase(phase scenario.Phase) *Builder {
	builder.scenario.Load.Phases = append(builder.scenario.Load.Phases, phase)
	return builder
}

func (builder *Builder) Step(request Request, options ...StepOption) *Builder {
	step, err := buildStep(request, options...)
	if err != nil {
		builder.note(err)
		return builder
	}
	builder.scenario.Steps = append(builder.scenario.Steps, step)
	return builder
}

func buildStep(request Request, options ...StepOption) (scenario.Step, error) {
	if request == nil {
		return scenario.Step{}, errors.New("passo sem requisicao")
	}
	protocolName, config, err := request.build()
	if err != nil {
		return scenario.Step{}, err
	}
	step := scenario.Step{Protocol: protocolName, Config: config}
	for _, option := range options {
		if err := option(&step); err != nil {
			return scenario.Step{}, err
		}
	}
	if step.Name == "" {
		step.Name = config.AggregationKey()
	}
	return step, nil
}

func Name(name string) StepOption {
	return func(step *scenario.Step) error {
		step.Name = name
		return nil
	}
}

// Capture takes the same expression the YAML takes: "$.invoice.id",
// "cabecalho:X-Id" or "/regex/".
func Capture(variable, expression string) StepOption {
	return func(step *scenario.Step) error {
		capture, err := scenario.ParseCapture(variable, expression)
		if err != nil {
			return err
		}
		step.Captures = append(step.Captures, capture)
		return nil
	}
}

func CaptureWithDefault(variable, expression, fallback string) StepOption {
	return func(step *scenario.Step) error {
		capture, err := scenario.ParseCapture(variable, expression)
		if err != nil {
			return err
		}
		capture.Default = fallback
		capture.Required = false
		step.Captures = append(step.Captures, capture)
		return nil
	}
}

func CheckStatus(status int) StepOption {
	return func(step *scenario.Step) error {
		step.Checks = append(step.Checks, scenario.Check{Kind: scenario.CheckStatus, Status: status})
		return nil
	}
}

func CheckBodyContains(fragment string) StepOption {
	return func(step *scenario.Step) error {
		step.Assertions = append(step.Assertions, scenario.Assertion{Kind: scenario.AssertBodyContains, Value: fragment})
		return nil
	}
}

func CheckBodyMatches(pattern string) StepOption {
	return func(step *scenario.Step) error {
		step.Assertions = append(step.Assertions, scenario.Assertion{Kind: scenario.AssertRegex, Value: pattern})
		return nil
	}
}

// CheckJSON takes the same comparison the YAML takes: "PAGA", "> 10",
// "existe", "contem parcial".
func CheckJSON(path, comparison string) StepOption {
	return func(step *scenario.Step) error {
		assertion := scenario.ParseComparison(path, comparison)
		assertion.Kind = scenario.AssertJSON
		step.Assertions = append(step.Assertions, assertion)
		return nil
	}
}

func CheckHeader(name, value string) StepOption {
	return func(step *scenario.Step) error {
		step.Assertions = append(step.Assertions, scenario.Assertion{
			Kind: scenario.AssertHeader, Target: name, Operator: scenario.OpEqual, Value: value,
		})
		return nil
	}
}

func (builder *Builder) SLO(step, metric, limit string) *Builder {
	rule, err := scenario.ParseSLORule(step, metric, limit)
	if err != nil {
		builder.note(err)
		return builder
	}
	builder.scenario.SLO = append(builder.scenario.SLO, rule)
	return builder
}

func (builder *Builder) OverallSLO(metric, limit string) *Builder {
	return builder.SLO("global", metric, limit)
}

func (builder *Builder) JourneySLO(metric, limit string) *Builder {
	return builder.SLO("jornada", metric, limit)
}

func (builder *Builder) RegressionSLO(metric, limit string) *Builder {
	return builder.SLO("regressao", metric, limit)
}

type Authenticator struct {
	auth scenario.Auth
	err  error
}

func WithToken(request Request, options ...StepOption) *Authenticator {
	step, err := buildStep(request, options...)
	if err != nil {
		return &Authenticator{err: err}
	}
	step.Name = "obter autenticacao"
	return &Authenticator{auth: scenario.Auth{Kind: scenario.AuthToken, Obtain: &step}}
}

func Basic(user, password string) *Authenticator {
	return &Authenticator{auth: scenario.Auth{
		Kind: scenario.AuthBasic, User: user, Password: password,
	}}
}

func WithHeaderAuth(header string) *Authenticator {
	return &Authenticator{auth: scenario.Auth{
		Kind: scenario.AuthHeader, Header: header,
	}}
}

func (authenticator *Authenticator) RefreshAfter(interval time.Duration) *Authenticator {
	authenticator.auth.RefreshAfter = interval
	return authenticator
}

func (authenticator *Authenticator) Header(header string) *Authenticator {
	authenticator.auth.Header = header
	return authenticator
}

func (builder *Builder) Auth(authenticator *Authenticator) *Builder {
	if authenticator.err != nil {
		builder.note(authenticator.err)
		return builder
	}
	auth := authenticator.auth
	if auth.Kind == scenario.AuthToken && auth.Obtain == nil {
		builder.note(errors.New("autenticacao por token precisa da requisicao que devolve o token"))
		return builder
	}
	if auth.Kind == scenario.AuthBasic && (auth.User == "" || auth.Password == "") {
		builder.note(errors.New("autenticacao basica precisa de usuario e senha"))
		return builder
	}
	if auth.Header == "" && auth.Kind != scenario.AuthBasic {
		auth.Header = "Authorization: Bearer ${token}"
	}
	builder.scenario.Auth = &auth
	return builder
}

func (builder *Builder) Build() (scenario.Spec, error) {
	built := builder.scenario
	built.Target = scenario.Interpolate(built.Target, built.Vars)
	if len(builder.errors) > 0 {
		return built, fmt.Errorf("cenario invalido:\n  - %s", strings.Join(messages(builder.errors), "\n  - "))
	}
	if err := built.Validate(); err != nil {
		return built, err
	}
	// The same refusal the YAML gets for a ${nome} that resolves from nowhere.
	// It ran only on the text of the file, so the Go scenario sent the empty
	// value out and nobody was told (ADR 0002).
	if err := scenario.CheckReferences(&built); err != nil {
		return built, err
	}
	return built, nil
}

func messages(errors []error) []string {
	texts := make([]string, 0, len(errors))
	for _, err := range errors {
		texts = append(texts, err.Error())
	}
	return texts
}

// BrokerAuth mirrors the `mensageria` block. The secret is a reference to an
// environment variable here for the same reason it is in the YAML: a Go file
// also goes to the repository.
type BrokerAuth struct {
	broker messaging.Broker
	err    error
}

func BrokerAt(addresses ...string) *BrokerAuth {
	return &BrokerAuth{broker: messaging.Broker{Addresses: addresses}}
}

func (authenticator *BrokerAuth) credential(kind messaging.Kind, user, passwordVar string) *BrokerAuth {
	name, reference := scenario.EnvironmentVariable(passwordVar)
	if !reference {
		authenticator.err = fmt.Errorf("a senha do broker precisa ser referencia a variavel de ambiente, como \"${KAFKA_SENHA}\", e veio %q", passwordVar)
		return authenticator
	}
	authenticator.broker.Auth = messaging.Auth{
		Kind: kind, User: scenario.ExpandFromEnv(user),
		PasswordVar: name, Password: scenario.ExpandFromEnv(passwordVar),
	}
	return authenticator
}

func (authenticator *BrokerAuth) Plain(user, passwordVar string) *BrokerAuth {
	return authenticator.credential(messaging.Plain, user, passwordVar)
}

func (authenticator *BrokerAuth) SCRAM256(user, passwordVar string) *BrokerAuth {
	return authenticator.credential(messaging.SCRAM256, user, passwordVar)
}

func (authenticator *BrokerAuth) SCRAM512(user, passwordVar string) *BrokerAuth {
	return authenticator.credential(messaging.SCRAM512, user, passwordVar)
}

// MSKIAM never takes a key: the signature comes from the AWS default chain.
func (authenticator *BrokerAuth) MSKIAM(region string) *BrokerAuth {
	authenticator.broker.Auth = messaging.Auth{Kind: messaging.MSKIAM, Region: region}
	authenticator.broker.TLS.Enabled = true
	return authenticator
}

func (authenticator *BrokerAuth) ClientCertificate(certificate, key string) *BrokerAuth {
	authenticator.broker.Auth.Kind = messaging.External
	authenticator.broker.TLS.Enabled = true
	authenticator.broker.TLS.Certificate = certificate
	authenticator.broker.TLS.Key = key
	return authenticator
}

func (authenticator *BrokerAuth) TLS() *BrokerAuth {
	authenticator.broker.TLS.Enabled = true
	return authenticator
}

func (authenticator *BrokerAuth) CA(path string) *BrokerAuth {
	authenticator.broker.TLS.Enabled = true
	authenticator.broker.TLS.CA = path
	return authenticator
}

func (builder *Builder) KafkaBroker(authenticator *BrokerAuth) *Builder {
	if authenticator.err != nil {
		builder.note(authenticator.err)
		return builder
	}
	if builder.scenario.Messaging == nil {
		builder.scenario.Messaging = &messaging.Settings{}
	}
	broker := authenticator.broker
	builder.scenario.Messaging.Kafka = &broker
	return builder
}

func (builder *Builder) AMQPBroker(authenticator *BrokerAuth) *Builder {
	if authenticator.err != nil {
		builder.note(authenticator.err)
		return builder
	}
	broker := authenticator.broker
	if err := broker.SupportsAMQP(); err != nil {
		builder.note(err)
		return builder
	}
	if builder.scenario.Messaging == nil {
		builder.scenario.Messaging = &messaging.Settings{}
	}
	builder.scenario.Messaging.AMQP = &broker
	return builder
}
