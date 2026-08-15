package alvo

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

type OpcoesDeProcessador struct {
	Brokers  []string
	Entrada  string
	Saida    string
	Atraso   time.Duration
	Congelar time.Duration
}

// Um servico assincrono minimo: consome do topico de entrada, demora o que foi
// declarado e publica no de saida com a mesma chave. Existe para que a medicao
// da cadeia ponta a ponta possa ser reproduzida sem depender de um servico de
// verdade.
type Processador struct {
	opcoes      OpcoesDeProcessador
	leitores    []*kafka.Reader
	escritor    *kafka.Writer
	processadas atomic.Int64
	cancelar    context.CancelFunc
	terminou    chan struct{}
}

func NovoProcessador(opcoes OpcoesDeProcessador) *Processador {
	return &Processador{opcoes: opcoes, terminou: make(chan struct{})}
}

func (p *Processador) Processadas() int64 { return p.processadas.Load() }

// Le o offset de cada particao antes de comecar, sem grupo de consumo: grupo
// negocia particao no primeiro poll e perde o que foi produzido durante a
// negociacao, o que apareceria no relatorio como lentidao do servico.
func (p *Processador) Iniciar() error {
	conexao, err := kafka.Dial("tcp", p.opcoes.Brokers[0])
	if err != nil {
		return fmt.Errorf("nao consegui falar com o broker: %w", err)
	}
	defer conexao.Close()

	particoes, err := conexao.ReadPartitions(p.opcoes.Entrada)
	if err != nil {
		return fmt.Errorf("nao consegui ler as particoes de %q: %w", p.opcoes.Entrada, err)
	}

	p.escritor = &kafka.Writer{
		Addr:                   kafka.TCP(p.opcoes.Brokers...),
		Topic:                  p.opcoes.Saida,
		Balancer:               &kafka.Hash{},
		BatchSize:              1,
		BatchTimeout:           time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
		AllowAutoTopicCreation: true,
	}

	ctx, cancelar := context.WithCancel(context.Background())
	p.cancelar = cancelar

	var trabalhando sync.WaitGroup
	for _, particao := range particoes {
		offset, err := ultimoOffset(p.opcoes.Brokers[0], p.opcoes.Entrada, particao.ID)
		if err != nil {
			cancelar()
			return err
		}

		leitor := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   p.opcoes.Brokers,
			Topic:     p.opcoes.Entrada,
			Partition: particao.ID,
			MinBytes:  1,
			MaxBytes:  10 << 20,
			MaxWait:   20 * time.Millisecond,
		})
		if err := leitor.SetOffset(offset); err != nil {
			cancelar()
			_ = leitor.Close()
			return fmt.Errorf("nao consegui posicionar a leitura da particao %d: %w", particao.ID, err)
		}
		p.leitores = append(p.leitores, leitor)

		trabalhando.Add(1)
		go func() {
			defer trabalhando.Done()
			for {
				mensagem, err := leitor.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					continue
				}
				if p.opcoes.Atraso > 0 {
					time.Sleep(p.opcoes.Atraso)
				}
				if err := p.escritor.WriteMessages(ctx, kafka.Message{
					Key:   mensagem.Key,
					Value: mensagem.Value,
				}); err != nil && ctx.Err() != nil {
					return
				}
				p.processadas.Add(1)
			}
		}()
	}

	go func() {
		trabalhando.Wait()
		close(p.terminou)
	}()
	return nil
}

func ultimoOffset(broker, topico string, particao int) (int64, error) {
	lider, err := kafka.DialLeader(context.Background(), "tcp", broker, topico, particao)
	if err != nil {
		return 0, fmt.Errorf("nao consegui falar com o lider da particao %d de %q: %w", particao, topico, err)
	}
	defer lider.Close()

	offset, err := lider.ReadLastOffset()
	if err != nil {
		return 0, fmt.Errorf("nao consegui ler o offset da particao %d: %w", particao, err)
	}
	return offset, nil
}

func (p *Processador) Encerrar() error {
	if p.cancelar != nil {
		p.cancelar()
	}
	for _, leitor := range p.leitores {
		_ = leitor.Close()
	}
	if p.escritor != nil {
		_ = p.escritor.Close()
	}
	select {
	case <-p.terminou:
	case <-time.After(2 * time.Second):
	}
	return nil
}
