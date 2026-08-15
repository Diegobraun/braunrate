package correlacao

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/tidwall/gjson"
)

type FalhaDeAssercao struct {
	Descricao string
	Esperado  string
	Obtido    string
}

func (f FalhaDeAssercao) Error() string {
	return fmt.Sprintf("%s: esperava %s, obteve %s", f.Descricao, f.Esperado, f.Obtido)
}

func Avaliar(assercao cenario.Assercao, resposta protocolo.Resposta, resolver func(string) string) error {
	esperado := resolver(assercao.Valor)

	switch assercao.Tipo {
	case cenario.AsserirStatus:
		return compararNumero(assercao, float64(resposta.Status), esperado, "status")
	case cenario.AsserirCorpoContem:
		if strings.Contains(string(resposta.Corpo), esperado) {
			return nil
		}
		return FalhaDeAssercao{"corpo da resposta", "conter " + strconv.Quote(esperado), "corpo sem o texto"}
	case cenario.AsserirCabecalho:
		for nome, valores := range resposta.Cabecalhos {
			if strings.EqualFold(nome, assercao.Alvo) {
				for _, valor := range valores {
					if valor == esperado || (assercao.Operador == cenario.OperadorContem && strings.Contains(valor, esperado)) {
						return nil
					}
				}
				return FalhaDeAssercao{"cabecalho " + assercao.Alvo, strconv.Quote(esperado), strconv.Quote(strings.Join(valores, ", "))}
			}
		}
		return FalhaDeAssercao{"cabecalho " + assercao.Alvo, strconv.Quote(esperado), "cabecalho ausente"}
	case cenario.AsserirRegex:
		compilado, err := compilar(esperado)
		if err != nil {
			return FalhaDeAssercao{"expressao regular", esperado, "expressao invalida: " + err.Error()}
		}
		if compilado.Match(resposta.Corpo) {
			return nil
		}
		return FalhaDeAssercao{"corpo da resposta", "casar com /" + esperado + "/", "nenhuma ocorrencia"}
	case cenario.AsserirJSON:
		return avaliarJSON(assercao, resposta, esperado)
	default:
		return FalhaDeAssercao{"assercao", string(assercao.Tipo), "tipo desconhecido"}
	}
}

func avaliarJSON(assercao cenario.Assercao, resposta protocolo.Resposta, esperado string) error {
	descricao := "campo " + assercao.Alvo
	resultado := gjson.GetBytes(resposta.Corpo, CaminhoParaGJSON(assercao.Alvo))

	if assercao.Operador == cenario.OperadorExiste {
		if resultado.Exists() {
			return nil
		}
		return FalhaDeAssercao{descricao, "existir na resposta", "ausente"}
	}
	if !resultado.Exists() {
		return FalhaDeAssercao{descricao, strconv.Quote(esperado), "campo ausente na resposta"}
	}

	if numero, err := strconv.ParseFloat(esperado, 64); err == nil && resultado.Type == gjson.Number {
		return compararNumero(assercao, resultado.Float(), strconv.FormatFloat(numero, 'f', -1, 64), descricao)
	}

	obtido := resultado.String()
	switch assercao.Operador {
	case cenario.OperadorDiferente:
		if obtido != esperado {
			return nil
		}
		return FalhaDeAssercao{descricao, "diferente de " + strconv.Quote(esperado), strconv.Quote(obtido)}
	case cenario.OperadorContem:
		if strings.Contains(obtido, esperado) {
			return nil
		}
		return FalhaDeAssercao{descricao, "conter " + strconv.Quote(esperado), strconv.Quote(obtido)}
	default:
		if obtido == esperado {
			return nil
		}
		return FalhaDeAssercao{descricao, strconv.Quote(esperado), strconv.Quote(obtido)}
	}
}

func compararNumero(assercao cenario.Assercao, obtido float64, esperadoTexto, descricao string) error {
	esperado, err := strconv.ParseFloat(strings.TrimSpace(esperadoTexto), 64)
	if err != nil {
		return FalhaDeAssercao{descricao, esperadoTexto, "valor esperado nao e numero"}
	}
	obtidoTexto := strconv.FormatFloat(obtido, 'f', -1, 64)

	atende := false
	switch assercao.Operador {
	case cenario.OperadorMenor:
		atende = obtido < esperado
	case cenario.OperadorMenorOuIgual:
		atende = obtido <= esperado
	case cenario.OperadorMaior:
		atende = obtido > esperado
	case cenario.OperadorMaiorOuIgual:
		atende = obtido >= esperado
	case cenario.OperadorDiferente:
		atende = obtido != esperado
	default:
		atende = obtido == esperado
	}
	if atende {
		return nil
	}
	operador := string(assercao.Operador)
	if operador == "" {
		operador = "=="
	}
	return FalhaDeAssercao{descricao, operador + " " + esperadoTexto, obtidoTexto}
}
