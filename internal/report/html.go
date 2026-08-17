package report

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/text"
)

type htmlPage struct {
	Document          metrics.Document
	Title             string
	Verdict           htmlVerdict
	Journey           metrics.Journey
	Steps             []htmlStep
	Mix               []htmlMixLine
	HasNeverRan       bool
	HasServiceLatency bool
	ClosedLoop        string
	Warnings          []htmlWarning
	Errors            []htmlError
	ConsumerLag       []htmlLag
	Count             string
	Rate              string
	ErrorRate         string
	P95               string
	JourneyP50        string
	JourneyP95        string
	JourneyP99        string
	JourneyMax        string
	Chart             template.HTML
	HasChart          bool
	Plan              []string
	Environment       []string
	Reliability       []string
	GeneratedAt       string
}

type htmlVerdict struct {
	Sentence    string
	Class       string
	Subtitle    string
	Evaluations []metrics.Evaluation
	Undeclared  []string
}

type htmlStep struct {
	Name      string
	Mark      string
	DeServico bool
	Count     string
	P50       string
	P95       string
	P99       string
	P999      string
	Max       string
	Errors    int64
	HasError  bool
	NeverRan  bool
}

// Peso de 60% que virou 45% na execucao e informacao, nao detalhe. So aparece
// quando o cenario declara mix.
type htmlMixLine struct {
	Name     string
	Declared string
	Observed string
	Count    string
	Total    string
}

type htmlError struct {
	Step    string
	Class   string
	Count   string
	Example string
}

type htmlLag struct {
	Group    string
	Topic    string
	Headline string
	Note     string
	Readings string
}

type htmlWarning struct {
	Class    string
	Label    string
	Message  string
	Evidence string
}

// HTML writes the report with a sentence on top instead of a table: whoever
// opens the file needs to know whether it passed before knowing how many
// milliseconds it took.
func HTML(out io.Writer, document metrics.Document) error {
	page := buildPage(document)
	return htmlTemplate.Execute(out, page)
}

func buildPage(document metrics.Document) htmlPage {
	page := htmlPage{
		Document:    document,
		Title:       document.Run.Spec,
		Journey:     document.Journey,
		GeneratedAt: document.Run.Start.Format("02/01/2006 15:04:05"),
	}

	overall := document.Overall.Reported()
	journey := document.Journey.Reported()

	page.ClosedLoop, _ = metrics.ClosedLoopWarning(document)
	page.Verdict = buildVerdict(document)
	page.Count = thousands(document.Overall.Count)
	page.Rate = fmt.Sprintf("%.0f", document.Overall.EffectiveRate)
	page.ErrorRate = percentage(document.Overall.ErrorRate * 100)
	page.P95 = milliseconds(overall.P95)
	page.JourneyP50 = milliseconds(journey.P50)
	page.JourneyP95 = milliseconds(journey.P95)
	page.JourneyP99 = milliseconds(journey.P99)
	page.JourneyMax = milliseconds(journey.Max)

	for _, step := range document.Steps {
		line := htmlStep{
			Name:     step.Name,
			Mark:     "1",
			Count:    thousands(step.Count),
			P50:      milliseconds(step.Reported().P50),
			P95:      milliseconds(step.Reported().P95),
			P99:      milliseconds(step.Reported().P99),
			P999:     milliseconds(step.Reported().P999),
			Max:      milliseconds(step.Reported().Max),
			Errors:   step.Errors,
			HasError: step.Errors > 0,
		}
		if step.LatencyKind == string(metrics.ServiceLatency) {
			line.Mark = "2"
			line.DeServico = true
			page.HasServiceLatency = true
		}
		page.Steps = append(page.Steps, line)
	}
	page.Mix = mixLines(document)

	// A step that never ran used to vanish from the table, and whoever read the
	// report never found out it existed.
	for _, name := range metrics.StepsThatNeverRan(document) {
		page.Steps = append(page.Steps, htmlStep{
			Name: name, Count: "0", P50: "—", P95: "—", P99: "—", P999: "—", Max: "—", NeverRan: true,
		})
		page.HasNeverRan = true
	}

	for _, finding := range document.Sanity.Findings {
		page.Warnings = append(page.Warnings, htmlWarning{
			Class: "high", Label: "invalid result",
			Message: finding.Message, Evidence: finding.Evidence,
		})
	}
	for _, warning := range document.Warnings {
		line := htmlWarning{Class: "low", Label: "observation", Message: warning.Message, Evidence: warning.Evidence}
		switch warning.Severity {
		case metrics.SeverityHigh:
			// Already listed above, as a sanity finding.
			if document.Sanity.Checked {
				continue
			}
			line.Class, line.Label = "high", "invalid result"
		case metrics.SeverityMedium:
			line.Class, line.Label = "medium", "warning"
		}
		page.Warnings = append(page.Warnings, line)
	}

	for _, line := range errorLines(document) {
		page.Errors = append(page.Errors, htmlError{
			Step: line.step, Class: line.class, Count: thousands(line.count), Example: line.example,
		})
	}
	for _, lag := range document.Run.ConsumerLag {
		headline, note := lagSentences(lag)
		page.ConsumerLag = append(page.ConsumerLag, htmlLag{
			Group: lag.Group, Topic: lag.Topic,
			Headline: headline, Note: note,
			Readings: text.Count(int64(lag.Readings), "reading", "readings"),
		})
	}
	page.Reliability = reliabilitySentences(document)
	page.Plan = planSentences(document)
	page.Environment = environmentSentences(document)

	if drawing, hasData := drawSeries(document.Series); hasData {
		page.Chart = drawing
		page.HasChart = true
	}
	return page
}

