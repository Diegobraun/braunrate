package cenarioemgo_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate"
	cenarioemgo "github.com/Diegobraun/braunrate/examples/cenario-em-go"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

// The published Go scenario runs, like the published YAML ones. Compiling is
// not enough: a scenario that builds and then fails against a real target is
// exactly the kind of example that costs someone an afternoon.
func TestPublishedGoScenarioRunsAndPasses(t *testing.T) {
	target := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := target.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	specification, err := cenarioemgo.Scenario(target.Address())
	if err != nil {
		t.Fatalf("o cenario publicado no README nao constroi: %v", err)
	}

	// Pela superficie publica, que e a que alguem de fora do modulo tem: rodar o
	// cenario publicado por um caminho que so existe aqui dentro nao provaria que
	// o caminho publicado funciona.
	document, err := braunrate.Run(context.Background(), specification, braunrate.Options{DataRoot: ".."})
	if err != nil {
		t.Fatalf("o cenario publicado nao rodou: %v", err)
	}
	if !document.Valid() {
		t.Fatalf("o cenario publicado produziu resultado invalido: %+v", document.Sanity.Findings)
	}
	if document.Overall.Errors > 0 {
		t.Fatalf("o cenario publicado teve %d erros: %+v", document.Overall.Errors, document.Steps)
	}
	if !braunrate.Passed(document) {
		t.Fatalf("o cenario publicado nao passou no proprio slo: %s", document.SLO.Sentence)
	}
	if code := braunrate.ExitCode(document); code != 0 {
		t.Fatalf("o codigo de saida do cenario publicado foi %d", code)
	}
}

// The snippet published on the site and this file are the same text, or the
// page is wrong — and a wrong page is the only documentation nobody notices is
// wrong.
func TestPublishedSnippetIsThisFile(t *testing.T) {
	readme, err := os.ReadFile("../../docs/guias/05-receitas.md")
	if err != nil {
		t.Fatalf("nao consegui ler a receita publicada: %v", err)
	}
	source, err := os.ReadFile("cenario.go")
	if err != nil {
		t.Fatalf("nao consegui ler o cenario: %v", err)
	}

	published, found := betweenFences(string(readme))
	if !found {
		t.Fatal("a receita nao tem mais o bloco de codigo Go; se ele saiu de proposito, este teste sai junto")
	}
	expected, found := betweenMarkers(string(source))
	if !found {
		t.Fatal("os marcadores README:inicio e README:fim sumiram de cenario.go")
	}

	if published != expected {
		t.Fatalf("docs/guias/05-receitas.md derivou de examples/cenario-em-go/cenario.go.\nna pagina:\n%s\n\nno arquivo:\n%s", published, expected)
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
