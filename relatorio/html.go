package relatorio

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/metrica"
)

type paginaHTML struct {
	Documento      metrica.Documento
	Titulo         string
	Veredito       vereditoHTML
	Jornada        metrica.Jornada
	Passos         []passoHTML
	TemDeServico   bool
	Avisos         []avisoHTML
	Erros          []erroHTML
	Contagem       string
	Taxa           string
	TaxaDeErro     string
	P95            string
	JornadaP50     string
	JornadaP95     string
	JornadaP99     string
	JornadaMaximo  string
	Grafico        template.HTML
	TemGrafico     bool
	Plano          []string
	Ambiente       []string
	Confiabilidade []string
	Geracao        string
}

type vereditoHTML struct {
	Frase      string
	Classe     string
	Subtitulo  string
	Avaliacoes []metrica.Avaliacao
}

type passoHTML struct {
	Nome      string
	Marca     string
	DeServico bool
	Contagem  string
	P50       string
	P95       string
	P99       string
	P999      string
	Maximo    string
	Erros     int64
	TemErro   bool
}

type erroHTML struct {
	Classe     string
	Quantidade string
}

type avisoHTML struct {
	Classe    string
	Rotulo    string
	Mensagem  string
	Evidencia string
}

// O topo do relatorio e uma frase, e nao uma tabela: quem abre o arquivo
// precisa saber se passou antes de saber quantos milissegundos deu.
func HTML(saida io.Writer, documento metrica.Documento) error {
	pagina := montarPagina(documento)
	return modeloHTML.Execute(saida, pagina)
}

func montarPagina(documento metrica.Documento) paginaHTML {
	pagina := paginaHTML{
		Documento: documento,
		Titulo:    documento.Execucao.Cenario,
		Jornada:   documento.Jornada,
		Geracao:   documento.Execucao.Inicio.Format("02/01/2006 15:04:05"),
	}

	pagina.Veredito = montarVeredito(documento)
	pagina.Contagem = milhar(documento.Global.Contagem)
	pagina.Taxa = fmt.Sprintf("%.0f", documento.Global.TaxaEfetiva)
	pagina.TaxaDeErro = porcentagem(documento.Global.TaxaDeErro * 100)
	pagina.P95 = milissegundos(documento.Global.Latencia.P95)
	pagina.JornadaP50 = milissegundos(documento.Jornada.Latencia.P50)
	pagina.JornadaP95 = milissegundos(documento.Jornada.Latencia.P95)
	pagina.JornadaP99 = milissegundos(documento.Jornada.Latencia.P99)
	pagina.JornadaMaximo = milissegundos(documento.Jornada.Latencia.Maximo)

	for _, passo := range documento.Passos {
		linha := passoHTML{
			Nome:     passo.Nome,
			Marca:    "1",
			Contagem: milhar(passo.Contagem),
			P50:      milissegundos(passo.Latencia.P50),
			P95:      milissegundos(passo.Latencia.P95),
			P99:      milissegundos(passo.Latencia.P99),
			P999:     milissegundos(passo.Latencia.P999),
			Maximo:   milissegundos(passo.Latencia.Maximo),
			Erros:    passo.Erros,
			TemErro:  passo.Erros > 0,
		}
		if passo.TipoDeLatencia == string(metrica.LatenciaDeServico) {
			linha.Marca = "2"
			linha.DeServico = true
			pagina.TemDeServico = true
		}
		pagina.Passos = append(pagina.Passos, linha)
	}

	for _, aviso := range documento.Avisos {
		linha := avisoHTML{Classe: "baixa", Rotulo: "observacao", Mensagem: aviso.Mensagem, Evidencia: aviso.Evidencia}
		switch aviso.Gravidade {
		case metrica.GravidadeAlta:
			linha.Classe, linha.Rotulo = "alta", "resultado invalido"
		case metrica.GravidadeMedia:
			linha.Classe, linha.Rotulo = "media", "atencao"
		}
		pagina.Avisos = append(pagina.Avisos, linha)
	}

	for _, linha := range errosPorClasse(documento) {
		pagina.Erros = append(pagina.Erros, erroHTML{Classe: linha.classe, Quantidade: milhar(linha.quantidade)})
	}
	pagina.Confiabilidade = frasesDeConfiabilidade(documento)
	pagina.Plano = frasesDoPlano(documento)
	pagina.Ambiente = frasesDoAmbiente(documento)

	if desenho, temDados := desenharSeries(documento.Series); temDados {
		pagina.Grafico = desenho
		pagina.TemGrafico = true
	}
	return pagina
}

