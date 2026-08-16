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
	"regexp"
	"sort"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

//go:embed estilo.css
var stylesheet string

//go:embed pagina.js
var script string

const guidesDirectory = "docs/guias"

type Page struct {
	Slug     string
	Title    string
	Section  string
	Summary  string
	Markdown string
	// Caminho do arquivo no repositorio, para o "editar esta pagina" do rodape.
	Source string
	Hero   *Hero
	// Pagina cujo indice proprio ja esta no corpo, como a grade de comandos.
	SemIndice bool
}

func Build(repositoryRoot, destination string, version string) error {
	pages, err := Pages(repositoryRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("não consegui criar %s: %w", destination, err)
	}
	realce, err := highlightStyles()
	if err != nil {
		return fmt.Errorf("não consegui gerar o destaque de sintaxe: %w", err)
	}
	index, err := searchIndex(pages)
	if err != nil {
		return fmt.Errorf("não consegui montar o índice de busca: %w", err)
	}
	for name, content := range map[string]string{"estilo.css": stylesheet + realce, "pagina.js": script, "indice.js": index} {
		if err := os.WriteFile(filepath.Join(destination, name), []byte(content), 0o644); err != nil {
			return fmt.Errorf("não consegui gravar %s: %w", name, err)
		}
	}
	for index, page := range pages {
		source := page.Markdown
		cards, stripped, hasCards := commandCards(source)
		if hasCards {
			source = stripped
		}
		body, err := renderMarkdown(source)
		if err != nil {
			return fmt.Errorf("%s: %w", page.Slug, err)
		}
		if hasCards {
			body = placeCards(body, cards)
			// A grade ja e o indice desta pagina; repetir a mesma lista na coluna
			// da direita gasta a largura que os cartoes precisam para caber numa
			// tela so.
			page.SemIndice = true
		}
		content := layout(page, pages, index, body, version)
		if err := os.WriteFile(filepath.Join(destination, fileOf(page, index)), []byte(content), 0o644); err != nil {
			return fmt.Errorf("não consegui gravar %s: %w", page.Slug, err)
		}
	}
	// GitHub Pages serves through Jekyll unless told otherwise, and Jekyll
	// silently drops files it does not understand.
	return os.WriteFile(filepath.Join(destination, ".nojekyll"), nil, 0o644)
}

func fileOf(page Page, index int) string {
	if index == 0 {
		return "index.html"
	}
	return page.Slug + ".html"
}

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

// O nome do arquivo decide ordem e secao; o primeiro titulo decide o nome da
// pagina. Declarar as duas coisas seria o caminho para menu e titulo
// discordarem.
func guides(repositoryRoot string) ([]Page, error) {
	directory := filepath.Join(repositoryRoot, guidesDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("não consegui ler %s: %w", directory, err)
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
			return nil, fmt.Errorf("não consegui ler %s: %w", name, err)
		}
		text := string(content)
		title, found := firstHeading(text)
		if !found {
			return nil, fmt.Errorf("%s não começa com um título '# '", name)
		}
		section, slug := sectionAndSlug(name)
		hero, text := extractHero(text)
		page := Page{
			Slug: slug, Title: title, Section: section, Summary: summary(text),
			Markdown: text, Source: guidesDirectory + "/" + name, Hero: hero,
		}
		if hero != nil {
			page.Summary = hero.Lema
			page.Markdown = strings.TrimPrefix(text, "# "+title+"\n")
		}
		pages = append(pages, page)
	}
	return pages, nil
}

var sectionNames = map[string]string{
	"comecar":    "Começar",
	"guias":      "Guias",
	"referencia": "Referência",
}

func sectionAndSlug(fileName string) (string, string) {
	name := strings.TrimSuffix(fileName, ".md")
	parts := strings.SplitN(name, "-", 3)
	if len(parts) < 3 {
		return "Guias", strings.Join(parts[1:], "-")
	}
	return sectionNames[parts[1]], parts[2]
}

