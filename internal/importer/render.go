package importer

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Every importer ends here: curl, .jmx and whatever comes next build a script
// and the YAML is written in a single place. Two separate writers would mean a
// secret masked on one path and leaking on the other.
type ImportedStep struct {
	Name            string
	Method          string
	Path            string
	Headers         map[string]string
	Body            string
	FollowRedirects bool
}

type ImportedSource struct {
	Name    string
	File    string
	Consume string
}

type Script struct {
	Name      string
	Target    string
	Data      []ImportedSource
	Steps     []ImportedStep
	Phases    []string
	Warnings  []string
	Descartes []string
}

// A credential header never reaches the YAML: the generated file goes to the
// repository, and a committed token is the most common way to leak one.
var secretHeaders = map[string]string{
	"authorization": "TOKEN",
	"x-api-key":     "API_KEY",
	"api-key":       "API_KEY",
	"cookie":        "COOKIE",
}

func RenderYAML(script Script) Import {
	importResult := Import{Warnings: append([]string{}, script.Warnings...)}
	var lines []string
	write := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	steps := make([]ImportedStep, len(script.Steps))
	copy(steps, script.Steps)
	vars := map[string]string{}
	for index := range steps {
		withoutSecret := map[string]string{}
		for name, value := range steps[index].Headers {
			variable, secret := secretHeaders[strings.ToLower(name)]
			if !secret {
				withoutSecret[name] = value
				continue
			}
			local := strings.ToLower(variable)
			withoutSecret[name] = credentialPrefix(value) + "${" + local + "}"
			if _, jaAvisado := vars[local]; !jaAvisado {
				vars[local] = variable
				importResult.Warnings = append(importResult.Warnings,
					fmt.Sprintf("o cabecalho %s virou ${%s}: rode com %s=... no ambiente, para nao versionar credencial", name, local, variable))
			}
		}
		steps[index].Headers = withoutSecret
	}

	write("# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json")
	write("nome: %q", script.Name)
	write("alvo: ${ALVO:-%s}", script.Target)
	write("")

	if len(vars) > 0 {
		write("variaveis:")
		var declarations []string
		for local, environment := range vars {
			declarations = append(declarations, fmt.Sprintf("  %s: ${%s}", local, environment))
		}
		sort.Strings(declarations)
		lines = append(lines, declarations...)
		write("")
	}

	if len(script.Data) > 0 {
		write("dados:")
		for _, source := range script.Data {
			consume := source.Consume
			if consume == "" {
				consume = "circular"
			}
			write("  %s: { arquivo: %s, consumo: %s }", source.Name, source.File, consume)
		}
		write("")
	}

	write("carga:")
	write("  perfis:")
	phases := script.Phases
	if len(phases) == 0 {
		phases = []string{
			"    - rampa: { de: 1/s, ate: 20/s, durante: 30s }",
			"    - patamar: { taxa: 20/s, durante: 1m }",
		}
	}
	lines = append(lines, phases...)
	write("")
	write("cenario:")

	for _, step := range steps {
		simple := step.Body == "" && len(step.Headers) == 0 && !step.FollowRedirects
		if simple {
			write("  - http: %s %s", step.Method, step.Path)
			write("    nome: %s", step.Name)
		} else {
			write("  - nome: %s", step.Name)
			write("    http:")
			write("      metodo: %s", step.Method)
			write("      caminho: %s", step.Path)
			if len(step.Headers) > 0 {
				write("      cabecalhos:")
				names := make([]string, 0, len(step.Headers))
				for name := range step.Headers {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					write("        %s: %q", name, step.Headers[name])
				}
			}
			if step.Body != "" {
				write("      corpo: %s", inlineBody(step.Body))
			}
			if step.FollowRedirects {
				write("      seguir_redirect: true")
			}
		}
		write("    verificar:")
		write("      status: 200")
	}

	write("")
	write("slo:")
	for _, step := range steps {
		write("  - %s: { p95: < 500ms }", step.Name)
	}
	write("  - global: { erros: < 1 }")

	importResult.Warnings = append(importResult.Warnings,
		"os numeros de carga e de slo sao um chute de partida, nao uma medicao: ajuste antes de usar como gate")
	importResult.YAML = strings.Join(lines, "\n") + "\n"
	return importResult
}

func credentialPrefix(value string) string {
	if parts := strings.SplitN(value, " ", 2); len(parts) == 2 && !strings.Contains(parts[0], "=") {
		return parts[0] + " "
	}
	return ""
}

func inlineBody(body string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
	return "'" + strings.ReplaceAll(clean, "'", "''") + "'"
}

// Identifiers and version prefixes stay out of the step name: the name is the
// report's aggregation key, and a name per id would give one row per request
// instead of one row per operation.
func resource(path string) string {
	semConsulta, _, _ := strings.Cut(path, "?")
	var parts []string
	for _, part := range strings.Split(semConsulta, "/") {
		if part == "" || looksLikeIdentifier(part) || looksLikeVersion(part) {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "raiz"
	}
	return strings.Join(parts, " ")
}

func looksLikeVersion(part string) bool {
	if len(part) < 2 || (part[0] != 'v' && part[0] != 'V') {
		return false
	}
	for _, char := range part[1:] {
		if !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func looksLikeIdentifier(part string) bool {
	digits := 0
	for _, char := range part {
		if unicode.IsDigit(char) {
			digits++
		}
	}
	return digits >= 3 || len(part) >= 16
}

func hasIdentifier(path string) bool {
	semConsulta, _, _ := strings.Cut(path, "?")
	for _, part := range strings.Split(semConsulta, "/") {
		if part != "" && looksLikeIdentifier(part) {
			return true
		}
	}
	return false
}
