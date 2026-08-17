package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/text"
)

func ProgressLine(snapshot metrics.Snapshot, targetRate float64, remaining time.Duration) string {
	alert := ""
	if snapshot.Sent > 0 {
		proportion := float64(snapshot.LateDispatches) / float64(snapshot.Sent)
		if proportion >= 0.01 {
			alert = fmt.Sprintf("  WARNING: the generator is not keeping up with the load (%.1f%% behind schedule)", proportion*100)
		}
	}
	return fmt.Sprintf("load %.0f/s | sent %d | completed %d | errors %d | half within %.1f ms | 99%% within %.1f ms | %s left%s",
		targetRate, snapshot.Sent, snapshot.Completed, snapshot.Errors,
		snapshot.LatencyP50Ms, snapshot.LatencyP99Ms, remaining.Round(time.Second), alert)
}

// No target rate to show: in the closed loop the rate is a result, so what goes
// on screen is what the users are getting, never what they were asked for.
func ClosedProgressLine(snapshot metrics.Snapshot, users int, remaining time.Duration) string {
	return fmt.Sprintf("%d users in a loop | completed %d | errors %d | half within %.1f ms | 99%% within %.1f ms | %s left",
		users, snapshot.Completed, snapshot.Errors,
		snapshot.LatencyP50Ms, snapshot.LatencyP99Ms, remaining.Round(time.Second))
}

// Summary has two layers: the plain-language sentence says what happened, and
// the number sits right below it for whoever needs it.
func Summary(out io.Writer, document metrics.Document, verdict slo.Verdict) error {
	lines := &lineWriter{out: out}
	write := lines.writef

	write("")
	write("%s — against %s", document.Run.Spec, document.Run.Target)
	write("")

	if warning, closed := metrics.ClosedLoopWarning(document); closed {
		write("WARNING: %s", warning)
		write("")
	}

	if !document.Valid() && document.Sanity.Checked {
		write("%s", document.Sanity.Sentence)
		write("")
		for _, finding := range document.Sanity.Findings {
			write("  - %s", finding.Message)
			write("    %s", finding.Evidence)
		}
		write("")
	}

	if len(verdict.Evaluations) > 0 {
		write("%s", verdict.Sentence)
		write("")
	}

	overall := document.Overall
	overallLatency := overall.Reported()
	journey := document.Journey.Reported()
	duration := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(100 * time.Millisecond)
	write("What happened")
	write("  %s requests in %s, %.0f per second, %s of them errors",
		thousands(overall.Count), duration, overall.EffectiveRate, percentage(overall.ErrorRate*100))
	write("  Half the responses within %s; 95%% within %s; 99%% within %s; the worst took %s",
		milliseconds(overallLatency.P50), milliseconds(overallLatency.P95),
		milliseconds(overallLatency.P99), milliseconds(overallLatency.Max))
	write("")

	if document.Journey.Started > 0 {
		write("The whole journey")
		write("  %s", document.Journey.Sentence)
		write("  half %s | 95%% %s | 99%% %s | worst %s",
			milliseconds(journey.P50), milliseconds(journey.P95),
			milliseconds(journey.P99), milliseconds(journey.Max))
		// With a mix, each iteration is one alternative: the journey percentile
		// starts pooling populations of different cost, and the reader looks for a
		// tail where there is a blend. The tool knows it and says so.
		if alternatives := mixedAlternatives(document); alternatives > 1 {
			write("  Each journey here is one of the %d alternatives of the mix, so these percentiles pool", alternatives)
			write("  populations of different cost. To read them apart, use the per-step table.")
		}
		write("")
	}

	writeStepTable(lines, document)

	if len(verdict.Evaluations) > 0 || len(verdict.Undeclared) > 0 {
		write("SLO")
		for _, evaluation := range verdict.Evaluations {
			mark := "ok  "
			switch {
			case evaluation.Untrustworthy:
				mark = "?    "
			case !evaluation.Passed:
				mark = "FAIL"
			}
			write("  %-5s %s", mark, evaluation.Sentence)
		}
		for _, missing := range verdict.Undeclared {
			write("  --    %s", missing)
		}
		write("")
	}

	errors := errorLines(document)
	if len(errors) > 0 {
		write("Errors")
		write("  %-26s %-34s %10s   %s", "step", "what happened", "count", "example")
		for _, line := range errors {
			write("  %-26s %-34s %10s   %s", trim(line.step, 26), trim(line.class, 34), thousands(line.count), trim(line.example, exampleWidth))
		}
		// The column that fits the table was cutting the messages exactly where
		// the cause is: what survived was the URL, which the reader already
		// knew, and what was lost was the part that says what to do.
		for _, line := range errors {
			if len(line.example) > exampleWidth {
				write("    %s", line.example)
			}
		}
		write("")
	}

	writeConsumerLag(lines, document)

	write("How trustworthy the measurement is")
	for _, warning := range document.Warnings {
		if warning.Severity == metrics.SeverityHigh {
			// Already reported at the top, as a sanity finding.
			if document.Sanity.Checked {
				continue
			}
			write("  INVALID RESULT: %s", warning.Message)
		} else {
			write("  Warning: %s", warning.Message)
		}
		write("            %s", warning.Evidence)
	}
	if document.Closed() {
		write("  There is no schedule to compare against: the effective rate of %.0f/s was a consequence of the", document.Overall.EffectiveRate)
		write("  target response time, not a declared load. If the target slows down, the load drops with it.")
	} else {
		if document.Scheduling.LateDispatches == 0 && document.Scheduling.DroppedByInflightLimit == 0 {
			write("  Every request went out on schedule, so the numbers above reflect the target, not the generator.")
		}
		write("  Typical delay to fire: %s; worst case: %s (the response time already discounts it)",
			milliseconds(document.Scheduling.Skew.P50), milliseconds(document.Scheduling.Skew.Max))
		hidden := document.Overall.Latency.P99 - document.Overall.ServiceLatency.P99
		if hidden >= 1 {
			write("  A closed-loop tool would have reported %s less at the 99%%.", milliseconds(hidden))
		}
	}
	write("")

	write("Environment")
	write("  %s %s/%s, %d cores | braunrate %s | %s",
		document.Environment.Host, document.Environment.OS, document.Environment.Arch,
		document.Environment.Cores, document.Version, document.Run.Start.Format("2006-01-02 15:04:05"))
	if len(document.Environment.Protocols) > 0 {
		write("  Compiled protocols: %s", strings.Join(document.Environment.Protocols, ", "))
	}
	for _, broker := range document.Run.Brokers {
		write("  Messaging: %s", broker)
	}
	for _, variety := range document.Variety {
		if !variety.Notable() {
			continue
		}
		write("  %s", variety.Sentence)
	}
	if len(document.Run.Seeds) > 0 {
		write("  Data seeds: %s (the same seed generates the same values again)",
			seeds(document.Run.Seeds, document.Run.SeedsFrom))
		// A seed that came from the environment is a number nobody knows how to
		// reproduce later — unless the report prints the command line that brings
		// this same case back.
		if repeat := repeatWithSeeds(document.Run); repeat != "" {
			write("  To repeat exactly this data, run again with %s", repeat)
		}
	}
	if document.Run.AuthObtains > 0 {
		write("  Auth obtained %s and reused by every journey.",
			text.Times(document.Run.AuthObtains))
		write("  If the target has caching, rate limiting or sharding by token, this number comes out optimistic.")
	}
	write("")
	return lines.err
}

