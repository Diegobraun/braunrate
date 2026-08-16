package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/build"
)

// Binario compilado a mao nao pode se apresentar com numero de release: o
// documento de resultado guarda a versao, a comparacao recusa versoes
// diferentes, e um "0.4.0" que na verdade e a arvore de trabalho de alguem faz
// as duas coisas mentirem.
func TestAHandBuiltBinarySaysItIsDev(t *testing.T) {
	if build.Version != "dev" {
		t.Fatalf("a versao padrao saiu %q e devia ser dev", build.Version)
	}
	for name, value := range map[string]string{"commit": build.Commit, "data": build.Date} {
		if value != "desconhecido" {
			t.Errorf("%s padrao saiu %q", name, value)
		}
	}
}

// O que a publicacao escreve no -ldflags precisa ser o caminho destes
// simbolos. O teste acima prova que a injecao funciona; este prova que e esta
// a injecao que a release vai usar.
func TestTheReleaseConfigurationInjectsTheseSymbols(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("nao consegui ler a configuracao de release: %v", err)
	}
	const symbols = "github.com/Diegobraun/braunrate/internal/build"
	for _, name := range []string{"Version", "Commit", "Date"} {
		if !strings.Contains(string(content), "-X "+symbols+"."+name+"=") {
			t.Errorf("a release nao injeta %s.%s: o binario publicado sai com o valor padrao", symbols, name)
		}
	}
}

// O caminho do simbolo e o que a publicacao escreve no -ldflags. Um erro de
// digitacao ali nao quebra a compilacao: ela passa, o binario sai com "dev", e
// so se descobre depois da release publicada. Este teste compila com a injecao
// de verdade.
func TestTheInjectedValuesReachTheBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("compila um binario")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("nao consegui achar a raiz do modulo: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "braunrate")

	const symbols = "github.com/Diegobraun/braunrate/internal/build"
	compile := exec.Command("go", "build",
		"-ldflags", "-X "+symbols+".Version=9.9.9 -X "+symbols+".Commit=abcdef1 -X "+symbols+".Date=2026-01-02T03:04:05Z",
		"-o", binary, "./cmd/braunrate")
	compile.Dir = root
	compile.Env = os.Environ()
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("nao consegui compilar com injecao: %v\n%s", err, output)
	}

	printed, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("o binario injetado nao rodou: %v\n%s", err, printed)
	}
	for _, expected := range []string{"braunrate 9.9.9", "commit: abcdef1", "data: 2026-01-02T03:04:05Z"} {
		if !strings.Contains(string(printed), expected) {
			t.Errorf("a saida nao traz %q:\n%s", expected, printed)
		}
	}
}
