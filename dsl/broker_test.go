package dsl_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/dsl"
	"github.com/Diegobraun/braunrate/internal/messaging"
)

func brokerScenario(t *testing.T, broker *dsl.BrokerAuth) error {
	t.Helper()
	_, err := dsl.New("Cobranca").
		Target("kafka.homolog:9093").
		KafkaBroker(broker).
		Steady(dsl.PerSecond(10), 5*time.Second).
		Step(dsl.Kafka("pedidos").Value("{}"), dsl.Name("publicar pedido")).
		Build()
	return err
}

// The Go audience writes the scenario in a file that goes to the repository
// just like the YAML one, so it gets the same refusal.
func TestLiteralPasswordIsRefusedInTheDSLToo(t *testing.T) {
	for _, password := range []string{"p4ssw0rd", "${KAFKA_SENHA:-p4ssw0rd}", ""} {
		err := brokerScenario(t, dsl.BrokerAt("kafka.homolog:9093").SCRAM512("ana", password))
		if err == nil {
			t.Fatalf("a senha %q foi aceita no código", password)
		}
		if !strings.Contains(err.Error(), "variável de ambiente") {
			t.Fatalf("o erro não ensina a forma certa para %q: %v", password, err)
		}
	}
}

func TestEnvironmentReferenceIsAcceptedInTheDSL(t *testing.T) {
	t.Setenv("KAFKA_SENHA", "segredo-do-ambiente")

	spec, err := dsl.New("Cobranca").
		Target("kafka.homolog:9093").
		KafkaBroker(dsl.BrokerAt("kafka.homolog:9093").SCRAM512("ana", "${KAFKA_SENHA}").CA("/etc/ssl/ca.pem")).
		Steady(dsl.PerSecond(10), 5*time.Second).
		Step(dsl.Kafka("pedidos").Value("{}"), dsl.Name("publicar pedido")).
		Build()
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	broker := spec.Messaging.Kafka
	if broker.Auth.Kind != messaging.SCRAM512 || broker.Auth.PasswordVar != "KAFKA_SENHA" {
		t.Fatalf("credencial lida errada: %+v", broker.Auth)
	}
	if broker.Auth.Password != "segredo-do-ambiente" {
		t.Fatalf("a senha não veio do ambiente: %+v", broker.Auth)
	}
	if !broker.TLS.Enabled || broker.TLS.CA != "/etc/ssl/ca.pem" {
		t.Fatalf("tls lido errado: %+v", broker.TLS)
	}
}

// The whole point of msk_iam is that there is nowhere to put a key.
func TestMSKIAMTakesRegionAndNoCredentialInTheDSL(t *testing.T) {
	spec, err := dsl.New("Cobranca").
		Target("b-1.msk.exemplo:9098").
		KafkaBroker(dsl.BrokerAt("b-1.msk.exemplo:9098").MSKIAM("us-east-1")).
		Steady(dsl.PerSecond(10), 5*time.Second).
		Step(dsl.Kafka("pedidos").Value("{}"), dsl.Name("publicar pedido")).
		Build()
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	broker := spec.Messaging.Kafka
	if broker.Auth.Region != "us-east-1" || broker.Auth.Password != "" || broker.Auth.PasswordVar != "" {
		t.Fatalf("msk_iam guardou credencial ou perdeu a região: %+v", broker.Auth)
	}
	if !broker.TLS.Enabled {
		t.Fatal("msk_iam sem TLS: a porta 9098 não aceita outra coisa")
	}
}