func buildVerdict(document metrics.Document) htmlVerdict {
	verdict := htmlVerdict{Evaluations: document.SLO.Evaluations, Undeclared: document.SLO.Undeclared}

	if !document.Valid() {
		verdict.Class = "invalid"
		verdict.Sentence = "Invalid result: the run did not measure what it set out to measure."
		verdict.Subtitle = "This is not a verdict on the target — it is the measurement that does not hold, and that is why no SLO rule was evaluated."
		if !document.Sanity.Checked {
			verdict.Sentence = "Invalid result: the generator did not sustain the declared load."
			verdict.Subtitle = "The numbers below measure the generator, not the target. Run again with a lower rate or on a bigger machine before drawing any conclusion."
		}
		return verdict
	}

	switch {
	case len(document.SLO.Evaluations) == 0:
		verdict.Class = "neutral"
		verdict.Sentence = fmt.Sprintf("%s answered %s requests with %s of them errors.",
			document.Run.Target, thousands(document.Overall.Count), percentage(document.Overall.ErrorRate*100))
		verdict.Subtitle = "No slo declared: this run describes, but neither approves nor rejects. Declare an 'slo' block to turn it into a CI gate."
	case document.SLO.Passed:
		verdict.Class = "passed"
		verdict.Sentence = document.SLO.Sentence
		verdict.Subtitle = phraseVolume(document)
	default:
		verdict.Class = "failed"
		verdict.Sentence = document.SLO.Sentence
		verdict.Subtitle = phraseVolume(document)
	}
	return verdict
}

func phraseVolume(document metrics.Document) string {
	duration := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(time.Second)
	if document.Journey.Started > 0 {
		return fmt.Sprintf("%s journeys in %s, %s requests at %.0f per second, %s of them errors.",
			thousands(document.Journey.Started), duration, thousands(document.Overall.Count),
			document.Overall.EffectiveRate, percentage(document.Overall.ErrorRate*100))
	}
	return fmt.Sprintf("%s requests in %s, %.0f per second, %s of them errors.",
		thousands(document.Overall.Count), duration, document.Overall.EffectiveRate,
		percentage(document.Overall.ErrorRate*100))
}

// The axis shows few digits on purpose: an axis number is a reading reference,
// and the exact value is already in the table above.
func axisLabel(value float64) string {
	switch {
	case value == 0:
		return "0"
	case value >= 100:
		return fmt.Sprintf("%.0f ms", value)
	case value >= 10:
		return fmt.Sprintf("%.0f ms", value)
	default:
		return fmt.Sprintf("%.1f ms", value)
	}
}

func reliabilitySentences(document metrics.Document) []string {
	var sentences []string
	scheduling := document.Scheduling
	if document.Closed() {
		return []string{
			fmt.Sprintf("There is no schedule to compare against: the effective rate of %.0f/s was a consequence of the target response time, not a declared load.",
				document.Overall.EffectiveRate),
			"If the target slows down, the virtual users ask less often and the load drops with it — the delay never shows up in the numbers.",
		}
	}
	if scheduling.LateDispatches == 0 && scheduling.DroppedByInflightLimit == 0 {
		sentences = append(sentences, "Every request went out on schedule, so the numbers above reflect the target, not the generator.")
	}
	sentences = append(sentences, fmt.Sprintf("Typical delay to fire: %s; worst case: %s. The response time already discounts that delay.",
		milliseconds(scheduling.Skew.P50), milliseconds(scheduling.Skew.Max)))

	hidden := document.Overall.Latency.P99 - document.Overall.ServiceLatency.P99
	if hidden >= 1 {
		sentences = append(sentences, fmt.Sprintf("A closed-loop tool would have reported %s less at the 99%%: it is the part of the delay that only shows up when counting from the scheduled instant.",
			milliseconds(hidden)))
	}
	if scheduling.PeakInflight > 0 {
		sentences = append(sentences, fmt.Sprintf("Peak of %s requests at the same time, with a limit of %s.",
			thousands(scheduling.PeakInflight), thousands(document.Run.MaxInflight)))
	}
	return sentences
}