func seeds(values map[string]int64, origins map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		if origin, fromEnvironment := origins[name]; fromEnvironment {
			parts = append(parts, fmt.Sprintf("%s=%d (de $%s)", name, values[name], origin))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", name, values[name]))
	}
	return strings.Join(parts, ", ")
}

func repeatWithSeeds(run metrics.Run) string {
	names := make([]string, 0, len(run.SeedsFrom))
	for name := range run.SeedsFrom {
		names = append(names, name)
	}
	sort.Strings(names)
	assignments := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		variable := run.SeedsFrom[name]
		if seen[variable] {
			continue
		}
		seen[variable] = true
		assignments = append(assignments, fmt.Sprintf("%s=%d", variable, run.Seeds[name]))
	}
	return strings.Join(assignments, " ")
}

type errorLine struct {
	step    string
	class   string
	count   int64
	example string
}

var classNames = map[string]string{
	"network":       "network failure",
	"timeout":       "timed out",
	"status":        "unexpected HTTP status",
	"assertion":     "content outside what was expected",
	"correlation":   "I could not capture a value",
	"config":        "configuration error in the scenario",
	"auth":          "I could not authenticate",
	"authorization": "credential accepted, no permission on that resource",
	"messaging":     "the broker refused the message",
	"saturation":    "the generator did not sustain the rate",
	"graphql":       "error in the GraphQL response body (with status 200)",
}

// A class with no entry here used to print an empty line, which says less than
// the raw name of the class.
func className(class string) string {
	if name := classNames[class]; name != "" {
		return name
	}
	return class
}

// One line per class was the whole error section: "status HTTP inesperado 60"
// does not say which status, nor in which step, and both are in the JSON.
func errorLines(document metrics.Document) []errorLine {
	var lines []errorLine
	for _, step := range document.Steps {
		for class, count := range step.ErrorsByClass {
			lines = append(lines, errorLine{
				step:    step.Name,
				class:   className(class),
				count:   count,
				example: mostFrequent(step.Details, class),
			})
		}
	}
	sort.Slice(lines, func(first, second int) bool {
		if lines[first].count != lines[second].count {
			return lines[first].count > lines[second].count
		}
		return lines[first].step < lines[second].step
	})
	return lines
}

// The detail map holds every distinct message; the most frequent one is the one
// worth a line, and the count already says how many there were.
func mostFrequent(details map[string]int64, class string) string {
	best, most := "", int64(0)
	for detail, count := range details {
		if count > most || (count == most && detail < best) {
			best, most = detail, count
		}
	}
	if best == "" {
		return class
	}
	return strings.Join(strings.Fields(best), " ")
}

