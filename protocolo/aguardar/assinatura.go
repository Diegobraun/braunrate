package aguardar

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

// Guarda mensagens que chegaram antes de alguem pedir por elas. Sem isso, a
// resposta rapida perde a corrida contra o registro da espera e viraria timeout
// — a medicao acusaria lentidao que nao existe.
const tetoDeMensagensGuardadas = 50000

type mensagem struct {
	corpo     []byte
	atributos map[string]string
	chegadaEm time.Time
}

type assinatura struct {
	campo string

	mutex     sync.Mutex
	chegadas  map[string]mensagem
	ordem     []string
	esperando map[string]chan mensagem

	cancelar  context.CancelFunc
	encerrada sync.Once
	fechar    func()
}

func novaAssinatura(campo string) *assinatura {
	return &assinatura{
		campo:     campo,
		chegadas:  map[string]mensagem{},
		esperando: map[string]chan mensagem{},
	}
}

func (a *assinatura) entregar(correlacao string, recebida mensagem) {
	if correlacao == "" {
		return
	}
	a.mutex.Lock()
	canal, esperando := a.esperando[correlacao]
	if esperando {
		delete(a.esperando, correlacao)
		a.mutex.Unlock()
		canal <- recebida
		return
	}

	a.chegadas[correlacao] = recebida
	a.ordem = append(a.ordem, correlacao)
	if len(a.ordem) > tetoDeMensagensGuardadas {
		maisAntiga := a.ordem[0]
		a.ordem = a.ordem[1:]
		delete(a.chegadas, maisAntiga)
	}
	a.mutex.Unlock()
}

func (a *assinatura) esperar(ctx context.Context, correlacao string, timeout time.Duration) (mensagem, bool) {
	a.mutex.Lock()
	if recebida, chegou := a.chegadas[correlacao]; chegou {
		delete(a.chegadas, correlacao)
		a.mutex.Unlock()
		return recebida, true
	}
	canal := make(chan mensagem, 1)
	a.esperando[correlacao] = canal
	a.mutex.Unlock()

	temporizador := time.NewTimer(timeout)
	defer temporizador.Stop()

	select {
	case recebida := <-canal:
		return recebida, true
	case <-temporizador.C:
	case <-ctx.Done():
	}

	a.mutex.Lock()
	delete(a.esperando, correlacao)
	a.mutex.Unlock()
	select {
	case recebida := <-canal:
		return recebida, true
	default:
	}
	return mensagem{}, false
}

func (a *assinatura) encerrar() {
	a.encerrada.Do(func() {
		if a.cancelar != nil {
			a.cancelar()
		}
		if a.fechar != nil {
			a.fechar()
		}
	})
}

func (a *assinatura) correlacaoDe(chave, valor []byte) string {
	if a.campo == "" {
		return string(chave)
	}
	caminho := strings.TrimPrefix(strings.TrimPrefix(a.campo, "$."), "$")
	return gjson.GetBytes(valor, caminho).String()
}

func abrirAssinatura(configuracao *Configuracao, enderecos []string) (*assinatura, error) {
	switch configuracao.Fonte {
	case "kafka":
		return abrirKafka(configuracao, enderecos)
	case "amqp":
		return abrirAMQP(configuracao, enderecos)
	default:
		return nil, fmt.Errorf("fonte desconhecida em aguardar: %q", configuracao.Fonte)
	}
}

