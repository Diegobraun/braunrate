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
		Noise: fmt.Sprintf("Duas execucoes nao dao intervalo de confianca: variacao abaixo de %.0f%% e tratada como ruido.",
			comparison.AcceptedNoise*100),
	}

	page.Class = "neutro"
	switch {
	case !c.Comparable:
		page.Class = "invalido"
		page.Subtitle = "Nenhum numero abaixo foi comparado: uma das duas execucoes nao vale como medicao."
	case c.Journey.Direction == comparison.DirectionWorse || c.Overall.Direction == comparison.DirectionWorse:
		page.Class = "falhou"
	case c.Journey.Direction == comparison.DirectionBetter || c.Overall.Direction == comparison.DirectionBetter:
		page.Class = "passou"
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
			line.Note = "passo novo"
		case step.Vanished:
			line.Note = "nao existe mais"
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
		Metric: difference.Metrica,
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
		return "pior"
	case comparison.DirectionBetter:
		return "melhor"
	}
	return "igual"
}

var comparisonTemplate = template.Must(template.Must(template.Must(
	template.New("comparison").Parse(pageStyle)).
	Parse(comparisonStyle)).
	Parse(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}: antes e depois — braunrate</title>
{{template "style"}}
{{template "comparison-style"}}
</head>
<body>
<main>
<header>
  <div class="cenario">{{.Title}} — antes e depois</div>
  <h1 class="{{.Class}}">{{.Sentence}}</h1>
  {{if .Subtitle}}<p class="subtitulo">{{.Subtitle}}</p>{{end}}
</header>

<h2>Comparando</h2>
<table>
  <tr><th>execucao</th><th>cenario</th><th>alvo</th><th>quando</th><th>versao</th></tr>
  <tr><td>antes</td><td>{{.Before.Spec}}</td><td>{{.Before.Target}}</td><td>{{.Before.Start}}</td><td>{{.Before.Version}}</td></tr>
  <tr><td>depois</td><td>{{.After.Spec}}</td><td>{{.After.Target}}</td><td>{{.After.Start}}</td><td>{{.After.Version}}</td></tr>
</table>

{{if .Comparable}}
<div class="leitura">{{.Journey}}</div>

<h2>A jornada inteira</h2>
<table>
  <tr><th>percentil</th><th>antes</th><th>depois</th><th>variacao</th></tr>
  {{range .JourneyRows}}<tr><td>{{.Metric}}</td><td>{{.Before}}</td><td>{{.After}}</td><td class="{{.Class}}">{{.Change}}</td></tr>{{end}}
</table>

<h2>Todas as requisicoes</h2>
<table>
  <tr><th>percentil</th><th>antes</th><th>depois</th><th>variacao</th></tr>
  {{range .OverallRows}}<tr><td>{{.Metric}}</td><td>{{.Before}}</td><td>{{.After}}</td><td class="{{.Class}}">{{.Change}}</td></tr>{{end}}
</table>

<h2>Por passo</h2>
<table>
  <tr><th>passo</th><th>95% antes</th><th>95% depois</th><th>variacao</th></tr>
  {{range .Steps}}<tr>
    <td>{{.Step}}{{if .Note}} <span class="marca">({{.Note}})</span>{{end}}</td>
    <td>{{.Before}}</td><td>{{.After}}</td><td class="{{.Class}}">{{.Change}}</td>
  </tr>{{end}}
</table>

<h2>Erros</h2>
<ul class="frases"><li>{{.Errors}}</li></ul>
{{end}}

<h2>O que pode explicar a diferenca sem ser o servico</h2>
<ul class="frases">
  {{if not .Caveats}}<li>Nada: mesmo cenario, mesmo alvo, mesma maquina, mesmo plano de carga e mesma versao.</li>{{end}}
  {{range .Caveats}}<li>{{.Text}}{{if .Blocking}} <strong>(isso sozinho explica a diferenca)</strong>{{end}}</li>{{end}}
  <li>{{.Noise}}</li>
</ul>

<footer>
  braunrate {{.Version}} — comparacao de duas execucoes ja gravadas.
  Este arquivo abre sem rede: nao busca script, fonte nem imagem externa.
</footer>
</main>
</body>
</html>
`))

const comparisonStyle = `{{define "comparison-style"}}
<style>
td.pior { color: var(--falhou); font-weight: 600; }
td.melhor { color: var(--passou); font-weight: 600; }
td.igual { color: var(--suave); }
</style>
{{end}}`
