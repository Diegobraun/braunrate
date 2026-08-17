package scenario_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func parseMessaging(t *testing.T, block string) (scenario.Spec, error) {
	t.Helper()
	return scenario.Parse([]byte(`
name: x
target: kafka.homolog:9093

messaging:
` + block + `
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - kafka: { topic: pedidos, value: "{}" }
`))
}

// The scenario goes to the repository. A password written in it is a password
// published, and no phase of this tool is allowed to accept that.
func TestLiteralSecretIsRefusedAndTheMessageTeachesTheWayOut(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{"valor literal", `password: p4ssw0rd`},
		{"literal entre aspas", `password: "p4ssw0rd"`},
		{"referência com valor de reserva", `password: "${KAFKA_PASSWORD:-p4ssw0rd}"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMessaging(t, "  kafka:\n    auth: { type: scramSha512, user: ana, "+c.password+" }\n")
			if err == nil {
				t.Fatal("o cenário com segredo no arquivo foi aceito")
			}
			for _, fragment := range []string{"${BROKER_PASSWORD}", "goes into the repository", "BROKER_PASSWORD=..."} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("a mensagem não ensina %q: %v", fragment, err)
				}
			}
		})
	}
}

func TestEnvironmentReferenceIsAcceptedAndKeepsTheVariableName(t *testing.T) {
	t.Setenv("KAFKA_SENHA", "segredo-do-ambiente")

	spec, err := parseMessaging(t, "  kafka:\n    brokers: [kafka.homolog:9093]\n"+
		"    auth: { type: scramSha512, user: ana, password: \"${KAFKA_SENHA}\" }\n    tls: { ca: /etc/ssl/ca.pem }\n")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	broker := spec.Messaging.Kafka
	if broker.Auth.Kind != messaging.SCRAM512 || broker.Auth.User != "ana" {
		t.Fatalf("autenticação lida errada: %+v", broker.Auth)
	}
	if broker.Auth.Password != "segredo-do-ambiente" || broker.Auth.PasswordVar != "KAFKA_SENHA" {
		t.Fatalf("a senha não veio do ambiente com o nome da variável: %+v", broker.Auth)
	}
	if !broker.TLS.Enabled || broker.TLS.CA != "/etc/ssl/ca.pem" {
		t.Fatalf("tls lido errado: %+v", broker.TLS)
	}
}

// Asking for a key in the scenario is the mistake this refuses to make
// possible, so the key names are refused by name.
func TestAccessKeyIsRefusedByNameAndPointsAtTheCredentialChain(t *testing.T) {
	for _, field := range []string{"key", "token", "secret", "access_key", "secret_key"} {
		_, err := parseMessaging(t, "  kafka:\n    auth: { type: mskIam, region: us-east-1, "+field+": AKIA123 }\n")
		if err == nil {
			t.Fatalf("o campo %q foi aceito", field)
		}
		if !strings.Contains(err.Error(), "standard AWS chain") {
			t.Fatalf("o campo %q não aponta o caminho certo: %v", field, err)
		}
	}
}

func TestMSKWithoutRegionIsRefusedWithTheExample(t *testing.T) {
	_, err := parseMessaging(t, "  kafka:\n    auth: { type: mskIam }\n")
	if err == nil || !strings.Contains(err.Error(), "region: us-east-1") {
		t.Fatalf("esperava erro ensinando a região, veio: %v", err)
	}
}

func TestRabbitMQRefusesAMechanismItDoesNotHave(t *testing.T) {
	_, err := parseMessaging(t, "  amqp:\n    auth: { type: scramSha512, user: ana, password: \"${SENHA}\" }\n")
	if err == nil || !strings.Contains(err.Error(), "saslPlain") {
		t.Fatalf("esperava erro dizendo o que o RabbitMQ aceita, veio: %v", err)
	}
}

// Terminal, HTML, JSON and debug all read this one function. If it ever prints
// the secret, it prints it everywhere at once.
func TestWhatMayBePrintedShowsKindAndUserAndNeverTheSecret(t *testing.T) {
	t.Setenv("KAFKA_SENHA", "p4ssw0rd-que-nao-pode-vazar")

	spec, err := parseMessaging(t, "  kafka:\n    brokers: [kafka.homolog:9093]\n"+
		"    auth: { type: scramSha512, user: ana, password: \"${KAFKA_SENHA}\" }\n    tls: { ca: /etc/ssl/ca.pem }\n")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	printed := strings.Join(scenario.DescribeMessaging(spec.Messaging), "\n")
	if strings.Contains(printed, "p4ssw0rd-que-nao-pode-vazar") {
		t.Fatalf("a senha apareceu na saída:\n%s", printed)
	}
	for _, fragment := range []string{"scramSha512", "user ana", "TLS with a private CA"} {
		if !strings.Contains(printed, fragment) {
			t.Fatalf("a saída não diz %q:\n%s", fragment, printed)
		}
	}
}

func TestMSKIsDescribedByRegionAndChainNeverByCredential(t *testing.T) {
	spec, err := parseMessaging(t, "  kafka:\n    auth: { type: mskIam, region: us-east-1 }\n")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	printed := strings.Join(scenario.DescribeMessaging(spec.Messaging), "\n")
	if !strings.Contains(printed, "standard AWS chain") || !strings.Contains(printed, "us-east-1") {
		t.Fatalf("a descricao do msk_iam saiu incompleta:\n%s", printed)
	}
	if !spec.Messaging.Kafka.TLS.Enabled {
		t.Fatal("msk_iam sem TLS: a AWS só aceita a porta 9098 com TLS")
	}
}

// A kafka step with no address in the step, none in the messaging block and an
// HTTP target can never run: `debug` said so at the first iteration and
// `validate` — the cheap gate CI runs — signed it off as valid.
func TestKafkaStepWithoutAnyBrokerIsRefusedByValidation(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8090

load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - name: publicar evento
    kafka: { topic: pedidos, value: "{}" }
`))
	if err != nil {
		t.Fatalf("o cenário não deveria falhar na leitura: %v", err)
	}
	problem := spec.Validate()
	if problem == nil {
		t.Fatal("validação aprovou um passo kafka sem endereço de broker em lugar nenhum")
	}
	for _, expected := range []string{"publicar evento", "messaging", "brokers"} {
		if !strings.Contains(problem.Error(), expected) {
			t.Errorf("a mensagem não ensina o path: falta %q em\n%s", expected, problem)
		}
	}
}

