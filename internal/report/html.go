package report

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
)

type htmlPage struct {
	Document          metrics.Document
	Title             string
	Verdict           htmlVerdict
	Journey           metrics.Journey
	Steps             []htmlStep
	HasServiceLatency bool
	Warnings          []htmlWarning
	Errors            []htmlError
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
}

type htmlError struct {
	Class string
	Count string
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

	page.Verdict = buildVerdict(document)
	page.Count = thousands(document.Overall.Count)
	page.Rate = fmt.Sprintf("%.0f", document.Overall.EffectiveRate)
	page.ErrorRate = percentage(document.Overall.ErrorRate * 100)
	page.P95 = milliseconds(document.Overall.Latency.P95)
	page.JourneyP50 = milliseconds(document.Journey.Latency.P50)
	page.JourneyP95 = milliseconds(document.Journey.Latency.P95)
	page.JourneyP99 = milliseconds(document.Journey.Latency.P99)
	page.JourneyMax = milliseconds(document.Journey.Latency.Max)

	for _, step := range document.Steps {
		line := htmlStep{
			Name:     step.Name,
			Mark:     "1",
			Count:    thousands(step.Count),
			P50:      milliseconds(step.Latency.P50),
			P95:      milliseconds(step.Latency.P95),
			P99:      milliseconds(step.Latency.P99),
			P999:     milliseconds(step.Latency.P999),
			Max:      milliseconds(step.Latency.Max),
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

	for _, finding := range document.Sanity.Findings {
		page.Warnings = append(page.Warnings, htmlWarning{
			Class: "alta", Label: "resultado invalido",
			Message: finding.Message, Evidence: finding.Evidence,
		})
	}
	for _, warning := range document.Warnings {
		line := htmlWarning{Class: "baixa", Label: "observacao", Message: warning.Message, Evidence: warning.Evidence}
		switch warning.Severity {
		case metrics.SeverityHigh:
			// Already listed above, as a sanity finding.
			if document.Sanity.Checked {
				continue
			}
			line.Class, line.Label = "alta", "resultado invalido"
		case metrics.SeverityMedium:
			line.Class, line.Label = "media", "atencao"
		}
		page.Warnings = append(page.Warnings, line)
	}

	for _, line := range errorsByClass(document) {
		page.Errors = append(page.Errors, htmlError{Class: line.class, Count: thousands(line.count)})
	}
	page.Reliability = reliabilitySentences(document)
	page.Plan = planSentences(document)
	page.Environment = environmentSentences(document)

	if desenho, hasData := drawSeries(document.Series); hasData {
		page.Chart = desenho
		page.HasChart = true
	}
	return page
}

func buildVerdict(document metrics.Document) htmlVerdict {
	verdict := htmlVerdict{Evaluations: document.SLO.Evaluations}

	if !document.Valid() {
		verdict.Class = "invalido"
		verdict.Sentence = "Resultado invalido: a execucao nao mediu o que se propos a medir."
		verdict.Subtitle = "Isto nao e veredito sobre o alvo — e a medicao que nao vale, e por isso nenhuma regra de SLO foi avaliada."
		if !document.Sanity.Checked {
			verdict.Sentence = "Resultado invalido: o gerador nao sustentou a carga declarada."
			verdict.Subtitle = "Os numeros abaixo medem o gerador, nao o alvo. Rode de novo com taxa menor ou em uma maquina maior antes de tirar qualquer conclusao."
		}
		return verdict
	}

	switch {
	case len(document.SLO.Evaluations) == 0 && document.SLO.Sentence == "":
		verdict.Class = "neutro"
		verdict.Sentence = fmt.Sprintf("%s respondeu %s requisicoes com %s de erro.",
			document.Run.Target, thousands(document.Overall.Count), percentage(document.Overall.ErrorRate*100))
		verdict.Subtitle = "Nenhum slo declarado: esta execucao descreve, mas nao aprova nem reprova. Declare um bloco 'slo' para virar gate de CI."
	case document.SLO.Passed:
		verdict.Class = "passou"
		verdict.Sentence = document.SLO.Sentence
		verdict.Subtitle = phraseVolume(document)
	default:
		verdict.Class = "falhou"
		verdict.Sentence = document.SLO.Sentence
		verdict.Subtitle = phraseVolume(document)
	}
	return verdict
}

func phraseVolume(document metrics.Document) string {
	duration := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(time.Second)
	if document.Journey.Started > 0 {
		return fmt.Sprintf("%s jornadas em %s, %s requisicoes a %.0f por segundo, %s de erro.",
			thousands(document.Journey.Started), duration, thousands(document.Overall.Count),
			document.Overall.EffectiveRate, percentage(document.Overall.ErrorRate*100))
	}
	return fmt.Sprintf("%s requisicoes em %s, %.0f por segundo, %s de erro.",
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
	if scheduling.LateDispatches == 0 && scheduling.DroppedByInflightLimit == 0 {
		sentences = append(sentences, "O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.")
	}
	sentences = append(sentences, fmt.Sprintf("Atraso tipico para disparar: %s; pior caso: %s. O tempo de resposta ja desconta esse atraso.",
		milliseconds(scheduling.Skew.P50), milliseconds(scheduling.Skew.Max)))

	hidden := document.Overall.Latency.P99 - document.Overall.ServiceLatency.P99
	if hidden >= 1 {
		sentences = append(sentences, fmt.Sprintf("Uma ferramenta de laco fechado teria reportado %s a menos no 99%%: e a parte do atraso que so aparece contando do instante agendado.",
			milliseconds(hidden)))
	}
	if scheduling.PeakInflight > 0 {
		sentences = append(sentences, fmt.Sprintf("Pico de %s requisicoes ao mesmo tempo, com limite de %s.",
			thousands(scheduling.PeakInflight), thousands(document.Run.MaxInflight)))
	}
	return sentences
}

func planSentences(document metrics.Document) []string {
	var sentences []string
	for _, phase := range document.Run.AppliedPlan {
		duration := (time.Duration(phase.DurationMs) * time.Millisecond).Round(time.Second)
		if phase.Kind == "rampa" {
			sentences = append(sentences, fmt.Sprintf("rampa de %.0f/s ate %.0f/s durante %s", phase.From, phase.To, duration))
			continue
		}
		sentences = append(sentences, fmt.Sprintf("%s de %.0f/s durante %s", phase.Kind, phase.To, duration))
	}
	return sentences
}

func environmentSentences(document metrics.Document) []string {
	sentences := []string{
		fmt.Sprintf("%s, %s/%s, %d nucleos", document.Environment.Host, document.Environment.OS,
			document.Environment.Arch, document.Environment.Cores),
		fmt.Sprintf("braunrate %s (%s), gerador e alvo medidos como declarado acima", document.Version, document.Environment.GoVersion),
	}
	for _, variety := range document.Variety {
		sentences = append(sentences, "Variedade observada: "+variety.Sentence+".")
	}
	if len(document.Run.Seeds) > 0 {
		sentences = append(sentences, "Semente das fontes sinteticas: "+seeds(document.Run.Seeds)+" — a mesma semente gera os mesmos valores de novo.")
	}
	if document.Run.AuthObtains > 0 {
		sentences = append(sentences, fmt.Sprintf("Autenticacao obtida %d vez(es) e reaproveitada por todas as jornadas. Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.",
			document.Run.AuthObtains))
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
	fmt.Fprintf(&svg, `<svg viewBox="0 0 %d %d" role="img" aria-label="latencia por segundo">`, chartWidth, chartHeight)

	for _, fraction := range []float64{0, 0.5, 1} {
		value := maxLatency * (1 - fraction)
		y := marginTop + height*fraction
		fmt.Fprintf(&svg, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" class="grade"/>`,
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
		fmt.Fprintf(&svg, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%.1f" class="erro"/>`,
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
		fmt.Fprintf(&svg, `<text x="%.1f" y="%d" class="eixo" text-anchor="middle">%ds</text>`,
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

var htmlTemplate = template.Must(template.New("relatorio").Parse(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — braunrate</title>
<style>
:root {
  --fundo: #ffffff; --texto: #14181f; --suave: #5b6472; --borda: #e2e6ec;
  --passou: #0f7a3d; --falhou: #b3261e; --atencao: #8a5a00; --neutro: #2a5c9a;
  --fundo-cartao: #f7f9fb;
}
@media (prefers-color-scheme: dark) {
  :root { --fundo: #0f1319; --texto: #e8ecf2; --suave: #98a2b3; --borda: #232a35;
          --passou: #4ad07f; --falhou: #ff6b5e; --atencao: #f0b429; --neutro: #6aa6ff;
          --fundo-cartao: #161b23; }
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--fundo); color: var(--texto);
  font: 16px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
main { max-width: 960px; margin: 0 auto; padding: 40px 24px 72px; }
header { border-bottom: 1px solid var(--borda); padding-bottom: 20px; margin-bottom: 28px; }
.cenario { font-size: 14px; color: var(--suave); text-transform: uppercase; letter-spacing: .08em; }
h1 { font-size: 27px; line-height: 1.3; margin: 12px 0 8px; font-weight: 650; }
h1.passou { color: var(--passou); }
h1.falhou, h1.invalido { color: var(--falhou); }
h1.neutro { color: var(--neutro); }
.subtitulo { color: var(--suave); font-size: 16px; margin: 0; }
h2 { font-size: 15px; text-transform: uppercase; letter-spacing: .07em; color: var(--suave);
  margin: 36px 0 12px; font-weight: 600; }
table { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; }
th, td { text-align: right; padding: 9px 10px; border-bottom: 1px solid var(--borda); font-size: 15px; }
th:first-child, td:first-child { text-align: left; }
th { font-size: 13px; color: var(--suave); font-weight: 600; }
td.erro { color: var(--falhou); font-weight: 600; }
.marca { display: inline-block; min-width: 18px; font-size: 12px; color: var(--suave); }
.numeros { display: flex; flex-wrap: wrap; gap: 12px; margin: 0; padding: 0; list-style: none; }
.numeros li { flex: 1 1 150px; background: var(--fundo-cartao); border: 1px solid var(--borda);
  border-radius: 10px; padding: 14px 16px; }
.numeros .valor { font-size: 23px; font-weight: 620; font-variant-numeric: tabular-nums; }
.numeros .rotulo { font-size: 13px; color: var(--suave); }
.leitura { background: var(--fundo-cartao); border: 1px solid var(--borda); border-left: 3px solid var(--neutro);
  border-radius: 8px; padding: 14px 16px; margin: 14px 0; }
.nota { color: var(--suave); font-size: 14px; margin: 10px 0 0; }
ul.frases { list-style: none; padding: 0; margin: 0; }
ul.frases li { padding: 7px 0; border-bottom: 1px solid var(--borda); font-size: 15px; }
ul.frases li:last-child { border-bottom: none; }
.aviso { border-radius: 8px; padding: 13px 16px; margin: 10px 0; border: 1px solid var(--borda); }
.aviso .rotulo { font-size: 12px; text-transform: uppercase; letter-spacing: .08em; font-weight: 700; }
.aviso.alta { border-color: var(--falhou); } .aviso.alta .rotulo { color: var(--falhou); }
.aviso.media { border-color: var(--atencao); } .aviso.media .rotulo { color: var(--atencao); }
.aviso .evidencia { color: var(--suave); font-size: 14px; }
.slo li { display: flex; gap: 10px; align-items: baseline; }
.slo .ok { color: var(--passou); font-weight: 700; }
.slo .nao { color: var(--falhou); font-weight: 700; }
svg { width: 100%; height: auto; }
svg .grade { stroke: var(--borda); stroke-width: 1; }
svg .eixo { fill: var(--suave); font-size: 12px; }
svg .p50 { fill: none; stroke: var(--neutro); stroke-width: 2; }
svg .p99 { fill: none; stroke: var(--atencao); stroke-width: 2; }
svg .erro { stroke: var(--falhou); stroke-width: 1; opacity: .35; }
.legenda { display: flex; gap: 18px; font-size: 13px; color: var(--suave); margin-top: 6px; }
.legenda .amostra { display: inline-block; width: 14px; height: 3px; vertical-align: middle; margin-right: 6px; }
footer { margin-top: 44px; padding-top: 18px; border-top: 1px solid var(--borda);
  color: var(--suave); font-size: 13px; }
</style>
</head>
<body>
<main>
<header>
  <div class="cenario">{{.Title}} — {{.Document.Run.Target}}</div>
  <h1 class="{{.Verdict.Class}}">{{.Verdict.Sentence}}</h1>
  {{if .Verdict.Subtitle}}<p class="subtitulo">{{.Verdict.Subtitle}}</p>{{end}}
</header>

{{range .Warnings}}
<div class="aviso {{.Class}}">
  <div class="rotulo">{{.Label}}</div>
  <div>{{.Message}}</div>
  <div class="evidencia">{{.Evidence}}</div>
</div>
{{end}}

<h2>O que aconteceu</h2>
<ul class="numeros">
  <li><div class="valor">{{.Count}}</div><div class="rotulo">requisicoes</div></li>
  <li><div class="valor">{{.Rate}}</div><div class="rotulo">por segundo</div></li>
  <li><div class="valor">{{.ErrorRate}}</div><div class="rotulo">de erro</div></li>
  <li><div class="valor">{{.P95}}</div><div class="rotulo">95% das respostas ate</div></li>
</ul>

{{if .Journey.Started}}
<h2>A jornada inteira</h2>
<div class="leitura">{{.Journey.Sentence}}</div>
<table>
  <tr><th>jornada</th><th>metade</th><th>95%</th><th>99%</th><th>pior</th></tr>
  <tr>
    <td>do instante agendado ao ultimo passo</td>
    <td>{{.JourneyP50}}</td><td>{{.JourneyP95}}</td>
    <td>{{.JourneyP99}}</td><td>{{.JourneyMax}}</td>
  </tr>
</table>
{{end}}

<h2>Por passo</h2>
<table>
  <tr><th>passo</th><th>requisicoes</th><th>metade</th><th>95%</th><th>99%</th><th>99,9%</th><th>pior</th><th>erros</th></tr>
  {{range .Steps}}
  <tr>
    <td><span class="marca">({{.Mark}})</span> {{.Name}}</td>
    <td>{{.Count}}</td><td>{{.P50}}</td><td>{{.P95}}</td><td>{{.P99}}</td>
    <td>{{.P999}}</td><td>{{.Max}}</td>
    <td{{if .HasError}} class="erro"{{end}}>{{.Errors}}</td>
  </tr>
  {{end}}
</table>
<p class="nota">(1) tempo contado do instante em que a requisicao deveria ter partido — inclui qualquer atraso e por isso nao esconde travada do alvo.</p>
{{if .HasServiceLatency}}
<p class="nota">(2) tempo de resposta puro, contado de quando o passo anterior terminou. Esse passo depende de um valor capturado antes dele, entao nao tem instante agendado proprio. Para a leitura honesta da jornada, use o bloco "A jornada inteira".</p>
{{end}}

{{if .HasChart}}
<h2>Ao longo do tempo</h2>
{{.Chart}}
<div class="legenda">
  <span><span class="amostra" style="background: var(--neutro)"></span>metade das respostas</span>
  <span><span class="amostra" style="background: var(--atencao)"></span>99% das respostas</span>
  <span><span class="amostra" style="background: var(--falhou)"></span>segundo com erro</span>
</div>
{{end}}

{{if .Verdict.Evaluations}}
<h2>SLO</h2>
<ul class="frases slo">
  {{range .Verdict.Evaluations}}
  <li>{{if .Passed}}<span class="ok">ok</span>{{else}}<span class="nao">falha</span>{{end}}<span>{{.Sentence}}</span></li>
  {{end}}
</ul>
{{end}}

{{if .Errors}}
<h2>Erros</h2>
<table>
  <tr><th>tipo</th><th>quantidade</th></tr>
  {{range .Errors}}<tr><td>{{.Class}}</td><td>{{.Count}}</td></tr>{{end}}
</table>
{{end}}

<h2>Confiabilidade da medicao</h2>
<ul class="frases">
  {{range .Reliability}}<li>{{.}}</li>{{end}}
</ul>

<h2>Como este numero foi produzido</h2>
<ul class="frases">
  <li>Modelo de chegada {{.Document.Run.Model}}: a carga nao espera o alvo responder, entao travada do alvo aparece na conta.</li>
  {{range .Plan}}<li>Plano: {{.}}</li>{{end}}
  {{range .Environment}}<li>{{.}}</li>{{end}}
</ul>

<footer>
  braunrate {{.Document.Version}} — execucao de {{.GeneratedAt}}, formato de resultado {{.Document.FormatVersion}}.
  Este arquivo abre sem rede: nao busca script, fonte nem imagem externa.
</footer>
</main>
</body>
</html>
`))
