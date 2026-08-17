package importer

import (
	"fmt"
	"regexp"
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
	ExpectedStatus  int
	Captures        []ImportedCapture
	GraphQL         *ImportedGraphQL
}

// A GraphQL step is not an HTTP step with a body: the report aggregates it by
// operation, and every operation of a service arrives at the same address. As
// an HTTP step the cheapest query and the most expensive mutation share a row.
type ImportedGraphQL struct {
	Operation string
	Query     string
	Variables string
	Path      string
}

// Suggested marks what was inferred rather than read: the comment goes into the
// file so whoever opens it knows which line the tool guessed.
type ImportedCapture struct {
	Variable   string
	Expression string
	Suggested  bool
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
}

// Cookie is masked pair by pair, not as one string: masking the whole header
// would throw away the session the recorder just correlated, and keeping the
// whole header would version the cookies it did not correlate.
func maskCookies(header string, vars map[string]string) (string, []string) {
	var notices []string
	pairs := strings.Split(header, ";")
	for index, pair := range pairs {
		pairs[index] = strings.TrimSpace(pair)
		name, value, split := strings.Cut(pairs[index], "=")
		if !split || strings.HasPrefix(value, "${") {
			continue
		}
		local := "cookie_" + strings.ToLower(sanitizeName(name))
		environment := strings.ToUpper(local)
		if _, announced := vars[local]; !announced {
			vars[local] = environment
			notices = append(notices, fmt.Sprintf(
				"the cookie %q was not born in any recorded response and became ${%s}: run with %s=... in the environment, so a session does not get versioned",
				name, local, environment))
		}
		pairs[index] = name + "=${" + local + "}"
	}
	return strings.Join(pairs, "; "), notices
}

// The GraphQL step goes into the file without a name so the report keys it by
// operation, which is what makes one row per operation instead of one row per
// address. The SLO has to point at that same key.
func reportKey(step ImportedStep) string {
	if step.GraphQL != nil {
		return "graphql " + step.GraphQL.Operation
	}
	return step.Name
}

func sortedNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sanitizeName(text string) string {
	var out strings.Builder
	for _, char := range strings.ToLower(text) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			continue
		}
		out.WriteRune('_')
	}
	return strings.Trim(out.String(), "_")
}

// The body leaks the same way the header does: a recorded login carries the
// password in plain text, and the generated file goes to the repository.
var secretFields = map[string]string{
	"senha": "SENHA", "password": "SENHA", "pwd": "SENHA", "passwd": "SENHA",
	"secret": "SEGREDO", "client_secret": "SEGREDO", "clientsecret": "SEGREDO",
	"token": "TOKEN", "access_token": "TOKEN", "refresh_token": "TOKEN",
	"apikey": "API_KEY", "api_key": "API_KEY", "authorization": "TOKEN",
}

var jsonField = regexp.MustCompile(`"([A-Za-z_][A-Za-z0-9_-]*)"\s*:\s*"([^"]*)"`)

