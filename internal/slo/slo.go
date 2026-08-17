package slo

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Verdict = metrics.Verdict

type Evaluation = metrics.Evaluation

// Baseline is nil when none was given; the rules that need it then say so
// instead of passing by default.
type Baseline struct {
	Comparison comparison.Comparison
	Path       string
}

// Evaluate assumes the run already passed the sanity check: whether the result
// means anything is decided before any rule is read, in metrics.CheckSanity,
// and a run that failed there never reaches here.
func Evaluate(rules []scenario.SLORule, document metrics.Document, baseline *Baseline) Verdict {
	verdict := Verdict{Passed: true}
	byStep := map[string]metrics.StepResult{}
	for _, step := range document.Steps {
		byStep[step.Name] = step
	}

	for _, rule := range rules {
		evaluation := evaluateRule(rule, document, byStep, baseline)
		if !evaluation.Passed {
			verdict.Passed = false
		}
		verdict.Evaluations = append(verdict.Evaluations, evaluation)
	}

	verdict.Undeclared = undeclared(rules, document, baseline)
	verdict.Sentence = phrase(verdict)
	return verdict
}

func evaluateRule(rule scenario.SLORule, document metrics.Document, byStep map[string]metrics.StepResult, baseline *Baseline) Evaluation {
	evaluation := Evaluation{
		Step:   targetName(rule),
		Metric: rule.Metric,
		Rule:   rule.Text,
		Limit:  rule.Limit,
		Unit:   rule.Unit,
	}

	if rule.Scope == scenario.ScopeRegression {
		return evaluateRegression(rule, evaluation, baseline)
	}

	var distribution metrics.Distribution
	var count, errors, successes int64
	var sampled, worked int64

	switch rule.Scope {
	case scenario.ScopeOverall:
		distribution = document.Overall.Reported()
		count, errors, successes = document.Overall.Count, document.Overall.Errors, document.Overall.Successes
		sampled, worked = count, count-errors
	case scenario.ScopeJourney:
		distribution = document.Journey.Reported()
		count = document.Journey.Completed
		sampled, worked = document.Journey.Started, document.Journey.Completed
	default:
		step, exists := byStep[rule.Step]
		if !exists {
			evaluation.NoData = true
			evaluation.Passed = false
			evaluation.Sentence = fmt.Sprintf("No data: the step %q produced no requests, so the rule %q cannot be checked.", rule.Step, rule.Text)
			return evaluation
		}
		distribution = step.Latency
		count, errors, successes = step.Count, step.Errors, step.Successes
		sampled, worked = count, count-errors
	}

	if evaluation, refused := refuseLatencyOverFailures(rule, evaluation, sampled, worked); refused {
		return evaluation
	}

	switch rule.Metric {
	case "p50":
		evaluation.Obtained = distribution.P50
	case "p75":
		evaluation.Obtained = distribution.P75
	case "p90":
		evaluation.Obtained = distribution.P90
	case "p95":
		evaluation.Obtained = distribution.P95
	case "p99":
		evaluation.Obtained = distribution.P99
	case "p99.9":
		evaluation.Obtained = distribution.P999
	case "max":
		evaluation.Obtained = distribution.Max
	case "errors":
		if count > 0 {
			evaluation.Obtained = float64(errors) / float64(count) * 100
		}
	case "success":
		if count > 0 {
			evaluation.Obtained = float64(successes) / float64(count) * 100
		}
	case "throughput":
		evaluation.Obtained = document.Overall.EffectiveRate
	}

	evaluation.Passed = compare(evaluation.Obtained, rule.Operator, rule.Limit)
	evaluation.Sentence = phraseEvaluation(evaluation, rule)
	return evaluation
}

// With a blocking caveat the rule is reported and does not fail the build:
// failing would blame the service for a difference the comparison cannot
// attribute to it.
func evaluateRegression(rule scenario.SLORule, evaluation Evaluation, baseline *Baseline) Evaluation {
	if baseline == nil {
		evaluation.NoData = true
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("No baseline: the rule %q needs a previous run. Run with -baseline=previous-run.json.", rule.Text)
		return evaluation
	}

	difference, found := differenceOf(baseline.Comparison, rule.Metric)
	if !found {
		evaluation.NoData = true
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("No baseline: %q does not exist in both runs, so the rule %q cannot be checked.", rule.Metric, rule.Text)
		return evaluation
	}

	evaluation.Obtained = worsePercentage(difference)
	evaluation.Passed = compare(evaluation.Obtained, rule.Operator, rule.Limit)

	if blocking := blockingCaveats(baseline.Comparison); len(blocking) > 0 {
		evaluation.Untrustworthy = true
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("No verdict: %s is %s than the baseline, but the comparison with %s is not trustworthy (%s), so the rule %q does not fail the build.",
			readableName(rule.Metric), changeText(evaluation.Obtained), baseline.Path, strings.Join(blocking, "; "), rule.Text)
		return evaluation
	}

	if difference.Direction == comparison.DirectionSame {
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("Passed: %s stayed within the noise against the baseline (a change below %.0f%% is not read as a regression).",
			readableName(rule.Metric), comparison.AcceptedNoise*100)
		return evaluation
	}
	evaluation.Sentence = phraseRegression(evaluation, rule, difference, baseline.Path)
	return evaluation
}

