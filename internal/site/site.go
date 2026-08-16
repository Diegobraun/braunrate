// Package site turns the guides in docs/guias into the published site. The
// content lives in the repository and travels through pull request like the
// code does; the site is a projection of it, never a second copy that someone
// has to remember to update.
package site

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

//go:embed estilo.css
var stylesheet string

const guidesDirectory = "docs/guias"

type Page struct {
	Slug     string
	Title    string
	Markdown string
}

// Build writes the whole site under destination. Pages generated from the
// schema and from the ADRs are assembled here and go through the same renderer
// as the hand-written ones: a generated page that looked different would read
// as a different product.
func Build(repositoryRoot, destination string, version string) error {
	pages, err := Pages(repositoryRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("nao consegui criar %s: %w", destination, err)
	}
	if err := os.WriteFile(filepath.Join(destination, "estilo.css"), []byte(stylesheet), 0o644); err != nil {
		return fmt.Errorf("nao consegui gravar a folha de estilo: %w", err)
	}
	for index, page := range pages {
		body, err := renderMarkdown(page.Markdown)
		if err != nil {
			return fmt.Errorf("%s: %w", page.Slug, err)
		}
		file := page.Slug + ".html"
		if index == 0 {
			file = "index.html"
		}
		content := layout(page, pages, index, body, version)
		if err := os.WriteFile(filepath.Join(destination, file), []byte(content), 0o644); err != nil {
			return fmt.Errorf("nao consegui gravar %s: %w", file, err)
		}
	}
	// GitHub Pages serves through Jekyll unless told otherwise, and Jekyll
	// silently drops files it does not understand.
	return os.WriteFile(filepath.Join(destination, ".nojekyll"), nil, 0o644)
}

// Pages is the ordered list of everything the site publishes, hand-written and
// generated alike. Exported because the tests walk it: a page nobody rendered
// is a page nobody checked.
func Pages(repositoryRoot string) ([]Page, error) {
	written, err := guides(repositoryRoot)
	if err != nil {
		return nil, err
	}
	reference, err := ReferencePage(repositoryRoot)
	if err != nil {
		return nil, err
	}
	decisions, err := DecisionsPage(repositoryRoot)
	if err != nil {
		return nil, err
	}
	return append(written, reference, decisions), nil
}

// The file name decides the order — 00-inicio, 01-instalacao — and the first
// heading decides the title. Two places to say the same thing is how a title
// and a menu entry end up disagreeing.
func guides(repositoryRoot string) ([]Page, error) {
	directory := filepath.Join(repositoryRoot, guidesDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("nao consegui ler %s: %w", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	pages := make([]Page, 0, len(names))
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("nao consegui ler %s: %w", name, err)
		}
		text := string(content)
		title, found := firstHeading(text)
		if !found {
			return nil, fmt.Errorf("%s nao comeca com um titulo '# '", name)
		}
		pages = append(pages, Page{Slug: slug(name), Title: title, Markdown: text})
	}
	return pages, nil
}

func slug(fileName string) string {
	name := strings.TrimSuffix(fileName, ".md")
	if _, rest, found := strings.Cut(name, "-"); found {
		return rest
	}
	return name
}

func firstHeading(markdown string) (string, bool) {
	for _, line := range strings.Split(markdown, "\n") {
		if rest, isHeading := strings.CutPrefix(line, "# "); isHeading {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

func renderMarkdown(source string) (string, error) {
	converter := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	var rendered bytes.Buffer
	if err := converter.Convert([]byte(source), &rendered); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func layout(page Page, pages []Page, index int, body, version string) string {
	var menu strings.Builder
	for position, other := range pages {
		file := other.Slug + ".html"
		if position == 0 {
			file = "index.html"
		}
		current := ""
		if position == index {
			current = ` aria-current="page"`
		}
		fmt.Fprintf(&menu, "      <li><a href=%q%s>%s</a></li>\n", file, current, html.EscapeString(other.Title))
	}

	// A pagina inicial se chama braunrate, e "braunrate — braunrate" na aba do
	// navegador nao diz nada a mais.
	title := page.Title + " — braunrate"
	if page.Title == "braunrate" {
		title = "braunrate"
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<link rel="stylesheet" href="estilo.css">
</head>
<body>
<div class="pagina">
  <nav>
    <a class="marca" href="index.html">braunrate</a>
    <span class="versao">%s</span>
    <ol>
%s    </ol>
  </nav>
  <main>
%s    <footer>
      Gerado de <code>docs/guias</code> deste repositorio. Os cenarios e as
      linhas de comando publicados aqui sao conferidos pela suite de testes:
      cenario que nao carrega ou opcao que nao existe mais reprovam o build.
    </footer>
  </main>
</div>
</body>
</html>
`, html.EscapeString(title), html.EscapeString(version), menu.String(), body)
}
