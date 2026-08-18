package websocket

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	xws "golang.org/x/net/websocket"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Path        string
	Send        string
	AwaitReply  bool
	MaxMessages int64
	Origin      string
	Headers     map[string]string
	Timeout     time.Duration
}

func (config *Config) Protocol() string { return "websocket" }

func (config *Config) AggregationKey() string {
	if config.Path == "" {
		return "websocket /"
	}
	return "websocket " + config.Path
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Path = resolve(config.Path)
	clone.Send = resolve(config.Send)
	clone.Origin = resolve(config.Origin)
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
	action := "send"
	switch {
	case config.MaxMessages > 0:
		action = fmt.Sprintf("send and drain up to %d messages", config.MaxMessages)
	case config.AwaitReply:
		action = "send and await one reply"
	}
	lines := []string{fmt.Sprintf("connect %s, %s", config.Path, action)}

	names := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, transport.MaskSecret(name, config.Headers[name])))
	}
	if config.Send != "" {
		lines = append(lines, "send: "+summarize(config.Send))
	}
	return lines
}

func (config *Config) RequestBody() []byte { return []byte(config.Send) }

type Protocol struct {
	options protocol.Options
}

func New(options protocol.Options) *Protocol { return &Protocol{options: options} }

func (implementation *Protocol) Name() string { return "websocket" }

func (implementation *Protocol) Close() error { return nil }

func (implementation *Protocol) UseTLS(settings *tls.Config) {
	implementation.options.TLS = settings
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil {
		return nil, errors.New("websocket step with no configuration")
	}
	config := &Config{Headers: map[string]string{}}

	if node.Kind == yaml.ScalarNode {
		config.Path = node.Value
		return finish(config)
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New(`a websocket step is the path or a map, like this:
  - websocket:
      path: /stream
      send: '{"subscribe":"orders"}'`)
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "path", "url":
			config.Path = value.Value
		case "send":
			config.Send = value.Value
		case "awaitReply", "await":
			config.AwaitReply = value.Value == "true"
		case "maxMessages", "max_messages":
			count, err := strconv.ParseInt(strings.TrimSpace(value.Value), 10, 64)
			if err != nil || count < 0 {
				return nil, fmt.Errorf("maxMessages has to be a whole number, got %q", value.Value)
			}
			config.MaxMessages = count
		case "origin":
			config.Origin = value.Value
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
			return nil, fmt.Errorf("unknown key in the websocket step: %q (use path, send, awaitReply, maxMessages, origin, headers or timeout)", key.Value)
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
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not a websocket one"}
	}

	address, err := wsURL(request.URLBase, config.Path)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}
	origin := config.Origin
	if origin == "" {
		origin = originFor(address)
	}
	settings, err := xws.NewConfig(address, origin)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}
	if implementation.options.TLS != nil {
		settings.TlsConfig = implementation.options.TLS
	}
	settings.Header = http.Header{}
	for name, value := range config.Headers {
		settings.Header.Set(name, value)
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = implementation.options.Timeout
	}
	deadline := time.Now().Add(timeout)

	// x/net/websocket dial ignores context; a buffered channel lets a cancelled
	// run return without waiting on the TCP timeout.
	type dialed struct {
		conn *xws.Conn
		err  error
	}
	channel := make(chan dialed, 1)
	go func() { conn, err := xws.DialConfig(settings); channel <- dialed{conn, err} }()

	var conn *xws.Conn
	select {
	case <-runContext.Done():
		return protocol.Response{Class: protocol.ErrTimeout, Detail: "the run ended before the websocket connected"}
	case result := <-channel:
		if result.err != nil {
			return protocol.Response{Class: transport.Classify(result.err), Detail: transport.SummarizeError(result.err)}
		}
		conn = result.conn
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(deadline)

	if config.Send != "" {
		if err := xws.Message.Send(conn, config.Send); err != nil {
			return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
		}
	}

	out := protocol.Response{Class: protocol.Success, Bytes: int64(len(config.Send))}
	if config.MaxMessages > 0 {
		var count, bytes int64
		var first []byte
		for {
			var reply []byte
			if err := xws.Message.Receive(conn, &reply); err != nil {
				if isStreamEnd(err) {
					break
				}
				return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err), Messages: count, Bytes: out.Bytes + bytes}
			}
			count++
			bytes += int64(len(reply))
			if first == nil {
				first = reply
			}
			if count >= config.MaxMessages {
				break
			}
		}
		out.Messages = count
		out.Bytes += bytes
		out.Body = first
		return out
	}
	if config.AwaitReply {
		var reply []byte
		if err := xws.Message.Receive(conn, &reply); err != nil {
			return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
		}
		out.Body = reply
		out.Bytes += int64(len(reply))
	}
	return out
}

// isStreamEnd tells the two clean ways a drained stream stops — the server
// closed, or the deadline that bounds the stream arrived — from a real failure.
func isStreamEnd(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func wsURL(base, path string) (string, error) {
	if strings.HasPrefix(path, "ws://") || strings.HasPrefix(path, "wss://") {
		return path, nil
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid target %q for a websocket", base)
	}
	if parsed.Scheme == "https" || parsed.Scheme == "wss" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	if path != "" {
		if strings.HasPrefix(path, "/") {
			parsed.Path = path
		} else {
			parsed.Path = "/" + path
		}
	}
	return parsed.String(), nil
}

func originFor(address string) string {
	parsed, err := url.Parse(address)
	if err != nil {
		return "http://localhost"
	}
	scheme := "http"
	if parsed.Scheme == "wss" {
		scheme = "https"
	}
	return scheme + "://" + parsed.Host
}

func summarize(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 140 {
		return value[:140] + "…"
	}
	return value
}