func maskBody(body string, vars map[string]string) (string, []string) {
	var notices []string
	masked := jsonField.ReplaceAllStringFunc(body, func(occurrence string) string {
		parts := jsonField.FindStringSubmatch(occurrence)
		variable, secret := secretFields[strings.ToLower(parts[1])]
		if !secret || parts[2] == "" || strings.HasPrefix(parts[2], "${") {
			return occurrence
		}
		local := strings.ToLower(variable)
		if _, announced := vars[local]; !announced {
			vars[local] = variable
			notices = append(notices,
				fmt.Sprintf("the body field %q became ${%s}: run with %s=... in the environment, so a credential does not get versioned", parts[1], local, variable))
		}
		return fmt.Sprintf("%q: %q", parts[1], "${"+local+"}")
	})
	return masked, notices
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
			if strings.EqualFold(name, "Cookie") {
				masked, notices := maskCookies(value, vars)
				withoutSecret[name] = masked
				importResult.Warnings = append(importResult.Warnings, notices...)
				continue
			}
			variable, secret := secretHeaders[strings.ToLower(name)]
			if !secret || alreadyInterpolated(value) {
				withoutSecret[name] = value
				continue
			}
			local := strings.ToLower(variable)
			withoutSecret[name] = credentialPrefix(value) + "${" + local + "}"
			if _, announced := vars[local]; !announced {
				vars[local] = variable
				importResult.Warnings = append(importResult.Warnings,
					fmt.Sprintf("the header %s became ${%s}: run with %s=... in the environment, so a credential does not get versioned", name, local, variable))
			}
		}
		steps[index].Headers = withoutSecret

		masked, notices := maskBody(steps[index].Body, vars)
		steps[index].Body = masked
		importResult.Warnings = append(importResult.Warnings, notices...)
	}

	write("# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json")
	write("name: %q", script.Name)
	write("target: ${TARGET:-%s}", script.Target)
	write("")

	if len(vars) > 0 {
		write("variables:")
		var declarations []string
		for local, environment := range vars {
			declarations = append(declarations, fmt.Sprintf("  %s: ${%s}", local, environment))
		}
		sort.Strings(declarations)
		lines = append(lines, declarations...)
		write("")
	}

	if len(script.Data) > 0 {
		write("data:")
		for _, source := range script.Data {
			consume := source.Consume
			if consume == "" {
				consume = "circular"
			}
			write("  %s: { file: %s, consume: %s }", source.Name, source.File, consume)
		}
		write("")
	}

	write("load:")
	write("  profiles:")
	phases := script.Phases
	if len(phases) == 0 {
		phases = []string{
			"    - ramp: { from: 1/s, to: 20/s, duration: 30s }",
			"    - steady: { rate: 20/s, duration: 1m }",
		}
	}
	lines = append(lines, phases...)
	write("")
	write("scenario:")

	for _, step := range steps {
		simple := step.Body == "" && len(step.Headers) == 0 && !step.FollowRedirects
		switch {
		case step.GraphQL != nil:
			write("  - graphql:")
			write("      query: |")
			for _, line := range strings.Split(strings.TrimSpace(step.GraphQL.Query), "\n") {
				write("        %s", line)
			}
			write("      variables: %s", step.GraphQL.Variables)
			if step.GraphQL.Path != "" && step.GraphQL.Path != "/graphql" {
				write("      path: %s", step.GraphQL.Path)
			}
			if len(step.Headers) > 0 {
				write("      headers:")
				for _, name := range sortedNames(step.Headers) {
					write("        %s: %q", name, step.Headers[name])
				}
			}
		case simple:
			write("  - http: %s %s", step.Method, step.Path)
			write("    name: %s", step.Name)
		default:
			write("  - name: %s", step.Name)
			write("    http:")
			write("      method: %s", step.Method)
			write("      path: %s", step.Path)
			if len(step.Headers) > 0 {
				write("      headers:")
				for _, name := range sortedNames(step.Headers) {
					write("        %s: %q", name, step.Headers[name])
				}
			}
			if step.Body != "" {
				write("      body: %s", inlineBody(step.Body))
			}
			if step.FollowRedirects {
				write("      followRedirects: true")
			}
		}
		if len(step.Captures) > 0 {
			write("    capture:")
			for _, capture := range step.Captures {
				suffix := ""
				if capture.Suggested {
					suffix = "   # the recorder's guess: check that this is really the value the next call needs"
				}
				write("      %s: %s%s", capture.Variable, capture.Expression, suffix)
			}
		}
		write("    expect:")
		write("      status: %d", statusOr(step.ExpectedStatus))
	}

	write("")
	write("slo:")
	for _, step := range steps {
		write("  - %s: { p95: < 500ms }", reportKey(step))
	}
	write("  - global: { errors: < 1 }")

	importResult.Warnings = append(importResult.Warnings,
		"the load and slo numbers are a starting guess, not a measurement: tune them before using this as a gate")
	importResult.YAML = strings.Join(lines, "\n") + "\n"
	return importResult
}

// A value that is already a reference carries no literal secret, and replacing
// it would throw away a correlation the recorder found.
func alreadyInterpolated(value string) bool {
	rest := strings.TrimSpace(value)
	if prefix := credentialPrefix(rest); prefix != "" {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, prefix))
	}
	return strings.HasPrefix(rest, "${") && strings.HasSuffix(rest, "}")
}

// A recorded 201 becomes verificar: { status: 201 }. Writing 200 for every step
// would fail the scenario on its first run against the very service it was
// recorded from.
func statusOr(status int) int {
	if status == 0 {
		return 200
	}
	return status
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
func Resource(path string) string {
	semConsulta, _, _ := strings.Cut(path, "?")
	var parts []string
	for _, part := range strings.Split(semConsulta, "/") {
		if part == "" || LooksLikeIdentifier(part) || looksLikeVersion(part) {
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

func LooksLikeIdentifier(part string) bool {
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
		if part != "" && LooksLikeIdentifier(part) {
			return true
		}
	}
	return false
}
