package wait

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"gopkg.in/yaml.v3"
)

const defaultTimeout = 30 * time.Second

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Source    string
	Topic     string
	Addresses []string
	Expected  string
	Field     string
	Timeout   time.Duration

	Path     string
	To       Condition
	Interval time.Duration
}

func (config *Config) Protocol() string { return "await" }

func (config *Config) AggregationKey() string {
	if config.Source == "http" {
		return "await " + config.Path
	}
	return "await " + config.Topic
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Topic = resolve(config.Topic)
	clone.Expected = resolve(config.Expected)
	clone.Field = resolve(config.Field)
	clone.Path = resolve(config.Path)
	clone.To.Value = resolve(config.To.Value)
	clone.To.BodyContains = resolve(config.To.BodyContains)
	return &clone
}

func (config *Config) Describe() []string {
	if config.Source == "http" {
		interval := config.Interval
		if interval <= 0 {
			interval = defaultInterval
		}
		return []string{
			fmt.Sprintf("await on GET %s until %s", config.Path, config.To.describe()),
			fmt.Sprintf("polling every %s, gives up after %s", interval, config.Timeout),
			"the measured latency has the granularity of the polling",
		}
	}
	where := "message key"
	if config.Field != "" {
		where = config.Field
	}
	lines := []string{
		fmt.Sprintf("await on %s %s for %s = %q", config.Source, config.Topic, where, config.Expected),
		"gives up after " + config.Timeout.String(),
	}
	// Printing "addresses:" with nothing after it reads like a defect. When the
	// step declares none, the address is the target of the scenario, and saying
	// so is what the reader needs.
	if len(config.Addresses) > 0 {
		lines = append(lines, "addresses: "+strings.Join(config.Addresses, ", "))
	} else {
		lines = append(lines, "addresses: the ones from the scenario target")
	}
	return lines
}

type Protocol struct {
	mu            sync.Mutex
	subscriptions map[string]*subscription
	http          *nethttp.Client
	tls           *tls.Config
}

// Polling over HTTPS reaches the same target the HTTP steps reach, so it uses
// the same settings the scenario declared.
func (implementation *Protocol) UseTLS(settings *tls.Config) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	implementation.tls = settings
	if implementation.http != nil {
		implementation.http.CloseIdleConnections()
		implementation.http = nil
	}
}

func New(protocol.Options) *Protocol {
	return &Protocol{subscriptions: map[string]*subscription{}}
}

func (implementation *Protocol) Name() string { return "await" }

func (implementation *Protocol) Close() error {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	for key, subscription := range implementation.subscriptions {
		subscription.shutdown()
		delete(implementation.subscriptions, key)
	}
	return nil
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, errors.New(`an await step has to be a map, for example:
  - await:
      kafka: { topic: orders-processed }
      key: "${orderId}"
      timeout: 30s`)
	}

	config := Default()
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "kafka", "amqp":
			config.Source = key.Value
			if err := readSource(config, value); err != nil {
				return nil, err
			}
		case "http":
			config.Source = "http"
			if err := readHTTPSource(config, value); err != nil {
				return nil, err
			}
		case "until":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New(`until has to be a map, for example:
      until: { $.status: PROCESSED }
      until: { status: 200 }`)
			}
			raw := map[string]string{}
			for i := 0; i+1 < len(value.Content); i += 2 {
				raw[value.Content[i].Value] = value.Content[i+1].Value
			}
			condition, err := readCondition("until", raw)
			if err != nil {
				return nil, err
			}
			config.To = condition
		case "interval":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid interval: %q (use 200ms, 1s)", value.Value)
			}
			config.Interval = duration
		case "key":
			config.Expected = value.Value
		case "field":
			config.Field = value.Value
		case "equals":
			config.Expected = value.Value
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout: %q (use 30s, 2m)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("unknown key in the await step: %q (use kafka, amqp, http, key, field, equals, until, interval or timeout)", key.Value)
		}
	}

	if err := Validate(config); err != nil {
		return nil, err
	}
	return config, nil
}

func Default() *Config {
	return &Config{Timeout: defaultTimeout}
}

// Validate enforces correlation for the DSL too: without it the measurement
// would take the first message to show up and time the fastest consumer.
func Validate(config *Config) error {
	if config.Source == "http" {
		return validateHTTP(config)
	}
	if config.Source == "" {
		return errors.New(`an await step has to say where to wait, for example:
  - await:
      kafka: { topic: orders-processed }
      key: "${orderId}"`)
	}
	if config.Expected == "" {
		return errors.New(`an await step needs the value that identifies this iteration's message.
Without it any message would do, and the measurement would time the fastest consumer instead of the chain:
  - await:
      kafka: { topic: orders-processed }
      key: "${orderId}"`)
	}
	return nil
}

