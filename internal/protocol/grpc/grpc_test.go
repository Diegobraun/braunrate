package grpc

import (
	"context"
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

func TestGRPCIsRegistered(t *testing.T) {
	if _, ok := protocol.Lookup("grpc"); !ok {
		t.Fatal("grpc protocol is not registered")
	}
}

func TestGRPCSplitsServiceAndMethod(t *testing.T) {
	config := decode(t, `order.OrderService/Lookup`)
	if config.Service != "order.OrderService" || config.Method != "Lookup" {
		t.Fatalf("unexpected split: %+v", config)
	}
	if key := config.AggregationKey(); key != "grpc order.OrderService/Lookup" {
		t.Fatalf("aggregation key = %q", key)
	}
}

func TestGRPCRefusesToRunWithoutTransport(t *testing.T) {
	config := decode(t, "method: order.OrderService/Lookup\nmessage: '{\"id\":\"1\"}'")
	response := New(protocol.DefaultOptions()).Execute(context.Background(), protocol.Request{Config: config})
	if response.Class != protocol.ErrConfig {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
}
