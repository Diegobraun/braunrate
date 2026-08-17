// Package site turns the guides in docs/guias into the published site. The
// content lives in the repository and travels through pull request like the
// code does; the site is a projection of it, never a second copy that someone
// has to remember to update.
package site

import (
	"bytes"
	_ "embed"
	"encoding/json"
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

//go:embed style.css
var stylesheet string

//go:embed page.js
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
	WithoutContents bool
}

// O texto da moldura vive junto, e nao espalhado pelo gerador: e ele que muda
// quando a pagina muda de lingua, e uma frase perdida dentro de uma funcao e
// uma frase que ninguem acha na hora de traduzir.
type chrome struct {
	Sections       map[string]string
	Search         string
	SearchLabel    string
	SearchHint     string
	Placeholder    string
	Theme          string
	OnThisPage     string
	Pages          string
	Previous       string
	Next           string
	EditThisPage   string
	GeneratedFrom  string
	Repository     string
	License        string
	Copy           string
	Copied         string
	CopyByHand     string
	LightTheme     string
	DarkTheme      string
	UseLightTheme  string
	UseDarkTheme   string
	TypeToSearch   string
	NothingFound   string
	CalloutClasses map[string]string
}

var portuguese = chrome{
	Sections: map[string]string{
		"comecar":    "Começar",
		"guias":      "Guias",
		"referencia": "Referência",
	},
	Search:        "buscar",
	SearchLabel:   "Buscar na documentação",
	SearchHint:    "navega · <kbd>enter</kbd> abre · <kbd>esc</kbd> fecha",
	Placeholder:   "buscar na documentação",
	Theme:         "tema",
	OnThisPage:    "Nesta página",
	Pages:         "Páginas",
	Previous:      "anterior",
	Next:          "próxima",
	EditThisPage:  "editar esta página",
	GeneratedFrom: "gerada de",
	Repository:    "repositório",
	License:       "licença MIT",
	Copy:          "copiar",
	Copied:        "copiado",
	CopyByHand:    "copie à mão",
	LightTheme:    "claro",
	DarkTheme:     "escuro",
	UseLightTheme: "Usar tema claro",
	UseDarkTheme:  "Usar tema escuro",
	TypeToSearch:  "Digite para buscar nas {pages} páginas.",
	NothingFound:  "Nada encontrado para “{term}”.",
	CalloutClasses: map[string]string{
		"Nota": "note", "Atenção": "warning", "Importante": "important", "Dica": "tip",
	},
}