func planSentences(document metrics.Document) []string {
	var sentences []string
	for _, phase := range document.Run.AppliedPlan {
		duration := (time.Duration(phase.DurationMs) * time.Millisecond).Round(time.Second)
		if phase.Kind == "ramp" {
			sentences = append(sentences, fmt.Sprintf("ramp from %.0f/s to %.0f/s over %s", phase.From, phase.To, duration))
			continue
		}
		sentences = append(sentences, fmt.Sprintf("%s at %.0f/s over %s", phase.Kind, phase.To, duration))
	}
	return sentences
}

func environmentSentences(document metrics.Document) []string {
	sentences := []string{
		fmt.Sprintf("%s, %s/%s, %d cores", document.Environment.Host, document.Environment.OS,
			document.Environment.Arch, document.Environment.Cores),
		fmt.Sprintf("braunrate %s (%s), generator and target measured as declared above", document.Version, document.Environment.GoVersion),
	}
	// Two binaries with the same version number can carry different protocols,
	// and without this line the difference would leave no trace (ADR 0004).
	if len(document.Environment.Protocols) > 0 {
		sentences = append(sentences, "Protocols compiled into this binary: "+strings.Join(document.Environment.Protocols, ", ")+".")
	}
	for _, broker := range document.Run.Brokers {
		sentences = append(sentences, "Messaging: "+broker+".")
	}
	for _, variety := range document.Variety {
		if !variety.Notable() {
			continue
		}
		sentences = append(sentences, "Observed variety: "+variety.Sentence+".")
	}
	if len(document.Run.Seeds) > 0 {
		sentence := "Data seeds: " + seeds(document.Run.Seeds, document.Run.SeedsFrom) + " — the same seed generates the same values again."
		if repeat := repeatWithSeeds(document.Run); repeat != "" {
			sentence += " To repeat exactly this data, run again with " + repeat + "."
		}
		sentences = append(sentences, sentence)
	}
	if document.Run.AuthObtains > 0 {
		sentences = append(sentences, fmt.Sprintf("Auth obtained %s and reused by every journey. If the target has caching, rate limiting or sharding by token, this number comes out optimistic.",
			text.Times(document.Run.AuthObtains)))
	}
	return sentences
}

const (
	chartWidth   = 900
	chartHeight  = 260
	marginLeft   = 56
	marginRight  = 16
	marginTop    = 16
	marginBottom = 34
)

