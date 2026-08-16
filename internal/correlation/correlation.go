package correlation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/tidwall/gjson"
)

var (
	mutexDeRegex    sync.Mutex
	regexCompilados = map[string]*regexp.Regexp{}
)

func compile(expression string) (*regexp.Regexp, error) {
	mutexDeRegex.Lock()
	defer mutexDeRegex.Unlock()
	if compiled, exists := regexCompilados[expression]; exists {
		return compiled, nil
	}
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return nil, err
	}
	regexCompilados[expression] = compiled
	return compiled, nil
}

type CaptureError struct {
	Variable   string
	Expression string
	Reason     string
}

func (e CaptureError) Error() string {
	return fmt.Sprintf("nao consegui capturar %q com %s: %s", e.Variable, e.Expression, e.Reason)
}

func Extract(capture scenario.Capture, response protocol.Response) (string, error) {
	switch capture.Origin {
	case scenario.CaptureJSON:
		return extrairDeJSON(capture, response)
	case scenario.CaptureHeader:
		for name, values := range response.Headers {
			if strings.EqualFold(name, capture.Expression) && len(values) > 0 {
				return values[0], nil
			}
		}
		return "", CaptureError{capture.Variable, capture.Expression, "cabecalho ausente na resposta"}
	case scenario.CaptureRegex:
		compiled, err := compile(capture.Expression)
		if err != nil {
			return "", CaptureError{capture.Variable, capture.Expression, "expressao regular invalida: " + err.Error()}
		}
		found := compiled.FindSubmatch(response.Body)
		if found == nil {
			return "", CaptureError{capture.Variable, capture.Expression, "nenhuma ocorrencia no corpo"}
		}
		if len(found) > 1 {
			return string(found[1]), nil
		}
		return string(found[0]), nil
	case scenario.CaptureStatus:
		return strconv.Itoa(response.Status), nil
	case scenario.CaptureBody:
		return string(response.Body), nil
	default:
		return "", CaptureError{capture.Variable, capture.Expression, "origem de captura desconhecida"}
	}
}

// JSONPath aqui e o subconjunto que cobre o caso comum: caminho com pontos e
// indice de lista. Aceita a forma "$.a.b[0].c" e a forma curta "a.b.0.c".
func extrairDeJSON(capture scenario.Capture, response protocol.Response) (string, error) {
	path := PathToGJSON(capture.Expression)
	result := gjson.GetBytes(response.Body, path)
	if !result.Exists() {
		if !gjson.ValidBytes(response.Body) {
			return "", CaptureError{capture.Variable, capture.Expression, "a resposta nao e JSON valido"}
		}
		return "", CaptureError{capture.Variable, capture.Expression, "caminho nao encontrado no corpo da resposta"}
	}
	return result.String(), nil
}

func PathToGJSON(expression string) string {
	path := strings.TrimPrefix(strings.TrimSpace(expression), "$")
	path = strings.TrimPrefix(path, ".")
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	return path
}
