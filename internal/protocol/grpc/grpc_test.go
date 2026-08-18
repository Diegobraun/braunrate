package grpc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
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

// A local server with the health service and reflection turned on exercises the
// whole path: dial, resolve the method by reflection, build the request from
// JSON, invoke, and read the reply back as JSON.
func TestGRPCCallsThroughReflection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, health.NewServer())
	reflection.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	config := decode(t, "method: grpc.health.v1.Health/Check\nmessage: '{}'")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: listener.Addr().String(), Config: config},
	)
	if response.Class != protocol.Success {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
	if !strings.Contains(string(response.Body), "SERVING") {
		t.Fatalf("reply = %q", response.Body)
	}
}

func TestGRPCReportsAnUnreachableTarget(t *testing.T) {
	config := decode(t, "method: order.OrderService/Lookup\ntimeout: 300ms")
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: "127.0.0.1:1", Config: config},
	)
	if response.Class == protocol.Success {
		t.Fatal("a call to a dead port should not succeed")
	}
}

// descriptorSetFile writes the file descriptor and its imports as a
// FileDescriptorSet, the same shape `protoc --descriptor_set_out --include_imports`
// produces, so the descriptor-set path can be exercised without protoc.
func descriptorSetFile(t *testing.T, fd protoreflect.FileDescriptor) string {
	t.Helper()
	set := &descriptorpb.FileDescriptorSet{}
	seen := map[string]bool{}
	var add func(file protoreflect.FileDescriptor)
	add = func(file protoreflect.FileDescriptor) {
		if seen[file.Path()] {
			return
		}
		seen[file.Path()] = true
		imports := file.Imports()
		for i := 0; i < imports.Len(); i++ {
			add(imports.Get(i).FileDescriptor)
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(file))
	}
	add(fd)
	raw, err := proto.Marshal(set)
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	path := filepath.Join(t.TempDir(), "health.protoset")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write descriptor set: %v", err)
	}
	return path
}

// A server WITHOUT reflection, resolved from a descriptor set on disk, proves the
// fallback for targets that do not expose reflection.
func TestGRPCCallsThroughDescriptorSet(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, health.NewServer())
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	set := descriptorSetFile(t, healthpb.File_grpc_health_v1_health_proto)
	config := decode(t, "method: grpc.health.v1.Health/Check\nmessage: '{}'\ndescriptorSet: "+set)
	response := New(protocol.DefaultOptions()).Execute(
		context.Background(),
		protocol.Request{URLBase: listener.Addr().String(), Config: config},
	)
	if response.Class != protocol.Success {
		t.Fatalf("class = %s, detail = %q", response.Class, response.Detail)
	}
	if !strings.Contains(string(response.Body), "SERVING") {
		t.Fatalf("reply = %q", response.Body)
	}
}
