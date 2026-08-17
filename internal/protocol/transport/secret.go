package transport

import (
	"regexp"
	"strconv"
	"strings"
)

// Um segredo aparece no que a ferramenta imprime por quatro portas: cabecalho,
// parametro de consulta, campo de corpo e variavel. As quatro cortam pelo mesmo
// nome, em IsSecretName, e cortam aqui — nao em cada saida que as imprime.

// MaskQuery cuts the value of every credential-looking query parameter. O
// caminho vira chave de agregacao e entra no documento de resultado, entao um
// token na URL sai no JSON, no HTML e na comparacao sem passar por mascara
// nenhuma.
func MaskQuery(path string) string {
	address, query, found := strings.Cut(path, "?")
	if !found {
		return path
	}
	pairs := strings.Split(query, "&")
	for index, pair := range pairs {
		name, value, hasValue := strings.Cut(pair, "=")
		if !hasValue || value == "" || !IsSecretName(name) {
			continue
		}
		pairs[index] = name + "=" + cut(value)
	}
	return address + "?" + strings.Join(pairs, "&")
}

var jsonField = regexp.MustCompile(`"([A-Za-z_][A-Za-z0-9_.-]*)"\s*:\s*"([^"]*)"`)
var formField = regexp.MustCompile(`(^|&)([A-Za-z_][A-Za-z0-9_.-]*)=([^&]*)`)

// MaskBody cuts credential-looking fields in a request or response body, and
// leaves the rest as it is. O corpo da resposta e o que o 'debug' existe para
// mostrar: apagar tudo tiraria o diagnostico junto com o segredo, e apagar nada
// publica o token que o login acabou de devolver.
func MaskBody(body string) string {
	masked := jsonField.ReplaceAllStringFunc(body, func(occurrence string) string {
		parts := jsonField.FindStringSubmatch(occurrence)
		if !IsSecretName(parts[1]) || parts[2] == "" {
			return occurrence
		}
		return `"` + parts[1] + `": "` + cut(parts[2]) + `"`
	})
	return formField.ReplaceAllStringFunc(masked, func(occurrence string) string {
		parts := formField.FindStringSubmatch(occurrence)
		if !IsSecretName(parts[2]) || parts[3] == "" {
			return occurrence
		}
		return parts[1] + parts[2] + "=" + cut(parts[3])
	})
}

// O corte unico: seis caracteres dizem que o campo chegou e com que valor ele
// comeca, e o tamanho diz se o que chegou tem a cara do que deveria. Mais do que
// isso e o segredo entregue em prestacoes.
const shown = 6

func cut(value string) string {
	if len(value) <= shown {
		return "***"
	}
	return value[:shown] + "… (" + strconv.Itoa(len(value)) + " characters)"
}
