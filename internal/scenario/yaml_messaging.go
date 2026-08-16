package scenario

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"gopkg.in/yaml.v3"
)

// A secret reaches the scenario only as the name of an environment variable.
// A default value would be the secret written in the file, which is the very
// thing this refuses, so ${VAR:-algo} is rejected together with the literal.
var environmentReference = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

const brokerExample = "  mensageria:\n" +
	"    kafka:\n" +
	"      brokers: [kafka.homolog:9093]\n" +
	"      autenticacao: { tipo: scram_sha512, usuario: \"${KAFKA_USUARIO}\", senha: \"${KAFKA_SENHA}\" }\n" +
	"      tls: { ca: /caminho/ca.pem }"

func readMessaging(node *yaml.Node) (*messaging.Settings, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "mensageria precisa ser um mapa por tecnologia, por exemplo:\n"+brokerExample)
	}
	settings := &messaging.Settings{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		broker, err := readBroker(key.Value, value)
		if err != nil {
			return nil, err
		}
		switch key.Value {
		case "kafka":
			settings.Kafka = broker
		case "amqp":
			settings.AMQP = broker
		default:
			return nil, nodeError(key, "tecnologia desconhecida em mensageria: %q\n%s",
				key.Value, suggest(key.Value, []string{"kafka", "amqp"}))
		}
	}
	return settings, nil
}

func readBroker(technology string, node *yaml.Node) (*messaging.Broker, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "%s precisa ser um mapa, por exemplo:\n%s", technology, brokerExample)
	}
	broker := &messaging.Broker{Line: node.Line}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "brokers", "enderecos":
			if value.Kind != yaml.SequenceNode {
				return nil, nodeError(value, "brokers precisa ser uma lista, por exemplo: brokers: [kafka.homolog:9093]")
			}
			for _, item := range value.Content {
				broker.Addresses = append(broker.Addresses, ExpandFromEnv(item.Value))
			}
		case "autenticacao":
			auth, err := readBrokerAuth(value)
			if err != nil {
				return nil, err
			}
			broker.Auth = auth
		case "tls":
			settings, err := readBrokerTLS(value)
			if err != nil {
				return nil, err
			}
			broker.TLS = settings
		default:
			return nil, nodeError(key, "chave desconhecida em mensageria.%s: %q\n%s",
				technology, key.Value, suggest(key.Value, []string{"brokers", "autenticacao", "tls"}))
		}
	}

	if broker.Auth.Kind == messaging.MSKIAM && broker.Auth.Region == "" {
		return nil, nodeError(node, "msk_iam precisa da região, por exemplo:\n"+
			"    autenticacao: { tipo: msk_iam, regiao: us-east-1 }")
	}
	if broker.Auth.Kind == messaging.MSKIAM {
		broker.TLS.Enabled = true
	}
	if technology == "amqp" {
		if err := broker.SupportsAMQP(); err != nil {
			return nil, nodeError(node, "%v", err)
		}
	}
	return broker, nil
}

