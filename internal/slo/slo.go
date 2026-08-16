package slo

import (
	"fmt"
	"strings"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Veredito = metrics.Veredito

type Avaliacao = metrics.Avaliacao

// Execucao em que nenhuma jornada chegou ao fim nao pode passar, mesmo sem SLO
// declarado que a pegue: o cenario nao mediu o que se propos a medir, e um
// veredito verde ali e afirmacao errada, nao falta de criterio.
func Avaliar(regras []scenario.RegraDeSLO, documento metrics.Documento) Veredito {
	veredito := Veredito{Passou: true}
	if avaliacao, houve := jornadaNuncaCompletou(documento); houve {
		veredito.Passou = false
		veredito.Avaliacoes = append(veredito.Avaliacoes, avaliacao)
	}
	porPasso := map[string]metrics.ResultadoDePasso{}
	for _, passo := range documento.Passos {
		porPasso[passo.Nome] = passo
	}

	for _, regra := range regras {
		avaliacao := avaliarRegra(regra, documento, porPasso)
		if !avaliacao.Passou {
			veredito.Passou = false
		}
		veredito.Avaliacoes = append(veredito.Avaliacoes, avaliacao)
	}

	veredito.Frase = frasear(veredito)
	return veredito
}

func jornadaNuncaCompletou(documento metrics.Documento) (Avaliacao, bool) {
	if documento.Jornada.Iniciadas == 0 || documento.Jornada.Completas > 0 {
		return Avaliacao{}, false
	}
	return Avaliacao{
		Passo:   "jornada",
		Metrica: "completas",
		Regra:   "toda jornada precisa chegar ao fim",
		Obtido:  0,
		Limite:  float64(documento.Jornada.Iniciadas),
		Unidade: "jornadas",
		Passou:  false,
		Frase: fmt.Sprintf("Falhou: nenhuma das %d jornadas chegou ao fim, entao o cenario nao mediu o que se propos a medir. Rode 'braunrate depurar' para ver onde a iteracao para.",
			documento.Jornada.Iniciadas),
	}, true
}

func avaliarRegra(regra scenario.RegraDeSLO, documento metrics.Documento, porPasso map[string]metrics.ResultadoDePasso) Avaliacao {
	avaliacao := Avaliacao{
		Passo:   nomeDoAlvo(regra),
		Metrica: regra.Metrica,
		Regra:   regra.Texto,
		Limite:  regra.Limite,
		Unidade: regra.Unidade,
	}

	var distribuicao metrics.Distribuicao
	var contagem, erros int64

	if regra.Global {
		distribuicao = documento.Global.Latencia
		contagem, erros = documento.Global.Contagem, documento.Global.Erros
	} else {
		passo, existe := porPasso[regra.Passo]
		if !existe {
			avaliacao.SemDados = true
			avaliacao.Passou = false
			avaliacao.Frase = fmt.Sprintf("Sem dados: o passo %q nao produziu nenhuma requisicao, entao a regra %q nao pode ser verificada.", regra.Passo, regra.Texto)
			return avaliacao
		}
		distribuicao = passo.Latencia
		contagem, erros = passo.Contagem, passo.Erros
	}

	switch regra.Metrica {
	case "p50":
		avaliacao.Obtido = distribuicao.P50
	case "p75":
		avaliacao.Obtido = distribuicao.P75
	case "p90":
		avaliacao.Obtido = distribuicao.P90
	case "p95":
		avaliacao.Obtido = distribuicao.P95
	case "p99":
		avaliacao.Obtido = distribuicao.P99
	case "p99.9":
		avaliacao.Obtido = distribuicao.P999
	case "max":
		avaliacao.Obtido = distribuicao.Maximo
	case "erros":
		if contagem > 0 {
			avaliacao.Obtido = float64(erros) / float64(contagem) * 100
		}
	case "vazao":
		avaliacao.Obtido = documento.Global.TaxaEfetiva
	}

	avaliacao.Passou = comparar(avaliacao.Obtido, regra.Operador, regra.Limite)
	avaliacao.Frase = frasearAvaliacao(avaliacao, regra)
	return avaliacao
}

func comparar(obtido float64, operador scenario.Operador, limite float64) bool {
	switch operador {
	case scenario.OperadorMenor:
		return obtido < limite
	case scenario.OperadorMenorOuIgual:
		return obtido <= limite
	case scenario.OperadorMaior:
		return obtido > limite
	case scenario.OperadorMaiorOuIgual:
		return obtido >= limite
	case scenario.OperadorDiferente:
		return obtido != limite
	default:
		return obtido == limite
	}
}

func nomeDoAlvo(regra scenario.RegraDeSLO) string {
	if regra.Global {
		return "global"
	}
	return regra.Passo
}

func nomeLegivel(metrica string) string {
	switch metrica {
	case "erros":
		return "taxa de erro"
	case "vazao":
		return "vazao"
	case "max":
		return "latencia maxima"
	default:
		return "latencia " + metrica
	}
}

func formatar(valor float64, unidade string) string {
	switch unidade {
	case "ms":
		return fmt.Sprintf("%.0f ms", valor)
	case "%":
		return fmt.Sprintf("%.2f%%", valor)
	case "/s":
		return fmt.Sprintf("%.0f/s", valor)
	default:
		return fmt.Sprintf("%.2f", valor)
	}
}

func frasearAvaliacao(avaliacao Avaliacao, regra scenario.RegraDeSLO) string {
	alvo := avaliacao.Passo
	if regra.Global {
		alvo = "o cenario inteiro"
	} else {
		alvo = fmt.Sprintf("%q", alvo)
	}
	comparacao := "acima do limite de"
	if regra.Operador == scenario.OperadorMaior || regra.Operador == scenario.OperadorMaiorOuIgual {
		comparacao = "abaixo do minimo de"
	}
	if avaliacao.Passou {
		return fmt.Sprintf("Passou: %s teve %s de %s, dentro do limite de %s.",
			alvo, nomeLegivel(avaliacao.Metrica), formatar(avaliacao.Obtido, avaliacao.Unidade), formatar(avaliacao.Limite, avaliacao.Unidade))
	}
	return fmt.Sprintf("Falhou: %s teve %s de %s, %s %s.",
		alvo, nomeLegivel(avaliacao.Metrica), formatar(avaliacao.Obtido, avaliacao.Unidade),
		comparacao, formatar(avaliacao.Limite, avaliacao.Unidade))
}

func frasear(veredito Veredito) string {
	if len(veredito.Avaliacoes) == 0 {
		return "Sem SLO declarado: nada foi verificado."
	}
	var falhas []string
	for _, avaliacao := range veredito.Avaliacoes {
		if !avaliacao.Passou {
			falhas = append(falhas, avaliacao.Frase)
		}
	}
	if len(falhas) == 0 {
		if len(veredito.Avaliacoes) == 1 {
			return "Passou: a unica regra de SLO foi atendida."
		}
		return fmt.Sprintf("Passou: as %d regras de SLO foram atendidas.", len(veredito.Avaliacoes))
	}
	return strings.Join(falhas, " ")
}
