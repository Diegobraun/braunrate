package slo_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
)

func sampleDocument() metrics.Document {
	return metrics.Document{
		Steps: []metrics.StepResult{
			{Name: "consultar pedido", Count: 1000, Errors: 0,
				Latency: metrics.Distribution{P95: 210, P99: 300}},
			{Name: "criar pedido", Count: 1000, Errors: 5,
				Latency: metrics.Distribution{P95: 90, P99: 120}},
		},
		Overall: metrics.OverallResult{Count: 2000, Errors: 5, ErrorRate: 0.0025,
			EffectiveRate: 800, Latency: metrics.Distribution{P95: 150, P99: 260}},
	}
}

func scopeOf(step string) scenario.SLOScope {
	switch step {
	case "global":
		return scenario.ScopeOverall
	case "jornada":
		return scenario.ScopeJourney
	default:
		return scenario.ScopeStep
	}
}

func rule(step, metricName string, operator scenario.Operator, limit float64, unit string) scenario.SLORule {
	return scenario.SLORule{Step: step, Scope: scopeOf(step), Metrica: metricName,
		Operator: operator, Limit: limit, Unit: unit,
		Text: metricName + " " + string(operator) + " limite"}
}

func TestSLOThatPassesAndThatFails(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{
		rule("consultar pedido", "p95", scenario.OpLess, 150, "ms"),
		rule("criar pedido", "p95", scenario.OpLess, 150, "ms"),
	}, sampleDocument(), nil)

	if verdict.Passed {
		t.Fatal("o veredito deveria falhar: consultar pedido teve p95 de 210 ms")
	}
	if len(verdict.Evaluations) != 2 {
		t.Fatalf("avaliacoes = %d", len(verdict.Evaluations))
	}
	if verdict.Evaluations[0].Passed || !verdict.Evaluations[1].Passed {
		t.Errorf("avaliacoes erradas: %+v", verdict.Evaluations)
	}
}

func TestFailureSentenceIsReadableByNonEngineers(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{
		rule("consultar pedido", "p95", scenario.OpLess, 150, "ms"),
	}, sampleDocument(), nil)

	expected := `Falhou: "consultar pedido" respondeu 95% em até 210 ms, acima do limite de 150 ms.`
	if verdict.Sentence != expected {
		t.Errorf("frase = %q\nesperada = %q", verdict.Sentence, expected)
	}
}

func TestErrorRateIsEvaluatedAsPercentage(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{
		rule("criar pedido", "erros", scenario.OpLessOrEqual, 0, "%"),
	}, sampleDocument(), nil)

	if verdict.Passed {
		t.Fatal("criar pedido teve 5 erros em 1000; a regra de 0% deveria falhar")
	}
	if !strings.Contains(verdict.Sentence, "0.50%") {
		t.Errorf("a frase precisa dizer a taxa obtida: %q", verdict.Sentence)
	}
}

func TestOverallRuleUsesWholeScenarioNumbers(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{
		rule("global", "erros", scenario.OpLess, 1, "%"),
		rule("global", "p99", scenario.OpLess, 300, "ms"),
	}, sampleDocument(), nil)

	if !verdict.Passed {
		t.Fatalf("as duas regras globais deveriam passar: %+v", verdict.Evaluations)
	}
	if !strings.Contains(verdict.Sentence, "2 regras") {
		t.Errorf("frase = %q", verdict.Sentence)
	}
}

func TestUnknownStepFailsWithClearMessage(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{
		rule("passo que não existe", "p95", scenario.OpLess, 100, "ms"),
	}, sampleDocument(), nil)

	if verdict.Passed {
		t.Fatal("regra apontando para passo inexistente não pode passar em silencio")
	}
	if !strings.Contains(verdict.Sentence, "não produziu nenhuma requisição") {
		t.Errorf("frase = %q", verdict.Sentence)
	}
}

func TestNoRulesMeansNoVerdict(t *testing.T) {
	verdict := slo.Evaluate(nil, sampleDocument(), nil)
	if !verdict.Passed {
		t.Error("sem regras declaradas não há o que falhar")
	}
	if !strings.Contains(verdict.Sentence, "Sem SLO declarado") {
		t.Errorf("frase = %q", verdict.Sentence)
	}
}

