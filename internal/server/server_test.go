package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/server"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

const version = "teste"

func target(t *testing.T) *testsupport.Server {
	t.Helper()
	fake := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := fake.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = fake.Close() })
	return fake
}

func directoryWith(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("não consegui escrever %s: %v", name, err)
		}
	}
	return directory
}

func scenarioText(address string) string {
	return fmt.Sprintf(`
name: Consulta de pedidos
target: %s

load:
  profiles:
    - steady: { rate: 50/s, duration: 1s }

scenario:
  - http: GET /pedido
    name: consultar pedido
    expect: { status: 200 }
`, address)
}

func serverOn(t *testing.T, directory string, concurrent bool) *httptest.Server {
	t.Helper()
	options := server.DefaultOptions(version)
	options.Directory = directory
	options.Concurrent = concurrent
	httpServer := httptest.NewServer(server.New(options).Handler())
	t.Cleanup(httpServer.Close)
	return httpServer
}

func call(t *testing.T, method, url string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("requisição inválida: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("o servidor não respondeu: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, body
}

func waitForRun(t *testing.T, base, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, body := call(t, http.MethodGet, base+"/runs/"+id)
		var answer map[string]any
		if err := json.Unmarshal(body, &answer); err != nil {
			t.Fatalf("resposta não e JSON: %s", body)
		}
		if status == http.StatusOK && answer["status"] != "running" {
			return answer
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("a execução %s não terminou", id)
	return nil
}

func startRun(t *testing.T, base, name string) string {
	t.Helper()
	status, body := call(t, http.MethodPost, base+"/scenarios/"+name+"/runs")
	if status != http.StatusAccepted {
		t.Fatalf("inicio da execução respondeu %d: %s", status, body)
	}
	var answer struct{ ID string }
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta não e JSON: %s", body)
	}
	return answer.ID
}

// The rule of this phase is that the server adds no logic. A rule like that is
// only worth anything if something fails when it stops being true, so the same
// scenario goes through the CLI path and through the server and the two result
// documents have to say the same thing.
func TestServerAndCLIProduceTheSameDocument(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	// Two attempts, because these are two real runs: a machine that was busy in
	// between invalidates one of them for saturation, and that says nothing
	// about whether the server added logic. Twice in a row is a defect.
	for attempt := 1; attempt <= 2; attempt++ {
		fromCLI, err := runner.Execute(context.Background(), filepath.Join(directory, "cenario.yaml"), runner.DefaultOptions(version))
		if err != nil {
			t.Fatalf("execução pela CLI falhou: %v", err)
		}

		answer := waitForRun(t, base, startRun(t, base, "cenario.yaml"))
		body, err := json.Marshal(answer)
		if err != nil {
			t.Fatalf("não consegui reserializar: %v", err)
		}
		var fromServer metrics.Document
		if err := json.Unmarshal(body, &fromServer); err != nil {
			t.Fatalf("a resposta não e um documento de resultado: %v", err)
		}

		got, want := comparable(fromServer), comparable(fromCLI.Document)
		if got == want {
			return
		}
		if attempt < 2 && (!fromServer.Valid() || !fromCLI.Document.Valid()) {
			t.Logf("uma das execuções saiu inválida numa máquina ocupada; repetindo o par:\n servidor: %s\n cli:      %s", got, want)
			continue
		}
		t.Fatalf("o servidor produziu documento diferente do da CLI:\n servidor: %s\n cli:      %s", got, want)
	}
}

// What is compared is what a person would compare: the shape of the result and
// the verdict. Timestamps, host and duration change between two runs of the
// same scenario, and comparing them would only prove that time passes.
func comparable(document metrics.Document) string {
	return fmt.Sprintf("formato=%s ferramenta=%s passos=%d valido=%t slo_passou=%t avisos=%d classes=%v modelo=%s alvo=%s",
		document.FormatVersion, document.Tool, len(document.Steps), document.Valid(),
		document.SLO.Passed, len(document.Warnings), errorClasses(document), document.Run.Model, document.Run.Target)
}

func errorClasses(document metrics.Document) []string {
	var classes []string
	for _, step := range document.Steps {
		for class := range step.ErrorsByClass {
			classes = append(classes, class)
		}
	}
	return classes
}

func TestBrokenScenarioIsRefusedWithTheSameMessageAndThePosition(t *testing.T) {
	broken := "name: x\ntarget: http://127.0.0.1:8080\nload:\n  profiles:\n    - steady: { rate: 1/s, duration: 1s }\nscenario:\n  - http: GET /${nao_declarada}\n"
	directory := directoryWith(t, map[string]string{"quebrado.yaml": broken})
	base := serverOn(t, directory, false).URL

	_, _, cliError := runner.Load(filepath.Join(directory, "quebrado.yaml"))
	if cliError == nil {
		t.Fatal("a CLI aceitou o cenário quebrado")
	}

	status, body := call(t, http.MethodPost, base+"/scenarios/quebrado.yaml/validate")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("cenário quebrado respondeu %d: %s", status, body)
	}
	var answer struct {
		Valid   bool
		Message string
		Line    int
		Column  int
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta não e JSON: %s", body)
	}
	if answer.Valid {
		t.Fatal("o servidor aprovou o que a CLI recusou")
	}
	if answer.Message != cliError.Error() {
		t.Fatalf("mensagens diferentes:\n servidor: %s\n cli:      %s", answer.Message, cliError.Error())
	}
	if answer.Line == 0 || answer.Column == 0 {
		t.Fatalf("a resposta não carrega linha e coluna: %s", body)
	}
}

