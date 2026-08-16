package graphql

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	"github.com/Diegobraun/braunrate/internal/texto"
	"gopkg.in/yaml.v3"
)

const defaultPath = "/graphql"

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Operation string
	Kind      string
	Query     string
	Vars      string
	Path      string
	Headers   map[string]string
	Timeout   time.Duration
}

func (config *Config) Protocol() string { return "graphql" }

// AggregationKey is the operation, never the URL: in GraphQL every operation
// arrives at the same address, and aggregating by URL would put the cheapest
// query and the most expensive mutation on the same row.
func (config *Config) AggregationKey() string {
	return "graphql " + config.Operation
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Vars = resolve(config.Vars)
	clone.Path = resolve(config.Path)
	clone.Headers = make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		clone.Headers[name] = resolve(value)
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
	lines := []string{fmt.Sprintf("%s %s em POST %s", config.Kind, config.Operation, config.Path)}

	names := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, transport.MaskSecret(name, config.Headers[name])))
	}
	if config.Vars != "" && config.Vars != "{}" {
		lines = append(lines, "variables: "+config.Vars)
	}
	lines = append(lines, "query: "+summarizeQuery(config.Query))
	return lines
}

func summarizeQuery(query string) string {
	fields := strings.Join(strings.Fields(query), " ")
	if len(fields) > 160 {
		return fields[:160] + "…"
	}
	return fields
}

type Protocol struct {
	client  *http.Client
	options protocol.Options
}

func (implementation *Protocol) UseTLS(settings *tls.Config) {
	implementation.options.TLS = settings
	implementation.client.CloseIdleConnections()
	implementation.client = transport.NewClient(implementation.options)
}

func New(options protocol.Options) *Protocol {
	return &Protocol{client: transport.NewClient(options), options: options}
}

func (implementation *Protocol) Name() string { return "graphql" }

func (implementation *Protocol) Close() error {
	implementation.client.CloseIdleConnections()
	return nil
}

var operationPattern = regexp.MustCompile(`(?s)\b(query|mutation|subscription)\s+([A-Za-z_][A-Za-z0-9_]*)`)

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil {
		return nil, errors.New("graphql step with no configuration")
	}
	config := Default()

	if node.Kind == yaml.ScalarNode {
		config.Query = node.Value
		return Finish(config)
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New(`passo graphql precisa ser a consulta ou um mapa, por exemplo:
  - graphql: |
      query ConsultarPedido { pedido(id: "1") { status } }`)
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "query":
			config.Query = value.Value
		case "operation":
			config.Operation = value.Value
		case "variables":
			vars, err := readVars(value)
			if err != nil {
				return nil, err
			}
			config.Vars = vars
		case "path", "url":
			config.Path = value.Value
		case "headers":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("headers has to be a map")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Headers[value.Content[i].Value] = value.Content[i+1].Value
			}
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout: %q (use 30s, 2m)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("unknown key in the graphql step: %q (use query, operation, variables, path, headers or timeout)", key.Value)
		}
	}
	return Finish(config)
}

// Default and Finish are the single construction path: the Go DSL builds the
// same config the YAML builds, operation name extraction included.
func Default() *Config {
	return &Config{Path: defaultPath, Headers: map[string]string{}, Vars: "{}"}
}

func Finish(config *Config) (protocol.Config, error) {
	if strings.TrimSpace(config.Query) == "" {
		return nil, errors.New(`passo graphql sem consulta, por exemplo:
  - graphql: |
      query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }`)
	}

	parts := operationPattern.FindStringSubmatch(config.Query)
	if parts != nil {
		config.Kind = parts[1]
		if config.Operation == "" {
			config.Operation = parts[2]
		}
	}
	if config.Kind == "" {
		config.Kind = "query"
	}
	if config.Operation == "" {
		return nil, errors.New(`a operação graphql precisa de nome: e o nome que vira a linha do relatório.
Sem nome, todas as operações cairiam na mesma linha e a mais cara ficaria escondida na média.
  - graphql: |
      query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }`)
	}
	if config.Path == "" {
		config.Path = defaultPath
	}
	return config, nil
}

