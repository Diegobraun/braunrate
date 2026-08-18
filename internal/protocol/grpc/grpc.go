package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Service     string
	Method      string
	Message     string
	Metadata    map[string]string
	Target      string
	MaxMessages int64
	Timeout     time.Duration
}

func (config *Config) Protocol() string { return "grpc" }

func (config *Config) AggregationKey() string { return "grpc " + config.fullMethod() }

func (config *Config) fullMethod() string {
	if config.Service == "" {
		return config.Method
	}
	return config.Service + "/" + config.Method
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Message = resolve(config.Message)
	clone.Target = resolve(config.Target)
	clone.Metadata = make(map[string]string, len(config.Metadata))
	for name, value := range config.Metadata {
		clone.Metadata[name] = resolve(value)
	}
	return &clone
}

func (config *Config) WithHeader(name, value string) protocol.Config {
	clone := *config
	clone.Metadata = make(map[string]string, len(config.Metadata)+1)
	for key, content := range config.Metadata {
		clone.Metadata[key] = content
	}
	clone.Metadata[strings.ToLower(name)] = value
	return &clone
}

func (config *Config) Describe() []string {
	lines := []string{"call " + config.fullMethod()}
	if config.Target != "" {
		lines = append(lines, "target: "+config.Target)
	}
	names := make([]string, 0, len(config.Metadata))
	for name := range config.Metadata {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, transport.MaskSecret(name, config.Metadata[name])))
	}
	if config.Message != "" {
		lines = append(lines, "message: "+summarize(config.Message))
	}
	return lines
}

func (config *Config) RequestBody() []byte { return []byte(config.Message) }

type Protocol struct {
	options protocol.Options
	tls     *tls.Config

	mu      sync.Mutex
	conns   map[string]*grpc.ClientConn
	methods map[string]protoreflect.MethodDescriptor
}

func New(options protocol.Options) *Protocol {
	return &Protocol{
		options: options,
		conns:   map[string]*grpc.ClientConn{},
		methods: map[string]protoreflect.MethodDescriptor{},
	}
}

func (implementation *Protocol) Name() string { return "grpc" }

func (implementation *Protocol) UseTLS(settings *tls.Config) { implementation.tls = settings }

func (implementation *Protocol) Close() error {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	for _, conn := range implementation.conns {
		_ = conn.Close()
	}
	implementation.conns = map[string]*grpc.ClientConn{}
	implementation.methods = map[string]protoreflect.MethodDescriptor{}
	return nil
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil {
		return nil, errors.New("grpc step with no configuration")
	}
	config := &Config{Metadata: map[string]string{}}

	if node.Kind == yaml.ScalarNode {
		if err := setMethod(config, node.Value); err != nil {
			return nil, err
		}
		return finish(config)
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New(`a grpc step is the method or a map, like this:
  - grpc:
      method: order.OrderService/Lookup
      message: '{"id":"1"}'`)
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "method", "call":
			if err := setMethod(config, value.Value); err != nil {
				return nil, err
			}
		case "service":
			config.Service = value.Value
		case "message":
			config.Message = value.Value
		case "target":
			config.Target = value.Value
		case "maxMessages", "max_messages":
			count, err := strconv.ParseInt(strings.TrimSpace(value.Value), 10, 64)
			if err != nil || count < 0 {
				return nil, fmt.Errorf("maxMessages has to be a whole number, got %q", value.Value)
			}
			config.MaxMessages = count
		case "metadata", "headers":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("metadata has to be a map")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Metadata[strings.ToLower(value.Content[i].Value)] = value.Content[i+1].Value
			}
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout: %q (use 30s, 2m)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("unknown key in the grpc step: %q (use method, service, message, target, maxMessages, metadata or timeout)", key.Value)
		}
	}
	return finish(config)
}

func setMethod(config *Config, raw string) error {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "/"))
	if raw == "" {
		return errors.New("grpc step with no method")
	}
	if slash := strings.LastIndex(raw, "/"); slash >= 0 {
		config.Service = raw[:slash]
		config.Method = raw[slash+1:]
	} else {
		config.Method = raw
	}
	return nil
}