// A run where 98% of the requests failed was approving a latency criterion with
// the p95 of an error page: the failures are fast, they fill the sample, and
// every quantile stops describing the work the scenario meant to measure.
func TestLatencyRuleIsNotApprovedOverASampleOfFailures(t *testing.T) {
	document := metrics.Document{
		Steps: []metrics.StepResult{{
			Name: "consultar", Count: 500, Errors: 490, Successes: 10,
			Latency: metrics.Distribution{P50: 0.4, P95: 1.4, P99: 301},
		}},
		Overall: metrics.OverallResult{Count: 500, Errors: 490, ErrorRate: 0.98},
	}
	rules := []scenario.SLORule{{
		Scope: scenario.ScopeStep, Step: "consultar", Metrica: "p95",
		Operator: scenario.OpLess, Limit: 200, Unit: "ms", Text: "p95: < 200ms",
	}}

	verdict := slo.Evaluate(rules, document, nil)
	if verdict.Passed {
		t.Fatal("o gate aprovou latência num passo que falhou em 98% das requisições")
	}
	sentence := verdict.Evaluations[0].Sentence
	for _, expected := range []string{"98% de falha", "tempo de falhar"} {
		if !strings.Contains(sentence, expected) {
			t.Errorf("a frase não explica por que não avaliou: falta %q em %q", expected, sentence)
		}
	}
}

// The majority is the line: while the requests that worked are most of the
// sample the percentile still describes them, and a criterion that stopped
// being evaluated on any run with errors would be a criterion nobody declares.
func TestLatencyRuleIsStillEvaluatedWhileMostRequestsWork(t *testing.T) {
	document := metrics.Document{
		Steps: []metrics.StepResult{{
			Name: "consultar", Count: 1000, Errors: 300, Successes: 700,
			Latency: metrics.Distribution{P95: 120},
		}},
	}
	rules := []scenario.SLORule{{
		Scope: scenario.ScopeStep, Step: "consultar", Metrica: "p95",
		Operator: scenario.OpLess, Limit: 200, Unit: "ms", Text: "p95: < 200ms",
	}}

	verdict := slo.Evaluate(rules, document, nil)
	if !verdict.Passed {
		t.Fatalf("30%% de erro deixou de avaliar a latência: %q", verdict.Evaluations[0].Sentence)
	}
}

// The error rate itself is a fact about the sample, not a reading of it: a
// criterion on errors has to keep working precisely when everything failed.
func TestErrorRuleIsStillEvaluatedWhenEverythingFails(t *testing.T) {
	document := metrics.Document{
		Steps:   []metrics.StepResult{{Name: "consultar", Count: 500, Errors: 490, Successes: 10}},
		Overall: metrics.OverallResult{Count: 500, Errors: 490},
	}
	rules := []scenario.SLORule{{
		Scope: scenario.ScopeOverall, Metrica: "erros",
		Operator: scenario.OpLess, Limit: 1, Unit: "%", Text: "erros: < 1",
	}}

	verdict := slo.Evaluate(rules, document, nil)
	if verdict.Passed {
		t.Fatal("98% de erro passou no critério de erro")
	}
	if verdict.Evaluations[0].NoData {
		t.Fatalf("o critério de erro deixou de ser avaliado: %q", verdict.Evaluations[0].Sentence)
	}
}

// The journey histogram records the ones that aborted too, so a journey
// criterion has the same hole: most journeys stopping at the first step in a
// millisecond would approve a p95 that no user ever waited.
func TestJourneyRuleIsNotApprovedWhenMostJourneysAbort(t *testing.T) {
	document := metrics.Document{
		Journey: metrics.Journey{Started: 500, Completed: 10,
			Latency: metrics.Distribution{P95: 2}},
	}
	rules := []scenario.SLORule{{
		Scope: scenario.ScopeJourney, Metrica: "p95",
		Operator: scenario.OpLess, Limit: 2000, Unit: "ms", Text: "p95: < 2s",
	}}

	verdict := slo.Evaluate(rules, document, nil)
	if verdict.Passed {
		t.Fatal("o gate aprovou a jornada com 490 de 500 jornadas interrompidas")
	}
}

// The report travels alone: pasted into a ticket, "pior que a base" does not
// say which base, and nobody can check it.
func TestRegressionSentenceNamesTheBaseline(t *testing.T) {
	document := metrics.Document{
		Journey: metrics.Journey{Started: 100, Completed: 100,
			Latency: metrics.Distribution{P95: 31}},
	}
	before := metrics.Document{
		Journey: metrics.Journey{Started: 100, Completed: 100,
			Latency: metrics.Distribution{P95: 2}},
	}
	rules := []scenario.SLORule{{
		Scope: scenario.ScopeRegression, Metrica: "jornada_p95",
		Operator: scenario.OpLess, Limit: 20, Unit: "%", Text: "jornada_p95: < 20",
	}}

	base := &slo.Baseline{Comparison: comparison.Compare(before, document), Path: "antes-do-cache.json"}
	verdict := slo.Evaluate(rules, document, base)
	if verdict.Passed {
		t.Fatal("regressão de 15 vezes passou no gate")
	}
	if !strings.Contains(verdict.Evaluations[0].Sentence, "antes-do-cache.json") {
		t.Errorf("a frase não diz contra qual base comparou: %q", verdict.Evaluations[0].Sentence)
	}
}
