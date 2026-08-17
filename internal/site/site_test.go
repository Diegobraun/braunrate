package site_test

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/site"
	"gopkg.in/yaml.v3"
)

const root = "../.."

// Documentacao com bloco que nao roda e pior que documentacao ausente: quem
// copia perde tempo procurando o proprio erro. Um bloco marcado apenas ```yaml
// e um cenario inteiro, e passa pelo mesmo parser e pela mesma validacao que o
// comando usa.
func TestEveryScenarioBlockIsAScenarioTheToolAccepts(t *testing.T) {
	checked := 0
	for _, block := range allBlocks(t, "yaml") {
		spec, err := scenario.Parse([]byte(block.code))
		if err != nil {
			t.Errorf("%s: o cenário publicado não carrega: %v\n%s", block.where, err, block.code)
			continue
		}
		if err := spec.Validate(); err != nil {
			t.Errorf("%s: o cenário publicado não passa na validação: %v\n%s", block.where, err, block.code)
			continue
		}
		checked++
	}
	if checked == 0 {
		t.Log("nenhum cenário completo publicado; os trechos são conferidos pelo teste seguinte")
	}
}

// Trecho e recorte de cenario, entao ele nao valida sozinho: falta o alvo, falta
// a carga, e as referencias vem de blocos que o texto ja mostrou antes. O que da
// para conferir — e o que envelhece — sao os nomes: chave de topo que deixou de
// existir e protocolo que saiu do binario.
func TestEveryFragmentUsesKeysThatStillExist(t *testing.T) {
	blocks := allBlocks(t, "yaml fragment")
	if len(blocks) == 0 {
		t.Fatal("nenhum trecho encontrado: o teste não estaria provando nada")
	}
	for _, block := range blocks {
		var document map[string]yaml.Node
		if err := yaml.Unmarshal([]byte(block.code), &document); err != nil {
			t.Errorf("%s: o trecho não e YAML válido: %v\n%s", block.where, err, block.code)
			continue
		}
		for key, value := range document {
			if !slices.Contains(scenario.TopKeys, key) {
				t.Errorf("%s: %q não e chave de topo do cenário; aceitas: %s",
					block.where, key, strings.Join(scenario.TopKeys, ", "))
			}
			if key == "scenario" {
				checkSteps(t, block.where, value)
			}
		}
	}
}

func checkSteps(t *testing.T, where string, steps yaml.Node) {
	t.Helper()
	valid := append(slices.Clone(scenario.StepKeys), protocol.Registered()...)
	for _, step := range steps.Content {
		for index := 0; index+1 < len(step.Content); index += 2 {
			key := step.Content[index].Value
			if !slices.Contains(valid, key) {
				t.Errorf("%s: %q não e chave de passo nem protocolo compilado; aceitos: %s",
					where, key, strings.Join(valid, ", "))
			}
		}
	}
}

