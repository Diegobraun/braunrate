package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const schemaPath = "docs/braunrate.schema.json"

// The schema is what the editor already uses for autocomplete, and a test
// keeps it in step with the parser. Writing the reference by hand would create
// a third list of keys, and the third list is always the one that ages.
type schemaNode struct {
	Type        jsonType              `json:"type"`
	Description string                `json:"description"`
	Enum        []any                 `json:"enum"`
	Examples    []any                 `json:"examples"`
	Required    []string              `json:"required"`
	Properties  map[string]schemaNode `json:"properties"`
	Items       *schemaNode           `json:"items"`
	Reference   string                `json:"$ref"`
	OneOf       []schemaNode          `json:"oneOf"`
	AnyOf       []schemaNode          `json:"anyOf"`
	Definitions map[string]schemaNode `json:"$defs"`
	Additional  *additional           `json:"additionalProperties"`
	Default     any                   `json:"default"`
}

type additional struct{ shape *schemaNode }

func (extra *additional) UnmarshalJSON(data []byte) error {
	var closed bool
	if err := json.Unmarshal(data, &closed); err == nil {
		return nil
	}
	var shape schemaNode
	if err := json.Unmarshal(data, &shape); err != nil {
		return err
	}
	extra.shape = &shape
	return nil
}

// No JSON Schema "type" e um texto ou uma lista deles.
type jsonType struct{ names []string }

func (kind *jsonType) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		kind.names = []string{single}
		return nil
	}
	return json.Unmarshal(data, &kind.names)
}

func (kind jsonType) String() string {
	if len(kind.names) == 0 {
		return ""
	}
	return kind.names[0]
}

func ReferencePage(repositoryRoot string, language Language) (Page, error) {
	root, err := readSchema(repositoryRoot)
	if err != nil {
		return Page{}, err
	}
	text := language.Text
	var markdown strings.Builder
	fmt.Fprintf(&markdown, "# %s\n\n%s\n\n", text.ReferenceTitle, text.ReferenceIntro)
	fmt.Fprintf(&markdown, "## %s\n\n%s\n\n```yaml\n%s```\n\n",
		text.ReferenceWhole, text.ReferenceWholeIntro, completeScenario)
	writeBlock(&markdown, root, root, text.ReferenceTop, 2, text)
	for _, name := range sortedNames(root.Definitions) {
		definition := root.Definitions[name]
		if len(shapeProperties(definition)) == 0 {
			continue
		}
		writeBlock(&markdown, root, definition, "`"+name+"`", 2, text)
	}
	return Page{Slug: "reference", Title: text.ReferenceTitle, Section: text.Sections["reference"],
		Summary:  text.ReferenceSummary,
		Markdown: markdown.String(), Source: schemaPath}, nil
}

func readSchema(repositoryRoot string) (schemaNode, error) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot, schemaPath))
	if err != nil {
		return schemaNode{}, fmt.Errorf("could not read the schema: %w", err)
	}
	var root schemaNode
	if err := json.Unmarshal(content, &root); err != nil {
		return schemaNode{}, fmt.Errorf("the schema does not load: %w", err)
	}
	return root, nil
}

func writeBlock(out *strings.Builder, root, node schemaNode, path string, level int, text chrome) {
	properties := shapeProperties(node)
	fmt.Fprintf(out, "%s %s\n\n", strings.Repeat("#", level), path)
	if node.Description != "" {
		fmt.Fprintf(out, "%s\n\n", node.Description)
	}
	if len(properties) == 0 {
		return
	}

	fmt.Fprintf(out, "%s\n|---|---|---|---|---|\n", text.ReferenceColumns)
	for _, name := range sortedNames(properties) {
		property := resolve(root, properties[name])
		fmt.Fprintf(out, "| `%s` | %s | %s | %s | %s |\n",
			name, kindOf(root, property, text)+fallback(property, text), required(node, name, text),
			cell(property.Description), examplesOf(property))
	}
	out.WriteString("\n")

	// Chave declarada dentro do proprio campo nao tem secao para onde apontar:
	// sem descer nela, "durante" e "ca" ficariam de fora da referencia.
	for _, name := range sortedNames(properties) {
		for _, nested := range inlineShapes(properties[name]) {
			writeBlock(out, root, nested, path+"."+name, min(level+1, 5), text)
		}
	}
}

func shapeProperties(node schemaNode) map[string]schemaNode {
	if len(node.Properties) > 0 {
		return node.Properties
	}
	merged := map[string]schemaNode{}
	for _, shape := range inlineShapes(node) {
		for name, property := range shape.Properties {
			merged[name] = property
		}
	}
	return merged
}

func inlineShapes(node schemaNode) []schemaNode {
	if node.Reference != "" {
		return nil
	}
	var shapes []schemaNode
	if len(node.Properties) > 0 {
		shapes = append(shapes, node)
	}
	if node.Items != nil {
		shapes = append(shapes, inlineShapes(*node.Items)...)
	}
	if node.Additional != nil && node.Additional.shape != nil {
		shapes = append(shapes, inlineShapes(*node.Additional.shape)...)
	}
	for _, branch := range append(slices.Clone(node.OneOf), node.AnyOf...) {
		shapes = append(shapes, inlineShapes(branch)...)
	}
	return shapes
}

