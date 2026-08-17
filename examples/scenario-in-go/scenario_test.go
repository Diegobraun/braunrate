package scenarioingo_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate"
	scenarioingo "github.com/Diegobraun/braunrate/examples/scenario-in-go"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

// The published Go scenario runs, like the published YAML ones. Compiling is
// not enough: a scenario that builds and then fails against a real target is
// exactly the kind of example that costs someone an afternoon.
func TestPublishedGoScenarioRunsAndPasses(t *testing.T) {
	target := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := target.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	specification, err := scenarioingo.Scenario(target.Address())
	if err != nil {
		t.Fatalf("o cenário publicado no README não constroi: %v", err)
	}

	// Pela superficie publica, que e a que alguem de fora do modulo tem: rodar o
	// cenario publicado por um caminho que so existe aqui dentro nao provaria que
	// o caminho publicado funciona.
	document, err := braunrate.Run(context.Background(), specification, braunrate.Options{DataRoot: ".."})
	if err != nil {
		t.Fatalf("o cenário publicado não rodou: %v", err)
	}
	if !document.Valid() {
		t.Fatalf("o cenário publicado produziu resultado inválido: %+v", document.Sanity.Findings)
	}
	if document.Overall.Errors > 0 {
		t.Fatalf("o cenário publicado teve %d errors: %+v", document.Overall.Errors, document.Steps)
	}
	if !braunrate.Passed(document) {
		t.Fatalf("o cenário publicado não passou no próprio slo: %s", document.SLO.Sentence)
	}
	if code := braunrate.ExitCode(document); code != 0 {
		t.Fatalf("o código de saída do cenário publicado foi %d", code)
	}
}

// The snippet published on the site and this file are the same text, or the
// page is wrong — and a wrong page is the only documentation nobody notices is
// wrong.
func TestPublishedSnippetIsThisFile(t *testing.T) {
	readme, err := os.ReadFile("../../docs/guias/50-guias-receitas.md")
	if err != nil {
		t.Fatalf("não consegui ler a receita publicada: %v", err)
	}
	source, err := os.ReadFile("scenario.go")
	if err != nil {
		t.Fatalf("não consegui ler o cenário: %v", err)
	}

	published, found := betweenFences(string(readme))
	if !found {
		t.Fatal("a receita não tem mais o bloco de código Go; se ele saiu de proposito, este teste sai junto")
	}
	expected, found := betweenMarkers(string(source))
	if !found {
		t.Fatal("os marcadores README:inicio e README:fim sumiram de scenario.go")
	}

	if published != expected {
		t.Fatalf("docs/guias/50-guias-receitas.md derivou de examples/scenario-in-go/scenario.go.\nna página:\n%s\n\nno file:\n%s", published, expected)
	}
}

func betweenFences(text string) (string, bool) {
	_, after, found := strings.Cut(text, "```go\n")
	if !found {
		return "", false
	}
	block, _, found := strings.Cut(after, "```")
	return strings.TrimSpace(block), found
}

func betweenMarkers(text string) (string, bool) {
	_, after, found := strings.Cut(text, "// README:inicio\n")
	if !found {
		return "", false
	}
	block, _, found := strings.Cut(after, "// README:fim")
	return strings.TrimSpace(block), found
}
