package main

import (
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/server"
)

// An option pasted after the file was silently ignored: the report was never
// written and nothing warned. Silence is the worst failure mode.
func TestFlagWorksBeforeAndAfterFile(t *testing.T) {
	testCases := [][]string{
		{"-html", "relatorio.html", "cenario.yaml"},
		{"cenario.yaml", "-html", "relatorio.html"},
		{"cenario.yaml", "-html=relatorio.html"},
	}
	for _, args := range testCases {
		set := flag.NewFlagSet("executar", flag.ContinueOnError)
		html := set.String("html", "", "arquivo HTML")
		positional := parseArguments(set, args)

		if len(positional) != 1 || positional[0] != "cenario.yaml" {
			t.Fatalf("%v: o arquivo de cenário não foi lido: %v", args, positional)
		}
		if *html != "relatorio.html" {
			t.Errorf("%v: a opção foi ignorada", args)
		}
	}
}

// From an empty folder there was no path to a first scenario: every command
// took a file and none created one.
func TestNewCommandWritesValidScenarioAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "cenario.yaml")

	if code := newOne([]string{destination}); code != 0 {
		t.Fatalf("novo devolveu %d", code)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("o arquivo não foi criado: %v", err)
	}
	c, err := scenario.Parse(content)
	if err != nil {
		t.Fatalf("o cenário de partida não carrega:\n%v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("o cenário de partida não e válido: %v", err)
	}
	if len(c.SLO) == 0 {
		t.Error("o cenário de partida precisa mostrar como se declara slo")
	}
	if code := newOne([]string{destination}); code == 0 {
		t.Error("novo sobre arquivo existente não pode sobrescrever em silencio")
	}
}

// O proprio autor errou este nome na primeira volta com o binario publicado e
// recebeu dez opcoes em lista. A resposta certa a ferramenta ja tinha: a
// sugestao por semelhanca existia desde a validacao de cenario.
func TestUnknownFlagSuggestsTheRightOneAndRebuildsTheCommand(t *testing.T) {
	set := newFlagSet("target")
	set.String("address", "127.0.0.1:8080", "endereço de escuta")
	args := []string{"-addr", ":8080"}

	message := unknownFlagMessage(set, args, errors.New(notDefined+"addr"))

	for _, expected := range []string{
		`"-addr" does not exist`,
		`Did you mean "-address"?`,
		"braunrate target -address :8080",
		"Every option: braunrate target -h",
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("a mensagem não traz %q:\n%s", expected, message)
		}
	}
}

// A word with no relative gets no guess: a wrong "did you mean" costs more
// than saying nothing.
func TestAFlagWithNoRelativeGetsNoGuess(t *testing.T) {
	set := newFlagSet("execute")
	set.String("html", "", "arquivo HTML")

	message := unknownFlagMessage(set, []string{"-xyzw"}, errors.New(notDefined+"xyzw"))

	if strings.Contains(message, "Did you mean") {
		t.Errorf("palpite sem parentesco:\n%s", message)
	}
}

// A busy port is the first error of whoever starts the interface twice, and up
// to here the output was the operating system text after the invitation to open
// the browser. The invitation only goes out after the bind; this is the message
// that is left.
func TestABusyPortSaysHowToChooseAnother(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("não consegui ocupar uma porta: %v", err)
	}
	defer func() { _ = listener.Close() }()

	address := listener.Addr().String()
	options := server.DefaultOptions("teste")
	options.Address = address

	_, err = server.New(options).Bind()
	if err == nil {
		t.Fatal("escutei numa porta ocupada")
	}

	message := captureStderr(t, func() { portInUse("ui", "-addr", address, err) })
	for _, expected := range []string{address + " is already taken", "braunrate ui -addr"} {
		if !strings.Contains(message, expected) {
			t.Errorf("a mensagem não traz %q:\n%s", expected, message)
		}
	}
}

func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("não consegui abrir o cano: %v", err)
	}
	original := os.Stderr
	os.Stderr = writer
	run()
	os.Stderr = original
	_ = writer.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("não consegui ler o que saiu: %v", err)
	}
	return string(content)
}

// O site promete que toda opcao aceita -h. Dois comandos nao passam por um
// FlagSet, e neles a bandeira virava nome de arquivo: 'validate -h' procurava um
// arquivo chamado "-h" e 'new -h' escrevia scenario.yaml no diretorio de quem so
// queria ler a ajuda. Pedir ajuda nunca pode ter efeito colateral.
func TestAskingForHelpNeverWritesAnythingAndNeverFails(t *testing.T) {
	for _, flagUsed := range []string{"-h", "--help", "-help"} {
		root := t.TempDir()
		previous, err := os.Getwd()
		if err != nil {
			t.Fatalf("não consegui ler o diretório atual: %v", err)
		}
		if err := os.Chdir(root); err != nil {
			t.Fatalf("não consegui entrar no diretório do teste: %v", err)
		}

		if code := newOne([]string{flagUsed}); code != 0 {
			t.Errorf("'new %s' saiu com %d", flagUsed, code)
		}
		if code := validate([]string{flagUsed}); code != 0 {
			t.Errorf("'validate %s' saiu com %d", flagUsed, code)
		}

		left, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("não consegui listar o diretório: %v", err)
		}
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("não consegui voltar: %v", err)
		}
		if len(left) != 0 {
			names := make([]string, 0, len(left))
			for _, entry := range left {
				names = append(names, entry.Name())
			}
			t.Errorf("pedir ajuda com %s escreveu %v", flagUsed, names)
		}
	}
}
