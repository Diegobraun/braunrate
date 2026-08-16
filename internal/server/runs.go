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
type run struct {
	ID       string
	Scenario string
	Name     string
	Status   string
	Exit     int
	Message  string
	Started  time.Time
	Finished time.Time
	Document metrics.Document

	closed   bool
	lines    []string
	watchers []chan string
	mutex    sync.Mutex
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
		ID: id, Scenario: scenarioName, Name: spec.Name,
		Status: statusRunning, Started: time.Now(),
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

	found.Finished = time.Now()
	if err != nil {
		found.Status = statusFailed
		found.Message = err.Error()
		found.Exit = runner.ExitBadFile
		if fault, is := err.(runner.Fault); is {
			found.Exit = fault.Exit
		}
		found.append(found.Message)
		found.close()
		return
	}
	found.Status = statusDone
	found.Document = result.Document
	found.Exit = result.Exit
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
		line := runLine{
			ID: found.ID, Scenario: found.Scenario, Name: found.Name,
			Status: found.Status, Exit: found.Exit, Started: found.Started,
			Verdict: verdictOf(found),
		}
		if found.Status == statusDone {
			line.Summary = summaryOf(found.Document)
		}
		lines = append(lines, line)
	}
	sort.SliceStable(lines, func(first, second int) bool {
		return lines[first].Started.After(lines[second].Started)
	})
	return lines
}

func verdictOf(found *run) string {
	switch found.Status {
	case statusRunning:
		return "em andamento"
	case statusFailed:
		return "did not run"
	case statusDone:
		if !found.Document.Valid() {
			return "invalid result"
		}
		if !found.Document.SLO.Passed {
			return "falhou o SLO"
		}
		return "passou"
	}
	return found.Status
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
