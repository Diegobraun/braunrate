package comparison

import (
	"fmt"
	"math"
	"sort"

	"github.com/Diegobraun/braunrate/internal/metrics"
)

// Duas execucoes nao produzem intervalo de confianca. Variacao menor que isto
// e tratada como ruido porque, com uma amostra de cada lado, nao da para
// afirmar que mudou: chamar 3% de regressao seria inventar precisao.
const RuidoAceito = 0.05

type Comparacao struct {
	Antes      Identificacao       `json:"antes"`
	Depois     Identificacao       `json:"depois"`
	Frase      string              `json:"frase"`
	Comparavel bool                `json:"comparavel"`
	Ressalvas  []string            `json:"ressalvas"`
	Jornada    Diferenca           `json:"jornada"`
	Global     Diferenca           `json:"global"`
	Passos     []DiferencaDePasso  `json:"passos"`
	Erro       DiferencaDeContagem `json:"taxa_de_erro"`
}

type Identificacao struct {
	Cenario string `json:"cenario"`
	Alvo    string `json:"alvo"`
	Inicio  string `json:"inicio"`
	Versao  string `json:"versao"`
}

type Diferenca struct {
	Metrica  string  `json:"metrica"`
	Antes    float64 `json:"antes_ms"`
	Depois   float64 `json:"depois_ms"`
	Variacao float64 `json:"variacao"`
	Sentido  string  `json:"sentido"`
	Frase    string  `json:"frase"`
}

type DiferencaDePasso struct {
	Passo string    `json:"passo"`
	P95   Diferenca `json:"p95"`
	P99   Diferenca `json:"p99"`
	Novo  bool      `json:"novo"`
	Sumiu bool      `json:"sumiu"`
}

type DiferencaDeContagem struct {
	Antes  float64 `json:"antes"`
	Depois float64 `json:"depois"`
	Frase  string  `json:"frase"`
}

const (
	SentidoPiorou = "piorou"
	SentidoMelhor = "melhorou"
	SentidoIgual  = "sem diferenca que valha leitura"
)

func Comparar(antes, depois metrics.Documento) Comparacao {
	c := Comparacao{
		Antes:      identificar(antes),
		Depois:     identificar(depois),
		Comparavel: true,
	}

	c.Ressalvas = levantarRessalvas(antes, depois)
	if !antes.ResultadoValido() || !depois.ResultadoValido() {
		c.Comparavel = false
	}

	c.Jornada = compararDistribuicao("jornada inteira (95%)", antes.Jornada.Latencia.P95, depois.Jornada.Latencia.P95)
	c.Global = compararDistribuicao("todas as requisicoes (95%)", antes.Global.Latencia.P95, depois.Global.Latencia.P95)
	c.Passos = compararPassos(antes, depois)
	c.Erro = compararErro(antes, depois)
	c.Frase = frasear(c, antes, depois)
	return c
}

func identificar(documento metrics.Documento) Identificacao {
	return Identificacao{
		Cenario: documento.Execucao.Cenario,
		Alvo:    documento.Execucao.Alvo,
		Inicio:  documento.Execucao.Inicio.Format("02/01/2006 15:04"),
		Versao:  documento.Versao,
	}
}