// A referencia sai do schema, e o schema tem teste que reprova o build se ele
// oferecer chave que o parser recusa ou esquecer chave que o parser aceita. Este
// fecha a corrente do outro lado: chave que existe no schema e nao chegou na
// pagina.
func TestEverySchemaKeyReachesTheReference(t *testing.T) {
	page, err := site.ReferencePage(root, site.Languages[0])
	if err != nil {
		t.Fatalf("a referência não foi gerada: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("não consegui ler o schema: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o schema não carrega: %v", err)
	}

	missing := 0
	for _, key := range schemaKeys(document) {
		if !strings.Contains(page.Markdown, "`"+key+"`") {
			t.Errorf("a chave %q existe no schema e não aparece na referência publicada", key)
			missing++
		}
	}
	if missing == 0 && len(schemaKeys(document)) == 0 {
		t.Fatal("não achei chave nenhuma no schema: o teste não estaria provando nada")
	}
}

func schemaKeys(node any) []string {
	var keys []string
	switch typed := node.(type) {
	case map[string]any:
		for name, value := range typed {
			if name == "properties" {
				if properties, is := value.(map[string]any); is {
					for key := range properties {
						keys = append(keys, key)
					}
				}
			}
			keys = append(keys, schemaKeys(value)...)
		}
	case []any:
		for _, item := range typed {
			keys = append(keys, schemaKeys(item)...)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// A regra do relatorio HTML — nada buscado da rede — vale para o site pelo mesmo
// motivo: ele precisa abrir em rede fechada, e uma fonte ou um script de CDN
// entrega ao terceiro a lista de quem leu a documentacao.
func TestThePagesFetchNothingFromTheNetwork(t *testing.T) {
	files := build(t)

	// Uma ancora para o GitHub e o leitor clicando, nao a pagina buscando. O que
	// nao pode existir e recurso carregado de outro servidor.
	fetching := regexp.MustCompile(`(?i)(<script[^>]+src="https?:|<img|<iframe|@import|url\(\s*['"]?https?:|<link[^>]+href="https?:` +
		`|XMLHttpRequest|importScripts|fetch\(\s*['"` + "`" + `]?https?:)`)
	// O hreflang e endereco declarado para o buscador, nao recurso que a pagina
	// carrega: ele fica de fora da conta como a ancora para o GitHub fica.
	alternate := regexp.MustCompile(`<link rel="alternate"[^>]*>\n?`)

	pages := 0
	for name, content := range files {
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".js") {
			continue
		}
		if found := fetching.FindString(alternate.ReplaceAllString(content, "")); found != "" {
			t.Errorf("%s busca recurso externo: %q", name, found)
		}
		pages++
	}
	if pages == 0 {
		t.Fatal("nenhuma página gerada")
	}
}

func TestEveryPageHasATitleAndABody(t *testing.T) {
	for _, language := range site.Languages {
		pages, _, err := site.Pages(root, language)
		if err != nil {
			t.Fatalf("%s: não consegui montar as páginas: %v", language.Code, err)
		}
		if len(pages) < 10 {
			t.Fatalf("%s: o site tem só %d páginas; a estrutura minima tem inicio, instalacao, primeiros passos, conceitos, protocolos, receitas, comandos, problemas, referência e decisões",
				language.Code, len(pages))
		}
		for _, page := range pages {
			if strings.TrimSpace(page.Title) == "" || strings.TrimSpace(page.Slug) == "" {
				t.Errorf("%s: página sem título ou sem endereço: %+v", language.Code, page)
			}
			if len(page.Markdown) < 200 {
				t.Errorf("%s: a página %q esta praticamente vazia", language.Code, page.Slug)
			}
		}
	}
}

type block struct {
	where string
	code  string
}

var fence = regexp.MustCompile("(?s)```([^\n]*)\n(.*?)```")

func allBlocks(t *testing.T, language string) []block {
	t.Helper()
	directory := filepath.Join(root, "docs", "guides")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("não consegui ler %s: %v", directory, err)
	}
	var found []block
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("não consegui ler %s: %v", entry.Name(), err)
		}
		for _, match := range fence.FindAllStringSubmatch(string(content), -1) {
			if strings.TrimSpace(match[1]) == language {
				found = append(found, block{where: entry.Name(), code: match[2]})
			}
		}
	}
	return found
}

