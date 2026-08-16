// Package runner is the single place where a scenario turns into a result.
// The CLI and the server both call it, which is what makes "the server adds no
// logic" a fact instead of an intention: a rule that lived only in main.go
// would exist on one side and not on the other.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
)

// Exit codes are part of the contract with CI, so they are decided here and
// not by whoever is printing.
const (
	ExitPassed  = 0
	ExitSLO     = 1
	ExitBadFile = 2
	ExitInvalid = 3
)

// Fault says which of the exit codes a failure is worth, so the CLI and the
// server do not each decide on their own what a broken scenario means.
type Fault struct {
	Exit    int
	Message string
	// The original error travels along so the line and the column survive the
	// formatting: an editor needs them as numbers, not inside a sentence.
	Cause error
}

func (fault Fault) Error() string { return fault.Message }

func (fault Fault) Unwrap() error { return fault.Cause }

func badFile(cause error, format string, args ...any) Fault {
	return Fault{Exit: ExitBadFile, Message: fmt.Sprintf(format, args...), Cause: cause}
}

type Options struct {
	Version       string
	MaxInflight   int64
	LateThreshold time.Duration
	BaselinePath  string
	OnProgress    engine.ProgressFunc
}

func DefaultOptions(version string) Options {
	return Options{
		Version:       version,
		MaxInflight:   20000,
		LateThreshold: 10 * time.Millisecond,
	}
}

type Result struct {
	Spec     scenario.Spec
	Plan     engine.Plan
	Document metrics.Document
	Exit     int
}

// Load reads and validates without executing. Every entry point starts here,
// which is why a scenario refused by the CLI is refused by the server with the
// same message.
func Load(path string) (scenario.Spec, engine.Plan, error) {
	spec, err := scenario.ParseFile(path)
	if err != nil {
		return spec, engine.Plan{}, badFile(err, "erro no cenario: %v", err)
	}
	if err := spec.Validate(); err != nil {
		return spec, engine.Plan{}, badFile(err, "%v", err)
	}
	return spec, engine.CompilePlan(spec.Load), nil
}

func Execute(runContext context.Context, path string, options Options) (Result, error) {
	spec, _, err := Load(path)
	if err != nil {
		return Result{}, err
	}
	if err := RequireEnvironment(spec); err != nil {
		return Result{}, err
	}

	engineOptions := engine.DefaultOptions()
	engineOptions.Version = options.Version
	engineOptions.MaxInflight = options.MaxInflight
	engineOptions.LateThreshold = options.LateThreshold
	engineOptions.DataRoot = filepath.Dir(path)
	engineOptions.OnProgress = options.OnProgress

	executor, err := engine.New(spec, engineOptions)
	if err != nil {
		return Result{}, badFile(err, "%v", err)
	}

	result := Result{Spec: spec, Plan: executor.Plan()}
	result.Document = executor.Execute(runContext)
	protocol.CloseAll()

	var baseline *slo.Baseline
	if options.BaselinePath != "" {
		before, err := ReadDocument(options.BaselinePath)
		if err != nil {
			return result, badFile(err, "%v", err)
		}
		baseline = &slo.Baseline{Comparison: comparison.Compare(before, result.Document), Path: options.BaselinePath}
	}
	// A run that did not measure what it set out to measure has nothing to
	// approve or reject, so the SLO is not even evaluated.
	if result.Document.Valid() {
		result.Document.SLO = slo.Evaluate(spec.SLO, result.Document, baseline)
	}
	result.Exit = verdict(result.Document)
	return result, nil
}

func verdict(document metrics.Document) int {
	switch {
	case !document.Valid():
		return ExitInvalid
	case !document.SLO.Passed:
		return ExitSLO
	}
	return ExitPassed
}

type Iteration struct {
	Observations []engine.Observation
	Vars         map[string]string
	Spec         scenario.Spec
}

func Debug(runContext context.Context, path string, version string) (Iteration, error) {
	spec, _, err := Load(path)
	if err != nil {
		return Iteration{}, err
	}
	if err := RequireEnvironment(spec); err != nil {
		return Iteration{}, err
	}

	engineOptions := engine.DefaultOptions()
	engineOptions.Version = version
	engineOptions.DataRoot = filepath.Dir(path)

	executor, err := engine.New(spec, engineOptions)
	if err != nil {
		return Iteration{}, badFile(err, "%v", err)
	}

	observations, vars, err := executor.Debug(runContext)
	protocol.CloseAll()
	if err != nil {
		return Iteration{Spec: spec}, Fault{Exit: ExitSLO, Message: fmt.Sprintf("nao consegui chegar ao primeiro passo: %v", err)}
	}
	return Iteration{Observations: observations, Vars: vars, Spec: spec}, nil
}

func (iteration Iteration) Complete() bool {
	if len(iteration.Observations) < len(iteration.Spec.Steps) {
		return false
	}
	for _, observation := range iteration.Observations {
		if observation.Class != protocol.Success {
			return false
		}
	}
	return true
}

