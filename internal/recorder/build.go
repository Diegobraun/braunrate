package recorder

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/internal/importer"
)

type DataFile struct {
	Name   string
	Values []string
}

type group struct {
	method  string
	subject string
	entries []Entry
}

// Build turns what passed through the proxy into a scenario. Nothing here is
// read from the wire: grouping, correlation and data are inferences, and each
// one that reaches the file is marked as a suggestion.
func Build(entries []Entry, dataPrefix string) (importer.Script, []DataFile) {
	groups := groupByRoute(entries)
	if len(groups) == 0 {
		return importer.Script{}, nil
	}

	target := "http://" + groups[0].entries[0].URL.Host
	if groups[0].entries[0].URL.Scheme != "" {
		target = groups[0].entries[0].URL.Scheme + "://" + groups[0].entries[0].URL.Host
	}

	script := importer.Script{
		Name:   "Gravado de " + hostOnly(groups[0].entries[0].URL.Host),
		Target: target,
	}

	captures, substitutions := correlate(groups)
	values := map[string][]string{}
	var order []string

	for index, current := range groups {
		representative := current.entries[0]
		path, used := templatePath(current, substitutions[index], values, &order)

		headers := map[string]string{}
		for name, value := range representative.Headers {
			headers[name] = apply(value, substitutions[index])
		}
		body := apply(representative.Body, substitutions[index])

		script.Steps = append(script.Steps, importer.ImportedStep{
			Name:           current.method + " " + current.subject,
			Method:         strings.ToUpper(current.method),
			Path:           path,
			Headers:        headers,
			Body:           body,
			ExpectedStatus: representative.Status,
			Captures:       captures[index],
		})
		script.Warnings = append(script.Warnings, used...)
	}

	var files []DataFile
	for _, name := range order {
		files = append(files, DataFile{Name: name, Values: values[name]})
		script.Data = append(script.Data, importer.ImportedSource{
			Name: name, File: fmt.Sprintf("%s-%s.csv", dataPrefix, strings.ReplaceAll(name, "_", "-")), Consume: "circular",
		})
		if len(values[name]) < 2 {
			script.Warnings = append(script.Warnings, fmt.Sprintf(
				"a fonte %q ficou com um valor so: com um valor o alvo responde de cache e o numero fica otimista. Grave mais navegacao ou troque por um bloco 'gerar'", name))
		}
	}

	script.Warnings = append(script.Warnings,
		"a sequencia gravada e uma passagem so: o mix de producao tem outras proporcoes entre as rotas, e nenhuma medicao aqui sabe disso")
	return script, files
}

// Two requests to the same route are the same step: keeping one step per
// identifier would give one line per request in the report instead of one line
// per operation.
func groupByRoute(entries []Entry) []group {
	var groups []group
	position := map[string]int{}
	for _, entry := range entries {
		method := strings.ToLower(entry.Method)
		key := method + " " + importer.Resource(entry.URL.Path)
		index, seen := position[key]
		if !seen {
			position[key] = len(groups)
			groups = append(groups, group{method: method, subject: importer.Resource(entry.URL.Path), entries: []Entry{entry}})
			continue
		}
		groups[index].entries = append(groups[index].entries, entry)
	}
	return groups
}

type substitution struct {
	value    string
	variable string
}

// Without correlation the recorded scenario breaks on its second run, which is
// exactly what a JMeter recording does: it replays an identifier that only
// existed in the session that was recorded.
func correlate(groups []group) (map[int][]importer.ImportedCapture, map[int][]substitution) {
	captures := map[int][]importer.ImportedCapture{}
	substitutions := map[int][]substitution{}
	claimed := map[string]bool{}

	for producer := range groups {
		response := groups[producer].entries[0]
		for _, candidate := range candidatesOf(response) {
			if claimed[candidate.value] {
				continue
			}
			var consumers []int
			for consumer := producer + 1; consumer < len(groups); consumer++ {
				if uses(groups[consumer].entries[0], candidate.value) {
					consumers = append(consumers, consumer)
				}
			}
			if len(consumers) == 0 {
				continue
			}
			claimed[candidate.value] = true
			variable := variableName(candidate.name, groups[producer].subject, captures)
			captures[producer] = append(captures[producer], importer.ImportedCapture{
				Variable: variable, Expression: candidate.path, Suggested: true,
			})
			for _, consumer := range consumers {
				substitutions[consumer] = append(substitutions[consumer], substitution{candidate.value, variable})
			}
		}
	}
	return captures, substitutions
}

type candidate struct {
	path  string
	name  string
	value string
}

// A session cookie is the most common correlation of a web application, and it
// is born in a response header instead of a body: reading only the body means
// every recorded journey reuses the one session that was recorded.
func candidatesOf(response Entry) []candidate {
	found := cookieCandidates(response)
	if strings.Contains(strings.ToLower(response.ContentType), "json") {
		found = append(found, flatten(response.ResponseBody)...)
	}
	return found
}