// The target is the address whenever it is not an HTTP one, with or without
// scheme. Refusing here what the run accepts would be worse than not refusing.
func TestBrokerTargetIsEnoughForAKafkaStep(t *testing.T) {
	for _, target := range []string{"kafka://127.0.0.1:9092", "127.0.0.1:9092"} {
		spec, err := scenario.Parse([]byte(`
name: x
target: ` + target + `

load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - kafka: { topic: pedidos, value: "{}" }
`))
		if err != nil {
			t.Fatalf("alvo %q: %v", target, err)
		}
		if problem := spec.Validate(); problem != nil {
			t.Errorf("alvo %q e endereço de broker e foi recusado: %v", target, problem)
		}
	}
}

// Waiting over HTTP polls the target and needs no broker at all.
func TestWaitingOverHTTPNeedsNoBroker(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8090

load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - http: POST /pedidos
    capture: { pedidoId: $.pedido.id }
  - await:
      http: { path: "/pedidos/${pedidoId}" }
      until: { $.status: PROCESSADO }
`))
	if err != nil {
		t.Fatalf("o cenário não deveria falhar na leitura: %v", err)
	}
	if problem := spec.Validate(); problem != nil {
		t.Errorf("aguardar por http foi cobrado por um broker que não usa: %v", problem)
	}
}

// A rule naming a step that does not exist only failed at the end of the run,
// with the whole load already spent.
func TestSLONamingAStepThatDoesNotExistIsRefused(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8090
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - name: consultar produtos
    http: GET /produtos
slo:
  - consultar: { p95: < 200ms }
`))
	if err != nil {
		t.Fatalf("o cenário não deveria falhar na leitura: %v", err)
	}
	problem := spec.Validate()
	if problem == nil {
		t.Fatal("slo apontando para passo inexistente foi aprovado")
	}
	if !strings.Contains(problem.Error(), "consultar produtos") {
		t.Errorf("a mensagem não diz qual passo existe: %v", problem)
	}
}

// An unnamed step reports under its aggregation key, and that is the name a
// rule has to use.
func TestSLOMatchesTheAggregationKeyOfAnUnnamedStep(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8090
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /produtos
slo:
  - GET /produtos: { p95: < 200ms }
`))
	if err != nil {
		t.Fatalf("o cenário não deveria falhar na leitura: %v", err)
	}
	if problem := spec.Validate(); problem != nil {
		t.Fatalf("slo pela chave de agregação foi recusado: %v", problem)
	}
}