func readVars(node *yaml.Node) (string, error) {
	if node.Kind == yaml.ScalarNode {
		return node.Value, nil
	}
	var structure any
	if err := node.Decode(&structure); err != nil {
		return "", fmt.Errorf("invalid variables: %v", err)
	}
	content, err := json.Marshal(structure)
	if err != nil {
		return "", fmt.Errorf("the variables do not serialize to JSON: %v", err)
	}
	return string(content), nil
}

type requestBody struct {
	Query         string          `json:"query"`
	OperationName string          `json:"operationName,omitempty"`
	Vars          json.RawMessage `json:"variables,omitempty"`
}

type responseBody struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
	Extras json.RawMessage `json:"extensions,omitempty"`
}

type graphQLError struct {
	Message    string `json:"message"`
	Path       []any  `json:"path"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not a graphql one"}
	}

	address, err := transport.BuildURL(request.URLBase, config.Path)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	body := requestBody{Query: config.Query, OperationName: config.Operation}
	if vars := strings.TrimSpace(config.Vars); vars != "" && vars != "{}" {
		if !json.Valid([]byte(vars)) {
			return protocol.Response{
				Class:  protocol.ErrConfig,
				Detail: "the variables did not form valid JSON after interpolation: " + summarize(vars),
			}
		}
		body.Vars = json.RawMessage(vars)
	}
	serialized, err := json.Marshal(body)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, config.Timeout)
		defer cancel()
	}

	order, err := http.NewRequestWithContext(runContext, http.MethodPost, address, bytes.NewReader(serialized))
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}
	order.Header.Set("Content-Type", "application/json")
	order.Header.Set("Accept", "application/json")
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

	out := protocol.Response{
		Status:  response.StatusCode,
		Body:    content,
		Headers: response.Header,
		Bytes:   int64(len(content)),
		Class:   protocol.Success,
	}

	if response.StatusCode >= 400 {
		out.Class = protocol.ErrStatus
		out.Detail = fmt.Sprintf("status %d", response.StatusCode)
		return out
	}

	class, detail := classifyBody(content)
	out.Class = class
	out.Detail = detail
	return out
}

// A GraphQL error arrives with status 200: treating the step as a success
// because HTTP said 200 is the most common way a load test approves a service
// that is failing every single request.
func classifyBody(content []byte) (protocol.ErrorClass, string) {
	var body responseBody
	if err := json.Unmarshal(content, &body); err != nil {
		return protocol.ErrGraphQL, "the response is not GraphQL JSON: " + summarize(string(content))
	}
	if len(body.Errors) == 0 {
		if len(body.Data) == 0 || string(body.Data) == "null" {
			return protocol.ErrGraphQL, "resposta sem data e sem errors"
		}
		return protocol.Success, ""
	}

	first := body.Errors[0]
	detail := first.Message
	if first.Extensions.Code != "" {
		detail = first.Extensions.Code + ": " + detail
	}
	if path := formatPath(first.Path); path != "" {
		detail += " (em " + path + ")"
	}
	if len(body.Errors) > 1 {
		detail = fmt.Sprintf("%s (+%s)", detail, texto.Count(int64(len(body.Errors)-1), "erro", "erros"))
	}
	if len(body.Data) > 0 && string(body.Data) != "null" {
		detail = "resposta parcial — " + detail
	}
	return protocol.ErrGraphQL, summarize(detail)
}

func formatPath(path []any) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, 0, len(path))
	for _, item := range path {
		parts = append(parts, fmt.Sprint(item))
	}
	return strings.Join(parts, ".")
}

func summarize(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 140 {
		return text[:140] + "…"
	}
	return text
}

// The shape that matters in GraphQL is the one of the variables: the query is
// fixed by the scenario, and it is the variables that change per iteration.
func (config *Config) RequestBody() []byte { return []byte(config.Vars) }
