package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/segmentio/kafka-go"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Register(New(protocol.DefaultOptions()))
}

type Config struct {
	Topic   string
	Key     string
	Value   []byte
	Headers map[string]string
	Brokers []string
	Acks    string
	Timeout time.Duration
}

func (c *Config) Protocol() string { return "kafka" }

// A chave e o topico, e nao o broker: quem le o relatorio precisa saber qual
// fluxo de negocio ficou lento, nao qual maquina recebeu o byte.
func (c *Config) AggregationKey() string {
	return "kafka produzir " + c.Topic
}

func (c *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *c
	clone.Topic = resolve(c.Topic)
	clone.Key = resolve(c.Key)
	if len(c.Value) > 0 {
		clone.Value = []byte(resolve(string(c.Value)))
	}
	clone.Headers = make(map[string]string, len(c.Headers))
	for name, value := range c.Headers {
		clone.Headers[name] = resolve(value)
	}
	return &clone
}

func (c *Config) Describe() []string {
	lines := []string{fmt.Sprintf("produzir em %s (chave %q)", c.Topic, c.Key)}
	names := make([]string, 0, len(c.Headers))
	for name := range c.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("cabecalho %s: %s", name, c.Headers[name]))
	}
	if len(c.Brokers) > 0 {
		lines = append(lines, "brokers: "+strings.Join(c.Brokers, ", "))
	}
	lines = append(lines, "acks: "+c.Acks)
	if len(c.Value) > 0 {
		lines = append(lines, "valor: "+string(c.Value))
	}
	return lines
}

type Protocol struct {
	opts       protocol.Options
	mu         sync.Mutex
	writers    map[string]*kafka.Writer
	partitions map[string]int64
}

func New(opts protocol.Options) *Protocol {
	return &Protocol{opts: opts, writers: map[string]*kafka.Writer{}, partitions: map[string]int64{}}
}

func (p *Protocol) Name() string { return "kafka" }

func (p *Protocol) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var last error
	for key, writer := range p.writers {
		if err := writer.Close(); err != nil {
			last = err
		}
		delete(p.writers, key)
	}
	return last
}

// Quantas particoes cada topico tem. E o que permite ao relatorio dizer que
// mandar tudo para uma particao so foi defeito de chave, e nao um topico de
// uma particao.
func (p *Protocol) Available() map[string]int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	available := make(map[string]int64, len(p.partitions))
	for topic, howMany := range p.partitions {
		available["kafka.particao."+topic] = howMany
	}
	return available
}