func readBrokerAuth(node *yaml.Node) (messaging.Auth, error) {
	auth := messaging.Auth{}
	if node.Kind != yaml.MappingNode {
		return auth, nodeError(node, "autenticação precisa ser um mapa, por exemplo:\n"+
			"    autenticacao: { tipo: scram_sha512, usuario: \"${KAFKA_USUARIO}\", senha: \"${KAFKA_SENHA}\" }")
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "tipo":
			kind := messaging.Kind(value.Value)
			if !slices.Contains(messaging.KnownKinds, kind) {
				return auth, nodeError(value, "tipo de autenticação desconhecido: %q\n%s\n"+
					"    OAUTHBEARER não entra na v1: veja o README", value.Value, suggest(value.Value, kindNames()))
			}
			auth.Kind = kind
		case "usuario":
			auth.User = ExpandFromEnv(value.Value)
		case "senha":
			if err := refuseLiteralSecret("senha", value); err != nil {
				return auth, err
			}
			auth.PasswordVar, _ = EnvironmentVariable(value.Value)
			auth.Password = ExpandFromEnv(value.Value)
		case "regiao":
			auth.Region = ExpandFromEnv(value.Value)
		case "chave", "token", "segredo", "secret_key", "access_key":
			return auth, nodeError(key, "não existe %q aqui, e não vai existir: chave de acesso nunca é pedida no cenário.\n"+
				"    Para AWS MSK use a cadeia padrão da AWS (variável de ambiente, perfil ou role da máquina):\n"+
				"      autenticacao: { tipo: msk_iam, regiao: us-east-1 }", key.Value)
		default:
			return auth, nodeError(key, "chave desconhecida em autenticação: %q\n%s",
				key.Value, suggest(key.Value, []string{"tipo", "usuario", "senha", "regiao"}))
		}
	}

	if auth.Kind == messaging.NoAuth {
		return auth, nodeError(node, "autenticação sem 'tipo': declare qual, entre %s", strings.Join(kindNames(), ", "))
	}
	return auth, nil
}

func readBrokerTLS(node *yaml.Node) (messaging.TLS, error) {
	settings := messaging.TLS{Enabled: true}
	if node.Kind == yaml.ScalarNode {
		if node.Value != "true" && node.Value != "false" {
			return settings, nodeError(node, "tls aceita true, false ou um mapa com 'ca', 'certificado' e 'chave'")
		}
		settings.Enabled = node.Value == "true"
		return settings, nil
	}
	if node.Kind != yaml.MappingNode {
		return settings, nodeError(node, "tls precisa ser true, false ou um mapa, por exemplo: tls: { ca: /caminho/ca.pem }")
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "ca":
			settings.CA = ExpandFromEnv(value.Value)
		case "certificado":
			settings.Certificate = ExpandFromEnv(value.Value)
		case "chave":
			settings.Key = ExpandFromEnv(value.Value)
		default:
			return settings, nodeError(key, "chave desconhecida em tls: %q\n%s",
				key.Value, suggest(key.Value, []string{"ca", "certificado", "chave"}))
		}
	}
	return settings, nil
}

// EnvironmentVariable answers whether the text is only a reference, and gives
// the name back. The DSL asks the same question the YAML parser asks, so both
// audiences refuse a literal secret by the same rule.
func EnvironmentVariable(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !environmentReference.MatchString(trimmed) {
		return "", false
	}
	return strings.Trim(trimmed, "${}"), true
}

func refuseLiteralSecret(field string, node *yaml.Node) error {
	if environmentReference.MatchString(strings.TrimSpace(node.Value)) {
		return nil
	}
	return nodeError(node, "%s literal no cenário: credencial nunca vai para o arquivo, porque o arquivo vai para o repositório.\n"+
		"    troque por:  %s: ${BROKER_SENHA}\n"+
		"    e rode com:  BROKER_SENHA=... braunrate execute cenario.yaml\n"+
		"    valor de reserva (${VAR:-algo}) também não serve: a reserva seria o segredo escrito no arquivo", field, field)
}

func kindNames() []string {
	names := make([]string, 0, len(messaging.KnownKinds))
	for _, kind := range messaging.KnownKinds {
		names = append(names, string(kind))
	}
	return names
}

// DescribeMessaging is what the report is allowed to print about the broker.
func DescribeMessaging(settings *messaging.Settings) []string {
	if settings == nil {
		return nil
	}
	var lines []string
	for _, pair := range []struct {
		name   string
		broker *messaging.Broker
	}{{"kafka", settings.Kafka}, {"amqp", settings.AMQP}} {
		if pair.broker == nil {
			continue
		}
		addresses := "endereço do alvo"
		if len(pair.broker.Addresses) > 0 {
			addresses = strings.Join(pair.broker.Addresses, ", ")
		}
		lines = append(lines, fmt.Sprintf("%s em %s: %s", pair.name, addresses, pair.broker.Describe()))
	}
	return lines
}