// Polling without a condition would measure the first response instead of the
// effect: the step would end before the system did what it had to do.
func validateHTTP(config *Config) error {
	if config.Path == "" {
		return errors.New(`an await step over http needs the path, for example:
  - await:
      http: { path: "/orders/${orders.id}" }
      until: { $.status: PROCESSED }`)
	}
	if config.To.empty() {
		return errors.New(`an await step over http needs 'until': with no condition the first response would end the wait,
and the measurement would be of the time to answer, not of the time until the effect happened:
  - await:
      http: { path: "/orders/${orders.id}" }
      until: { $.status: PROCESSED }`)
	}
	return nil
}

func readHTTPSource(config *Config, node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		config.Path = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return errors.New("await.http has to be the path or a map with 'path'")
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "path", "url":
			config.Path = value.Value
		default:
			return fmt.Errorf("unknown key in await.http: %q (use path)", key.Value)
		}
	}
	return nil
}

func readSource(config *Config, node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		config.Topic = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("the source %q has to be the topic name or a map", config.Source)
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "topic", "queue":
			config.Topic = value.Value
		case "brokers", "url", "addresses":
			if value.Kind == yaml.ScalarNode {
				config.Addresses = strings.Split(value.Value, ",")
				continue
			}
			for _, item := range value.Content {
				config.Addresses = append(config.Addresses, item.Value)
			}
		default:
			return fmt.Errorf("unknown key in await.%s: %q (use topic or brokers)", config.Source, key.Value)
		}
	}
	if config.Topic == "" {
		return fmt.Errorf("await.%s with no topic", config.Source)
	}
	return nil
}

// Prepare opens the subscription before the load: the read offset is fixed
// now, not after the first message has already been produced.
func (implementation *Protocol) Prepare(_ context.Context, request protocol.Request) error {
	config, ok := request.Config.(*Config)
	if !ok || config.Source == "http" {
		return nil
	}
	_, err := implementation.subscribe(config, request.URLBase, request.Messaging.BrokerFor(config.Source))
	return err
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "the configuration is not an await one"}
	}

	if config.Source == "http" {
		return implementation.awaitOverHTTP(runContext, request, config)
	}

	subscription, err := implementation.subscribe(config, request.URLBase, request.Messaging.BrokerFor(config.Source))
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	message, arrived := subscription.await(runContext, config.Expected, timeout)
	if !arrived {
		// "timed out" is what happened, not what to do about it. Nothing
		// arriving has two usual causes, and both are checked somewhere else.
		return protocol.Response{
			Class: protocol.ErrTimeout,
			Detail: fmt.Sprintf("the message with %s=%q did not arrive within %s on topic %s.\n"+
				"check that a consumer is running and that it writes to that topic;\n"+
				"and that both sides use the same correlation value — here the expected one is %q",
				lookupField(config), config.Expected, timeout, config.Topic, config.Expected),
		}
	}

	return protocol.Response{
		Body:       message.body,
		Bytes:      int64(len(message.body)),
		Class:      protocol.Success,
		Attributes: message.attributes,
		Collapses:  message.collapses,
	}
}

func lookupField(config *Config) string {
	if config.Field != "" {
		return config.Field
	}
	return "key"
}

func (implementation *Protocol) subscribe(config *Config, target string, broker *messaging.Broker) (*subscription, error) {
	addresses := config.Addresses
	if len(addresses) == 0 && broker != nil {
		addresses = broker.Addresses
	}
	if len(addresses) == 0 {
		addresses = targetAddresses(target)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("await on %s with no address: declare 'brokers' (kafka) or 'url' (amqp) in the step, or point the scenario target at the broker", config.Source)
	}

	key := config.Source + "|" + strings.Join(addresses, ",") + "|" + config.Topic + "|" + broker.Describe()

	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if existing, has := implementation.subscriptions[key]; has {
		return existing, nil
	}

	created, err := openSubscription(config, addresses, broker)
	if err != nil {
		return nil, err
	}
	implementation.subscriptions[key] = created
	return created, nil
}

func targetAddresses(target string) []string {
	if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return nil
	}
	return strings.Split(strings.TrimSuffix(target, "/"), ",")
}

// Waiting over HTTP needs no broker; waiting on a topic does, and it is the
// same address the producing step would use.
func (config *Config) BrokerTechnology() string {
	if config.Source == "http" {
		return ""
	}
	return config.Source
}

func (config *Config) DeclaredBrokers() []string { return config.Addresses }
