package scenario

import (
	"encoding/json"
	"os"
	"path/filepath"
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
