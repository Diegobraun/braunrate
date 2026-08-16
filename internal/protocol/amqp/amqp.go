package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
	return "amqp publicar " + destination
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
	lines := []string{fmt.Sprintf("publicar em troca %q com rota %q", config.Exchange, config.Route)}
	if config.Identity != "" {
		lines = append(lines, "identidade da mensagem: "+config.Identity)
	}
	if config.Confirm {
		lines = append(lines, "espera confirmacao do broker")
	}
	if len(config.Body) > 0 {
		lines = append(lines, "corpo: "+string(config.Body))
	}
	return lines
}

type Protocol struct {
	mu    sync.Mutex
	conns map[string]*conn
}

type conn struct {
	link     *amqp.Connection
	canais   chan *amqp.Channel
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
		close(open.canais)
		for canal := range open.canais {
			_ = canal.Close()
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
		case "troca":
			config.Exchange = value.Value
		case "rota":
			config.Route = value.Value
		case "fila":
			config.Queue = value.Value
			if config.Route == "" {
				config.Route = value.Value
			}
		case "corpo":
			body, err := readBody(value)
			if err != nil {
				return nil, err
			}
			config.Body = body
		case "identidade":
			config.Identity = value.Value
		case "cabecalhos":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Headers[value.Content[i].Value] = value.Content[i+1].Value
			}
		case "url":
			config.URL = value.Value
		case "persistente":
			config.Persistent = value.Value == "true"
		case "confirmar":
			config.Confirm = value.Value == "true"
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 5s, 30s)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("chave desconhecida no passo amqp: %q (use fila, troca, rota, corpo, identidade, cabecalhos, url, persistente, confirmar ou timeout)", key.Value)
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
		return errors.New(`passo amqp sem corpo: uma mensagem vazia nao exercita o consumidor.
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
		return nil, fmt.Errorf("corpo invalido: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, fmt.Errorf("corpo nao serializa para JSON: %v", err)
	}
	return body, nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "configuracao nao e de amqp"}
	}

	address := config.URL
	if address == "" {
		address = request.URLBase
	}
	if address == "" || strings.HasPrefix(address, "http") {
		return protocol.Response{
			Class:  protocol.ErrConfig,
			Detail: "sem endereco: declare 'url' no passo ou aponte o alvo do cenario para amqp://usuario:senha@host:5672/",
		}
	}

	open, err := implementation.conexaoDe(normalize(address), config)
	if err != nil {
		return protocol.Response{Class: protocol.ErrNetwork, Detail: summarize(err.Error())}
	}

	canal, err := open.pegarCanal()
	if err != nil {
		return protocol.Response{Class: protocol.ErrNetwork, Detail: summarize(err.Error())}
	}
	defer open.devolverCanal(canal)

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

	confirmation, err := canal.PublishWithDeferredConfirmWithContext(runContext, config.Exchange, config.Route, false, false, delivery)
	if err != nil {
		return protocol.Response{Class: classificar(err), Detail: summarize(err.Error())}
	}

	// Without waiting for the confirmation the measured time would be the
	// socket write, not the broker accepting the message: it would time the
	// local network.
	if config.Confirm && confirmation != nil {
		accepts, err := confirmation.WaitContext(runContext)
		if err != nil {
			return protocol.Response{Class: classificar(err), Detail: summarize(err.Error())}
		}
		if !accepts {
			return protocol.Response{
				Class:  protocol.ErrMessaging,
				Detail: fmt.Sprintf("o broker recusou a mensagem para a rota %q", config.Route),
			}
		}
	}

	return protocol.Response{
		Bytes:      int64(len(config.Body)),
		Class:      protocol.Success,
		Attributes: map[string]string{"amqp.rota": config.Route},
	}
}

func (implementation *Protocol) conexaoDe(address string, config *Config) (*conn, error) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if existing, has := implementation.conns[address]; has {
		return existing, nil
	}

	link, err := amqp.Dial(address)
	if err != nil {
		return nil, err
	}
	created := &conn{link: link, canais: make(chan *amqp.Channel, 64), confirms: config.Confirm}

	if config.Queue != "" {
		canal, err := link.Channel()
		if err != nil {
			_ = link.Close()
			return nil, err
		}
		if _, err := canal.QueueDeclare(config.Queue, true, false, false, false, nil); err != nil {
			_ = canal.Close()
			_ = link.Close()
			return nil, fmt.Errorf("nao consegui declarar a fila %q: %v", config.Queue, err)
		}
		_ = canal.Close()
	}

	implementation.conns[address] = created
	return created, nil
}

// An AMQP channel is not safe for concurrent use, so each request takes one
// from the pool and returns it; opening a channel per message would put an
// extra round trip inside the measurement.
func (conn *conn) pegarCanal() (*amqp.Channel, error) {
	select {
	case canal := <-conn.canais:
		if canal != nil && !canal.IsClosed() {
			return canal, nil
		}
	default:
	}

	canal, err := conn.link.Channel()
	if err != nil {
		return nil, err
	}
	if conn.confirms {
		if err := canal.Confirm(false); err != nil {
			_ = canal.Close()
			return nil, err
		}
	}
	return canal, nil
}

func (conn *conn) devolverCanal(canal *amqp.Channel) {
	if canal == nil || canal.IsClosed() {
		return
	}
	select {
	case conn.canais <- canal:
	default:
		_ = canal.Close()
	}
}

func normalize(address string) string {
	if strings.HasPrefix(address, "amqp://") || strings.HasPrefix(address, "amqps://") {
		return address
	}
	return "amqp://" + address
}

func classificar(err error) protocol.ErrorClass {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocol.ErrTimeout
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
