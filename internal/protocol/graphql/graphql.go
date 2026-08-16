package graphql

import (
	"bytes"
	"context"
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

func (c *Config) Protocol() string { return "graphql" }

// AggregationKey is the operation, never the URL: in GraphQL every operation
// arrives at the same address, and aggregating by URL would put the cheapest
// query and the most expensive mutation on the same row.
func (c *Config) AggregationKey() string {
	return "graphql " + c.Operation
}

func (c *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *c
	clone.Vars = resolve(c.Vars)
	clone.Path = resolve(c.Path)
	clone.Headers = make(map[string]string, len(c.Headers))
	for name, value := range c.Headers {
		clone.Headers[name] = resolve(value)
	}
	return &clone
}

func (c *Config) WithHeader(name, value string) protocol.Config {
	clone := *c
	clone.Headers = make(map[string]string, len(c.Headers)+1)
	for key, content := range c.Headers {
		clone.Headers[key] = content
	}
	clone.Headers[name] = value
	return &clone
}

func (c *Config) Describe() []string {
	lines := []string{fmt.Sprintf("%s %s em POST %s", c.Kind, c.Operation, c.Path)}

	names := make([]string, 0, len(c.Headers))
	for name := range c.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, transport.MaskSecret(name, c.Headers[name])))
	}
	if c.Vars != "" && c.Vars != "{}" {
		lines = append(lines, "variaveis: "+c.Vars)
	}
	lines = append(lines, "consulta: "+summarizeQuery(c.Query))
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
	client *http.Client
}

func New(opts protocol.Options) *Protocol {
	return &Protocol{client: transport.NewClient(opts)}
}

func (p *Protocol) Name() string { return "graphql" }

func (p *Protocol) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

var operationPattern = regexp.MustCompile(`(?s)\b(query|mutation|subscription)\s+([A-Za-z_][A-Za-z0-9_]*)`)

func (p *Protocol) Decode(no *yaml.Node) (protocol.Config, error) {
	if no == nil {
		return nil, errors.New("passo graphql sem configuracao")
	}
	config := Default()

	if no.Kind == yaml.ScalarNode {
		config.Query = no.Value
		return Finish(config)
	}
	if no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo graphql precisa ser a consulta ou um mapa, por exemplo:
  - graphql: |
      query ConsultarPedido { pedido(id: "1") { status } }`)
	}

	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
		switch key.Value {
		case "consulta", "query":
			config.Query = value.Value
		case "operacao":
			config.Operation = value.Value
		case "variaveis":
			vars, err := readVars(value)
			if err != nil {
				return nil, err
			}
			config.Vars = vars
		case "caminho", "url":
			config.Path = value.Value
		case "cabecalhos":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Headers[value.Content[i].Value] = value.Content[i+1].Value
			}
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 30s, 2m)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("chave desconhecida no passo graphql: %q (use consulta, operacao, variaveis, caminho, cabecalhos ou timeout)", key.Value)
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
		return nil, errors.New(`a operacao graphql precisa de nome: e o nome que vira a linha do relatorio.
Sem nome, todas as operacoes cairiam na mesma linha e a mais cara ficaria escondida na media.
  - graphql: |
      query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }`)
	}
	if config.Path == "" {
		config.Path = defaultPath
	}
	return config, nil
}

func readVars(no *yaml.Node) (string, error) {
	if no.Kind == yaml.ScalarNode {
		return no.Value, nil
	}
	var structure any
	if err := no.Decode(&structure); err != nil {
		return "", fmt.Errorf("variaveis invalidas: %v", err)
	}
	content, err := json.Marshal(structure)
	if err != nil {
		return "", fmt.Errorf("variaveis nao serializam para JSON: %v", err)
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

func (p *Protocol) Execute(ctx context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "configuracao nao e de graphql"}
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
				Detail: "as variaveis nao formaram JSON valido depois da interpolacao: " + summarize(vars),
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
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	order, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(serialized))
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}
	order.Header.Set("Content-Type", "application/json")
	order.Header.Set("Accept", "application/json")
	for name, value := range config.Headers {
		order.Header.Set(name, value)
	}

	response, err := p.client.Do(order)
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
		return protocol.ErrGraphQL, "a resposta nao e JSON de GraphQL: " + summarize(string(content))
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
		detail = fmt.Sprintf("%s (+%d erro(s))", detail, len(body.Errors)-1)
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
