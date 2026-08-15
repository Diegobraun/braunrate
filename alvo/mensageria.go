package alvo

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

type OpcoesDeProcessador struct {
	Brokers  []string
	Entrada  string
	Saida    string
	Grupo    string
	Atraso   time.Duration
	Congelar time.Duration
}

// Um servico assincrono minimo: consome do topico de entrada, demora o que foi
// declarado e publica no de saida com a mesma chave. Existe para que a medicao
// da cadeia ponta a ponta possa ser reproduzida sem depender de um servico de
// verdade.
type Processador struct {
	opcoes      OpcoesDeProcessador
	leitor      *kafka.Reader
	escritor    *kafka.Writer
	processadas atomic.Int64
	cancelar    context.CancelFunc
	terminou    chan struct{}
}

func NovoProcessador(opcoes OpcoesDeProcessador) *Processador {
	if opcoes.Grupo == "" {
		opcoes.Grupo = "braunrate-processador"
	}
	return &Processador{opcoes: opcoes, terminou: make(chan struct{})}
}

func (p *Processador) Processadas() int64 { return p.processadas.Load() }

func (p *Processador) Iniciar() error {
	p.leitor = kafka.NewReader(kafka.ReaderConfig{
		Brokers:     p.opcoes.Brokers,
		Topic:       p.opcoes.Entrada,
		GroupID:     p.opcoes.Grupo,
		MinBytes:    1,
		MaxBytes:    10 << 20,
		MaxWait:     20 * time.Millisecond,
		StartOffset: kafka.LastOffset,
	})
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

	go func() {
		defer close(p.terminou)
		for {
			mensagem, err := p.leitor.ReadMessage(ctx)
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
	return nil
}

func (p *Processador) Encerrar() error {
	if p.cancelar != nil {
		p.cancelar()
	}
	if p.leitor != nil {
		_ = p.leitor.Close()
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
