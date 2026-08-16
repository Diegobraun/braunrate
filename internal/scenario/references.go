package scenario

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// A ${referencia} that resolves from nowhere used to become an empty string in
// silence: the request went out with an empty field, the target answered 401 or
// 404, and nothing in the output connected the two. Refusing at validation is
// the only moment where the person is still looking at the file.
func checkReferences(document *yaml.Node, spec *Spec) error {
	known := knownVariables(*spec)
	missing := map[string]bool{}
	err := walkScalars(document, func(node *yaml.Node) error {
		for _, used := range referencesIn(node.Value) {
			if err := known.resolve(used, node); err != nil {
				return err
			}
			// Only a name the environment has to supply counts here: a dotted
			// one comes from a data source and a known one from the file.
			if !used.hasDefault && environmentName.MatchString(used.name) && !defined(used.name) {
				missing[used.name] = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for name := range missing {
		spec.MissingEnvironment = append(spec.MissingEnvironment, name)
	}
	sort.Strings(spec.MissingEnvironment)
	return nil
}

func defined(name string) bool {
	_, exists := os.LookupEnv(name)
	return exists
}

type reference struct {
	name       string
	hasDefault bool
	offset     int
}

func referencesIn(text string) []reference {
	if !strings.Contains(text, "${") {
		return nil
	}
	var found []reference
	for _, match := range varPattern.FindAllStringSubmatchIndex(text, -1) {
		found = append(found, reference{
			name:       text[match[2]:match[3]],
			hasDefault: match[4] >= 0,
			offset:     match[0],
		})
	}
	return found
}

type variableScope struct {
	names   map[string]bool
	sources map[string]DataSource
}

func knownVariables(spec Spec) variableScope {
	known := variableScope{names: map[string]bool{}, sources: map[string]DataSource{}}
	for name := range spec.Vars {
		known.names[name] = true
	}
	for _, source := range spec.Data {
		known.sources[source.Name] = source
	}
	for _, step := range spec.Steps {
		for _, capture := range step.Captures {
			known.names[capture.Variable] = true
		}
	}
	if spec.Auth != nil && spec.Auth.Obtain != nil {
		for _, capture := range spec.Auth.Obtain.Captures {
			known.names[capture.Variable] = true
		}
	}
	return known
}

func (known variableScope) resolve(used reference, node *yaml.Node) error {
	if used.hasDefault || fromEnvironment(used.name) || known.names[used.name] {
		return nil
	}
	if source, field, dotted := strings.Cut(used.name, "."); dotted {
		return known.resolveFromSource(source, field, used, node)
	}
	return referenceError(node, used, fmt.Sprintf("nao sei de onde vem ${%s}.\n%s", used.name, known.wherePeopleDeclare(used.name)))
}

// Case is what tells the two apart, and it is the convention the tool already
// follows everywhere it writes a scenario: ${API_KEY} comes from the
// environment, ${faturaId} comes from the file. Checking the environment
// instead would make 'validate' impossible on a machine without the secret,
// which is exactly where someone checks a scenario before committing it.
var environmentName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func fromEnvironment(name string) bool {
	return environmentName.MatchString(name) || defined(name)
}

func (known variableScope) resolveFromSource(source, field string, used reference, node *yaml.Node) error {
	declared, exists := known.sources[source]
	if !exists {
		return referenceError(node, used, fmt.Sprintf("${%s} pede o campo %q da fonte de dados %q, e nenhuma fonte com esse nome foi declarada.\n%s",
			used.name, field, source, known.availableSources(source)))
	}
	// A CSV brings its columns from the file, which is not open at this point;
	// only a synthetic source declares its fields here.
	if declared.Synthetic() && len(declared.Fields) > 0 && !hasField(declared.Fields, field) {
		return referenceError(node, used, fmt.Sprintf("a fonte %q nao gera o campo %q.\n%s",
			source, field, suggest(field, sortedFields(declared.Fields))))
	}
	return nil
}

// A close name means a typo, and repeating the four ways to declare a variable
// under a correct guess buries the one line that solves it.
func (known variableScope) wherePeopleDeclare(name string) string {
	available := known.declaredNames()
	if len(available) == 0 {
		return declarationForms(name)
	}
	if _, isTypo := closest(name, available); isTypo {
		return suggest(name, available)
	}
	return suggest(name, available) + "\n" + declarationForms(name)
}

func declarationForms(name string) string {
	return fmt.Sprintf("    declare de onde ela vem:\n"+
		"      variaveis: { %s: valor }                 # fixa no cenario\n"+
		"      variaveis: { %s: \"${%s:-reserva}\" }   # do ambiente, com reserva\n"+
		"      captura: { %s: $.campo }                 # de uma resposta anterior\n"+
		"      dados: { pedidos: { arquivo: dados.csv } }  # e entao ${pedidos.%s}\n"+
		"    nome em CAIXA ALTA vem do ambiente sem precisar declarar: ${%s}",
		name, name, strings.ToUpper(name), name, name, strings.ToUpper(name))
}

func (known variableScope) declaredNames() []string {
	names := make([]string, 0, len(known.names)+len(known.sources))
	for name := range known.names {
		names = append(names, name)
	}
	for name, source := range known.sources {
		for field := range source.Fields {
			names = append(names, name+"."+field)
		}
		if len(source.Fields) == 0 {
			names = append(names, name+".<coluna do csv>")
		}
	}
	sort.Strings(names)
	return names
}

func (known variableScope) availableSources(received string) string {
	names := make([]string, 0, len(known.sources))
	for name := range known.sources {
		names = append(names, name)
	}
	if len(names) == 0 {
		return "    nenhuma fonte de dados foi declarada; o bloco 'dados' e onde elas entram"
	}
	sort.Strings(names)
	return suggest(received, names)
}

func hasField(fields map[string]Generator, name string) bool {
	_, exists := fields[name]
	return exists
}

func sortedFields(fields map[string]Generator) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// The column of the reference inside the scalar, not the column of the scalar:
// a line with three references has to point at the one that failed. A quoted
// scalar starts one character after the position yaml reports, and a value
// broken across lines gives up and points at the start.
func referenceError(node *yaml.Node, used reference, message string) error {
	line, column := node.Line, node.Column
	if !strings.Contains(node.Value[:used.offset], "\n") {
		column += used.offset
		if node.Style == yaml.SingleQuotedStyle || node.Style == yaml.DoubleQuotedStyle {
			column++
		}
	}
	return ScenarioError{Line: line, Column: column, Message: message}
}

func walkScalars(node *yaml.Node, visit func(*yaml.Node) error) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		return visit(node)
	}
	for _, child := range node.Content {
		if err := walkScalars(child, visit); err != nil {
			return err
		}
	}
	return nil
}
