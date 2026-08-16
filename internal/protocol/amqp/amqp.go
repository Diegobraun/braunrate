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

func (c *Config) Protocol() string { return "amqp" }

// A chave e a rota do negocio, e nao a conexao: e o que aparece no relatorio
// quando uma rota especifica fica lenta.
func (c *Config) AggregationKey() string {
	destination := c.Route
	if c.Exchange != "" {
		destination = c.Exchange + "/" + c.Route
	}
	return "amqp publicar " + destination
}

func (c *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *c
	clone.Exchange = resolve(c.Exchange)
	clone.Route = resolve(c.Route)
	clone.Queue = resolve(c.Queue)
	clone.Identity = resolve(c.Identity)
	if len(c.Body) > 0 {
		clone.Body = []byte(resolve(string(c.Body)))
	}
	clone.Headers = make(map[string]string, len(c.Headers))
	for name, value := range c.Headers {
		clone.Headers[name] = resolve(value)
	}
	return &clone
}

func (c *Config) Describe() []string {
	lines := []string{fmt.Sprintf("publicar em troca %q com rota %q", c.Exchange, c.Route)}
	if c.Identity != "" {
		lines = append(lines, "identidade da mensagem: "+c.Identity)
	}
	if c.Confirm {
		lines = append(lines, "espera confirmacao do broker")
	}
	if len(c.Body) > 0 {
		lines = append(lines, "corpo: "+string(c.Body))
	}
	return lines
}

type Protocol struct {
	mu    sync.Mutex
	conns map[string]*conn
}

type conn struct {
	link     *amqp.Connection
	mu       sync.Mutex
	canais   chan *amqp.Channel
	confirms bool
}

func New(protocol.Options) *Protocol {
	return &Protocol{conns: map[string]*conn{}}
}

func (p *Protocol) Name() string { return "amqp" }

func (p *Protocol) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for address, open := range p.conns {
		close(open.canais)
		for canal := range open.canais {
			_ = canal.Close()
		}
		_ = open.link.Close()
		delete(p.conns, address)
	}
	return nil
}

func (p *Protocol) Decode(no *yaml.Node) (protocol.Config, error) {
	if no == nil || no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo amqp precisa ser um mapa, por exemplo:
  - amqp:
      fila: pedidos
      corpo: { id: "${assinantes.id}" }`)
	}

	config := Default()
	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
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

func readBody(no *yaml.Node) ([]byte, error) {
	if no.Kind == yaml.ScalarNode {
		return []byte(no.Value), nil
	}
	var structure any
	if err := no.Decode(&structure); err != nil {
		return nil, fmt.Errorf("corpo invalido: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, fmt.Errorf("corpo nao serializa para JSON: %v", err)
	}
	return body, nil
}

func (p *Protocol) Execute(ctx context.Context, request protocol.Request) protocol.Response {
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

	open, err := p.conexaoDe(normalize(address), config)
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
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
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

	confirmation, err := canal.PublishWithDeferredConfirmWithContext(ctx, config.Exchange, config.Route, false, false, delivery)
	if err != nil {
		return protocol.Response{Class: classificar(err), Detail: summarize(err.Error())}
	}

	// Sem esperar a confirmacao, o tempo medido seria o de escrever no socket,
	// e nao o de o broker aceitar a mensagem — mediria a rede local.
	if config.Confirm && confirmation != nil {
		accepts, err := confirmation.WaitContext(ctx)
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

func (p *Protocol) conexaoDe(address string, config *Config) (*conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, has := p.conns[address]; has {
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

	p.conns[address] = created
	return created, nil
}

// Um canal AMQP nao e seguro para uso concorrente, entao cada requisicao pega
// um do pool e devolve; abrir um canal por mensagem custaria um ida e volta a
// mais dentro da medicao.
func (c *conn) pegarCanal() (*amqp.Channel, error) {
	select {
	case canal := <-c.canais:
		if canal != nil && !canal.IsClosed() {
			return canal, nil
		}
	default:
	}

	canal, err := c.link.Channel()
	if err != nil {
		return nil, err
	}
	if c.confirms {
		if err := canal.Confirm(false); err != nil {
			_ = canal.Close()
			return nil, err
		}
	}
	return canal, nil
}

func (c *conn) devolverCanal(canal *amqp.Channel) {
	if canal == nil || canal.IsClosed() {
		return
	}
	select {
	case c.canais <- canal:
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
