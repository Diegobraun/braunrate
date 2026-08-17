package messaging_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/testsupport"
	"github.com/segmentio/kafka-go"
)

// Messaging can only be measured against a real broker: a double answers in
// whatever time the test dictates, and the number would stop meaning anything.
// With no broker the test declares it skipped instead of pretending it passed.
func kafkaBroker(t *testing.T) string {
	t.Helper()
	address := os.Getenv("BRAUNRATE_KAFKA")
	if address == "" {
		t.Skip("sem BRAUNRATE_KAFKA: teste de mensageria pulado, não aprovado")
	}
	return address
}

func amqpBroker(t *testing.T) string {
	t.Helper()
	address := os.Getenv("BRAUNRATE_AMQP")
	if address == "" {
		t.Skip("sem BRAUNRATE_AMQP: teste de mensageria pulado, não aprovado")
	}
	return address
}

func topic(t *testing.T, brokers string, name string, partitions int) string {
	t.Helper()
	complete := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	conn, err := kafka.Dial("tcp", brokers)
	if err != nil {
		t.Fatalf("não consegui falar com o broker: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             complete,
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	}); err != nil {
		t.Fatalf("não consegui criar o tópico: %v", err)
	}
	return complete
}

func execute(t *testing.T, content string) (metrics.Document, slo.Verdict) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "cenario.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("não consegui escrever o cenário: %v", err)
	}
	c, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenário não carregou: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	options := engine.DefaultOptions()
	options.DataRoot = root
	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	document := m.Execute(context.Background())
	t.Cleanup(func() { protocol.CloseAll() })
	return document, slo.Evaluate(c.SLO, document, nil)
}

const chainScenario = `
name: Cadeia assincrona
target: %s

data:
  pedidos:
    generate: { id: uuid }

load:
  profiles:
    - steady: { rate: 30/s, duration: 2s }

scenario:
  - kafka:
      topic: %s
      key: "%s"
      value: { pedido: "${pedidos.id}" }

  - await:
      kafka: { topic: %s }
      key: "${pedidos.id}"
      timeout: 15s

slo:
  - global: { errors: < 1 }
`

// Async chain measurement: the wait step only ends when that iteration's
// message shows up on the other side, so the journey measures producer,
// processor and consumer together, which is what the end user feels.
func TestAsyncChainMeasuresProducerToConsumer(t *testing.T) {
	brokers := kafkaBroker(t)
	input := topic(t, brokers, "entrada", 4)
	out := topic(t, brokers, "saida", 4)

	const processorDelay = 40 * time.Millisecond
	processor := testsupport.NewProcessor(testsupport.ProcessorOptions{
		Brokers: strings.Split(brokers, ","),
		Input:   input,
		Output:  out,
		Delay:   processorDelay,
	})
	if err := processor.Start(); err != nil {
		t.Fatalf("processador não subiu: %v", err)
	}
	t.Cleanup(func() { _ = processor.Close() })

	document, verdict := execute(t, fmt.Sprintf(chainScenario,
		brokers, input, "${pedidos.id}", out))

	if !verdict.Passed {
		t.Fatalf("a cadeia deveria fechar sem erro: %s", verdict.Sentence)
	}
	if document.Journey.Completed == 0 {
		t.Fatal("nenhuma jornada chegou ao fim: a mensagem produzida nunca voltou")
	}

	byName := map[string]metrics.StepResult{}
	for _, step := range document.Steps {
		byName[step.Name] = step
	}
	production, hasProduction := byName["kafka produce "+input]
	wait, temEspera := byName["await "+out]
	if !hasProduction || !temEspera {
		t.Fatalf("o relatório precisa de uma linha por destino: %+v", byName)
	}
	if wait.Latency.P50 < float64(processorDelay.Milliseconds()) {
		t.Errorf("a espera mediu %0.1f ms, menos que os %s do processador: não mediu a cadeia",
			wait.Latency.P50, processorDelay)
	}
	if production.Latency.P50 > wait.Latency.P50 {
		t.Errorf("produzir (%0.1f ms) não deveria custar mais que a cadeia inteira (%0.1f ms)",
			production.Latency.P50, wait.Latency.P50)
	}
	if document.Journey.Latency.P50 < wait.Latency.P50 {
		t.Error("a jornada precisa incluir produção e espera")
	}
}

// A always-equal partition key sends everything to the same partition: the
// rest of the cluster sits idle and the number turns optimistic, exactly like
// the single subscriber in the identity bug.
func TestFixedPartitionKeyInvalidatesResult(t *testing.T) {
	brokers := kafkaBroker(t)
	input := topic(t, brokers, "fixa", 4)
	out := topic(t, brokers, "fixa-saida", 4)

	processor := testsupport.NewProcessor(testsupport.ProcessorOptions{
		Brokers: strings.Split(brokers, ","),
		Input:   input,
		Output:  out,
	})
	if err := processor.Start(); err != nil {
		t.Fatalf("processador não subiu: %v", err)
	}
	t.Cleanup(func() { _ = processor.Close() })

	document, _ := execute(t, fmt.Sprintf(chainScenario,
		brokers, input, "sempre-a-mesma", out))

	var found bool
	for _, warning := range document.Warnings {
		if warning.Kind != "missingVariety" || !strings.Contains(warning.Message, "partition") {
			continue
		}
		found = true
		if warning.Severity != metrics.SeverityHigh {
			t.Errorf("gravidade = %q, esperava alta", warning.Severity)
		}
		if !strings.Contains(warning.Message, "Make the message key vary") {
			t.Errorf("a mensagem precisa dizer o que fazer: %q", warning.Message)
		}
	}
	if !found {
		t.Fatalf("carga inteira numa partição só precisa avisar; avisos: %+v", document.Warnings)
	}
	if document.Valid() {
		t.Error("resultado concentrado numa partição não pode ser dado como válido")
	}
}

