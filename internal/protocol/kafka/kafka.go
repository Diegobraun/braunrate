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

func (config *Config) Protocol() string { return "kafka" }

// AggregationKey is the topic, never the broker: whoever reads the report
// needs to know which business flow got slow, not which machine took the byte.
func (config *Config) AggregationKey() string {
	return "kafka produzir " + config.Topic
}

func (config *Config) Resolve(resolve func(string) string) protocol.Config {
	clone := *config
	clone.Topic = resolve(config.Topic)
	clone.Key = resolve(config.Key)
	if len(config.Value) > 0 {
		clone.Value = []byte(resolve(string(config.Value)))
	}
	clone.Headers = make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		clone.Headers[name] = resolve(value)
	}
	return &clone
}

func (config *Config) Describe() []string {
	lines := []string{fmt.Sprintf("produzir em %s (chave %q)", config.Topic, config.Key)}
	names := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("cabecalho %s: %s", name, config.Headers[name]))
	}
	if len(config.Brokers) > 0 {
		lines = append(lines, "brokers: "+strings.Join(config.Brokers, ", "))
	}
	lines = append(lines, "acks: "+config.Acks)
	if len(config.Value) > 0 {
		lines = append(lines, "valor: "+string(config.Value))
	}
	return lines
}

type Protocol struct {
	options    protocol.Options
	mu         sync.Mutex
	writers    map[string]*kafka.Writer
	partitions map[string]int64
}

func New(options protocol.Options) *Protocol {
	return &Protocol{options: options, writers: map[string]*kafka.Writer{}, partitions: map[string]int64{}}
}

func (implementation *Protocol) Name() string { return "kafka" }

func (implementation *Protocol) Close() error {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	var last error
	for key, writer := range implementation.writers {
		if err := writer.Close(); err != nil {
			last = err
		}
		delete(implementation.writers, key)
	}
	return last
}

// Available reports how many partitions each topic has. It is what lets the
// report tell a bad partition key from a topic that only has one partition.
func (implementation *Protocol) Available() map[string]int64 {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	available := make(map[string]int64, len(implementation.partitions))
	for topic, howMany := range implementation.partitions {
		available["kafka.particao."+topic] = howMany
	}
	return available
}

func (implementation *Protocol) Decode(node *yaml.Node) (protocol.Config, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, errors.New(`passo kafka precisa ser um mapa, por exemplo:
  - kafka:
      topico: pedidos
      chave: "${assinantes.id}"
      valor: { id: "${assinantes.id}", total: 199.90 }`)
	}

	config := Default()
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
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

// Default and Validate are the single construction path: the Go DSL refuses
// the same scenario the YAML refuses, with the same message.
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

func readValue(node *yaml.Node) ([]byte, error) {
	if node.Kind == yaml.ScalarNode {
		return []byte(node.Value), nil
	}
	var structure any
	if err := node.Decode(&structure); err != nil {
		return nil, fmt.Errorf("valor invalido: %v", err)
	}
	body, err := json.Marshal(structure)
	if err != nil {
		return nil, fmt.Errorf("valor nao serializa para JSON: %v", err)
	}
	return body, nil
}

func (implementation *Protocol) Execute(runContext context.Context, request protocol.Request) protocol.Response {
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

	writer, err := implementation.writerOf(brokers, config)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(runContext, config.Timeout)
		defer cancel()
	}

	message := kafka.Message{Value: config.Value}
	if config.Key != "" {
		message.Key = []byte(config.Key)
	}
	for name, value := range config.Headers {
		message.Headers = append(message.Headers, kafka.Header{Key: name, Value: []byte(value)})
	}

	if err := writer.WriteMessages(runContext, message); err != nil {
		return protocol.Response{Class: classificar(err), Detail: summarize(err.Error())}
	}

	partition := implementation.partitionOf(brokers, config.Topic, message.Key)
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

func (implementation *Protocol) writerOf(brokers []string, config *Config) (*kafka.Writer, error) {
	key := strings.Join(brokers, ",") + "|" + config.Topic + "|" + config.Acks

	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if writer, exists := implementation.writers[key]; exists {
		return writer, nil
	}

	// No batching and no linger: braunrate measures the time until the broker
	// confirms the message of that scheduled arrival. Batching would raise
	// throughput and measure the batch, not the message.
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
	implementation.writers[key] = writer

	if _, measured := implementation.partitions[config.Topic]; !measured {
		if howMany := countPartitions(brokers, config.Topic); howMany > 0 {
			implementation.partitions[config.Topic] = int64(howMany)
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

// The partition is recomputed with the same balancer used to send: kafka-go
// does not return the chosen partition, and the alternative would be declaring
// nothing about distribution, which is exactly where load turns optimistic
// unseen.
func (implementation *Protocol) partitionOf(brokers []string, topic string, key []byte) int {
	implementation.mu.Lock()
	howMany, known := implementation.partitions[topic]
	implementation.mu.Unlock()
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
	defer func() { _ = conn.Close() }()
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