func milliseconds(value float64) string {
	switch {
	case value >= 1000:
		return fmt.Sprintf("%.2f s", value/1000)
	case value >= 10:
		return fmt.Sprintf("%.0f ms", value)
	case value >= 1:
		return fmt.Sprintf("%.1f ms", value)
	default:
		return fmt.Sprintf("%.3f ms", value)
	}
}

func percentage(value float64) string {
	if value == 0 {
		return "0%"
	}
	if value < 0.01 {
		return fmt.Sprintf("%.4f%%", value)
	}
	return fmt.Sprintf("%.2f%%", value)
}

func thousands(value int64) string {
	text := fmt.Sprintf("%d", value)
	if len(text) <= 3 {
		return text
	}
	var parts []string
	for len(text) > 3 {
		parts = append([]string{text[len(text)-3:]}, parts...)
		text = text[:len(text)-3]
	}
	parts = append([]string{text}, parts...)
	return strings.Join(parts, ",")
}

// Wide enough for a short cause, narrow enough to keep the table readable.
const exampleWidth = 44

func trim(text string, size int) string {
	if len(text) <= size {
		return text
	}
	return strings.TrimSpace(text[:size-1]) + "…"
}

// The header over an empty table says "there is nothing here" in the least
// useful way there is.
func writeStepTable(output *lineWriter, document metrics.Document) {
	write := output.writef
	never := metrics.StepsThatNeverRan(document)

	write("Per step")
	if len(document.Steps) == 0 && len(never) == 0 {
		write("  No step recorded a sample: the run never got to measure anything.")
		write("  Run 'braunrate debug' to see where the iteration stops.")
		write("")
		return
	}

	write("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7s", "step", "", "requests", "half", "95%", "99%", "99.9%", "worst", "errors")
	hasServiceStep := false
	for _, step := range document.Steps {
		mark := "(1)"
		if step.LatencyKind == string(metrics.ServiceLatency) {
			mark = "(2)"
			hasServiceStep = true
		}
		write("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7d",
			trim(step.Name, 26), mark, thousands(step.Count),
			milliseconds(step.Reported().P50), milliseconds(step.Reported().P95),
			milliseconds(step.Reported().P99), milliseconds(step.Reported().P999),
			milliseconds(step.Reported().Max), step.Errors)
	}
	// A step that never ran used to vanish from here, and whoever read the
	// report never found out it existed.
	for _, name := range never {
		write("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7s",
			trim(name, 26), "", "0", "\u2014", "\u2014", "\u2014", "\u2014", "\u2014", "\u2014")
	}
	if len(never) > 0 {
		write("")
		write("  A step with a dash never got to run: the iteration stopped before it. The reason")
		write("  is under \"Errors\", on the step that failed first.")
	}
	write("")

	if document.Closed() {
		write("  (2) plain response time. In a closed loop there is no scheduled instant: the")
		write("      virtual user only asks again after the previous response, so no queueing")
		write("      delay shows up in these numbers.")
	} else {
		write("  (1) time counted from the instant the request should have gone out \u2014 it includes")
		write("      any delay, and for that reason it does not hide a freeze in the target.")
		if hasServiceStep {
			write("  (2) plain response time, counted from when the previous step finished. Because")
			write("      that step depends on a value captured before it, it has no scheduled")
			write("      instant of its own. For the reading that includes the wait, use \"The whole journey\".")
		}
	}
	write("")
	writeMix(output, document)
}

// A declared 60% that came out 45% is information, not detail: the proportion
// is what makes the load a mix instead of three scenarios, and a proportion
// that was not met changes what the number means. It only shows up when the
// scenario declares a mix.
func mixedAlternatives(document metrics.Document) int {
	declared := 0
	for _, step := range document.Steps {
		if step.DeclaredShare > 0 {
			declared++
		}
	}
	return declared
}

func writeMix(output *lineWriter, document metrics.Document) {
	total := int64(0)
	declared := false
	for _, step := range document.Steps {
		total += step.Count
		if step.DeclaredShare > 0 {
			declared = true
		}
	}
	if !declared || total == 0 {
		return
	}
	write := output.writef
	write("Mix declared and observed")
	for _, step := range document.Steps {
		if step.DeclaredShare <= 0 {
			continue
		}
		observed := float64(step.Count) / float64(total)
		write("  %-26s %6.1f%% declared   %6.1f%% observed (%s of %s)",
			trim(step.Name, 26), step.DeclaredShare*100, observed*100,
			thousands(step.Count), thousands(total))
	}
	write("")
}

// Producing fast says the broker accepted the message. Whether the service kept
// up is a different number, and it is the one that decides if the chain held.
func writeConsumerLag(output *lineWriter, document metrics.Document) {
	if len(document.Run.ConsumerLag) == 0 {
		return
	}
	write := output.writef
	write("Consumer lag")
	for _, lag := range document.Run.ConsumerLag {
		headline, note := lagSentences(lag)
		write("  group %s on %s: %s", lag.Group, lag.Topic, headline)
		if note != "" {
			write("  %s", note)
		}
	}
	write("")
}
