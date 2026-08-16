package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	// Um mapa de chaves livres — a regra de SLO por passo, por exemplo — declara
	// a forma do valor aqui, e e onde vivem p95, erros e taxa_efetiva.
	Additional *additional `json:"additionalProperties"`
}

// additionalProperties e "false" ou um schema, e um campo tipado como schema
// engasga no booleano.
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

// "type" is either a string or a list of them in JSON Schema, and the list form
// is what a field that accepts a number or the text of an environment reference
// uses.
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

func ReferencePage(repositoryRoot string) (Page, error) {
	root, err := readSchema(repositoryRoot)
	if err != nil {
		return Page{}, err
	}
	var markdown strings.Builder
	markdown.WriteString(`# Referencia do cenario

Esta pagina e gerada de ` + "`docs/braunrate.schema.json`" + `, o mesmo arquivo que
o seu editor usa para completar as chaves. Chave que o braunrate aceita e nao
aparece aqui reprova o build.

`)
	writeBlock(&markdown, root, root, "Topo do arquivo", 2)
	for _, name := range sortedNames(root.Definitions) {
		definition := root.Definitions[name]
		if len(shapeProperties(definition)) == 0 {
			continue
		}
		writeBlock(&markdown, root, definition, "`"+name+"`", 2)
	}
	return Page{Slug: "referencia", Title: "Referencia do cenario", Markdown: markdown.String()}, nil
}

func readSchema(repositoryRoot string) (schemaNode, error) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot, schemaPath))
	if err != nil {
		return schemaNode{}, fmt.Errorf("nao consegui ler o schema: %w", err)
	}
	var root schemaNode
	if err := json.Unmarshal(content, &root); err != nil {
		return schemaNode{}, fmt.Errorf("o schema nao carrega: %w", err)
	}
	return root, nil
}

func writeBlock(out *strings.Builder, root, node schemaNode, path string, level int) {
	properties := shapeProperties(node)
	fmt.Fprintf(out, "%s %s\n\n", strings.Repeat("#", level), path)
	if node.Description != "" {
		fmt.Fprintf(out, "%s\n\n", node.Description)
	}
	if len(properties) == 0 {
		return
	}

	out.WriteString("| chave | tipo | obrigatoria | o que faz | exemplo |\n|---|---|---|---|---|\n")
	for _, name := range sortedNames(properties) {
		property := resolve(root, properties[name])
		fmt.Fprintf(out, "| `%s` | %s | %s | %s | %s |\n",
			name, kindOf(root, property), required(node, name),
			cell(property.Description), example(property))
	}
	out.WriteString("\n")

	// A chave declarada dentro do proprio campo, sem $ref, nao tem secao para
	// onde apontar: sem descer nela, "durante" e "ca" existiriam no schema, o
	// editor completaria as duas, e a referencia publicada nao teria nenhuma.
	for _, name := range sortedNames(properties) {
		for _, nested := range inlineShapes(properties[name]) {
			writeBlock(out, root, nested, path+"."+name, min(level+1, 5))
		}
	}
}

// As chaves que este bloco aceita, venham elas de properties, de um ramo de
// oneOf ou da forma declarada em additionalProperties.
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

// A $ref with a description of its own keeps it: the reference says what the
// shape is, the field says what it means where it is used.
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
	return target
}

func kindOf(root, node schemaNode) string {
	switch {
	case len(node.Enum) > 0:
		return "`" + strings.Join(literals(node.Enum), "` ou `") + "`"
	case node.Type.String() == "array" && node.Items != nil:
		return "lista de " + shapeName(*node.Items)
	case node.Type.String() != "":
		return translated(node.Type.String())
	case len(node.OneOf) > 0 || len(node.AnyOf) > 0:
		return "forma curta ou objeto"
	}
	return "objeto"
}

func shapeName(node schemaNode) string {
	if node.Reference != "" {
		return "`" + strings.TrimPrefix(node.Reference, "#/$defs/") + "`"
	}
	return translated(node.Type.String())
}

func translated(jsonType string) string {
	switch jsonType {
	case "string":
		return "texto"
	case "integer":
		return "inteiro"
	case "number":
		return "numero"
	case "boolean":
		return "sim ou nao"
	case "object":
		return "objeto"
	case "array":
		return "lista"
	}
	return jsonType
}

func required(parent schemaNode, name string) string {
	for _, declared := range parent.Required {
		if declared == name {
			return "sim"
		}
	}
	return "nao"
}

func example(node schemaNode) string {
	if len(node.Examples) == 0 {
		return ""
	}
	return "`" + literals(node.Examples)[0] + "`"
}

// Um exemplo que e mapa ou lista tem que voltar como a pessoa escreveria. A
// formatacao padrao do Go vira map[$.status:PROCESSADO], que nao e YAML, nao e
// JSON e nao e nada que alguem consiga colar.
func literals(values []any) []string {
	written := make([]string, 0, len(values))
	for _, value := range values {
		switch value.(type) {
		case string, float64, bool, nil:
			written = append(written, strings.TrimSpace(fmt.Sprintf("%v", value)))
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			encoded = []byte(fmt.Sprintf("%v", value))
		}
		written = append(written, string(encoded))
	}
	return written
}

// A description with a pipe in it would silently split the Markdown table into
// an extra column, and the reader would see a truncated sentence.
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
