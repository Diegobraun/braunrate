package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, source string) *Config {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	config, err := New(protocol.DefaultOptions()).Decode(document.Content[0])
	if err != nil {
		t.Fatalf("decode %q: %v", source, err)
	}
	return config.(*Config)
}

func TestSSEIsRegistered(t *testing.T) {
	if _, ok := protocol.Lookup("sse"); !ok {
		t.Fatal("sse protocol is not registered")
	}
}

func TestSSEDecodesShorthandAndMap(t *testing.T) {
	if got := decode(t, `/events`).Path; got != "/events" {
		t.Fatalf("shorthand path = %q", got)
	}
	config := decode(t, "path: /feed\nmaxMessages: 5")
	if config.Path != "/feed" || config.MaxMessages != 5 {
		t.Fatalf("unexpected config: %+v", config)
	}
}

// A server that emits several events and closes proves the drain: the step
// counts each event and stops at maxMessages.
func TestSSEDrainsEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		for i := 0; i < 5; i++ {
			_, _ = fmt.Fprintf(writer, "data: event-%d\n\n", i)
			flusher.Flush()
		}
	}))
	defer server.Close()

	config := decode(t, "maxMessages: 3\ntimeout: 2s")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: server.URL, Config: config},
	)
	if response.Class != protocol.Success {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
	if response.Messages != 3 {
		t.Fatalf("messages = %d, want 3", response.Messages)
	}
	if string(response.Body) != "event-0" {
		t.Fatalf("first event = %q", response.Body)
	}
}

// A stream that stays open ends cleanly on the deadline, not as a failure.
func TestSSEEndsOnTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		_, _ = fmt.Fprint(writer, "data: only-one\n\n")
		flusher.Flush()
		<-request.Context().Done() // hold open until the client leaves
	}))
	defer server.Close()

	config := decode(t, "maxMessages: 10\ntimeout: 300ms")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: server.URL, Config: config},
	)
	if response.Class != protocol.Success {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
	if response.Messages != 1 {
		t.Fatalf("messages = %d, want 1", response.Messages)
	}
}

func TestSSEReportsANon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := decode(t, "path: /missing\ntimeout: 2s")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: server.URL, Config: config},
	)
	if response.Class != protocol.ErrStatus || response.Status != http.StatusNotFound {
		t.Fatalf("class = %s, status = %d", response.Class, response.Status)
	}
}
