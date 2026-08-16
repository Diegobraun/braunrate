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
		Step:    targetName(rule),
		Metrica: rule.Metrica,
		Rule:    rule.Text,
		Limit:   rule.Limit,
		Unit:    rule.Unit,
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
			evaluation.Sentence = fmt.Sprintf("Sem dados: o passo %q nao produziu nenhuma requisicao, entao a regra %q nao pode ser verificada.", rule.Step, rule.Text)
			return evaluation
		}
		distribution = step.Latency
		count, errors, successes = step.Count, step.Errors, step.Successes
		sampled, worked = count, count-errors
	}

	if evaluation, refused := refuseLatencyOverFailures(rule, evaluation, sampled, worked); refused {
		return evaluation
	}

	switch rule.Metrica {
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
	case "erros":
		if count > 0 {
			evaluation.Obtained = float64(errors) / float64(count) * 100
		}
	case "sucesso":
		if count > 0 {
			evaluation.Obtained = float64(successes) / float64(count) * 100
		}
	case "vazao", "taxa_efetiva":
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
		evaluation.Sentence = fmt.Sprintf("Sem base: a regra %q precisa de uma execucao anterior. Rode com -baseline=execucao-anterior.json.", rule.Text)
		return evaluation
	}

	difference, found := differenceOf(baseline.Comparison, rule.Metrica)
	if !found {
		evaluation.NoData = true
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("Sem base: %q nao existe nas duas execucoes, entao a regra %q nao pode ser verificada.", rule.Metrica, rule.Text)
		return evaluation
	}

	evaluation.Obtained = worsePercentage(difference)
	evaluation.Passed = compare(evaluation.Obtained, rule.Operator, rule.Limit)

	if blocking := blockingCaveats(baseline.Comparison); len(blocking) > 0 {
		evaluation.Untrustworthy = true
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("Sem veredito: %s esta %s que a base, mas a comparacao com %s nao e confiavel (%s), entao a regra %q nao reprova.",
			readableName(rule.Metrica), changeText(evaluation.Obtained), baseline.Path, strings.Join(blocking, "; "), rule.Text)
		return evaluation
	}

	if difference.Direction == comparison.DirectionSame {
		evaluation.Passed = true
		evaluation.Sentence = fmt.Sprintf("Passou: %s ficou dentro do ruido em relacao a base (variacao abaixo de %.0f%% nao e lida como regressao).",
			readableName(rule.Metrica), comparison.AcceptedNoise*100)
		return evaluation
	}
	evaluation.Sentence = phraseRegression(evaluation, rule, difference, baseline.Path)
	return evaluation
}

func differenceOf(compared comparison.Comparison, metric string) (comparison.Difference, bool) {
	prefix, percentile, _ := strings.Cut(metric, "_")
	var byPercentile map[string]comparison.Difference
	switch prefix {
	case "jornada":
		byPercentile = compared.JourneyPercentiles
	case "global":
		byPercentile = compared.OverallPercentiles
	default:
		return comparison.Difference{}, false
	}
	difference, found := byPercentile[percentile]
	return difference, found
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
		return "igual ou melhor"
	}
	return fmt.Sprintf("%.1f%% pior", percentage)
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
		return "jornada"
	case scenario.ScopeRegression:
		return "regressao"
	default:
		return rule.Step
	}
}

// The noun form, for the sentences that talk about the metric instead of
// reporting a value: "o tempo de resposta de 95% das respostas esta 12% pior
// que a base".
func readableName(metric string) string {
	switch metric {
	case "erros":
		return "taxa de erro"
	case "sucesso":
		return "taxa de sucesso"
	case "vazao", "taxa_efetiva":
		return "taxa efetiva"
	case "max":
		return "o pior tempo de resposta"
	}
	if prefix, percentile, found := strings.Cut(metric, "_"); found {
		if prefix == "jornada" {
			return "o tempo de resposta de " + share(percentile) + " das jornadas"
		}
		return "o tempo de resposta de " + share(percentile) + " das respostas"
	}
	return "o tempo de resposta de " + share(metric) + " das respostas"
}

