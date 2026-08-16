package metrics_test

import (
	"go/build"
	"os"
	"strings"
	"testing"
)

// metrics may depend on the shared protocol vocabulary — error class, response
// attributes, consumer lag — because none of that belongs to a protocol in
// particular. What it may never depend on is one protocol: that is where
// protocol-specific measurement starts, and from there the numbers of Kafka and
// of HTTP stop being comparable (ADR 0003 §3).
func TestMetricsDoesNotKnowAnyProtocolInParticular(t *testing.T) {
	packageInfo, err := build.Import("github.com/Diegobraun/braunrate/internal/metrics", "", 0)
	if err != nil {
		t.Fatalf("nao consegui ler o pacote: %v", err)
	}
	for _, imported := range packageInfo.Imports {
		if strings.Contains(imported, "internal/protocol/") {
			t.Errorf("metrics importa %q: medicao especifica de protocolo comeca aqui", imported)
		}
	}
}

// O import nao era o unico jeito de conhecer um protocolo: `variety.go` escrevia
// a frase de particao do Kafka reconhecendo o prefixo do nome da dimensao, e o
// nome do protocolo estava ali em texto. Ficou registrado como divida em
// docs/arquitetura.md porque as duas saidas obvias estavam fechadas — generalizar
// perde o conselho util, e deixar o protocolo escrever o relatorio contraria o
// ADR 0003 §3. A terceira saida e esta: o protocolo declara o que a dimensao
// significa, e a medicao decide se avisa.
func TestMetricsDoesNotNameAnyProtocolInItsText(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("nao consegui ler o pacote: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("nao consegui ler %s: %v", entry.Name(), err)
		}
		lines := strings.Split(string(content), "\n")
		for number, line := range lines {
			lowered := strings.ToLower(line)
			for _, name := range []string{"kafka", "amqp", "graphql", "rabbitmq"} {
				if !strings.Contains(lowered, name) || fromTheSharedVocabulary(lines, number) {
					continue
				}
				t.Errorf("%s:%d cita %q: o que a dimensao significa e do protocolo, nao da medicao",
					entry.Name(), number+1, name)
			}
		}
	}
}

// Uma classe de erro declarada no vocabulario compartilhado obriga a medicao a
// ter uma frase para ela, e a frase nomeia a tecnologia porque e disso que a
// classe trata. Isso e o vocabulario, nao conhecimento de um protocolo.
func fromTheSharedVocabulary(lines []string, number int) bool {
	for back := number; back >= 0 && back > number-2; back-- {
		if strings.Contains(lines[back], "protocol.Err") {
			return true
		}
	}
	return false
}
