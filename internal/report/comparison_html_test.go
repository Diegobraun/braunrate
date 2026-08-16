package report_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
)

func comparisonPage(t *testing.T, before, after metrics.Document) string {
	t.Helper()
	var out strings.Builder
	if err := report.ComparisonHTML(&out, comparison.Compare(before, after), "0.4.0"); err != nil {
		t.Fatalf("não gerou a comparação em HTML: %v", err)
	}
	return out.String()
}

func slower(document metrics.Document) metrics.Document {
	document.Journey.Latency = metrics.Distribution{P50: 26, P95: 29, P99: 30, Max: 54}
	document.Overall.Latency = metrics.Distribution{P50: 13, P95: 15, P99: 30}
	for index := range document.Steps {
		document.Steps[index].Latency = metrics.Distribution{P50: 13, P95: 15, P99: 16, P999: 19, Max: 39}
	}
	return document
}

// The HTML says what the terminal says, or two people looking at the same pair
// of runs reach two conclusions.
func TestComparisonInHTMLReachesTheSameVerdictAsTheTerminal(t *testing.T) {
	before := sampleDocument()
	after := slower(sampleDocument())

	result := comparison.Compare(before, after)
	var terminal strings.Builder
	if err := report.Comparison(&terminal, result); err != nil {
		t.Fatalf("não gerou o terminal: %v", err)
	}
	page := comparisonPage(t, before, after)

	if !strings.Contains(page, result.Sentence) {
		t.Fatalf("a página não trouxe a frase do veredito %q", result.Sentence)
	}
	if !strings.Contains(terminal.String(), result.Sentence) {
		t.Fatalf("o terminal não trouxe a frase do veredito %q", result.Sentence)
	}
	for _, step := range result.Steps {
		if !strings.Contains(page, step.Step) {
			t.Fatalf("a página não trouxe o passo %q", step.Step)
		}
	}
}

// Percentiles come from a map. Iterated raw they would come out in a different
// order at every run, and two pages of the same pair would not diff.
func TestComparisonPercentilesComeOutInReadingOrder(t *testing.T) {
	page := comparisonPage(t, sampleDocument(), slower(sampleDocument()))

	position := 0
	for _, percentile := range []string{"p50", "p75", "p90", "p95", "p99", "p99.9", "max"} {
		found := strings.Index(page, ">"+percentile+"<")
		if found < 0 {
			t.Fatalf("a página não trouxe o percentil %s", percentile)
		}
		if found < position {
			t.Fatalf("o percentil %s saiu fora de ordem", percentile)
		}
		position = found
	}
}

// A comparison whose runs do not hold up is not a smaller comparison: no number
// in it means anything, and the page has to say that instead of showing them.
func TestComparisonOfInvalidRunShowsNoNumbers(t *testing.T) {
	before := sampleDocument()
	after := slower(sampleDocument())
	after.Sanity = metrics.Sanity{
		Checked: true, Valid: false,
		Findings: []metrics.SanityFinding{{Kind: "gerador_saturado", Message: "o gerador não sustentou a carga"}},
	}

	page := comparisonPage(t, before, after)
	if !strings.Contains(page, "não vale como medição") {
		t.Fatalf("a página não disse que a comparação não vale:\n%s", page)
	}
	if strings.Contains(page, "Por passo") {
		t.Fatalf("a página mostrou tabela de passo de uma comparação que não vale")
	}
}

func TestComparisonInHTMLFetchesNothingFromNetwork(t *testing.T) {
	page := comparisonPage(t, sampleDocument(), slower(sampleDocument()))
	for _, forbidden := range []string{"<script", "src=", "@import", "cdn.", "https://fonts", "<link"} {
		if strings.Contains(page, forbidden) {
			t.Errorf("a comparação deixou de ser autocontida: encontrei %q", forbidden)
		}
	}
}
