package wait_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/wait"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, text string) (protocol.Config, error) {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	return wait.New(protocol.DefaultOptions()).Decode(document.Content[0])
}

func TestAggregationKeyIsAwaitedDestination(t *testing.T) {
	config, err := decode(t, "kafka: { topico: pedidos-processados }\nchave: \"${pedidos.id}\"\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if config.AggregationKey() != "aguardar pedidos-processados" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
}

// With no correlation value any message would do: the measurement would time
// the fastest consumer on the topic instead of that iteration's message.
func TestWaitWithoutCorrelationIsRefused(t *testing.T) {
	_, err := decode(t, "kafka: { topico: pedidos-processados }\n")
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "mediria o consumidor mais rapido") {
		t.Errorf("a mensagem precisa dizer por que isso invalida a medicao: %q", err.Error())
	}
}

func TestTimeoutHasDefaultAndCanBeDeclared(t *testing.T) {
	config, err := decode(t, "kafka: { topico: t }\nchave: x\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	description := strings.Join(config.(protocol.Describable).Describe(), " ")
	if !strings.Contains(description, "30s") {
		t.Errorf("faltou o timeout padrao na descricao: %s", description)
	}

	withTimeout, err := decode(t, "kafka: { topico: t }\nchave: x\ntimeout: 90s\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if !strings.Contains(strings.Join(withTimeout.(protocol.Describable).Describe(), " "), "1m30s") {
		t.Error("timeout declarado nao apareceu na descricao")
	}
}

func TestMissingAddressTeachesWhereToDeclare(t *testing.T) {
	config, err := decode(t, "kafka: { topico: t }\nchave: x\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	response := wait.New(protocol.DefaultOptions()).Execute(t.Context(), protocol.Request{Config: config})
	if response.Class != protocol.ErrConfig || !strings.Contains(response.Detail, "brokers") {
		t.Errorf("classe = %q, detalhe = %q", response.Class, response.Detail)
	}
}

// C11 of the audit: "tempo esgotado" says what happened, not what to do about
// it, and "enderecos:" came out empty when the address came from the target.
func TestWaitDescribesWhereTheAddressComesFromWhenTheStepDeclaresNone(t *testing.T) {
	config, err := decode(t, "kafka: { topico: pedidos-processados }\nchave: abc\ntimeout: 5s\n")
	if err != nil {
		t.Fatalf("passo invalido: %v", err)
	}
	description := strings.Join(config.(protocol.Describable).Describe(), "\n")

	if strings.Contains(description, "enderecos: \n") || strings.HasSuffix(description, "enderecos:") {
		t.Fatalf("o campo de enderecos saiu vazio:\n%s", description)
	}
	if !strings.Contains(description, "alvo do cenario") {
		t.Fatalf("a descricao nao diz de onde vem o endereco:\n%s", description)
	}
}