func montarVeredito(documento metrica.Documento) vereditoHTML {
	veredito := vereditoHTML{Avaliacoes: documento.SLO.Avaliacoes}

	if !documento.ResultadoValido() {
		veredito.Classe = "invalido"
		veredito.Frase = "Resultado invalido: o gerador nao sustentou a carga declarada."
		veredito.Subtitulo = "Os numeros abaixo medem o gerador, nao o alvo. Rode de novo com taxa menor ou em uma maquina maior antes de tirar qualquer conclusao."
		return veredito
	}

	switch {
	case len(documento.SLO.Avaliacoes) == 0 && documento.SLO.Frase == "":
		veredito.Classe = "neutro"
		veredito.Frase = fmt.Sprintf("%s respondeu %s requisicoes com %s de erro.",
			documento.Execucao.Alvo, milhar(documento.Global.Contagem), porcentagem(documento.Global.TaxaDeErro*100))
		veredito.Subtitulo = "Nenhum slo declarado: esta execucao descreve, mas nao aprova nem reprova. Declare um bloco 'slo' para virar gate de CI."
	case documento.SLO.Passou:
		veredito.Classe = "passou"
		veredito.Frase = documento.SLO.Frase
		veredito.Subtitulo = frasearVolume(documento)
	default:
		veredito.Classe = "falhou"
		veredito.Frase = documento.SLO.Frase
		veredito.Subtitulo = frasearVolume(documento)
	}
	return veredito
}

func frasearVolume(documento metrica.Documento) string {
	duracao := (time.Duration(documento.Execucao.DuracaoMs) * time.Millisecond).Round(time.Second)
	if documento.Jornada.Iniciadas > 0 {
		return fmt.Sprintf("%s jornadas em %s, %s requisicoes a %.0f por segundo, %s de erro.",
			milhar(documento.Jornada.Iniciadas), duracao, milhar(documento.Global.Contagem),
			documento.Global.TaxaEfetiva, porcentagem(documento.Global.TaxaDeErro*100))
	}
	return fmt.Sprintf("%s requisicoes em %s, %.0f por segundo, %s de erro.",
		milhar(documento.Global.Contagem), duracao, documento.Global.TaxaEfetiva,
		porcentagem(documento.Global.TaxaDeErro*100))
}

// O eixo mostra pouca casa de propósito: numero de eixo e referencia de leitura,
// e o valor exato ja esta na tabela acima.
func rotuloDeEixo(valor float64) string {
	switch {
	case valor == 0:
		return "0"
	case valor >= 100:
		return fmt.Sprintf("%.0f ms", valor)
	case valor >= 10:
		return fmt.Sprintf("%.0f ms", valor)
	default:
		return fmt.Sprintf("%.1f ms", valor)
	}
}

func frasesDeConfiabilidade(documento metrica.Documento) []string {
	var frases []string
	agendamento := documento.Agendamento
	if agendamento.DespachosAtrasados == 0 && agendamento.DescartadasPorLimiteDeVoo == 0 {
		frases = append(frases, "O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.")
	}
	frases = append(frases, fmt.Sprintf("Atraso tipico para disparar: %s; pior caso: %s. O tempo de resposta ja desconta esse atraso.",
		milissegundos(agendamento.Desvio.P50), milissegundos(agendamento.Desvio.Maximo)))

	escondido := documento.Global.Latencia.P99 - documento.Global.LatenciaDeServico.P99
	if escondido >= 1 {
		frases = append(frases, fmt.Sprintf("Uma ferramenta de laco fechado teria reportado %s a menos no 99%%: e a parte do atraso que so aparece contando do instante agendado.",
			milissegundos(escondido)))
	}
	if agendamento.PicoEmVoo > 0 {
		frases = append(frases, fmt.Sprintf("Pico de %s requisicoes ao mesmo tempo, com limite de %s.",
			milhar(agendamento.PicoEmVoo), milhar(documento.Execucao.MaximoSimultaneas)))
	}
	return frases
}

func frasesDoPlano(documento metrica.Documento) []string {
	var frases []string
	for _, fase := range documento.Execucao.PlanoAplicado {
		duracao := (time.Duration(fase.DuracaoMs) * time.Millisecond).Round(time.Second)
		if fase.Tipo == "rampa" {
			frases = append(frases, fmt.Sprintf("rampa de %.0f/s ate %.0f/s durante %s", fase.De, fase.Ate, duracao))
			continue
		}
		frases = append(frases, fmt.Sprintf("%s de %.0f/s durante %s", fase.Tipo, fase.Ate, duracao))
	}
	return frases
}

