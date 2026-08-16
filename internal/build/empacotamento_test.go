package build_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/goreleaser/fileglob"
	"gopkg.in/yaml.v3"
)

// O empacotamento ja saiu errado uma vez: o padrao era examples/**/*, que casa
// so o que esta dentro de subpasta, e nenhum cenario da raiz de examples/ foi
// para dentro do .tar.gz. Passou pela configuracao, pelo goreleaser e pela
// conferencia; so apareceu porque alguem abriu o arquivo.
//
// Este teste fecha a categoria comparando as duas listas entre si — a que o CI
// executa e a que o artefato leva —, sem lista intermediaria escrita a mao que
// envelhece quando entra exemplo novo.
//
// A resolucao dos padroes usa o fileglob, que e a biblioteca que o proprio
// goreleaser chama em internal/archivefiles: reimplementar a semantica de glob
// aqui seria testar a nossa leitura dela, nao o que vai acontecer.
func TestTheExamplesInTheArchiveAreTheOnesTheCIRuns(t *testing.T) {
	root := repositoryRoot(t)
	restore := workingDirectory(t, root)
	defer restore()

	inArchive := yamlScenarios(archiveFiles(t))
	inCI := yamlScenarios(ciScenarios(t))

	if len(inCI) == 0 {
		t.Fatal("nao achei nenhum cenario no laco do script de exemplos: o teste nao estaria provando nada")
	}
	for _, scenario := range inCI {
		if !slices.Contains(inArchive, scenario) {
			t.Errorf("o CI executa %s e o artefato publicado nao leva: quem baixa o binario nao consegue rodar o exemplo", scenario)
		}
	}
	for _, scenario := range inArchive {
		if !slices.Contains(inCI, scenario) {
			t.Errorf("o artefato leva %s e o CI nao executa: exemplo publicado que ninguem roda ja quebrou tres vezes sem ninguem ver", scenario)
		}
	}
}

// Os padroes declarados em archives[].files do .goreleaser.yaml, ja resolvidos
// contra o repositorio.
func archiveFiles(t *testing.T) []string {
	t.Helper()
	content, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("nao consegui ler a configuracao de release: %v", err)
	}
	var configuration struct {
		Archives []struct {
			Files []string `yaml:"files"`
		} `yaml:"archives"`
	}
	if err := yaml.Unmarshal(content, &configuration); err != nil {
		t.Fatalf("a configuracao de release nao e YAML valido: %v", err)
	}
	if len(configuration.Archives) == 0 {
		t.Fatal("a configuracao de release nao declara nenhum arquivo")
	}

	var packaged []string
	for _, pattern := range configuration.Archives[0].Files {
		matches, err := fileglob.Glob(pattern)
		if err != nil {
			t.Fatalf("o padrao %q nao casa com nada no repositorio: %v", pattern, err)
		}
		packaged = append(packaged, matches...)
	}
	return packaged
}

// O laco do script de exemplos, lido do proprio script: uma copia da lista aqui
// seria a lista escrita a mao que este teste existe para nao ter.
func ciScenarios(t *testing.T) []string {
	t.Helper()
	script := filepath.Join(".github", "executar-exemplos.sh")
	content, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("nao consegui ler %s: %v", script, err)
	}
	loop := regexp.MustCompile(`for +\w+ +in +(\S+); *do`).FindSubmatch(content)
	if loop == nil {
		t.Fatalf("nao achei o laco que roda os exemplos em %s", script)
	}
	matches, err := filepath.Glob(string(loop[1]))
	if err != nil {
		t.Fatalf("o padrao %q do script nao resolve: %v", loop[1], err)
	}
	return matches
}

func yamlScenarios(paths []string) []string {
	var scenarios []string
	for _, path := range paths {
		normalized := filepath.ToSlash(path)
		if strings.HasSuffix(normalized, ".yaml") && strings.HasPrefix(normalized, "examples/") {
			scenarios = append(scenarios, normalized)
		}
	}
	slices.Sort(scenarios)
	return scenarios
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("nao consegui achar a raiz do modulo: %v", err)
	}
	return root
}

// O fileglob resolve contra o diretorio de trabalho, que e o que o goreleaser
// tem quando roda: para ler o mesmo resultado, o teste precisa estar la.
func workingDirectory(t *testing.T, path string) func() {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("nao consegui ler o diretorio de trabalho: %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("nao consegui entrar em %s: %v", path, err)
	}
	return func() { _ = os.Chdir(previous) }
}
