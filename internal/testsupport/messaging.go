package testsupport

import (
	"context"
	"fmt"
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

// Um servico assincrono minimo: consome do topico de entrada, demora o que foi
// declarado e publica no de saida com a mesma chave. Existe para que a medicao
// da cadeia ponta a ponta possa ser reproduzida sem depender de um servico de
// verdade.
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

// Le o offset de cada particao antes de comecar, sem grupo de consumo: grupo
// negocia particao no primeiro poll e perde o que foi produzido durante a
// negociacao, o que apareceria no relatorio como lentidao do servico.
func (p *Processor) Start() error {
	conn, err := kafka.Dial("tcp", p.opts.Brokers[0])
	if err != nil {
		return fmt.Errorf("nao consegui falar com o broker: %w", err)
	}
	defer conn.Close()

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
	leader, err := kafka.DialLeader(context.Background(), "tcp", broker, topic, partition)
	if err != nil {
		return 0, fmt.Errorf("nao consegui falar com o lider da particao %d de %q: %w", partition, topic, err)
	}
	defer leader.Close()

	offset, err := leader.ReadLastOffset()
	if err != nil {
		return 0, fmt.Errorf("nao consegui ler o offset da particao %d: %w", partition, err)
	}
	return offset, nil
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
