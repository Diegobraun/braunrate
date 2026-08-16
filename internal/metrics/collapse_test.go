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
// prefixo "kafka.particao." e escrevia a frase inteira.
func TestCollapseSentenceComesFromWhoeverOwnsTheDimension(t *testing.T) {
	collector := metrics.NewCollector(time.Now(), 10*time.Millisecond)
	for range 40 {
		collector.RecordDimensions(
			map[string]string{"kafka.particao.pedidos": "0"},
			map[string]protocol.Collapse{"kafka.particao.pedidos": {
				Subject: "uma particao so de pedidos",
				Meaning: "o resto do cluster ficou parado e o numero nao representa producao",
				Remedy:  "Faca a chave da mensagem variar por iteracao",
			}},
		)
	}
	collector.Close()

	varieties := collector.Varieties(metrics.Availability{"kafka.particao.pedidos": 3})
	warnings := metrics.VarietyWarnings(varieties)
	if len(warnings) != 1 {
		t.Fatalf("esperava um aviso de variedade, vieram %d: %+v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Message, "uma particao so de pedidos") ||
		!strings.Contains(warnings[0].Message, "Faca a chave da mensagem variar por iteracao") {
		t.Fatalf("a frase perdeu o conselho de quem sabe: %q", warnings[0].Message)
	}
	if warnings[0].Severity != metrics.SeverityHigh {
		t.Fatalf("colapso que ninguem pediu saiu com gravidade %q", warnings[0].Severity)
	}
}

// Concentracao que o cenario pediu nao e o mesmo defeito: ninguem esqueceu de
// variar, e mandar variar manda procurar um defeito que a pessoa nao escreveu.
func TestDeclaredCollapseIsLessGraveAndSaysSo(t *testing.T) {
	collector := metrics.NewCollector(time.Now(), 10*time.Millisecond)
	for range 40 {
		collector.RecordDimensions(
			map[string]string{"kafka.particao.declarada.pedidos": "2"},
			map[string]protocol.Collapse{"kafka.particao.declarada.pedidos": {
				Subject:  "a particao declarada de pedidos",
				Meaning:  "e o numero de uma particao, nao o do topico",
				Remedy:   "Tire 'particao' do passo para distribuir",
				Declared: true,
			}},
		)
	}
	collector.Close()

	warnings := metrics.VarietyWarnings(collector.Varieties(metrics.Availability{"kafka.particao.declarada.pedidos": 3}))
	if len(warnings) != 1 {
		t.Fatalf("esperava um aviso, vieram %d", len(warnings))
	}
	if warnings[0].Severity != metrics.SeverityMedium {
		t.Fatalf("a concentracao declarada saiu com gravidade %q", warnings[0].Severity)
	}
	if strings.Contains(warnings[0].Message, "variar por iteracao") {
		t.Fatalf("mandou variar a chave numa concentracao que o cenario pediu: %q", warnings[0].Message)
	}
}
