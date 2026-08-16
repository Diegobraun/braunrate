package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	Method          string
	Path            string
	Headers         map[string]string
	Body            []byte
	ContentType     string
	Timeout         time.Duration
	FollowRedirects *bool
}

func (config *Config) Protocol() string { return "http" }

func (config *Config) AggregationKey() string {
	return fmt.Sprintf("%s %s", config.Method, config.Path)
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Path = resolve(config.Path)
	clone.Headers = make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		clone.Headers[name] = resolve(value)
	}
	if len(config.Body) > 0 {
		clone.Body = []byte(resolve(string(config.Body)))
	}
	return &clone
}

func (config *Config) WithHeader(name, value string) protocol.Config {
	clone := *config
	clone.Headers = make(map[string]string, len(config.Headers)+1)
	for key, content := range config.Headers {
		clone.Headers[key] = content
	}
	clone.Headers[name] = value
	return &clone
}

func (config *Config) Describe() []string {
	lines := []string{fmt.Sprintf("%s %s", config.Method, config.Path)}

	names := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, transport.MaskSecret(name, config.Headers[name])))
	}
	// A declared header wins over the one inferred from the body, so printing
	// both showed a Content-Type that never went on the wire.
	if config.ContentType != "" && !hasHeader(config.Headers, "Content-Type") {
		lines = append(lines, "Content-Type: "+config.ContentType)
	}
	if len(config.Body) > 0 {
		lines = append(lines, "corpo: "+string(config.Body))
	}
	if config.Timeout > 0 {
		lines = append(lines, "timeout: "+config.Timeout.String())
	}
	return lines
}

type Protocol struct {
	client  *http.Client
	options protocol.Options
}

func New(options protocol.Options) *Protocol {
	return &Protocol{client: transport.NewClient(options), options: options}
}

func (implementation *Protocol) Name() string { return "http" }

func (implementation *Protocol) Close() error {
	implementation.client.CloseIdleConnections()
	return nil
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil {
		return nil, errors.New("passo http sem configuracao")
	}
	config := Default()

	if node.Kind == yaml.ScalarNode {
		parts := strings.Fields(node.Value)
		switch len(parts) {
		case 1:
			config.Path = parts[0]
		case 2:
			config.Method = strings.ToUpper(parts[0])
			config.Path = parts[1]
		default:
			return nil, fmt.Errorf("forma curta do passo http deve ser \"METODO /caminho\", recebido %q", node.Value)
		}
		return config, nil
	}

	if node.Kind != yaml.MappingNode {
		return nil, errors.New("passo http precisa ser um texto ou um mapa")
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "metodo":
			config.Method = strings.ToUpper(value.Value)
		case "caminho", "url":
			config.Path = value.Value
		case "cabecalhos":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Headers[value.Content[i].Value] = value.Content[i+1].Value
			}
		case "corpo":
			body, kind, err := readBody(value)
			if err != nil {
				return nil, err
			}
			config.Body = body
			config.ContentType = kind
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q", value.Value)
			}
			config.Timeout = duration
		case "seguir_redirect":
			follow := value.Value == "true"
			config.FollowRedirects = &follow
		default:
			return nil, fmt.Errorf("chave desconhecida no passo http: %q", key.Value)
		}
	}

	if err := Validate(config); err != nil {
		return nil, err
	}
	return config, nil
}

// Default and Validate exist so the Go DSL enters through the same door as the
// YAML: a default applied by only one of them would become a measurement
// difference between the two audiences.
func Default() *Config {
	return &Config{Method: http.MethodGet, Headers: map[string]string{}}
}

func Validate(config *Config) error {
	if config.Path == "" {
		return errors.New("passo http sem caminho")
	}
	return nil
}

func readBody(node *yaml.Node) ([]byte, string, error) {
	if node.Kind == yaml.ScalarNode {
		return []byte(node.Value), "text/plain", nil
	}
	var structure any
	if err := node.Decode(&structure); err != nil {
		return nil, "", fmt.Errorf("corpo invalido: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, "", fmt.Errorf("corpo nao serializa para JSON: %v", err)
	}
	return body, "application/json", nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "configuracao nao e de http"}
	}

	address, err := transport.BuildURL(request.URLBase, config.Path)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	var body io.Reader
	if len(config.Body) > 0 {
		body = bytes.NewReader(config.Body)
	}

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, config.Timeout)
		defer cancel()
	}

	order, err := http.NewRequestWithContext(runContext, config.Method, address, body)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}
	if config.ContentType != "" {
		order.Header.Set("Content-Type", config.ContentType)
	}
	for name, value := range config.Headers {
		order.Header.Set(name, value)
	}

	response, err := implementation.client.Do(order)
	if err != nil {
		return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
	}
	defer func() { _ = response.Body.Close() }()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return protocol.Response{Status: response.StatusCode, Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
	}

	class := protocol.Success
	detail := ""
	if response.StatusCode >= 400 {
		class = protocol.ErrStatus
		detail = fmt.Sprintf("status %d", response.StatusCode)
	}

	return protocol.Response{
		Status:  response.StatusCode,
		Body:    content,
		Headers: response.Header,
		Bytes:   int64(len(content)),
		Class:   class,
		Detail:  detail,
	}
}

func hasHeader(headers map[string]string, name string) bool {
	for declared := range headers {
		if strings.EqualFold(declared, name) {
			return true
		}
	}
	return false
}

// RequestBody is what the engine measures the shape of. The field cannot serve
// as the method, so the name says what it is for.
func (config *Config) RequestBody() []byte { return config.Body }
