package slo

import (
	"fmt"
	"strings"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Verdict = metrics.Verdict

type Evaluation = metrics.Evaluation

// Evaluate assumes the run already passed the sanity check: whether the result
// means anything is decided before any rule is read, in metrics.CheckSanity,
// and a run that failed there never reaches here.
func Evaluate(rules []scenario.SLORule, document metrics.Document) Verdict {
	verdict := Verdict{Passed: true}
	byStep := map[string]metrics.StepResult{}
	for _, step := range document.Steps {
		byStep[step.Name] = step
	}

	for _, rule := range rules {
		evaluation := avaliarRegra(rule, document, byStep)
		if !evaluation.Passed {
			verdict.Passed = false
		}
		verdict.Evaluations = append(verdict.Evaluations, evaluation)
	}

	verdict.Sentence = phrase(verdict)
	return verdict
}

func avaliarRegra(rule scenario.SLORule, document metrics.Document, byStep map[string]metrics.StepResult) Evaluation {
	evaluation := Evaluation{
		Step:    targetName(rule),
		Metrica: rule.Metrica,
		Rule:    rule.Text,
		Limit:   rule.Limit,
		Unit:    rule.Unit,
	}

	var distribution metrics.Distribution
	var count, errors int64

	if rule.Overall {
		distribution = document.Overall.Latency
		count, errors = document.Overall.Count, document.Overall.Errors
	} else {
		step, exists := byStep[rule.Step]
		if !exists {
			evaluation.NoData = true
			evaluation.Passed = false
			evaluation.Sentence = fmt.Sprintf("Sem dados: o passo %q nao produziu nenhuma requisicao, entao a regra %q nao pode ser verificada.", rule.Step, rule.Text)
			return evaluation
		}
		distribution = step.Latency
		count, errors = step.Count, step.Errors
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
	case "vazao":
		evaluation.Obtained = document.Overall.EffectiveRate
	}

	evaluation.Passed = compare(evaluation.Obtained, rule.Operator, rule.Limit)
	evaluation.Sentence = phraseEvaluation(evaluation, rule)
	return evaluation
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
	if rule.Overall {
		return "global"
	}
	return rule.Step
}

func readableName(metric string) string {
	switch metric {
	case "erros":
		return "taxa de erro"
	case "vazao":
		return "vazao"
	case "max":
		return "latencia maxima"
	default:
		return "latencia " + metric
	}
}

func format(value float64, unit string) string {
	switch unit {
	case "ms":
		return fmt.Sprintf("%.0f ms", value)
	case "%":
		return fmt.Sprintf("%.2f%%", value)
	case "/s":
		return fmt.Sprintf("%.0f/s", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func phraseEvaluation(evaluation Evaluation, rule scenario.SLORule) string {
	target := evaluation.Step
	if rule.Overall {
		target = "o cenario inteiro"
	} else {
		target = fmt.Sprintf("%q", target)
	}
	comparison := "acima do limite de"
	if rule.Operator == scenario.OpGreater || rule.Operator == scenario.OpGreaterOrEqual {
		comparison = "abaixo do minimo de"
	}
	if evaluation.Passed {
		return fmt.Sprintf("Passou: %s teve %s de %s, dentro do limite de %s.",
			target, readableName(evaluation.Metrica), format(evaluation.Obtained, evaluation.Unit), format(evaluation.Limit, evaluation.Unit))
	}
	return fmt.Sprintf("Falhou: %s teve %s de %s, %s %s.",
		target, readableName(evaluation.Metrica), format(evaluation.Obtained, evaluation.Unit),
		comparison, format(evaluation.Limit, evaluation.Unit))
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