func TestValidScenarioAnswersTheSameLinesTheTerminalPrints(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	spec, plan, err := runner.Load(filepath.Join(directory, "cenario.yaml"))
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}

	status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/validate")
	if status != http.StatusOK {
		t.Fatalf("validação respondeu %d: %s", status, body)
	}
	var answer struct {
		Valid bool
		Lines []string
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta não e JSON: %s", body)
	}
	if !answer.Valid || strings.Join(answer.Lines, "\n") != strings.Join(runner.Describe(spec, plan), "\n") {
		t.Fatalf("o servidor descreveu o cenário de outro jeito:\n%v", answer.Lines)
	}
}

// Two runs on the same machine share the CPU that has to dispatch on the
// scheduled instant, so neither measures what it set out to measure.
func TestSecondRunIsRefusedWhileTheFirstIsRunning(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	first := startRun(t, base, "cenario.yaml")

	status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/runs")
	if status != http.StatusConflict {
		t.Fatalf("a segunda execução respondeu %d: %s", status, body)
	}
	if !strings.Contains(string(body), "-concurrent") {
		t.Fatalf("a recusa não diz como aceitar a contaminação: %s", body)
	}

	waitForRun(t, base, first)
	if status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/runs"); status != http.StatusAccepted {
		t.Fatalf("depois de terminar, a execução seguinte respondeu %d: %s", status, body)
	}
}

func TestConcurrentIsOptInAndSaysSoOnStartup(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, true).URL

	startRun(t, base, "cenario.yaml")
	if status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/runs"); status != http.StatusAccepted {
		t.Fatalf("com -concurrent a segunda execução respondeu %d: %s", status, body)
	}

	options := server.DefaultOptions(version)
	options.Concurrent = true
	warning := strings.Join(server.New(options).StartupWarning(), "\n")
	for _, fragment := range []string{"No authentication", "127.0.0.1", "Concurrent runs enabled"} {
		if !strings.Contains(warning, fragment) {
			t.Fatalf("o aviso de partida não diz %q:\n%s", fragment, warning)
		}
	}
}

func TestRunListCarriesVerdictAndExitCode(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	waitForRun(t, base, startRun(t, base, "cenario.yaml"))

	status, body := call(t, http.MethodGet, base+"/runs")
	if status != http.StatusOK {
		t.Fatalf("a lista respondeu %d: %s", status, body)
	}
	var answer struct {
		Runs []struct {
			ID       string
			Status   string
			Verdict  string
			ExitCode int `json:"exit_code"`
		}
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta não e JSON: %s", body)
	}
	if len(answer.Runs) != 1 {
		t.Fatalf("esperava uma execução na lista: %s", body)
	}
	if answer.Runs[0].Verdict == "" || answer.Runs[0].Status != "done" {
		t.Fatalf("a lista não diz o veredito: %s", body)
	}
}

