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
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	return amqp.New(protocol.DefaultOptions()).Decode(document.Content[0])
}

func TestQueueAloneIsEnoughAndBecomesRoute(t *testing.T) {
	config, err := decode(t, "fila: pedidos\ncorpo: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if config.AggregationKey() != "amqp publicar pedidos" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
}

func TestExchangeWithRouteAppearsInKey(t *testing.T) {
	config, err := decode(t, "troca: cobranca\nrota: pedido.criado\ncorpo: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if config.AggregationKey() != "amqp publicar cobranca/pedido.criado" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
}

// Sem confirmacao, o tempo medido e o de escrever no socket: mediria a rede
// local, e nao o broker aceitando a mensagem.
func TestConfirmationIsDefault(t *testing.T) {
	config, err := decode(t, "fila: pedidos\ncorpo: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	description := strings.Join(config.(protocol.Describable).Describe(), " ")
	if !strings.Contains(description, "espera confirmacao do broker") {
		t.Errorf("descricao = %s", description)
	}
}

func TestStepWithoutDestinationOrBodyTeachesRightForm(t *testing.T) {
	for name, text := range map[string]string{
		"sem destino": "corpo: { id: 1 }\n",
		"sem corpo":   "fila: pedidos\n",
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
