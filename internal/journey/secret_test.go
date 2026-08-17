package journey

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/importer"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/server"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

// Valor improvavel de aparecer por acaso, para a busca nao dar falso positivo, e
// longo o bastante para que um corte parcial nao o reconstrua.
const canary = "s3cr3t-canary-value-9f2a1c"

// A cada correcao pontual a protecao cobriu a saida que motivou o defeito e
// deixou a vizinha descoberta: cookie no gravador, corpo na mascara de
// cabecalho, lista de variaveis ao lado do cabecalho ja cortado. Este teste
// troca "uma correcao, um teste" por uma lista: saida nova entra aqui e nasce
// coberta, e campo novo onde cabe credencial entra na outra lista.
//
// O segredo entra pelo ambiente porque o literal ja e recusado no parse — e e
// exatamente por entrar legitimamente que ele pode sair sem ninguem notar.
func TestNoOutputEverPrintsASecretInFull(t *testing.T) {
	for _, place := range placements() {
		t.Run(place.what, func(t *testing.T) {
			for name, value := range place.environment {
				t.Setenv(name, value)
			}
			run := runWithSecret(t, place)
			for _, surface := range run.surfaces(t) {
				t.Run(surface.name, func(t *testing.T) {
					if strings.Contains(surface.text, canary) {
						t.Errorf("a saída %q imprimiu o segredo inteiro\n%s",
							surface.name, excerptAround(surface.text, canary))
					}
					// Mostrar "s3cr3t…" ensina que o campo chegou; mostrar o
					// bastante para reconstruir o segredo nao e corte, e vazamento
					// escrito com reticencias.
					if leak := longestRun(surface.text, canary); leak > safePrefix {
						t.Errorf("a saída %q mostrou %d caracteres seguidos do segredo (o limite é %d)\n%s",
							surface.name, leak, safePrefix, excerptAround(surface.text, canary[:leak]))
					}
				})
			}
		})
	}
}

// O corte que a ferramenta ja usava mostra seis caracteres. Passar disso deixa de
// ser "o campo chegou" e vira o segredo em prestacoes.
const safePrefix = 6

// ------------------------------------------------------------------ os lugares

// Onde uma credencial pode aparecer num cenario. Os tipos de auth se excluem, e
// broker nao roda sem broker, entao cada lugar traz o cenario minimo que o
// exercita em vez de um cenario unico tentando conter todos.
type placement struct {
	what        string
	scenario    string
	environment map[string]string
	// Sem broker de verdade no teste, o cenario carrega mas nao executa: ele
	// passa pelas saidas que so precisam do arquivo.
	staticOnly bool
	// O segredo chega pela resposta do alvo em vez de sair do arquivo, que e o
	// caso da credencial capturada.
	fromTheTarget bool
	// O parse recusa este campo com o valor escrito no arquivo.
	refusesLiteral bool
}

func placements() []placement {
	secret := map[string]string{"CANARY": canary}
	return []placement{
		{
			what:           "variables",
			environment:    secret,
			refusesLiteral: true,
			scenario: header + `
variables:
  apiToken: "${CANARY}"
` + steadyLoad + `
scenario:
  - name: look up order
    http:
      method: GET
      path: /orders/1001
      headers: { X-API-Key: "${apiToken}" }
`,
		},
		{
			what:           "auth.obtain body password",
			environment:    secret,
			refusesLiteral: true,
			scenario: header + `
auth:
  type: token
  obtain:
    http:
      method: POST
      path: /auth/token
      body: { user: ana, password: "${CANARY}" }
    capture: { token: $.access_token }
` + steadyLoad + simpleStep,
		},
		{
			what:           "auth basic password",
			environment:    secret,
			refusesLiteral: true,
			scenario:       header + "auth: { type: basic, user: ana, password: \"${CANARY}\" }\n" + steadyLoad + simpleStep,
		},
		{
			what:           "auth header, bearer",
			environment:    secret,
			refusesLiteral: true,
			scenario:       header + "auth: { type: header, header: \"Authorization: Bearer ${CANARY}\" }\n" + steadyLoad + simpleStep,
		},
		{
			what:           "auth header, api key",
			environment:    secret,
			refusesLiteral: true,
			scenario:       header + "auth: { type: header, header: \"X-API-Key: ${CANARY}\" }\n" + steadyLoad + simpleStep,
		},
		{
			what:           "step header Authorization",
			environment:    secret,
			refusesLiteral: true,
			scenario: header + steadyLoad + `
scenario:
  - name: look up order
    http:
      method: GET
      path: /orders/1001
      headers: { Authorization: "Bearer ${CANARY}" }
`,
		},
		{
			what:           "step header X-API-Key",
			environment:    secret,
			refusesLiteral: true,
			scenario: header + steadyLoad + `
scenario:
  - name: look up order
    http:
      method: GET
      path: /orders/1001
      headers: { X-API-Key: "${CANARY}" }
`,
		},
		{
			what:        "token in the query string",
			environment: secret,
			scenario: header + steadyLoad + `
scenario:
  - name: look up order
    http:
      method: GET
      path: /orders/1001?access_token=${CANARY}
`,
		},
		{
			what:           "step body",
			environment:    secret,
			refusesLiteral: true,
			scenario: header + steadyLoad + `
scenario:
  - name: pay invoice
    http:
      method: POST
      path: /invoices/1001/pay
      body: { value: 199.90, password: "${CANARY}" }
`,
		},
		{
			what:        "value that came from a capture",
			environment: secret,
			// O segredo aqui nao esta no arquivo: ele volta na resposta e a captura
			// o guarda. Foi o defeito da lista de variaveis do 'debug' de ontem, e o
			// alvo de teste devolve o caminho chamado, o que permite trazer o
			// canario de volta sem inventar um alvo novo.
			fromTheTarget: true,
			scenario: header + `
auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: ana } }
    capture: { token: $.access_token }
` + steadyLoad + simpleStep,
		},
		{
			what:           "kafka broker password",
			environment:    secret,
			refusesLiteral: true,
			staticOnly:     true,
			scenario: header + `
messaging:
  kafka:
    brokers: [127.0.0.1:9092]
    auth: { type: scramSha512, user: ana, password: "${CANARY}" }
` + steadyLoad + simpleStep,
		},
		{
			what:           "amqp broker password",
			environment:    secret,
			refusesLiteral: true,
			staticOnly:     true,
			scenario: header + `
messaging:
  amqp:
    brokers: ["amqp://127.0.0.1:5672/"]
    auth: { type: saslPlain, user: ana, password: "${CANARY}" }
` + steadyLoad + simpleStep,
		},
	}
}

