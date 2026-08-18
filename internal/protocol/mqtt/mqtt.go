package mqtt

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	paho "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Topic    string
	Payload  []byte
	QoS      byte
	Retain   bool
	Broker   string
	ClientID string
	Username string
	Password string
	Timeout  time.Duration
}

func (config *Config) Protocol() string { return "mqtt" }

// AggregationKey is the topic, never the broker connection: it is what shows in
// the report when one topic gets slow to acknowledge.
func (config *Config) AggregationKey() string { return "mqtt publish " + config.Topic }

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Topic = resolve(config.Topic)
	clone.Broker = resolve(config.Broker)
	clone.Username = resolve(config.Username)
	clone.Password = resolve(config.Password)
	if len(config.Payload) > 0 {
		clone.Payload = []byte(resolve(string(config.Payload)))
	}
	return &clone
}

func (config *Config) Describe() []string {
	lines := []string{fmt.Sprintf("publish to %s (qos %d)", config.Topic, config.QoS)}
	if config.Broker != "" {
		lines = append(lines, "broker: "+config.Broker)
	}
	if config.Username != "" {
		lines = append(lines, "username: "+config.Username)
	}
	if len(config.Payload) > 0 {
		lines = append(lines, "payload: "+summarize(string(config.Payload)))
	}
	return lines
}

func (config *Config) RequestBody() []byte { return config.Payload }

type Protocol struct {
	options protocol.Options
	tls     *tls.Config

	mu      sync.Mutex
	clients map[string]paho.Client
}

func New(options protocol.Options) *Protocol {
	return &Protocol{options: options, clients: map[string]paho.Client{}}
}

func (implementation *Protocol) Name() string { return "mqtt" }

func (implementation *Protocol) UseTLS(settings *tls.Config) { implementation.tls = settings }

func (implementation *Protocol) Close() error {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	for _, client := range implementation.clients {
		client.Disconnect(250)
	}
	implementation.clients = map[string]paho.Client{}
	return nil
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil {
		return nil, errors.New("mqtt step with no configuration")
	}
	config := &Config{}

	if node.Kind == yaml.ScalarNode {
		config.Topic = node.Value
		return finish(config)
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New(`an mqtt step is the topic or a map, like this:
  - mqtt:
      topic: sensors/temperature
      payload: '{"c":21.4}'
      qos: 1`)
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "topic":
			config.Topic = value.Value
		case "payload", "body", "message":
			body, err := readBody(value)
			if err != nil {
				return nil, err
			}
			config.Payload = body
		case "qos":
			level, err := strconv.Atoi(strings.TrimSpace(value.Value))
			if err != nil || level < 0 || level > 2 {
				return nil, fmt.Errorf("qos is 0, 1 or 2, got %q", value.Value)
			}
			config.QoS = byte(level)
		case "retain":
			config.Retain = value.Value == "true"
		case "broker", "target":
			config.Broker = value.Value
		case "clientId", "client_id":
			config.ClientID = value.Value
		case "username", "user":
			config.Username = value.Value
		case "password":
			config.Password = value.Value
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout: %q (use 30s, 2m)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("unknown key in the mqtt step: %q (use topic, payload, qos, retain, broker, clientId, username, password or timeout)", key.Value)
		}
	}
	return finish(config)
}

// readBody takes a scalar payload as-is and serializes a map or list to JSON, so
// a step can carry a structured message without hand-escaping it.
func readBody(node *yaml.Node) ([]byte, error) {
	if node.Kind == yaml.ScalarNode {
		return []byte(node.Value), nil
	}
	var structure any
	if err := node.Decode(&structure); err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, fmt.Errorf("the payload does not serialize to JSON: %v", err)
	}
	return body, nil
}

func finish(config *Config) (protocol.Config, error) {
	if config.Topic == "" {
		return nil, errors.New(`an mqtt step needs a topic:
  - mqtt: sensors/temperature`)
	}
	return config, nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not an mqtt one"}
	}
	address := brokerURL(config.Broker)
	if address == "" {
		address = brokerURL(request.URLBase)
	}
	if address == "" {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "no broker for the mqtt publish (set the scenario target or the step's broker)"}
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = implementation.options.Timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	client, err := implementation.connect(address, config, timeout)
	if err != nil {
		return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
	}

	token := client.Publish(config.Topic, config.QoS, config.Retain, config.Payload)
	if !token.WaitTimeout(timeout) {
		return protocol.Response{Class: protocol.ErrTimeout, Detail: "the broker did not acknowledge the publish in time"}
	}
	if err := token.Error(); err != nil {
		// A broker reached but refusing to acknowledge drops the cached client so
		// the next iteration reconnects instead of reusing a broken session.
		implementation.forget(address)
		return protocol.Response{Class: transport.Classify(err), Detail: transport.SummarizeError(err)}
	}
	return protocol.Response{Class: protocol.Success, Bytes: int64(len(config.Payload))}
}

func (implementation *Protocol) connect(address string, config *Config, timeout time.Duration) (paho.Client, error) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if client, ok := implementation.clients[address]; ok && client.IsConnected() {
		return client, nil
	}

	options := paho.NewClientOptions().AddBroker(address)
	options.SetClientID(clientID(config))
	options.SetConnectTimeout(timeout)
	options.SetWriteTimeout(timeout)
	options.SetAutoReconnect(false)
	if config.Username != "" {
		options.SetUsername(config.Username)
		options.SetPassword(config.Password)
	}
	if implementation.tls != nil {
		options.SetTLSConfig(implementation.tls)
	}
	client := paho.NewClient(options)
	token := client.Connect()
	if !token.WaitTimeout(timeout) {
		return nil, errors.New("the broker did not answer the connection in time")
	}
	if err := token.Error(); err != nil {
		return nil, err
	}
	implementation.clients[address] = client
	return client, nil
}

func (implementation *Protocol) forget(address string) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if client, ok := implementation.clients[address]; ok {
		client.Disconnect(0)
		delete(implementation.clients, address)
	}
}

// clientID keeps one stable id per configured id, so the broker sees one session
// for the run instead of churning; MQTT ids must be unique per connection.
func clientID(config *Config) string {
	if config.ClientID != "" {
		return config.ClientID
	}
	return "braunrate"
}

// brokerURL normalizes to what paho expects (scheme://host:port), defaulting a
// bare host:port to tcp and mapping the mqtt scheme onto tcp.
func brokerURL(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if !strings.Contains(target, "://") {
		return "tcp://" + target
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	switch parsed.Scheme {
	case "mqtt":
		parsed.Scheme = "tcp"
	case "mqtts":
		parsed.Scheme = "ssl"
	}
	return parsed.String()
}

func summarize(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 140 {
		return value[:140] + "…"
	}
	return value
}
