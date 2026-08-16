package wait

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"
	"sync"
	"time"

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

func (config *Config) Protocol() string { return "aguardar" }

func (config *Config) AggregationKey() string {
	if config.Source == "http" {
		return "aguardar " + config.Path
	}
	return "aguardar " + config.Topic
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
			fmt.Sprintf("aguardar em GET %s ate %s", config.Path, config.To.describe()),
			fmt.Sprintf("sondando a cada %s, desiste depois de %s", interval, config.Timeout),
			"a latencia medida tem a granularidade da sondagem",
		}
	}
	where := "chave da mensagem"
	if config.Field != "" {
		where = config.Field
	}
	lines := []string{
		fmt.Sprintf("aguardar em %s %s por %s = %q", config.Source, config.Topic, where, config.Expected),
		"desiste depois de " + config.Timeout.String(),
	}
	if len(config.Addresses) > 0 {
		lines = append(lines, "enderecos: "+strings.Join(config.Addresses, ", "))
	}
	return lines
}

type Protocol struct {
	mu            sync.Mutex
	subscriptions map[string]*subscription
	http          *nethttp.Client
}

func New(protocol.Options) *Protocol {
	return &Protocol{subscriptions: map[string]*subscription{}}
}

func (implementation *Protocol) Name() string { return "aguardar" }

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
		return nil, errors.New(`passo aguardar precisa ser um mapa, por exemplo:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"
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
		case "ate":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New(`ate precisa ser um mapa, por exemplo:
      ate: { $.status: PROCESSADO }
      ate: { status: 200 }`)
			}
			raw := map[string]string{}
			for i := 0; i+1 < len(value.Content); i += 2 {
				raw[value.Content[i].Value] = value.Content[i+1].Value
			}
			condition, err := readCondition("ate", raw)
			if err != nil {
				return nil, err
			}
			config.To = condition
		case "intervalo":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("intervalo invalido: %q (use 200ms, 1s)", value.Value)
			}
			config.Interval = duration
		case "chave":
			config.Expected = value.Value
		case "campo":
			config.Field = value.Value
		case "igual_a":
			config.Expected = value.Value
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 30s, 2m)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("chave desconhecida no passo aguardar: %q (use kafka, amqp, http, chave, campo, igual_a, ate, intervalo ou timeout)", key.Value)
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
		return errors.New(`o passo aguardar precisa dizer onde esperar, por exemplo:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"`)
	}
	if config.Expected == "" {
		return errors.New(`o passo aguardar precisa do valor que identifica a mensagem desta iteracao.
Sem isso, qualquer mensagem serviria e a medicao mediria o consumidor mais rapido, nao a cadeia:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"`)
	}
	return nil
}

// Polling without a condition would measure the first response instead of the
// effect: the step would end before the system did what it had to do.
func validateHTTP(config *Config) error {
	if config.Path == "" {
		return errors.New(`o passo aguardar por http precisa do caminho, por exemplo:
  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }`)
	}
	if config.To.empty() {
		return errors.New(`o passo aguardar por http precisa de 'ate': sem condicao, a primeira resposta encerraria a espera
e a medicao seria do tempo de responder, nao do tempo ate o efeito acontecer:
  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }`)
	}
	return nil
}

func readHTTPSource(config *Config, node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		config.Path = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return errors.New("aguardar.http precisa ser o caminho ou um mapa com 'caminho'")
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "caminho", "url":
			config.Path = value.Value
		default:
			return fmt.Errorf("chave desconhecida em aguardar.http: %q (use caminho)", key.Value)
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
		return fmt.Errorf("a fonte %q precisa ser o nome do topico ou um mapa", config.Source)
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "topico", "fila":
			config.Topic = value.Value
		case "brokers", "url", "enderecos":
			if value.Kind == yaml.ScalarNode {
				config.Addresses = strings.Split(value.Value, ",")
				continue
			}
			for _, item := range value.Content {
				config.Addresses = append(config.Addresses, item.Value)
			}
		default:
			return fmt.Errorf("chave desconhecida em aguardar.%s: %q (use topico ou brokers)", config.Source, key.Value)
		}
	}
	if config.Topic == "" {
		return fmt.Errorf("aguardar.%s sem topico", config.Source)
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
	_, err := implementation.subscribe(config, request.URLBase)
	return err
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "configuracao nao e de aguardar"}
	}

	if config.Source == "http" {
		return implementation.awaitOverHTTP(runContext, request, config)
	}

	subscription, err := implementation.subscribe(config, request.URLBase)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	message, arrived := subscription.await(runContext, config.Expected, timeout)
	if !arrived {
		return protocol.Response{
			Class: protocol.ErrTimeout,
			Detail: fmt.Sprintf("a mensagem com %s=%q nao chegou em %s no topico %s",
				lookupField(config), config.Expected, timeout, config.Topic),
		}
	}

	return protocol.Response{
		Body:       message.body,
		Bytes:      int64(len(message.body)),
		Class:      protocol.Success,
		Attributes: message.attributes,
	}
}

func lookupField(config *Config) string {
	if config.Field != "" {
		return config.Field
	}
	return "chave"
}

func (implementation *Protocol) subscribe(config *Config, target string) (*subscription, error) {
	addresses := config.Addresses
	if len(addresses) == 0 {
		addresses = targetAddresses(target)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("aguardar em %s sem endereco: declare 'brokers' (kafka) ou 'url' (amqp) no passo, ou aponte o alvo do cenario para o broker", config.Source)
	}

	key := config.Source + "|" + strings.Join(addresses, ",") + "|" + config.Topic

	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if existing, has := implementation.subscriptions[key]; has {
		return existing, nil
	}

	created, err := openSubscription(config, addresses)
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
