package report_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
)

func sampleDocument() metrics.Document {
	start := time.Date(2026, 8, 15, 22, 0, 0, 0, time.UTC)
	return metrics.Document{
		FormatVersion: metrics.ResultFormatVersion,
		Tool:          "braunrate",
		Version:       "0.3.0",
		Environment:   metrics.Environment{Host: "maquina-de-teste", OS: "darwin", Arch: "arm64", Cores: 10},
		Run: metrics.Run{
			Spec: "Jornada de cobrança", Target: "http://127.0.0.1:8080",
			Start: start, End: start.Add(10 * time.Second), DurationMs: 10000,
			Model: "aberto", MaxInflight: 20000, AuthObtains: 1,
			AppliedPlan: []metrics.AppliedPhase{{Kind: "patamar", To: 300, DurationMs: 10000}},
		},
		Scheduling: metrics.Scheduling{Sent: 3000, Completed: 3000, Skew: metrics.Distribution{P50: 0.01, Max: 1.2}},
		Journey: metrics.Journey{
			Started: 1500, Completed: 1500,
			Latency:  metrics.Distribution{P50: 8.7, P95: 9.5, P99: 10, Max: 18},
			Sentence: "Todas as 1500 jornadas chegaram ao fim; metade levou até 9 ms e 95% até 10 ms, contados do instante em que deveriam ter comecado.",
		},
		Steps: []metrics.StepResult{
			{Name: "consultar pedido", LatencyKind: string(metrics.CorrectedLatency), Count: 1500,
				Latency: metrics.Distribution{P50: 4.3, P95: 4.9, P99: 5.3, P999: 6.2, Max: 13}},
			{Name: "pagar fatura", LatencyKind: string(metrics.ServiceLatency), Count: 1500, Errors: 3,
				ErrorsByClass: map[string]int64{"status": 3},
				Latency:       metrics.Distribution{P50: 4.3, P95: 4.8, P99: 5.1, P999: 5.8, Max: 11}},
		},
		Overall: metrics.OverallResult{
			Count: 3000, Successes: 2997, Errors: 3, ErrorRate: 0.001, EffectiveRate: 300,
			Latency:        metrics.Distribution{P50: 4.3, P95: 4.9, P99: 9.8},
			ServiceLatency: metrics.Distribution{P50: 4.3, P95: 4.8, P99: 5.1},
		},
		Series: []metrics.Bucket{
			{StartEpochMs: 1000, Sent: 300, Completed: 300, LatencyP50Ms: 4.2, LatencyP99Ms: 5.1},
			{StartEpochMs: 2000, Sent: 300, Completed: 300, LatencyP50Ms: 4.3, LatencyP99Ms: 5.4},
			{StartEpochMs: 3000, Sent: 300, Completed: 299, Errors: 1, LatencyP50Ms: 4.4, LatencyP99Ms: 9.9},
		},
	}
}

func generate(t *testing.T, document metrics.Document) string {
	t.Helper()
	var out strings.Builder
	if err := report.HTML(&out, document); err != nil {
		t.Fatalf("não gerou o HTML: %v", err)
	}
	return out.String()
}

