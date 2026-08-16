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
	"strings"
	"time"

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

func (c *Builder) note(err error) {
	if err != nil {
		c.errors = append(c.errors, err)
	}
}

func (c *Builder) Target(target string) *Builder {
	c.scenario.Target = target
	return c
}

func (c *Builder) Variable(name, value string) *Builder {
	c.scenario.Vars[name] = scenario.ExpandFromEnv(value)
	return c
}

type DataOption func(*scenario.DataSource)

func Consume(policy scenario.ConsumePolicy) DataOption {
	return func(source *scenario.DataSource) { source.Consume = policy }
}

func Seed(seed int64) DataOption {
	return func(source *scenario.DataSource) { source.Seed = seed }
}

func (c *Builder) DataFromFile(name, file string, opts ...DataOption) *Builder {
	source := scenario.DataSource{Name: name, File: file, Consume: scenario.ConsumeCircular}
	for _, option := range opts {
		option(&source)
	}
	c.scenario.Data = append(c.scenario.Data, source)
	return c
}

func (c *Builder) GeneratedData(name string, fields map[string]string, opts ...DataOption) *Builder {
	source := scenario.DataSource{Name: name, Consume: scenario.ConsumeCircular, Fields: map[string]string{}}
	for field, recipe := range fields {
		source.Fields[field] = recipe
	}
	for _, option := range opts {
		option(&source)
	}
	if len(source.Fields) == 0 {
		c.note(fmt.Errorf("a fonte de dados %q precisa de pelo menos um campo em 'gerar'", name))
	}
	c.scenario.Data = append(c.scenario.Data, source)
	return c
}

func (c *Builder) Ramp(from, to Rate, during time.Duration) *Builder {
	return c.phase(scenario.Phase{Kind: scenario.PhaseRamp, From: float64(from), To: float64(to), For: during})
}

func (c *Builder) Plateau(rate Rate, during time.Duration) *Builder {
	return c.phase(scenario.Phase{Kind: scenario.PhasePlateau, To: float64(rate), For: during})
}

func (c *Builder) Spike(rate Rate, during time.Duration) *Builder {
	return c.phase(scenario.Phase{Kind: scenario.PhaseSpike, To: float64(rate), For: during})
}

func (c *Builder) Constant(rate Rate, during time.Duration) *Builder {
	return c.phase(scenario.Phase{Kind: scenario.PhaseConstant, To: float64(rate), For: during})
}

func (c *Builder) phase(phase scenario.Phase) *Builder {
	c.scenario.Load.Phases = append(c.scenario.Load.Phases, phase)
	return c
}

func (c *Builder) Step(request Request, opts ...StepOption) *Builder {
	step, err := buildStep(request, opts...)
	if err != nil {
		c.note(err)
		return c
	}
	c.scenario.Steps = append(c.scenario.Steps, step)
	return c
}

func buildStep(request Request, opts ...StepOption) (scenario.Step, error) {
	if request == nil {
		return scenario.Step{}, errors.New("passo sem requisicao")
	}
	protocolName, config, err := request.build()
	if err != nil {
		return scenario.Step{}, err
	}
	step := scenario.Step{Protocol: protocolName, Config: config}
	for _, option := range opts {
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

func (c *Builder) SLO(step, metric, limit string) *Builder {
	rule, err := scenario.ParseSLORule(step, metric, limit)
	if err != nil {
		c.note(err)
		return c
	}
	c.scenario.SLO = append(c.scenario.SLO, rule)
	return c
}

func (c *Builder) OverallSLO(metric, limit string) *Builder {
	return c.SLO("global", metric, limit)
}

type Authenticator struct {
	auth scenario.Auth
	err  error
}

func WithToken(request Request, opts ...StepOption) *Authenticator {
	step, err := buildStep(request, opts...)
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

func (a *Authenticator) RefreshAfter(interval time.Duration) *Authenticator {
	a.auth.RefreshAfter = interval
	return a
}

func (a *Authenticator) Header(header string) *Authenticator {
	a.auth.Header = header
	return a
}

func (c *Builder) Auth(authenticator *Authenticator) *Builder {
	if authenticator.err != nil {
		c.note(authenticator.err)
		return c
	}
	auth := authenticator.auth
	if auth.Kind == scenario.AuthToken && auth.Obtain == nil {
		c.note(errors.New("autenticacao por token precisa da requisicao que devolve o token"))
		return c
	}
	if auth.Kind == scenario.AuthBasic && (auth.User == "" || auth.Password == "") {
		c.note(errors.New("autenticacao basica precisa de usuario e senha"))
		return c
	}
	if auth.Header == "" && auth.Kind != scenario.AuthBasic {
		auth.Header = "Authorization: Bearer ${token}"
	}
	c.scenario.Auth = &auth
	return c
}

func (c *Builder) Build() (scenario.Spec, error) {
	built := c.scenario
	built.Target = scenario.Interpolate(built.Target, built.Vars)
	if len(c.errors) > 0 {
		return built, fmt.Errorf("cenario invalido:\n  - %s", strings.Join(messages(c.errors), "\n  - "))
	}
	if err := built.Validate(); err != nil {
		return built, err
	}
	return built, nil
}

func messages(errors []error) []string {
	textos := make([]string, 0, len(errors))
	for _, err := range errors {
		textos = append(textos, err.Error())
	}
	return textos
}