// Link interno quebrado e o defeito que mais aparece em site de documentacao e
// menos aparece em revisao: ninguem clica em tudo. A pagina destino tem que
// existir, e a ancora tambem.
func TestEveryInternalLinkResolves(t *testing.T) {
	pages := build(t)

	link := regexp.MustCompile(`href="([^"]+)"`)
	checked := 0
	for name, content := range pages {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		for _, match := range link.FindAllStringSubmatch(content, -1) {
			target := match[1]
			if strings.HasPrefix(target, "http") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, anchor, _ := strings.Cut(target, "#")
			if file == "" {
				file = name
			} else {
				file = path.Join(path.Dir(name), file)
			}
			if strings.HasSuffix(file, ".css") || strings.HasSuffix(file, ".js") {
				if _, exists := pages[file]; !exists {
					t.Errorf("%s aponta para %q, que o site não publica", name, target)
				}
				continue
			}
			destinationContent, exists := pages[file]
			if !exists {
				t.Errorf("%s aponta para %q, que o site não publica", name, target)
				continue
			}
			if anchor != "" && !strings.Contains(destinationContent, `id="`+anchor+`"`) {
				t.Errorf("%s aponta para %q, e essa ancora não existe em %s", name, target, file)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("nenhum link interno encontrado: o teste não estaria provando nada")
	}
}

// O site tem duas arvores, e o nome de cada arquivo aqui e o caminho relativo a
// raiz: "index.html" e o ingles, "pt-BR/index.html" e o portugues.
func build(t *testing.T) map[string]string {
	t.Helper()
	files, _ := buildWithWarnings(t)
	return files
}

func buildWithWarnings(t *testing.T) (map[string]string, []string) {
	t.Helper()
	destination := t.TempDir()
	warnings, err := site.Build(root, destination, "teste")
	if err != nil {
		t.Fatalf("o site não foi gerado: %v", err)
	}
	files := map[string]string{}
	err = filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("não consegui ler o site gerado: %v", err)
	}
	return files, warnings
}

// A dobra e o que decide se a pessoa fica na pagina, e o numero dela e o mesmo
// da tabela que o resto da pagina mostra: dois lugares com o numero e um lugar
// para ele envelhecer sozinho.
func TestTheFoldSaysWhatWhyAndTheFirstCommand(t *testing.T) {
	files := build(t)

	for _, fold := range []struct {
		page, firstSection string
		expected, jargon   []string
	}{
		{
			page: "index.html", firstSection: `<h2 id="start"`,
			expected: []string{"Load testing that does not lie about its own result.", "braunrate demo",
				"972.3 ms", "3.5 ms", "968.8 ms"},
			jargon: []string{"HDR", "back-pressure", "open arrival model"},
		},
		{
			page: "pt-BR/index.html", firstSection: `<h2 id="comecar"`,
			expected: []string{"Teste de carga que não mente sobre o próprio resultado.", "braunrate demo",
				"972,3 ms", "3,5 ms", "968,8 ms"},
			jargon: []string{"HDR", "back-pressure", "modelo de chegada aberto"},
		},
	} {
		page := files[fold.page]
		above, below, found := strings.Cut(page, fold.firstSection)
		if !found {
			t.Fatalf("%s não tem a primeira seção depois da dobra", fold.page)
		}
		for _, expected := range fold.expected {
			if !strings.Contains(above, expected) {
				t.Errorf("%s: a dobra não traz %q", fold.page, expected)
			}
		}
		for _, jargon := range fold.jargon {
			if strings.Contains(above, jargon) {
				t.Errorf("%s: a dobra usa %q sem tradução: quem chega não conhece o termo", fold.page, jargon)
			}
		}
		for _, number := range fold.expected[2:] {
			if !strings.Contains(below, number) {
				t.Errorf("%s: a dobra mostra %s e a página não mostra esse número em lugar nenhum", fold.page, number)
			}
		}
	}
}

// Contraste conferido, e nao estimado: a paleta do destaque de sintaxe nao foi
// desenhada para este fundo, e o comentario cinza nasce fora do AA.
func TestEveryColorMeetsAA(t *testing.T) {
	css := build(t)["style.css"]
	light := themeTokens(t, css, ":root {")
	dark := themeTokens(t, css, `:root[data-theme="dark"] {`)

	pairs := []struct{ text, background, what string }{
		{"text", "background", "texto do corpo"},
		{"soft", "background", "texto secundário"},
		{"soft", "background-soft", "texto secundário em cartão"},
		{"soft", "background-deep", "texto do rodapé"},
		{"brand", "background", "link"},
		{"brand", "background-soft", "número da prova"},
		{"brand", "brand-soft", "item ativo da navegação"},
	}
	for theme, tokens := range map[string]map[string]string{"claro": light, "escuro": dark} {
		for _, pair := range pairs {
			ratio := site.Contrast(tokens[pair.text], tokens[pair.background])
			if ratio < 4.5 {
				t.Errorf("tema %s: %s tem %.2f:1 (%s sobre %s), abaixo do AA",
					theme, pair.what, ratio, tokens[pair.text], tokens[pair.background])
			}
		}
	}

	for _, palette := range []struct{ prefix, background string }{
		{".chroma", light["background-code"]},
		{`:root:not([data-theme="light"]) .chroma`, dark["background-code"]},
		{`:root[data-theme="dark"] .chroma`, dark["background-code"]},
	} {
		checked := 0
		for _, rule := range strings.Split(css, "\n") {
			selector, declarations, isRule := strings.Cut(rule, "{")
			if _, after, hasComment := strings.Cut(selector, "*/"); hasComment {
				selector = after
			}
			selector = strings.TrimSpace(selector)
			if !isRule || !strings.HasPrefix(selector, palette.prefix) ||
				strings.Contains(declarations, "background-color") {
				continue
			}
			for _, match := range textColor.FindAllStringSubmatch(declarations, -1) {
				ratio := site.Contrast(match[1], palette.background)
				if ratio < 4.5 {
					t.Errorf("destaque de sintaxe: %s tem %.2f:1 sobre %s em %q",
						match[1], ratio, palette.background, strings.TrimSpace(selector))
				}
				checked++
			}
		}
		if checked == 0 {
			t.Errorf("nenhuma cor conferida em %q: o teste não estaria provando nada", palette.prefix)
		}
	}
}

var textColor = regexp.MustCompile(`(?:^|[^-])color: (#[0-9a-fA-F]{6})`)

func themeTokens(t *testing.T, css, opening string) map[string]string {
	t.Helper()
	_, after, found := strings.Cut(css, opening)
	if !found {
		t.Fatalf("a folha não tem o bloco %q", opening)
	}
	block, _, _ := strings.Cut(after, "}")
	tokens := map[string]string{}
	for _, match := range regexp.MustCompile(`--([a-z-]+): (#[0-9a-fA-F]{3,6})`).FindAllStringSubmatch(block, -1) {
		tokens[match[1]] = match[2]
	}
	if len(tokens) < 8 {
		t.Fatalf("%q rendeu %d cores; esperava a paleta inteira", opening, len(tokens))
	}
	return tokens
}

// Os cartoes saem da tabela de comandos da propria pagina: uma segunda lista
// escrita a mao seria a lista que envelhece.
func TestTheCommandIndexBecomesCards(t *testing.T) {
	files := build(t)
	for page, guide := range map[string]string{
		"commands.html":       "60-guides-commands.en.md",
		"pt-BR/commands.html": "60-guides-commands.pt-BR.md",
	} {
		rendered := files[page]
		source, err := os.ReadFile(filepath.Join(root, "docs", "guides", guide))
		if err != nil {
			t.Fatalf("não consegui ler o guia de comandos: %v", err)
		}
		rows := regexp.MustCompile("(?m)^\\| \\[`([a-z]+)`\\]").FindAllStringSubmatch(string(source), -1)
		if len(rows) < 10 {
			t.Fatalf("achei %d comandos na tabela de %s: o teste não estaria provando nada", len(rows), guide)
		}
		for _, row := range rows {
			if !strings.Contains(rendered, `<p class="name"><code>`+row[1]+`</code></p>`) {
				t.Errorf("%s: o comando %q não virou cartão", page, row[1])
			}
		}
		if strings.Contains(rendered, "COMMAND-CARDS") {
			t.Errorf("%s: o marcador dos cartões ficou visível na página", page)
		}
		if strings.Count(rendered, `class="card"`) != len(rows) {
			t.Errorf("%s: %d cartões para %d comandos", page, strings.Count(rendered, `class="card"`), len(rows))
		}
	}
}

// A busca precisa abrir em rede fechada como o resto do site: o indice viaja
// junto, gerado das mesmas paginas.
func TestSearchTravelsWithTheSite(t *testing.T) {
	files := build(t)
	// Um indice por lingua: um so entregaria resultado em portugues a quem esta
	// lendo em ingles.
	for _, directory := range []string{"", "pt-BR/"} {
		index, exists := files[directory+"search-index.js"]
		if !exists {
			t.Fatalf("o site não publica índice de busca em %q", directory)
		}
		payload, found := strings.CutPrefix(strings.TrimSpace(index), "window.SEARCH_INDEX=")
		if !found {
			t.Fatalf("o índice de %q não declara window.SEARCH_INDEX", directory)
		}
		var raw []map[string]string
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			t.Fatalf("o índice de %q não é JSON: %v", directory, err)
		}
		if len(raw) < 50 {
			t.Fatalf("o índice de %q tem %d entradas para dez páginas", directory, len(raw))
		}

		pages := map[string]bool{}
		for _, entry := range raw {
			pages[entry["p"]] = true
			if entry["a"] != "" && !strings.Contains(files[directory+entry["p"]], `id="`+entry["a"]+`"`) {
				t.Errorf("o índice aponta %s%s#%s, e essa âncora não existe", directory, entry["p"], entry["a"])
			}
		}
		for name := range files {
			file, isThisLanguage := strings.CutPrefix(name, directory)
			if !isThisLanguage || !strings.HasSuffix(file, ".html") || strings.Contains(file, "/") {
				continue
			}
			if !pages[file] {
				t.Errorf("%s não entrou no índice de busca", name)
			}
		}
	}
	if !strings.Contains(files["page.js"], "window.SEARCH_INDEX") {
		t.Error("a página não usa o índice publicado")
	}
}

