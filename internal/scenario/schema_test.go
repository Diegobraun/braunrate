package scenario

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
)

type schema struct {
	Properties  map[string]json.RawMessage `json:"properties"`
	Definitions map[string]struct {
		Pattern    string                     `json:"pattern"`
		Properties map[string]json.RawMessage `json:"properties"`
	} `json:"$defs"`
}

func readSchema(t *testing.T) schema {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o schema publicado: %v", err)
	}
	var read schema
	if err := json.Unmarshal(content, &read); err != nil {
		t.Fatalf("o schema publicado nao e JSON valido: %v", err)
	}
	return read
}

func TestPublishedSchemaHasSameTopKeysAsParser(t *testing.T) {
	read := readSchema(t)
	compareKeys(t, "topo", TopKeys, keysOf(read.Properties))
}

func TestPublishedSchemaHasSameStepKeysAsParser(t *testing.T) {
	read := readSchema(t)
	expected := append(append([]string{}, StepKeys...), protocol.Registered()...)
	compareKeys(t, "passo", expected, keysOf(read.Definitions["passo"].Properties))
}

// O editor le o schema antes de a ferramenta ler o arquivo: um campo que o
// parser resolve pelo ambiente e o schema marca em vermelho vira a mesma
// inconsistencia de sempre, agora com o QA vendo o erro antes de rodar.
func TestPublishedSchemaAcceptsEnvironmentReferencesWhereTheParserDoes(t *testing.T) {
	read := readSchema(t)
	accepted := map[string][]string{
		"taxa":    {"300/s", "1.5/m", "${TAXA}/s", "${TAXA:-100}/s", "${TAXA}"},
		"duracao": {"30s", "1.5m", "${DURACAO}", "${DURACAO:-1m}"},
	}
	for name, values := range accepted {
		pattern, err := regexp.Compile(read.Definitions[name].Pattern)
		if err != nil {
			t.Fatalf("o padrao de %q nao compila: %v", name, err)
		}
		for _, value := range values {
			if !pattern.MatchString(value) {
				t.Errorf("o parser aceita %s: %q e o schema marca erro", name, value)
			}
		}
	}
	for name, refused := range map[string]string{"taxa": "300", "duracao": "30"} {
		pattern := regexp.MustCompile(read.Definitions[name].Pattern)
		if pattern.MatchString(refused) {
			t.Errorf("o schema de %s passou a aceitar %q, que nao tem unidade", name, refused)
		}
	}
}

func compareKeys(t *testing.T, where string, parser, schema []string) {
	t.Helper()
	sort.Strings(parser)
	sort.Strings(schema)

	schemaNode := map[string]bool{}
	for _, key := range schema {
		schemaNode[key] = true
	}
	for _, key := range parser {
		if !schemaNode[key] {
			t.Errorf("o parser aceita %q no %s e o schema nao documenta: o editor vai marcar erro em cenario valido", key, where)
		}
	}

	parserNode := map[string]bool{}
	for _, key := range parser {
		parserNode[key] = true
	}
	for _, key := range schema {
		if !parserNode[key] {
			t.Errorf("o schema oferece %q no %s e o parser recusa: o editor vai completar chave que nao roda", key, where)
		}
	}
}

func keysOf(properties map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	return keys
}

func TestExampleScenariosPointToSchema(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("nao encontrei cenarios de exemplo em examples/: %v", err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if len(content) < 20 || string(content[:19]) != "# yaml-language-ser" {
			t.Errorf("%s nao comeca com a linha de yaml-language-server: quem abrir no editor nao ganha autocompletar", file)
		}
		if _, err := Parse(content); err != nil {
			t.Errorf("%s nao carrega: %v", file, err)
		}
	}
}