func resolve(root, node schemaNode) schemaNode {
	if node.Reference == "" {
		return node
	}
	name := strings.TrimPrefix(node.Reference, "#/$defs/")
	target, found := root.Definitions[name]
	if !found {
		return node
	}
	if node.Description != "" {
		target.Description = node.Description
	}
	if len(node.Examples) > 0 {
		target.Examples = node.Examples
	}
	if node.Default != nil {
		target.Default = node.Default
	}
	return target
}

// O padrao ao lado do tipo, e nao numa coluna propria: a maioria das chaves nao
// tem um, e uma coluna quase vazia empurra a descricao para fora da tela.
func fallback(node schemaNode, text chrome) string {
	if node.Default == nil {
		return ""
	}
	return " · " + text.ReferenceDefault + " `" + literals([]any{node.Default})[0] + "`"
}

func kindOf(root, node schemaNode, text chrome) string {
	switch {
	case len(node.Enum) > 0:
		return "`" + strings.Join(literals(node.Enum), "`"+text.ReferenceEitherOr+"`") + "`"
	case node.Type.String() == "array" && node.Items != nil:
		return text.ReferenceListOf + " " + shapeName(*node.Items, text)
	case node.Type.String() != "":
		return typeName(node.Type.String(), text)
	case len(node.OneOf) > 0 || len(node.AnyOf) > 0:
		return text.ReferenceShort
	}
	return text.ReferenceObject
}

func shapeName(node schemaNode, text chrome) string {
	if node.Reference != "" {
		return "`" + strings.TrimPrefix(node.Reference, "#/$defs/") + "`"
	}
	return typeName(node.Type.String(), text)
}

func typeName(jsonType string, text chrome) string {
	if name, known := text.ReferenceTypes[jsonType]; known {
		return name
	}
	return jsonType
}

func required(parent schemaNode, name string, text chrome) string {
	for _, declared := range parent.Required {
		if declared == name {
			return text.ReferenceRequired[0]
		}
	}
	return text.ReferenceRequired[1]
}

// Ate tres: o primeiro mostra a forma comum, e os outros mostram que a chave
// aceita mais de uma. A quarta linha ja e a referencia inteira dentro de uma
// celula.
func examplesOf(node schemaNode) string {
	if len(node.Examples) == 0 {
		return ""
	}
	written := literals(node.Examples)
	if len(written) > 3 {
		written = written[:3]
	}
	for index, value := range written {
		written[index] = "`" + value + "`"
	}
	return strings.Join(written, " · ")
}

// A formatacao padrao do Go vira map[$.status:PROCESSED], que nao da para colar
// em lugar nenhum, e a do JSON enche a celula de aspas que o YAML nao pede. O
// que sai daqui e YAML de fluxo: cabe em uma linha e cola no cenario como esta.
func literals(values []any) []string {
	written := make([]string, 0, len(values))
	for _, value := range values {
		written = append(written, flow(value))
	}
	return written
}

func flow(value any) string {
	switch typed := value.(type) {
	case string:
		return quoted(typed)
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, quoted(name)+": "+flow(typed[name]))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, flow(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case nil:
		return "null"
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

// Aspas so onde o YAML precisa delas: dentro de um mapa em linha, a virgula e as
// chaves fecham o valor antes da hora, e um texto que parece numero voltaria
// como numero.
func needsQuotes(text string) bool {
	if text == "" || strings.ContainsAny(text, ",{}[]&*!|>%@`\"'") || strings.Contains(text, ": ") ||
		strings.HasPrefix(text, "#") || text != strings.TrimSpace(text) {
		return true
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return true
	}
	switch strings.ToLower(text) {
	case "true", "false", "null", "yes", "no", "on", "off":
		return true
	}
	return false
}

func quoted(text string) string {
	if !needsQuotes(text) {
		return text
	}
	return `"` + strings.ReplaceAll(text, `"`, `\"`) + `"`
}

// Uma barra vertical na descricao partiria a tabela em uma coluna a mais.
func cell(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "|", "\\|"), "\n", " ")
}

func sortedNames(values map[string]schemaNode) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// O cenario que abre a pagina e um cenario de verdade, e nao um recorte: quem
// chega na referencia procurando "como e um arquivo inteiro" nao encontra isso
// em nenhuma tabela de chaves. Um teste o carrega e o valida.
const completeScenario = `# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
name: Billing journey
target: ${TARGET:-http://127.0.0.1:8080}

variables:
  tenant: acme

auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: ana, password: "${PASSWORD}" } }
    capture: { token: $.access_token }
  refreshAfter: 25m

data:
  subscribers: { file: data/subscribers.csv, consume: circular }

load:
  profiles:
    - ramp: { from: 50/s, to: 300/s, duration: 5s }
    - steady: { rate: 300/s, duration: 30s }

scenario:
  - name: look up order
    http:
      method: GET
      path: /orders/${subscribers.id}
      headers: { X-Tenant: "${tenant}" }
    expect: { status: 200, json: { $.status: OPEN } }
    capture: { invoiceId: $.lastInvoice.id }

  - name: pay invoice
    http:
      method: POST
      path: /invoices/${invoiceId}/pay
      body: { amount: 199.90 }
    expect: { status: 200 }

slo:
  - look up order: { p95: "< 150ms" }
  - journey: { p95: "< 2s" }
  - global: { errors: "< 0.1" }
`
