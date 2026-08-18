package grpc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Service  string
	Method   string
	Message  string
	Metadata map[string]string
	Target   string
	Timeout  time.Duration
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
	lines = append(lines, "note: this build has no gRPC transport compiled in (ADR 0022)")
	return lines
}

func (config *Config) RequestBody() []byte { return []byte(config.Message) }

type Protocol struct {
	options protocol.Options
}

func New(options protocol.Options) *Protocol { return &Protocol{options: options} }

func (implementation *Protocol) Name() string { return "grpc" }

func (implementation *Protocol) Close() error { return nil }

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
			return nil, fmt.Errorf("unknown key in the grpc step: %q (use method, service, message, target, metadata or timeout)", key.Value)
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
	return protocol.Response{
		Class:  protocol.ErrConfig,
		Detail: fmt.Sprintf("grpc %s is declared but this build has no gRPC transport compiled in (ADR 0022)", config.fullMethod()),
	}
}

func summarize(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 140 {
		return value[:140] + "…"
	}
	return value
}
