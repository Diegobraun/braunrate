// Package demo is the first thing someone runs. It stands up a target, writes
// a scenario, executes it and explains the numbers while they appear — because
// the audience includes people who never ran a load test, and a report is not
// self-explanatory to someone who has never seen one.
package demo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/selfcheck"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

const (
	rate           = "100/s"
	duration       = "5s"
	runDuration    = 5 * time.Second
	targetLatency  = 5 * time.Millisecond
	freezeAfter    = 2 * time.Second
	freezeDuration = 2 * time.Second
	preferredPort  = "127.0.0.1:8080"
	anyFreePort    = "127.0.0.1:0"
	closedLoopPath = "/order"
)

type Options struct {
	WithFailure bool
	Directory   string
	Version     string
	Output      io.Writer
}

func Run(runContext context.Context, options Options) error {
	if options.WithFailure {
		return runFreezing(runContext, options)
	}
	return runHealthy(runContext, options)
}

func runHealthy(runContext context.Context, options Options) error {
	say(options, `
This demonstration runs against a fake service started right here, so you can
try it out without affecting anything.

`)

	target, stop, err := startTarget(options, testsupport.Options{Latency: targetLatency})
	if err != nil {
		return err
	}
	defer stop()

	say(options, `[1/3] Starting an example service at %s
      It answers in ~%d ms, the way a healthy API would.

`, address(target), targetLatency.Milliseconds())

	scenarioPath := filepath.Join(options.Directory, "demo.yaml")
	if err := write(scenarioPath, healthyScenario(target)); err != nil {
		return err
	}

	say(options, `[2/3] Running: %s requests per second, for %s.

      That is the rate: braunrate fires at that pace whether the service is
      fast or slow — the way real users do. Tools that wait for the previous
      response before sending the next one go easy on the system exactly when
      it is struggling.

      The scenario that is running was left at %s, commented.

`, strings.TrimSuffix(rate, "/s"), duration, scenarioPath)

	result, err := runner.Execute(runContext, scenarioPath, runner.DefaultOptions(options.Version))
	if err != nil {
		return err
	}
	document := result.Document

	say(options, "[3/3] Done. What the numbers say:\n\n")
	sayMeasurement(options, document)
	sayVerdict(options, document)
	sayFixedDataCaveat(options, document, scenarioPath)

	htmlPath := filepath.Join(options.Directory, "demo-report.html")
	if err := runner.WriteHTML(htmlPath, document); err != nil {
		return err
	}

	say(options, `Full report: %s
Both files were left in the current directory; delete them whenever you want.

Want to see the tool catching a real problem?

    braunrate demo --with-failure

`, htmlPath)
	return nil
}

func runFreezing(runContext context.Context, options Options) error {
	say(options, `
This demonstration measures the same frozen service in two ways, and shows what
each one reports.

`)

	freezing := testsupport.Options{
		Latency:     targetLatency,
		FreezeAfter: freezeAfter,
		FreezeFor:   freezeDuration,
	}
	target, stop, err := startTarget(options, freezing)
	if err != nil {
		return err
	}
	defer stop()

	say(options, `[1/4] Starting an example service at %s, with one difference: it
      freezes for %d seconds halfway through the run. It is what a long GC, a
      lock or a failover does to a real service.

`, address(target), int(freezeDuration.Seconds()))

	scenarioPath := filepath.Join(options.Directory, "demo-with-failure.yaml")
	if err := write(scenarioPath, freezingScenario(target)); err != nil {
		return err
	}

	say(options, "[2/4] Running braunrate: %s per second for %s.\n\n",
		strings.TrimSuffix(rate, "/s"), duration)

	result, err := runner.Execute(runContext, scenarioPath, runner.DefaultOptions(options.Version))
	if err != nil {
		return err
	}
	document := result.Document

	say(options, `[3/4] Now a closed loop, against an identical service that freezes the same
      way. A closed loop is how JMeter and Locust measure: the next request
      only goes out after the previous one answers.

`)

	closedTarget, stopClosed, err := startTwinTarget(freezing)
	if err != nil {
		return err
	}
	defer stopClosed()
	closed := selfcheck.RunClosedLoop(runContext, closedTarget, closedLoopPath, runDuration)

	open := document.Overall.Reported()
	say(options, `[4/4] Same freeze, same kind of target, same request, two measurements:

      closed loop (JMeter, Locust):  99%% within %.1f ms over %d requests
      braunrate (open model):        99%% within %.1f ms over %d requests

      %.1f ms the closed loop never counted.

      The closed loop does not lie because of a bug. When the target freezes it
      stops sending, and the requests that should have gone out never enter the
      count — including the ones a real user would have sent. braunrate counts
      from the instant the request should have gone out, so the freeze shows up.

`, closed.P99, closed.Samples, open.P99, document.Overall.Count, open.P99-closed.P99)

	sayMeasurement(options, document)
	sayVerdict(options, document)
	if result.Exit == runner.ExitSLO {
		say(options, `      If this were your CI, braunrate would have exited with code 1 and the
      pipeline would fail. With a closed-loop measurement, the same criterion
      would pass.

`)
	}

	htmlPath := filepath.Join(options.Directory, "demo-with-failure-report.html")
	if err := runner.WriteHTML(htmlPath, document); err != nil {
		return err
	}
	say(options, "Full report: %s\n\n", htmlPath)
	return nil
}

