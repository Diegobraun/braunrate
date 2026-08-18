package report

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/Diegobraun/braunrate/internal/metrics"
)

// CSV exists for spreadsheets and for stitching runs together; the latencyKind
// column goes with it because a p95 column without it mixes corrected latency
// and service time in the same average.
func CSV(out io.Writer, document metrics.Document) error {
	writer := csv.NewWriter(out)
	defer writer.Flush()

	header := []string{
		"scenario", "target", "start", "step", "latencyKind", "count", "errors",
		"p50Ms", "p95Ms", "p99Ms", "p999Ms", "maxMs", "bytes", "messages",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	start := document.Run.Start.Format("2006-01-02T15:04:05Z07:00")
	line := func(name, kind string, count, errors, bytes, messages int64, distribution metrics.Distribution) []string {
		return []string{
			document.Run.Spec, document.Run.Target, start, name, kind,
			fmt.Sprintf("%d", count), fmt.Sprintf("%d", errors),
			number(distribution.P50), number(distribution.P95), number(distribution.P99),
			number(distribution.P999), number(distribution.Max), fmt.Sprintf("%d", bytes), fmt.Sprintf("%d", messages),
		}
	}

	if document.Journey.Started > 0 {
		lost := document.Journey.Started - document.Journey.Completed
		if err := writer.Write(line("whole journey", "corrected", document.Journey.Started, lost, 0, 0, document.Journey.Reported())); err != nil {
			return err
		}
	}
	for _, step := range document.Steps {
		if err := writer.Write(line(step.Name, step.LatencyKind, step.Count, step.Errors, step.Bytes, step.Messages, step.Latency)); err != nil {
			return err
		}
	}
	return writer.Write(line("global", "corrected", document.Overall.Count, document.Overall.Errors, 0, 0, document.Overall.Reported()))
}

func number(value float64) string {
	return fmt.Sprintf("%.3f", value)
}
