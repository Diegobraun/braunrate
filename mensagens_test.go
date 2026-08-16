package braunrate_test

import (
	"encoding/json"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A mensagem que ensina e a mensagem que a pessoa copia sao a mesma linha, e a
// varredura de acentuacao passou por cima disso: o erro de rampa ensinava
// `ate:` escrito `até:`, que o parser recusa. A fronteira da decisao 9 vale
// tambem dentro da frase, e aqui ela e conferida contra o esquema publicado —
// que e a mesma lista de chaves que o editor autocompleta.
func TestNoMessageTeachesAnAccentedKey(t *testing.T) {
	keys := schemaKeys(t)

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, literal := range stringLiterals(t, path) {
			for _, word := range keysTaughtBy(literal.text) {
				plain := withoutAccents(word)
				if word == plain || !keys[strings.ToLower(plain)] {
					continue
				}
				t.Errorf("%s:%d ensina %q, e o parser aceita %q: quem copiar a mensagem toma outro erro",
					path, literal.line, word, plain)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("não consegui percorrer o repositório: %v", err)
	}
}

type literal struct {
	text string
	line int
}

func stringLiterals(t *testing.T, path string) []literal {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("não consegui ler %s: %v", path, err)
	}
	set := token.NewFileSet()
	file := set.AddFile(path, set.Base(), len(content))
	var source scanner.Scanner
	source.Init(file, content, nil, 0)

	var found []literal
	for {
		position, tok, text := source.Scan()
		if tok == token.EOF {
			return found
		}
		if tok != token.STRING {
			continue
		}
		unquoted, err := strconv.Unquote(text)
		if err != nil {
			continue
		}
		found = append(found, literal{text: unquoted, line: set.Position(position).Line})
	}
}

// Uma chave dentro de exemplo aparece de dois jeitos: num trecho de YAML, que
// traz `{` ou comeca comentado, e numa enumeracao das chaves aceitas, que vem
// entre parenteses depois de "use" ou depois de "available:". Nome de bloco no
// meio de uma frase — "erro no cenário:" — nao e exemplo, e continua acentuado.
var (
	insideYAML  = regexp.MustCompile(`(?:^|[{,#]\s*|\n\s*)([\p{L}_]+)\s*:`)
	acceptedSet = regexp.MustCompile(`(?:\(use |dispon[ií]veis: )([^)\n]+)\)?`)
	enumeration = regexp.MustCompile(`^[\p{L}_0-9.]+(?:(?:, | ou )[\p{L}_0-9.]+)*$`)
	listedKey   = regexp.MustCompile(`[\p{L}_]+`)
)

func keysTaughtBy(text string) []string {
	var taught []string
	if strings.Contains(text, "{") || strings.HasPrefix(strings.TrimSpace(text), "#") {
		for _, match := range insideYAML.FindAllStringSubmatch(text, -1) {
			taught = append(taught, match[1])
		}
	}
	for _, match := range acceptedSet.FindAllStringSubmatch(text, -1) {
		list := strings.TrimSuffix(strings.TrimSpace(match[1]), ")")
		if !enumeration.MatchString(list) {
			continue
		}
		taught = append(taught, listedKey.FindAllString(list, -1)...)
	}
	return taught
}

func schemaKeys(t *testing.T) map[string]bool {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("não consegui ler o esquema publicado: %v", err)
	}
	var document any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o esquema publicado não é JSON: %v", err)
	}
	keys := map[string]bool{}
	collectProperties(document, keys)
	if len(keys) < 50 {
		t.Fatalf("o esquema rendeu %d chaves; esperava a lista inteira do formato", len(keys))
	}
	return keys
}

func collectProperties(node any, into map[string]bool) {
	switch value := node.(type) {
	case map[string]any:
		for name, child := range value {
			if name == "properties" {
				if properties, ok := child.(map[string]any); ok {
					for key := range properties {
						into[key] = true
					}
				}
			}
			collectProperties(child, into)
		}
	case []any:
		for _, child := range value {
			collectProperties(child, into)
		}
	}
}

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "Á", "A", "Â", "A", "Ã", "A",
	"é", "e", "ê", "e", "É", "E", "Ê", "E",
	"í", "i", "Í", "I", "ó", "o", "ô", "o", "õ", "o", "Ó", "O", "Ô", "O", "Õ", "O",
	"ú", "u", "ü", "u", "Ú", "U", "ç", "c", "Ç", "C",
)

func withoutAccents(word string) string { return accents.Replace(word) }
