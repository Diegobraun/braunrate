package correlation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/tidwall/gjson"
)

type AssertionFailure struct {
	Description string
	Expected    string
	Obtained    string
}

func (assertionFailure AssertionFailure) Error() string {
	return fmt.Sprintf("%s: esperava %s, obteve %s", assertionFailure.Description, assertionFailure.Expected, assertionFailure.Obtained)
}

func Evaluate(assertion scenario.Assertion, response protocol.Response, resolve func(string) string) error {
	expected := resolve(assertion.Value)

	switch assertion.Kind {
	case scenario.AssertStatus:
		return compareNumbers(assertion, float64(response.Status), expected, "status")
	case scenario.AssertBodyContains:
		if strings.Contains(string(response.Body), expected) {
			return nil
		}
		return AssertionFailure{"corpo da resposta", "conter " + strconv.Quote(expected), "corpo sem o texto"}
	case scenario.AssertHeader:
		for name, values := range response.Headers {
			if strings.EqualFold(name, assertion.Target) {
				for _, value := range values {
					if value == expected || (assertion.Operator == scenario.OpContains && strings.Contains(value, expected)) {
						return nil
					}
				}
				return AssertionFailure{"cabecalho " + assertion.Target, strconv.Quote(expected), strconv.Quote(strings.Join(values, ", "))}
			}
		}
		return AssertionFailure{"cabecalho " + assertion.Target, strconv.Quote(expected), "cabecalho ausente"}
	case scenario.AssertRegex:
		compiled, err := compile(expected)
		if err != nil {
			return AssertionFailure{"expressao regular", expected, "expressao inválida: " + err.Error()}
		}
		if compiled.Match(response.Body) {
			return nil
		}
		return AssertionFailure{"corpo da resposta", "casar com /" + expected + "/", "nenhuma ocorrência"}
	case scenario.AssertJSON:
		return avaliarJSON(assertion, response, expected)
	default:
		return AssertionFailure{"assercao", string(assertion.Kind), "tipo desconhecido"}
	}
}

func avaliarJSON(assertion scenario.Assertion, response protocol.Response, expected string) error {
	description := "campo " + assertion.Target
	result := gjson.GetBytes(response.Body, PathToGJSON(assertion.Target))

	if assertion.Operator == scenario.OpExists {
		if result.Exists() {
			return nil
		}
		return AssertionFailure{description, "existir na resposta", "ausente"}
	}
	if !result.Exists() {
		return AssertionFailure{description, strconv.Quote(expected), "campo ausente na resposta"}
	}

	if number, err := strconv.ParseFloat(expected, 64); err == nil && result.Type == gjson.Number {
		return compareNumbers(assertion, result.Float(), strconv.FormatFloat(number, 'f', -1, 64), description)
	}

	obtained := result.String()
	switch assertion.Operator {
	case scenario.OpNotEqual:
		if obtained != expected {
			return nil
		}
		return AssertionFailure{description, "diferente de " + strconv.Quote(expected), strconv.Quote(obtained)}
	case scenario.OpContains:
		if strings.Contains(obtained, expected) {
			return nil
		}
		return AssertionFailure{description, "conter " + strconv.Quote(expected), strconv.Quote(obtained)}
	default:
		if obtained == expected {
			return nil
		}
		return AssertionFailure{description, strconv.Quote(expected), strconv.Quote(obtained)}
	}
}

func compareNumbers(assertion scenario.Assertion, obtained float64, esperadoTexto, description string) error {
	expected, err := strconv.ParseFloat(strings.TrimSpace(esperadoTexto), 64)
	if err != nil {
		return AssertionFailure{description, esperadoTexto, "valor esperado não e número"}
	}
	obtidoTexto := strconv.FormatFloat(obtained, 'f', -1, 64)

	serves := false
	switch assertion.Operator {
	case scenario.OpLess:
		serves = obtained < expected
	case scenario.OpLessOrEqual:
		serves = obtained <= expected
	case scenario.OpGreater:
		serves = obtained > expected
	case scenario.OpGreaterOrEqual:
		serves = obtained >= expected
	case scenario.OpNotEqual:
		serves = obtained != expected
	default:
		serves = obtained == expected
	}
	if serves {
		return nil
	}
	operator := string(assertion.Operator)
	if operator == "" {
		operator = "=="
	}
	return AssertionFailure{description, operator + " " + esperadoTexto, obtidoTexto}
}
