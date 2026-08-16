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
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = fake.Close() })
	return fake
}

func directoryWith(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("nao consegui escrever %s: %v", name, err)
		}
	}
	return directory
}

func scenarioText(address string) string {
	return fmt.Sprintf(`
nome: Consulta de pedidos
alvo: %s

carga:
  perfis:
    - patamar: { taxa: 50/s, durante: 1s }

cenario:
  - http: GET /pedido
    nome: consultar pedido
    verificar: { status: 200 }
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
		t.Fatalf("requisicao invalida: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("o servidor nao respondeu: %v", err)
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
			t.Fatalf("resposta nao e JSON: %s", body)
		}
		if status == http.StatusOK && answer["status"] != "running" {
			return answer
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("a execucao %s nao terminou", id)
	return nil
}

func startRun(t *testing.T, base, name string) string {
	t.Helper()
	status, body := call(t, http.MethodPost, base+"/scenarios/"+name+"/runs")
	if status != http.StatusAccepted {
		t.Fatalf("inicio da execucao respondeu %d: %s", status, body)
	}
	var answer struct{ ID string }
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta nao e JSON: %s", body)
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
			t.Fatalf("execucao pela CLI falhou: %v", err)
		}

		answer := waitForRun(t, base, startRun(t, base, "cenario.yaml"))
		body, err := json.Marshal(answer)
		if err != nil {
			t.Fatalf("nao consegui reserializar: %v", err)
		}
		var fromServer metrics.Document
		if err := json.Unmarshal(body, &fromServer); err != nil {
			t.Fatalf("a resposta nao e um documento de resultado: %v", err)
		}

		got, want := comparable(fromServer), comparable(fromCLI.Document)
		if got == want {
			return
		}
		if attempt < 2 && (!fromServer.Valid() || !fromCLI.Document.Valid()) {
			t.Logf("uma das execucoes saiu invalida numa maquina ocupada; repetindo o par:\n servidor: %s\n cli:      %s", got, want)
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
	broken := "nome: x\nalvo: http://127.0.0.1:8080\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /${nao_declarada}\n"
	directory := directoryWith(t, map[string]string{"quebrado.yaml": broken})
	base := serverOn(t, directory, false).URL

	_, _, cliError := runner.Load(filepath.Join(directory, "quebrado.yaml"))
	if cliError == nil {
		t.Fatal("a CLI aceitou o cenario quebrado")
	}

	status, body := call(t, http.MethodPost, base+"/scenarios/quebrado.yaml/validate")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("cenario quebrado respondeu %d: %s", status, body)
	}
	var answer struct {
		Valid   bool
		Message string
		Line    int
		Column  int
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta nao e JSON: %s", body)
	}
	if answer.Valid {
		t.Fatal("o servidor aprovou o que a CLI recusou")
	}
	if answer.Message != cliError.Error() {
		t.Fatalf("mensagens diferentes:\n servidor: %s\n cli:      %s", answer.Message, cliError.Error())
	}
	if answer.Line == 0 || answer.Column == 0 {
		t.Fatalf("a resposta nao carrega linha e coluna: %s", body)
	}
}

func TestValidScenarioAnswersTheSameLinesTheTerminalPrints(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, false).URL

	spec, plan, err := runner.Load(filepath.Join(directory, "cenario.yaml"))
	if err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}

	status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/validate")
	if status != http.StatusOK {
		t.Fatalf("validacao respondeu %d: %s", status, body)
	}
	var answer struct {
		Valid bool
		Lines []string
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("resposta nao e JSON: %s", body)
	}
	if !answer.Valid || strings.Join(answer.Lines, "\n") != strings.Join(runner.Describe(spec, plan), "\n") {
		t.Fatalf("o servidor descreveu o cenario de outro jeito:\n%v", answer.Lines)
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
		t.Fatalf("a segunda execucao respondeu %d: %s", status, body)
	}
	if !strings.Contains(string(body), "-concurrent") {
		t.Fatalf("a recusa nao diz como aceitar a contaminacao: %s", body)
	}

	waitForRun(t, base, first)
	if status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/runs"); status != http.StatusAccepted {
		t.Fatalf("depois de terminar, a execucao seguinte respondeu %d: %s", status, body)
	}
}

func TestConcurrentIsOptInAndSaysSoOnStartup(t *testing.T) {
	fake := target(t)
	directory := directoryWith(t, map[string]string{"cenario.yaml": scenarioText(fake.Address())})
	base := serverOn(t, directory, true).URL

	startRun(t, base, "cenario.yaml")
	if status, body := call(t, http.MethodPost, base+"/scenarios/cenario.yaml/runs"); status != http.StatusAccepted {
		t.Fatalf("com -concurrent a segunda execucao respondeu %d: %s", status, body)
	}

	options := server.DefaultOptions(version)
	options.Concurrent = true
	warning := strings.Join(server.New(options).StartupWarning(), "\n")
	for _, fragment := range []string{"Sem autenticacao", "127.0.0.1", "Execucao concorrente ligada"} {
		if !strings.Contains(warning, fragment) {
			t.Fatalf("o aviso de partida nao diz %q:\n%s", fragment, warning)
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
		t.Fatalf("resposta nao e JSON: %s", body)
	}
	if len(answer.Runs) != 1 {
		t.Fatalf("esperava uma execucao na lista: %s", body)
	}
	if answer.Runs[0].Verdict == "" || answer.Runs[0].Status != "done" {
		t.Fatalf("a lista nao diz o veredito: %s", body)
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
		t.Fatalf("o relatorio HTML respondeu %d: %.200s", status, body)
	}

	status, body = call(t, http.MethodGet, base+"/runs/"+before+"/comparison/"+after)
	if status != http.StatusOK {
		t.Fatalf("a comparacao respondeu %d: %s", status, body)
	}
	if !strings.Contains(string(body), "passos") && !strings.Contains(string(body), "steps") {
		t.Fatalf("a comparacao nao trouxe os passos: %.300s", body)
	}

	status, body = call(t, http.MethodGet, base+"/runs/"+before+"/comparison/"+after+"/report")
	if status != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Fatalf("a comparacao em HTML respondeu %d: %.200s", status, body)
	}
	if !strings.Contains(string(body), "antes e depois") {
		t.Fatalf("a pagina de comparacao nao se identifica como comparacao: %.300s", body)
	}
	for _, forbidden := range []string{"<script", "src=", "@import", "<link"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("a comparacao em HTML deixou de ser autocontida: encontrei %q", forbidden)
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
	if !strings.Contains(text, "executando") {
		t.Fatalf("o stream nao replicou o inicio:\n%s", text)
	}
	if !strings.Contains(text, "codigo") {
		t.Fatalf("o stream nao termina com o veredito:\n%s", text)
	}
}

func TestScenarioNameIsNeverAPath(t *testing.T) {
	directory := directoryWith(t, map[string]string{"cenario.yaml": "nome: x\n"})
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
		t.Fatalf("execucao inexistente respondeu %d: %s", status, body)
	}
	if !strings.Contains(string(body), "memoria") {
		t.Fatalf("a mensagem nao diz que as execucoes vivem na memoria: %s", body)
	}
}
