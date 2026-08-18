package mqtt

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
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

func TestMQTTIsRegistered(t *testing.T) {
	if _, ok := protocol.Lookup("mqtt"); !ok {
		t.Fatal("mqtt protocol is not registered")
	}
}

func TestMQTTDecodesShorthandAndMap(t *testing.T) {
	if got := decode(t, `sensors/temp`).Topic; got != "sensors/temp" {
		t.Fatalf("shorthand topic = %q", got)
	}
	config := decode(t, "topic: orders\npayload: hi\nqos: 1\nretain: true")
	if config.Topic != "orders" || string(config.Payload) != "hi" || config.QoS != 1 || !config.Retain {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestMQTTRejectsBadQoS(t *testing.T) {
	var document yaml.Node
	_ = yaml.Unmarshal([]byte("topic: t\nqos: 5"), &document)
	if _, err := New(protocol.DefaultOptions()).Decode(document.Content[0]); err == nil {
		t.Fatal("qos 5 should be rejected")
	}
}

// freePort binds :0, reads the address the OS chose, and frees it so the broker
// can listen there. A tiny race, acceptable for a test.
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}

// An in-process broker (no docker) proves the whole publish path: connect,
// publish with QoS 1, wait for the broker's acknowledgement, and — through an
// inline subscription — see the payload land on the topic.
func TestMQTTPublishesToABroker(t *testing.T) {
	address := freePort(t)
	server := mochi.New(&mochi.Options{InlineClient: true})
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if err := server.AddListener(listeners.NewTCP(listeners.Config{ID: "t1", Address: address})); err != nil {
		t.Fatalf("listener: %v", err)
	}
	go func() { _ = server.Serve() }()
	defer func() { _ = server.Close() }()

	delivered := make(chan []byte, 1)
	if err := server.Subscribe("sensors/temperature", 1, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
		delivered <- pk.Payload
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	config := decode(t, "topic: sensors/temperature\npayload: '{\"c\":21}'\nqos: 1\ntimeout: 3s")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: "tcp://" + address, Config: config},
	)
	if response.Class != protocol.Success {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
	select {
	case payload := <-delivered:
		if string(payload) != `{"c":21}` {
			t.Fatalf("delivered = %q", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the broker never delivered the message")
	}
}

func TestMQTTReportsAnUnreachableBroker(t *testing.T) {
	config := decode(t, "topic: t\npayload: x\ntimeout: 300ms")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: "tcp://127.0.0.1:1", Config: config},
	)
	if response.Class == protocol.Success {
		t.Fatal("a publish to a dead port should not succeed")
	}
}
