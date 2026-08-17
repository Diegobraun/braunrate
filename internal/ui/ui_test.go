package ui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/ui"
)

func get(t *testing.T, address, path string) (int, string) {
	t.Helper()
	response, err := http.Get(address + path)
	if err != nil {
		t.Fatalf("não consegui pedir %s: %v", path, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("não consegui ler %s: %v", path, err)
	}
	return response.StatusCode, string(body)
}

func serve(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(ui.Handler())
	t.Cleanup(server.Close)
	return server.URL
}

func TestTheInterfaceTravelsInsideTheBinary(t *testing.T) {
	address := serve(t)

	for _, file := range []string{"/", "/style.css", "/app.js"} {
		status, body := get(t, address, file)
		if status != http.StatusOK || body == "" {
			t.Errorf("%s respondeu %d com %d bytes", file, status, len(body))
		}
	}
}

// O roteamento e por hash, entao endereco desconhecido devolve a pagina: quem
// recarrega em /#/cenario/x nao pode cair num 404.
func TestAnyRouteAnswersWithThePage(t *testing.T) {
	address := serve(t)

	status, body := get(t, address, "/cenario/qualquer")
	if status != http.StatusOK || !strings.Contains(body, "<title>braunrate</title>") {
		t.Fatalf("rota desconhecida respondeu %d: %s", status, body)
	}
}

var externalReference = regexp.MustCompile(`(?i)(src|href)\s*=\s*"https?:`)

// Mesma regra do relatorio: a interface abre em rede fechada, entao nada e
// buscado de fora enquanto a pagina carrega.
func TestThePageFetchesNothingFromTheNetwork(t *testing.T) {
	address := serve(t)

	for _, file := range []string{"/", "/style.css", "/app.js"} {
		_, body := get(t, address, file)
		if found := externalReference.FindString(body); found != "" {
			t.Errorf("%s busca da rede: %s", file, found)
		}
		for _, forbidden := range []string{"cdn.", "googleapis", "unpkg", "jsdelivr"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s cita %q", file, forbidden)
			}
		}
	}
}

// A interface e um editor do file: se ela montar uma arvore de campos com
// estado proprio, o YAML deixa de ser a verdade.
func TestTheEditorIsATextAreaOverTheFile(t *testing.T) {
	address := serve(t)

	_, script := get(t, address, "/app.js")
	if !strings.Contains(script, "<textarea") {
		t.Error("o editor nao e uma area de texto sobre o arquivo")
	}
	for _, route := range []string{"/scenarios/", "/text", "/runs"} {
		if !strings.Contains(script, route) {
			t.Errorf("a interface nao usa a rota %q", route)
		}
	}
}

// The terminal demonstration explains five concepts at the point where the
// number appears. Whoever arrives through the screen cannot get less.
func TestTheScreenTeachesTheSameFiveIdeasTheTerminalTeaches(t *testing.T) {
	address := serve(t)
	_, script := get(t, address, "/app.js")

	for _, idea := range []string{
		"Rate is the pace the generator fires at",
		"means 5% of the people waited longer than that",
		"are the acceptance criterion",
		"measures the target's cache, not the target",
		"No number of this run counts as an answer",
	} {
		if !strings.Contains(script, idea) {
			t.Errorf("a tela não explica %q", idea)
		}
	}

	_, page := get(t, address, "/")
	if !strings.Contains(page, `id="explanations"`) || !strings.Contains(script, "without-explanations") {
		t.Error("não há como desligar as explicações, e o -quiet do terminal tem")
	}
}

// A folha da interface nao tinha uma regra de foco sequer, num produto cuja tela
// principal e um editor com botoes: quem navega por teclado nao via onde estava.
// O site ao lado ja fazia certo, e agora as duas superficies usam a mesma regra.
func TestKeyboardFocusIsVisible(t *testing.T) {
	address := serve(t)
	status, sheet := get(t, address, "/style.css")
	if status != http.StatusOK {
		t.Fatalf("a folha não veio: status %d", status)
	}
	if !strings.Contains(sheet, ":focus-visible") {
		t.Error("a interface não declara nenhum estilo de foco")
	}
	if !regexp.MustCompile(`:focus-visible\s*\{[^}]*outline:`).MatchString(sheet) {
		t.Error("o foco não desenha contorno: sem ele o estado é invisível")
	}
}

// Cancelar era "use Ctrl+C no terminal que subiu a interface", que derruba o
// servidor e as outras execucoes junto.
func TestTheRunViewOffersCancelling(t *testing.T) {
	address := serve(t)
	status, script := get(t, address, "/app.js")
	if status != http.StatusOK {
		t.Fatalf("o script não veio: status %d", status)
	}
	if !strings.Contains(script, "method: 'DELETE'") {
		t.Error("a interface não chama o cancelamento")
	}
	if strings.Contains(script, "use Ctrl+C in the terminal") {
		t.Error("a interface ainda manda derrubar o servidor para cancelar")
	}
}
