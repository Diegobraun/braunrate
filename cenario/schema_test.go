package cenario

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Diegobraun/braunrate/protocolo"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
)

type esquema struct {
	Propriedades map[string]json.RawMessage `json:"properties"`
	Definicoes   map[string]struct {
		Propriedades map[string]json.RawMessage `json:"properties"`
	} `json:"$defs"`
}

func lerEsquema(t *testing.T) esquema {
	t.Helper()
	conteudo, err := os.ReadFile(filepath.Join("..", "docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o schema publicado: %v", err)
	}
	var lido esquema
	if err := json.Unmarshal(conteudo, &lido); err != nil {
		t.Fatalf("o schema publicado nao e JSON valido: %v", err)
	}
	return lido
}

func TestSchemaPublicadoTemAsMesmasChavesDeTopoQueOParser(t *testing.T) {
	lido := lerEsquema(t)
	compararChaves(t, "topo", ChavesDeTopo, chavesDe(lido.Propriedades))
}

func TestSchemaPublicadoTemAsMesmasChavesDePassoQueOParser(t *testing.T) {
	lido := lerEsquema(t)
	esperadas := append(append([]string{}, ChavesDePasso...), protocolo.Registrados()...)
	compararChaves(t, "passo", esperadas, chavesDe(lido.Definicoes["passo"].Propriedades))
}

func compararChaves(t *testing.T, onde string, parser, schema []string) {
	t.Helper()
	sort.Strings(parser)
	sort.Strings(schema)

	noSchema := map[string]bool{}
	for _, chave := range schema {
		noSchema[chave] = true
	}
	for _, chave := range parser {
		if !noSchema[chave] {
			t.Errorf("o parser aceita %q no %s e o schema nao documenta: o editor vai marcar erro em cenario valido", chave, onde)
		}
	}

	noParser := map[string]bool{}
	for _, chave := range parser {
		noParser[chave] = true
	}
	for _, chave := range schema {
		if !noParser[chave] {
			t.Errorf("o schema oferece %q no %s e o parser recusa: o editor vai completar chave que nao roda", chave, onde)
		}
	}
}

func chavesDe(propriedades map[string]json.RawMessage) []string {
	chaves := make([]string, 0, len(propriedades))
	for chave := range propriedades {
		chaves = append(chaves, chave)
	}
	return chaves
}

func TestCenariosDeExemploApontamParaOSchema(t *testing.T) {
	arquivos, err := filepath.Glob(filepath.Join("..", "cenarios", "*.yaml"))
	if err != nil || len(arquivos) == 0 {
		t.Fatalf("nao encontrei cenarios de exemplo: %v", err)
	}
	for _, arquivo := range arquivos {
		conteudo, err := os.ReadFile(arquivo)
		if err != nil {
			t.Fatalf("%s: %v", arquivo, err)
		}
		if len(conteudo) < 20 || string(conteudo[:19]) != "# yaml-language-ser" {
			t.Errorf("%s nao comeca com a linha de yaml-language-server: quem abrir no editor nao ganha autocompletar", arquivo)
		}
		if _, err := Carregar(conteudo); err != nil {
			t.Errorf("%s nao carrega: %v", arquivo, err)
		}
	}
}
