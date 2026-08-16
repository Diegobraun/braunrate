package wait

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/segmentio/kafka-go"
	"github.com/tidwall/gjson"
)

// Cap on messages held for waiters that have not arrived yet. Without holding
// them, a fast response loses the race against the wait registration and turns
// into a timeout, blaming slowness that does not exist.
const heldMessagesCap = 50000

type message struct {
	body       []byte
	attributes map[string]string
	chegadaEm  time.Time
}

type subscription struct {
	field string

	mu       sync.Mutex
	arrivals map[string]message
	order    []string
	waiting  map[string]chan message

	cancel    context.CancelFunc
	encerrada sync.Once
	fechar    func()
}

func newSubscription(field string) *subscription {
	return &subscription{
		field:    field,
		arrivals: map[string]message{},
		waiting:  map[string]chan message{},
	}
}

func (subscription *subscription) deliver(correlation string, received message) {
	if correlation == "" {
		return
	}
	subscription.mu.Lock()
	canal, waiting := subscription.waiting[correlation]
	if waiting {
		delete(subscription.waiting, correlation)
		subscription.mu.Unlock()
		canal <- received
		return
	}

	subscription.arrivals[correlation] = received
	subscription.order = append(subscription.order, correlation)
	if len(subscription.order) > heldMessagesCap {
		maisAntiga := subscription.order[0]
		subscription.order = subscription.order[1:]
		delete(subscription.arrivals, maisAntiga)
	}
	subscription.mu.Unlock()
}

func (subscription *subscription) await(runContext context.Context, correlation string, timeout time.Duration) (message, bool) {
	subscription.mu.Lock()
	if received, arrived := subscription.arrivals[correlation]; arrived {
		delete(subscription.arrivals, correlation)
		subscription.mu.Unlock()
		return received, true
	}
	canal := make(chan message, 1)
	subscription.waiting[correlation] = canal
	subscription.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case received := <-canal:
		return received, true
	case <-timer.C:
	case <-runContext.Done():
	}

	subscription.mu.Lock()
	delete(subscription.waiting, correlation)
	subscription.mu.Unlock()
	select {
	case received := <-canal:
		return received, true
	default:
	}
	return message{}, false
}

func (subscription *subscription) shutdown() {
	subscription.encerrada.Do(func() {
		if subscription.cancel != nil {
			subscription.cancel()
		}
		if subscription.fechar != nil {
			subscription.fechar()
		}
	})
}

func (subscription *subscription) correlationOf(key, value []byte) string {
	if subscription.field == "" {
		return string(key)
	}
	path := strings.TrimPrefix(strings.TrimPrefix(subscription.field, "$."), "$")
	return gjson.GetBytes(value, path).String()
}

func openSubscription(config *Config, addresses []string, broker *messaging.Broker) (*subscription, error) {
	switch config.Source {
	case "kafka":
		return openKafka(config, addresses, broker)
	case "amqp":
		return openAMQP(config, addresses, broker)
	default:
		return nil, fmt.Errorf("fonte desconhecida em aguardar: %q", config.Source)
	}
}

