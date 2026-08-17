package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/Diegobraun/braunrate/internal/protocol"
	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Exchange   string
	Route      string
	Queue      string
	Body       []byte
	Identity   string
	Headers    map[string]string
	URL        string
	Persistent bool
	Confirm    bool
	Timeout    time.Duration
}

func (config *Config) Protocol() string { return "amqp" }

// AggregationKey is the business route, never the connection: it is what shows
// in the report when one specific route gets slow.
func (config *Config) AggregationKey() string {
	destination := config.Route
	if config.Exchange != "" {
		destination = config.Exchange + "/" + config.Route
	}
	return "amqp publish " + destination
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Exchange = resolve(config.Exchange)
	clone.Route = resolve(config.Route)
	clone.Queue = resolve(config.Queue)
	clone.Identity = resolve(config.Identity)
	if len(config.Body) > 0 {
		clone.Body = []byte(resolve(string(config.Body)))
	}
	clone.Headers = make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		clone.Headers[name] = resolve(value)
	}
	return &clone
}

func (config *Config) Describe() []string {
	lines := []string{fmt.Sprintf("publish to exchange %q with route %q", config.Exchange, config.Route)}
	if config.Identity != "" {
		lines = append(lines, "message identity: "+config.Identity)
	}
	if config.Confirm {
		lines = append(lines, "waits for the broker to confirm")
	}
	if len(config.Body) > 0 {
		lines = append(lines, "body: "+string(config.Body))
	}
	return lines
}

type Protocol struct {
	mu    sync.Mutex
	conns map[string]*conn
}

type conn struct {
	link     *amqp.Connection
	channels chan *amqp.Channel
	confirms bool
}

func New(protocol.Options) *Protocol {
	return &Protocol{conns: map[string]*conn{}}
}

func (implementation *Protocol) Name() string { return "amqp" }

func (implementation *Protocol) Close() error {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	for address, open := range implementation.conns {
		close(open.channels)
		for channel := range open.channels {
			_ = channel.Close()
		}
		_ = open.link.Close()
		delete(implementation.conns, address)
	}
	return nil
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, errors.New(`passo amqp precisa ser um mapa, por exemplo:
  - amqp:
      fila: pedidos
      corpo: { id: "${assinantes.id}" }`)
	}

	config := Default()
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "exchange":
			config.Exchange = value.Value
		case "routingKey":
			config.Route = value.Value
		case "queue":
			config.Queue = value.Value
			if config.Route == "" {
				config.Route = value.Value
			}
		case "body":
			body, err := readBody(value)
			if err != nil {
				return nil, err
			}
			config.Body = body
		case "messageId":
			config.Identity = value.Value
		case "headers":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("headers has to be a map")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Headers[value.Content[i].Value] = value.Content[i+1].Value
			}
		case "url":
			config.URL = value.Value
		case "persistent":
			config.Persistent = value.Value == "true"
		case "confirm":
			config.Confirm = value.Value == "true"
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout: %q (use 5s, 30s)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("unknown key in the amqp step: %q (use queue, exchange, routingKey, body, messageId, headers, url, persistent, confirm or timeout)", key.Value)
		}
	}

	if err := Validate(config); err != nil {
		return nil, err
	}
	return config, nil
}

func Default() *Config {
	return &Config{Headers: map[string]string{}, Persistent: true, Confirm: true}
}

func Validate(config *Config) error {
	if config.Route == "" && config.Queue == "" {
		return errors.New(`passo amqp sem destino: declare 'fila' (caso comum) ou 'troca' com 'rota'.
  - amqp: { fila: pedidos, corpo: { id: "${assinantes.id}" } }`)
	}
	if len(config.Body) == 0 {
		return errors.New(`passo amqp sem corpo: uma mensagem vazia não exercita o consumidor.
  - amqp: { fila: pedidos, corpo: { id: "${assinantes.id}" } }`)
	}
	return nil
}