func TestReportAndComparisonComeFromTheSameProjectionsTheCLIUses(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	before := startRun(t, base, "cenario.yaml")
	waitForRun(t, base, before)
	after := startRun(t, base, "cenario.yaml")
	waitForRun(t, base, after)

	status, body := call(t, http.MethodGet, base+"/runs/"+after+"/report")
	if status != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Fatalf("o relatório HTML respondeu %d: %.200s", status, body)
	}

	status, body = call(t, http.MethodGet, base+"/runs/"+before+"/comparison/"+after)
	if status != http.StatusOK {
		t.Fatalf("a comparação respondeu %d: %s", status, body)
	}
	if !strings.Contains(string(body), "passos") && !strings.Contains(string(body), "steps") {
		t.Fatalf("a comparação não trouxe os passos: %.300s", body)
	}

	status, body = call(t, http.MethodGet, base+"/runs/"+before+"/comparison/"+after+"/report")
	if status != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Fatalf("a comparação em HTML respondeu %d: %.200s", status, body)
	}
	if !strings.Contains(string(body), "before and after") {
		t.Fatalf("a página de comparação não se identifica como comparação: %.300s", body)
	}
	for _, forbidden := range []string{"<script", "src=", "@import", "<link"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("a comparação em HTML deixou de ser autocontida: encontrei %q", forbidden)
		}
	}
}

// A stream that only delivers what comes next shows a run with no beginning to
// anyone who connects a second late.
func TestStreamReplaysWhatAlreadyHappenedAndEndsWithTheVerdict(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	id := startRun(t, base, "cenario.yaml")
	waitForRun(t, base, id)

	status, body := call(t, http.MethodGet, base+"/runs/"+id+"/stream")
	if status != http.StatusOK {
		t.Fatalf("o stream respondeu %d: %s", status, body)
	}
	text := string(body)
	if !strings.Contains(text, "running") {
		t.Fatalf("o stream não replicou o inicio:\n%s", text)
	}
	if !strings.Contains(text, "code") {
		t.Fatalf("o stream não termina com o veredito:\n%s", text)
	}
}

func TestScenarioNameIsNeverAPath(t *testing.T) {
	directory := directoryWith(t, map[string]string{"cenario.yaml": "name: x\n"})
	base := serverOn(t, directory, false).URL

	for _, name := range []string{"..%2f..%2fetc%2fpasswd", "%2etecla", "sub%2fcenario.yaml"} {
		status, body := call(t, http.MethodPost, base+"/scenarios/"+name+"/validate")
		if status != http.StatusBadRequest {
			t.Fatalf("o nome %q respondeu %d: %s", name, status, body)
		}
	}
}

func TestUnknownRunSaysWhereTheRunsLive(t *testing.T) {
	base := serverOn(t, directoryWith(t, map[string]string{}), false).URL

	status, body := call(t, http.MethodGet, base+"/runs/r999")
	if status != http.StatusNotFound {
		t.Fatalf("execução inexistente respondeu %d: %s", status, body)
	}
	if !strings.Contains(string(body), "memory") {
		t.Fatalf("a mensagem não diz que as execuções vivem na memória: %s", body)
	}
}

func writableServerOn(t *testing.T, directory string) *httptest.Server {
	t.Helper()
	options := server.DefaultOptions(version)
	options.Directory = directory
	options.Writable = true
	httpServer := httptest.NewServer(server.New(options).Handler())
	t.Cleanup(httpServer.Close)
	return httpServer
}

func send(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("não consegui montar o pedido: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("não consegui falar com o servidor: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("não consegui ler a resposta: %v", err)
	}
	return response.StatusCode, answer
}

// A interface edita o arquivo, entao o que ela grava e o que o terminal le, byte
// a byte — comentario incluido.
func TestTheTextThatComesBackIsTheFileOnDisk(t *testing.T) {
	original := "# um comentario que o autor escreveu\nname: Consulta\ntarget: http://127.0.0.1:8080\n"
	directory := directoryWith(t, map[string]string{"cenario.yaml": original})
	base := writableServerOn(t, directory).URL

	status, body := call(t, http.MethodGet, base+"/scenarios/cenario.yaml/text")
	if status != http.StatusOK || string(body) != original {
		t.Fatalf("a leitura mudou o arquivo (%d):\n%s", status, body)
	}

	edited := original + "# linha nova\n"
	if status, answer := send(t, http.MethodPut, base+"/scenarios/cenario.yaml/text", edited); status != http.StatusOK {
		t.Fatalf("a gravação respondeu %d: %s", status, answer)
	}
	onDisk, err := os.ReadFile(filepath.Join(directory, "cenario.yaml"))
	if err != nil {
		t.Fatalf("não consegui ler o arquivo gravado: %v", err)
	}
	if string(onDisk) != edited {
		t.Fatalf("o arquivo no disco não é o que a interface gravou:\n%s", onDisk)
	}
}

