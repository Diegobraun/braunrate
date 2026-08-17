// Package runner is the single place where a scenario turns into a result.
// The CLI and the server both call it, which is what makes "the server adds no
// logic" a fact instead of an intention: a rule that lived only in main.go
// would exist on one side and not on the other.
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/text"
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

	// Environment sao valores que a interface guarda so na sessao e injeta como
	// se fossem variaveis de ambiente. O arquivo mantem ${NOME}. Ver ADR 0021.
	Environment map[string]string
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
	content, err := os.ReadFile(path)
	if err != nil {
		spec, parseErr := scenario.ParseFile(path)
		return spec, engine.Plan{}, badFile(parseErr, "error in the scenario: %v", parseErr)
	}
	return Check(content, path)
}

// Check e a mesma leitura do Load a partir do texto, para quem ainda nao gravou
// o arquivo — a interface confere o rascunho enquanto a pessoa digita. Duas
// leituras diferentes dariam uma resposta no editor e outra no terminal.
func Check(content []byte, name string) (scenario.Spec, engine.Plan, error) {
	spec, err := scenario.Parse(content)
	if err != nil {
		if positioned, is := err.(scenario.ScenarioError); is {
			positioned.File = name
			err = positioned
		}
		return spec, engine.Plan{}, badFile(err, "error in the scenario: %v", err)
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
	return ExecuteSpec(runContext, spec, filepath.Dir(path), options)
}

// ExecuteSpec e o mesmo caminho a partir de um cenario ja construido, que e o
// que o cenario escrito em Go tem na mao. Duas montagens diferentes aqui
// significariam um veredito para quem escreve YAML e outro para quem escreve
// Go, que e exatamente o que o ADR 0009 proibe.
func ExecuteSpec(runContext context.Context, spec scenario.Spec, dataRoot string, options Options) (Result, error) {
	spec.ResolveWith(options.Environment)
	if err := spec.Validate(); err != nil {
		return Result{}, badFile(err, "%v", err)
	}
	if err := RequireEnvironment(spec); err != nil {
		return Result{}, err
	}

	engineOptions := engine.DefaultOptions()
	engineOptions.Version = options.Version
	engineOptions.MaxInflight = options.MaxInflight
	engineOptions.LateThreshold = options.LateThreshold
	engineOptions.DataRoot = dataRoot
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

func Debug(runContext context.Context, path string, options Options) (Iteration, error) {
	spec, _, err := Load(path)
	if err != nil {
		return Iteration{}, err
	}
	spec.ResolveWith(options.Environment)
	if err := RequireEnvironment(spec); err != nil {
		return Iteration{}, err
	}

	engineOptions := engine.DefaultOptions()
	engineOptions.Version = options.Version
	engineOptions.DataRoot = filepath.Dir(path)

	executor, err := engine.New(spec, engineOptions)
	if err != nil {
		return Iteration{}, badFile(err, "%v", err)
	}

	observations, vars, err := executor.Debug(runContext)
	protocol.CloseAll()
	if err != nil {
		return Iteration{Spec: spec}, Fault{Exit: ExitSLO, Message: fmt.Sprintf("I could not reach the first step: %v", err)}
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
		return fmt.Errorf("could not serialize the result: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("could not write the result: %v", err)
	}
	return nil
}

func WriteHTML(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create %s: %v", path, err)
	}
	if err := report.HTML(file, document); err != nil {
		_ = file.Close()
		return fmt.Errorf("error generating the HTML report: %v", err)
	}
	// Close reports the write the operating system had not flushed yet. Deferred
	// and discarded, a full disk produced a truncated file and a message saying
	// the report was ready.
	if err := file.Close(); err != nil {
		return fmt.Errorf("error closing %s, the report may be incomplete: %v", path, err)
	}
	return nil
}

func WriteComparisonHTML(path string, result comparison.Comparison, version string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create %s: %v", path, err)
	}
	if err := report.ComparisonHTML(file, result, version); err != nil {
		_ = file.Close()
		return fmt.Errorf("error generating the comparison in HTML: %v", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("error closing %s, the comparison may be incomplete: %v", path, err)
	}
	return nil
}

func WriteCSV(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create %s: %v", path, err)
	}
	if err := report.CSV(file, document); err != nil {
		_ = file.Close()
		return fmt.Errorf("could not generate the CSV: %v", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("could not close %s, the CSV may be incomplete: %v", path, err)
	}
	return nil
}

// A result read from disk is a contract with a past version, so what is refused
// here is refused with the reason: wrong tool, wrong format version, not JSON.
func ReadDocument(path string) (metrics.Document, error) {
	var document metrics.Document
	content, err := os.ReadFile(path)
	if err != nil {
		return document, fmt.Errorf("could not read %s: %v", path, err)
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return document, fmt.Errorf("%s is not a braunrate result: %v", path, err)
	}
	// The fields were in Portuguese until 0.5.0, so a document from then decodes
	// into an empty one: without this the reader would be told the file was not
	// generated by braunrate, which is both wrong and unactionable.
	if document.Tool == "" && bytes.Contains(content, []byte(`"ferramenta"`)) {
		return document, fmt.Errorf("%s was written by braunrate 0.5.0, whose result fields were in Portuguese.\n"+
			"    run the scenario again with this version to get a result in the current format", path)
	}
	if document.Tool != "braunrate" {
		return document, fmt.Errorf("%s was not generated by braunrate; use the file from -result", path)
	}
	if !slices.Contains(metrics.ReadableResultFormats, document.FormatVersion) {
		return document, fmt.Errorf("%s is in result format %q and this version reads the formats %s",
			path, document.FormatVersion, strings.Join(metrics.ReadableResultFormats, ", "))
	}
	document.TranslateOlderValues()
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
		lines = append(lines, fmt.Sprintf("Valid scenario: %q, %s, %d users in a closed loop for %s.",
			spec.Name, text.Count(int64(len(spec.Steps)), "step", "steps"), spec.Load.Users, plan.Duration()))
	} else {
		lines = append(lines, fmt.Sprintf("Valid scenario: %q, %s, %s in %s.",
			spec.Name, text.Count(int64(len(spec.Steps)), "step", "steps"),
			text.Count(plan.TotalRequests(), "iteration", "iterations"), plan.Duration()))
	}
	if warning, closed := scenario.ClosedModelWarning(spec); closed {
		lines = append(lines, warning)
	}
	if len(spec.SLO) == 0 {
		lines = append(lines, "No slo declared: the run will never fail for slowness. Add an 'slo' block to turn it into a CI gate.")
	}
	for _, broker := range scenario.DescribeMessaging(spec.Messaging) {
		lines = append(lines, "Messaging: "+broker)
	}
	if len(spec.Requires) > 0 {
		lines = append(lines, fmt.Sprintf("It depends on external infrastructure: %s. Without it the run does not happen.",
			strings.Join(spec.Requires, ", ")))
	}
	if len(spec.MissingEnvironment) > 0 {
		lines = append(lines, missingEnvironmentWarning(spec))
	}
	if warning := unusualRateWarning(plan); warning != "" {
		lines = append(lines, warning)
	}
	lines = append(lines, scenario.FixedStepWarnings(spec)...)
	return append(lines, scenario.GateWarnings(spec)...)
}

// The tool does not know the target's traffic, so it cannot say which rate is
// right — teaching the arithmetic is a different thing from guessing the number.
// What it does know is the generator: phase 0 measured this arrival model
// sustaining 20,000/s on every repetition and missing some above that. Past that
// line the number in the report can be the generator instead of the target, and
// that is worth a question, not a refusal.
const measuredCeiling = 20000

func unusualRateWarning(plan engine.Plan) string {
	peak := plan.PeakRate()
	if peak <= measuredCeiling {
		return ""
	}
	return fmt.Sprintf("Is that the rate you meant? The profile peaks at %s/s, which over %s is %s requests. "+
		"The generator was measured sustaining %s/s on every repetition and missing some above that (docs/medicoes-fase0.md), "+
		"so past that point the report can be measuring braunrate and not the target. Nothing here refuses: if that is the rate, run it.",
		text.Grouped(int64(peak)), plan.Duration(), text.Grouped(plan.TotalRequests()), text.Grouped(measuredCeiling))
}

// Validation is about the file and runs where the secret is not: it warns.
// Execution is what sends the request, and a request with an empty credential
// comes back 401 with nothing in the output connecting the two, so it refuses.
func missingEnvironmentWarning(spec scenario.Spec) string {
	return fmt.Sprintf("Environment variable not set: %s. Here that is only a warning; in a run the field would come out empty, so 'braunrate execute' refuses until the variable exists.",
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
		"the scenario uses %s, and that variable is not in the environment: the field would come out empty and the target would answer with an error that explains nothing.\n"+
			"    run with:  %s=... braunrate execute scenario.yaml\n"+
			"    or declare a fallback in the scenario itself:  variables: { %s: \"${%s:-value}\" }",
		strings.Join(spec.MissingEnvironment, ", "), first, strings.ToLower(first), first)}
}