// Comparar duas execucoes so vale quando as duas mediram a mesma coisa do
// mesmo jeito; cada diferenca aqui pode explicar sozinha a variacao inteira.
func levantarRessalvas(antes, depois metrics.Documento) []string {
	var ressalvas []string

	if antes.Execucao.Cenario != depois.Execucao.Cenario {
		ressalvas = append(ressalvas, fmt.Sprintf("os cenarios sao diferentes: %q e %q", antes.Execucao.Cenario, depois.Execucao.Cenario))
	}
	if antes.Execucao.Alvo != depois.Execucao.Alvo {
		ressalvas = append(ressalvas, fmt.Sprintf("os alvos sao diferentes: %s e %s", antes.Execucao.Alvo, depois.Execucao.Alvo))
	}
	if antes.Ambiente.Maquina != depois.Ambiente.Maquina || antes.Ambiente.Nucleos != depois.Ambiente.Nucleos {
		ressalvas = append(ressalvas, fmt.Sprintf("as maquinas geradoras sao diferentes: %s com %d nucleos e %s com %d nucleos",
			antes.Ambiente.Maquina, antes.Ambiente.Nucleos, depois.Ambiente.Maquina, depois.Ambiente.Nucleos))
	}
	if planoResumido(antes) != planoResumido(depois) {
		ressalvas = append(ressalvas, fmt.Sprintf("os planos de carga sao diferentes: %s e %s", planoResumido(antes), planoResumido(depois)))
	}
	if antes.Versao != depois.Versao {
		ressalvas = append(ressalvas, fmt.Sprintf("as execucoes usaram versoes diferentes do braunrate: %s e %s", antes.Versao, depois.Versao))
	}
	if antes.Execucao.Autenticacoes > 0 || depois.Execucao.Autenticacoes > 0 {
		ressalvas = append(ressalvas, "as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao")
	}
	if !antes.ResultadoValido() {
		ressalvas = append(ressalvas, "a execucao anterior tem resultado invalido: o gerador saturou e o numero dela nao vale como base")
	}
	if !depois.ResultadoValido() {
		ressalvas = append(ressalvas, "a execucao nova tem resultado invalido: o gerador saturou e o numero dela nao vale como comparacao")
	}
	return ressalvas
}

func planoResumido(documento metrics.Documento) string {
	if len(documento.Execucao.PlanoAplicado) == 0 {
		return "sem plano declarado"
	}
	resumo := ""
	for indice, fase := range documento.Execucao.PlanoAplicado {
		if indice > 0 {
			resumo += " + "
		}
		resumo += fmt.Sprintf("%s ate %.0f/s por %ds", fase.Tipo, fase.Ate, fase.DuracaoMs/1000)
	}
	return resumo
}

func compararDistribuicao(nome string, antes, depois float64) Diferenca {
	diferenca := Diferenca{Metrica: nome, Antes: antes, Depois: depois, Sentido: SentidoIgual}
	if antes > 0 {
		diferenca.Variacao = (depois - antes) / antes
	}
	switch {
	case math.Abs(diferenca.Variacao) < RuidoAceito:
		diferenca.Sentido = SentidoIgual
	case diferenca.Variacao > 0:
		diferenca.Sentido = SentidoPiorou
	default:
		diferenca.Sentido = SentidoMelhor
	}
	diferenca.Frase = frasearDiferenca(diferenca)
	return diferenca
}

func frasearDiferenca(diferenca Diferenca) string {
	if diferenca.Antes == 0 && diferenca.Depois == 0 {
		return fmt.Sprintf("%s: sem amostra nas duas execucoes.", diferenca.Metrica)
	}
	if diferenca.Sentido == SentidoIgual {
		return fmt.Sprintf("%s: %.0f ms contra %.0f ms — diferenca dentro do ruido de duas execucoes.",
			diferenca.Metrica, diferenca.Antes, diferenca.Depois)
	}
	verbo := "mais lento"
	if diferenca.Sentido == SentidoMelhor {
		verbo = "mais rapido"
	}
	return fmt.Sprintf("%s: %s %s — de %.0f ms para %.0f ms.",
		diferenca.Metrica, Magnitude(diferenca), verbo, diferenca.Antes, diferenca.Depois)
}

