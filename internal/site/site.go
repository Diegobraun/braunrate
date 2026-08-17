// Package site turns the guides in docs/guides into the published site. The
// content lives in the repository and travels through pull request like the
// code does; the site is a projection of it, never a second copy that someone
// has to remember to update.
package site

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
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

const (
	guidesDirectory = "docs/guides"
	baseURL         = "https://diegobraun.github.io/braunrate/"
)

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
	// Traducao que saiu de uma versao anterior do original.
	Stale bool
}

// Build escreve as duas linguas e devolve os avisos de tradução atrasada. A
// build nao reprova por causa deles: texto que envelheceu ainda ajuda mais do
// que pagina que sumiu, e a propria pagina diz ao leitor o que aconteceu.
func Build(repositoryRoot, destination string, version string) ([]string, error) {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return nil, fmt.Errorf("could not create %s: %w", destination, err)
	}
	highlight, err := highlightStyles()
	if err != nil {
		return nil, fmt.Errorf("could not generate the syntax highlighting: %w", err)
	}
	for name, content := range map[string]string{"style.css": stylesheet + highlight, "page.js": script} {
		if err := os.WriteFile(filepath.Join(destination, name), []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("could not write %s: %w", name, err)
		}
	}

	var warnings []string
	for _, language := range Languages {
		pages, stale, err := Pages(repositoryRoot, language)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, stale...)
		if err := writeLanguage(destination, language, pages, version); err != nil {
			return nil, err
		}
	}
	// GitHub Pages serves through Jekyll unless told otherwise, and Jekyll
	// silently drops files it does not understand.
	return warnings, os.WriteFile(filepath.Join(destination, ".nojekyll"), nil, 0o644)
}

func writeLanguage(destination string, language Language, pages []Page, version string) error {
	directory := filepath.Join(destination, language.Directory)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", directory, err)
	}
	// Um indice por lingua: um so entregaria resultado em portugues a quem esta
	// lendo em ingles, e o trecho levaria para uma pagina que ele nao consegue
	// ler.
	index, err := searchIndex(pages)
	if err != nil {
		return fmt.Errorf("could not build the search index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "search-index.js"), []byte(index), 0o644); err != nil {
		return fmt.Errorf("could not write the search index: %w", err)
	}
	for position, page := range pages {
		source := page.Markdown
		cards, stripped, hasCards := commandCards(source, language.Text)
		if hasCards {
			source = stripped
		}
		body, err := renderMarkdown(source, language.Text)
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
		if page.Slug == "reference" {
			body = markRequired(body, language.Text)
		}
		if page.Stale {
			body = staleNotice(language) + body
		}
		content := layout(language, page, pages, position, body, version)
		if err := os.WriteFile(filepath.Join(directory, fileOf(page, position)), []byte(content), 0o644); err != nil {
			return fmt.Errorf("could not write %s: %w", page.Slug, err)
		}
	}
	return nil
}

// A coluna "obrigatoria" e a que decide o que a pessoa precisa escrever, e um
// "sim" e um "nao" em cinza igual nao separam nada a distancia de leitura. A
// troca vale so na referencia, que e a unica pagina com essa coluna.
func markRequired(body string, text chrome) string {
	body = strings.ReplaceAll(body, "<td>"+text.ReferenceRequired[0]+"</td>",
		`<td class="required">`+text.ReferenceRequired[0]+"</td>")
	return strings.ReplaceAll(body, "<td>"+text.ReferenceRequired[1]+"</td>", `<td class="optional">—</td>`)
}

func staleNotice(language Language) string {
	return fmt.Sprintf("<aside class=\"note note-warning\"><p class=\"label\">%s</p><p>%s</p></aside>\n",
		html.EscapeString(language.Text.StaleLabel), html.EscapeString(language.Text.StaleNotice))
}

func fileOf(page Page, index int) string {
	if index == 0 {
		return "index.html"
	}
	return page.Slug + ".html"
}