// The chart is hand-written SVG because the report has to open with no
// network: a charting library would come from a CDN and the file would stop
// being self-contained.
func drawSeries(series []metrics.Bucket) (template.HTML, bool) {
	if len(series) < 2 {
		return "", false
	}
	sorted := append([]metrics.Bucket{}, series...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartEpochMs < sorted[j].StartEpochMs })

	maxLatency := 0.0
	for _, bucket := range sorted {
		if bucket.LatencyP99Ms > maxLatency {
			maxLatency = bucket.LatencyP99Ms
		}
	}
	if maxLatency <= 0 {
		return "", false
	}

	width := float64(chartWidth - marginLeft - marginRight)
	height := float64(chartHeight - marginTop - marginBottom)
	step := width / float64(len(sorted)-1)

	y := func(value float64) float64 {
		return marginTop + height - (value/maxLatency)*height
	}

	line := func(choose func(metrics.Bucket) float64) string {
		var points []string
		for index, bucket := range sorted {
			x := float64(marginLeft) + float64(index)*step
			points = append(points, fmt.Sprintf("%.1f,%.1f", x, y(choose(bucket))))
		}
		return strings.Join(points, " ")
	}

	var svg strings.Builder
	fmt.Fprintf(&svg, `<svg viewBox="0 0 %d %d" role="img" aria-label="response time per second">`, chartWidth, chartHeight)

	for _, fraction := range []float64{0, 0.5, 1} {
		value := maxLatency * (1 - fraction)
		y := marginTop + height*fraction
		fmt.Fprintf(&svg, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" class="grid"/>`,
			marginLeft, y, chartWidth-marginRight, y)
		fmt.Fprintf(&svg, `<text x="%d" y="%.1f" class="eixo" text-anchor="end">%s</text>`,
			marginLeft-8, y+4, template.HTMLEscapeString(axisLabel(value)))
	}

	for _, bucket := range sorted {
		if bucket.Errors == 0 {
			continue
		}
		index := indiceDe(sorted, bucket.StartEpochMs)
		x := float64(marginLeft) + float64(index)*step
		fmt.Fprintf(&svg, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%.1f" class="error"/>`,
			x, marginTop, x, marginTop+height)
	}

	fmt.Fprintf(&svg, `<polyline class="p99" points="%s"/>`, line(func(b metrics.Bucket) float64 { return b.LatencyP99Ms }))
	fmt.Fprintf(&svg, `<polyline class="p50" points="%s"/>`, line(func(b metrics.Bucket) float64 { return b.LatencyP50Ms }))

	first := sorted[0].StartEpochMs
	for index, bucket := range sorted {
		if index%maximum(1, len(sorted)/8) != 0 {
			continue
		}
		x := float64(marginLeft) + float64(index)*step
		seconds := (bucket.StartEpochMs - first) / 1000
		fmt.Fprintf(&svg, `<text x="%.1f" y="%d" class="axis" text-anchor="middle">%ds</text>`,
			x, chartHeight-12, seconds)
	}

	svg.WriteString(`</svg>`)
	return template.HTML(svg.String()), true
}

func indiceDe(buckets []metrics.Bucket, epoch int64) int {
	for index, bucket := range buckets {
		if bucket.StartEpochMs == epoch {
			return index
		}
	}
	return 0
}