// Acima de duas vezes, porcentagem para de ser legivel: "6994% mais lento"
// obriga o leitor a dividir de cabeca para chegar em "70 vezes".
func Magnitude(diferenca Diferenca) string {
	if diferenca.Sentido == SentidoIgual {
		return "sem diferenca"
	}
	if diferenca.Antes <= 0 || diferenca.Depois <= 0 {
		return fmt.Sprintf("%.0f%%", math.Abs(diferenca.Variacao)*100)
	}
	maior, menor := diferenca.Depois, diferenca.Antes
	if menor > maior {
		maior, menor = menor, maior
	}
	vezes := maior / menor
	if vezes < 2 {
		return fmt.Sprintf("%.0f%%", math.Abs(diferenca.Variacao)*100)
	}
	if vezes < 10 {
		return fmt.Sprintf("%.1f vezes", vezes)
	}
	return fmt.Sprintf("%.0f vezes", vezes)
}

func compararPassos(antes, depois metrics.Documento) []DiferencaDePasso {
	porNome := func(documento metrics.Documento) map[string]metrics.ResultadoDePasso {
		mapa := map[string]metrics.ResultadoDePasso{}
		for _, passo := range documento.Passos {
			mapa[passo.Nome] = passo
		}
		return mapa
	}
	deAntes, deDepois := porNome(antes), porNome(depois)

	nomes := map[string]bool{}
	for nome := range deAntes {
		nomes[nome] = true
	}
	for nome := range deDepois {
		nomes[nome] = true
	}
	ordenados := make([]string, 0, len(nomes))
	for nome := range nomes {
		ordenados = append(ordenados, nome)
	}
	sort.Strings(ordenados)

	var diferencas []DiferencaDePasso
	for _, nome := range ordenados {
		anterior, existiaAntes := deAntes[nome]
		novo, existeAgora := deDepois[nome]
		diferenca := DiferencaDePasso{
			Passo: nome,
			Novo:  !existiaAntes,
			Sumiu: !existeAgora,
			P95:   compararDistribuicao(nome+" (95%)", anterior.Latencia.P95, novo.Latencia.P95),
			P99:   compararDistribuicao(nome+" (99%)", anterior.Latencia.P99, novo.Latencia.P99),
		}
		diferencas = append(diferencas, diferenca)
	}
	return diferencas
}

func compararErro(antes, depois metrics.Documento) DiferencaDeContagem {
	diferenca := DiferencaDeContagem{Antes: antes.Global.TaxaDeErro * 100, Depois: depois.Global.TaxaDeErro * 100}
	switch {
	case diferenca.Antes == 0 && diferenca.Depois == 0:
		diferenca.Frase = "Nenhuma das duas execucoes teve erro."
	case diferenca.Depois > diferenca.Antes:
		diferenca.Frase = fmt.Sprintf("A taxa de erro subiu de %.2f%% para %.2f%%.", diferenca.Antes, diferenca.Depois)
	case diferenca.Depois < diferenca.Antes:
		diferenca.Frase = fmt.Sprintf("A taxa de erro caiu de %.2f%% para %.2f%%.", diferenca.Antes, diferenca.Depois)
	default:
		diferenca.Frase = fmt.Sprintf("A taxa de erro ficou em %.2f%% nas duas.", diferenca.Antes)
	}
	return diferenca
}

func frasear(c Comparacao, antes, depois metrics.Documento) string {
	if !c.Comparavel {
		return "Nao da para comparar: pelo menos uma das execucoes tem resultado invalido porque o gerador saturou."
	}

	principal := c.Jornada
	if antes.Jornada.Iniciadas == 0 || depois.Jornada.Iniciadas == 0 {
		principal = c.Global
	}

	prefixo := "Sem mudanca que valha leitura"
	if principal.Sentido == SentidoPiorou {
		prefixo = "Ficou mais lento"
	}
	if principal.Sentido == SentidoMelhor {
		prefixo = "Ficou mais rapido"
	}

	frase := fmt.Sprintf("%s: %s", prefixo, principal.Frase)
	if c.Erro.Depois != c.Erro.Antes {
		frase += " " + c.Erro.Frase
	}
	if len(c.Ressalvas) > 0 {
		frase += fmt.Sprintf(" Com %d ressalva(s) que podem explicar a diferenca sozinhas.", len(c.Ressalvas))
	}
	return frase
}
