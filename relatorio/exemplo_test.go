package relatorio_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/relatorio"
)

// O exemplo publicado e a primeira coisa que alguem abre para decidir se a
// ferramenta presta. Ele e gerado de um resultado real congelado, e este teste
// falha quando o arquivo commitado envelhece em relacao ao gerador.
func TestExemploPublicadoEstaAtualizado(t *testing.T) {
	conteudo, err := os.ReadFile(filepath.Join("..", "docs", "exemplo-resultado.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o resultado de exemplo: %v", err)
	}
	var documento metrica.Documento
	if err := json.Unmarshal(conteudo, &documento); err != nil {
		t.Fatalf("o resultado de exemplo nao carrega: %v", err)
	}

	var gerado strings.Builder
	if err := relatorio.HTML(&gerado, documento); err != nil {
		t.Fatalf("nao gerou o HTML: %v", err)
	}

	caminho := filepath.Join("..", "docs", "exemplo-relatorio.html")
	commitado, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("nao consegui ler o exemplo publicado: %v", err)
	}

	if string(commitado) != gerado.String() {
		t.Errorf(`docs/exemplo-relatorio.html esta diferente do que o gerador produz hoje.
Regenere com:
  go run ./cmd/braunrate relatorio docs/exemplo-resultado.json -html=docs/exemplo-relatorio.html`)
	}
}

func TestExemploPublicadoContinuaSendoUmaExecucaoReal(t *testing.T) {
	conteudo, err := os.ReadFile(filepath.Join("..", "docs", "exemplo-resultado.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o resultado de exemplo: %v", err)
	}
	var documento metrica.Documento
	if err := json.Unmarshal(conteudo, &documento); err != nil {
		t.Fatalf("o resultado de exemplo nao carrega: %v", err)
	}
	if documento.Global.Contagem == 0 || len(documento.Series) == 0 {
		t.Error("o exemplo precisa vir de uma execucao com carga, nao de um documento montado a mao")
	}
	if documento.VersaoDoFormato != metrica.VersaoDoFormatoDeResultado {
		t.Errorf("o exemplo esta no formato %q e o atual e %q: regenere a execucao",
			documento.VersaoDoFormato, metrica.VersaoDoFormatoDeResultado)
	}
}
