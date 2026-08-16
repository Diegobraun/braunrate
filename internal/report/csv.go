package report

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/Diegobraun/braunrate/internal/metrics"
)

// O CSV existe para planilha e para juntar execucoes; o campo tipo_de_latencia
// vai junto porque uma coluna de p95 sem ele mistura latencia corrigida com
// tempo de servico na mesma media.
func CSV(out io.Writer, document metrics.Document) error {
	writer := csv.NewWriter(out)
	defer writer.Flush()

	header := []string{
		"cenario", "alvo", "inicio", "passo", "tipo_de_latencia", "contagem", "erros",
		"p50_ms", "p95_ms", "p99_ms", "p99_9_ms", "max_ms", "bytes",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	start := document.Run.Start.Format("2006-01-02T15:04:05Z07:00")
	line := func(name, kind string, count, errors, bytes int64, distribution metrics.Distribution) []string {
		return []string{
			document.Run.Spec, document.Run.Target, start, name, kind,
			fmt.Sprintf("%d", count), fmt.Sprintf("%d", errors),
			number(distribution.P50), number(distribution.P95), number(distribution.P99),
			number(distribution.P999), number(distribution.Max), fmt.Sprintf("%d", bytes),
		}
	}

	if document.Journey.Started > 0 {
		lost := document.Journey.Started - document.Journey.Completed
		if err := writer.Write(line("jornada inteira", "corrigida", document.Journey.Started, lost, 0, document.Journey.Latency)); err != nil {
			return err
		}
	}
	for _, step := range document.Steps {
		if err := writer.Write(line(step.Name, step.LatencyKind, step.Count, step.Errors, step.Bytes, step.Latency)); err != nil {
			return err
		}
	}
	return writer.Write(line("global", "corrigida", document.Overall.Count, document.Overall.Errors, 0, document.Overall.Latency))
}

func number(value float64) string {
	return fmt.Sprintf("%.3f", value)
}
