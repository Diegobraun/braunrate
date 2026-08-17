package amqp_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/amqp"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, text string) (protocol.Config, error) {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("yaml inválido no teste: %v", err)
	}
	return amqp.New(protocol.DefaultOptions()).Decode(document.Content[0])
}

func TestQueueAloneIsEnoughAndBecomesRoute(t *testing.T) {
	config, err := decode(t, "queue: pedidos\nbody: { id: 1 }\n")
	if err != nil {
		t.Fatalf("não decodificou: %v", err)
	}
	if config.AggregationKey() != "amqp publish pedidos" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
}

func TestExchangeWithRouteAppearsInKey(t *testing.T) {
	config, err := decode(t, "exchange: cobranca\nroutingKey: pedido.criado\nbody: { id: 1 }\n")
	if err != nil {
		t.Fatalf("não decodificou: %v", err)
	}
	if config.AggregationKey() != "amqp publish cobranca/pedido.criado" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
}

// Without confirmation the measured time is the socket write: it would time
// the local network, not the broker accepting the message.
func TestConfirmationIsDefault(t *testing.T) {
	config, err := decode(t, "queue: pedidos\nbody: { id: 1 }\n")
	if err != nil {
		t.Fatalf("não decodificou: %v", err)
	}
	description := strings.Join(config.(protocol.Describable).Describe(), " ")
	if !strings.Contains(description, "waits for the broker to confirm") {
		t.Errorf("descricao = %s", description)
	}
}

func TestStepWithoutDestinationOrBodyTeachesRightForm(t *testing.T) {
	for name, text := range map[string]string{
		"sem destino": "body: { id: 1 }\n",
		"sem corpo":   "queue: pedidos\n",
	} {
		_, err := decode(t, text)
		if err == nil {
			t.Fatalf("%s: esperava erro", name)
		}
		if !strings.Contains(err.Error(), "- amqp:") {
			t.Errorf("%s: a mensagem precisa mostrar um exemplo: %q", name, err.Error())
		}
	}
}
