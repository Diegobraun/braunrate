// Package server exposes over HTTP what the CLI already does, and nothing
// else. Every route ends in internal/runner, which is what keeps a scenario
// approved in the terminal approved with the same words over the network.
//
// Route names and JSON fields are in English, like the rest of the code;
// messages are in Portuguese, like the rest of the product (ADR 0010).
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Options struct {
	Address    string
	Directory  string
	Version    string
	Concurrent bool
}

func DefaultOptions(version string) Options {
	return Options{Address: "127.0.0.1:8080", Directory: ".", Version: version}
}

type Server struct {
	options Options
	runs    *runStore
	// A load generator that runs two scenarios at once measures neither: they
	// share the CPU that has to dispatch on the scheduled instant. Refusing is
	// the default and letting it through is a declared choice.
	busy   bool
	busyMu sync.Mutex
}

func New(options Options) *Server {
	return &Server{options: options, runs: newRunStore()}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /scenarios", server.listScenarios)
	mux.HandleFunc("POST /scenarios/{name}/validate", server.validate)
	mux.HandleFunc("POST /scenarios/{name}/debug", server.debug)
	mux.HandleFunc("POST /scenarios/{name}/runs", server.startRun)
	mux.HandleFunc("GET /runs", server.listRuns)
	mux.HandleFunc("GET /runs/{id}", server.getRun)
	mux.HandleFunc("GET /runs/{id}/report", server.getReport)
	mux.HandleFunc("GET /runs/{id}/stream", server.streamRun)
	mux.HandleFunc("GET /runs/{before}/comparison/{after}", server.compare)
	mux.HandleFunc("GET /runs/{before}/comparison/{after}/report", server.compareReport)
	return mux
}

// StartupWarning is printed, not logged: whoever started the server has to see
// that the port has no authentication before pointing anyone at it.
func (server *Server) StartupWarning() []string {
	lines := []string{
		fmt.Sprintf("braunrate serve em http://%s, servindo cenarios de %s", server.options.Address, server.options.Directory),
		"Sem autenticacao e sem TLS: qualquer um que alcance esta porta pode disparar carga contra os alvos dos cenarios.",
		"Foi feito para rodar em 127.0.0.1. Expor em outra interface e outra decisao, e ela ainda nao foi tomada.",
	}
	if server.options.Concurrent {
		lines = append(lines, "Execucao concorrente ligada: duas execucoes ao mesmo tempo disputam a CPU que precisa despachar no instante agendado, e nenhuma das duas mede o que se propos a medir.")
	}
	return lines
}

func (server *Server) Listen() error {
	httpServer := &http.Server{
		Addr:              server.options.Address,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return httpServer.ListenAndServe()
}

func (server *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"tool": "braunrate", "version": server.options.Version})
}

type scenarioLine struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (server *Server) listScenarios(writer http.ResponseWriter, _ *http.Request) {
	entries, err := os.ReadDir(server.options.Directory)
	if err != nil {
		writeProblem(writer, http.StatusInternalServerError, fmt.Sprintf("nao consegui ler %s: %v", server.options.Directory, err))
		return
	}
	found := []scenarioLine{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		found = append(found, scenarioLine{Name: name, Path: filepath.Join(server.options.Directory, name)})
	}
	writeJSON(writer, http.StatusOK, map[string]any{"scenarios": found})
}

// The scenario name is a file name inside -dir and never a path: without this,
// a request could read any file the process can reach.
func (server *Server) pathOf(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("nome de cenario invalido: %q. Use o nome do arquivo dentro do diretorio servido, sem caminho", name)
	}
	return filepath.Join(server.options.Directory, name), nil
}

type validationAnswer struct {
	Valid   bool     `json:"valid"`
	Lines   []string `json:"lines,omitempty"`
	Message string   `json:"message,omitempty"`
	File    string   `json:"file,omitempty"`
	Line    int      `json:"line,omitempty"`
	Column  int      `json:"column,omitempty"`
}

func (server *Server) validate(writer http.ResponseWriter, request *http.Request) {
	path, err := server.pathOf(request.PathValue("name"))
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, err.Error())
		return
	}
	spec, plan, err := runner.Load(path)
	if err != nil {
		writeJSON(writer, http.StatusUnprocessableEntity, refusal(err))
		return
	}
	writeJSON(writer, http.StatusOK, validationAnswer{Valid: true, Lines: runner.Describe(spec, plan)})
}

