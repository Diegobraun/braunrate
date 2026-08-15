package main

import (
	"flag"
	"testing"
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