func TestEveryPageSaysWhereItComesFrom(t *testing.T) {
	for name, content := range build(t) {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		if !strings.Contains(content, "<footer>") {
			t.Errorf("%s não tem rodapé", name)
		}
		if !strings.Contains(content, "edit this page") && !strings.Contains(content, "generated from") &&
			!strings.Contains(content, "editar esta página") && !strings.Contains(content, "gerada de") {
			t.Errorf("%s não diz de onde o conteúdo dela sai", name)
		}
		if !strings.Contains(content, "MIT license") && !strings.Contains(content, "licença MIT") {
			t.Errorf("%s não traz a licença", name)
		}
	}
}

// Medido em 375 px antes desta regra: o menu ocupava a primeira tela e o titulo
// so comecava a 629 px. Recolher e uma decisao que some sem alarde, porque nao
// quebra nada quando volta a nascer aberto.
func TestTheMenuArrivesFoldedOnANarrowScreen(t *testing.T) {
	built := build(t)
	page := built["index.html"]

	if !strings.Contains(page, `<details class="menu" id="menu" open>`) {
		t.Error("o menu não é recolhível, ou não nasce aberto para quem está sem script")
	}
	if !strings.Contains(page, "<summary>") {
		t.Error("o menu recolhido não tem barra para abrir")
	}
	if !strings.Contains(built["page.js"], "menu.open = false") {
		t.Error("nada recolhe o menu no estreito")
	}

	narrow := narrowRules(t, built["style.css"])
	if !strings.Contains(narrow, "details.menu > summary {") {
		t.Error("a barra de abrir não aparece no estreito")
	}
	for _, control := range []string{"nav.sections a", "header .brand", ".block .copy", ".anchor"} {
		if !strings.Contains(narrow, control) {
			t.Errorf("%s não ganha alvo de toque no estreito", control)
		}
	}
	for _, rule := range regexp.MustCompile(`min-height:\s*(\d+)px`).FindAllStringSubmatch(narrow, -1) {
		if rule[1] != "44" {
			t.Errorf("alvo de toque de %spx no estreito; a recomendação das duas plataformas é 44", rule[1])
		}
	}
}

func narrowRules(t *testing.T, css string) string {
	t.Helper()
	start := strings.Index(css, "@media (max-width: 860px) {")
	if start < 0 {
		t.Fatal("a folha não tem a faixa estreita")
	}
	depth := 0
	for index := start; index < len(css); index++ {
		switch css[index] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return css[start : index+1]
			}
		}
	}
	t.Fatal("a faixa estreita não fecha")
	return ""
}