func frasesDoAmbiente(documento metrica.Documento) []string {
	frases := []string{
		fmt.Sprintf("%s, %s/%s, %d nucleos", documento.Ambiente.Maquina, documento.Ambiente.SistemaOperacional,
			documento.Ambiente.Arquitetura, documento.Ambiente.Nucleos),
		fmt.Sprintf("braunrate %s (%s), gerador e alvo medidos como declarado acima", documento.Versao, documento.Ambiente.VersaoDoGo),
	}
	for _, variedade := range documento.Variedade {
		frases = append(frases, "Variedade observada: "+variedade.Frase+".")
	}
	if len(documento.Execucao.Sementes) > 0 {
		frases = append(frases, "Semente das fontes sinteticas: "+sementes(documento.Execucao.Sementes)+" — a mesma semente gera os mesmos valores de novo.")
	}
	if documento.Execucao.Autenticacoes > 0 {
		frases = append(frases, fmt.Sprintf("Autenticacao obtida %d vez(es) e reaproveitada por todas as jornadas. Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.",
			documento.Execucao.Autenticacoes))
	}
	return frases
}

const (
	larguraDoGrafico = 900
	alturaDoGrafico  = 260
	margemEsquerda   = 56
	margemDireita    = 16
	margemSuperior   = 16
	margemInferior   = 34
)

// O grafico e SVG escrito na mao porque o relatorio precisa abrir sem rede:
// biblioteca de grafico viria de CDN e o arquivo deixaria de ser autocontido.
func desenharSeries(series []metrica.Bucket) (template.HTML, bool) {
	if len(series) < 2 {
		return "", false
	}
	ordenadas := append([]metrica.Bucket{}, series...)
	sort.Slice(ordenadas, func(i, j int) bool { return ordenadas[i].InicioEpochMs < ordenadas[j].InicioEpochMs })

	maiorLatencia := 0.0
	for _, bucket := range ordenadas {
		if bucket.LatenciaP99Ms > maiorLatencia {
			maiorLatencia = bucket.LatenciaP99Ms
		}
	}
	if maiorLatencia <= 0 {
		return "", false
	}

	largura := float64(larguraDoGrafico - margemEsquerda - margemDireita)
	altura := float64(alturaDoGrafico - margemSuperior - margemInferior)
	passo := largura / float64(len(ordenadas)-1)

	posicaoY := func(valor float64) float64 {
		return margemSuperior + altura - (valor/maiorLatencia)*altura
	}

	linha := func(escolher func(metrica.Bucket) float64) string {
		var pontos []string
		for indice, bucket := range ordenadas {
			x := float64(margemEsquerda) + float64(indice)*passo
			pontos = append(pontos, fmt.Sprintf("%.1f,%.1f", x, posicaoY(escolher(bucket))))
		}
		return strings.Join(pontos, " ")
	}

	var svg strings.Builder
	fmt.Fprintf(&svg, `<svg viewBox="0 0 %d %d" role="img" aria-label="latencia por segundo">`, larguraDoGrafico, alturaDoGrafico)

	for _, fracao := range []float64{0, 0.5, 1} {
		valor := maiorLatencia * (1 - fracao)
		y := margemSuperior + altura*fracao
		fmt.Fprintf(&svg, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" class="grade"/>`,
			margemEsquerda, y, larguraDoGrafico-margemDireita, y)
		fmt.Fprintf(&svg, `<text x="%d" y="%.1f" class="eixo" text-anchor="end">%s</text>`,
			margemEsquerda-8, y+4, template.HTMLEscapeString(rotuloDeEixo(valor)))
	}

	for _, bucket := range ordenadas {
		if bucket.Erros == 0 {
			continue
		}
		indice := indiceDe(ordenadas, bucket.InicioEpochMs)
		x := float64(margemEsquerda) + float64(indice)*passo
		fmt.Fprintf(&svg, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%.1f" class="erro"/>`,
			x, margemSuperior, x, margemSuperior+altura)
	}

	fmt.Fprintf(&svg, `<polyline class="p99" points="%s"/>`, linha(func(b metrica.Bucket) float64 { return b.LatenciaP99Ms }))
	fmt.Fprintf(&svg, `<polyline class="p50" points="%s"/>`, linha(func(b metrica.Bucket) float64 { return b.LatenciaP50Ms }))

	primeiro := ordenadas[0].InicioEpochMs
	for indice, bucket := range ordenadas {
		if indice%maximo(1, len(ordenadas)/8) != 0 {
			continue
		}
		x := float64(margemEsquerda) + float64(indice)*passo
		segundos := (bucket.InicioEpochMs - primeiro) / 1000
		fmt.Fprintf(&svg, `<text x="%.1f" y="%d" class="eixo" text-anchor="middle">%ds</text>`,
			x, alturaDoGrafico-12, segundos)
	}

	svg.WriteString(`</svg>`)
	return template.HTML(svg.String()), true
}

func indiceDe(buckets []metrica.Bucket, epoch int64) int {
	for indice, bucket := range buckets {
		if bucket.InicioEpochMs == epoch {
			return indice
		}
	}
	return 0
}

