package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/slo"
)

// docs/vocabulario.md fixa uma palavra por conceito. Estas sao as que nao tem
// leitura legitima no texto ao user: qualquer aparicao e o mesmo conceito
// ganhando um segundo nome, que e como quem nunca fez teste de carga conclui
// que sao duas coisas.
//
// A lista e curta de proposito. Termos como "cenario" aparecem na coluna
// "nunca use" de uma linha e sao o termo oficial de outra, entao varrer a
// tabela inteira daria alarme falso e o teste seria desligado.
var forbiddenInUserText = []string{
	"latencia",
	"latência",
	"percentil",
	"quantil",
	"throughput",
	"saturad",
	"back-pressure",
	"threshold",
}

func TestTheReportSpeaksTheVocabulary(t *testing.T) {
	document := publishedExample(t)

	var terminal strings.Builder
	if err := report.Summary(&terminal, document, slo.Verdict{}); err != nil {
		t.Fatalf("não gerou o resumo: %v", err)
	}
	var page strings.Builder
	if err := report.HTML(&page, document); err != nil {
		t.Fatalf("não gerou o HTML: %v", err)
	}

	for _, surface := range []struct {
		name string
		text string
	}{{"terminal", terminal.String()}, {"HTML", page.String()}} {
		lowered := strings.ToLower(surface.text)
		for _, term := range forbiddenInUserText {
			if strings.Contains(lowered, term) {
				t.Errorf("%s usa %q, que docs/vocabulario.md proibe no texto ao usuário", surface.name, term)
			}
		}
	}
}

func publishedExample(t *testing.T) metrics.Document {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "exemplo-resultado.json"))
	if err != nil {
		t.Fatalf("não consegui ler o resultado de exemplo: %v", err)
	}
	var document metrics.Document
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o resultado de exemplo não carrega: %v", err)
	}
	return document
}
