package site_test

import (
	"encoding/json"
	"os"
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
	blocks := allBlocks(t, "yaml trecho")
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
			if key == "cenario" {
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
	page, err := site.ReferencePage(root)
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
	destination := t.TempDir()
	if err := site.Build(root, destination, "teste"); err != nil {
		t.Fatalf("o site não foi gerado: %v", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("não consegui ler o site gerado: %v", err)
	}

	// Uma ancora para o GitHub e o leitor clicando, nao a pagina buscando. O que
	// nao pode existir e recurso carregado de outro servidor.
	fetching := regexp.MustCompile(`(?i)(<script[^>]+src="https?:|<img|<iframe|@import|url\(\s*['"]?https?:|<link[^>]+href="https?:` +
		`|XMLHttpRequest|importScripts|fetch\(\s*['"` + "`" + `]?https?:)`)
	pages := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".js") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(destination, entry.Name()))
		if err != nil {
			t.Fatalf("não consegui ler %s: %v", entry.Name(), err)
		}
		if found := fetching.FindString(string(content)); found != "" {
			t.Errorf("%s busca recurso externo: %q", entry.Name(), found)
		}
		pages++
	}
	if pages == 0 {
		t.Fatal("nenhuma página gerada")
	}
}

func TestEveryPageHasATitleAndABody(t *testing.T) {
	pages, err := site.Pages(root)
	if err != nil {
		t.Fatalf("não consegui montar as páginas: %v", err)
	}
	if len(pages) < 8 {
		t.Fatalf("o site tem só %d páginas; a estrutura minima tem inicio, instalacao, primeiros passos, conceitos, referência, protocolos, receitas, comandos, problemas e decisões", len(pages))
	}
	for _, page := range pages {
		if strings.TrimSpace(page.Title) == "" || strings.TrimSpace(page.Slug) == "" {
			t.Errorf("página sem título ou sem endereço: %+v", page)
		}
		if len(page.Markdown) < 200 {
			t.Errorf("a página %q esta praticamente vazia", page.Slug)
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
	directory := filepath.Join(root, "docs", "guias")
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
	destination := t.TempDir()
	if err := site.Build(root, destination, "teste"); err != nil {
		t.Fatalf("o site não foi gerado: %v", err)
	}

	pages := map[string]string{}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("não consegui ler o site gerado: %v", err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(destination, entry.Name()))
		if err != nil {
			t.Fatalf("não consegui ler %s: %v", entry.Name(), err)
		}
		pages[entry.Name()] = string(content)
	}

	link := regexp.MustCompile(`href="([^"]+)"`)
	checked := 0
	for name, content := range pages {
		for _, match := range link.FindAllStringSubmatch(content, -1) {
			target := match[1]
			if strings.HasPrefix(target, "http") || strings.HasPrefix(target, "mailto:") || target == "estilo.css" {
				continue
			}
			file, anchor, _ := strings.Cut(target, "#")
			if file == "" {
				file = name
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

func build(t *testing.T) map[string]string {
	t.Helper()
	destination := t.TempDir()
	if err := site.Build(root, destination, "teste"); err != nil {
		t.Fatalf("o site não foi gerado: %v", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("não consegui ler o site gerado: %v", err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(destination, entry.Name()))
		if err != nil {
			t.Fatalf("não consegui ler %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(content)
	}
	return files
}

// A dobra e o que decide se a pessoa fica na pagina, e o numero dela e o mesmo
// da tabela que o resto da pagina mostra: dois lugares com o numero e um lugar
// para ele envelhecer sozinho.
func TestTheFoldSaysWhatWhyAndTheFirstCommand(t *testing.T) {
	files := build(t)
	inicio := files["index.html"]

	fold, _, found := strings.Cut(inicio, `<h2 id="comecar"`)
	if !found {
		t.Fatal("a página inicial não tem a primeira seção depois da dobra")
	}
	for _, esperado := range []string{
		"Teste de carga que não mente sobre o próprio resultado.",
		"braunrate demo",
		"983,0 ms",
		"3,7 ms",
		"979,4 ms",
	} {
		if !strings.Contains(fold, esperado) {
			t.Errorf("a dobra não traz %q", esperado)
		}
	}
	for _, jargao := range []string{"HDR", "back-pressure", "modelo de chegada aberto"} {
		if strings.Contains(fold, jargao) {
			t.Errorf("a dobra usa %q sem tradução: quem chega não conhece o termo", jargao)
		}
	}
	body := inicio[strings.Index(inicio, `<h2 id="comecar"`):]
	for _, numero := range []string{"983,0 ms", "3,7 ms", "979,4 ms"} {
		if !strings.Contains(body, numero) {
			t.Errorf("a dobra mostra %s e a página não mostra esse número em lugar nenhum", numero)
		}
	}
}

// Contraste conferido, e nao estimado: a paleta do destaque de sintaxe nao foi
// desenhada para este fundo, e o comentario cinza nasce fora do AA.
func TestEveryColorMeetsAA(t *testing.T) {
	css := build(t)["estilo.css"]
	claro := themeTokens(t, css, ":root {")
	escuro := themeTokens(t, css, `:root[data-tema="escuro"] {`)

	pairs := []struct{ text, background, what string }{
		{"texto", "fundo", "texto do corpo"},
		{"suave", "fundo", "texto secundário"},
		{"suave", "fundo-suave", "texto secundário em cartão"},
		{"suave", "fundo-fundo", "texto do rodapé"},
		{"marca", "fundo", "link"},
		{"marca", "fundo-suave", "número da prova"},
		{"marca", "marca-suave", "item ativo da navegação"},
	}
	for theme, tokens := range map[string]map[string]string{"claro": claro, "escuro": escuro} {
		for _, pair := range pairs {
			ratio := site.Contrast(tokens[pair.text], tokens[pair.background])
			if ratio < 4.5 {
				t.Errorf("tema %s: %s tem %.2f:1 (%s sobre %s), abaixo do AA",
					theme, pair.what, ratio, tokens[pair.text], tokens[pair.background])
			}
		}
	}

	for _, palette := range []struct{ prefix, background string }{
		{".chroma", claro["fundo-codigo"]},
		{`:root:not([data-tema="claro"]) .chroma`, escuro["fundo-codigo"]},
		{`:root[data-tema="escuro"] .chroma`, escuro["fundo-codigo"]},
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
	comandos := files["comandos.html"]

	source, err := os.ReadFile(filepath.Join(root, "docs", "guias", "60-guias-comandos.md"))
	if err != nil {
		t.Fatalf("não consegui ler o guia de comandos: %v", err)
	}
	rows := regexp.MustCompile("(?m)^\\| \\[`([a-z]+)`\\]").FindAllStringSubmatch(string(source), -1)
	if len(rows) < 10 {
		t.Fatalf("achei %d comandos na tabela: o teste não estaria provando nada", len(rows))
	}
	for _, row := range rows {
		if !strings.Contains(comandos, `<p class="nome"><code>`+row[1]+`</code></p>`) {
			t.Errorf("o comando %q não virou cartão", row[1])
		}
	}
	if strings.Contains(comandos, "CARTOES-DE-COMANDO") {
		t.Error("o marcador dos cartões ficou visível na página")
	}
	if strings.Count(comandos, `class="cartao"`) != len(rows) {
		t.Errorf("%d cartões para %d comandos", strings.Count(comandos, `class="cartao"`), len(rows))
	}
}

// A busca precisa abrir em rede fechada como o resto do site: o indice viaja
// junto, gerado das mesmas paginas.
func TestSearchTravelsWithTheSite(t *testing.T) {
	files := build(t)
	index, exists := files["indice.js"]
	if !exists {
		t.Fatal("o site não publica índice de busca")
	}
	payload, found := strings.CutPrefix(strings.TrimSpace(index), "window.INDICE=")
	if !found {
		t.Fatal("o índice não declara window.INDICE")
	}
	var raw []map[string]string
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("o índice não é JSON: %v", err)
	}
	if len(raw) < 50 {
		t.Fatalf("o índice tem %d entradas para doze páginas", len(raw))
	}

	pages := map[string]bool{}
	for _, entry := range raw {
		pages[entry["p"]] = true
		if entry["a"] != "" && !strings.Contains(files[entry["p"]], `id="`+entry["a"]+`"`) {
			t.Errorf("o índice aponta %s#%s, e essa âncora não existe", entry["p"], entry["a"])
		}
	}
	for name := range files {
		if strings.HasSuffix(name, ".html") && !pages[name] {
			t.Errorf("%s não entrou no índice de busca", name)
		}
	}
	if !strings.Contains(files["pagina.js"], "window.INDICE") {
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
		if !strings.Contains(content, "editar esta página") && !strings.Contains(content, "gerada de") {
			t.Errorf("%s não diz de onde o conteúdo dela sai", name)
		}
		if !strings.Contains(content, "licença MIT") {
			t.Errorf("%s não traz a licença", name)
		}
	}
}
