package wait_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/wait"
)

func configuracaoHTTP(t *testing.T, body string) protocol.Config {
	t.Helper()
	config, err := decode(t, body)
	if err != nil {
		t.Fatalf("cenário não decodificou: %v", err)
	}
	return config
}

// Many async systems only expose the effect over an API: without polling the
// end-to-end chain cannot be measured on them.
func TestHTTPWaitPollsUntilEffectAppears(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) < 3 {
			_, _ = fmt.Fprint(w, `{"status":"PENDENTE"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"PROCESSADO"}`)
	}))
	t.Cleanup(server.Close)

	config := configuracaoHTTP(t, `
http: { path: /pedidos/1 }
until: { $.status: PROCESSADO }
interval: 20ms
timeout: 2s
`)

	start := time.Now()
	response := wait.New(protocol.DefaultOptions()).Execute(context.Background(), protocol.Request{
		URLBase: server.URL, Config: config,
	})
	elapsed := time.Since(start)

	if response.Class != protocol.Success {
		t.Fatalf("classe = %q, detalhe = %q", response.Class, response.Detail)
	}
	if calls.Load() < 3 {
		t.Errorf("sondou %d vezes; a espera terminou antes do efeito", calls.Load())
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("a espera levou %s: o tempo até o efeito precisa entrar na medição", elapsed)
	}
}

func TestHTTPWaitTimeoutSaysWhatItSawAndHowManyPolls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"status":"PENDENTE"}`)
	}))
	t.Cleanup(server.Close)

	config := configuracaoHTTP(t, `
http: { path: /pedidos/1 }
until: { $.status: PROCESSADO }
interval: 20ms
timeout: 120ms
`)

	response := wait.New(protocol.DefaultOptions()).Execute(context.Background(), protocol.Request{
		URLBase: server.URL, Config: config,
	})

	if response.Class != protocol.ErrTimeout {
		t.Fatalf("classe = %q", response.Class)
	}
	for _, expected := range []string{"PENDENTE", "sondagens", "$.status"} {
		if !strings.Contains(response.Detail, expected) {
			t.Errorf("o detalhe precisa conter %q para a pessoa saber o que aconteceu: %q", expected, response.Detail)
		}
	}
}

// Polling without a condition would measure response time, not time to effect.
func TestHTTPWaitWithoutConditionIsRefusedWithExplanation(t *testing.T) {
	_, err := decode(t, "http: { path: /pedidos/1 }\n")
	if err == nil {
		t.Fatal("aguardar por http sem 'ate' precisa ser recusado")
	}
	if !strings.Contains(err.Error(), "ate") || !strings.Contains(err.Error(), "efeito") {
		t.Errorf("a mensagem precisa ensinar a forma certa: %v", err)
	}
}
