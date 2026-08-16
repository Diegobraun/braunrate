package kafka_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/kafka"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, text string) (protocol.Config, error) {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	return kafka.New(protocol.DefaultOptions()).Decode(document.Content[0])
}

// The topic is the business flow; the broker is infrastructure. Whoever reads
// the report needs to know which flow got slow.
func TestAggregationKeyIsTopic(t *testing.T) {
	config, err := decode(t, "topico: pedidos\nchave: \"1\"\nvalor: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if config.AggregationKey() != "kafka produzir pedidos" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
}

func TestMessageKeyIsResolvedPerIteration(t *testing.T) {
	config, err := decode(t, "topico: pedidos\nchave: \"${pedidos.id}\"\nvalor: { id: \"${pedidos.id}\" }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	resolvida := config.Resolve(func(text string) string {
		return strings.ReplaceAll(text, "${pedidos.id}", "abc-123")
	})
	description := strings.Join(resolvida.(protocol.Describable).Describe(), " ")
	if !strings.Contains(description, "abc-123") {
		t.Errorf("chave e valor precisam ser resolvidos por iteracao: %s", description)
	}
}

func TestStepWithoutTopicOrValueTeachesRightForm(t *testing.T) {
	testCases := map[string]string{
		"sem topico": "valor: { id: 1 }\n",
		"sem valor":  "topico: pedidos\n",
	}
	for name, text := range testCases {
		_, err := decode(t, text)
		if err == nil {
			t.Fatalf("%s: esperava erro", name)
		}
		if !strings.Contains(err.Error(), "- kafka:") {
			t.Errorf("%s: a mensagem precisa mostrar um exemplo: %q", name, err.Error())
		}
	}
}

func TestUnknownAcksListsOptions(t *testing.T) {
	_, err := decode(t, "topico: pedidos\nvalor: { id: 1 }\nacks: talvez\n")
	if err == nil || !strings.Contains(err.Error(), "todos, lider ou nenhum") {
		t.Errorf("erro = %v", err)
	}
}

func TestMissingBrokerTeachesWhereToDeclare(t *testing.T) {
	config, err := decode(t, "topico: pedidos\nvalor: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	response := kafka.New(protocol.DefaultOptions()).Execute(t.Context(), protocol.Request{Config: config})
	if response.Class != protocol.ErrConfig {
		t.Fatalf("classe = %q", response.Class)
	}
	if !strings.Contains(response.Detail, "kafka://") {
		t.Errorf("detalhe = %q", response.Detail)
	}
}