// No consumer group: joining one negotiates partitions with the broker, and a
// message produced during that negotiation is lost, turning into a timeout that
// belongs to the consumer and not to the service. Here each partition's offset
// is read when the subscription opens, before the load starts.
func openKafka(config *Config, brokers []string, broker *messaging.Broker) (*subscription, error) {
	dialer, err := broker.Dialer()
	if err != nil {
		return nil, err
	}
	conn, err := dialWith(dialer, brokers[0])
	if err != nil {
		if kind, credential := messaging.ClassifyError(err); credential {
			return nil, fmt.Errorf("%s", messaging.Explain(kind, broker))
		}
		return nil, fmt.Errorf("nao consegui falar com o broker %s (%s): %v", brokers[0], broker.Describe(), err)
	}
	defer func() { _ = conn.Close() }()

	partitions, err := conn.ReadPartitions(config.Topic)
	if err != nil {
		return nil, fmt.Errorf("nao consegui ler as particoes de %q: %v", config.Topic, err)
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf("o topico %q nao existe no broker", config.Topic)
	}

	runContext, cancel := context.WithCancel(context.Background())
	subscription := newSubscription(config.Field)
	subscription.cancel = cancel

	readers := make([]*kafka.Reader, 0, len(partitions))
	for _, partition := range partitions {
		offset, err := lastOffset(brokers[0], config.Topic, partition.ID, dialer)
		if err != nil {
			cancel()
			for _, open := range readers {
				_ = open.Close()
			}
			return nil, err
		}

		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   brokers,
			Topic:     config.Topic,
			Partition: partition.ID,
			MinBytes:  1,
			MaxBytes:  10 << 20,
			MaxWait:   20 * time.Millisecond,
			Dialer:    dialer,
		})
		if err := reader.SetOffset(offset); err != nil {
			cancel()
			_ = reader.Close()
			for _, open := range readers {
				_ = open.Close()
			}
			return nil, fmt.Errorf("nao consegui posicionar a leitura da particao %d: %v", partition.ID, err)
		}
		readers = append(readers, reader)

		number := partition.ID
		go func() {
			for {
				record, err := reader.ReadMessage(runContext)
				if err != nil {
					if runContext.Err() != nil {
						return
					}
					continue
				}
				subscription.deliver(subscription.correlationOf(record.Key, record.Value), message{
					body:      record.Value,
					chegadaEm: time.Now(),
					attributes: map[string]string{
						"kafka.particao.consumida." + config.Topic: strconv.Itoa(number),
					},
				})
			}
		}()
	}

	subscription.fechar = func() {
		for _, reader := range readers {
			_ = reader.Close()
		}
	}
	return subscription, nil
}

// The connection has to be to that partition's leader: a generic broker
// connection does not know which partition the requested offset belongs to.
func dialWith(dialer *kafka.Dialer, address string) (*kafka.Conn, error) {
	if dialer == nil {
		return kafka.Dial("tcp", address)
	}
	return dialer.Dial("tcp", address)
}

func lastOffset(address, topic string, partition int, dialer *kafka.Dialer) (int64, error) {
	leader, err := dialLeaderWith(dialer, address, topic, partition)
	if err != nil {
		return 0, fmt.Errorf("nao consegui falar com o lider da particao %d de %q: %v", partition, topic, err)
	}
	defer func() { _ = leader.Close() }()

	offset, err := leader.ReadLastOffset()
	if err != nil {
		return 0, fmt.Errorf("nao consegui ler o offset da particao %d: %v", partition, err)
	}
	return offset, nil
}

func dialLeaderWith(dialer *kafka.Dialer, address, topic string, partition int) (*kafka.Conn, error) {
	if dialer == nil {
		return kafka.DialLeader(context.Background(), "tcp", address, topic, partition)
	}
	return dialer.DialLeader(context.Background(), "tcp", address, topic, partition)
}

func openAMQP(config *Config, addresses []string, broker *messaging.Broker) (*subscription, error) {
	conn, err := broker.DialAMQP(normalizeAMQP(addresses[0]))
	if err != nil {
		if kind, credential := messaging.ClassifyError(err); credential {
			return nil, fmt.Errorf("%s", messaging.Explain(kind, broker))
		}
		return nil, fmt.Errorf("nao consegui conectar em %s: %v", messaging.SafeAddress(addresses[0]), err)
	}
	canal, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("nao consegui abrir o canal AMQP: %v", err)
	}
	if _, err := canal.QueueDeclare(config.Topic, true, false, false, false, nil); err != nil {
		_ = canal.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("nao consegui declarar a fila %q: %v", config.Topic, err)
	}
	deliveries, err := canal.Consume(config.Topic, "", true, false, false, false, nil)
	if err != nil {
		_ = canal.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("nao consegui consumir a fila %q: %v", config.Topic, err)
	}

	subscription := newSubscription(config.Field)
	subscription.fechar = func() {
		_ = canal.Close()
		_ = conn.Close()
	}

	go func() {
		for delivery := range deliveries {
			key := []byte(delivery.MessageId)
			if len(key) == 0 {
				key = []byte(delivery.CorrelationId)
			}
			subscription.deliver(subscription.correlationOf(key, delivery.Body), message{
				body:       delivery.Body,
				chegadaEm:  time.Now(),
				attributes: nil,
			})
		}
	}()
	return subscription, nil
}

func normalizeAMQP(address string) string {
	if strings.HasPrefix(address, "amqp://") || strings.HasPrefix(address, "amqps://") {
		return address
	}
	return "amqp://" + address
}
