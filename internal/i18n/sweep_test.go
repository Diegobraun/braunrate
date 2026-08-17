// Package i18n holds no code: it holds the check that the format, the commands
// and everything the tool prints stayed in English after ADR 0019. A single
// Portuguese word left in a message is invisible to whoever wrote it and is the
// first thing the reader it was translated for runs into.
package i18n_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Whole words, not fragments: "do" and "no" are Portuguese and English at once,
// and a list that catches them catches every English sentence with them.
var portugueseWords = []string{
	"alvo", "aguardar", "arquivo", "assercao", "assinantes", "autenticacao", "autorizacao",
	"cabecalho", "caminho", "captura", "carga", "cenario", "chave", "consultar", "corpo",
	"dados", "duracao", "erro", "erros", "escolha", "esperava", "execucao", "falha", "falhou",
	"fatura", "fila", "gerador", "jornada", "latencia", "medicao", "mensageria", "metodo",
	"nenhum", "nenhuma", "nome", "obter", "passo", "passos", "patamar", "pedido", "pedidos",
	"perfil", "perfis", "quantidade", "rampa", "relatorio", "requisicao", "resposta", "saida",
	"segredo", "semente", "senha", "taxa", "topico", "troca", "usuario", "valor", "valores",
	"variaveis", "verificar", "vazio", "sucesso", "regressao", "sequencial",
}

// Portuguese-only marks. An English sentence never needs them, and every one of
// them that reaches the terminal is a word the reader cannot look up.
var portugueseMarks = regexp.MustCompile(`[ãõçáâàéêíóôúü]`)

var wordPattern = regexp.MustCompile(`\pL+`)

// The exception list is explicit on purpose: a sweep that silences itself with
// a broad pattern stops being a sweep. Each entry says why it is here.
var allowed = map[string]string{
	// Kept as a domain term, like k6 keeps 'vus': these identify a Brazilian
	// document and translating them would name something that does not exist.
	"cpf":  "generator of a Brazilian tax id, kept as the name of the thing",
	"cnpj": "generator of a Brazilian company id, kept as the name of the thing",
	// The tool refuses a literal credential by field name, and whoever wrote the
	// scenario may have named the field in Portuguese. Dropping these two names
	// would let exactly those files publish a password.
	"senha":   "field name that carries a credential, recognized so it can be refused",
	"segredo": "field name that carries a credential, recognized so it can be refused",
	// The parser recognizes the 0.5.0 format to teach the way out (ADR 0019).
	// The words are Portuguese because the old format was.
	"internal/scenario/migration.go": "the rename map of the old format, read from both sides",
}

// The site is Portuguese until the bilingual build of phase 2, and its
// generator lives apart from anything the binary prints.
var skippedDirectories = []string{
	filepath.Join("internal", "site"),
	filepath.Join("cmd", "site"),
	filepath.Join("internal", "tools"),
}

func TestNoPortugueseSurvivedInWhatTheToolPrints(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, where := range []string{"cmd", "internal", "dsl", "examples"} {
		walkGo(t, filepath.Join(root, where), func(path string, file *ast.File, positions *token.FileSet) {
			ast.Inspect(file, func(node ast.Node) bool {
				literal, is := node.(*ast.BasicLit)
				if !is || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					return true
				}
				if found := portugueseIn(value); found != "" {
					t.Errorf("%s:%d: %q is Portuguese, in %q",
						relative(root, path), positions.Position(literal.Pos()).Line, found, shorten(value))
				}
				return true
			})
		})
	}
}

// The schema is the other half of the format: the editor reads it before the
// tool reads the file, so a Portuguese key surviving here reaches the person
// through autocomplete even if the parser no longer accepts it.
func TestPublishedSchemaHasNoPortugueseKeyOrValue(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("could not read the published schema: %v", err)
	}
	var document any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("the published schema is not valid JSON: %v", err)
	}
	walkJSON(t, "", document)
}

func walkJSON(t *testing.T, where string, node any) {
	t.Helper()
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if found := portugueseIn(key); found != "" {
				t.Errorf("%s: the key %q is Portuguese (%q)", where, key, found)
			}
			walkJSON(t, where+"/"+key, value)
		}
	case []any:
		for index, value := range typed {
			walkJSON(t, where+"/"+strconv.Itoa(index), value)
		}
	case string:
		if found := portugueseIn(typed); found != "" {
			t.Errorf("%s: %q is Portuguese, in %q", where, found, shorten(typed))
		}
	}
}

// The examples are the first scenario anyone reads, and they run against the
// built-in target: a Portuguese key here is both a wrong lesson and a file that
// no longer loads.
func TestPublishedExamplesHaveNoPortugueseKey(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no example scenario found: %v", err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for number, line := range strings.Split(string(content), "\n") {
			key, _, isPair := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")), ":")
			if !isPair || strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if found := portugueseIn(key); found != "" {
				t.Errorf("%s:%d: the key %q is Portuguese (%q)", filepath.Base(file), number+1, key, found)
			}
		}
	}
}

func portugueseIn(text string) string {
	for _, word := range wordPattern.FindAllString(text, -1) {
		lowered := strings.ToLower(word)
		if allowed[lowered] != "" {
			continue
		}
		if slices.Contains(portugueseWords, lowered) {
			return word
		}
		if portugueseMarks.MatchString(lowered) {
			return word
		}
	}
	return ""
}

func walkGo(t *testing.T, root string, visit func(string, *ast.File, *token.FileSet)) {
	t.Helper()
	positions := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			for _, skipped := range skippedDirectories {
				if strings.HasSuffix(filepath.Clean(path), skipped) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if allowed[filepath.ToSlash(relative(filepath.Join(root, ".."), path))] != "" {
			return nil
		}
		file, err := parser.ParseFile(positions, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		visit(path, file, positions)
		return nil
	})
	if err != nil {
		t.Fatalf("could not walk %s: %v", root, err)
	}
}

func relative(root, path string) string {
	shortened, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(shortened)
}

func shorten(text string) string {
	flat := strings.Join(strings.Fields(text), " ")
	if len(flat) > 90 {
		return flat[:90] + "…"
	}
	return flat
}