// "p95" is the term of the trade and means nothing to someone who never ran a
// load test — and this is the line that decides whether the CI passes.
func share(percentile string) string {
	if trimmed := strings.TrimPrefix(percentile, "p"); trimmed != percentile {
		return strings.Replace(trimmed, ".", ",", 1) + "%"
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
	case "% pior":
		return fmt.Sprintf("%.0f%% pior", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func phraseEvaluation(evaluation Evaluation, rule scenario.SLORule) string {
	var target string
	switch rule.Scope {
	case scenario.ScopeOverall:
		target = "o cenario inteiro"
	case scenario.ScopeJourney:
		target = "a jornada inteira"
	default:
		target = fmt.Sprintf("%q", evaluation.Step)
	}
	comparison := "acima do limite de"
	if rule.Operator == scenario.OpGreater || rule.Operator == scenario.OpGreaterOrEqual {
		comparison = "abaixo do minimo de"
	}
	observed := observedPhrase(evaluation)
	if evaluation.Passed {
		within := "dentro do limite de"
		if rule.Operator == scenario.OpGreater || rule.Operator == scenario.OpGreaterOrEqual {
			within = "no minimo de"
		}
		return fmt.Sprintf("Passou: %s %s, %s %s.",
			target, observed, within, format(evaluation.Limit, evaluation.Unit))
	}
	return fmt.Sprintf("Falhou: %s %s, %s %s.",
		target, observed, comparison, format(evaluation.Limit, evaluation.Unit))
}

// A percentile reads as a share of the requests, not as a quantity something
// "had": "respondeu 95% em ate 6 ms" is the sentence the vocabulary fixes, and
// it is the line that decides whether the CI passes.
func observedPhrase(evaluation Evaluation) string {
	value := format(evaluation.Obtained, evaluation.Unit)
	if evaluation.Unit == "ms" && strings.HasPrefix(evaluation.Metrica, "p") {
		return fmt.Sprintf("respondeu %s em ate %s", share(evaluation.Metrica), value)
	}
	return fmt.Sprintf("teve %s de %s", readableName(evaluation.Metrica), value)
}

// The base is named because the report travels alone: pasted into a ticket,
// "pior que a base" does not say which base, and nobody can check it.
func phraseRegression(evaluation Evaluation, rule scenario.SLORule, difference comparison.Difference, base string) string {
	verb, side := "Falhou", "acima"
	if evaluation.Passed {
		verb, side = "Passou", "dentro"
	}
	return fmt.Sprintf("%s: %s ficou %s que %s, %s do limite de %s (de %.0f ms para %.0f ms).",
		verb, readableName(rule.Metrica), changeText(evaluation.Obtained), baseName(base), side,
		format(rule.Limit, rule.Unit), difference.Before, difference.After)
}

func baseName(path string) string {
	if strings.TrimSpace(path) == "" {
		return "a base"
	}
	return path
}

// A gate made only of step rules approves a scenario whose journey nobody
// measured, so what is missing is reported too — one line per scope, grouped,
// because a line per step would turn the useful part into noise.
func undeclared(rules []scenario.SLORule, document metrics.Document, baseline *Baseline) []string {
	if len(rules) == 0 {
		return []string{"nenhum criterio declarado — o cenario roda e reporta, mas nao serve de gate"}
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
		missing = append(missing, "jornada: sem criterio declarado — o gate mede passo isolado e deixa de fora o tempo que o usuario espera")
	}
	if !declared[scenario.ScopeOverall] {
		missing = append(missing, "global: sem criterio declarado — taxa de erro, taxa de sucesso e taxa efetiva nao entram no gate")
	}
	if line, had := stepsWithoutRule(document, declaredStep); had {
		missing = append(missing, line)
	}
	switch {
	case !declared[scenario.ScopeRegression]:
		missing = append(missing, "regressao: sem criterio declarado — o gate aprova sem comparar com a execucao anterior")
	case baseline == nil:
		missing = append(missing, "regressao: declarada, mas nenhuma base foi passada em -baseline")
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
	return fmt.Sprintf("passos sem criterio declarado (%d de %d): %s",
		len(without), len(document.Steps), shortList(without)), true
}

func shortList(names []string) string {
	const shown = 3
	if len(names) <= shown {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s e mais %d", strings.Join(names[:shown], ", "), len(names)-shown)
}

func phrase(verdict Verdict) string {
	if len(verdict.Evaluations) == 0 {
		return "Sem SLO declarado: nada foi verificado."
	}
	var failures []string
	for _, evaluation := range verdict.Evaluations {
		if !evaluation.Passed {
			failures = append(failures, evaluation.Sentence)
		}
	}
	if len(failures) == 0 {
		if len(verdict.Evaluations) == 1 {
			return "Passou: a unica regra de SLO foi atendida."
		}
		return fmt.Sprintf("Passou: as %d regras de SLO foram atendidas.", len(verdict.Evaluations))
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
	if !slices.Contains(latencyMetrics, rule.Metrica) {
		return evaluation, false
	}
	if sampled <= 0 || worked*2 >= sampled {
		return evaluation, false
	}
	evaluation.NoData = true
	evaluation.Passed = false
	evaluation.Sentence = fmt.Sprintf(
		"Nao avaliada: %s teve %.0f%% de falha, entao a %s acima e sobretudo o tempo de falhar, nao o tempo do trabalho. A regra %q nao pode ser verificada sobre esta amostra, e sem verificacao o gate nao aprova.",
		targetName(rule), float64(sampled-worked)/float64(sampled)*100, readableName(rule.Metrica), rule.Text)
	return evaluation, true
}