// Pages devolve as paginas da lingua e os avisos de tradução atrasada.
func Pages(repositoryRoot string, language Language) ([]Page, []string, error) {
	written, warnings, err := guides(repositoryRoot, language)
	if err != nil {
		return nil, nil, err
	}
	reference, err := ReferencePage(repositoryRoot, language)
	if err != nil {
		return nil, nil, err
	}
	decisions, err := DecisionsPage(repositoryRoot, language)
	if err != nil {
		return nil, nil, err
	}
	return append(written, reference, decisions), warnings, nil
}

// O nome do arquivo decide ordem e secao; o primeiro titulo decide o nome da
// pagina. Declarar as duas coisas seria o caminho para menu e titulo
// discordarem. O sufixo decide a lingua, e o resto do nome e o mesmo nas duas:
// e assim que o seletor sabe para onde ir sem uma tabela de equivalencias.
func guides(repositoryRoot string, language Language) ([]Page, []string, error) {
	directory := filepath.Join(repositoryRoot, guidesDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read %s: %w", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), language.Suffix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("no guide ends in %q in %s", language.Suffix, directory)
	}

	var warnings []string
	pages := make([]Page, 0, len(names))
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, nil, fmt.Errorf("could not read %s: %w", name, err)
		}
		front, text, err := frontMatter(string(content), name)
		if err != nil {
			return nil, nil, err
		}
		stale := false
		if !language.Source {
			stale, err = behindTheSource(repositoryRoot, name, front)
			if err != nil {
				return nil, nil, err
			}
			if stale {
				warnings = append(warnings, fmt.Sprintf(
					"%s was translated from an older %s: run the translation again and update source_hash",
					name, front.From))
			}
		}
		title, found := firstHeading(text)
		if !found {
			return nil, nil, fmt.Errorf("%s does not start with a '# ' heading", name)
		}
		section, slug := sectionAndSlug(name, language)
		hero, text := extractHero(text)
		page := Page{
			Slug: slug, Title: title, Section: section, Summary: summary(text),
			Markdown: text, Source: guidesDirectory + "/" + name, Hero: hero, Stale: stale,
		}
		if hero != nil {
			page.Summary = hero.Motto
			page.Markdown = strings.TrimPrefix(text, "# "+title+"\n")
		}
		pages = append(pages, page)
	}
	return pages, warnings, nil
}

type translated struct {
	From string
	Hash string
}

var frontMatterBlock = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)

// Traducao sem origem declarada e traducao que ninguem consegue conferir: seis
// meses depois ninguem lembra de qual versao ela saiu, e o leitor nao tem como
// saber se o que esta lendo ainda vale.
func frontMatter(content, name string) (translated, string, error) {
	match := frontMatterBlock.FindStringSubmatch(content)
	if match == nil {
		if strings.HasSuffix(name, english.Suffix) {
			return translated{}, content, nil
		}
		return translated{}, "", fmt.Errorf(
			"%s has no front matter: a translation declares translated_from and source_hash", name)
	}
	var front translated
	for _, line := range strings.Split(match[1], "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "translated_from":
			front.From = strings.TrimSpace(value)
		case "source_hash":
			front.Hash = strings.TrimSpace(value)
		}
	}
	if front.From == "" || front.Hash == "" {
		return translated{}, "", fmt.Errorf(
			"%s declares %q and %q; a translation needs translated_from and source_hash",
			name, front.From, front.Hash)
	}
	return front, content[len(match[0]):], nil
}

func behindTheSource(repositoryRoot, name string, front translated) (bool, error) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot, guidesDirectory, front.From))
	if err != nil {
		return false, fmt.Errorf("%s was translated from %s, which does not exist: %w", name, front.From, err)
	}
	return SourceHash(content) != front.Hash, nil
}

// Doze caracteres: o suficiente para nao colidir entre dez arquivos, e curto o
// bastante para caber no cabecalho sem virar ruido.
func SourceHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])[:12]
}

