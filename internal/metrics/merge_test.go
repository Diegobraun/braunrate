package metrics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
)

// The test ADR 0003 §5 asked for since phase 0 and never had: two documents
// written to file, read back and added produce the same percentiles as a single
// run with the same samples. If they do not, "agregado mergeavel" is a promise
// the format does not keep — and it was not keeping it, because what it
// published were percentiles, which do not add.
func TestTwoResultFilesAddUpToTheSameNumbersAsOneRun(t *testing.T) {
	first := latencies(1, 400)
	second := latencies(401, 800)

	partOne := documentOf(t, "gerador-um", first)
	partTwo := documentOf(t, "gerador-dois", second)
	whole := documentOf(t, "gerador-unico", append(append([]int64{}, first...), second...))

	merged, err := metrics.Merge(reread(t, partOne), reread(t, partTwo))
	if err != nil {
		t.Fatalf("não consegui somar os dois resultados: %v", err)
	}

	expected := whole.Steps[0].Reported()
	obtained := merged.Steps[0].Reported()

	for _, comparison := range []struct {
		name               string
		expected, obtained float64
	}{
		{"p50", expected.P50, obtained.P50},
		{"p75", expected.P75, obtained.P75},
		{"p90", expected.P90, obtained.P90},
		{"p95", expected.P95, obtained.P95},
		{"p99", expected.P99, obtained.P99},
		{"p99.9", expected.P999, obtained.P999},
		{"max", expected.Max, obtained.Max},
		{"min", expected.Minimum, obtained.Minimum},
	} {
		if comparison.expected != comparison.obtained {
			t.Errorf("%s da soma não bate com o da execução única: esperava %g ms, veio %g ms",
				comparison.name, comparison.expected, comparison.obtained)
		}
	}
	if expected.Samples != obtained.Samples {
		t.Errorf("a soma tem %d amostras e a execução única tem %d", obtained.Samples, expected.Samples)
	}
	if whole.Steps[0].Count != merged.Steps[0].Count {
		t.Errorf("a soma contou %d requisições e a execução única %d", merged.Steps[0].Count, whole.Steps[0].Count)
	}
	if whole.Overall.Reported().P99 != merged.Overall.Reported().P99 {
		t.Errorf("o p99 global da soma (%g) não bate com o da execução única (%g)",
			merged.Overall.Reported().P99, whole.Overall.Reported().P99)
	}
	if whole.Journey.Reported().P95 != merged.Journey.Reported().P95 {
		t.Errorf("o p95 de jornada da soma (%g) não bate com o da execução única (%g)",
			merged.Journey.Reported().P95, whole.Journey.Reported().P95)
	}
}

// The mean is the other thing ADR 0003 §5 forbids as a source of truth. It has
// to come out of the sum too, not out of the average of two averages.
func TestMeanOfTheSumIsNotTheAverageOfTwoMeans(t *testing.T) {
	few := latencies(1, 10)
	many := latencies(1000, 1990)

	merged, err := metrics.Merge(
		reread(t, documentOf(t, "poucas", few)),
		reread(t, documentOf(t, "muitas", many)),
	)
	if err != nil {
		t.Fatalf("não consegui somar: %v", err)
	}
	whole := documentOf(t, "unica", append(append([]int64{}, few...), many...))

	expected := whole.Steps[0].Reported().Mean
	obtained := merged.Steps[0].Reported().Mean
	if difference := expected - obtained; difference > 0.001 || difference < -0.001 {
		t.Fatalf("a média da soma (%g ms) não bate com a da execução única (%g ms)", obtained, expected)
	}
}

// A version 1 file has the percentiles and not the histogram behind them. It
// keeps being read; what it cannot do is be added, and the message says which
// of the two it is.
func TestOldFormatIsReadableAndRefusedForSummingWithAReason(t *testing.T) {
	current := reread(t, documentOf(t, "atual", latencies(1, 100)))
	old := reread(t, documentOf(t, "antigo", latencies(1, 100)))
	old.FormatVersion = "1"

	if old.Steps[0].Reported().P95 == 0 {
		t.Fatal("o documento antigo perdeu os percentis que ele já tinha")
	}

	_, err := metrics.Merge(current, old)
	if err == nil {
		t.Fatal("somou um resultado de formato antigo, cujos percentis não tem histograma por trás")
	}
	for _, expected := range []string{`formato "1"`, "histograma", "percentil não soma"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("a mensagem não explica por que não soma: falta %q em\n%v", expected, err)
		}
	}
}

func TestSummingRunsOfDifferentScenariosIsRefused(t *testing.T) {
	one := reread(t, documentOf(t, "um", latencies(1, 10)))
	two := reread(t, documentOf(t, "dois", latencies(1, 10)))
	two.Run.Spec = "Outro cenário"

	if _, err := metrics.Merge(one, two); err == nil {
		t.Fatal("somou execuções de cenários diferentes")
	}
}

func latencies(from, to int64) []int64 {
	values := make([]int64, 0, to-from+1)
	for value := from; value <= to; value++ {
		values = append(values, value)
	}
	return values
}

// Goes through the collector so the histogram in the file is the one the engine
// writes, not one the test built.
func documentOf(t *testing.T, host string, milliseconds []int64) metrics.Document {
	t.Helper()
	start := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	collector := metrics.NewCollector(start, 10*time.Millisecond)

	for index, value := range milliseconds {
		scheduled := start.Add(time.Duration(index) * time.Millisecond)
		finished := scheduled.Add(time.Duration(value) * time.Millisecond)
		collector.Record(metrics.Sample{
			LatencyKind: metrics.CorrectedLatency,
			Step:        "consultar", Key: "GET /pedidos", Protocol: "http",
			ScheduledAt: scheduled, SentAt: scheduled, FinishedAt: finished,
			Class: protocol.Success, Status: 200, Bytes: 100,
		})
		collector.RecordJourney(scheduled, finished, true)
	}
	collector.Close()

	document := metrics.BuildDocument(collector, metrics.DocumentInput{
		Spec: "Cenário somavel", Target: "http://alvo", Version: "teste",
		Start: start, End: start.Add(time.Duration(len(milliseconds)) * time.Millisecond),
		Model: "aberto",
	})
	document.Environment.Host = host
	return document
}

// Writing and reading back is the point: the promise is about what survives
// serialization, and an in-memory merge would prove nothing.
func reread(t *testing.T, document metrics.Document) metrics.Document {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resultado.json")
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("não consegui gravar o resultado: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("não consegui gravar o resultado: %v", err)
	}
	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("não consegui reler o resultado: %v", err)
	}
	var back metrics.Document
	if err := json.Unmarshal(read, &back); err != nil {
		t.Fatalf("não consegui reler o resultado: %v", err)
	}
	return back
}