func differenceOf(compared comparison.Comparison, metric string) (comparison.Difference, bool) {
	prefix, percentile, found := splitRegressionMetric(metric)
	if !found {
		return comparison.Difference{}, false
	}
	var byPercentile map[string]comparison.Difference
	switch prefix {
	case "journey":
		byPercentile = compared.JourneyPercentiles
	case "global":
		byPercentile = compared.OverallPercentiles
	default:
		return comparison.Difference{}, false
	}
	difference, has := byPercentile[percentile]
	return difference, has
}

// Improvement reads as zero worsening, not as a negative that would pass any
// limit by accident.
func worsePercentage(difference comparison.Difference) float64 {
	if difference.Direction != comparison.DirectionWorse {
		return 0
	}
	return difference.Change * 100
}

func blockingCaveats(compared comparison.Comparison) []string {
	var blocking []string
	for _, caveat := range compared.Caveats {
		if caveat.Blocking {
			blocking = append(blocking, caveat.Text)
		}
	}
	return blocking
}

func changeText(percentage float64) string {
	if percentage == 0 {
		return "the same or better"
	}
	return fmt.Sprintf("%.1f%% worse", percentage)
}

func compare(obtained float64, operator scenario.Operator, limit float64) bool {
	switch operator {
	case scenario.OpLess:
		return obtained < limit
	case scenario.OpLessOrEqual:
		return obtained <= limit
	case scenario.OpGreater:
		return obtained > limit
	case scenario.OpGreaterOrEqual:
		return obtained >= limit
	case scenario.OpNotEqual:
		return obtained != limit
	default:
		return obtained == limit
	}
}

func targetName(rule scenario.SLORule) string {
	switch rule.Scope {
	case scenario.ScopeOverall:
		return "global"
	case scenario.ScopeJourney:
		return "journey"
	case scenario.ScopeRegression:
		return "regression"
	default:
		return rule.Step
	}
}

// The noun form, for the sentences that talk about the metric instead of
// reporting a value: "the response time of 95% of the responses is 12% worse
// than the baseline".
func readableName(metric string) string {
	switch metric {
	case "errors":
		return "the error rate"
	case "success":
		return "the success rate"
	case "throughput":
		return "the effective rate"
	case "max":
		return "the worst response time"
	}
	if prefix, percentile, found := splitRegressionMetric(metric); found {
		if prefix == "journey" {
			return "the response time of " + share(percentile) + " of the journeys"
		}
		return "the response time of " + share(percentile) + " of the responses"
	}
	return "the response time of " + share(metric) + " of the responses"
}

// journeyP95 is one word to the reader and two to the code: the scope it
// compares against, and the percentile it reads inside that scope.
func splitRegressionMetric(metric string) (string, string, bool) {
	for _, prefix := range []string{"journey", "global"} {
		if rest, found := strings.CutPrefix(metric, prefix); found && rest != "" {
			return prefix, strings.ToLower(rest), true
		}
	}
	return "", "", false
}

// "p95" is the term of the trade and means nothing to someone who never ran a
// load test — and this is the line that decides whether the CI passes.
func share(percentile string) string {
	if trimmed := strings.TrimPrefix(percentile, "p"); trimmed != percentile {
		return trimmed + "%"
	}
	return percentile
}