func sectionAndSlug(fileName string, language Language) (string, string) {
	name := strings.TrimSuffix(fileName, language.Suffix)
	parts := strings.SplitN(name, "-", 3)
	if len(parts) < 3 {
		return language.Text.Sections["guides"], strings.Join(parts[1:], "-")
	}
	return language.Text.Sections[parts[1]], parts[2]
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

func renderMarkdown(source string, text chrome) (string, error) {
	source = headingLine.ReplaceAllStringFunc(source, func(line string) string {
		parts := headingLine.FindStringSubmatch(line)
		return fmt.Sprintf("%s %s {#%s}", parts[1], parts[2], slugify(parts[2]))
	})
	converter := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			// Em classe, com as duas paletas geradas em highlight.go: cor embutida
			// na tag so permite uma paleta, e ela ficava escura no tema claro.
			highlighting.NewHighlighting(highlighting.WithFormatOptions(chromahtml.WithClasses(true))),
		),
		goldmark.WithParserOptions(parser.WithAttribute()),
	)
	var rendered bytes.Buffer
	if err := converter.Convert([]byte(source), &rendered); err != nil {
		return "", err
	}
	return decorate(rendered.String(), text), nil
}

var (
	headingTag = regexp.MustCompile(`<(h[23])( id="([^"]+)")>(.*?)</h[23]>`)
	callout    = regexp.MustCompile(`(?s)<blockquote>\s*<p><strong>([^<]+)</strong>(.*?)</blockquote>`)
)

func decorate(page string, text chrome) string {
	page = headingTag.ReplaceAllString(page,
		fmt.Sprintf(`<$1$2>$4 <a class="anchor" href="#$3" aria-label=%q>#</a></$1>`, text.AnchorLabel))
	return callout.ReplaceAllStringFunc(page, func(found string) string {
		parts := callout.FindStringSubmatch(found)
		class, known := text.CalloutClasses[parts[1]]
		if !known {
			return found
		}
		return fmt.Sprintf(`<aside class="note note-%s"><p class="label">%s</p><p>%s</aside>`,
			class, parts[1], parts[2])
	})
}

func layout(language Language, page Page, pages []Page, index int, body, version string) string {
	text := language.Text
	title := page.Title + " — braunrate"
	if page.Title == "braunrate" {
		title = "braunrate"
	}

	hero := ""
	if page.Hero != nil {
		hero = page.Hero.render(text)
	}
	file := fileOf(page, index)

	return fmt.Sprintf(`<!doctype html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<meta name="description" content="%s">
%s<link rel="stylesheet" href="%sstyle.css">
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
  <a class="language" href="%s" hreflang="%s" lang="%s">%s</a>
  <a class="repository" href="https://github.com/Diegobraun/braunrate">GitHub</a>
</header>
<div class="page">
  <details class="menu" id="menu" open>
    <summary>%s</summary>
    <nav class="sections" aria-label="%s">
%s    </nav>
  </details>
  <main>
%s    <article%s>
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
<script src="%spage.js"></script>
</body>
</html>
`, language.Code, html.EscapeString(title), html.EscapeString(page.Summary),
		alternates(file), language.toRoot(), pageText(text),
		html.EscapeString(version), html.EscapeString(text.SearchLabel), text.Search,
		html.EscapeString(text.UseDarkTheme), text.Theme,
		otherLanguageHref(language, file), language.other().Code, language.other().Code, text.OtherLanguage,
		html.EscapeString(text.Pages), html.EscapeString(text.Sections["reference"]),
		menu(pages, index), hero, articleClass(page), body, pagination(pages, index, text), tableOfContents(page, text),
		footer(page, version, text), html.EscapeString(text.SearchLabel),
		html.EscapeString(text.Placeholder), text.SearchHint, language.toRoot())
}

// O seletor troca de lingua sem trocar de pagina: quem esta lendo protocolos em
// ingles cai em protocolos em portugues, e nao na pagina inicial.
// A referencia e a unica pagina cujas celulas carregam valor para copiar, e o
// script so liga o clique-para-copiar onde ele serve.
func articleClass(page Page) string {
	if page.Slug == "reference" {
		return ` class="reference"`
	}
	return ""
}

