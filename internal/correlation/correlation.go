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

func (captureError CaptureError) Error() string {
	return fmt.Sprintf("não consegui capturar %q com %s: %s", captureError.Variable, captureError.Expression, captureError.Reason)
}

func Extract(capture scenario.Capture, response protocol.Response) (string, error) {
	switch capture.Origin {
	case scenario.CaptureJSON:
		return extractFromJSON(capture, response)
	case scenario.CaptureHeader:
		for name, values := range response.Headers {
			if strings.EqualFold(name, capture.Expression) && len(values) > 0 {
				return values[0], nil
			}
		}
		return "", CaptureError{capture.Variable, capture.Expression, "cabecalho ausente na resposta"}
	case scenario.CaptureCookie:
		return extractCookie(capture, response)
	case scenario.CaptureRegex:
		compiled, err := compile(capture.Expression)
		if err != nil {
			return "", CaptureError{capture.Variable, capture.Expression, "expressao regular inválida: " + err.Error()}
		}
		found := compiled.FindSubmatch(response.Body)
		if found == nil {
			return "", CaptureError{capture.Variable, capture.Expression, "nenhuma ocorrência no corpo"}
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

// A response can carry more than one Set-Cookie, and only the pair matters: the
// attributes that follow it (Path, HttpOnly, Max-Age) describe the browser's
// duty, and sending them back as if they were cookies is what capturing the raw
// header would do.
func extractCookie(capture scenario.Capture, response protocol.Response) (string, error) {
	for name, values := range response.Headers {
		if !strings.EqualFold(name, "Set-Cookie") {
			continue
		}
		for _, value := range values {
			pair, _, _ := strings.Cut(value, ";")
			cookie, content, found := strings.Cut(strings.TrimSpace(pair), "=")
			if found && strings.EqualFold(strings.TrimSpace(cookie), capture.Expression) {
				return strings.TrimSpace(content), nil
			}
		}
	}
	return "", CaptureError{capture.Variable, capture.Expression, "a resposta não trouxe esse cookie em Set-Cookie"}
}

// JSONPath here is the subset that covers the common case: dotted paths and
// list indexes. It takes both "$.a.b[0].c" and the short "a.b.0.c".
func extractFromJSON(capture scenario.Capture, response protocol.Response) (string, error) {
	path := PathToGJSON(capture.Expression)
	result := gjson.GetBytes(response.Body, path)
	if !result.Exists() {
		if !gjson.ValidBytes(response.Body) {
			return "", CaptureError{capture.Variable, capture.Expression, "a resposta não e JSON válido"}
		}
		return "", CaptureError{capture.Variable, capture.Expression, "caminho não encontrado no corpo da resposta"}
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
