package recorder

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/importer"
)

func entry(method, address, body string, headers map[string]string, status int, contentType, response string) Entry {
	parsed, err := url.Parse(address)
	if err != nil {
		panic(err)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	return Entry{
		Method: method, URL: parsed, Headers: headers, Body: body,
		Status: status, ContentType: contentType, ResponseBody: []byte(response),
	}
}

// Without correlation the recorded scenario replays an identifier that only
// existed in the recorded session, which is the defect of the JMeter recorder.
func TestValueBornInOneResponseBecomesCaptureAndReferenceInTheNext(t *testing.T) {
	script, _ := Build([]Entry{
		entry("POST", "http://api.local/auth/token", `{"usuario":"ana"}`, nil, 200,
			"application/json", `{"access_token":"tok-abc-123"}`),
		entry("GET", "http://api.local/pedidos", "", map[string]string{"Authorization": "Bearer tok-abc-123"}, 200,
			"application/json", `{"itens":[]}`),
	}, "cenario")

	if len(script.Steps) != 2 {
		t.Fatalf("esperava 2 passos, vieram %d", len(script.Steps))
	}
	captures := script.Steps[0].Captures
	if len(captures) != 1 || captures[0].Variable != "access_token" || captures[0].Expression != "$.access_token" {
		t.Fatalf("a captura sugerida nao saiu certa: %+v", captures)
	}
	if !captures[0].Suggested {
		t.Fatal("a captura saiu como fato, nao como sugestao a revisar")
	}
	if got := script.Steps[1].Headers["Authorization"]; got != "Bearer ${access_token}" {
		t.Fatalf("o segundo passo continuou com o token literal: %q", got)
	}
}

// A step per identifier gives one line per request in the report instead of one
// line per operation, and the ids stop being data.
func TestSameRouteWithDifferentIdentifiersBecomesOneStepAndOneDataSource(t *testing.T) {
	script, files := Build([]Entry{
		entry("GET", "http://api.local/pedidos/9912", "", nil, 200, "application/json", `{"id":"9912"}`),
		entry("GET", "http://api.local/pedidos/8123", "", nil, 200, "application/json", `{"id":"8123"}`),
		entry("GET", "http://api.local/pedidos/7001", "", nil, 200, "application/json", `{"id":"7001"}`),
	}, "cenario")

	if len(script.Steps) != 1 {
		t.Fatalf("esperava 1 passo, vieram %d", len(script.Steps))
	}
	if script.Steps[0].Path != "/pedidos/${pedidos_id.valor}" {
		t.Fatalf("o caminho nao virou parametro: %q", script.Steps[0].Path)
	}
	if len(files) != 1 || len(files[0].Values) != 3 {
		t.Fatalf("os valores observados nao viraram fonte de dados: %+v", files)
	}
	if files[0].CSV() != "valor\n9912\n8123\n7001\n" {
		t.Fatalf("o CSV saiu diferente:\n%s", files[0].CSV())
	}
}

// Everything the proxy refuses has to be counted and named: a recorder that
// drops in silence produces a scenario that measures less than the person
// thinks it does.
func TestEveryKindOfNoiseIsDroppedWithAReasonOnScreen(t *testing.T) {
	recorder := New(Options{Hosts: []string{"api.local"}, Ignore: []string{"/health"}})
	cases := []struct {
		method  string
		address string
		reason  string
	}{
		{"GET", "http://cdn.outro.com/app.js", "dominio de fora (cdn.outro.com)"},
		{"GET", "http://api.local/static/app.js", "recurso estatico"},
		{"GET", "http://api.local/favicon.ico", "recurso estatico"},
		{"GET", "http://api.local/collect?v=1", "telemetria"},
		{"OPTIONS", "http://api.local/pedidos", "preflight de CORS"},
		{"GET", "http://api.local/health", "pedido por -ignore (/health)"},
	}

	for _, c := range cases {
		request, err := http.NewRequest(c.method, c.address, nil)
		if err != nil {
			t.Fatalf("requisicao invalida: %v", err)
		}
		reason, noise := recorder.classify(request)
		if !noise {
			t.Fatalf("%s %s foi gravado como se fosse trafego do fluxo", c.method, c.address)
		}
		if reason != c.reason {
			t.Fatalf("%s: esperava o motivo %q, veio %q", c.address, c.reason, reason)
		}
	}

	request, _ := http.NewRequest("GET", "http://api.local/pedidos/1", nil)
	if reason, noise := recorder.classify(request); noise {
		t.Fatalf("a chamada do fluxo foi descartada como %q", reason)
	}
}

func TestProxyRecordsWhatPassedAndAnswersTheClient(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"caminho":%q}`, request.URL.Path)
	}))
	defer origin.Close()

	recorder := New(Options{Address: "127.0.0.1:0"})
	runContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	listening := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- recorder.Serve(runContext, func(address string) { listening <- address }) }()

	address := <-listening
	proxyURL, _ := url.Parse("http://" + address)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}
	response, err := client.Get(origin.URL + "/pedidos/1")
	if err != nil {
		t.Fatalf("o proxy nao entregou a resposta: %v", err)
	}
	_ = response.Body.Close()

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("o proxy terminou com erro: %v", err)
	}

	entries := recorder.Entries()
	if len(entries) != 1 {
		t.Fatalf("esperava 1 requisicao gravada, vieram %d", len(entries))
	}
	if entries[0].URL.Path != "/pedidos/1" || entries[0].Status != 200 {
		t.Fatalf("gravou outra coisa: %+v", entries[0])
	}
	if !strings.Contains(string(entries[0].ResponseBody), "/pedidos/1") {
		t.Fatalf("o corpo da resposta nao foi guardado: %q", entries[0].ResponseBody)
	}
}

func TestRecordedStatusIsTheOneThatCameBack(t *testing.T) {
	script, _ := Build([]Entry{
		entry("POST", "http://api.local/pedidos", `{"item":"x"}`, nil, 201, "application/json", `{"id":"p-1"}`),
	}, "cenario")

	if script.Steps[0].ExpectedStatus != 201 {
		t.Fatalf("o passo saiu esperando %d, e o alvo respondeu 201", script.Steps[0].ExpectedStatus)
	}
}

// A session cookie is born in Set-Cookie and comes back in Cookie, which is the
// most common correlation of a web application. Reading only the body meant
// every journey replayed the one session that was recorded — the identity bug
// of phase 4, arriving through the recorder.
func TestSessionCookieBecomesCaptureAndIsSentBackCorrelated(t *testing.T) {
	login := entry("POST", "http://api.local/entrar", `{"usuario":"ana"}`, nil, 200,
		"application/json", `{"usuario":"ana"}`)
	login.ResponseCookies = []string{"sessao=8f3a1c2b4d; Path=/; HttpOnly"}

	script, _ := Build([]Entry{
		login,
		entry("GET", "http://api.local/perfil", "", map[string]string{"Cookie": "sessao=8f3a1c2b4d"}, 200,
			"application/json", `{"usuario":"ana"}`),
	}, "cenario")

	captures := script.Steps[0].Captures
	if len(captures) != 1 || captures[0].Variable != "sessao" || captures[0].Expression != "cookie:sessao" {
		t.Fatalf("o cookie de sessao nao virou captura: %+v", captures)
	}
	if got := script.Steps[1].Headers["Cookie"]; got != "sessao=${sessao}" {
		t.Fatalf("o segundo passo continuou com a sessao gravada: %q", got)
	}
}

// The pair that no recorded response produced is not a correlation, and leaving
// it literal versions somebody's session.
func TestCookieThatNoResponseProducedIsMaskedPairByPair(t *testing.T) {
	login := entry("POST", "http://api.local/entrar", "", nil, 200, "application/json", `{"ok":true}`)
	login.ResponseCookies = []string{"sessao=8f3a1c2b4d; Path=/"}

	script, _ := Build([]Entry{
		login,
		entry("GET", "http://api.local/perfil", "", map[string]string{"Cookie": "sessao=8f3a1c2b4d; rastreio=abc123"}, 200,
			"application/json", `{}`),
	}, "cenario")
	rendered := importer.RenderYAML(script)

	if !strings.Contains(rendered.YAML, "sessao=${sessao}; rastreio=${cookie_rastreio}") {
		t.Fatalf("o cabecalho Cookie nao saiu mascarado por par:\n%s", rendered.YAML)
	}
	if strings.Contains(rendered.YAML, "abc123") {
		t.Fatal("o cookie que nao foi correlacionado foi versionado com o valor da gravacao")
	}
}

// Grouping the same route into one step is right when the identifier varies. It
// is a loss when the call repeated identically, because then the repetition was
// the operation — and "5 requisicoes viraram 4 passos" does not say which one
// disappeared.
func TestIdenticalRepeatedCallSaysTheRepetitionIsNotInTheScenario(t *testing.T) {
	body := `{"conta":"12345","valor":100.5}`
	headers := map[string]string{"Idempotency-Key": "7f3a-2b1c"}
	script, _ := Build([]Entry{
		entry("POST", "http://api.local/transferencias", body, headers, 201, "application/json", `{"id":"tra-1"}`),
		entry("POST", "http://api.local/transferencias", body, headers, 200, "application/json", `{"id":"tra-1"}`),
	}, "cenario")

	if len(script.Steps) != 1 {
		t.Fatalf("esperava 1 passo, vieram %d", len(script.Steps))
	}
	var said string
	for _, warning := range script.Warnings {
		if strings.Contains(warning, "post transferencias") {
			said = warning
		}
	}
	if said == "" {
		t.Fatalf("a repeticao sumiu sem ser nomeada; os avisos foram %q", script.Warnings)
	}
	if !strings.Contains(said, "idempotencia") {
		t.Fatalf("o aviso nao diz o que se perde: %q", said)
	}
}

// The same route with different identifiers is the case grouping exists for,
// and warning there would teach the reader to skip the block that carries the
// warnings that matter.
func TestRouteWithVaryingIdentifiersIsNotWarnedAbout(t *testing.T) {
	script, _ := Build([]Entry{
		entry("GET", "http://api.local/pedidos/9912", "", nil, 200, "application/json", `{}`),
		entry("GET", "http://api.local/pedidos/8123", "", nil, 200, "application/json", `{}`),
	}, "cenario")

	for _, warning := range script.Warnings {
		if strings.Contains(warning, "virou um passo so") {
			t.Fatalf("avisou sobre o agrupamento que e o certo: %q", warning)
		}
	}
}

// Every operation of a GraphQL service arrives at the same address. Grouped by
// route, three operations become one step and a mutation disappears from the
// scenario without anything saying so.
func TestEachGraphQLOperationBecomesItsOwnStep(t *testing.T) {
	call := func(query string) Entry {
		return entry("POST", "http://api.local/graphql",
			`{"query":`+quote(query)+`,"variables":{"id":"ped-1"}}`, nil, 200, "application/json", `{"data":{}}`)
	}
	script, _ := Build([]Entry{
		call("query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }"),
		call("mutation PagarFatura($id: ID!) { pagarFatura(id: $id) { status } }"),
	}, "cenario")

	if len(script.Steps) != 2 {
		t.Fatalf("esperava um passo por operacao, vieram %d", len(script.Steps))
	}
	if script.Steps[0].GraphQL == nil || script.Steps[0].GraphQL.Operation != "ConsultarPedido" {
		t.Fatalf("o primeiro passo nao saiu como graphql: %+v", script.Steps[0])
	}
	if script.Steps[1].GraphQL == nil || script.Steps[1].GraphQL.Operation != "PagarFatura" {
		t.Fatalf("a mutation nao virou passo proprio: %+v", script.Steps[1])
	}
	rendered := importer.RenderYAML(script)
	if !strings.Contains(rendered.YAML, "- graphql ConsultarPedido: { p95: < 500ms }") {
		t.Fatalf("o slo nao aponta a chave que o relatorio usa:\n%s", rendered.YAML)
	}
}

func quote(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `\"`) + `"`
}