func maximo(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var modeloHTML = template.Must(template.New("relatorio").Parse(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Titulo}} — braunrate</title>
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
  <div class="cenario">{{.Titulo}} — {{.Documento.Execucao.Alvo}}</div>
  <h1 class="{{.Veredito.Classe}}">{{.Veredito.Frase}}</h1>
  {{if .Veredito.Subtitulo}}<p class="subtitulo">{{.Veredito.Subtitulo}}</p>{{end}}
</header>

{{range .Avisos}}
<div class="aviso {{.Classe}}">
  <div class="rotulo">{{.Rotulo}}</div>
  <div>{{.Mensagem}}</div>
  <div class="evidencia">{{.Evidencia}}</div>
</div>
{{end}}

<h2>O que aconteceu</h2>
<ul class="numeros">
  <li><div class="valor">{{.Contagem}}</div><div class="rotulo">requisicoes</div></li>
  <li><div class="valor">{{.Taxa}}</div><div class="rotulo">por segundo</div></li>
  <li><div class="valor">{{.TaxaDeErro}}</div><div class="rotulo">de erro</div></li>
  <li><div class="valor">{{.P95}}</div><div class="rotulo">95% das respostas ate</div></li>
</ul>

{{if .Jornada.Iniciadas}}
<h2>A jornada inteira</h2>
<div class="leitura">{{.Jornada.Frase}}</div>
<table>
  <tr><th>jornada</th><th>metade</th><th>95%</th><th>99%</th><th>pior</th></tr>
  <tr>
    <td>do instante agendado ao ultimo passo</td>
    <td>{{.JornadaP50}}</td><td>{{.JornadaP95}}</td>
    <td>{{.JornadaP99}}</td><td>{{.JornadaMaximo}}</td>
  </tr>
</table>
{{end}}

<h2>Por passo</h2>
<table>
  <tr><th>passo</th><th>requisicoes</th><th>metade</th><th>95%</th><th>99%</th><th>99,9%</th><th>pior</th><th>erros</th></tr>
  {{range .Passos}}
  <tr>
    <td><span class="marca">({{.Marca}})</span> {{.Nome}}</td>
    <td>{{.Contagem}}</td><td>{{.P50}}</td><td>{{.P95}}</td><td>{{.P99}}</td>
    <td>{{.P999}}</td><td>{{.Maximo}}</td>
    <td{{if .TemErro}} class="erro"{{end}}>{{.Erros}}</td>
  </tr>
  {{end}}
</table>
<p class="nota">(1) tempo contado do instante em que a requisicao deveria ter partido — inclui qualquer atraso e por isso nao esconde travada do alvo.</p>
{{if .TemDeServico}}
<p class="nota">(2) tempo de resposta puro, contado de quando o passo anterior terminou. Esse passo depende de um valor capturado antes dele, entao nao tem instante agendado proprio. Para a leitura honesta da jornada, use o bloco "A jornada inteira".</p>
{{end}}

{{if .TemGrafico}}
<h2>Ao longo do tempo</h2>
{{.Grafico}}
<div class="legenda">
  <span><span class="amostra" style="background: var(--neutro)"></span>metade das respostas</span>
  <span><span class="amostra" style="background: var(--atencao)"></span>99% das respostas</span>
  <span><span class="amostra" style="background: var(--falhou)"></span>segundo com erro</span>
</div>
{{end}}

{{if .Veredito.Avaliacoes}}
<h2>SLO</h2>
<ul class="frases slo">
  {{range .Veredito.Avaliacoes}}
  <li>{{if .Passou}}<span class="ok">ok</span>{{else}}<span class="nao">falha</span>{{end}}<span>{{.Frase}}</span></li>
  {{end}}
</ul>
{{end}}

{{if .Erros}}
<h2>Erros</h2>
<table>
  <tr><th>tipo</th><th>quantidade</th></tr>
  {{range .Erros}}<tr><td>{{.Classe}}</td><td>{{.Quantidade}}</td></tr>{{end}}
</table>
{{end}}

<h2>Confiabilidade da medicao</h2>
<ul class="frases">
  {{range .Confiabilidade}}<li>{{.}}</li>{{end}}
</ul>

<h2>Como este numero foi produzido</h2>
<ul class="frases">
  <li>Modelo de chegada {{.Documento.Execucao.Modelo}}: a carga nao espera o alvo responder, entao travada do alvo aparece na conta.</li>
  {{range .Plano}}<li>Plano: {{.}}</li>{{end}}
  {{range .Ambiente}}<li>{{.}}</li>{{end}}
</ul>

<footer>
  braunrate {{.Documento.Versao}} — execucao de {{.Geracao}}, formato de resultado {{.Documento.VersaoDoFormato}}.
  Este arquivo abre sem rede: nao busca script, fonte nem imagem externa.
</footer>
</main>
</body>
</html>
`))