func otherLanguageHref(language Language, file string) string {
	other := language.other()
	if other.Directory == "" {
		return "../" + file
	}
	return other.Directory + "/" + file
}

// Sem hreflang o buscador trata as duas arvores como paginas concorrentes, e
// escolhe uma. O x-default aponta para o ingles porque e dele que as outras
// saem.
func alternates(file string) string {
	// A pagina inicial e o proprio diretorio: anunciar "/index.html" ao lado de
	// "/" daria ao buscador dois enderecos para a mesma pagina.
	if file == "index.html" {
		file = ""
	}
	var written strings.Builder
	for _, language := range Languages {
		address := baseURL + file
		if language.Directory != "" {
			address = baseURL + language.Directory + "/" + file
		}
		fmt.Fprintf(&written, "<link rel=\"alternate\" hreflang=%q href=%q>\n", language.Code, address)
	}
	fmt.Fprintf(&written, "<link rel=\"alternate\" hreflang=\"x-default\" href=%q>\n", baseURL+file)
	return written.String()
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
func footer(page Page, version string, text chrome) string {
	origin := fmt.Sprintf(`<a href="https://github.com/Diegobraun/braunrate/edit/main/%s">%s</a>`,
		html.EscapeString(page.Source), text.EditThisPage)
	if !strings.HasSuffix(page.Source, ".md") {
		origin = fmt.Sprintf(`%s <a href="https://github.com/Diegobraun/braunrate/blob/main/%s"><code>%s</code></a>`,
			text.GeneratedFrom, html.EscapeString(page.Source), html.EscapeString(page.Source))
	}
	return fmt.Sprintf(`<footer>
  <p><a href="https://github.com/Diegobraun/braunrate">%s</a> · %s</p>
  <p class="right">braunrate %s · <a href="https://github.com/Diegobraun/braunrate/blob/main/LICENSE">%s</a></p>
</footer>
`, text.Repository, origin, html.EscapeString(version), text.License)
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

func pagination(pages []Page, index int, text chrome) string {
	var written strings.Builder
	fmt.Fprintf(&written, "    <nav class=\"pagination\" aria-label=%q>\n", text.Pages)
	if index > 0 {
		fmt.Fprintf(&written, "      <a class=\"previous\" href=%q><span>%s</span>%s</a>\n",
			fileOf(pages[index-1], index-1), text.Previous, html.EscapeString(pages[index-1].Title))
	}
	if index+1 < len(pages) {
		fmt.Fprintf(&written, "      <a class=\"next\" href=%q><span>%s</span>%s</a>\n",
			fileOf(pages[index+1], index+1), text.Next, html.EscapeString(pages[index+1].Title))
	}
	written.WriteString("    </nav>\n")
	return written.String()
}

var sectionHeading = regexp.MustCompile(`(?m)^(##|###) +(.+)$`)

func tableOfContents(page Page, text chrome) string {
	matches := sectionHeading.FindAllStringSubmatch(page.Markdown, -1)
	if page.WithoutContents || len(matches) < 3 {
		return ""
	}
	var written strings.Builder
	fmt.Fprintf(&written, "  <aside class=\"contents\" aria-label=%q>\n    <p class=\"section\">%s</p>\n    <ol>\n",
		text.OnThisPage, text.OnThisPage)
	for _, match := range matches {
		level := "level-2"
		if match[1] == "###" {
			level = "level-3"
		}
		heading := strings.TrimSpace(match[2])
		fmt.Fprintf(&written, "      <li class=%q><a href=\"#%s\">%s</a></li>\n",
			level, slugify(heading), html.EscapeString(plain(heading)))
	}
	written.WriteString("    </ol>\n  </aside>\n")
	return written.String()
}

var markup = regexp.MustCompile("`|\\*\\*|\\*|_")

func plain(text string) string { return markup.ReplaceAllString(text, "") }