// The position of the error is what the editor needs, so it travels as fields
// and not only inside the sentence.
func refusal(err error) validationAnswer {
	answer := validationAnswer{Message: err.Error()}
	var positioned scenario.ScenarioError
	if fault, is := err.(runner.Fault); is {
		answer.Message = fault.Message
	}
	if found := findPosition(err, &positioned); found {
		answer.File, answer.Line, answer.Column = positioned.File, positioned.Line, positioned.Column
	}
	return answer
}

func findPosition(err error, into *scenario.ScenarioError) bool {
	for err != nil {
		if positioned, is := err.(scenario.ScenarioError); is {
			*into = positioned
			return true
		}
		unwrapper, is := err.(interface{ Unwrap() error })
		if !is {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

type debugAnswer struct {
	Complete     bool                `json:"complete"`
	Text         string              `json:"text"`
	Vars         map[string]string   `json:"vars"`
	Observations []observationAnswer `json:"observations"`
}

type observationAnswer struct {
	Step     string `json:"step"`
	Class    string `json:"class"`
	Status   int    `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Duration string `json:"duration"`
}

func (server *Server) debug(writer http.ResponseWriter, request *http.Request) {
	path, err := server.pathOf(request.PathValue("name"))
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, err.Error())
		return
	}
	spec, _, err := runner.Load(path)
	if err != nil {
		writeJSON(writer, http.StatusUnprocessableEntity, refusal(err))
		return
	}

	release, taken := server.take()
	if !taken {
		writeProblem(writer, http.StatusConflict, busyMessage)
		return
	}
	defer release()

	iteration, err := runner.Debug(request.Context(), path, server.options.Version)
	if err != nil {
		writeJSON(writer, http.StatusUnprocessableEntity, refusal(err))
		return
	}

	var text strings.Builder
	for _, line := range scenario.DescribeMessaging(spec.Messaging) {
		fmt.Fprintf(&text, "mensageria: %s\n", line)
	}
	answer := debugAnswer{Complete: iteration.Complete(), Vars: iteration.Vars}
	for index, observation := range iteration.Observations {
		if err := report.Debug(&text, index+1, observation, true); err != nil {
			writeProblem(writer, http.StatusInternalServerError, fmt.Sprintf("nao consegui escrever a depuracao: %v", err))
			return
		}
		answer.Observations = append(answer.Observations, observationAnswer{
			Step:     observation.Step,
			Class:    string(observation.Class),
			Status:   observation.Response.Status,
			Detail:   observation.Response.Detail,
			Duration: observation.Duration.Round(100_000).String(),
		})
	}
	answer.Text = text.String()
	writeJSON(writer, http.StatusOK, answer)
}

const busyMessage = "ja existe uma execucao em andamento. Duas execucoes na mesma maquina disputam a CPU que precisa despachar no instante agendado, e nenhuma das duas mede o que se propos a medir. Espere a atual terminar, ou suba o servidor com -concurrent se a contaminacao for aceitavel neste caso."

func (server *Server) take() (func(), bool) {
	if server.options.Concurrent {
		return func() {}, true
	}
	server.busyMu.Lock()
	defer server.busyMu.Unlock()
	if server.busy {
		return nil, false
	}
	server.busy = true
	return func() {
		server.busyMu.Lock()
		server.busy = false
		server.busyMu.Unlock()
	}, true
}

func (server *Server) startRun(writer http.ResponseWriter, request *http.Request) {
	path, err := server.pathOf(request.PathValue("name"))
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, err.Error())
		return
	}
	spec, plan, err := runner.Load(path)
	if err != nil {
		writeJSON(writer, http.StatusUnprocessableEntity, refusal(err))
		return
	}

	release, taken := server.take()
	if !taken {
		writeProblem(writer, http.StatusConflict, busyMessage)
		return
	}

	run := server.runs.start(request.PathValue("name"), spec, plan)
	options := runner.DefaultOptions(server.options.Version)
	options.OnProgress = run.progress

	// The request that starts a run does not wait for it: a load test lasts
	// minutes, and an HTTP client that gives up would leave the run headless.
	go func() {
		defer release()
		result, err := runner.Execute(context.Background(), path, options)
		server.runs.finish(run.ID, result, err)
	}()

	writer.Header().Set("Location", "/runs/"+run.ID)
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"id":     run.ID,
		"stream": "/runs/" + run.ID + "/stream",
		"status": statusRunning,
	})
}

func (server *Server) listRuns(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"runs": server.runs.list()})
}

func (server *Server) getRun(writer http.ResponseWriter, request *http.Request) {
	run, found := server.runs.get(request.PathValue("id"))
	if !found {
		writeProblem(writer, http.StatusNotFound, unknownRun(request.PathValue("id")))
		return
	}
	if run.Status == statusRunning {
		writeJSON(writer, http.StatusOK, map[string]any{"id": run.ID, "status": run.Status,
			"message": "a execucao ainda esta em andamento; acompanhe em /runs/" + run.ID + "/stream"})
		return
	}
	if run.Status == statusFailed {
		writeProblem(writer, http.StatusUnprocessableEntity, run.Message)
		return
	}
	writeJSON(writer, http.StatusOK, run.Document)
}

func (server *Server) getReport(writer http.ResponseWriter, request *http.Request) {
	run, found := server.runs.get(request.PathValue("id"))
	if !found {
		writeProblem(writer, http.StatusNotFound, unknownRun(request.PathValue("id")))
		return
	}
	if run.Status != statusDone {
		writeProblem(writer, http.StatusConflict, "o relatorio so existe depois que a execucao termina; o estado agora e "+run.Status)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := report.HTML(writer, run.Document); err != nil {
		// The status line is already on the wire, so there is no error to send:
		// what is left is not pretending the report was complete.
		_, _ = fmt.Fprintf(writer, "\n<!-- relatorio interrompido: %v -->\n", err)
	}
}

// The stream is plain text, one line per progress tick, the same line the
// terminal prints. Anything richer would be a second way of saying the same
// thing, and the two would drift.
func (server *Server) streamRun(writer http.ResponseWriter, request *http.Request) {
	run, found := server.runs.get(request.PathValue("id"))
	if !found {
		writeProblem(writer, http.StatusNotFound, unknownRun(request.PathValue("id")))
		return
	}
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, canFlush := writer.(http.Flusher)

	for line := range server.runs.follow(request.Context(), run.ID) {
		// A client that hangs up mid-stream is the normal way of leaving; what
		// would be wrong is holding the run hostage to it.
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return
		}
		if canFlush {
			flusher.Flush()
		}
	}
}

func (server *Server) compare(writer http.ResponseWriter, request *http.Request) {
	before, after, ok := server.pairToCompare(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, comparison.Compare(before.Document, after.Document))
}

func (server *Server) compareReport(writer http.ResponseWriter, request *http.Request) {
	before, after, ok := server.pairToCompare(writer, request)
	if !ok {
		return
	}
	result := comparison.Compare(before.Document, after.Document)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := report.ComparisonHTML(writer, result, after.Document.Version); err != nil {
		// The status line is already on the wire, so there is no error to send:
		// what is left is not pretending the page was complete.
		_, _ = fmt.Fprintf(writer, "\n<!-- comparacao interrompida: %v -->\n", err)
	}
}

func (server *Server) pairToCompare(writer http.ResponseWriter, request *http.Request) (*run, *run, bool) {
	before, foundBefore := server.runs.get(request.PathValue("before"))
	after, foundAfter := server.runs.get(request.PathValue("after"))
	for id, found := range map[string]bool{request.PathValue("before"): foundBefore, request.PathValue("after"): foundAfter} {
		if !found {
			writeProblem(writer, http.StatusNotFound, unknownRun(id))
			return nil, nil, false
		}
	}
	if before.Status != statusDone || after.Status != statusDone {
		writeProblem(writer, http.StatusConflict, "so da para comparar execucao terminada; uma das duas ainda nao terminou")
		return nil, nil, false
	}
	return before, after, true
}

func unknownRun(id string) string {
	return fmt.Sprintf("nao conheco a execucao %q. As execucoes vivem na memoria deste processo e somem quando ele reinicia; /runs lista as que existem agora", id)
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(body)
}

func writeProblem(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]any{"message": message})
}

// Kept out of the store so the store deals only with state.
func summaryOf(document metrics.Document) map[string]any {
	return map[string]any{
		"requests": document.Overall.Count,
		"errors":   document.Overall.Errors,
		"valid":    document.Valid(),
	}
}
