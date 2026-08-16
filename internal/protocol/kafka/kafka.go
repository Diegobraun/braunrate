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

	"github.com/Diegobraun/braunrate/internal/messaging"
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
	// Declared partition, when the scenario wants to aim at one instead of
	// letting the key decide. Nil means the key decides, which is the default
	// and what represents production.
	Partition *int
	// Consumer group to watch while the load runs. The lag of that group is
	// what says whether the service kept up; the time to produce does not.
	Group string
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
	clone.Group = resolve(config.Group)
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
	if config.Partition != nil {
		lines = append(lines, "particao declarada: "+strconv.Itoa(*config.Partition))
	}
	if config.Group != "" {
		lines = append(lines, "observando o atraso do grupo "+config.Group)
	}
	lines = append(lines, "acks: "+config.Acks)
	if len(config.Value) > 0 {
		lines = append(lines, "valor: "+string(config.Value))
	}
	return lines
}

type Protocol struct {
	options      protocol.Options
	mu           sync.Mutex
	writers      map[string]*kafka.Writer
	partitions   map[string]int64
	declared     map[string]bool
	watchers     map[string]*lagWatcher
	stopWatch    context.CancelFunc
	watchContext context.Context
}

func New(options protocol.Options) *Protocol {
	return &Protocol{
		options: options, writers: map[string]*kafka.Writer{},
		partitions: map[string]int64{}, declared: map[string]bool{},
		watchers: map[string]*lagWatcher{},
	}
}

func (implementation *Protocol) Name() string { return "kafka" }

// ConsumerLag reports what each watched group was left behind by. The number
// that matters is the one after the load stops, so the watch is closed here and
// waited on: reading it while the sampler still runs would report a lag from the
// middle of the run as if it were the final one.
func (implementation *Protocol) ConsumerLag() []protocol.ConsumerLag {
	implementation.mu.Lock()
	if implementation.stopWatch != nil {
		implementation.stopWatch()
		implementation.stopWatch = nil
	}
	watchers := make([]*lagWatcher, 0, len(implementation.watchers))
	for _, watcher := range implementation.watchers {
		watchers = append(watchers, watcher)
	}
	implementation.mu.Unlock()

	for _, watcher := range watchers {
		<-watcher.done
	}

	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	var lags []protocol.ConsumerLag
	for _, watcher := range implementation.watchers {
		lags = append(lags, watcher.result())
	}
	sort.Slice(lags, func(first, second int) bool { return lags[first].Group < lags[second].Group })
	return lags
}

func (implementation *Protocol) Close() error {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if implementation.stopWatch != nil {
		implementation.stopWatch()
		implementation.stopWatch = nil
	}
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
		available[implementation.name(topic)] = howMany
	}
	return available
}

// A topic written to on a declared partition is counted under another name, so
// the report can say the concentration was asked for instead of accusing the
// scenario of a key that does not vary.
func (implementation *Protocol) declare(topic string) {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if implementation.declared == nil {
		implementation.declared = map[string]bool{}
	}
	implementation.declared[topic] = true
}

func (implementation *Protocol) varietyName(topic string) string {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	return implementation.name(topic)
}

func (implementation *Protocol) name(topic string) string {
	if implementation.declared[topic] {
		return "kafka.particao.declarada." + topic
	}
	return "kafka.particao." + topic
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
		case "particao":
			number, err := strconv.Atoi(value.Value)
			if err != nil || number < 0 {
				return nil, fmt.Errorf("particao invalida: %q (use um numero, como 0 ou 3)", value.Value)
			}
			config.Partition = &number
		case "grupo":
			config.Group = value.Value
		default:
			return nil, fmt.Errorf("chave desconhecida no passo kafka: %q (use topico, chave, valor, cabecalhos, brokers, acks, timeout, particao ou grupo)", key.Value)
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

	broker := request.Messaging.BrokerFor("kafka")

	brokers := config.Brokers
	if len(brokers) == 0 && broker != nil {
		brokers = broker.Addresses
	}
	if len(brokers) == 0 {
		brokers = targetBrokers(request.URLBase)
	}
	if len(brokers) == 0 {
		return protocol.Response{
			Class:  protocol.ErrConfig,
			Detail: "sem broker: declare 'brokers' no passo ou aponte o alvo do cenario para kafka://host:9092",
		}
	}

	writer, err := implementation.writerOf(brokers, config, broker)
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
		return protocol.Response{Class: classificar(err), Detail: detailOf(err, broker)}
	}

	partition := implementation.partitionOf(brokers, config.Topic, message.Key)
	if config.Partition != nil {
		partition = *config.Partition
		implementation.declare(config.Topic)
	}
	response := protocol.Response{
		Bytes: int64(len(config.Value) + len(message.Key)),
		Class: protocol.Success,
	}
	if partition >= 0 {
		response.Attributes = map[string]string{
			implementation.varietyName(config.Topic): strconv.Itoa(partition),
		}
	}
	return response
}

