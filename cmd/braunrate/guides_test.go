package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Documentacao que ensina uma opcao que nao existe mais e pior que documentacao
// ausente: quem copia procura o proprio erro. Cada linha de comando publicada
// nos guias e conferida contra o binario de verdade — o subcomando tem que
// existir, e cada opcao tem que estar na lista que o proprio comando imprime.
func TestEveryPublishedCommandExists(t *testing.T) {
	binary := compile(t)
	lines := commandLines(t)
	if len(lines) == 0 {
		t.Fatal("nenhuma linha de comando encontrada nos guias: o teste não estaria provando nada")
	}

	options := map[string][]string{}
	for _, line := range lines {
		if !slices.Contains(commands, line.subcommand) {
			t.Errorf("%s: %q não e um comando do braunrate; existem: %s",
				line.where, line.subcommand, strings.Join(commands, ", "))
			continue
		}
		known, asked := options[line.subcommand]
		if !asked {
			known = declaredOptions(t, binary, line.subcommand)
			options[line.subcommand] = known
		}
		// Comando sem conjunto de opcoes — 'new', 'import', 'validate', 'version'
		// leem os argumentos na mao — nao imprime lista, e ai nao ha o que
		// conferir alem do nome.
		if len(known) == 0 {
			continue
		}
		for _, option := range line.options {
			if !slices.Contains(known, option) {
				t.Errorf("%s: 'braunrate %s' não tem a opção -%s; tem: -%s",
					line.where, line.subcommand, option, strings.Join(known, ", -"))
			}
		}
	}
}

func compile(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "braunrate")
	command := exec.Command("go", "build", "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("não consegui compilar o binario: %v\n%s", err, output)
	}
	return binary
}

var optionLine = regexp.MustCompile(`(?m)^\s+-(\S+)`)

func declaredOptions(t *testing.T, binary, subcommand string) []string {
	t.Helper()
	output, _ := exec.Command(binary, subcommand, "-h").CombinedOutput()
	if !strings.Contains(string(output), "opções de "+subcommand) {
		return nil
	}
	var names []string
	for _, match := range optionLine.FindAllStringSubmatch(string(output), -1) {
		names = append(names, match[1])
	}
	return names
}

type commandLine struct {
	where      string
	subcommand string
	options    []string
}

func commandLines(t *testing.T) []commandLine {
	t.Helper()
	directory := filepath.Join("..", "..", "docs", "guides")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("não consegui ler %s: %v", directory, err)
	}
	var found []commandLine
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("não consegui ler %s: %v", entry.Name(), err)
		}
		for _, shell := range shellBlocks(string(content)) {
			for _, line := range strings.Split(shell, "\n") {
				if parsed, is := parseCommand(line); is {
					parsed.where = entry.Name()
					found = append(found, parsed)
				}
			}
		}
	}
	return found
}

var shellFence = regexp.MustCompile("(?s)```bash\n(.*?)```")

func shellBlocks(markdown string) []string {
	var blocks []string
	for _, match := range shellFence.FindAllStringSubmatch(markdown, -1) {
		blocks = append(blocks, match[1])
	}
	return blocks
}

// So conta o braunrate que esta sendo invocado. "go build -o braunrate" tem a
// palavra na linha e nao e um comando do braunrate.
func parseCommand(line string) (commandLine, bool) {
	words := split(line)
	position := -1
	starting := true
	for index, word := range words {
		if starting && assignment.MatchString(word) {
			continue
		}
		if starting && isBraunrate(word) {
			position = index
			break
		}
		starting = word == "|" || word == "&&" || word == ";"
	}
	if position < 0 || position+1 >= len(words) {
		return commandLine{}, false
	}
	parsed := commandLine{subcommand: words[position+1]}
	if strings.HasPrefix(parsed.subcommand, "-") {
		return commandLine{}, false
	}
	for _, word := range words[position+2:] {
		if !strings.HasPrefix(word, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(word, "-"), "=")
		parsed.options = append(parsed.options, name)
	}
	return parsed, true
}

var assignment = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*=`)

func isBraunrate(word string) bool {
	return word == "braunrate" || strings.HasSuffix(word, "/braunrate") || strings.HasSuffix(word, "braunrate.exe")
}

// O comando publicado carrega argumento entre aspas com opcoes dentro dele — o
// curl que vai para o 'import' e o exemplo. Cortar por espaco leria "-X" como
// opcao do braunrate.
func split(line string) []string {
	var words []string
	var current strings.Builder
	quote := rune(0)
	for _, character := range line {
		switch {
		case quote != 0 && character == quote:
			quote = 0
		case quote != 0:
			current.WriteRune(character)
		case character == '\'' || character == '"':
			quote = character
		case character == ' ' || character == '\t':
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(character)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}