func readBody(node *yaml.Node) ([]byte, error) {
	if node.Kind == yaml.ScalarNode {
		return []byte(node.Value), nil
	}
	var structure any
	if err := node.Decode(&structure); err != nil {
		return nil, fmt.Errorf("invalid body: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, fmt.Errorf("the body does not serialize to JSON: %v", err)
	}
	return body, nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not an amqp one"}
	}

	broker := request.Messaging.BrokerFor("amqp")

	address := config.URL
	if address == "" && broker != nil && len(broker.Addresses) > 0 {
		address = broker.Addresses[0]
	}
	if address == "" {
		address = request.URLBase
	}
	if address == "" || strings.HasPrefix(address, "http") {
		return protocol.Response{
			Class:  protocol.ErrConfig,
			Detail: "no address: declare 'url' in the step or point the scenario target at amqp://user:password@host:5672/",
		}
	}

	open, err := implementation.conexaoDe(normalize(address), config, broker)
	if err != nil {
		if kind, credential := messaging.ClassifyError(err); credential {
			return protocol.Response{Class: classOf(kind), Detail: messaging.Explain(kind, broker)}
		}
		return protocol.Response{Class: protocol.ErrNetwork, Detail: summarize(err.Error())}
	}

	channel, err := open.takeChannel()
	if err != nil {
		return protocol.Response{Class: protocol.ErrNetwork, Detail: summarize(err.Error())}
	}
	defer open.returnChannel(channel)

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, config.Timeout)
		defer cancel()
	}

	delivery := amqp.Publishing{
		Body:        config.Body,
		ContentType: "application/json",
		MessageId:   config.Identity,
		Timestamp:   time.Now(),
	}
	if config.Persistent {
		delivery.DeliveryMode = amqp.Persistent
	}
	if len(config.Headers) > 0 {
		delivery.Headers = amqp.Table{}
		for name, value := range config.Headers {
			delivery.Headers[name] = value
		}
	}

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(runContext, config.Exchange, config.Route, false, false, delivery)
	if err != nil {
		return protocol.Response{Class: classify(err), Detail: summarize(err.Error())}
	}

	// Without waiting for the confirmation the measured time would be the
	// socket write, not the broker accepting the message: it would time the
	// local network.
	if config.Confirm && confirmation != nil {
		accepts, err := confirmation.WaitContext(runContext)
		if err != nil {
			return protocol.Response{Class: classify(err), Detail: summarize(err.Error())}
		}
		if !accepts {
			return protocol.Response{
				Class:  protocol.ErrMessaging,
				Detail: fmt.Sprintf("the broker refused the message for route %q", config.Route),
			}
		}
	}

	return protocol.Response{
		Bytes:      int64(len(config.Body)),
		Class:      protocol.Success,
		Attributes: map[string]string{"amqp.rota": config.Route},
	}
}

func (implementation *Protocol) conexaoDe(address string, config *Config, broker *messaging.Broker) (*conn, error) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	key := address + "|" + broker.Describe()
	if existing, has := implementation.conns[key]; has {
		return existing, nil
	}

	link, err := broker.DialAMQP(address)
	if err != nil {
		return nil, err
	}
	created := &conn{link: link, channels: make(chan *amqp.Channel, 64), confirms: config.Confirm}

	if config.Queue != "" {
		channel, err := link.Channel()
		if err != nil {
			_ = link.Close()
			return nil, err
		}
		if _, err := channel.QueueDeclare(config.Queue, true, false, false, false, nil); err != nil {
			_ = channel.Close()
			_ = link.Close()
			return nil, fmt.Errorf("could not declare the queue %q: %v", config.Queue, err)
		}
		_ = channel.Close()
	}

	implementation.conns[key] = created
	return created, nil
}

// An AMQP channel is not safe for concurrent use, so each request takes one
// from the pool and returns it; opening a channel per message would put an
// extra round trip inside the measurement.
func (conn *conn) takeChannel() (*amqp.Channel, error) {
	select {
	case channel := <-conn.channels:
		if channel != nil && !channel.IsClosed() {
			return channel, nil
		}
	default:
	}

	channel, err := conn.link.Channel()
	if err != nil {
		return nil, err
	}
	if conn.confirms {
		if err := channel.Confirm(false); err != nil {
			_ = channel.Close()
			return nil, err
		}
	}
	return channel, nil
}

func (conn *conn) returnChannel(channel *amqp.Channel) {
	if channel == nil || channel.IsClosed() {
		return
	}
	select {
	case conn.channels <- channel:
	default:
		_ = channel.Close()
	}
}

func normalize(address string) string {
	if strings.HasPrefix(address, "amqp://") || strings.HasPrefix(address, "amqps://") {
		return address
	}
	return "amqp://" + address
}

func classOf(kind string) protocol.ErrorClass {
	if kind == "authentication" {
		return protocol.ErrAuth
	}
	return protocol.ErrAuthorization
}

func classify(err error) protocol.ErrorClass {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocol.ErrTimeout
	}
	if kind, credential := messaging.ClassifyError(err); credential {
		return classOf(kind)
	}
	text := err.Error()
	if strings.Contains(text, "timeout") || strings.Contains(text, "deadline") {
		return protocol.ErrTimeout
	}
	if strings.Contains(text, "closed") || strings.Contains(text, "connection") {
		return protocol.ErrNetwork
	}
	return protocol.ErrMessaging
}

func summarize(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 140 {
		return text[:140] + "…"
	}
	return text
}

func (config *Config) RequestBody() []byte { return config.Body }

func (config *Config) BrokerTechnology() string { return "amqp" }

func (config *Config) DeclaredBrokers() []string {
	if config.URL == "" {
		return nil
	}
	return []string{config.URL}
}
