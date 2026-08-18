package websocket

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	xws "golang.org/x/net/websocket"
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

func TestWebSocketIsRegistered(t *testing.T) {
	if _, ok := protocol.Lookup("websocket"); !ok {
		t.Fatal("websocket protocol is not registered")
	}
}

func TestWebSocketDecodesShorthandAndMap(t *testing.T) {
	if got := decode(t, `/stream`).Path; got != "/stream" {
		t.Fatalf("shorthand path = %q", got)
	}
	config := decode(t, "path: /orders\nsend: ping\nawaitReply: true")
	if config.Path != "/orders" || config.Send != "ping" || !config.AwaitReply {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestWebSocketSendsAndReceives(t *testing.T) {
	server := httptest.NewServer(xws.Handler(func(conn *xws.Conn) {
		var message string
		for {
			if err := xws.Message.Receive(conn, &message); err != nil {
				return
			}
			if err := xws.Message.Send(conn, "echo:"+message); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	config := decode(t, "send: ping\nawaitReply: true")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: server.URL, Config: config},
	)
	if response.Class != protocol.Success {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
	if string(response.Body) != "echo:ping" {
		t.Fatalf("reply = %q", response.Body)
	}
}

// A server that pushes several frames and then closes proves the drain: the
// step counts every message and stops at maxMessages.
func TestWebSocketDrainsAStream(t *testing.T) {
	server := httptest.NewServer(xws.Handler(func(conn *xws.Conn) {
		for i := 0; i < 5; i++ {
			if err := xws.Message.Send(conn, "tick"); err != nil {
				return
			}
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
}

// The deadline is a clean stop for a stream, not a failure: a server that stays
// open but sends nothing more ends the step as a success once the timeout hits.
func TestWebSocketStreamEndsOnTimeout(t *testing.T) {
	server := httptest.NewServer(xws.Handler(func(conn *xws.Conn) {
		_ = xws.Message.Send(conn, "one")
		<-make(chan struct{}) // hold the connection open until the client leaves
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
