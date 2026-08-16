package wait

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
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

func (a *subscription) deliver(correlation string, received message) {
	if correlation == "" {
		return
	}
	a.mu.Lock()
	canal, waiting := a.waiting[correlation]
	if waiting {
		delete(a.waiting, correlation)
		a.mu.Unlock()
		canal <- received
		return
	}

	a.arrivals[correlation] = received
	a.order = append(a.order, correlation)
	if len(a.order) > heldMessagesCap {
		maisAntiga := a.order[0]
		a.order = a.order[1:]
		delete(a.arrivals, maisAntiga)
	}
	a.mu.Unlock()
}

func (a *subscription) await(ctx context.Context, correlation string, timeout time.Duration) (message, bool) {
	a.mu.Lock()
	if received, arrived := a.arrivals[correlation]; arrived {
		delete(a.arrivals, correlation)
		a.mu.Unlock()
		return received, true
	}
	canal := make(chan message, 1)
	a.waiting[correlation] = canal
	a.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case received := <-canal:
		return received, true
	case <-timer.C:
	case <-ctx.Done():
	}

	a.mu.Lock()
	delete(a.waiting, correlation)
	a.mu.Unlock()
	select {
	case received := <-canal:
		return received, true
	default:
	}
	return message{}, false
}

func (a *subscription) shutdown() {
	a.encerrada.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		if a.fechar != nil {
			a.fechar()
		}
	})
}

func (a *subscription) correlationOf(key, value []byte) string {
	if a.field == "" {
		return string(key)
	}
	path := strings.TrimPrefix(strings.TrimPrefix(a.field, "$."), "$")
	return gjson.GetBytes(value, path).String()
}

func openSubscription(config *Config, addresses []string) (*subscription, error) {
	switch config.Source {
	case "kafka":
		return openKafka(config, addresses)
	case "amqp":
		return openAMQP(config, addresses)
	default:
		return nil, fmt.Errorf("fonte desconhecida em aguardar: %q", config.Source)
	}
}

// No consumer group: joining one negotiates partitions with the broker, and a
// message produced during that negotiation is lost, turning into a timeout that
// belongs to the consumer and not to the service. Here each partition's offset
// is read when the subscription opens, before the load starts.
func openKafka(config *Config, brokers []string) (*subscription, error) {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return nil, fmt.Errorf("nao consegui falar com o broker %s: %v", brokers[0], err)
	}
	defer func() { _ = conn.Close() }()

	partitions, err := conn.ReadPartitions(config.Topic)
	if err != nil {
		return nil, fmt.Errorf("nao consegui ler as particoes de %q: %v", config.Topic, err)
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf("o topico %q nao existe no broker", config.Topic)
	}

	ctx, cancel := context.WithCancel(context.Background())
	subscription := newSubscription(config.Field)
	subscription.cancel = cancel

	readers := make([]*kafka.Reader, 0, len(partitions))
	for _, partition := range partitions {
		offset, err := lastOffset(brokers[0], config.Topic, partition.ID)
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
				record, err := reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
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
func lastOffset(broker, topic string, partition int) (int64, error) {
	leader, err := kafka.DialLeader(context.Background(), "tcp", broker, topic, partition)
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

func openAMQP(config *Config, addresses []string) (*subscription, error) {
	conn, err := amqp.Dial(normalizeAMQP(addresses[0]))
	if err != nil {
		return nil, fmt.Errorf("nao consegui conectar em %s: %v", addresses[0], err)
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
