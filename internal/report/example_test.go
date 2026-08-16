package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
)

// The published example is the first thing someone opens to decide whether the
// tool is any good. It is generated from a frozen real result, and this test
// fails when the committed file ages behind the generator.
func TestPublishedExampleIsUpToDate(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "exemplo-resultado.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o resultado de exemplo: %v", err)
	}
	var document metrics.Document
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o resultado de exemplo nao carrega: %v", err)
	}

	var generated strings.Builder
	if err := report.HTML(&generated, document); err != nil {
		t.Fatalf("nao gerou o HTML: %v", err)
	}

	path := filepath.Join("..", "..", "docs", "exemplo-relatorio.html")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("nao consegui ler o exemplo publicado: %v", err)
	}

	if string(committed) != generated.String() {
		t.Errorf(`docs/exemplo-relatorio.html esta diferente do que o gerador produz hoje.
Regenere com:
  go run ./cmd/braunrate report docs/exemplo-resultado.json -html=docs/exemplo-relatorio.html`)
	}
}

func TestPublishedExampleStaysARealRun(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "exemplo-resultado.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o resultado de exemplo: %v", err)
	}
	var document metrics.Document
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o resultado de exemplo nao carrega: %v", err)
	}
	if document.Overall.Count == 0 || len(document.Series) == 0 {
		t.Error("o exemplo precisa vir de uma execucao com carga, nao de um documento montado a mao")
	}
	if document.FormatVersion != metrics.ResultFormatVersion {
		t.Errorf("o exemplo esta no formato %q e o atual e %q: regenere a execucao",
			document.FormatVersion, metrics.ResultFormatVersion)
	}
}
