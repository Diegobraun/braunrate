package slo_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
)

func journeyDocument(journeyP95, journeyP99 float64) metrics.Document {
	return metrics.Document{
		Journey: metrics.Journey{
			Started: 1000, Completed: 1000,
			Latency: metrics.Distribution{P95: journeyP95, P99: journeyP99},
		},
		Steps: []metrics.StepResult{
			{Name: "consultar pedido", Count: 1000, Successes: 999, Errors: 1,
				Latency: metrics.Distribution{P95: 40}},
			{Name: "pagar fatura", Count: 1000, Successes: 1000,
				Latency: metrics.Distribution{P95: 40}},
		},
		Overall: metrics.OverallResult{Count: 2000, Successes: 1999, Errors: 1, EffectiveRate: 200},
	}
}

func parse(t *testing.T, target, metric, limit string) scenario.SLORule {
	t.Helper()
	rule, err := scenario.ParseSLORule(target, metric, limit)
	if err != nil {
		t.Fatalf("regra %s/%s invalida: %v", target, metric, err)
	}
	return rule
}

// The whole point of the journey criterion: each step is fast and the sum is
// not. A gate made of step rules approves this run.
func TestJourneyCriterionCatchesWhatStepRulesApprove(t *testing.T) {
	document := journeyDocument(1800, 4000)

	stepsOnly := slo.Evaluate([]scenario.SLORule{
		parse(t, "consultar pedido", "p95", "< 150ms"),
		parse(t, "pagar fatura", "p95", "< 150ms"),
	}, document, nil)
	if !stepsOnly.Passed {
		t.Fatal("as regras de passo deveriam passar; sem isso o teste nao mostra nada")
	}

	withJourney := slo.Evaluate([]scenario.SLORule{
		parse(t, "consultar pedido", "p95", "< 150ms"),
		parse(t, "jornada", "p95", "< 1s"),
	}, document, nil)
	if withJourney.Passed {
		t.Fatal("jornada de 1800 ms passou por um limite de 1 s")
	}
	if !strings.Contains(withJourney.Sentence, "a jornada inteira") {
		t.Errorf("a frase nao diz que quem falhou foi a jornada: %q", withJourney.Sentence)
	}
}

func TestUndeclaredCriteriaAreReportedAsInformation(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{
		parse(t, "consultar pedido", "p95", "< 150ms"),
	}, journeyDocument(100, 200), nil)

	together := strings.Join(verdict.Undeclared, " | ")
	for _, expected := range []string{"jornada: sem criterio declarado", "global: sem criterio declarado", "regressao: sem criterio declarado"} {
		if !strings.Contains(together, expected) {
			t.Errorf("faltou declarar a ausencia de %q: %v", expected, verdict.Undeclared)
		}
	}
	if !verdict.Passed {
		t.Error("criterio nao declarado nao pode reprovar a execucao, so ser informado")
	}
}

func TestSuccessRateReadsTheSameWayAsErrorRate(t *testing.T) {
	document := journeyDocument(100, 200)

	success := slo.Evaluate([]scenario.SLORule{parse(t, "global", "sucesso", ">= 99.9")}, document, nil)
	errors := slo.Evaluate([]scenario.SLORule{parse(t, "global", "erros", "< 0.1")}, document, nil)

	if success.Passed != errors.Passed {
		t.Fatalf("sucesso >= 99.9 e erros < 0.1 discordaram sobre a mesma execucao: %v e %v",
			success.Passed, errors.Passed)
	}
	if !strings.Contains(success.Evaluations[0].Sentence, "taxa de sucesso") {
		t.Errorf("a regra declarada como sucesso foi exibida como outra coisa: %q", success.Evaluations[0].Sentence)
	}
	if !strings.Contains(errors.Evaluations[0].Sentence, "taxa de erro") {
		t.Errorf("a regra declarada como erro foi exibida como outra coisa: %q", errors.Evaluations[0].Sentence)
	}
}

func TestThroughputCriterionUsesTheEffectiveRate(t *testing.T) {
	document := journeyDocument(100, 200)

	if verdict := slo.Evaluate([]scenario.SLORule{parse(t, "global", "taxa_efetiva", ">= 200/s")}, document, nil); !verdict.Passed {
		t.Errorf("200/s efetivos falharam num minimo de 200/s: %q", verdict.Sentence)
	}
	if verdict := slo.Evaluate([]scenario.SLORule{parse(t, "global", "taxa_efetiva", ">= 500/s")}, document, nil); verdict.Passed {
		t.Error("200/s efetivos passaram num minimo de 500/s")
	}
}

// Throughput below target is ambiguous on purpose: the target may not have
// taken it, or the generator may not have produced it. The second case is an
// invalid measurement, decided before this package runs, so a saturated run
// never reaches the SLO to be reported as a service failure.
func TestSaturatedRunNeverBecomesAnSLOFailure(t *testing.T) {
	document := journeyDocument(100, 200)
	document.Sanity = metrics.Sanity{
		Checked: true, Valid: false,
		Findings: []metrics.SanityFinding{{Kind: "gerador_saturado"}},
	}
	if document.Valid() {
		t.Fatal("execucao saturada se declarou valida")
	}
}

func regressionBaseline(before, after float64, caveats ...comparison.Caveat) *slo.Baseline {
	worse := comparison.Difference{
		Metrica: "jornada inteira (p95)", Before: before, After: after,
		Change: (after - before) / before, Direction: comparison.DirectionWorse,
	}
	if worse.Change < comparison.AcceptedNoise {
		worse.Direction = comparison.DirectionSame
	}
	return &slo.Baseline{
		Path:       "antes.json",
		Comparison: comparison.Comparison{JourneyPercentiles: map[string]comparison.Difference{"p95": worse}, Caveats: caveats},
	}
}