func WriteJSON(path string, document metrics.Document) error {
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("erro ao gravar resultado: %v", err)
	}
	return nil
}

func WriteHTML(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar %s: %v", path, err)
	}
	if err := report.HTML(file, document); err != nil {
		_ = file.Close()
		return fmt.Errorf("erro ao gerar o relatorio HTML: %v", err)
	}
	// Close reports the write the operating system had not flushed yet. Deferred
	// and discarded, a full disk produced a truncated file and a message saying
	// the report was ready.
	if err := file.Close(); err != nil {
		return fmt.Errorf("erro ao fechar %s, o relatorio pode estar incompleto: %v", path, err)
	}
	return nil
}

func WriteCSV(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar %s: %v", path, err)
	}
	if err := report.CSV(file, document); err != nil {
		_ = file.Close()
		return fmt.Errorf("erro ao gerar o CSV: %v", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("erro ao fechar %s, o CSV pode estar incompleto: %v", path, err)
	}
	return nil
}

// A result read from disk is a contract with a past version, so what is refused
// here is refused with the reason: wrong tool, wrong format version, not JSON.
func ReadDocument(path string) (metrics.Document, error) {
	var document metrics.Document
	content, err := os.ReadFile(path)
	if err != nil {
		return document, fmt.Errorf("nao consegui ler %s: %v", path, err)
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return document, fmt.Errorf("%s nao e um resultado do braunrate: %v", path, err)
	}
	if document.Tool != "braunrate" {
		return document, fmt.Errorf("%s nao foi gerado pelo braunrate; use o arquivo de -result", path)
	}
	if document.FormatVersion != metrics.VersaoDoFormatoDeResultado {
		return document, fmt.Errorf("%s esta no formato de resultado %q e esta versao le o formato %q",
			path, document.FormatVersion, metrics.VersaoDoFormatoDeResultado)
	}
	return document, nil
}

func (iteration Iteration) Failed() bool {
	for _, observation := range iteration.Observations {
		if observation.Class != protocol.Success {
			return true
		}
	}
	return false
}

// Describe is what a valid scenario says about itself before running: size,
// what it needs, and every warning that does not stop it. The server answers
// with these same lines, so a scenario approved in the terminal is approved
// with the same words over HTTP.
func Describe(spec scenario.Spec, plan engine.Plan) []string {
	var lines []string
	if spec.Load.Closed() {
		lines = append(lines, fmt.Sprintf("Cenario valido: %q, %d passo(s), %d usuarios em laco fechado durante %s.",
			spec.Name, len(spec.Steps), spec.Load.Users, plan.Duration()))
	} else {
		lines = append(lines, fmt.Sprintf("Cenario valido: %q, %d passo(s), %d iteracoes em %s.",
			spec.Name, len(spec.Steps), plan.TotalRequests(), plan.Duration()))
	}
	if warning, closed := scenario.ClosedModelWarning(spec); closed {
		lines = append(lines, warning)
	}
	if len(spec.SLO) == 0 {
		lines = append(lines, "Sem slo declarado: a execucao nunca vai falhar por lentidao. Adicione um bloco 'slo' para virar gate de CI.")
	}
	for _, broker := range scenario.DescribeMessaging(spec.Messaging) {
		lines = append(lines, "Mensageria: "+broker)
	}
	if len(spec.Requires) > 0 {
		lines = append(lines, fmt.Sprintf("Depende de infraestrutura externa: %s. Sem isso a execucao nao roda.",
			strings.Join(spec.Requires, ", ")))
	}
	if len(spec.MissingEnvironment) > 0 {
		lines = append(lines, missingEnvironmentWarning(spec))
	}
	return append(lines, scenario.GateWarnings(spec)...)
}

// Validation is about the file and runs where the secret is not: it warns.
// Execution is what sends the request, and a request with an empty credential
// comes back 401 with nothing in the output connecting the two, so it refuses.
func missingEnvironmentWarning(spec scenario.Spec) string {
	return fmt.Sprintf("Variavel de ambiente nao definida: %s. Aqui isso e so aviso; na execucao o campo sairia vazio, entao 'braunrate execute' recusa ate a variavel existir.",
		strings.Join(spec.MissingEnvironment, ", "))
}

// RequireEnvironment is exported so the caller can refuse before printing the
// headline: announcing a run and then refusing it reads like a crash.
func RequireEnvironment(spec scenario.Spec) error {
	if len(spec.MissingEnvironment) == 0 {
		return nil
	}
	first := spec.MissingEnvironment[0]
	return Fault{Exit: ExitBadFile, Message: fmt.Sprintf(
		"o cenario usa %s, e essa variavel nao esta no ambiente: o campo sairia vazio e o alvo responderia com erro que nao explica nada.\n"+
			"    rode com:  %s=... braunrate execute cenario.yaml\n"+
			"    ou declare uma reserva no proprio cenario:  variaveis: { %s: \"${%s:-valor}\" }",
		strings.Join(spec.MissingEnvironment, ", "), first, strings.ToLower(first), first)}
}