const scenarioWithoutProcessor = `
name: Espera sem resposta
target: %s

data:
  pedidos:
    generate: { id: uuid }

load:
  profiles:
    - steady: { rate: 5/s, duration: 1s }

scenario:
  - kafka:
      topic: %s
      key: "${pedidos.id}"
      value: { pedido: "${pedidos.id}" }

  - await:
      kafka: { topic: %s }
      key: "${pedidos.id}"
      timeout: 2s
`

func TestMessageThatNeverArrivesBecomesExplainedTimeout(t *testing.T) {
	brokers := kafkaBroker(t)
	input := topic(t, brokers, "sem-processador", 1)
	out := topic(t, brokers, "sem-processador-saida", 1)

	document, _ := execute(t, fmt.Sprintf(scenarioWithoutProcessor, brokers, input, out))

	var wait metrics.StepResult
	for _, step := range document.Steps {
		if strings.HasPrefix(step.Name, "await ") {
			wait = step
		}
	}
	if wait.ErrorsByClass["timeout"] == 0 {
		t.Fatalf("sem processador, a espera precisa virar timeout: %+v detalhes=%+v", wait.ErrorsByClass, wait.Details)
	}
	var explicou bool
	for detail := range wait.Details {
		if strings.Contains(detail, "did not arrive within") && strings.Contains(detail, out) {
			explicou = true
		}
	}
	if !explicou {
		t.Errorf("o detalhe precisa dizer o que era esperado e onde: %+v", wait.Details)
	}
	if document.Journey.Completed != 0 {
		t.Error("jornada sem a mensagem de volta não pode contar como completa")
	}
}

const amqpScenario = `
name: Cadeia AMQP
target: %s

data:
  pedidos:
    generate: { id: uuid }

load:
  profiles:
    - steady: { rate: 50/s, duration: 2s }

scenario:
  - amqp:
      queue: %s
      messageId: "${pedidos.id}"
      body: { pedido: "${pedidos.id}" }

  - await:
      amqp: { queue: %s, url: %s }
      key: "${pedidos.id}"
      timeout: 10s

slo:
  - global: { errors: < 1 }
`

func TestAMQPPublishesAndWaitsOnSameQueue(t *testing.T) {
	address := amqpBroker(t)
	queue := fmt.Sprintf("braunrate-teste-%d", time.Now().UnixNano())

	document, verdict := execute(t, fmt.Sprintf(amqpScenario, address, queue, queue, address))
	if !verdict.Passed {
		t.Fatalf("a fila deveria fechar sem erro: %s", verdict.Sentence)
	}
	if document.Journey.Completed == 0 {
		t.Fatal("nenhuma mensagem publicada voltou pela fila")
	}
	var publishing bool
	for _, step := range document.Steps {
		if step.Name == "amqp publish "+queue {
			publishing = true
			if step.Latency.P50 <= 0 {
				t.Error("publicação com confirmacao precisa ter latência medida")
			}
		}
	}
	if !publishing {
		t.Errorf("faltou a linha da publicação no relatório: %+v", document.Steps)
	}
}

const saturationScenario = `
name: Saturacao produzindo
target: %s

data:
  pedidos:
    generate: { id: uuid }

load:
  profiles:
    - steady: { rate: 3000/s, duration: 1s }

scenario:
  - kafka:
      topic: %s
      key: "${pedidos.id}"
      value: { pedido: "${pedidos.id}" }
`

// Back-pressure is not a protocol matter: the scheduler is the same one. If
// the generator cannot sustain production, the chain number is wrong the same
// way it would be under HTTP load, and the warning needs the same severity.
func TestSaturatedGeneratorWhileProducingInvalidatesResult(t *testing.T) {
	brokers := kafkaBroker(t)
	loadTopic := topic(t, brokers, "saturacao", 1)

	root := t.TempDir()
	path := filepath.Join(root, "cenario.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(saturationScenario, brokers, loadTopic)), 0o644); err != nil {
		t.Fatalf("não consegui escrever o cenário: %v", err)
	}
	c, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenário não carregou: %v", err)
	}

	options := engine.DefaultOptions()
	options.DataRoot = root
	options.MaxInflight = 1

	m, err := engine.New(c, options)
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	document := m.Execute(context.Background())
	t.Cleanup(func() { protocol.CloseAll() })

	var found bool
	for _, warning := range document.Warnings {
		if warning.Kind != "generatorSaturated" {
			continue
		}
		found = true
		if warning.Severity != metrics.SeverityHigh {
			t.Errorf("gravidade = %q, esperava alta: o número não vale", warning.Severity)
		}
	}
	if !found {
		t.Fatalf("gerador que não sustentou a produção precisa avisar; avisos: %+v", document.Warnings)
	}
	if document.Valid() {
		t.Error("resultado com gerador saturado produzindo não pode ser dado como válido")
	}
	if document.Scheduling.DroppedByInflightLimit == 0 && document.Scheduling.LateDispatches == 0 {
		t.Error("a saturacao precisa aparecer no agendamento, e não só no texto")
	}
}