// A declared partition needs a balancer that answers it: kafka-go decides the
// partition through the balancer, and Message.Partition is ignored on the way
// out.
type fixedPartition struct{ number int }

func (fixed fixedPartition) Balance(_ kafka.Message, _ ...int) int { return fixed.number }

func balancerFor(config *Config) kafka.Balancer {
	if config.Partition != nil {
		return fixedPartition{number: *config.Partition}
	}
	return &kafka.Hash{}
}

func partitionKey(config *Config) string {
	if config.Partition == nil {
		return "chave"
	}
	return "particao " + strconv.Itoa(*config.Partition)
}

func (implementation *Protocol) writerOf(brokers []string, config *Config, broker *messaging.Broker) (*kafka.Writer, error) {
	key := strings.Join(brokers, ",") + "|" + config.Topic + "|" + config.Acks + "|" + broker.Describe() + "|" + partitionKey(config)

	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if writer, exists := implementation.writers[key]; exists {
		return writer, nil
	}

	transport, err := broker.Transport()
	if err != nil {
		return nil, err
	}

	// No batching and no linger: braunrate measures the time until the broker
	// confirms the message of that scheduled arrival. Batching would raise
	// throughput and measure the batch, not the message.
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  config.Topic,
		Balancer:               balancerFor(config),
		BatchSize:              1,
		BatchTimeout:           time.Millisecond,
		RequiredAcks:           acksOf(config.Acks),
		AllowAutoTopicCreation: true,
		Async:                  false,
	}
	if transport != nil {
		writer.Transport = transport
	}
	implementation.writers[key] = writer

	if _, measured := implementation.partitions[config.Topic]; !measured {
		if howMany := countPartitions(brokers, config.Topic, broker); howMany > 0 {
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

func countPartitions(brokers []string, topic string, broker *messaging.Broker) int {
	conn, err := dial(brokers[0], broker)
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

// The SASL and TLS handshake happens when the connection opens. Opening it here,
// before the load, keeps the handshake out of the latency of the first message —
// the same reason the consumer of the wait step subscribes during preparation.
func (implementation *Protocol) Prepare(runContext context.Context, request protocol.Request) error {
	config, isKafka := request.Config.(*Config)
	if !isKafka {
		return nil
	}
	broker := request.Messaging.BrokerFor("kafka")
	brokers := config.Brokers
	if len(brokers) == 0 && broker != nil {
		brokers = broker.Addresses
	}
	if len(brokers) == 0 {
		brokers = targetBrokers(request.URLBase)
	}
	if len(brokers) == 0 {
		return nil
	}
	// Watching starts before the load so the first reading is the lag the group
	// had at rest: without it, the growth measured would start from an unknown
	// point.
	if err := implementation.watchLag(config, brokers, broker); err != nil {
		return err
	}
	if !broker.Secured() {
		return nil
	}
	if _, err := implementation.writerOf(brokers, config, broker); err != nil {
		return err
	}
	conn, err := dial(brokers[0], broker)
	if err != nil {
		if kind, credential := messaging.ClassifyError(err); credential {
			return fmt.Errorf("%s", messaging.Explain(kind, broker))
		}
		return fmt.Errorf("nao consegui abrir conexao com %s (%s): %w", brokers[0], broker.Describe(), err)
	}
	return conn.Close()
}

func dial(address string, broker *messaging.Broker) (*kafka.Conn, error) {
	dialer, err := broker.Dialer()
	if err != nil {
		return nil, err
	}
	if dialer == nil {
		return kafka.Dial("tcp", address)
	}
	return dialer.Dial("tcp", address)
}

func detailOf(err error, broker *messaging.Broker) string {
	if kind, credential := messaging.ClassifyError(err); credential {
		return messaging.Explain(kind, broker)
	}
	return summarize(err.Error())
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
	if kind, credential := messaging.ClassifyError(err); credential {
		if kind == "autenticacao" {
			return protocol.ErrAuth
		}
		return protocol.ErrAuthorization
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

func (implementation *Protocol) watchLag(config *Config, brokers []string, broker *messaging.Broker) error {
	if config.Group == "" {
		return nil
	}
	key := config.Group + "|" + config.Topic

	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if _, watching := implementation.watchers[key]; watching {
		return nil
	}
	watcher, err := newLagWatcher(config.Group, config.Topic, brokers, broker)
	if err != nil {
		return err
	}
	implementation.watchers[key] = watcher

	if implementation.stopWatch == nil {
		watchContext, stop := context.WithCancel(context.Background())
		implementation.stopWatch = stop
		implementation.watchContext = watchContext
	}
	go watcher.watch(implementation.watchContext)
	return nil
}