func TestReportTopIsSentenceNotTable(t *testing.T) {
	document := sampleDocument()
	document.SLO = metrics.Verdict{
		Passed:      true,
		Evaluations: []metrics.Evaluation{{Step: "consultar pedido", Metric: "p95", Passed: true}},
		Sentence:    "Passou: as 3 regras de SLO foram atendidas.",
	}
	page := generate(t, document)

	title := regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`).FindStringSubmatch(page)
	if title == nil {
		t.Fatal("o relatório não tem título")
	}
	if title[1] != "Passou: as 3 regras de SLO foram atendidas." {
		t.Errorf("o topo precisa ser a frase do veredito, veio: %q", title[1])
	}
	if index := strings.Index(page, "<table"); index < strings.Index(page, "</h1>") {
		t.Error("existe tabela antes da frase de veredito")
	}
}

func TestFailureReportShowsReasonOnTop(t *testing.T) {
	document := sampleDocument()
	document.SLO = metrics.Verdict{
		Passed:   false,
		Sentence: `Falhou: "pagar fatura" teve latência p95 de 210 ms, acima do limite de 150 ms.`,
		Evaluations: []metrics.Evaluation{
			{Step: "pagar fatura", Passed: false, Sentence: `Falhou: "pagar fatura" teve latência p95 de 210 ms, acima do limite de 150 ms.`},
		},
	}
	page := generate(t, document)

	if !strings.Contains(page, `<h1 class="falhou">`) {
		t.Error("falha de SLO precisa aparecer como falha no topo")
	}
	if !strings.Contains(page, "acima do limite de 150 ms") {
		t.Error("o motivo da falha não aparece")
	}
}

func TestInvalidResultIsNotPresentedAsTargetNumber(t *testing.T) {
	document := sampleDocument()
	document.SLO = metrics.Verdict{
		Passed:      true,
		Evaluations: []metrics.Evaluation{{Step: "consultar pedido", Metric: "p95", Passed: true}},
		Sentence:    "Passou: as 3 regras de SLO foram atendidas.",
	}
	document.Warnings = []metrics.Warning{{
		Kind: "generatorSaturated", Severity: metrics.SeverityHigh,
		Message:  "o gerador não sustentou a taxa alvo",
		Evidence: "12% dos despachos atrasaram",
	}}
	page := generate(t, document)

	if strings.Contains(page, `<h1 class="passou">`) {
		t.Error("com o gerador saturado o topo não pode dizer que passou")
	}
	if !strings.Contains(page, "Resultado inválido") {
		t.Error("o topo precisa declarar que o resultado não vale")
	}
	if !strings.Contains(page, "medem o gerador, não o alvo") {
		t.Error("falta a leitura em portugues comum do resultado inválido")
	}
}

func TestReportDistinguishesCorrectedFromServiceLatency(t *testing.T) {
	page := generate(t, sampleDocument())
	if !strings.Contains(page, "(1)") || !strings.Contains(page, "(2)") {
		t.Error("os dois tipos de latência precisam estar marcados por passo")
	}
	if !strings.Contains(page, "não tem instante agendado próprio") {
		t.Error("falta a explicacao do que e latência de serviço")
	}
	if !strings.Contains(page, "A jornada inteira") {
		t.Error("falta a métrica que continua honesta para a jornada toda")
	}
}

func TestReportDeclaresSingleTokenLimitation(t *testing.T) {
	page := generate(t, sampleDocument())
	if !strings.Contains(page, "cache, rate limit ou sharding por token") {
		t.Error("execução com autenticação precisa declarar a limitacao de token único")
	}
}

// A load report tends to be opened from a closed network or attached to a
// ticket; if it depends on the network it opens broken exactly where it
// matters.
func TestReportFetchesNothingFromNetwork(t *testing.T) {
	page := generate(t, sampleDocument())
	forbidden := []string{"<script", "src=", "@import", "cdn.", "https://fonts", "<link"}
	for _, forbidden := range forbidden {
		if strings.Contains(page, forbidden) {
			t.Errorf("o relatório deixou de ser autocontido: encontrei %q", forbidden)
		}
	}
}

func TestReportWithoutSLOSaysItNeitherPassesNorFails(t *testing.T) {
	page := generate(t, sampleDocument())
	if !strings.Contains(page, "não aprova nem reprova") {
		t.Error("sem slo declarado o relatório precisa dizer que não decide nada")
	}
}

func TestTimeSeriesBecomesChartWithoutLibrary(t *testing.T) {
	page := generate(t, sampleDocument())
	if !strings.Contains(page, "<svg") || !strings.Contains(page, "<polyline") {
		t.Error("a serie temporal não virou gráfico")
	}
}

func TestCSVSeparatesCorrectedFromServiceLatency(t *testing.T) {
	var out strings.Builder
	if err := report.CSV(&out, sampleDocument()); err != nil {
		t.Fatalf("não gerou o CSV: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("esperava cabecalho, jornada, dois passos e global; vieram %d linhas", len(lines))
	}
	if !strings.Contains(lines[0], "tipo_de_latencia") {
		t.Error("o CSV precisa dizer de que tipo e cada latência")
	}
	if !strings.HasPrefix(lines[1], "Jornada de cobrança,http://127.0.0.1:8080") || !strings.Contains(lines[1], "jornada inteira") {
		t.Errorf("a primeira linha de dados precisa ser a jornada: %s", lines[1])
	}
	if !strings.Contains(lines[3], ",servico,") {
		t.Errorf("o passo de latência de serviço precisa estar marcado: %s", lines[3])
	}
}
