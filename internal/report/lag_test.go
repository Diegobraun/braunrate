package report_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/slo"
)

func documentWithLag(lag protocol.ConsumerLag) metrics.Document {
	document := sampleDocument()
	document.Run.ConsumerLag = []protocol.ConsumerLag{lag}
	return document
}

// The number that says whether the service kept up cannot exist in one output
// and not in the other: whoever reads the HTML has to reach the same conclusion
// as whoever read the terminal.
func TestConsumerLagAppearsInTheTerminalAndInTheHTML(t *testing.T) {
	document := documentWithLag(protocol.ConsumerLag{
		Group: "cobranca", Topic: "faturas", Max: 4200, Final: 4200, Readings: 10,
	})

	var terminal strings.Builder
	if err := report.Summary(&terminal, document, slo.Verdict{}); err != nil {
		t.Fatalf("nao gerou o terminal: %v", err)
	}
	html := generate(t, document)

	for name, output := range map[string]string{"terminal": terminal.String(), "html": html} {
		if !strings.Contains(output, "cobranca") || !strings.Contains(output, "faturas") {
			t.Fatalf("o %s nao nomeou o grupo e o topico observados", name)
		}
		if !strings.Contains(output, "4.200 mensagens") {
			t.Fatalf("o %s nao disse quantas mensagens o consumidor ficou para tras", name)
		}
		if !strings.Contains(output, "terminou a execucao para tras") {
			t.Fatalf("o %s mostrou o numero e nao disse o que ele significa", name)
		}
	}
}

// Lag it could not read is not lag zero. Reporting zero would say the consumer
// kept up, which is the opposite of not knowing.
func TestLagItCouldNotMeasureIsNotReportedAsZero(t *testing.T) {
	document := documentWithLag(protocol.ConsumerLag{
		Group: "cobranca", Topic: "faturas", Problem: "sem permissao para ler o offset do grupo",
	})

	var terminal strings.Builder
	if err := report.Summary(&terminal, document, slo.Verdict{}); err != nil {
		t.Fatalf("nao gerou o terminal: %v", err)
	}
	html := generate(t, document)

	for name, output := range map[string]string{"terminal": terminal.String(), "html": html} {
		if !strings.Contains(output, "nao consegui medir") {
			t.Fatalf("o %s escondeu que a medicao falhou", name)
		}
		if !strings.Contains(output, "sem permissao para ler o offset do grupo") {
			t.Fatalf("o %s nao disse por que a medicao falhou", name)
		}
		if strings.Contains(output, "terminou a execucao para tras") {
			t.Fatalf("o %s concluiu sobre o consumidor sem ter medido", name)
		}
	}
}

// A run without a declared group has nothing to say about lag, and a section
// with a table of nothing is worse than no section.
func TestReportWithoutAWatchedGroupHasNoLagSection(t *testing.T) {
	document := sampleDocument()

	var terminal strings.Builder
	if err := report.Summary(&terminal, document, slo.Verdict{}); err != nil {
		t.Fatalf("nao gerou o terminal: %v", err)
	}
	html := generate(t, document)

	for name, output := range map[string]string{"terminal": terminal.String(), "html": html} {
		if strings.Contains(output, "Atraso do consumidor") {
			t.Fatalf("o %s abriu secao de atraso sem grupo observado", name)
		}
	}
}

// Two binaries with the same version number can carry different protocols. The
// report has to leave a trace of which ones, or a result that differs from
// another has no explanation to point at (ADR 0004).
func TestReportSaysWhichProtocolsTheBinaryCarries(t *testing.T) {
	document := sampleDocument()
	document.Environment.Protocols = []string{"http", "kafka"}

	var terminal strings.Builder
	if err := report.Summary(&terminal, document, slo.Verdict{}); err != nil {
		t.Fatalf("nao gerou o terminal: %v", err)
	}
	html := generate(t, document)

	for name, output := range map[string]string{"terminal": terminal.String(), "html": html} {
		if !strings.Contains(output, "http, kafka") {
			t.Fatalf("o %s nao disse quais protocolos o binario carrega", name)
		}
	}
}
