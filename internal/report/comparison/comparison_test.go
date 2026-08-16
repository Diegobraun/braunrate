package comparison_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
)

func document(journeyP95, stepP95 float64) metrics.Document {
	start := time.Date(2026, 8, 15, 22, 0, 0, 0, time.UTC)
	return metrics.Document{
		Tool:        "braunrate",
		Version:     "0.3.0",
		Environment: metrics.Environment{Host: "maquina-de-teste", Cores: 10},
		Run: metrics.Run{
			Spec: "Jornada de cobranca", Target: "http://127.0.0.1:8080", Start: start,
			AppliedPlan: []metrics.AppliedPhase{{Kind: "patamar", To: 300, DurationMs: 10000}},
		},
		Journey: metrics.Journey{Started: 1500, Completed: 1500, Latency: metrics.Distribution{P95: journeyP95}},
		Steps: []metrics.StepResult{
			{Name: "consultar pedido", Count: 1500, Latency: metrics.Distribution{P95: stepP95, P99: stepP95 * 1.2}},
		},
		Overall: metrics.OverallResult{Count: 1500, Latency: metrics.Distribution{P95: stepP95}},
	}
}

func TestRegressionAppearsFirstInPlainLanguage(t *testing.T) {
	c := comparison.Compare(document(10, 5), document(20, 10))
	if !strings.HasPrefix(c.Sentence, "Ficou mais lento") {
		t.Errorf("a primeira frase precisa dizer o que aconteceu: %q", c.Sentence)
	}
	if !strings.Contains(c.Sentence, "de 10 ms para 20 ms") {
		t.Errorf("a frase precisa trazer os dois numeros: %q", c.Sentence)
	}
	if c.Journey.Direction != comparison.DirectionWorse {
		t.Errorf("sentido veio %q", c.Journey.Direction)
	}
}

func TestImprovementIsDeclaredToo(t *testing.T) {
	c := comparison.Compare(document(20, 10), document(10, 5))
	if !strings.HasPrefix(c.Sentence, "Ficou mais rapido") {
		t.Errorf("melhora precisa ser dita com a mesma clareza: %q", c.Sentence)
	}
}

// Two runs give no confidence interval; calling a small change a regression
// would invent precision the measurement does not have.
func TestSmallChangeIsTreatedAsNoise(t *testing.T) {
	c := comparison.Compare(document(10, 5), document(10.3, 5.1))
	if c.Journey.Direction != comparison.DirectionSame {
		t.Errorf("3%% de diferenca nao e regressao: %q", c.Journey.Sentence)
	}
	if !strings.Contains(c.Sentence, "Sem mudanca que valha leitura") {
		t.Errorf("frase veio: %q", c.Sentence)
	}
}

func TestDifferentEnvironmentBecomesCaveatNotConclusion(t *testing.T) {
	before := document(10, 5)
	after := document(20, 10)
	after.Environment.Host = "outra-maquina"
	after.Run.AppliedPlan = []metrics.AppliedPhase{{Kind: "patamar", To: 900, DurationMs: 10000}}

	c := comparison.Compare(before, after)
	together := strings.Join(c.Caveats, " | ")
	if !strings.Contains(together, "maquinas geradoras sao diferentes") {
		t.Errorf("maquina diferente precisa virar ressalva: %v", c.Caveats)
	}
	if !strings.Contains(together, "planos de carga sao diferentes") {
		t.Errorf("plano diferente precisa virar ressalva: %v", c.Caveats)
	}
	if !strings.Contains(c.Sentence, "ressalva") {
		t.Errorf("a frase principal precisa avisar que existem ressalvas: %q", c.Sentence)
	}
}

func TestInvalidResultIsNotCompared(t *testing.T) {
	before := document(10, 5)
	after := document(20, 10)
	after.Warnings = []metrics.Warning{{Severity: metrics.SeverityHigh, Message: "gerador saturado"}}

	c := comparison.Compare(before, after)
	if c.Comparable {
		t.Error("execucao com gerador saturado nao serve de comparacao")
	}
	if !strings.Contains(c.Sentence, "Nao da para comparar") {
		t.Errorf("frase veio: %q", c.Sentence)
	}
}

func TestNewStepIsMarkedInsteadOfCountedAsRegression(t *testing.T) {
	before := document(10, 5)
	after := document(10, 5)
	after.Steps = append(after.Steps, metrics.StepResult{
		Name: "emitir recibo", Count: 1500, Latency: metrics.Distribution{P95: 30},
	})

	c := comparison.Compare(before, after)
	var found bool
	for _, step := range c.Steps {
		if step.Step == "emitir recibo" {
			found = step.New
		}
	}
	if !found {
		t.Error("passo que so existe na execucao nova precisa ser marcado como novo")
	}
}
