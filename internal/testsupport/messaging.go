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
	options   ProcessorOptions
	readers   []*kafka.Reader
	writer    *kafka.Writer
	processed atomic.Int64
	cancel    context.CancelFunc
	finished  chan struct{}
}

func NewProcessor(options ProcessorOptions) *Processor {
	return &Processor{options: options, finished: make(chan struct{})}
}

func (processor *Processor) Processed() int64 { return processor.processed.Load() }

// Start reads each partition's offset before running, with no consumer group:
// a group negotiates partitions on the first poll and loses whatever was
// produced during that negotiation, which would show in the report as service
// slowness.
func createTopics(conn *kafka.Conn, topics ...string) error {
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("could not find the broker controller: %w", err)
	}
	leader, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("could not talk to the broker controller: %w", err)
	}
	defer func() { _ = leader.Close() }()

	settings := make([]kafka.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		settings = append(settings, kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	}
	if err := leader.CreateTopics(settings...); err != nil {
		return fmt.Errorf("could not create the topics %v: %w", topics, err)
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
			return fmt.Errorf("%s was not ready within %s: %w", what, wait, last)
		}
		time.Sleep(interval)
	}
}

func waitForLeader(conn *kafka.Conn, topic string) error {
	return untilReady(fmt.Sprintf("the topic %q", topic), func() error {
		partitions, err := conn.ReadPartitions(topic)
		if err != nil {
			return err
		}
		if len(partitions) == 0 {
			return fmt.Errorf("no partition")
		}
		for _, partition := range partitions {
			if partition.Leader.Host == "" {
				return fmt.Errorf("partition %d has no elected leader", partition.ID)
			}
		}
		return nil
	})
}

func (processor *Processor) Start() error {
	conn, err := kafka.Dial("tcp", processor.options.Brokers[0])
	if err != nil {
		return fmt.Errorf("could not talk to the broker: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Reading partitions of a topic that does not exist fails even with
	// auto-creation on, because auto-creation happens on the first write. The
	// test target creates both topics up front so the run does not depend on
	// whoever got there first.
	if err := createTopics(conn, processor.options.Input, processor.options.Output); err != nil {
		return err
	}

	partitions, err := conn.ReadPartitions(processor.options.Input)
	if err != nil {
		return fmt.Errorf("could not read the partitions of %q: %w", processor.options.Input, err)
	}

	processor.writer = &kafka.Writer{
		Addr:                   kafka.TCP(processor.options.Brokers...),
		Topic:                  processor.options.Output,
		Balancer:               &kafka.Hash{},
		BatchSize:              1,
		BatchTimeout:           time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
		AllowAutoTopicCreation: true,
	}

	runContext, cancel := context.WithCancel(context.Background())
	processor.cancel = cancel

	var working sync.WaitGroup
	for _, partition := range partitions {
		offset, err := lastOffset(processor.options.Brokers[0], processor.options.Input, partition.ID)
		if err != nil {
			cancel()
			return err
		}

		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   processor.options.Brokers,
			Topic:     processor.options.Input,
			Partition: partition.ID,
			MinBytes:  1,
			MaxBytes:  10 << 20,
			MaxWait:   20 * time.Millisecond,
		})
		if err := reader.SetOffset(offset); err != nil {
			cancel()
			_ = reader.Close()
			return fmt.Errorf("could not position the read on partition %d: %w", partition.ID, err)
		}
		processor.readers = append(processor.readers, reader)

		working.Add(1)
		go func() {
			defer working.Done()
			for {
				message, err := reader.ReadMessage(runContext)
				if err != nil {
					if runContext.Err() != nil {
						return
					}
					continue
				}
				if processor.options.Delay > 0 {
					time.Sleep(processor.options.Delay)
				}
				if err := processor.writer.WriteMessages(runContext, kafka.Message{
					Key:   message.Key,
					Value: message.Value,
				}); err != nil && runContext.Err() != nil {
					return
				}
				processor.processed.Add(1)
			}
		}()
	}

	go func() {
		working.Wait()
		close(processor.finished)
	}()
	return nil
}

func lastOffset(broker, topic string, partition int) (int64, error) {
	var offset int64
	err := untilReady(fmt.Sprintf("partition %d of %q", partition, topic), func() error {
		leader, err := kafka.DialLeader(context.Background(), "tcp", broker, topic, partition)
		if err != nil {
			return fmt.Errorf("could not talk to the leader: %w", err)
		}
		defer func() { _ = leader.Close() }()

		offset, err = leader.ReadLastOffset()
		if err != nil {
			return fmt.Errorf("could not read the offset: %w", err)
		}
		return nil
	})
	return offset, err
}

func (processor *Processor) Close() error {
	if processor.cancel != nil {
		processor.cancel()
	}
	for _, reader := range processor.readers {
		_ = reader.Close()
	}
	if processor.writer != nil {
		_ = processor.writer.Close()
	}
	select {
	case <-processor.finished:
	case <-time.After(2 * time.Second):
	}
	return nil
}
