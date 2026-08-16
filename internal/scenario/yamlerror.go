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
		return ScenarioError{Line: 1, Message: fmt.Sprintf("the file is not valid YAML: %v", err)}
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
		return "an inline map that does not close. " + inlineAdvice(text)
	case strings.Contains(problem, "could not find expected ':'"):
		return "faltou os dois-pontos depois da chave.\n" +
			"    em YAML cada chave termina em ':', por exemplo:  nome: Consulta de pedidos"
	case strings.Contains(problem, "found character that cannot start any token"):
		return "there is a tab character on this line, and YAML does not accept tabs for indentation.\n" +
			"    replace the tab with spaces (two per level, like the rest of the file)"
	case strings.Contains(problem, "mapping values are not allowed in this context"):
		return "there is a colon inside a value that is not quoted.\n" +
			"    ponha o valor entre aspas, por exemplo:  cabecalho: \"X-API-Key: ${API_KEY}\""
	case strings.Contains(problem, "did not find expected key"),
		strings.Contains(problem, "found a tab character"):
		return "indentacao inconsistente nesta linha.\n" +
			"    always use spaces, and the same number of spaces for items at the same level"
	}
	return "the file is not valid YAML on this line: " + problem
}

// The advice changes with what is on the line, because "a map that does not
// close" is the symptom of three different mistakes and pointing at the wrong
// one costs an edit.
func inlineAdvice(text string) string {
	switch {
	case strings.Contains(text, "${"):
		return "Inside { } YAML reads '{' and '}' as structure, and ${variable} carries both.\n" +
			"    quote the value, for example:\n" +
			"      kafka: { topic: orders, key: \"${orders.id}\" }"
	case strings.Contains(text, "[") && strings.Contains(text, "$."):
		return "A JSON path with a bracket, like $.items[0].id, needs quotes inside { }:\n" +
			"      capture: { invoiceId: \"$.items[0].id\" }"
	case strings.Contains(text, "#"):
		return "A '#' inside { } starts a comment and swallows the rest of the line.\n" +
			"    quote the value, for example:\n" +
			"      generate: { id: { type: pattern, format: \"ORD-######\" } }"
	}
	return "check that every key has a value and that keys are separated by commas, for example:\n" +
		"      http: { method: GET, path: /orders }"
}
