package correlacao

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/tidwall/gjson"
)

var (
	mutexDeRegex    sync.Mutex
	regexCompilados = map[string]*regexp.Regexp{}
)

func compilar(expressao string) (*regexp.Regexp, error) {
	mutexDeRegex.Lock()
	defer mutexDeRegex.Unlock()
	if compilado, existe := regexCompilados[expressao]; existe {
		return compilado, nil
	}
	compilado, err := regexp.Compile(expressao)
	if err != nil {
		return nil, err
	}
	regexCompilados[expressao] = compilado
	return compilado, nil
}

type ErroDeCaptura struct {
	Variavel  string
	Expressao string
	Motivo    string
}

func (e ErroDeCaptura) Error() string {
	return fmt.Sprintf("nao consegui capturar %q com %s: %s", e.Variavel, e.Expressao, e.Motivo)
}

func Extrair(captura cenario.Captura, resposta protocolo.Resposta) (string, error) {
	switch captura.Origem {
	case cenario.CapturaDeJSON:
		return extrairDeJSON(captura, resposta)
	case cenario.CapturaDeCabecalho:
		for nome, valores := range resposta.Cabecalhos {
			if strings.EqualFold(nome, captura.Expressao) && len(valores) > 0 {
				return valores[0], nil
			}
		}
		return "", ErroDeCaptura{captura.Variavel, captura.Expressao, "cabecalho ausente na resposta"}
	case cenario.CapturaDeRegex:
		compilado, err := compilar(captura.Expressao)
		if err != nil {
			return "", ErroDeCaptura{captura.Variavel, captura.Expressao, "expressao regular invalida: " + err.Error()}
		}
		encontrado := compilado.FindSubmatch(resposta.Corpo)
		if encontrado == nil {
			return "", ErroDeCaptura{captura.Variavel, captura.Expressao, "nenhuma ocorrencia no corpo"}
		}
		if len(encontrado) > 1 {
			return string(encontrado[1]), nil
		}
		return string(encontrado[0]), nil
	case cenario.CapturaDeStatus:
		return strconv.Itoa(resposta.Status), nil
	case cenario.CapturaDeCorpo:
		return string(resposta.Corpo), nil
	default:
		return "", ErroDeCaptura{captura.Variavel, captura.Expressao, "origem de captura desconhecida"}
	}
}

// JSONPath aqui e o subconjunto que cobre o caso comum: caminho com pontos e
// indice de lista. Aceita a forma "$.a.b[0].c" e a forma curta "a.b.0.c".
func extrairDeJSON(captura cenario.Captura, resposta protocolo.Resposta) (string, error) {
	caminho := CaminhoParaGJSON(captura.Expressao)
	resultado := gjson.GetBytes(resposta.Corpo, caminho)
	if !resultado.Exists() {
		if !gjson.ValidBytes(resposta.Corpo) {
			return "", ErroDeCaptura{captura.Variavel, captura.Expressao, "a resposta nao e JSON valido"}
		}
		return "", ErroDeCaptura{captura.Variavel, captura.Expressao, "caminho nao encontrado no corpo da resposta"}
	}
	return resultado.String(), nil
}

func CaminhoParaGJSON(expressao string) string {
	caminho := strings.TrimPrefix(strings.TrimSpace(expressao), "$")
	caminho = strings.TrimPrefix(caminho, ".")
	caminho = strings.ReplaceAll(caminho, "[", ".")
	caminho = strings.ReplaceAll(caminho, "]", "")
	return caminho
}