func firstHeading(markdown string) (string, bool) {
	for _, line := range strings.Split(markdown, "\n") {
		if rest, isHeading := strings.CutPrefix(line, "# "); isHeading {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

func summary(markdown string) string {
	_, after, found := strings.Cut(markdown, "\n")
	if !found {
		return ""
	}
	for _, paragraph := range strings.Split(strings.TrimSpace(after), "\n\n") {
		line := strings.TrimSpace(paragraph)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
			continue
		}
		return plain(strings.ReplaceAll(line, "\n", " "))
	}
	return ""
}

// O identificador do titulo e escrito aqui, e nao deixado para o goldmark: ele
// descarta letra acentuada em vez de transliterar, e "propositos" virava
// "propsitos" na barra de endereco.
var headingLine = regexp.MustCompile(`(?m)^(#{2,3}) +(.+?)\s*$`)

func renderMarkdown(source string) (string, error) {
	source = headingLine.ReplaceAllStringFunc(source, func(line string) string {
		parts := headingLine.FindStringSubmatch(line)
		return fmt.Sprintf("%s %s {#%s}", parts[1], parts[2], slugify(parts[2]))
	})
	converter := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			// Em classe, com as duas paletas geradas em realce.go: cor embutida na
			// tag so permite uma paleta, e ela ficava escura no tema claro.
			highlighting.NewHighlighting(highlighting.WithFormatOptions(chromahtml.WithClasses(true))),
		),
		goldmark.WithParserOptions(parser.WithAttribute()),
	)
	var rendered bytes.Buffer
	if err := converter.Convert([]byte(source), &rendered); err != nil {
		return "", err
	}
	return decorate(rendered.String()), nil
}

var (
	headingTag = regexp.MustCompile(`<(h[23])( id="([^"]+)")>(.*?)</h[23]>`)
	callout    = regexp.MustCompile(`(?s)<blockquote>\s*<p><strong>(Nota|Atenção|Importante|Dica)</strong>(.*?)</blockquote>`)
)

func decorate(page string) string {
	page = headingTag.ReplaceAllString(page,
		`<$1$2>$4 <a class="ancora" href="#$3" aria-label="link para esta seção">#</a></$1>`)
	return callout.ReplaceAllString(page,
		`<aside class="nota nota-$1"><p class="rotulo">$1</p><p>$2</aside>`)
}