func format(value float64, unit string) string {
	switch unit {
	case "ms":
		return fmt.Sprintf("%.0f ms", value)
	case "%":
		return fmt.Sprintf("%.2f%%", value)
	case "/s":
		return fmt.Sprintf("%.0f/s", value)
	case "% worse":
		return fmt.Sprintf("%.0f%% worse", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func phraseEvaluation(evaluation Evaluation, rule scenario.SLORule) string {
	var target string
	switch rule.Scope {
	case scenario.ScopeOverall:
		target = "the whole scenario"
	case scenario.ScopeJourney:
		target = "the whole journey"
	default:
		target = fmt.Sprintf("%q", evaluation.Step)
	}
	comparison := "above the limit of"
	if rule.Operator == scenario.OpGreater || rule.Operator == scenario.OpGreaterOrEqual {
		comparison = "below the minimum of"
	}
	observed := observedPhrase(evaluation)
	if evaluation.Passed {
		within := "within the limit of"
		if rule.Operator == scenario.OpGreater || rule.Operator == scenario.OpGreaterOrEqual {
			within = "at the minimum of"
		}
		return fmt.Sprintf("Passed: %s %s, %s %s.",
			target, observed, within, format(evaluation.Limit, evaluation.Unit))
	}
	return fmt.Sprintf("Failed: %s %s, %s %s.",
		target, observed, comparison, format(evaluation.Limit, evaluation.Unit))
}

// A percentile reads as a share of the requests, not as a quantity something
// "had": "answered 95% within 6 ms" is the sentence the vocabulary fixes, and
// it is the line that decides whether the CI passes.
func observedPhrase(evaluation Evaluation) string {
	value := format(evaluation.Obtained, evaluation.Unit)
	if evaluation.Unit == "ms" && strings.HasPrefix(evaluation.Metric, "p") {
		return fmt.Sprintf("answered %s within %s", share(evaluation.Metric), value)
	}
	return fmt.Sprintf("had %s of %s", readableName(evaluation.Metric), value)
}

// The baseline is named because the report travels alone: pasted into a
// ticket, "worse than the baseline" does not say which one, and nobody can
// check it.
func phraseRegression(evaluation Evaluation, rule scenario.SLORule, difference comparison.Difference, base string) string {
	verb, side := "Failed", "above"
	if evaluation.Passed {
		verb, side = "Passed", "within"
	}
	return fmt.Sprintf("%s: %s came out %s than %s, %s the limit of %s (from %.0f ms to %.0f ms).",
		verb, readableName(rule.Metric), changeText(evaluation.Obtained), baseName(base), side,
		format(rule.Limit, rule.Unit), difference.Before, difference.After)
}

func baseName(path string) string {
	if strings.TrimSpace(path) == "" {
		return "the baseline"
	}
	return path
}

// A gate made only of step rules approves a scenario whose journey nobody
// measured, so what is missing is reported too — one line per scope, grouped,
// because a line per step would turn the useful part into noise.
func undeclared(rules []scenario.SLORule, document metrics.Document, baseline *Baseline) []string {
	if len(rules) == 0 {
		return []string{"no criterion declared — the scenario runs and reports, but does not work as a gate"}
	}
	declared := map[scenario.SLOScope]bool{}
	declaredStep := map[string]bool{}
	for _, rule := range rules {
		declared[rule.Scope] = true
		if rule.Scope == scenario.ScopeStep {
			declaredStep[rule.Step] = true
		}
	}

	var missing []string
	if !declared[scenario.ScopeJourney] && document.Journey.Started > 0 && len(document.Steps) > 1 {
		missing = append(missing, "journey: no criterion declared — the gate measures isolated steps and leaves out the wait the user feels")
	}
	if !declared[scenario.ScopeOverall] {
		missing = append(missing, "global: no criterion declared — error rate, success rate and effective rate stay out of the gate")
	}
	if line, had := stepsWithoutRule(document, declaredStep); had {
		missing = append(missing, line)
	}
	switch {
	case !declared[scenario.ScopeRegression]:
		missing = append(missing, "regression: no criterion declared — the gate approves without comparing against the previous run")
	case baseline == nil:
		missing = append(missing, "regression: declared, but no baseline was passed in -baseline")
	}
	return missing
}

func stepsWithoutRule(document metrics.Document, declaredStep map[string]bool) (string, bool) {
	var without []string
	for _, step := range document.Steps {
		if !declaredStep[step.Name] {
			without = append(without, step.Name)
		}
	}
	if len(without) == 0 {
		return "", false
	}
	return fmt.Sprintf("steps with no criterion declared (%d of %d): %s",
		len(without), len(document.Steps), shortList(without)), true
}

func shortList(names []string) string {
	const shown = 3
	if len(names) <= shown {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:shown], ", "), len(names)-shown)
}

func phrase(verdict Verdict) string {
	if len(verdict.Evaluations) == 0 {
		return "No SLO declared: nothing was checked."
	}
	var failures []string
	for _, evaluation := range verdict.Evaluations {
		if !evaluation.Passed {
			failures = append(failures, evaluation.Sentence)
		}
	}
	if len(failures) == 0 {
		if len(verdict.Evaluations) == 1 {
			return "Passed: the single SLO rule was met."
		}
		return fmt.Sprintf("Passed: all %d SLO rules were met.", len(verdict.Evaluations))
	}
	return strings.Join(failures, " ")
}

var latencyMetrics = []string{"p50", "p75", "p90", "p95", "p99", "p99.9", "max"}

// A percentile describes the work the scenario meant to measure only while the
// requests that worked are the majority of the sample. Below that every
// quantile is drawn mostly from failures, and the number is the time the target
// took to refuse — the same reason the sanity check already invalidates a step
// that failed in 100% of the requests. There is one histogram per step, so the
// criterion cannot be re-evaluated over the successes alone; what the gate can
// do is refuse to approve. A run with 98% of errors was passing a latency
// criterion with the p95 of an error page.
func refuseLatencyOverFailures(rule scenario.SLORule, evaluation Evaluation, sampled, worked int64) (Evaluation, bool) {
	if !slices.Contains(latencyMetrics, rule.Metric) {
		return evaluation, false
	}
	if sampled <= 0 || worked*2 >= sampled {
		return evaluation, false
	}
	evaluation.NoData = true
	evaluation.Passed = false
	evaluation.Sentence = fmt.Sprintf(
		"Not evaluated: %s failed %.0f%% of the time, so %s above is mostly the time to fail, not the time to do the work. The rule %q cannot be checked over this sample, and with no check the gate does not approve.",
		targetName(rule), float64(sampled-worked)/float64(sampled)*100, readableName(rule.Metric), rule.Text)
	return evaluation, true
}