func finish(config *Config) (protocol.Config, error) {
	if config.Method == "" {
		return nil, errors.New(`a grpc step needs a method, like package.Service/Method:
  - grpc: order.OrderService/Lookup`)
	}
	return config, nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not a grpc one"}
	}
	address := stripScheme(config.Target)
	if address == "" {
		address = stripScheme(request.URLBase)
	}
	if address == "" {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "no target for the grpc call (set the scenario target or the step's target)"}
	}

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, config.Timeout)
		defer cancel()
	} else if implementation.options.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, implementation.options.Timeout)
		defer cancel()
	}

	conn, err := implementation.connect(address)
	if err != nil {
		return protocol.Response{Class: protocol.ErrNetwork, Detail: transport.SummarizeError(err)}
	}
	method, err := implementation.resolve(runContext, address, conn, config)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: transport.SummarizeError(err)}
	}

	message := dynamicpb.NewMessage(method.Input())
	if body := strings.TrimSpace(config.Message); body != "" {
		if err := protojson.Unmarshal([]byte(config.Message), message); err != nil {
			return protocol.Response{Class: protocol.ErrConfig, Detail: "the message is not valid JSON for " + string(method.Input().FullName()) + ": " + err.Error()}
		}
	}

	if len(config.Metadata) > 0 {
		pairs := make([]string, 0, len(config.Metadata)*2)
		for name, value := range config.Metadata {
			pairs = append(pairs, name, value)
		}
		runContext = metadata.AppendToOutgoingContext(runContext, pairs...)
	}

	if method.IsStreamingServer() {
		return implementation.drainServerStream(runContext, conn, config, method, message)
	}

	reply := dynamicpb.NewMessage(method.Output())
	if err := conn.Invoke(runContext, "/"+config.Service+"/"+config.Method, message, reply); err != nil {
		class, detail := classify(err)
		return protocol.Response{Class: class, Detail: detail}
	}

	out := protocol.Response{Class: protocol.Success}
	if content, err := protojson.Marshal(reply); err == nil {
		out.Body = content
		out.Bytes = int64(len(content))
	}
	return out
}

// drainServerStream sends the single request and reads the server's messages
// until it closes, maxMessages is reached, or the context ends. The iteration's
// latency is the whole stream's lifetime; Messages carries the count.
func (implementation *Protocol) drainServerStream(runContext context.Context, conn *grpc.ClientConn, config *Config, method protoreflect.MethodDescriptor, message *dynamicpb.Message) protocol.Response {
	runContext, cancel := context.WithCancel(runContext)
	defer cancel()

	stream, err := conn.NewStream(runContext, &grpc.StreamDesc{ServerStreams: true}, "/"+config.Service+"/"+config.Method)
	if err != nil {
		class, detail := classify(err)
		return protocol.Response{Class: class, Detail: detail}
	}
	if err := stream.SendMsg(message); err != nil {
		class, detail := classify(err)
		return protocol.Response{Class: class, Detail: detail}
	}
	_ = stream.CloseSend()

	var count, bytes int64
	var first []byte
	for {
		reply := dynamicpb.NewMessage(method.Output())
		if err := stream.RecvMsg(reply); err != nil {
			if err == io.EOF {
				break
			}
			class, detail := classify(err)
			return protocol.Response{Class: class, Detail: detail, Messages: count, Bytes: bytes}
		}
		count++
		if content, err := protojson.Marshal(reply); err == nil {
			bytes += int64(len(content))
			if first == nil {
				first = content
			}
		}
		if config.MaxMessages > 0 && count >= config.MaxMessages {
			break
		}
	}
	return protocol.Response{Class: protocol.Success, Messages: count, Bytes: bytes, Body: first}
}

func (implementation *Protocol) connect(address string) (*grpc.ClientConn, error) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if conn, ok := implementation.conns[address]; ok {
		return conn, nil
	}
	creds := insecure.NewCredentials()
	if implementation.tls != nil {
		creds = credentials.NewTLS(implementation.tls)
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	implementation.conns[address] = conn
	return conn, nil
}

func (implementation *Protocol) resolve(runContext context.Context, address string, conn *grpc.ClientConn, config *Config) (protoreflect.MethodDescriptor, error) {
	key := address + "|" + config.fullMethod()
	implementation.mu.Lock()
	if method, ok := implementation.methods[key]; ok {
		implementation.mu.Unlock()
		return method, nil
	}
	implementation.mu.Unlock()

	files, err := descriptorsBySymbol(runContext, conn, config.Service)
	if err != nil {
		return nil, err
	}
	descriptor, err := files.FindDescriptorByName(protoreflect.FullName(config.Service))
	if err != nil {
		return nil, fmt.Errorf("the target's reflection has no service %q: %v", config.Service, err)
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a service", config.Service)
	}
	method := service.Methods().ByName(protoreflect.Name(config.Method))
	if method == nil {
		return nil, fmt.Errorf("the service %q has no method %q", config.Service, config.Method)
	}
	if method.IsStreamingClient() {
		return nil, fmt.Errorf("%s is client-streaming; braunrate sends one request and drains the reply, so only unary and server-streaming methods are supported", config.fullMethod())
	}

	implementation.mu.Lock()
	implementation.methods[key] = method
	implementation.mu.Unlock()
	return method, nil
}