func (p *Protocol) Decode(no *yaml.Node) (protocol.Config, error) {
	if no == nil || no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo kafka precisa ser um mapa, por exemplo:
  - kafka:
      topico: pedidos
      chave: "${assinantes.id}"
      valor: { id: "${assinantes.id}", total: 199.90 }`)
	}

	config := Default()
	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
		switch key.Value {
		case "topico":
			config.Topic = value.Value
		case "chave":
			config.Key = value.Value
		case "valor":
			body, err := readValue(value)
			if err != nil {
				return nil, err
			}
			config.Value = body
		case "cabecalhos":
			if value.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				config.Headers[value.Content[i].Value] = value.Content[i+1].Value
			}
		case "brokers":
			if value.Kind == yaml.ScalarNode {
				config.Brokers = strings.Split(value.Value, ",")
				break
			}
			for _, item := range value.Content {
				config.Brokers = append(config.Brokers, item.Value)
			}
		case "acks":
			switch value.Value {
			case "todos", "lider", "nenhum":
				config.Acks = value.Value
			default:
				return nil, fmt.Errorf("acks desconhecido: %q (use todos, lider ou nenhum)", value.Value)
			}
		case "timeout":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 5s, 30s)", value.Value)
			}
			config.Timeout = duration
		default:
			return nil, fmt.Errorf("chave desconhecida no passo kafka: %q (use topico, chave, valor, cabecalhos, brokers, acks ou timeout)", key.Value)
		}
	}

	if err := Validate(config); err != nil {
		return nil, err
	}
	return config, nil
}

// Padrao e Validar sao o caminho unico de construcao: a DSL em Go recusa o
// mesmo cenario que o YAML recusa, com a mesma mensagem.
func Default() *Config {
	return &Config{Headers: map[string]string{}, Acks: "todos"}
}

func Validate(config *Config) error {
	if config.Topic == "" {
		return errors.New(`passo kafka sem topico, por exemplo:
  - kafka: { topico: pedidos, valor: { id: "${assinantes.id}" } }`)
	}
	if len(config.Value) == 0 {
		return errors.New(`passo kafka sem valor: uma mensagem vazia nao exercita o consumidor.
  - kafka: { topico: pedidos, valor: { id: "${assinantes.id}" } }`)
	}
	return nil
}

func readValue(no *yaml.Node) ([]byte, error) {
	if no.Kind == yaml.ScalarNode {
		return []byte(no.Value), nil
	}
	var structure any
	if err := no.Decode(&structure); err != nil {
		return nil, fmt.Errorf("valor invalido: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, fmt.Errorf("valor nao serializa para JSON: %v", err)
	}
	return body, nil
}

func (p *Protocol) Execute(ctx context.Context, request protocol.Request) protocol.Response {
	config, ok := request.Config.(*Config)
	if !ok {
		return protocol.Response{Class: protocol.ErrConfig, Detail: "configuracao nao e de kafka"}
	}

	brokers := config.Brokers
	if len(brokers) == 0 {
		brokers = targetBrokers(request.URLBase)
	}
	if len(brokers) == 0 {
		return protocol.Response{
			Class:  protocol.ErrConfig,
			Detail: "sem broker: declare 'brokers' no passo ou aponte o alvo do cenario para kafka://host:9092",
		}
	}

	writer, err := p.writerOf(brokers, config)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	message := kafka.Message{Value: config.Value}
	if config.Key != "" {
		message.Key = []byte(config.Key)
	}
	for name, value := range config.Headers {
		message.Headers = append(message.Headers, kafka.Header{Key: name, Value: []byte(value)})
	}

	if err := writer.WriteMessages(ctx, message); err != nil {
		return protocol.Response{Class: classificar(err), Detail: summarize(err.Error())}
	}

	partition := p.partitionOf(brokers, config.Topic, message.Key)
	response := protocol.Response{
		Bytes: int64(len(config.Value) + len(message.Key)),
		Class: protocol.Success,
	}
	if partition >= 0 {
		response.Attributes = map[string]string{
			"kafka.particao." + config.Topic: strconv.Itoa(partition),
		}
	}
	return response
}

func (p *Protocol) writerOf(brokers []string, config *Config) (*kafka.Writer, error) {
	key := strings.Join(brokers, ",") + "|" + config.Topic + "|" + config.Acks

	p.mu.Lock()
	defer p.mu.Unlock()
	if writer, exists := p.writers[key]; exists {
		return writer, nil
	}

	// Sem lote e sem espera: o braunrate mede o tempo ate o broker confirmar a
	// mensagem daquela chegada agendada. Agrupar mensagens melhoraria a vazao e
	// mediria o lote, nao a mensagem.
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  config.Topic,
		Balancer:               &kafka.Hash{},
		BatchSize:              1,
		BatchTimeout:           time.Millisecond,
		RequiredAcks:           acksOf(config.Acks),
		AllowAutoTopicCreation: true,
		Async:                  false,
	}
	p.writers[key] = writer

	if _, measured := p.partitions[config.Topic]; !measured {
		if howMany := countPartitions(brokers, config.Topic); howMany > 0 {
			p.partitions[config.Topic] = int64(howMany)
		}
	}
	return writer, nil
}

func acksOf(acks string) kafka.RequiredAcks {
	switch acks {
	case "nenhum":
		return kafka.RequireNone
	case "lider":
		return kafka.RequireOne
	default:
		return kafka.RequireAll
	}
}

// A particao e calculada com o mesmo balanceador usado no envio: o kafka-go nao
// devolve a particao escolhida, e a alternativa seria nao declarar nada sobre
// distribuicao — que e justamente onde a carga fica otimista sem ninguem ver.
func (p *Protocol) partitionOf(brokers []string, topic string, key []byte) int {
	p.mu.Lock()
	howMany, known := p.partitions[topic]
	p.mu.Unlock()
	if !known || howMany <= 0 {
		return -1
	}
	if len(key) == 0 {
		return -1
	}
	balancer := &kafka.Hash{}
	list := make([]int, 0, howMany)
	for index := 0; index < int(howMany); index++ {
		list = append(list, index)
	}
	return balancer.Balance(kafka.Message{Key: key}, list...)
}

func countPartitions(brokers []string, topic string) int {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return 0
	}
	defer conn.Close()
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0
	}
	return len(partitions)
}

func targetBrokers(target string) []string {
	if target == "" {
		return nil
	}
	address := strings.TrimPrefix(strings.TrimPrefix(target, "kafka://"), "tcp://")
	address = strings.TrimSuffix(address, "/")
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return nil
	}
	if address == "" {
		return nil
	}
	return strings.Split(address, ",")
}

func classificar(err error) protocol.ErrorClass {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocol.ErrTimeout
	}
	text := err.Error()
	if strings.Contains(text, "timeout") || strings.Contains(text, "deadline") {
		return protocol.ErrTimeout
	}
	if strings.Contains(text, "connection") || strings.Contains(text, "dial") || strings.Contains(text, "EOF") {
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
