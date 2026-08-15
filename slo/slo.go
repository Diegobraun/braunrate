package slo

import (
	"fmt"
	"strings"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
)

type Veredito struct {
	Passou     bool        `json:"passou"`
	Avaliacoes []Avaliacao `json:"avaliacoes"`
	Frase      string      `json:"frase"`
}

type Avaliacao struct {
	Passo    string  `json:"passo"`
	Metrica  string  `json:"metrica"`
	Regra    string  `json:"regra"`
	Obtido   float64 `json:"obtido"`
	Limite   float64 `json:"limite"`
	Unidade  string  `json:"unidade"`
	Passou   bool    `json:"passou"`
	Frase    string  `json:"frase"`
	SemDados bool    `json:"sem_dados"`
}

func Avaliar(regras []cenario.RegraDeSLO, documento metrica.Documento) Veredito {
	veredito := Veredito{Passou: true}
	porPasso := map[string]metrica.ResultadoDePasso{}
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

func avaliarRegra(regra cenario.RegraDeSLO, documento metrica.Documento, porPasso map[string]metrica.ResultadoDePasso) Avaliacao {
	avaliacao := Avaliacao{
		Passo:   nomeDoAlvo(regra),
		Metrica: regra.Metrica,
		Regra:   regra.Texto,
		Limite:  regra.Limite,
		Unidade: regra.Unidade,
	}

	var distribuicao metrica.Distribuicao
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

func comparar(obtido float64, operador cenario.Operador, limite float64) bool {
	switch operador {
	case cenario.OperadorMenor:
		return obtido < limite
	case cenario.OperadorMenorOuIgual:
		return obtido <= limite
	case cenario.OperadorMaior:
		return obtido > limite
	case cenario.OperadorMaiorOuIgual:
		return obtido >= limite
	case cenario.OperadorDiferente:
		return obtido != limite
	default:
		return obtido == limite
	}
}

func nomeDoAlvo(regra cenario.RegraDeSLO) string {
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

func frasearAvaliacao(avaliacao Avaliacao, regra cenario.RegraDeSLO) string {
	alvo := avaliacao.Passo
	if regra.Global {
		alvo = "o cenario inteiro"
	} else {
		alvo = fmt.Sprintf("%q", alvo)
	}
	comparacao := "acima do limite de"
	if regra.Operador == cenario.OperadorMaior || regra.Operador == cenario.OperadorMaiorOuIgual {
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
		return fmt.Sprintf("Passou: as %d regras de SLO foram atendidas.", len(veredito.Avaliacoes))
	}
	return strings.Join(falhas, " ")
}