// Editar por fora e legitimo: o arquivo e a verdade, e nao a copia que a tela
// tem na memoria.
func TestAnEditFromOutsideIsWhatTheInterfaceReads(t *testing.T) {
	directory := directoryWith(t, map[string]string{"cenario.yaml": "name: antes\n"})
	base := writableServerOn(t, directory).URL

	if err := os.WriteFile(filepath.Join(directory, "cenario.yaml"), []byte("name: depois\n"), 0o644); err != nil {
		t.Fatalf("não consegui editar por fora: %v", err)
	}
	_, body := call(t, http.MethodGet, base+"/scenarios/cenario.yaml/text")
	if string(body) != "name: depois\n" {
		t.Fatalf("a leitura não trouxe a edição de fora: %s", body)
	}
}

// Sem Writable a porta continua so de leitura: 'serve' nao grava arquivo.
func TestWithoutWritableNothingIsWritten(t *testing.T) {
	directory := directoryWith(t, map[string]string{"cenario.yaml": "name: x\n"})
	base := serverOn(t, directory, false).URL

	status, _ := send(t, http.MethodPut, base+"/scenarios/cenario.yaml/text", "name: outro\n")
	if status == http.StatusOK {
		t.Fatal("a gravação passou num servidor sem Writable")
	}
	onDisk, err := os.ReadFile(filepath.Join(directory, "cenario.yaml"))
	if err != nil || string(onDisk) != "name: x\n" {
		t.Fatalf("o arquivo mudou: %s (%v)", onDisk, err)
	}
}

// O rascunho e conferido pela mesma leitura do terminal, entao o editor nunca
// aprova o que 'braunrate validate' reprovaria.
func TestTheDraftIsCheckedByTheSameReadingAsTheTerminal(t *testing.T) {
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText("http://127.0.0.1:8080")})
	base := writableServerOn(t, directory).URL

	status, body := send(t, http.MethodPost, base+"/scenarios/cenario.yaml/validate", "name: sem alvo\ncarg: 1\n")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("o rascunho quebrado passou (%d): %s", status, body)
	}
	if !strings.Contains(string(body), "unknown key") {
		t.Fatalf("a recusa do rascunho não é a do terminal: %s", body)
	}

	status, body = call(t, http.MethodPost, base+"/scenarios/cenario.yaml/validate")
	if status != http.StatusOK {
		t.Fatalf("sem corpo, a validação do arquivo gravado respondeu %d: %s", status, body)
	}
}

// Gravar arquivo que nao e cenario transformaria a porta em escrita de arquivo
// qualquer, que e outra coisa.
func TestOnlyScenarioFilesAreWritten(t *testing.T) {
	base := writableServerOn(t, directoryWith(t, map[string]string{})).URL

	for _, name := range []string{"anotacoes.txt", "..%2fescapou.yaml"} {
		status, body := send(t, http.MethodPut, base+"/scenarios/"+name+"/text", "name: x\n")
		if status != http.StatusBadRequest {
			t.Fatalf("%q respondeu %d: %s", name, status, body)
		}
	}
}

