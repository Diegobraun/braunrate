package report

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/texto"
)

type htmlPage struct {
	Document          metrics.Document
	Title             string
	Verdict           htmlVerdict
	Journey           metrics.Journey
	Steps             []htmlStep
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
			Readings: texto.Count(int64(lag.Readings), "leitura", "leituras"),
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
	case len(document.SLO.Evaluations) == 0:
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
	if document.Closed() {
		return []string{
			fmt.Sprintf("Nao ha agendamento para comparar: a taxa efetiva de %.0f/s foi consequencia do tempo de resposta do alvo, nao uma carga declarada.",
				document.Overall.EffectiveRate),
			"Se o alvo ficar mais lento, os usuarios virtuais pedem menos e a carga cai junto — o atraso nao aparece nos numeros.",
		}
	}
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
	for _, broker := range document.Run.Brokers {
		sentences = append(sentences, "Mensageria: "+broker+".")
	}
	for _, variety := range document.Variety {
		if !variety.Notable() {
			continue
		}
		sentences = append(sentences, "Variedade observada: "+variety.Sentence+".")
	}
	if len(document.Run.Seeds) > 0 {
		sentences = append(sentences, "Semente das fontes sinteticas: "+seeds(document.Run.Seeds)+" — a mesma semente gera os mesmos valores de novo.")
	}
	if document.Run.AuthObtains > 0 {
		sentences = append(sentences, fmt.Sprintf("Autenticacao obtida %s e reaproveitada por todas as jornadas. Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.",
			texto.Times(document.Run.AuthObtains)))
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

var htmlTemplate = template.Must(template.Must(template.New("relatorio").Parse(estiloDaPagina)).Parse(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — braunrate</title>
{{template "estilo"}}
</head>
<body>
<main>
<header>
  <div class="cenario">{{.Title}} — {{.Document.Run.Target}}</div>
  <h1 class="{{.Verdict.Class}}">{{.Verdict.Sentence}}</h1>
  {{if .Verdict.Subtitle}}<p class="subtitulo">{{.Verdict.Subtitle}}</p>{{end}}
</header>

{{if .ClosedLoop}}
<div class="aviso media">
  <div class="rotulo">laco fechado</div>
  <div>{{.ClosedLoop}}</div>
</div>
{{end}}

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
{{if not .Steps}}
<p class="nota">Nenhum passo registrou amostra: a execucao nao chegou a medir nada. Rode <code>braunrate debug</code> para ver onde a iteracao para.</p>
{{else}}
<table>
  <tr><th>passo</th><th>requisicoes</th><th>metade</th><th>95%</th><th>99%</th><th>99,9%</th><th>pior</th><th>erros</th></tr>
  {{range .Steps}}
  <tr>
    <td>{{if .NeverRan}}{{.Name}}{{else}}<span class="marca">({{.Mark}})</span> {{.Name}}{{end}}</td>
    <td>{{.Count}}</td><td>{{.P50}}</td><td>{{.P95}}</td><td>{{.P99}}</td>
    <td>{{.P999}}</td><td>{{.Max}}</td>
    <td{{if .HasError}} class="erro"{{end}}>{{if .NeverRan}}—{{else}}{{.Errors}}{{end}}</td>
  </tr>
  {{end}}
</table>
{{if .HasNeverRan}}
<p class="nota">Passo com traco nunca chegou a executar: a iteracao parou antes dele. O motivo esta em "Erros", no passo que falhou primeiro.</p>
{{end}}
{{end}}
{{if .ClosedLoop}}
<p class="nota">(2) tempo de resposta puro. No laco fechado nao existe instante agendado: o usuario virtual so pede de novo depois da resposta anterior, entao nenhum atraso de fila aparece nestes numeros.</p>
{{else}}
<p class="nota">(1) tempo contado do instante em que a requisicao deveria ter partido — inclui qualquer atraso e por isso nao esconde travada do alvo.</p>
{{if .HasServiceLatency}}
<p class="nota">(2) tempo de resposta puro, contado de quando o passo anterior terminou. Esse passo depende de um valor capturado antes dele, entao nao tem instante agendado proprio. Para a leitura honesta da jornada, use o bloco "A jornada inteira".</p>
{{end}}
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

{{if or .Verdict.Evaluations .Verdict.Undeclared}}
<h2>SLO</h2>
<ul class="frases slo">
  {{range .Verdict.Evaluations}}
  <li>{{if .Untrustworthy}}<span class="sem">sem veredito</span>{{else if .Passed}}<span class="ok">ok</span>{{else}}<span class="nao">falha</span>{{end}}<span>{{.Sentence}}</span></li>
  {{end}}
  {{range .Verdict.Undeclared}}
  <li><span class="sem">nao declarado</span><span>{{.}}</span></li>
  {{end}}
</ul>
{{end}}

{{if .ConsumerLag}}
<h2>Atraso do consumidor</h2>
<table>
  <tr><th>grupo</th><th>topico</th><th>atraso</th><th>amostras</th></tr>
  {{range .ConsumerLag}}<tr><td>{{.Group}}</td><td>{{.Topic}}</td><td>{{.Headline}}</td><td>{{.Readings}}</td></tr>{{end}}
</table>
<ul class="frases">
  {{range .ConsumerLag}}{{if .Note}}<li>{{.Note}}</li>{{end}}{{end}}
</ul>
{{end}}

{{if .Errors}}
<h2>Erros</h2>
<table>
  <tr><th>passo</th><th>o que aconteceu</th><th>quantidade</th><th>exemplo</th></tr>
  {{range .Errors}}<tr><td>{{.Step}}</td><td>{{.Class}}</td><td>{{.Count}}</td><td>{{.Example}}</td></tr>{{end}}
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