const (
	header     = "name: Order lookup\ntarget: %s\n"
	steadyLoad = "\nload:\n  profiles:\n    - steady: { rate: 50/s, duration: 1s }\n"
	simpleStep = "\nscenario:\n  - http: GET /orders/1001\n    name: look up order\n"
)

// ------------------------------------------------------------------ a execucao

type secretRun struct {
	place    placement
	spec     scenario.Spec
	path     string
	root     string
	document metrics.Document
	verdict  slo.Verdict
	plan     engine.Plan
	address  string
}

type surface struct {
	name string
	text string
}

func runWithSecret(t *testing.T, place placement) secretRun {
	t.Helper()
	options := testsupport.Options{Latency: time.Millisecond}
	if place.fromTheTarget {
		options.Token = canary
	}
	fake := testsupport.New(options)
	if err := fake.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = fake.Close() })

	root := t.TempDir()
	path := filepath.Join(root, "scenario.yaml")
	content := fmt.Sprintf(place.scenario, fake.Address())
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("não consegui escrever o cenário: %v", err)
	}

	spec, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenário não carregou: %v", err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}

	run := secretRun{place: place, spec: spec, path: path, root: root, address: fake.Address()}
	run.plan = engine.CompilePlan(spec.Load)
	if place.staticOnly {
		return run
	}

	engineOptions := engine.DefaultOptions()
	engineOptions.DataRoot = root
	engineOptions.Version = "teste"
	executor, err := engine.New(spec, engineOptions)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	run.document = executor.Execute(context.Background())
	run.verdict = slo.Evaluate(spec.SLO, run.document, nil)
	return run
}

// A lista de saidas. Acrescentar uma saida ao produto e acrescentar uma linha
// aqui — e e por isso que ela e uma lista, e nao um teste por saida.
func (run secretRun) surfaces(t *testing.T) []surface {
	t.Helper()
	surfaces := []surface{
		{"validate", strings.Join(runner.Describe(run.spec, run.plan), "\n")},
		{"migrate", run.migrated(t)},
		{"import curl", run.imported(t)},
		{"scenario file as serve returns it", run.fileText(t)},
	}
	if run.place.staticOnly {
		return surfaces
	}
	return append(surfaces,
		surface{"execute summary", render(t, func(out io.Writer) error {
			return report.Summary(out, run.document, run.verdict)
		})},
		surface{"report.html", render(t, func(out io.Writer) error {
			return report.HTML(out, run.document)
		})},
		surface{"result.csv", render(t, func(out io.Writer) error {
			return report.CSV(out, run.document)
		})},
		surface{"result.json", run.resultJSON(t)},
		surface{"compare", render(t, func(out io.Writer) error {
			return report.Comparison(out, comparison.Compare(run.document, run.document))
		})},
		surface{"compare.html", render(t, func(out io.Writer) error {
			return report.ComparisonHTML(out, comparison.Compare(run.document, run.document), "teste")
		})},
		surface{"debug", run.debugText(t)},
		surface{"serve payloads", run.servePayloads(t)},
	)
}

func render(t *testing.T, write func(io.Writer) error) string {
	t.Helper()
	var out bytes.Buffer
	if err := write(&out); err != nil {
		t.Fatalf("não consegui gerar a saída: %v", err)
	}
	return out.String()
}