func Build(repositoryRoot, destination string, version string) error {
	pages, err := Pages(repositoryRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", destination, err)
	}
	highlight, err := highlightStyles()
	if err != nil {
		return fmt.Errorf("could not generate the syntax highlighting: %w", err)
	}
	index, err := searchIndex(pages)
	if err != nil {
		return fmt.Errorf("could not build the search index: %w", err)
	}
	for name, content := range map[string]string{
		"style.css": stylesheet + highlight, "page.js": script, "search-index.js": index,
	} {
		if err := os.WriteFile(filepath.Join(destination, name), []byte(content), 0o644); err != nil {
			return fmt.Errorf("could not write %s: %w", name, err)
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
			page.WithoutContents = true
		}
		content := layout(page, pages, index, body, version)
		if err := os.WriteFile(filepath.Join(destination, fileOf(page, index)), []byte(content), 0o644); err != nil {
			return fmt.Errorf("could not write %s: %w", page.Slug, err)
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
		return nil, fmt.Errorf("could not read %s: %w", directory, err)
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
			return nil, fmt.Errorf("could not read %s: %w", name, err)
		}
		text := string(content)
		title, found := firstHeading(text)
		if !found {
			return nil, fmt.Errorf("%s does not start with a '# ' heading", name)
		}
		section, slug := sectionAndSlug(name)
		hero, text := extractHero(text)
		page := Page{
			Slug: slug, Title: title, Section: section, Summary: summary(text),
			Markdown: text, Source: guidesDirectory + "/" + name, Hero: hero,
		}
		if hero != nil {
			page.Summary = hero.Motto
			page.Markdown = strings.TrimPrefix(text, "# "+title+"\n")
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func sectionAndSlug(fileName string) (string, string) {
	name := strings.TrimSuffix(fileName, ".md")
	parts := strings.SplitN(name, "-", 3)
	if len(parts) < 3 {
		return portuguese.Sections["guias"], strings.Join(parts[1:], "-")
	}
	return portuguese.Sections[parts[1]], parts[2]
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
	callout    = regexp.MustCompile(`(?s)<blockquote>\s*<p><strong>([^<]+)</strong>(.*?)</blockquote>`)
)

// A classe do aviso e a mesma nas duas linguas; o que muda e a palavra que o
// autor escreve no markdown. Sem a tabela, a folha precisaria de um seletor por
// lingua para pintar a mesma tarja.
func decorate(page string) string {
	page = headingTag.ReplaceAllString(page,
		`<$1$2>$4 <a class="anchor" href="#$3" aria-label="link para esta seção">#</a></$1>`)
	return callout.ReplaceAllStringFunc(page, func(found string) string {
		parts := callout.FindStringSubmatch(found)
		class, known := portuguese.CalloutClasses[parts[1]]
		if !known {
			return found
		}
		return fmt.Sprintf(`<aside class="note note-%s"><p class="label">%s</p><p>%s</aside>`,
			class, parts[1], parts[2])
	})
}

func layout(page Page, pages []Page, index int, body, version string) string {
	title := page.Title + " — braunrate"
	if page.Title == "braunrate" {
		title = "braunrate"
	}

	hero := ""
	if page.Hero != nil {
		hero = page.Hero.render(portuguese)
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<meta name="description" content="%s">
<link rel="stylesheet" href="style.css">
<script>
// Antes da primeira pintura: escolher o tema depois dela pisca a pagina inteira.
(function () {
  try {
    var chosen = localStorage.getItem('braunrate-theme')
    if (chosen) document.documentElement.setAttribute('data-theme', chosen)
  } catch (error) { /* navegacao privada nao guarda, e o tema do sistema vale */ }
})()
window.SITE_TEXT = %s
</script>
</head>
<body>
<header>
  <a class="brand" href="index.html"><span class="mark" aria-hidden="true"></span>braunrate</a>
  <span class="version">%s</span>
  <button type="button" class="open-search" id="open-search" aria-label="%s">
    %s <kbd>/</kbd>
  </button>
  <button type="button" class="theme" id="theme" aria-label="%s">%s</button>
  <a class="repository" href="https://github.com/Diegobraun/braunrate">GitHub</a>
</header>
<div class="page">
  <nav class="sections" aria-label="%s">
%s  </nav>
  <main>
%s    <article>
%s    </article>
%s  </main>
%s</div>
%s<div class="search" id="search" hidden>
  <div class="search-box" role="dialog" aria-modal="true" aria-label="%s">
    <input type="search" id="term" placeholder="%s" autocomplete="off" spellcheck="false">
    <ol id="results" aria-live="polite"></ol>
    <p class="search-hint"><kbd>↑</kbd><kbd>↓</kbd> %s</p>
  </div>
</div>
<script src="search-index.js"></script>
<script src="page.js"></script>
</body>
</html>
`, html.EscapeString(title), html.EscapeString(page.Summary), pageText(portuguese),
		html.EscapeString(version), html.EscapeString(portuguese.SearchLabel), portuguese.Search,
		html.EscapeString(portuguese.UseDarkTheme), portuguese.Theme,
		html.EscapeString(portuguese.Sections["referencia"]),
		menu(pages, index), hero, body, pagination(pages, index), tableOfContents(page),
		footer(page, version), html.EscapeString(portuguese.SearchLabel),
		html.EscapeString(portuguese.Placeholder), portuguese.SearchHint)
}

// O script e o mesmo nas duas linguas, e o que ele escreve na tela vem daqui.
func pageText(text chrome) string {
	encoded, err := json.Marshal(map[string]string{
		"copy": text.Copy, "copied": text.Copied, "copyByHand": text.CopyByHand,
		"lightTheme": text.LightTheme, "darkTheme": text.DarkTheme,
		"useLightTheme": text.UseLightTheme, "useDarkTheme": text.UseDarkTheme,
		"typeToSearch": text.TypeToSearch, "nothingFound": text.NothingFound,
	})
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

// Pagina escrita a mao convida a edicao; pagina gerada diz de onde ela sai,
// porque editar o HTML dela nao mudaria nada no proximo build.
func footer(page Page, version string) string {
	origin := fmt.Sprintf(`<a href="https://github.com/Diegobraun/braunrate/edit/main/%s">%s</a>`,
		html.EscapeString(page.Source), portuguese.EditThisPage)
	if !strings.HasSuffix(page.Source, ".md") {
		origin = fmt.Sprintf(`%s <a href="https://github.com/Diegobraun/braunrate/blob/main/%s"><code>%s</code></a>`,
			portuguese.GeneratedFrom, html.EscapeString(page.Source), html.EscapeString(page.Source))
	}
	return fmt.Sprintf(`<footer>
  <p><a href="https://github.com/Diegobraun/braunrate">%s</a> · %s</p>
  <p class="right">braunrate %s · <a href="https://github.com/Diegobraun/braunrate/blob/main/LICENSE">%s</a></p>
</footer>
`, portuguese.Repository, origin, html.EscapeString(version), portuguese.License)
}

func menu(pages []Page, index int) string {
	var written strings.Builder
	section := ""
	for position, page := range pages {
		if page.Section != section {
			if section != "" {
				written.WriteString("    </ol>\n")
			}
			fmt.Fprintf(&written, "    <p class=\"section\">%s</p>\n    <ol>\n", html.EscapeString(page.Section))
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
	fmt.Fprintf(&written, "    <nav class=\"pagination\" aria-label=%q>\n", portuguese.Pages)
	if index > 0 {
		fmt.Fprintf(&written, "      <a class=\"previous\" href=%q><span>%s</span>%s</a>\n",
			fileOf(pages[index-1], index-1), portuguese.Previous, html.EscapeString(pages[index-1].Title))
	}
	if index+1 < len(pages) {
		fmt.Fprintf(&written, "      <a class=\"next\" href=%q><span>%s</span>%s</a>\n",
			fileOf(pages[index+1], index+1), portuguese.Next, html.EscapeString(pages[index+1].Title))
	}
	written.WriteString("    </nav>\n")
	return written.String()
}

var sectionHeading = regexp.MustCompile(`(?m)^(##|###) +(.+)$`)

func tableOfContents(page Page) string {
	matches := sectionHeading.FindAllStringSubmatch(page.Markdown, -1)
	if page.WithoutContents || len(matches) < 3 {
		return ""
	}
	var written strings.Builder
	fmt.Fprintf(&written, "  <aside class=\"contents\" aria-label=%q>\n    <p class=\"section\">%s</p>\n    <ol>\n",
		portuguese.OnThisPage, portuguese.OnThisPage)
	for _, match := range matches {
		level := "level-2"
		if match[1] == "###" {
			level = "level-3"
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