func sayMeasurement(options Options, document metrics.Document) {
	overall := document.Overall
	latency := overall.Reported()
	elapsed := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(100 * time.Millisecond)
	say(options, `  %d requests in %s, %.0f per second, %.2f%% of them errors
  Half the responses within %.1f ms; 95%% within %.1f ms; the worst took %.0f ms

      Notice there is no average on that line. An average hides things: if 95
      responses take 5 ms and 5 take 2 seconds, the average reads 105 ms and
      nobody notices the five slow ones. "95%% within %.1f ms" means 5%% of the
      people waited longer than that.

`, overall.Count, elapsed, overall.EffectiveRate, overall.ErrorRate*100,
		latency.P50, latency.P95, latency.Max, latency.P95)
}

func sayVerdict(options Options, document metrics.Document) {
	if !document.Valid() && document.Sanity.Checked {
		say(options, "  %s\n", document.Sanity.Sentence)
		for _, finding := range document.Sanity.Findings {
			say(options, "      %s\n", finding.Message)
		}
		say(options, `
      An invalid result is not the same as a bad result: it means the run did
      not measure what it set out to measure, and no number above works as an
      answer.

`)
		return
	}
	for _, evaluation := range document.SLO.Evaluations {
		mark := "ok  "
		if !evaluation.Passed {
			mark = "FAIL"
		}
		say(options, "  %-5s %s\n", mark, evaluation.Sentence)
	}
	if len(document.SLO.Evaluations) > 0 {
		say(options, `
      That is an acceptance criterion: a limit you declare in the file. If the
      run goes over it, braunrate exits with code 1 — you can wire it straight
      into your CI.

`)
	}
}

// The report already raises this; repeating it here is not decoration. Someone
// reading their first report does not know that a fixed path measures the
// cache, and the demo is exactly where that is learned.
func sayFixedDataCaveat(options Options, document metrics.Document, scenarioPath string) {
	for _, warning := range document.Warnings {
		if warning.Kind != "stepWithoutVariation" && warning.Kind != "fixedValue" && warning.Kind != "missingVariety" {
			continue
		}
		say(options, `  A caveat the report itself raises:
      %s
      An always-identical request measures the target cache, not the target. In
      %s, swap /orders/1 for /orders/${id} and declare where ${id} comes from.

`, warning.Message, scenarioPath)
		return
	}
}

// A busy 8080 is the common case on a developer machine, and a demo that dies
// on it is a demo nobody sees. Any free port serves; what cannot happen is the
// address changing without the person being told, because the scenario file on
// disk carries it.
func startTarget(options Options, targetOptions testsupport.Options) (string, func(), error) {
	server := testsupport.New(targetOptions)
	if err := server.Start(preferredPort); err != nil {
		if err := server.Start(anyFreePort); err != nil {
			return "", nil, fmt.Errorf("could not start the example target: %w", err)
		}
		say(options, "      (%s is busy, so the target came up at %s)\n\n",
			preferredPort, address(server.Address()))
	}
	return server.Address(), func() { _ = server.Close() }, nil
}

// The twin the closed loop drives never appears in a file and never gets
// asked for by address, so it takes whatever port is free without a word about
// it.
func startTwinTarget(targetOptions testsupport.Options) (string, func(), error) {
	server := testsupport.New(targetOptions)
	if err := server.Start(anyFreePort); err != nil {
		return "", nil, fmt.Errorf("could not start the second example target: %w", err)
	}
	return server.Address(), func() { _ = server.Close() }, nil
}

func address(target string) string { return strings.TrimPrefix(target, "http://") }

func write(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return nil
}

func say(options Options, format string, args ...any) {
	if options.Output == nil {
		return
	}
	_, _ = fmt.Fprintf(options.Output, format, args...)
}
