package report

import (
	"fmt"
	"html/template"
	"io"

	"github.com/Diegobraun/braunrate/internal/report/comparison"
)

type comparisonPage struct {
	Title       string
	Sentence    string
	Class       string
	Subtitle    string
	Comparable  bool
	Before      comparison.Identification
	After       comparison.Identification
	Journey     string
	JourneyRows []comparisonLine
	OverallRows []comparisonLine
	Steps       []comparisonStep
	Errors      string
	Caveats     []comparisonCaveat
	Noise       string
	Version     string
}

type comparisonLine struct {
	Metric string
	Before string
	After  string
	Change string
	Class  string
}

type comparisonStep struct {
	Step   string
	Before string
	After  string
	Change string
	Class  string
	Note   string
}

type comparisonCaveat struct {
	Text     string
	Blocking bool
}

// ComparisonHTML writes the same comparison the terminal prints, as a file that
// can be attached to a pull request. It is built from the same Comparison value:
// a second calculation here would be a second answer to one question.
func ComparisonHTML(out io.Writer, c comparison.Comparison, version string) error {
	return comparisonTemplate.Execute(out, buildComparisonPage(c, version))
}

func buildComparisonPage(c comparison.Comparison, version string) comparisonPage {
	page := comparisonPage{
		Title:      c.After.Spec,
		Sentence:   c.Sentence,
		Comparable: c.Comparable,
		Before:     c.Before,
		After:      c.After,
		Errors:     c.Error.Sentence,
		Version:    version,
		Noise: fmt.Sprintf("Two runs give no confidence interval: a change below %.0f%% is treated as noise.",
			comparison.AcceptedNoise*100),
	}

	page.Class = "neutral"
	switch {
	case !c.Comparable:
		page.Class = "invalid"
		page.Subtitle = "None of the numbers below were compared: one of the two runs does not hold as a measurement."
	case c.Journey.Direction == comparison.DirectionWorse || c.Overall.Direction == comparison.DirectionWorse:
		page.Class = "failed"
	case c.Journey.Direction == comparison.DirectionBetter || c.Overall.Direction == comparison.DirectionBetter:
		page.Class = "passed"
	}

	page.Journey = c.Journey.Sentence
	page.JourneyRows = percentileLines(c.JourneyPercentiles)
	page.OverallRows = percentileLines(c.OverallPercentiles)

	for _, step := range c.Steps {
		line := comparisonStep{
			Step:   step.Step,
			Before: milliseconds(step.P95.Before),
			After:  milliseconds(step.P95.After),
			Change: change(step.P95),
			Class:  classOf(step.P95.Direction),
		}
		switch {
		case step.New:
			line.Note = "new step"
		case step.Vanished:
			line.Note = "gone"
		}
		page.Steps = append(page.Steps, line)
	}

	for _, caveat := range c.Caveats {
		page.Caveats = append(page.Caveats, comparisonCaveat{Text: caveat.Text, Blocking: caveat.Blocking})
	}
	return page
}

func comparisonLineOf(difference comparison.Difference) comparisonLine {
	return comparisonLine{
		Metric: difference.Metric,
		Before: milliseconds(difference.Before),
		After:  milliseconds(difference.After),
		Change: change(difference),
		Class:  classOf(difference.Direction),
	}
}

// Percentiles come out in the order a person reads them, not in the order of a
// Go map: a page that shuffles its rows at every run cannot be diffed against
// the one from yesterday.
var percentileOrder = []string{"p50", "p75", "p90", "p95", "p99", "p99.9", "max"}

func percentileLines(percentiles map[string]comparison.Difference) []comparisonLine {
	lines := make([]comparisonLine, 0, len(percentileOrder))
	for _, name := range percentileOrder {
		difference, has := percentiles[name]
		if !has {
			continue
		}
		line := comparisonLineOf(difference)
		line.Metric = name
		lines = append(lines, line)
	}
	return lines
}

func classOf(direction string) string {
	switch direction {
	case comparison.DirectionWorse:
		return "worse"
	case comparison.DirectionBetter:
		return "better"
	}
	return "same"
}

var comparisonTemplate = template.Must(template.Must(template.Must(
	template.New("comparison").Parse(pageStyle)).
	Parse(comparisonStyle)).
	Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}: before and after — braunrate</title>
{{template "style"}}
{{template "comparison-style"}}
</head>
<body>
<main>
<header>
  <div class="scenario">{{.Title}} — before and after</div>
  <h1 class="{{.Class}}">{{.Sentence}}</h1>
  {{if .Subtitle}}<p class="subtitle">{{.Subtitle}}</p>{{end}}
</header>

<h2>Comparing</h2>
<table>
  <tr><th>run</th><th>scenario</th><th>target</th><th>when</th><th>version</th></tr>
  <tr><td>before</td><td>{{.Before.Spec}}</td><td>{{.Before.Target}}</td><td>{{.Before.Start}}</td><td>{{.Before.Version}}</td></tr>
  <tr><td>after</td><td>{{.After.Spec}}</td><td>{{.After.Target}}</td><td>{{.After.Start}}</td><td>{{.After.Version}}</td></tr>
</table>

{{if .Comparable}}
<div class="reading">{{.Journey}}</div>

<h2>The whole journey</h2>
<table>
  <tr><th>percentile</th><th>before</th><th>after</th><th>change</th></tr>
  {{range .JourneyRows}}<tr><td>{{.Metric}}</td><td>{{.Before}}</td><td>{{.After}}</td><td class="{{.Class}}">{{.Change}}</td></tr>{{end}}
</table>

<h2>All requests</h2>
<table>
  <tr><th>percentile</th><th>before</th><th>after</th><th>change</th></tr>
  {{range .OverallRows}}<tr><td>{{.Metric}}</td><td>{{.Before}}</td><td>{{.After}}</td><td class="{{.Class}}">{{.Change}}</td></tr>{{end}}
</table>

<h2>Per step</h2>
<table>
  <tr><th>step</th><th>95% before</th><th>95% after</th><th>change</th></tr>
  {{range .Steps}}<tr>
    <td>{{.Step}}{{if .Note}} <span class="mark">({{.Note}})</span>{{end}}</td>
    <td>{{.Before}}</td><td>{{.After}}</td><td class="{{.Class}}">{{.Change}}</td>
  </tr>{{end}}
</table>

<h2>Errors</h2>
<ul class="sentences"><li>{{.Errors}}</li></ul>
{{end}}

<h2>What could explain the difference other than the service</h2>
<ul class="sentences">
  {{if not .Caveats}}<li>Nothing that can be compared differs: scenario, target, machine, load plan and version are the same. The contents of the data files are not on this list — if they changed between the two, the difference may be theirs.</li>{{end}}
  {{range .Caveats}}<li>{{.Text}}{{if .Blocking}} <strong>(this alone explains the difference)</strong>{{end}}</li>{{end}}
  <li>{{.Noise}}</li>
</ul>

<footer>
  braunrate {{.Version}} — comparison of two runs already recorded.
  This file opens with no network: it fetches no script, font or external image.
</footer>
</main>
</body>
</html>
`))

const comparisonStyle = `{{define "comparison-style"}}
<style>
td.worse { color: var(--failed); font-weight: 600; }
td.better { color: var(--passed); font-weight: 600; }
td.same { color: var(--soft); }
</style>
{{end}}`
