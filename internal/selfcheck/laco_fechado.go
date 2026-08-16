// Package selfcheck drives a target the way the tools braunrate is meant to
// replace drive it, so the difference between the two measurements is measured
// instead of argued.
package selfcheck

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

const (
	microsecondsInMillisecond = 1000
	histogramFloor            = 1
	histogramCeiling          = 600_000_000
	histogramPrecision        = 3
)

type ClosedLoopResult struct {
	Samples int64
	P50     float64
	P99     float64
	Max     float64
}

// RunClosedLoop only sends the next request after the previous one answers.
// That is how JMeter and Locust measure, and it is why a pause on the target
// disappears from their percentiles: during the pause nothing is sent, so the
// requests that should have gone out never enter the count.
//
// This lives outside the test file because the demonstration the product is
// built on cannot be measured by one piece of code in the test suite and a
// different one in the tool people run.
func RunClosedLoop(runContext context.Context, address, path string, duration time.Duration) ClosedLoopResult {
	histogram := hdrhistogram.New(histogramFloor, histogramCeiling, histogramPrecision)
	client := &http.Client{Timeout: 30 * time.Second}
	limit := time.Now().Add(duration)

	for time.Now().Before(limit) {
		if runContext.Err() != nil {
			break
		}
		start := time.Now()
		request, err := http.NewRequestWithContext(runContext, http.MethodGet, address+path, nil)
		if err != nil {
			break
		}
		response, err := client.Do(request)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		_ = histogram.RecordValue(time.Since(start).Microseconds())
	}

	return ClosedLoopResult{
		Samples: histogram.TotalCount(),
		P50:     float64(histogram.ValueAtQuantile(50)) / microsecondsInMillisecond,
		P99:     float64(histogram.ValueAtQuantile(99)) / microsecondsInMillisecond,
		Max:     float64(histogram.Max()) / microsecondsInMillisecond,
	}
}
