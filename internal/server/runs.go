package server

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

const (
	statusRunning = "running"
	statusDone    = "done"
	statusFailed  = "failed"
)

// Runs live in memory and die with the process. A database would be a second
// source of truth next to the YAML files, and the YAML files are the truth.
// A execucao roda em uma goroutine e os manipuladores HTTP leem o resultado
// dela enquanto ela acontece. O mutex que ja guardava as linhas guarda tambem o
// desfecho: dois mutexes e como metade do estado acaba sem nenhum.
//
// Identidade e inicio sao escritos antes de a execucao ser publicada na loja, e
// nunca mudam depois — esses sao lidos sem trava.
type run struct {
	ID       string
	Scenario string
	Name     string
	Started  time.Time

	mutex    sync.Mutex
	outcome  outcome
	closed   bool
	lines    []string
	watchers []chan string
}

type outcome struct {
	Status   string
	Exit     int
	Message  string
	Finished time.Time
	Document metrics.Document
}

func (created *run) state() outcome {
	created.mutex.Lock()
	defer created.mutex.Unlock()
	return created.outcome
}

func (created *run) settle(settled outcome) {
	created.mutex.Lock()
	created.outcome = settled
	created.mutex.Unlock()
}

type runStore struct {
	mutex sync.RWMutex
	byID  map[string]*run
	order []string
	next  int
}

func newRunStore() *runStore {
	return &runStore{byID: map[string]*run{}}
}

func (store *runStore) start(scenarioName string, spec scenario.Spec, plan engine.Plan) *run {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.next++
	id := fmt.Sprintf("r%03d", store.next)
	created := &run{
		ID: id, Scenario: scenarioName, Name: spec.Name, Started: time.Now(),
		outcome: outcome{Status: statusRunning},
	}
	created.append(headline(spec, plan))
	store.byID[id] = created
	store.order = append(store.order, id)
	return created
}

func headline(spec scenario.Spec, plan engine.Plan) string {
	if spec.Load.Closed() {
		return fmt.Sprintf("running %q against %s: %d users in a closed loop for %s",
			spec.Name, spec.Target, spec.Load.Users, plan.Duration())
	}
	return fmt.Sprintf("running %q against %s: %d iterations in %s",
		spec.Name, spec.Target, plan.TotalRequests(), plan.Duration())
}

func (created *run) progress(snapshot metrics.Snapshot, targetRate float64, remaining time.Duration) {
	created.append(report.ProgressLine(snapshot, targetRate, remaining))
}

func (created *run) append(line string) {
	created.mutex.Lock()
	created.lines = append(created.lines, line)
	for _, watcher := range created.watchers {
		select {
		case watcher <- line:
		default:
		}
	}
	created.mutex.Unlock()
}

func (created *run) close() {
	created.mutex.Lock()
	created.closed = true
	for _, watcher := range created.watchers {
		close(watcher)
	}
	created.watchers = nil
	created.mutex.Unlock()
}

func (store *runStore) finish(id string, result runner.Result, err error) {
	store.mutex.RLock()
	found := store.byID[id]
	store.mutex.RUnlock()
	if found == nil {
		return
	}

	if err != nil {
		failure := outcome{Status: statusFailed, Message: err.Error(),
			Exit: runner.ExitBadFile, Finished: time.Now()}
		if fault, is := err.(runner.Fault); is {
			failure.Exit = fault.Exit
		}
		found.settle(failure)
		found.append(failure.Message)
		found.close()
		return
	}
	found.settle(outcome{Status: statusDone, Exit: result.Exit,
		Document: result.Document, Finished: time.Now()})
	found.append(verdictLine(result))
	found.close()
}

// The stream ends with the same sentence the terminal ends with, because the
// verdict is the part someone following a run is waiting for.
func verdictLine(result runner.Result) string {
	if !result.Document.Valid() {
		return fmt.Sprintf("invalid result (code %d): the run did not measure what it set out to measure", result.Exit)
	}
	if !result.Document.SLO.Passed {
		return fmt.Sprintf("failed the SLO (code %d)", result.Exit)
	}
	return fmt.Sprintf("passed (code %d)", result.Exit)
}

func (store *runStore) get(id string) (*run, bool) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	found, exists := store.byID[id]
	return found, exists
}

type runLine struct {
	ID       string         `json:"id"`
	Scenario string         `json:"scenario"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Exit     int            `json:"exitCode"`
	Verdict  string         `json:"verdict"`
	Started  time.Time      `json:"startedAt"`
	Summary  map[string]any `json:"summary,omitempty"`
}

func (store *runStore) list() []runLine {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	lines := make([]runLine, 0, len(store.order))
	for _, id := range store.order {
		found := store.byID[id]
		state := found.state()
		line := runLine{
			ID: found.ID, Scenario: found.Scenario, Name: found.Name,
			Status: state.Status, Exit: state.Exit, Started: found.Started,
			Verdict: verdictOf(state),
		}
		if state.Status == statusDone {
			line.Summary = summaryOf(state.Document)
		}
		lines = append(lines, line)
	}
	sort.SliceStable(lines, func(first, second int) bool {
		return lines[first].Started.After(lines[second].Started)
	})
	return lines
}

func verdictOf(state outcome) string {
	switch state.Status {
	case statusRunning:
		return "in progress"
	case statusFailed:
		return "did not run"
	case statusDone:
		if !state.Document.Valid() {
			return "invalid result"
		}
		if !state.Document.SLO.Passed {
			return "failed the SLO"
		}
		return "passed"
	}
	return state.Status
}

// follow replays what already happened before delivering what comes next: a
// client that connects one second late would otherwise see a run with no
// beginning.
func (store *runStore) follow(streamContext context.Context, id string) <-chan string {
	out := make(chan string, 64)
	found, exists := store.get(id)
	if !exists {
		close(out)
		return out
	}

	found.mutex.Lock()
	replay := append([]string(nil), found.lines...)
	if found.closed {
		found.mutex.Unlock()
		go func() {
			defer close(out)
			for _, line := range replay {
				out <- line
			}
		}()
		return out
	}
	live := make(chan string, 64)
	found.watchers = append(found.watchers, live)
	found.mutex.Unlock()

	go func() {
		defer close(out)
		for _, line := range replay {
			select {
			case out <- line:
			case <-streamContext.Done():
				return
			}
		}
		for {
			select {
			case line, open := <-live:
				if !open {
					return
				}
				select {
				case out <- line:
				case <-streamContext.Done():
					return
				}
			case <-streamContext.Done():
				return
			}
		}
	}()
	return out
}