// Sem grupo de consumo: entrar num grupo negocia particao com o broker e a
// mensagem produzida durante a negociacao se perde, virando timeout que e do
// consumidor e nao do servico. Aqui o offset de cada particao e lido no momento
// em que a assinatura abre, antes de a carga comecar, e a leitura segue dali.
func abrirKafka(configuracao *Configuracao, brokers []string) (*assinatura, error) {
	conexao, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return nil, fmt.Errorf("nao consegui falar com o broker %s: %v", brokers[0], err)
	}
	defer conexao.Close()

	particoes, err := conexao.ReadPartitions(configuracao.Topico)
	if err != nil {
		return nil, fmt.Errorf("nao consegui ler as particoes de %q: %v", configuracao.Topico, err)
	}
	if len(particoes) == 0 {
		return nil, fmt.Errorf("o topico %q nao existe no broker", configuracao.Topico)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	assinada := novaAssinatura(configuracao.Campo)
	assinada.cancelar = cancelar

	leitores := make([]*kafka.Reader, 0, len(particoes))
	for _, particao := range particoes {
		offset, err := ultimoOffset(brokers[0], configuracao.Topico, particao.ID)
		if err != nil {
			cancelar()
			for _, aberto := range leitores {
				_ = aberto.Close()
			}
			return nil, err
		}

		leitor := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   brokers,
			Topic:     configuracao.Topico,
			Partition: particao.ID,
			MinBytes:  1,
			MaxBytes:  10 << 20,
			MaxWait:   20 * time.Millisecond,
		})
		if err := leitor.SetOffset(offset); err != nil {
			cancelar()
			_ = leitor.Close()
			for _, aberto := range leitores {
				_ = aberto.Close()
			}
			return nil, fmt.Errorf("nao consegui posicionar a leitura da particao %d: %v", particao.ID, err)
		}
		leitores = append(leitores, leitor)

		numero := particao.ID
		go func() {
			for {
				registro, err := leitor.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					continue
				}
				assinada.entregar(assinada.correlacaoDe(registro.Key, registro.Value), mensagem{
					corpo:     registro.Value,
					chegadaEm: time.Now(),
					atributos: map[string]string{
						"kafka.particao.consumida." + configuracao.Topico: strconv.Itoa(numero),
					},
				})
			}
		}()
	}

	assinada.fechar = func() {
		for _, leitor := range leitores {
			_ = leitor.Close()
		}
	}
	return assinada, nil
}

// A conexao precisa ser com o lider daquela particao: uma conexao generica com
// o broker nao sabe de qual particao e o offset pedido.
func ultimoOffset(broker, topico string, particao int) (int64, error) {
	lider, err := kafka.DialLeader(context.Background(), "tcp", broker, topico, particao)
	if err != nil {
		return 0, fmt.Errorf("nao consegui falar com o lider da particao %d de %q: %v", particao, topico, err)
	}
	defer lider.Close()

	offset, err := lider.ReadLastOffset()
	if err != nil {
		return 0, fmt.Errorf("nao consegui ler o offset da particao %d: %v", particao, err)
	}
	return offset, nil
}

func abrirAMQP(configuracao *Configuracao, enderecos []string) (*assinatura, error) {
	conexao, err := amqp.Dial(normalizarAMQP(enderecos[0]))
	if err != nil {
		return nil, fmt.Errorf("nao consegui conectar em %s: %v", enderecos[0], err)
	}
	canal, err := conexao.Channel()
	if err != nil {
		_ = conexao.Close()
		return nil, fmt.Errorf("nao consegui abrir o canal AMQP: %v", err)
	}
	if _, err := canal.QueueDeclare(configuracao.Topico, true, false, false, false, nil); err != nil {
		_ = canal.Close()
		_ = conexao.Close()
		return nil, fmt.Errorf("nao consegui declarar a fila %q: %v", configuracao.Topico, err)
	}
	entregas, err := canal.Consume(configuracao.Topico, "", true, false, false, false, nil)
	if err != nil {
		_ = canal.Close()
		_ = conexao.Close()
		return nil, fmt.Errorf("nao consegui consumir a fila %q: %v", configuracao.Topico, err)
	}

	assinada := novaAssinatura(configuracao.Campo)
	assinada.fechar = func() {
		_ = canal.Close()
		_ = conexao.Close()
	}

	go func() {
		for entrega := range entregas {
			chave := []byte(entrega.MessageId)
			if len(chave) == 0 {
				chave = []byte(entrega.CorrelationId)
			}
			assinada.entregar(assinada.correlacaoDe(chave, entrega.Body), mensagem{
				corpo:     entrega.Body,
				chegadaEm: time.Now(),
				atributos: nil,
			})
		}
	}()
	return assinada, nil
}

func normalizarAMQP(endereco string) string {
	if strings.HasPrefix(endereco, "amqp://") || strings.HasPrefix(endereco, "amqps://") {
		return endereco
	}
	return "amqp://" + endereco
}
