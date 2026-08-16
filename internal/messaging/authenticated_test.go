package messaging_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"net"
	"strconv"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/segmentio/kafka-go"
)

type credential struct{ address, user, password, ca string }

func authenticatedBroker(t *testing.T) credential {
	t.Helper()
	address := os.Getenv("BRAUNRATE_KAFKA_TLS")
	if address == "" {
		t.Skip("defina BRAUNRATE_KAFKA_TLS, BRAUNRATE_KAFKA_USER, BRAUNRATE_KAFKA_PASSWORD e BRAUNRATE_KAFKA_CA para rodar contra o broker autenticado")
	}
	return credential{
		address:  address,
		user:     os.Getenv("BRAUNRATE_KAFKA_USER"),
		password: os.Getenv("BRAUNRATE_KAFKA_PASSWORD"),
		ca:       os.Getenv("BRAUNRATE_KAFKA_CA"),
	}
}

// The topic is created before the run for the same reason the chain target
// creates it: the first write to a topic that does not exist fails once, and
// that failure would be read as a defect of the broker.
func createTopic(t *testing.T, broker credential, name string) {
	t.Helper()
	settings := &messaging.Broker{
		Addresses: []string{broker.address},
		Auth:      messaging.Auth{Kind: messaging.SCRAM512, User: broker.user, Password: broker.password},
		TLS:       messaging.TLS{Enabled: true, CA: broker.ca},
	}
	dialer, err := settings.Dialer()
	if err != nil {
		t.Fatalf("dialer nao montou: %v", err)
	}
	conn, err := dialer.Dial("tcp", broker.address)
	if err != nil {
		t.Fatalf("nao consegui falar com o broker autenticado: %v", err)
	}
	defer func() { _ = conn.Close() }()

	controller, err := conn.Controller()
	if err != nil {
		t.Fatalf("nao achei o controlador: %v", err)
	}
	leader, err := dialer.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		t.Fatalf("nao consegui falar com o controlador: %v", err)
	}
	defer func() { _ = leader.Close() }()

	if err := leader.CreateTopics(kafka.TopicConfig{Topic: name, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatalf("nao consegui criar o topico %q: %v", name, err)
	}
	for attempt := 0; attempt < 50; attempt++ {
		partitions, err := conn.ReadPartitions(name)
		if err == nil && len(partitions) > 0 && partitions[0].Leader.Host != "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("o topico %q nao ficou pronto", name)
}

func runScenario(t *testing.T, content string) metrics.Document {
	t.Helper()
	spec, err := scenario.Parse([]byte(content))
	if err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("cenario nao validou: %v", err)
	}
	executor, err := engine.New(spec, engine.DefaultOptions())
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := executor.Execute(context.Background())
	protocol.CloseAll()
	return document
}

func authenticatedScenario(t *testing.T, broker credential, topic, password string) string {
	t.Helper()
	t.Setenv("BROKER_SENHA", password)
	return fmt.Sprintf(`
nome: Broker autenticado
alvo: %s

mensageria:
  kafka:
    brokers: [%s]
    autenticacao: { tipo: scram_sha512, usuario: %s, senha: "${BROKER_SENHA}" }
    tls: { ca: %s }

carga:
  perfis:
    - patamar: { taxa: 20/s, durante: 2s }

cenario:
  - kafka: { topico: %s, chave: "${iteracao}", valor: '{"id":"${iteracao}"}' }
    nome: publicar pedido
`, broker.address, broker.address, broker.user, broker.ca, topic)
}

// A test that only runs against a local broker with no credentials decides
// nothing about a broker in staging. This one exercises the whole path: SCRAM
// over TLS with a CA of our own.
func TestProducesThroughSCRAMOverTLS(t *testing.T) {
	broker := authenticatedBroker(t)
	topic := fmt.Sprintf("autenticado-%d", time.Now().UnixNano())
	createTopic(t, broker, topic)

	document := runScenario(t, authenticatedScenario(t, broker, topic, broker.password))

	if !document.Valid() {
		t.Fatalf("resultado invalido: %+v", document.Sanity.Findings)
	}
	if document.Overall.Errors > 0 {
		t.Fatalf("%d erro(s) contra o broker autenticado: %+v", document.Overall.Errors, document.Steps[0].Details)
	}
	if document.Overall.Count == 0 {
		t.Fatal("nenhuma mensagem entrou na medicao")
	}
}

// The wrong password has to arrive as a wrong password. Reporting it as a
// network failure sends the person to look at the firewall.
func TestWrongPasswordIsReportedAsAuthenticationNeverAsNetwork(t *testing.T) {
	broker := authenticatedBroker(t)
	topic := fmt.Sprintf("autenticado-errado-%d", time.Now().UnixNano())

	document := runScenario(t, authenticatedScenario(t, broker, topic, "senha-errada-de-proposito"))

	if document.Overall.Errors == 0 {
		t.Fatal("o broker aceitou uma senha errada")
	}
	classes := document.Steps[0].ErrorsByClass
	if classes[string(protocol.ErrAuth)] == 0 {
		t.Fatalf("a senha errada nao foi classificada como autenticacao: %+v", classes)
	}
	if classes[string(protocol.ErrNetwork)] > 0 {
		t.Fatalf("credencial errada virou falha de rede: %+v", classes)
	}
	explained := false
	for detail := range document.Steps[0].Details {
		if strings.Contains(detail, "recusou a credencial") {
			explained = true
		}
		if strings.Contains(detail, "senha-errada-de-proposito") {
			t.Fatalf("a senha apareceu no relatorio: %q", detail)
		}
	}
	if !explained {
		t.Fatalf("o erro nao explica o que fazer: %+v", document.Steps[0].Details)
	}
}

// The TLS and SASL handshake is paid once, when the connection opens. If it
// entered the message latency, the first message would carry the whole
// handshake and the p99 would describe the connection, not the broker.
func TestHandshakeDoesNotLandInTheLatencyOfTheFirstMessage(t *testing.T) {
	broker := authenticatedBroker(t)
	topic := fmt.Sprintf("autenticado-handshake-%d", time.Now().UnixNano())
	createTopic(t, broker, topic)

	document := runScenario(t, authenticatedScenario(t, broker, topic, broker.password))

	if document.Overall.Errors > 0 {
		t.Fatalf("erro na execucao: %+v", document.Steps[0].Details)
	}
	latency := document.Steps[0].Reported()
	if latency.Max > 10*latency.P50+500 {
		t.Fatalf("a pior mensagem levou %.1f ms contra %.1f ms na metade: o handshake caiu dentro da medicao",
			latency.Max, latency.P50)
	}
}

func TestBrokerIsDescribedInTheReportWithoutTheSecret(t *testing.T) {
	broker := authenticatedBroker(t)
	topic := fmt.Sprintf("autenticado-relatorio-%d", time.Now().UnixNano())
	createTopic(t, broker, topic)

	document := runScenario(t, authenticatedScenario(t, broker, topic, broker.password))

	if len(document.Run.Brokers) == 0 {
		t.Fatal("o relatorio nao diz contra que broker a medicao foi feita")
	}
	printed := strings.Join(document.Run.Brokers, "\n")
	if strings.Contains(printed, broker.password) {
		t.Fatalf("a senha apareceu no documento: %s", printed)
	}
	if !strings.Contains(printed, "scram_sha512") || !strings.Contains(printed, "TLS com CA propria") {
		t.Fatalf("o relatorio nao descreve a conexao: %s", printed)
	}
}
