package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// An option pasted after the file was silently ignored: the report was never
// written and nothing warned. Silence is the worst failure mode.
func TestFlagWorksBeforeAndAfterFile(t *testing.T) {
	testCases := [][]string{
		{"-html", "relatorio.html", "cenario.yaml"},
		{"cenario.yaml", "-html", "relatorio.html"},
		{"cenario.yaml", "-html=relatorio.html"},
	}
	for _, args := range testCases {
		set := flag.NewFlagSet("executar", flag.ContinueOnError)
		html := set.String("html", "", "arquivo HTML")
		positional := analisar(set, args)

		if len(positional) != 1 || positional[0] != "cenario.yaml" {
			t.Fatalf("%v: o arquivo de cenario nao foi lido: %v", args, positional)
		}
		if *html != "relatorio.html" {
			t.Errorf("%v: a opcao foi ignorada", args)
		}
	}
}

// From an empty folder there was no path to a first scenario: every command
// took a file and none created one.
func TestNewCommandWritesValidScenarioAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "cenario.yaml")

	if code := newOne([]string{destination}); code != 0 {
		t.Fatalf("novo devolveu %d", code)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("o arquivo nao foi criado: %v", err)
	}
	c, err := scenario.Parse(content)
	if err != nil {
		t.Fatalf("o cenario de partida nao carrega:\n%v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("o cenario de partida nao e valido: %v", err)
	}
	if len(c.SLO) == 0 {
		t.Error("o cenario de partida precisa mostrar como se declara slo")
	}
	if code := newOne([]string{destination}); code == 0 {
		t.Error("novo sobre arquivo existente nao pode sobrescrever em silencio")
	}
}