func maximum(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var htmlTemplate = template.Must(template.Must(template.New("report").Parse(pageStyle)).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — braunrate</title>
{{template "style"}}
</head>
<body>
<main>
<header>
  <div class="scenario">{{.Title}} — {{.Document.Run.Target}}</div>
  <h1 class="{{.Verdict.Class}}">{{.Verdict.Sentence}}</h1>
  {{if .Verdict.Subtitle}}<p class="subtitle">{{.Verdict.Subtitle}}</p>{{end}}
</header>

{{if .ClosedLoop}}
<div class="warning medium">
  <div class="label">closed loop</div>
  <div>{{.ClosedLoop}}</div>
</div>
{{end}}

{{range .Warnings}}
<div class="warning {{.Class}}">
  <div class="label">{{.Label}}</div>
  <div>{{.Message}}</div>
  <div class="evidence">{{.Evidence}}</div>
</div>
{{end}}

<h2>What happened</h2>
<ul class="numbers">
  <li><div class="value">{{.Count}}</div><div class="label">requests</div></li>
  <li><div class="value">{{.Rate}}</div><div class="label">per second</div></li>
  <li><div class="value">{{.ErrorRate}}</div><div class="label">of them errors</div></li>
  <li><div class="value">{{.P95}}</div><div class="label">95% of the responses within</div></li>
</ul>

{{if .Journey.Started}}
<h2>The whole journey</h2>
<div class="reading">{{.Journey.Sentence}}</div>
<table>
  <tr><th>journey</th><th>half</th><th>95%</th><th>99%</th><th>worst</th></tr>
  <tr>
    <td>from the scheduled instant to the last step</td>
    <td>{{.JourneyP50}}</td><td>{{.JourneyP95}}</td>
    <td>{{.JourneyP99}}</td><td>{{.JourneyMax}}</td>
  </tr>
</table>
{{end}}

<h2>Per step</h2>
{{if not .Steps}}
<p class="note">No step recorded a sample: the run never got to measure anything. Run <code>braunrate debug</code> to see where the iteration stops.</p>
{{else}}
<table>
  <tr><th>step</th><th>requests</th><th>half</th><th>95%</th><th>99%</th><th>99.9%</th><th>worst</th><th>errors</th></tr>
  {{range .Steps}}
  <tr>
    <td>{{if .NeverRan}}{{.Name}}{{else}}<span class="mark">({{.Mark}})</span> {{.Name}}{{end}}</td>
    <td>{{.Count}}</td><td>{{.P50}}</td><td>{{.P95}}</td><td>{{.P99}}</td>
    <td>{{.P999}}</td><td>{{.Max}}</td>
    <td{{if .HasError}} class="error"{{end}}>{{if .NeverRan}}—{{else}}{{.Errors}}{{end}}</td>
  </tr>
  {{end}}
</table>
{{if .HasNeverRan}}
<p class="note">A step with a dash never got to run: the iteration stopped before it. The reason is under "Errors", on the step that failed first.</p>
{{end}}
{{end}}
{{- if .Mix}}
<h2>Mix declared and observed</h2>
<table>
  <tr><th>alternative</th><th>declared</th><th>observed</th><th>requests</th></tr>
  {{range .Mix}}
  <tr><td>{{.Name}}</td><td>{{.Declared}}</td><td>{{.Observed}}</td><td>{{.Count}} of {{.Total}}</td></tr>
  {{end}}
</table>
{{- end}}
{{if .ClosedLoop}}
<p class="note">(2) plain response time. In a closed loop there is no scheduled instant: the virtual user only asks again after the previous response, so no queueing delay shows up in these numbers.</p>
{{else}}
<p class="note">(1) time counted from the instant the request should have gone out — it includes any delay, and for that reason it does not hide a freeze in the target.</p>
{{if .HasServiceLatency}}
<p class="note">(2) plain response time, counted from when the previous step finished. That step depends on a value captured before it, so it has no scheduled instant of its own. For the honest reading of the journey, use the "The whole journey" block.</p>
{{end}}
{{end}}

{{if .HasChart}}
<h2>Over time</h2>
{{.Chart}}
<div class="legend">
  <span><span class="sample" style="background: var(--neutral)"></span>half the responses</span>
  <span><span class="sample" style="background: var(--warning)"></span>99% of the responses</span>
  <span><span class="sample" style="background: var(--failed)"></span>second with errors</span>
</div>
{{end}}

{{if or .Verdict.Evaluations .Verdict.Undeclared}}
<h2>SLO</h2>
<ul class="sentences slo">
  {{range .Verdict.Evaluations}}
  <li>{{if .Untrustworthy}}<span class="none">no verdict</span>{{else if .Passed}}<span class="ok">ok</span>{{else}}<span class="no">fail</span>{{end}}<span>{{.Sentence}}</span></li>
  {{end}}
  {{range .Verdict.Undeclared}}
  <li><span class="none">not declared</span><span>{{.}}</span></li>
  {{end}}
</ul>
{{end}}

{{if .ConsumerLag}}
<h2>Consumer lag</h2>
<table>
  <tr><th>group</th><th>topic</th><th>lag</th><th>samples</th></tr>
  {{range .ConsumerLag}}<tr><td>{{.Group}}</td><td>{{.Topic}}</td><td>{{.Headline}}</td><td>{{.Readings}}</td></tr>{{end}}
</table>
<ul class="sentences">
  {{range .ConsumerLag}}{{if .Note}}<li>{{.Note}}</li>{{end}}{{end}}
</ul>
{{end}}

{{if .Errors}}
<h2>Errors</h2>
<table>
  <tr><th>step</th><th>what happened</th><th>count</th><th>example</th></tr>
  {{range .Errors}}<tr><td>{{.Step}}</td><td>{{.Class}}</td><td>{{.Count}}</td><td>{{.Example}}</td></tr>{{end}}
</table>
{{end}}

<h2>How trustworthy the measurement is</h2>
<ul class="sentences">
  {{range .Reliability}}<li>{{.}}</li>{{end}}
</ul>

<h2>How this number was produced</h2>
<ul class="sentences">
  <li>Arrival model {{.Document.Run.Model}}: the load does not wait for the target to answer, so a freeze in the target lands in the count.</li>
  {{range .Plan}}<li>Plan: {{.}}</li>{{end}}
  {{range .Environment}}<li>{{.}}</li>{{end}}
</ul>

<footer>
  braunrate {{.Document.Version}} — run of {{.GeneratedAt}}, result format {{.Document.FormatVersion}}.
  This file opens with no network: it fetches no script, font or external image.
</footer>
</main>
</body>
</html>
`))

func mixLines(document metrics.Document) []htmlMixLine {
	total := int64(0)
	declared := false
	for _, step := range document.Steps {
		total += step.Count
		if step.DeclaredShare > 0 {
			declared = true
		}
	}
	if !declared || total == 0 {
		return nil
	}
	var lines []htmlMixLine
	for _, step := range document.Steps {
		if step.DeclaredShare <= 0 {
			continue
		}
		lines = append(lines, htmlMixLine{
			Name:     step.Name,
			Declared: percentage(step.DeclaredShare * 100),
			Observed: percentage(float64(step.Count) / float64(total) * 100),
			Count:    thousands(step.Count),
			Total:    thousands(total),
		})
	}
	return lines
}
