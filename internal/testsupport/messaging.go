package testsupport

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

type ProcessorOptions struct {
	Brokers []string
	Input   string
	Output  string
	Delay   time.Duration
	Freeze  time.Duration
}

// Processor is a minimal async service: it consumes from the input topic,
// takes the declared delay and publishes to the output topic with the same key.
// It exists so end-to-end chain measurement can be reproduced without a real
// service.
type Processor struct {
	opts      ProcessorOptions
	readers   []*kafka.Reader
	writer    *kafka.Writer
	processed atomic.Int64
	cancel    context.CancelFunc
	finished  chan struct{}
}

func NewProcessor(opts ProcessorOptions) *Processor {
	return &Processor{opts: opts, finished: make(chan struct{})}
}

func (p *Processor) Processed() int64 { return p.processed.Load() }

// Start reads each partition's offset before running, with no consumer group:
// a group negotiates partitions on the first poll and loses whatever was
// produced during that negotiation, which would show in the report as service
// slowness.
func createTopics(conn *kafka.Conn, topics ...string) error {
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("nao consegui achar o controlador do broker: %w", err)
	}
	leader, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("nao consegui falar com o controlador do broker: %w", err)
	}
	defer func() { _ = leader.Close() }()

	settings := make([]kafka.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		settings = append(settings, kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	}
	if err := leader.CreateTopics(settings...); err != nil {
		return fmt.Errorf("nao consegui criar os topicos %v: %w", topics, err)
	}
	for _, topic := range topics {
		if err := waitForLeader(conn, topic); err != nil {
			return err
		}
	}
	return nil
}

const topicReadyWait = 10 * time.Second

// CreateTopics returns before the partition is servable: the broker publishes
// the leader in the metadata before answering as one, so what retries here is
// the read itself. Bounded because a broker that does not settle in ten seconds
// is broken, and hiding that would turn it into a slow one.
func untilReady(what string, attempt func() error) error {
	return untilReadyWithin(topicReadyWait, 200*time.Millisecond, what, attempt)
}

func untilReadyWithin(wait, interval time.Duration, what string, attempt func() error) error {
	deadline := time.Now().Add(wait)
	for {
		last := attempt()
		if last == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%s nao ficou pronto em %s: %w", what, wait, last)
		}
		time.Sleep(interval)
	}
}

func waitForLeader(conn *kafka.Conn, topic string) error {
	return untilReady(fmt.Sprintf("o topico %q", topic), func() error {
		partitions, err := conn.ReadPartitions(topic)
		if err != nil {
			return err
		}
		if len(partitions) == 0 {
			return fmt.Errorf("nenhuma particao")
		}
		for _, partition := range partitions {
			if partition.Leader.Host == "" {
				return fmt.Errorf("particao %d sem lider eleito", partition.ID)
			}
		}
		return nil
	})
}

func (p *Processor) Start() error {
	conn, err := kafka.Dial("tcp", p.opts.Brokers[0])
	if err != nil {
		return fmt.Errorf("nao consegui falar com o broker: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Reading partitions of a topic that does not exist fails even with
	// auto-creation on, because auto-creation happens on the first write. The
	// test target creates both topics up front so the run does not depend on
	// whoever got there first.
	if err := createTopics(conn, p.opts.Input, p.opts.Output); err != nil {
		return err
	}

	partitions, err := conn.ReadPartitions(p.opts.Input)
	if err != nil {
		return fmt.Errorf("nao consegui ler as particoes de %q: %w", p.opts.Input, err)
	}

	p.writer = &kafka.Writer{
		Addr:                   kafka.TCP(p.opts.Brokers...),
		Topic:                  p.opts.Output,
		Balancer:               &kafka.Hash{},
		BatchSize:              1,
		BatchTimeout:           time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
		AllowAutoTopicCreation: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	var working sync.WaitGroup
	for _, partition := range partitions {
		offset, err := lastOffset(p.opts.Brokers[0], p.opts.Input, partition.ID)
		if err != nil {
			cancel()
			return err
		}

		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   p.opts.Brokers,
			Topic:     p.opts.Input,
			Partition: partition.ID,
			MinBytes:  1,
			MaxBytes:  10 << 20,
			MaxWait:   20 * time.Millisecond,
		})
		if err := reader.SetOffset(offset); err != nil {
			cancel()
			_ = reader.Close()
			return fmt.Errorf("nao consegui posicionar a leitura da particao %d: %w", partition.ID, err)
		}
		p.readers = append(p.readers, reader)

		working.Add(1)
		go func() {
			defer working.Done()
			for {
				message, err := reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					continue
				}
				if p.opts.Delay > 0 {
					time.Sleep(p.opts.Delay)
				}
				if err := p.writer.WriteMessages(ctx, kafka.Message{
					Key:   message.Key,
					Value: message.Value,
				}); err != nil && ctx.Err() != nil {
					return
				}
				p.processed.Add(1)
			}
		}()
	}

	go func() {
		working.Wait()
		close(p.finished)
	}()
	return nil
}

func lastOffset(broker, topic string, partition int) (int64, error) {
	var offset int64
	err := untilReady(fmt.Sprintf("a particao %d de %q", partition, topic), func() error {
		leader, err := kafka.DialLeader(context.Background(), "tcp", broker, topic, partition)
		if err != nil {
			return fmt.Errorf("nao consegui falar com o lider: %w", err)
		}
		defer func() { _ = leader.Close() }()

		offset, err = leader.ReadLastOffset()
		if err != nil {
			return fmt.Errorf("nao consegui ler o offset: %w", err)
		}
		return nil
	})
	return offset, err
}

func (p *Processor) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	for _, reader := range p.readers {
		_ = reader.Close()
	}
	if p.writer != nil {
		_ = p.writer.Close()
	}
	select {
	case <-p.finished:
	case <-time.After(2 * time.Second):
	}
	return nil
}
