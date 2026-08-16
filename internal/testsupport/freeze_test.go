package testsupport

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func get(t *testing.T, address string) time.Duration {
	t.Helper()
	start := time.Now()
	response, err := http.Get(address + "/pedido")
	if err != nil {
		t.Fatalf("o alvo nao respondeu: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return time.Since(start)
}

// Counted from Start, the freeze sat on the wall clock of the whole test: engine
// setup, data loading and a busy machine shifted the run relative to it, and the
// window could fall outside the measurement. That is what made the coordinated
// omission test flaky. Counted from the first request it lands in the same place
// every time.
func TestFreezeIsCountedFromTheFirstRequestNotFromStart(t *testing.T) {
	server := New(Options{FreezeAfter: 300 * time.Millisecond, FreezeFor: 400 * time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("o alvo nao subiu: %v", err)
	}
	defer func() { _ = server.Close() }()

	// Long enough that a freeze anchored on Start would already be over.
	time.Sleep(800 * time.Millisecond)

	if first := get(t, server.Address()); first > 100*time.Millisecond {
		t.Fatalf("a primeira requisicao levou %s: a pausa ja tinha comecado antes de alguem pedir", first)
	}
	time.Sleep(400 * time.Millisecond)

	if during := get(t, server.Address()); during < 100*time.Millisecond {
		t.Fatalf("a requisicao no meio da pausa levou %s: a pausa nao aconteceu", during)
	}
}
