package slo_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
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

	expected := `Falhou: "consultar pedido" teve latencia p95 de 210 ms, acima do limite de 150 ms.`
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
		rule("passo que nao existe", "p95", scenario.OpLess, 100, "ms"),
	}, sampleDocument(), nil)

	if verdict.Passed {
		t.Fatal("regra apontando para passo inexistente nao pode passar em silencio")
	}
	if !strings.Contains(verdict.Sentence, "nao produziu nenhuma requisicao") {
		t.Errorf("frase = %q", verdict.Sentence)
	}
}

func TestNoRulesMeansNoVerdict(t *testing.T) {
	verdict := slo.Evaluate(nil, sampleDocument(), nil)
	if !verdict.Passed {
		t.Error("sem regras declaradas nao ha o que falhar")
	}
	if !strings.Contains(verdict.Sentence, "Sem SLO declarado") {
		t.Errorf("frase = %q", verdict.Sentence)
	}
}
