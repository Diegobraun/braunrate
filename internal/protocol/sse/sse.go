package sse

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
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
	Path        string
	Headers     map[string]string
	MaxMessages int64
	Timeout     time.Duration
}

func (config *Config) Protocol() string { return "sse" }

func (config *Config) AggregationKey() string {
	if config.Path == "" {
		return "sse /"
	}
	return "sse " + config.Path
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
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
	action := "read the event stream"
	if config.MaxMessages > 0 {
		action = fmt.Sprintf("read up to %d events", config.MaxMessages)
	}
	lines := []string{fmt.Sprintf("subscribe %s, %s", config.Path, action)}
	names := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, transport.MaskSecret(name, config.Headers[name])))
	}
	return lines
}

type Protocol struct {
	options protocol.Options
	client  *http.Client
}

func New(options protocol.Options) *Protocol {
	return &Protocol{options: options, client: streamingClient(options)}
}

func (implementation *Protocol) Name() string { return "sse" }

// streamingClient has no client-level timeout: a stream is meant to stay open,
// and the deadline that bounds it comes from the step's context instead.
func streamingClient(options protocol.Options) *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 65536,
		MaxConnsPerHost:     options.ConnsPerHost,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
		TLSClientConfig:     options.TLS,
	}}
}

func (implementation *Protocol) UseTLS(settings *tls.Config) {
	implementation.options.TLS = settings
	implementation.client.CloseIdleConnections()
	implementation.client = streamingClient(implementation.options)
}

func (implementation *Protocol) Close() error {
	implementation.client.CloseIdleConnections()
	return nil
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil {
		return nil, errors.New("sse step with no configuration")
	}
	config := &Config{Headers: map[string]string{}}

	if node.Kind == yaml.ScalarNode {
		config.Path = node.Value
		return finish(config)
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New(`an sse step is the path or a map, like this:
  - sse:
      path: /events
      maxMessages: 20`)
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "path", "url":
			config.Path = value.Value
		case "maxMessages", "max_messages":
			count, err := strconv.ParseInt(strings.TrimSpace(value.Value), 10, 64)
			if err != nil || count < 0 {
				return nil, fmt.Errorf("maxMessages has to be a whole number, got %q", value.Value)
			}
			config.MaxMessages = count
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
			return nil, fmt.Errorf("unknown key in the sse step: %q (use path, maxMessages, headers or timeout)", key.Value)
		}
	}
	return finish(config)
}

func finish(config *Config) (protocol.Config, error) {
	if config.Path == "" {
		config.Path = "/"
	}
	return config, nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not an sse one"}
	}
	address, err := transport.BuildURL(request.URLBase, config.Path)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = implementation.options.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, timeout)
		defer cancel()
	}

	order, err := http.NewRequestWithContext(runContext, http.MethodGet, address, nil)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}
	order.Header.Set("Accept", "text/event-stream")
	order.Header.Set("Cache-Control", "no-cache")
	for name, value := range config.Headers {
		order.Header.Set(name, value)
	}

	response, err := implementation.client.Do(order)
	if err != nil {
		return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return protocol.Response{Class: protocol.ErrStatus, Status: response.StatusCode, Detail: fmt.Sprintf("the event stream answered %d", response.StatusCode)}
	}

	return drain(runContext, response, config.MaxMessages)
}

// drain reads server-sent events until the server closes, maxMessages is
// reached, or the context deadline arrives. Events are separated by a blank
// line; only blocks that carried a data field are counted, matching what a
// browser's EventSource dispatches.
func drain(runContext context.Context, response *http.Response, maxMessages int64) protocol.Response {
	reader := bufio.NewReader(response.Body)
	var count, bytes int64
	var first, data []byte
	hasData := false

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			field := strings.TrimRight(line, "\r\n")
			switch {
			case field == "":
				if hasData {
					count++
					bytes += int64(len(data))
					if first == nil {
						first = append([]byte(nil), data...)
					}
					if maxMessages > 0 && count >= maxMessages {
						return protocol.Response{Class: protocol.Success, Status: response.StatusCode, Messages: count, Bytes: bytes, Body: first}
					}
				}
				data = data[:0]
				hasData = false
			case strings.HasPrefix(field, ":"):
				// A comment line keeps the connection alive; it is not an event.
			case strings.HasPrefix(field, "data:"):
				hasData = true
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(field, "data:"), " ")...)
			}
		}
		if err != nil {
			if err == io.EOF || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || runContext.Err() != nil {
				break
			}
			return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err), Status: response.StatusCode, Messages: count, Bytes: bytes}
		}
	}
	return protocol.Response{Class: protocol.Success, Status: response.StatusCode, Messages: count, Bytes: bytes, Body: first}
}