func (run secretRun) resultJSON(t *testing.T) string {
	t.Helper()
	path := filepath.Join(run.root, "result.json")
	if err := runner.WriteJSON(path, run.document); err != nil {
		t.Fatalf("não consegui escrever o json: %v", err)
	}
	return readAll(t, path)
}

func (run secretRun) migrated(t *testing.T) string {
	t.Helper()
	content, _, err := scenario.Migrate([]byte(readAll(t, run.path)))
	if err != nil {
		t.Fatalf("migrate falhou: %v", err)
	}
	return string(content)
}

// O importador nao le o cenario: ele parte de um curl. O curl que interessa aqui
// e o que carrega a credencial na linha de comando.
func (run secretRun) imported(t *testing.T) string {
	t.Helper()
	command := fmt.Sprintf(`curl http://%s/orders/1001 -H "Authorization: Bearer %s" -d '{"password":"%s"}'`,
		run.address, canary, canary)
	generated, err := importer.FromCurl(command)
	if err != nil {
		t.Fatalf("import curl falhou: %v", err)
	}
	return generated.YAML
}

func (run secretRun) fileText(t *testing.T) string {
	t.Helper()
	return readAll(t, run.path)
}

// O 'debug' imprime passo a passo e, no fim, as variaveis da iteracao. As duas
// coisas entram na mesma saida porque e assim que a pessoa as le.
func (run secretRun) debugText(t *testing.T) string {
	t.Helper()
	iteration, err := runner.Debug(context.Background(), run.path, "teste")
	if err != nil {
		t.Fatalf("debug falhou: %v", err)
	}
	var out bytes.Buffer
	for index, observation := range iteration.Observations {
		if err := report.Debug(&out, index+1, observation, true); err != nil {
			t.Fatalf("não consegui escrever o debug: %v", err)
		}
	}
	if err := report.IterationVars(&out, iteration.Vars); err != nil {
		t.Fatalf("não consegui escrever as variáveis: %v", err)
	}
	return out.String()
}

// O modo servidor devolve o mesmo material por HTTP, e por caminhos proprios: o
// 'debug' de la monta a resposta em vez de reusar o texto do terminal.
func (run secretRun) servePayloads(t *testing.T) string {
	t.Helper()
	options := server.DefaultOptions("teste")
	options.Directory = run.root
	httpServer := httptest.NewServer(server.New(options).Handler())
	t.Cleanup(httpServer.Close)

	var everything strings.Builder
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/scenarios"},
		{http.MethodGet, "/scenarios/scenario.yaml/text"},
		{http.MethodPost, "/scenarios/scenario.yaml/validate"},
		{http.MethodPost, "/scenarios/scenario.yaml/debug"},
	} {
		request, err := http.NewRequest(call.method, httpServer.URL+call.path, nil)
		if err != nil {
			t.Fatalf("requisição inválida: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("o servidor não respondeu: %v", err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		everything.WriteString("\n--- " + call.path + "\n")
		everything.Write(body)
	}
	return everything.String()
}

// --------------------------------------------------------------------- apoio

func readAll(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("não consegui ler %s: %v", path, err)
	}
	return string(content)
}

// O maior pedaco do segredo que aparece de uma vez. Procurar so o valor inteiro
// deixaria passar um corte generoso demais, que e vazamento em prestacoes.
func longestRun(text, secret string) int {
	for size := len(secret); size > safePrefix; size-- {
		for start := 0; start+size <= len(secret); start++ {
			if strings.Contains(text, secret[start:start+size]) {
				return size
			}
		}
	}
	return 0
}

func excerptAround(text, needle string) string {
	position := strings.Index(text, needle)
	if position < 0 {
		return ""
	}
	start := max(0, position-260)
	end := min(len(text), position+len(needle)+120)
	return "    ..." + strings.ReplaceAll(text[start:end], "\n", "\n    ") + "..."
}

// O outro lado da mesma categoria. Onde o segredo pode aparecer, ou a ferramenta
// recusa o literal no parse — e ensina a forma certa —, ou ele entra pelo
// ambiente e cai na lista de saidas acima. Campo que nao faz nem uma coisa nem
// outra e campo por onde a credencial vai para o repositorio.
func TestEveryPlaceThatTakesACredentialRefusesALiteral(t *testing.T) {
	for _, place := range placements() {
		if !place.refusesLiteral {
			continue
		}
		t.Run(place.what, func(t *testing.T) {
			literal := strings.ReplaceAll(place.scenario, "${CANARY}", canary)
			_, err := scenario.Parse([]byte(fmt.Sprintf(literal, "http://127.0.0.1:8080")))
			if err == nil {
				t.Fatalf("credencial literal em %s foi aceita e vai para o repositório", place.what)
			}
			if !strings.Contains(err.Error(), "${") {
				t.Errorf("a recusa de %s não ensina a forma certa: %v", place.what, err)
			}
		})
	}
}
