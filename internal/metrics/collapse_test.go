package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
)

// A frase de colapso agora e composta: o dominio vem de quem sabe, a decisao de
// avisar e a gravidade ficam na medicao. Antes disso a medicao reconhecia o
// prefix "kafka.partition." and wrote the whole sentence itself.
func TestCollapseSentenceComesFromWhoeverOwnsTheDimension(t *testing.T) {
	collector := metrics.NewCollector(time.Now(), 10*time.Millisecond)
	for range 40 {
		collector.RecordDimensions(
			map[string]string{"kafka.partition.pedidos": "0"},
			map[string]protocol.Collapse{"kafka.partition.pedidos": {
				Subject: "uma partição só de pedidos",
				Meaning: "o resto do cluster ficou parado e o número não representa produção",
				Remedy:  "Faça a chave da mensagem variar por iteração",
			}},
		)
	}
	collector.Close()

	varieties := collector.Varieties(metrics.Availability{"kafka.partition.pedidos": 3})
	warnings := metrics.VarietyWarnings(varieties)
	if len(warnings) != 1 {
		t.Fatalf("esperava um aviso de variedade, vieram %d: %+v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Message, "uma partição só de pedidos") ||
		!strings.Contains(warnings[0].Message, "Faça a chave da mensagem variar por iteração") {
		t.Fatalf("a frase perdeu o conselho de quem sabe: %q", warnings[0].Message)
	}
	if warnings[0].Severity != metrics.SeverityHigh {
		t.Fatalf("colapso que ninguém pediu saiu com gravidade %q", warnings[0].Severity)
	}
}

// Concentracao que o cenario pediu nao e o mesmo defeito: ninguem esqueceu de
// variar, e mandar variar manda procurar um defeito que a pessoa nao escreveu.
func TestDeclaredCollapseIsLessGraveAndSaysSo(t *testing.T) {
	collector := metrics.NewCollector(time.Now(), 10*time.Millisecond)
	for range 40 {
		collector.RecordDimensions(
			map[string]string{"kafka.declaredPartition.pedidos": "2"},
			map[string]protocol.Collapse{"kafka.declaredPartition.pedidos": {
				Subject:  "a partição declarada de pedidos",
				Meaning:  "e o número de uma partição, não o do tópico",
				Remedy:   "Drop 'partition' from the step to spread the load",
				Declared: true,
			}},
		)
	}
	collector.Close()

	warnings := metrics.VarietyWarnings(collector.Varieties(metrics.Availability{"kafka.declaredPartition.pedidos": 3}))
	if len(warnings) != 1 {
		t.Fatalf("esperava um aviso, vieram %d", len(warnings))
	}
	if warnings[0].Severity != metrics.SeverityMedium {
		t.Fatalf("a concentracao declarada saiu com gravidade %q", warnings[0].Severity)
	}
	if strings.Contains(warnings[0].Message, "variar por iteração") {
		t.Fatalf("mandou variar a chave numa concentracao que o cenário pediu: %q", warnings[0].Message)
	}
}
