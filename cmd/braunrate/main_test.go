package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Diegobraun/braunrate/cenario"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
)

// A opcao colada depois do arquivo era ignorada em silencio: o relatorio nao
// era gravado e nada avisava. Silencio e o pior modo de falhar.
func TestOpcaoValeAntesEDepoisDoArquivo(t *testing.T) {
	casos := [][]string{
		{"-html", "relatorio.html", "cenario.yaml"},
		{"cenario.yaml", "-html", "relatorio.html"},
		{"cenario.yaml", "-html=relatorio.html"},
	}
	for _, argumentos := range casos {
		conjunto := flag.NewFlagSet("executar", flag.ContinueOnError)
		html := conjunto.String("html", "", "arquivo HTML")
		posicionais := analisar(conjunto, argumentos)

		if len(posicionais) != 1 || posicionais[0] != "cenario.yaml" {
			t.Fatalf("%v: o arquivo de cenario nao foi lido: %v", argumentos, posicionais)
		}
		if *html != "relatorio.html" {
			t.Errorf("%v: a opcao foi ignorada", argumentos)
		}
	}
}

// De uma pasta vazia nao havia caminho ate o primeiro cenario: todo comando
// recebia um arquivo e nenhum criava um.
func TestNovoGeraCenarioQueValidaEQueNaoSobrescreve(t *testing.T) {
	raiz := t.TempDir()
	destino := filepath.Join(raiz, "cenario.yaml")

	if codigo := novo([]string{destino}); codigo != 0 {
		t.Fatalf("novo devolveu %d", codigo)
	}
	conteudo, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("o arquivo nao foi criado: %v", err)
	}
	c, err := cenario.Carregar(conteudo)
	if err != nil {
		t.Fatalf("o cenario de partida nao carrega:\n%v", err)
	}
	if err := c.Validar(); err != nil {
		t.Fatalf("o cenario de partida nao e valido: %v", err)
	}
	if len(c.SLO) == 0 {
		t.Error("o cenario de partida precisa mostrar como se declara slo")
	}
	if codigo := novo([]string{destino}); codigo == 0 {
		t.Error("novo sobre arquivo existente nao pode sobrescrever em silencio")
	}
}