func layout(page Page, pages []Page, index int, body, version string) string {
	title := page.Title + " — braunrate"
	if page.Title == "braunrate" {
		title = "braunrate"
	}

	hero := ""
	if page.Hero != nil {
		hero = page.Hero.render()
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<meta name="description" content="%s">
<link rel="stylesheet" href="estilo.css">
<script>
// Antes da primeira pintura: escolher o tema depois dela pisca a pagina inteira.
(function () {
  try {
    var escolhido = localStorage.getItem('braunrate-tema')
    if (escolhido) document.documentElement.setAttribute('data-tema', escolhido)
  } catch (erro) { /* navegacao privada nao guarda, e o tema do sistema vale */ }
})()
</script>
</head>
<body>
<header>
  <a class="marca" href="index.html"><span class="sinal" aria-hidden="true"></span>braunrate</a>
  <span class="versao">%s</span>
  <button type="button" class="abrir-busca" id="abrir-busca" aria-label="Buscar na documentação">
    buscar <kbd>/</kbd>
  </button>
  <button type="button" class="tema" id="tema" aria-label="Alternar tema claro e escuro">tema</button>
  <a class="repositorio" href="https://github.com/Diegobraun/braunrate">GitHub</a>
</header>
<div class="pagina">
  <nav class="secoes" aria-label="Seções">
%s  </nav>
  <main>
%s    <article>
%s    </article>
%s  </main>
%s</div>
%s<div class="busca" id="busca" hidden>
  <div class="caixa-busca" role="dialog" aria-modal="true" aria-label="Buscar na documentação">
    <input type="search" id="termo" placeholder="buscar na documentação" autocomplete="off" spellcheck="false">
    <ol id="resultados" aria-live="polite"></ol>
    <p class="dica-busca"><kbd>↑</kbd><kbd>↓</kbd> navega · <kbd>enter</kbd> abre · <kbd>esc</kbd> fecha</p>
  </div>
</div>
<script src="indice.js"></script>
<script src="pagina.js"></script>
</body>
</html>
`, html.EscapeString(title), html.EscapeString(page.Summary), html.EscapeString(version),
		menu(pages, index), hero, body, pagination(pages, index), tableOfContents(page),
		footer(page, version))
}

// Pagina escrita a mao convida a edicao; pagina gerada diz de onde ela sai,
// porque editar o HTML dela nao mudaria nada no proximo build.
func footer(page Page, version string) string {
	origem := fmt.Sprintf(`<a href="https://github.com/Diegobraun/braunrate/edit/main/%s">editar esta página</a>`,
		html.EscapeString(page.Source))
	if !strings.HasSuffix(page.Source, ".md") {
		origem = fmt.Sprintf(`gerada de <a href="https://github.com/Diegobraun/braunrate/blob/main/%s"><code>%s</code></a>`,
			html.EscapeString(page.Source), html.EscapeString(page.Source))
	}
	return fmt.Sprintf(`<footer>
  <p><a href="https://github.com/Diegobraun/braunrate">repositório</a> · %s</p>
  <p class="direita">braunrate %s · <a href="https://github.com/Diegobraun/braunrate/blob/main/LICENSE">licença MIT</a></p>
</footer>
`, origem, html.EscapeString(version))
}

func menu(pages []Page, index int) string {
	var written strings.Builder
	section := ""
	for position, page := range pages {
		if page.Section != section {
			if section != "" {
				written.WriteString("    </ol>\n")
			}
			fmt.Fprintf(&written, "    <p class=\"secao\">%s</p>\n    <ol>\n", html.EscapeString(page.Section))
			section = page.Section
		}
		current := ""
		if position == index {
			current = ` aria-current="page"`
		}
		fmt.Fprintf(&written, "      <li><a href=%q%s>%s</a></li>\n",
			fileOf(page, position), current, html.EscapeString(page.Title))
	}
	written.WriteString("    </ol>\n")
	return written.String()
}

func pagination(pages []Page, index int) string {
	var written strings.Builder
	written.WriteString("    <nav class=\"paginacao\" aria-label=\"Páginas\">\n")
	if index > 0 {
		fmt.Fprintf(&written, "      <a class=\"anterior\" href=%q><span>anterior</span>%s</a>\n",
			fileOf(pages[index-1], index-1), html.EscapeString(pages[index-1].Title))
	}
	if index+1 < len(pages) {
		fmt.Fprintf(&written, "      <a class=\"proxima\" href=%q><span>próxima</span>%s</a>\n",
			fileOf(pages[index+1], index+1), html.EscapeString(pages[index+1].Title))
	}
	written.WriteString("    </nav>\n")
	return written.String()
}

var sectionHeading = regexp.MustCompile(`(?m)^(##|###) +(.+)$`)

func tableOfContents(page Page) string {
	matches := sectionHeading.FindAllStringSubmatch(page.Markdown, -1)
	if page.SemIndice || len(matches) < 3 {
		return ""
	}
	var written strings.Builder
	written.WriteString("  <aside class=\"indice\" aria-label=\"Nesta página\">\n    <p class=\"secao\">Nesta página</p>\n    <ol>\n")
	for _, match := range matches {
		level := "nivel-2"
		if match[1] == "###" {
			level = "nivel-3"
		}
		text := strings.TrimSpace(match[2])
		fmt.Fprintf(&written, "      <li class=%q><a href=\"#%s\">%s</a></li>\n",
			level, slugify(text), html.EscapeString(plain(text)))
	}
	written.WriteString("    </ol>\n  </aside>\n")
	return written.String()
}

var markup = regexp.MustCompile("`|\\*\\*|\\*|_")

func plain(text string) string { return markup.ReplaceAllString(text, "") }

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "í", "i", "î", "i", "ì", "i",
	"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
	"ú", "u", "û", "u", "ù", "u", "ü", "u", "ç", "c", "ñ", "n",
)

func slugify(text string) string {
	var written strings.Builder
	previousDash := false
	for _, character := range accents.Replace(strings.ToLower(plain(text))) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			written.WriteRune(character)
			previousDash = false
		case !previousDash && written.Len() > 0:
			written.WriteRune('-')
			previousDash = true
		}
	}
	return strings.TrimSuffix(written.String(), "-")
}