func cookieCandidates(response Entry) []candidate {
	var found []candidate
	for _, header := range response.ResponseCookies {
		pair, _, _ := strings.Cut(header, ";")
		name, value, split := strings.Cut(strings.TrimSpace(pair), "=")
		if !split || value == "" {
			continue
		}
		found = append(found, candidate{path: "cookie:" + name, name: name, value: value})
	}
	return found
}

// Only leaf strings and numbers become candidates, and only from a certain
// length up: correlating "1" or "ok" would rewrite half the scenario by chance.
func flatten(body []byte) []candidate {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	var found []candidate
	var walk func(path string, node any)
	walk = func(path string, node any) {
		switch typed := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(path+"."+key, typed[key])
			}
		case []any:
			for index, item := range typed {
				if index > 2 {
					return
				}
				walk(fmt.Sprintf("%s.%d", path, index), item)
			}
		case string:
			if len(typed) >= 4 {
				found = append(found, candidate{path: path, name: leaf(path), value: typed})
			}
		case float64:
			text := strings.TrimSuffix(fmt.Sprintf("%v", typed), ".0")
			if len(text) >= 4 {
				found = append(found, candidate{path: path, name: leaf(path), value: text})
			}
		}
	}
	walk("$", decoded)
	return found
}

func uses(entry Entry, value string) bool {
	for _, segment := range strings.Split(entry.URL.Path, "/") {
		if segment == value {
			return true
		}
	}
	for _, values := range entry.URL.Query() {
		for _, item := range values {
			if item == value {
				return true
			}
		}
	}
	for _, header := range entry.Headers {
		if strings.Contains(header, value) {
			return true
		}
	}
	return entry.Body != "" && strings.Contains(entry.Body, value)
}

func leaf(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

func variableName(candidateName, subject string, taken map[int][]importer.ImportedCapture) string {
	name := sanitize(candidateName)
	if name == "" {
		name = "valor"
	}
	for _, captures := range taken {
		for _, capture := range captures {
			if capture.Variable == name {
				return sanitize(subject) + "_" + name
			}
		}
	}
	return name
}

func apply(text string, substitutions []substitution) string {
	for _, item := range substitutions {
		text = strings.ReplaceAll(text, item.value, "${"+item.variable+"}")
	}
	return text
}

// The path is the one place where a whole segment has to match: replacing a
// substring here would break /pedidos/12 into /pe${id}dos/12.
func templatePath(current group, substitutions []substitution, values map[string][]string, order *[]string) (string, []string) {
	representative := current.entries[0]
	segments := strings.Split(representative.URL.Path, "/")
	var warnings []string

	for index, segment := range segments {
		if segment == "" {
			continue
		}
		if variable, correlated := correlatedSegment(segment, substitutions); correlated {
			segments[index] = "${" + variable + "}"
			continue
		}
		if !importer.LooksLikeIdentifier(segment) {
			continue
		}
		name := parameterName(segments, index)
		segments[index] = "${" + name + ".valor}"
		if _, known := values[name]; !known {
			*order = append(*order, name)
		}
		values[name] = merge(values[name], observed(current, index))
	}

	path := strings.Join(segments, "/")
	if representative.URL.RawQuery != "" {
		path += "?" + apply(representative.URL.RawQuery, substitutions)
		warnings = append(warnings, fmt.Sprintf(
			"o passo %q ficou com a query da gravacao: se ela tiver identificador, troque por ${dados.coluna} tambem",
			current.method+" "+current.subject))
	}
	return path, warnings
}

func correlatedSegment(segment string, substitutions []substitution) (string, bool) {
	for _, item := range substitutions {
		if item.value == segment {
			return item.variable, true
		}
	}
	return "", false
}

func parameterName(segments []string, index int) string {
	for back := index - 1; back >= 0; back-- {
		if segments[back] != "" && !importer.LooksLikeIdentifier(segments[back]) {
			return sanitize(segments[back]) + "_id"
		}
	}
	return "id"
}

func observed(current group, index int) []string {
	var found []string
	for _, entry := range current.entries {
		segments := strings.Split(entry.URL.Path, "/")
		if index < len(segments) && segments[index] != "" {
			found = append(found, segments[index])
		}
	}
	return found
}

func merge(existing, incoming []string) []string {
	seen := map[string]bool{}
	for _, value := range existing {
		seen[value] = true
	}
	for _, value := range incoming {
		if seen[value] {
			continue
		}
		seen[value] = true
		existing = append(existing, value)
	}
	return existing
}

func sanitize(text string) string {
	var out strings.Builder
	for _, char := range strings.ToLower(text) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			out.WriteRune(char)
		case char == ' ' || char == '-' || char == '_':
			out.WriteRune('_')
		}
	}
	return strings.Trim(out.String(), "_")
}

func (file DataFile) CSV() string {
	lines := append([]string{"valor"}, file.Values...)
	return strings.Join(lines, "\n") + "\n"
}