func TestRegressionGateFailsOnlyWhenTheComparisonHolds(t *testing.T) {
	rule := []scenario.SLORule{parse(t, "regressao", "jornada_p95", "<= 10% pior")}
	document := journeyDocument(100, 200)

	within := slo.Evaluate(rule, document, regressionBaseline(100, 108))
	if !within.Passed {
		t.Errorf("8%% pior reprovou num limite de 10%%: %q", within.Sentence)
	}

	beyond := slo.Evaluate(rule, document, regressionBaseline(100, 140))
	if beyond.Passed {
		t.Errorf("40%% pior passou num limite de 10%%: %q", beyond.Sentence)
	}

	noise := slo.Evaluate(rule, document, regressionBaseline(100, 103))
	if !noise.Passed || !strings.Contains(noise.Evaluations[0].Sentence, "ruido") {
		t.Errorf("3%% de diferenca precisa ser lido como ruido: %q", noise.Evaluations[0].Sentence)
	}
}

// A caveat that explains the difference by itself takes the verdict away from
// the gate: blaming the service for another machine would be a wrong claim,
// and passing in silence would hide that nothing was compared.
func TestUntrustworthyComparisonWarnsInsteadOfFailing(t *testing.T) {
	rule := []scenario.SLORule{parse(t, "regressao", "jornada_p95", "<= 10% pior")}
	blocked := regressionBaseline(100, 900, comparison.Caveat{
		Text: "as maquinas geradoras sao diferentes", Blocking: true,
	})

	verdict := slo.Evaluate(rule, journeyDocument(100, 200), blocked)
	if !verdict.Passed {
		t.Error("regressao reprovou com a comparacao declarada nao confiavel")
	}
	evaluation := verdict.Evaluations[0]
	if !evaluation.Untrustworthy {
		t.Error("a avaliacao nao ficou marcada como nao confiavel")
	}
	if !strings.Contains(evaluation.Sentence, "nao e confiavel") || !strings.Contains(evaluation.Sentence, "maquinas geradoras") {
		t.Errorf("a frase precisa dizer por que nao ha veredito: %q", evaluation.Sentence)
	}
}

// The single-token caveat is present on every authenticated scenario. Treating
// it as blocking would disable the regression gate for exactly the scenarios
// that most need it.
func TestReadingCaveatDoesNotDisableTheGate(t *testing.T) {
	rule := []scenario.SLORule{parse(t, "regressao", "jornada_p95", "<= 10% pior")}
	withReadingCaveat := regressionBaseline(100, 900, comparison.Caveat{
		Text: "as duas execucoes usaram um token para tudo",
	})

	verdict := slo.Evaluate(rule, journeyDocument(100, 200), withReadingCaveat)
	if verdict.Passed {
		t.Error("ressalva de leitura desligou o gate de regressao")
	}
}

func TestRegressionWithoutBaselineSaysSoInsteadOfPassing(t *testing.T) {
	verdict := slo.Evaluate([]scenario.SLORule{parse(t, "regressao", "jornada_p95", "<= 10% pior")},
		journeyDocument(100, 200), nil)

	if !strings.Contains(verdict.Evaluations[0].Sentence, "-baseline") {
		t.Errorf("sem base, a regra precisa ensinar como passar uma: %q", verdict.Evaluations[0].Sentence)
	}
	if !verdict.Evaluations[0].NoData {
		t.Error("regra sem base precisa ficar marcada como sem dados")
	}
}

func TestGateWarnsWhenTheJourneyIsLeftOut(t *testing.T) {
	spec := scenario.Spec{
		Steps: []scenario.Step{{Name: "consultar pedido"}, {Name: "pagar fatura"}},
		SLO:   []scenario.SLORule{parse(t, "consultar pedido", "p95", "< 150ms")},
	}
	warnings := strings.Join(scenario.GateWarnings(spec), " ")
	if !strings.Contains(warnings, "deixa de fora a jornada inteira") {
		t.Errorf("cenario com 2 passos e sem criterio de jornada precisa avisar: %q", warnings)
	}

	spec.SLO = append(spec.SLO, parse(t, "jornada", "p95", "< 2s"))
	if warnings := scenario.GateWarnings(spec); len(warnings) > 0 {
		t.Errorf("com criterio de jornada declarado nao ha o que avisar: %v", warnings)
	}

	single := scenario.Spec{
		Steps: []scenario.Step{{Name: "consultar pedido"}},
		SLO:   []scenario.SLORule{parse(t, "consultar pedido", "p95", "< 150ms")},
	}
	if warnings := scenario.GateWarnings(single); len(warnings) > 0 {
		t.Errorf("cenario de um passo so nao tem jornada a declarar: %v", warnings)
	}
}

func TestMetricThatMeasuresSomethingElseIsRefused(t *testing.T) {
	if _, err := scenario.ParseSLORule("consultar pedido", "taxa_efetiva", ">= 50/s"); err == nil {
		t.Error("taxa efetiva por passo era medida com o numero global e passava calada")
	}
	if _, err := scenario.ParseSLORule("jornada", "erros", "< 0.1"); err == nil {
		t.Error("taxa de erro da jornada nao existe e precisa ser recusada")
	}
	if _, err := scenario.ParseSLORule("regressao", "p95", "<= 10% pior"); err == nil {
		t.Error("metrica de regressao precisa dizer de qual grupo: jornada_p95 ou global_p95")
	}
}
