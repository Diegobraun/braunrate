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
			t.Errorf("%s: o cenario publicado nao carrega: %v\n%s", block.where, err, block.code)
			continue
		}
		if err := spec.Validate(); err != nil {
			t.Errorf("%s: o cenario publicado nao passa na validacao: %v\n%s", block.where, err, block.code)
			continue
		}
		checked++
	}
	if checked == 0 {
		t.Log("nenhum cenario completo publicado; os trechos sao conferidos pelo teste seguinte")
	}
}

// Trecho e recorte de cenario, entao ele nao valida sozinho: falta o alvo, falta
// a carga, e as referencias vem de blocos que o texto ja mostrou antes. O que da
// para conferir — e o que envelhece — sao os nomes: chave de topo que deixou de
// existir e protocolo que saiu do binario.
func TestEveryFragmentUsesKeysThatStillExist(t *testing.T) {
	blocks := allBlocks(t, "yaml trecho")
	if len(blocks) == 0 {
		t.Fatal("nenhum trecho encontrado: o teste nao estaria provando nada")
	}
	for _, block := range blocks {
		var document map[string]yaml.Node
		if err := yaml.Unmarshal([]byte(block.code), &document); err != nil {
			t.Errorf("%s: o trecho nao e YAML valido: %v\n%s", block.where, err, block.code)
			continue
		}
		for key, value := range document {
			if !slices.Contains(scenario.TopKeys, key) {
				t.Errorf("%s: %q nao e chave de topo do cenario; aceitas: %s",
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
				t.Errorf("%s: %q nao e chave de passo nem protocolo compilado; aceitos: %s",
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
		t.Fatalf("a referencia nao foi gerada: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("nao consegui ler o schema: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o schema nao carrega: %v", err)
	}

	missing := 0
	for _, key := range schemaKeys(document) {
		if !strings.Contains(page.Markdown, "`"+key+"`") {
			t.Errorf("a chave %q existe no schema e nao aparece na referencia publicada", key)
			missing++
		}
	}
	if missing == 0 && len(schemaKeys(document)) == 0 {
		t.Fatal("nao achei chave nenhuma no schema: o teste nao estaria provando nada")
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
		t.Fatalf("o site nao foi gerado: %v", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("nao consegui ler o site gerado: %v", err)
	}

	// Uma ancora para o GitHub e o leitor clicando, nao a pagina buscando. O que
	// nao pode existir e recurso carregado sozinho.
	fetching := regexp.MustCompile(`(?i)(<script|<img|<iframe|@import|url\(\s*['"]?https?:|<link[^>]+href="https?:)`)
	pages := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".html") && !strings.HasSuffix(entry.Name(), ".css") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(destination, entry.Name()))
		if err != nil {
			t.Fatalf("nao consegui ler %s: %v", entry.Name(), err)
		}
		if found := fetching.FindString(string(content)); found != "" {
			t.Errorf("%s busca recurso externo: %q", entry.Name(), found)
		}
		pages++
	}
	if pages == 0 {
		t.Fatal("nenhuma pagina gerada")
	}
}

func TestEveryPageHasATitleAndABody(t *testing.T) {
	pages, err := site.Pages(root)
	if err != nil {
		t.Fatalf("nao consegui montar as paginas: %v", err)
	}
	if len(pages) < 8 {
		t.Fatalf("o site tem so %d paginas; a estrutura minima tem inicio, instalacao, primeiros passos, conceitos, referencia, protocolos, receitas, comandos, problemas e decisoes", len(pages))
	}
	for _, page := range pages {
		if strings.TrimSpace(page.Title) == "" || strings.TrimSpace(page.Slug) == "" {
			t.Errorf("pagina sem titulo ou sem endereco: %+v", page)
		}
		if len(page.Markdown) < 200 {
			t.Errorf("a pagina %q esta praticamente vazia", page.Slug)
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
		t.Fatalf("nao consegui ler %s: %v", directory, err)
	}
	var found []block
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("nao consegui ler %s: %v", entry.Name(), err)
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
		t.Fatalf("o site nao foi gerado: %v", err)
	}

	pages := map[string]string{}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("nao consegui ler o site gerado: %v", err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(destination, entry.Name()))
		if err != nil {
			t.Fatalf("nao consegui ler %s: %v", entry.Name(), err)
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
				t.Errorf("%s aponta para %q, que o site nao publica", name, target)
				continue
			}
			if anchor != "" && !strings.Contains(destinationContent, `id="`+anchor+`"`) {
				t.Errorf("%s aponta para %q, e essa ancora nao existe em %s", name, target, file)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("nenhum link interno encontrado: o teste nao estaria provando nada")
	}
}
