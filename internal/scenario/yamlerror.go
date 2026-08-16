package scenario

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The yaml parser fails before any of ours runs, so this is the only place in
// the product that used to answer in English, with no file and no column, about
// the most common mistake of all: ${ and } are exactly the characters an inline
// map uses.
var yamlLine = regexp.MustCompile(`^yaml: (?:line (\d+): )?(.*)$`)

func translateYAMLError(content []byte, err error) error {
	match := yamlLine.FindStringSubmatch(strings.TrimSpace(err.Error()))
	if match == nil {
		return ScenarioError{Line: 1, Message: fmt.Sprintf("o arquivo nao e YAML valido: %v", err)}
	}

	problem := match[2]
	// The parser reports where it gave up, which is the start of the construct
	// and not always the character that broke it. The suspect is looked for from
	// there onward, and the position reported is the one of the suspect.
	line, column := suspect(content, atoiOr(match[1], 1))
	return ScenarioError{Line: line, Column: column, Message: explainYAML(problem, lineAt(content, line))}
}

func atoiOr(text string, fallback int) int {
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

var suspects = []string{"${", "$.", "#"}

func suspect(content []byte, from int) (int, int) {
	lines := strings.Split(string(content), "\n")
	for number := from; number <= from+3 && number <= len(lines); number++ {
		text := lines[number-1]
		if !strings.Contains(text, "{") && !strings.Contains(text, "[") {
			continue
		}
		for _, mark := range suspects {
			if at := strings.Index(text, mark); at >= 0 {
				return number, at + 1
			}
		}
	}
	return from, 1
}

func lineAt(content []byte, line int) string {
	if line <= 0 {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
}

func explainYAML(problem, text string) string {
	switch {
	case strings.Contains(problem, "did not find expected ',' or '}'"),
		strings.Contains(problem, "did not find expected ',' or ']'"):
		return "mapa em linha que nao fecha. " + inlineAdvice(text)
	case strings.Contains(problem, "could not find expected ':'"):
		return "faltou os dois-pontos depois da chave.\n" +
			"    em YAML cada chave termina em ':', por exemplo:  nome: Consulta de pedidos"
	case strings.Contains(problem, "found character that cannot start any token"):
		return "ha um caractere de tabulacao nesta linha, e YAML nao aceita tabulacao para indentar.\n" +
			"    troque a tabulacao por espacos (dois por nivel, como no resto do arquivo)"
	case strings.Contains(problem, "mapping values are not allowed in this context"):
		return "ha dois-pontos dentro de um valor que nao esta entre aspas.\n" +
			"    ponha o valor entre aspas, por exemplo:  cabecalho: \"X-API-Key: ${API_KEY}\""
	case strings.Contains(problem, "did not find expected key"),
		strings.Contains(problem, "found a tab character"):
		return "indentacao inconsistente nesta linha.\n" +
			"    use sempre espacos, e o mesmo numero de espacos para itens do mesmo nivel"
	}
	return "o arquivo nao e YAML valido nesta linha: " + problem
}

// The advice changes with what is on the line, because "a map that does not
// close" is the symptom of three different mistakes and pointing at the wrong
// one costs an edit.
func inlineAdvice(text string) string {
	switch {
	case strings.Contains(text, "${"):
		return "Dentro de { } o YAML trata '{' e '}' como estrutura, e ${variavel} carrega os dois.\n" +
			"    ponha o valor entre aspas, por exemplo:\n" +
			"      kafka: { topico: pedidos, chave: \"${pedidos.id}\" }"
	case strings.Contains(text, "[") && strings.Contains(text, "$."):
		return "Um caminho JSON com colchete, como $.itens[0].id, precisa de aspas dentro de { }:\n" +
			"      captura: { faturaId: \"$.itens[0].id\" }"
	case strings.Contains(text, "#"):
		return "Um '#' dentro de { } comeca comentario e engole o resto da linha.\n" +
			"    ponha o valor entre aspas, por exemplo:\n" +
			"      gerar: { id: { tipo: padrao, formato: \"PED-######\" } }"
	}
	return "confira se toda chave tem valor e se as chaves estao separadas por virgula, por exemplo:\n" +
		"      http: { metodo: GET, caminho: /pedidos }"
}
