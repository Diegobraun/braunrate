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
nome: x
alvo: kafka.homolog:9093

mensageria:
` + block + `
carga:
  perfis:
    - patamar: { taxa: 1/s, durante: 1s }

cenario:
  - kafka: { topico: pedidos, valor: "{}" }
`))
}

// The scenario goes to the repository. A password written in it is a password
// published, and no phase of this tool is allowed to accept that.
func TestLiteralSecretIsRefusedAndTheMessageTeachesTheWayOut(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{"valor literal", `senha: p4ssw0rd`},
		{"literal entre aspas", `senha: "p4ssw0rd"`},
		{"referencia com valor de reserva", `senha: "${KAFKA_SENHA:-p4ssw0rd}"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMessaging(t, "  kafka:\n    autenticacao: { tipo: scram_sha512, usuario: ana, "+c.password+" }\n")
			if err == nil {
				t.Fatal("o cenario com segredo no arquivo foi aceito")
			}
			for _, fragment := range []string{"${BROKER_SENHA}", "vai para o repositorio", "BROKER_SENHA=..."} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("a mensagem nao ensina %q: %v", fragment, err)
				}
			}
		})
	}
}

func TestEnvironmentReferenceIsAcceptedAndKeepsTheVariableName(t *testing.T) {
	t.Setenv("KAFKA_SENHA", "segredo-do-ambiente")

	spec, err := parseMessaging(t, "  kafka:\n    brokers: [kafka.homolog:9093]\n"+
		"    autenticacao: { tipo: scram_sha512, usuario: ana, senha: \"${KAFKA_SENHA}\" }\n    tls: { ca: /etc/ssl/ca.pem }\n")
	if err != nil {
		t.Fatalf("cenario recusado: %v", err)
	}

	broker := spec.Messaging.Kafka
	if broker.Auth.Kind != messaging.SCRAM512 || broker.Auth.User != "ana" {
		t.Fatalf("autenticacao lida errada: %+v", broker.Auth)
	}
	if broker.Auth.Password != "segredo-do-ambiente" || broker.Auth.PasswordVar != "KAFKA_SENHA" {
		t.Fatalf("a senha nao veio do ambiente com o nome da variavel: %+v", broker.Auth)
	}
	if !broker.TLS.Enabled || broker.TLS.CA != "/etc/ssl/ca.pem" {
		t.Fatalf("tls lido errado: %+v", broker.TLS)
	}
}

// Asking for a key in the scenario is the mistake this refuses to make
// possible, so the key names are refused by name.
func TestAccessKeyIsRefusedByNameAndPointsAtTheCredentialChain(t *testing.T) {
	for _, field := range []string{"chave", "token", "segredo", "access_key", "secret_key"} {
		_, err := parseMessaging(t, "  kafka:\n    autenticacao: { tipo: msk_iam, regiao: us-east-1, "+field+": AKIA123 }\n")
		if err == nil {
			t.Fatalf("o campo %q foi aceito", field)
		}
		if !strings.Contains(err.Error(), "cadeia padrao da AWS") {
			t.Fatalf("o campo %q nao aponta o caminho certo: %v", field, err)
		}
	}
}

func TestMSKWithoutRegionIsRefusedWithTheExample(t *testing.T) {
	_, err := parseMessaging(t, "  kafka:\n    autenticacao: { tipo: msk_iam }\n")
	if err == nil || !strings.Contains(err.Error(), "regiao: us-east-1") {
		t.Fatalf("esperava erro ensinando a regiao, veio: %v", err)
	}
}

func TestRabbitMQRefusesAMechanismItDoesNotHave(t *testing.T) {
	_, err := parseMessaging(t, "  amqp:\n    autenticacao: { tipo: scram_sha512, usuario: ana, senha: \"${SENHA}\" }\n")
	if err == nil || !strings.Contains(err.Error(), "sasl_plain") {
		t.Fatalf("esperava erro dizendo o que o RabbitMQ aceita, veio: %v", err)
	}
}

// Terminal, HTML, JSON and debug all read this one function. If it ever prints
// the secret, it prints it everywhere at once.
func TestWhatMayBePrintedShowsKindAndUserAndNeverTheSecret(t *testing.T) {
	t.Setenv("KAFKA_SENHA", "p4ssw0rd-que-nao-pode-vazar")

	spec, err := parseMessaging(t, "  kafka:\n    brokers: [kafka.homolog:9093]\n"+
		"    autenticacao: { tipo: scram_sha512, usuario: ana, senha: \"${KAFKA_SENHA}\" }\n    tls: { ca: /etc/ssl/ca.pem }\n")
	if err != nil {
		t.Fatalf("cenario recusado: %v", err)
	}

	printed := strings.Join(scenario.DescribeMessaging(spec.Messaging), "\n")
	if strings.Contains(printed, "p4ssw0rd-que-nao-pode-vazar") {
		t.Fatalf("a senha apareceu na saida:\n%s", printed)
	}
	for _, fragment := range []string{"scram_sha512", "usuario ana", "TLS com CA propria"} {
		if !strings.Contains(printed, fragment) {
			t.Fatalf("a saida nao diz %q:\n%s", fragment, printed)
		}
	}
}

func TestMSKIsDescribedByRegionAndChainNeverByCredential(t *testing.T) {
	spec, err := parseMessaging(t, "  kafka:\n    autenticacao: { tipo: msk_iam, regiao: us-east-1 }\n")
	if err != nil {
		t.Fatalf("cenario recusado: %v", err)
	}
	printed := strings.Join(scenario.DescribeMessaging(spec.Messaging), "\n")
	if !strings.Contains(printed, "cadeia padrao da AWS") || !strings.Contains(printed, "us-east-1") {
		t.Fatalf("a descricao do msk_iam saiu incompleta:\n%s", printed)
	}
	if !spec.Messaging.Kafka.TLS.Enabled {
		t.Fatal("msk_iam sem TLS: a AWS so aceita a porta 9098 com TLS")
	}
}