// descriptorsBySymbol asks the target's server reflection for the file holding a
// symbol and every file it imports, then builds a registry from them. The
// dependency closure is fetched by name because reflection returns the file with
// the symbol but not, on its own, the files that one imports.
func descriptorsBySymbol(runContext context.Context, conn *grpc.ClientConn, symbol string) (*protoregistry.Files, error) {
	client := reflectionpb.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(runContext)
	if err != nil {
		return nil, fmt.Errorf("server reflection is not available on the target: %v", err)
	}
	defer func() { _ = stream.CloseSend() }()

	collected := map[string]*descriptorpb.FileDescriptorProto{}
	request := func(message *reflectionpb.ServerReflectionRequest) error {
		if err := stream.Send(message); err != nil {
			return err
		}
		response, err := stream.Recv()
		if err != nil {
			return err
		}
		if failure := response.GetErrorResponse(); failure != nil {
			return fmt.Errorf("%s", failure.GetErrorMessage())
		}
		for _, raw := range response.GetFileDescriptorResponse().GetFileDescriptorProto() {
			file := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(raw, file); err != nil {
				return err
			}
			collected[file.GetName()] = file
		}
		return nil
	}

	if err := request(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: symbol},
	}); err != nil {
		return nil, fmt.Errorf("reflection could not resolve %q (is it enabled on the target?): %v", symbol, err)
	}
	for {
		missing := ""
		for _, file := range collected {
			for _, dependency := range file.GetDependency() {
				if _, have := collected[dependency]; !have {
					missing = dependency
					break
				}
			}
			if missing != "" {
				break
			}
		}
		if missing == "" {
			break
		}
		if err := request(&reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_FileByFilename{FileByFilename: missing},
		}); err != nil {
			return nil, fmt.Errorf("reflection could not fetch the imported file %q: %v", missing, err)
		}
	}

	ordered, err := topological(collected)
	if err != nil {
		return nil, err
	}
	registry, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: ordered})
	if err != nil {
		return nil, fmt.Errorf("the descriptors from reflection did not assemble: %v", err)
	}
	return registry, nil
}

func topological(files map[string]*descriptorpb.FileDescriptorProto) ([]*descriptorpb.FileDescriptorProto, error) {
	var ordered []*descriptorpb.FileDescriptorProto
	placed := map[string]bool{}
	var visit func(name string, path map[string]bool) error
	visit = func(name string, path map[string]bool) error {
		if placed[name] {
			return nil
		}
		file, ok := files[name]
		if !ok {
			return nil
		}
		if path[name] {
			return fmt.Errorf("the proto files import each other in a cycle at %q", name)
		}
		path[name] = true
		for _, dependency := range file.GetDependency() {
			if err := visit(dependency, path); err != nil {
				return err
			}
		}
		delete(path, name)
		placed[name] = true
		ordered = append(ordered, file)
		return nil
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func classify(err error) (protocol.ErrorClass, string) {
	state, ok := status.FromError(err)
	if !ok {
		if err == io.EOF {
			return protocol.ErrNetwork, "the connection closed before a reply"
		}
		return protocol.ErrNetwork, transport.SummarizeError(err)
	}
	detail := state.Code().String()
	if state.Message() != "" {
		detail += ": " + state.Message()
	}
	switch state.Code() {
	case codes.Unauthenticated:
		return protocol.ErrAuth, detail
	case codes.PermissionDenied:
		return protocol.ErrAuthorization, detail
	case codes.DeadlineExceeded:
		return protocol.ErrTimeout, detail
	case codes.Unavailable:
		return protocol.ErrNetwork, detail
	default:
		return protocol.ErrStatus, detail
	}
}

func stripScheme(target string) string {
	target = strings.TrimSpace(target)
	for _, scheme := range []string{"grpc://", "grpcs://", "http://", "https://"} {
		if strings.HasPrefix(target, scheme) {
			return strings.TrimSuffix(strings.TrimPrefix(target, scheme), "/")
		}
	}
	return target
}

func summarize(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 140 {
		return value[:140] + "…"
	}
	return value
}
