package scenario_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// The scenarios published with 0.5.0, frozen. They are the only evidence that
// the migration works on files someone actually has, and the format they are in
// no longer exists anywhere else in the repository.
func oldScenarios(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "formato-0.5.0", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no 0.5.0 scenario in testdata: %v", err)
	}
	return files
}

func TestEveryPublishedScenarioOf050ConvertsAndValidates(t *testing.T) {
	for _, file := range oldScenarios(t) {
		t.Run(filepath.Base(file), func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if _, err := scenario.Parse(content); err == nil {
				t.Fatal("the old format loaded as if it were the current one")
			}

			converted, changes, err := scenario.Migrate(content)
			if err != nil {
				t.Fatalf("migration failed: %v", err)
			}
			if len(changes) == 0 {
				t.Fatal("no key changed, and the file is in the old format")
			}

			spec, err := scenario.Parse(converted)
			if err != nil {
				t.Fatalf("the converted scenario does not load: %v\n%s", err, converted)
			}
			if problem := spec.Validate(); problem != nil {
				t.Fatalf("the converted scenario does not validate: %v", problem)
			}
		})
	}
}

// The comments are the reason the migration rewrites by position instead of
// re-encoding the tree. Losing one is losing what the author explained to the
// next reader.
func TestMigrationKeepsEveryCommentAndTheOrderOfTheKeys(t *testing.T) {
	for _, file := range oldScenarios(t) {
		t.Run(filepath.Base(file), func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("%v", err)
			}
			converted, _, err := scenario.Migrate(content)
			if err != nil {
				t.Fatalf("migration failed: %v", err)
			}

			before, after := commentsOf(string(content)), commentsOf(string(converted))
			if len(before) != len(after) {
				t.Fatalf("comments went from %d to %d", len(before), len(after))
			}
			for index := range before {
				if before[index] != after[index] {
					t.Errorf("comment %d changed:\n before: %s\n after:  %s", index, before[index], after[index])
				}
			}
			if lines, converted := strings.Count(string(content), "\n"), strings.Count(string(converted), "\n"); lines != converted {
				t.Errorf("the file went from %d lines to %d: a key moved", lines, converted)
			}
		})
	}
}

// A second pass over a converted file used to rename nothing and still rewrite
// it, leaving a .bak of a file identical to itself.
func TestAConvertedScenarioIsRefusedOnASecondPass(t *testing.T) {
	for _, file := range oldScenarios(t) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%v", err)
		}
		converted, _, err := scenario.Migrate(content)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}
		again, changes, err := scenario.Migrate(converted)
		if err != nil {
			t.Fatalf("second pass failed: %v", err)
		}
		if len(changes) != 0 {
			t.Errorf("%s: the second pass changed %d key(s): %+v", filepath.Base(file), len(changes), changes)
		}
		if string(again) != string(converted) {
			t.Errorf("%s: the second pass rewrote the file", filepath.Base(file))
		}
	}
}

// An unnamed step reports under the key its protocol derives, and the rule has
// to point at that key. Those prefixes are ours, not the author's: leaving them
// alone converted every messaging scenario into one that no longer validates.
func TestTheSLOKeyDerivedByAProtocolIsRenamedAndTheAuthorsNameIsNot(t *testing.T) {
	old := `nome: x
alvo: 127.0.0.1:9092
dados:
  pedidos: { gerar: { id: uuid } }
carga:
  perfis:
    - patamar: { taxa: 1/s, durante: 1s }
cenario:
  - kafka: { topico: pedidos, chave: "${pedidos.id}", valor: { id: "${pedidos.id}" } }
  - nome: aguardar o processador
    aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidos.id}"
slo:
  - kafka produzir pedidos: { p95: < 100ms }
  - aguardar o processador: { p95: < 2s }
`
	converted, _, err := scenario.Migrate([]byte(old))
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	text := string(converted)
	if !strings.Contains(text, "- kafka produce pedidos: { p95: < 100ms }") {
		t.Errorf("the key the protocol derives was not renamed:\n%s", text)
	}
	if !strings.Contains(text, "- aguardar o processador: { p95: < 2s }") {
		t.Errorf("a step name the author wrote was renamed:\n%s", text)
	}

	spec, err := scenario.Parse(converted)
	if err != nil {
		t.Fatalf("the converted scenario does not load: %v", err)
	}
	if problem := spec.Validate(); problem != nil {
		t.Fatalf("the converted scenario does not validate: %v", problem)
	}
}

func commentsOf(text string) []string {
	var comments []string
	for _, line := range strings.Split(text, "\n") {
		if index := strings.Index(line, "#"); index >= 0 {
			comments = append(comments, strings.TrimSpace(line[index:]))
		}
	}
	return comments
}
