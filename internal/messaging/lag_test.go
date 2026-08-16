package messaging_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/segmentio/kafka-go"
)

func plainBroker(t *testing.T) string {
	t.Helper()
	address := os.Getenv("BRAUNRATE_KAFKA")
	if address == "" {
		t.Skip("defina BRAUNRATE_KAFKA para rodar contra um broker de verdade")
	}
	return address
}

// A consumer group that reads one message per tick, on purpose: the point is a
// service that does not keep up, which is exactly the case where producing
// looks fast and the chain is broken.
func slowGroup(t *testing.T, address, topic, group string, perMessage time.Duration) {
	t.Helper()
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{address},
		Topic:       topic,
		GroupID:     group,
		MinBytes:    1,
		MaxBytes:    10 << 20,
		StartOffset: kafka.FirstOffset,
	})
	readContext, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, err := reader.ReadMessage(readContext); err != nil {
				return
			}
			time.Sleep(perMessage)
		}
	}()
	t.Cleanup(func() {
		stop()
		_ = reader.Close()
		<-done
	})
}

func TestConsumerLagShowsTheServiceFallingBehind(t *testing.T) {
	address := plainBroker(t)
	topic := fmt.Sprintf("lag-%d", time.Now().UnixNano())
	group := topic + "-grupo"

	// The topic is created before anything else for the same reason the chain
	// test creates it: a broker without automatic creation refuses the first
	// write, and the failure would be read as a defect of the measurement.
	createPlainTopic(t, address, topic, 1)
	slowGroup(t, address, topic, group, 40*time.Millisecond)
	time.Sleep(2 * time.Second)

	document := runScenario(t, fmt.Sprintf(`
nome: Atraso do consumidor
alvo: %s

dados:
  pedidos:
    gerar: { id: uuid }

carga:
  perfis:
    - patamar: { taxa: 200/s, durante: 5s }

cenario:
  - kafka: { topico: %s, grupo: %s, chave: "${pedidos.id}", valor: '{"id":"${pedidos.id}"}' }
    nome: publicar pedido
`, address, topic, group))

	if document.Overall.Errors > 0 {
		t.Fatalf("erro ao produzir: %+v", document.Steps[0].Details)
	}
	if len(document.Run.ConsumerLag) != 1 {
		t.Fatalf("o atraso do grupo nao foi medido: %+v", document.Run.ConsumerLag)
	}

	lag := document.Run.ConsumerLag[0]
	if lag.Problem != "" {
		t.Fatalf("a medicao do atraso falhou: %s", lag.Problem)
	}
	if lag.Group != group || lag.Topic != topic {
		t.Fatalf("o atraso foi atribuido a outro grupo: %+v", lag)
	}
	// 200/s against a consumer that takes 40 ms per message: the group cannot
	// keep up, and that is the whole point of measuring this.
	if lag.Max < 100 {
		t.Fatalf("consumidor de 25/s sob carga de 200/s deveria ficar para tras; atraso maximo medido: %d", lag.Max)
	}
	t.Logf("atraso maximo %d, no fim %d, em %d leituras", lag.Max, lag.Final, lag.Readings)
}

// Producing to a declared partition is the opposite of the usual advice, so it
// only happens when the scenario says so — and then it has to actually land
// there, or the report would describe a distribution that did not happen.
func TestDeclaredPartitionIsWhereTheMessageLands(t *testing.T) {
	address := plainBroker(t)
	topic := fmt.Sprintf("particao-%d", time.Now().UnixNano())
	createPlainTopic(t, address, topic, 3)

	document := runScenario(t, fmt.Sprintf(`
nome: Particao declarada
alvo: %s

dados:
  pedidos:
    gerar: { id: uuid }

carga:
  perfis:
    - patamar: { taxa: 20/s, durante: 2s }

cenario:
  - kafka: { topico: %s, particao: 2, chave: "${pedidos.id}", valor: '{"id":"${pedidos.id}"}' }
    nome: publicar pedido
`, address, topic))

	if document.Overall.Errors > 0 {
		t.Fatalf("erro ao produzir: %+v", document.Steps[0].Details)
	}

	found := false
	for _, variety := range document.Variety {
		if !strings.Contains(variety.Name, topic) {
			continue
		}
		found = true
		if !strings.HasPrefix(variety.Name, "kafka.particao.declarada.") {
			t.Fatalf("a particao declarada foi contada como se tivesse sido escolhida pela chave: %s", variety.Name)
		}
		if variety.Distinct != 1 {
			t.Fatalf("a carga foi para %d particoes, e o cenario declarou uma", variety.Distinct)
		}
		if variety.Available != 3 {
			t.Fatalf("o topico tem 3 particoes e o relatorio viu %d", variety.Available)
		}
	}
	if !found {
		t.Fatalf("o relatorio nao registrou a particao de %s: %+v", topic, document.Variety)
	}

	// Concentration asked for is still concentration: the report has to say the
	// number is that of one partition, without accusing the key of not varying.
	warned := false
	for _, warning := range document.Warnings {
		if !strings.Contains(warning.Message, "particao declarada") {
			continue
		}
		warned = true
		if warning.Severity != metrics.SeverityMedium {
			t.Fatalf("particao declarada e deliberada, e o aviso saiu como %q", warning.Severity)
		}
		if strings.Contains(warning.Message, "chave") {
			t.Fatalf("o aviso mandou variar a chave, que nao e o que resolve: %s", warning.Message)
		}
	}
	if !warned {
		t.Fatalf("carga numa particao so nao foi avisada: %+v", document.Warnings)
	}
}

func createPlainTopic(t *testing.T, address, name string, partitions int) {
	t.Helper()
	conn, err := kafka.Dial("tcp", address)
	if err != nil {
		t.Fatalf("nao consegui falar com o broker: %v", err)
	}
	defer func() { _ = conn.Close() }()

	err = conn.CreateTopics(kafka.TopicConfig{
		Topic: name, NumPartitions: partitions, ReplicationFactor: 1,
	})
	// A broker with automatic creation may have got there first, and that is not
	// a failure of the test — the topic existing is what the test wanted.
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		t.Fatalf("nao consegui criar o topico: %v", err)
	}
	for attempt := 0; attempt < 50; attempt++ {
		found, err := conn.ReadPartitions(name)
		if err == nil && len(found) == partitions {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("o topico %q nao ficou pronto", name)
}