// Apontar a carga para o ambiente errado e o erro mais comum e mais caro de um
// teste de carga. Ate aqui a unica saida era matar o processo, o que derruba o
// servidor e as outras execucoes junto.
func TestARunCanBeCanceledAndNeverComesOutAsFinished(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"longa.yaml": longScenario(fake.Address())})
	httpServer := serverOn(t, directory, false)

	status, body := call(t, http.MethodPost, httpServer.URL+"/scenarios/longa.yaml/runs")
	if status != http.StatusAccepted {
		t.Fatalf("a execução não começou: status %d, %s", status, body)
	}
	var started struct{ ID string }
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("resposta ilegível: %v", err)
	}

	status, body = call(t, http.MethodDelete, httpServer.URL+"/runs/"+started.ID)
	if status != http.StatusAccepted {
		t.Fatalf("o cancelamento foi recusado: status %d, %s", status, body)
	}

	// O cancelamento chega ao agendador, entao a execucao para sozinha; o teste
	// espera o fim dela em vez de supor um instante.
	deadline := time.Now().Add(30 * time.Second)
	var state string
	for time.Now().Before(deadline) {
		_, listed := call(t, http.MethodGet, httpServer.URL+"/runs")
		var answer struct {
			Runs []struct{ ID, Status, Verdict string }
		}
		if err := json.Unmarshal(listed, &answer); err == nil && len(answer.Runs) > 0 {
			state = answer.Runs[0].Status
			if state != "running" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if state != "interrupted" {
		t.Fatalf("a execução cancelada terminou como %q; nunca pode sair como concluída", state)
	}

	// Segundo pedido nao encontra o que cancelar, e diz isso em vez de fingir.
	status, _ = call(t, http.MethodDelete, httpServer.URL+"/runs/"+started.ID)
	if status != http.StatusConflict {
		t.Errorf("cancelar de novo devolveu %d, esperava 409", status)
	}
}

func longScenario(address string) string {
	return fmt.Sprintf(`
name: Execução longa
target: %s

load:
  profiles:
    - steady: { rate: 20/s, duration: 5m }

scenario:
  - http: GET /pedido
    name: consultar pedido
`, address)
}

func callBody(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("requisição inválida: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("o servidor não respondeu: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	got, _ := io.ReadAll(response.Body)
	return response.StatusCode, got
}

const authScenario = `
name: Perfil autenticado
target: %s

load:
  profiles:
    - steady: { rate: 20/s, duration: 1s }

auth:
  type: header
  header: "Authorization: Bearer ${TOKEN}"

scenario:
  - http: GET /pedido
    name: perfil
    expect: { status: 200 }
`

// A tela precisa dar valor ao ${TOKEN} sem escrever o segredo no arquivo. O valor
// fica na memória do servidor, some no reinício, e nunca é devolvido pela leitura.
// Ver ADR 0021.
func TestTheSessionValueLetsARunStartAndIsNeverEchoedBack(t *testing.T) {
	const canary = "s3cr3t-canary-de-sessao"
	fake := target(t)
	directory := directoryWith(t, map[string]string{"perfil.yaml": fmt.Sprintf(authScenario, fake.Address())})
	httpServer := serverOn(t, directory, false)

	needs := func(raw []byte) []string {
		var answer struct{ Needs, Provided []string }
		if err := json.Unmarshal(raw, &answer); err != nil {
			t.Fatalf("resposta não é JSON: %s", raw)
		}
		return answer.Needs
	}
	provided := func(raw []byte) []string {
		var answer struct{ Provided []string }
		if err := json.Unmarshal(raw, &answer); err != nil {
			t.Fatalf("resposta não é JSON: %s", raw)
		}
		return answer.Provided
	}

	// Antes de fornecer: a validação diz que falta TOKEN.
	status, body := callBody(t, http.MethodPost, httpServer.URL+"/scenarios/perfil.yaml/validate", "")
	if status != http.StatusOK {
		t.Fatalf("validação falhou: %d %s", status, body)
	}
	if got := needs(body); len(got) != 1 || got[0] != "TOKEN" {
		t.Fatalf("a validação deveria pedir TOKEN: %v", got)
	}

	// Fornecer o valor de sessão. A resposta nunca traz o segredo.
	status, body = callBody(t, http.MethodPut, httpServer.URL+"/environment", `{"TOKEN":"`+canary+`"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT /environment falhou: %d %s", status, body)
	}
	if strings.Contains(string(body), canary) {
		t.Fatalf("a resposta do PUT devolveu o segredo: %s", body)
	}
	if got := provided(body); len(got) != 1 || got[0] != "TOKEN" {
		t.Errorf("o PUT deveria confirmar TOKEN pelo nome: %v", got)
	}

	// A leitura de volta traz só o nome, nunca o valor.
	status, body = call(t, http.MethodGet, httpServer.URL+"/environment")
	if status != http.StatusOK || strings.Contains(string(body), canary) {
		t.Fatalf("GET /environment vazou o segredo ou falhou: %d %s", status, body)
	}

	// Agora a validação não pede mais TOKEN.
	_, body = callBody(t, http.MethodPost, httpServer.URL+"/scenarios/perfil.yaml/validate", "")
	if got := needs(body); len(got) != 0 {
		t.Errorf("TOKEN foi fornecido, a validação não devia mais pedir: %v", got)
	}

	// E o run começa em vez de ser recusado por variável ausente.
	status, body = call(t, http.MethodPost, httpServer.URL+"/scenarios/perfil.yaml/runs")
	if status != http.StatusAccepted {
		t.Fatalf("o run deveria começar com o TOKEN de sessão: %d %s", status, body)
	}
}

func TestASessionNameThatIsNotAnEnvironmentReferenceIsRefused(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"perfil.yaml": fmt.Sprintf(authScenario, fake.Address())})
	httpServer := serverOn(t, directory, false)

	status, _ := callBody(t, http.MethodPut, httpServer.URL+"/environment", `{"token":"x"}`)
	if status != http.StatusBadRequest {
		t.Errorf("nome minúsculo não é referência de ambiente e devia ser recusado, veio %d", status)
	}
}
