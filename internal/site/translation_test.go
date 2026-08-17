package site_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/site"
)

// Traducao que envelhece em silencio e pior que pagina que nao existe: quem le
// acredita no texto velho sem nenhum sinal de que ele nao vale mais. O aviso da
// build e para quem edita o original; a tarja na pagina e para quem le.
func TestNoPublishedTranslationIsBehindItsSource(t *testing.T) {
	_, warnings := buildWithWarnings(t)
	for _, warning := range warnings {
		t.Errorf("tradução atrasada: %s", warning)
	}
}

func TestATranslationBehindItsSourceWarnsAndSaysSoOnThePage(t *testing.T) {
	repository := copyOfTheDocumentation(t)
	guide := filepath.Join(repository, "docs", "guides", "30-guides-concepts.pt-BR.md")
	content, err := os.ReadFile(guide)
	if err != nil {
		t.Fatalf("não consegui ler o guia: %v", err)
	}
	front, body, found := strings.Cut(string(content), "source_hash: ")
	if !found {
		t.Fatal("o guia traduzido não declara source_hash")
	}
	_, rest, _ := strings.Cut(body, "\n")
	if err := os.WriteFile(guide, []byte(front+"source_hash: 000000000000\n"+rest), 0o644); err != nil {
		t.Fatalf("não consegui reescrever o guia: %v", err)
	}

	destination := t.TempDir()
	warnings, err := site.Build(repository, destination, "teste")
	if err != nil {
		t.Fatalf("o site não foi gerado: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "30-guides-concepts.pt-BR.md") {
		t.Fatalf("a build não avisou sobre a tradução atrasada; avisou: %v", warnings)
	}

	page, err := os.ReadFile(filepath.Join(destination, "pt-BR", "concepts.html"))
	if err != nil {
		t.Fatalf("não consegui ler a página traduzida: %v", err)
	}
	if !strings.Contains(string(page), "Tradução atrasada") {
		t.Error("a página não diz ao leitor que a tradução está atrás do original")
	}
	english, err := os.ReadFile(filepath.Join(destination, "concepts.html"))
	if err != nil {
		t.Fatalf("não consegui ler a página em inglês: %v", err)
	}
	if strings.Contains(string(english), "Translation behind") {
		t.Error("a página em inglês é a fonte: ela não pode estar atrasada em relação a si mesma")
	}
}

// A build so le docs/ e o schema, entao a copia e barata e o teste nao precisa
// de um repositorio inteiro para mexer num arquivo.
func copyOfTheDocumentation(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	source := filepath.Join(root, "docs")
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(repository, "docs", relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o644)
	})
	if err != nil {
		t.Fatalf("não consegui copiar a documentação: %v", err)
	}
	return repository
}
