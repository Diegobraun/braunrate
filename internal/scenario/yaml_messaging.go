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

const brokerExample = "  messaging:\n" +
	"    kafka:\n" +
	"      brokers: [kafka.staging:9093]\n" +
	"      auth: { type: scramSha512, user: \"${KAFKA_USER}\", password: \"${KAFKA_PASSWORD}\" }\n" +
	"      tls: { ca: /path/to/ca.pem }"

func readMessaging(node *yaml.Node) (*messaging.Settings, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "messaging has to be a map by technology, for example:\n"+brokerExample)
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
			return nil, nodeError(key, "unknown technology in messaging: %q\n%s",
				key.Value, suggest(key.Value, []string{"kafka", "amqp"}))
		}
	}
	return settings, nil
}

func readBroker(technology string, node *yaml.Node) (*messaging.Broker, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "%s has to be a map, for example:\n%s", technology, brokerExample)
	}
	broker := &messaging.Broker{Line: node.Line}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "brokers", "addresses":
			if value.Kind != yaml.SequenceNode {
				return nil, nodeError(value, "brokers has to be a list, for example: brokers: [kafka.staging:9093]")
			}
			for _, item := range value.Content {
				broker.Addresses = append(broker.Addresses, ExpandFromEnv(item.Value))
			}
		case "auth":
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
			return nil, nodeError(key, "unknown key in messaging.%s: %q\n%s",
				technology, key.Value, suggest(key.Value, []string{"brokers", "auth", "tls"}))
		}
	}

	if broker.Auth.Kind == messaging.MSKIAM && broker.Auth.Region == "" {
		return nil, nodeError(node, "mskIam needs the region, for example:\n"+
			"    auth: { type: mskIam, region: us-east-1 }")
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
		return auth, nodeError(node, "auth has to be a map, for example:\n"+
			"    auth: { type: scramSha512, user: \"${KAFKA_USER}\", password: \"${KAFKA_PASSWORD}\" }")
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "type":
			kind := messaging.Kind(value.Value)
			if !slices.Contains(messaging.KnownKinds, kind) {
				return auth, nodeError(value, "unknown auth type: %q\n%s\n"+
					"    OAUTHBEARER is not in v1: see the README", value.Value, suggest(value.Value, kindNames()))
			}
			auth.Kind = kind
		case "user":
			auth.User = ExpandFromEnv(value.Value)
		case "password":
			if err := refuseLiteralSecret("password", value); err != nil {
				return auth, err
			}
			auth.PasswordVar, _ = EnvironmentVariable(value.Value)
			auth.Password = ExpandFromEnv(value.Value)
		case "region":
			auth.Region = ExpandFromEnv(value.Value)
		case "key", "token", "secret", "secret_key", "access_key":
			return auth, nodeError(key, "there is no %q here, and there will not be: an access key is never asked for in the scenario.\n"+
				"    For AWS MSK use the standard AWS chain (environment variable, profile or machine role):\n"+
				"      auth: { type: mskIam, region: us-east-1 }", key.Value)
		default:
			return auth, nodeError(key, "unknown key in auth: %q\n%s",
				key.Value, suggest(key.Value, []string{"type", "user", "password", "region"}))
		}
	}

	if auth.Kind == messaging.NoAuth {
		return auth, nodeError(node, "auth with no 'type': declare which one, among %s", strings.Join(kindNames(), ", "))
	}
	return auth, nil
}

func readBrokerTLS(node *yaml.Node) (messaging.TLS, error) {
	settings := messaging.TLS{Enabled: true}
	if node.Kind == yaml.ScalarNode {
		if node.Value != "true" && node.Value != "false" {
			return settings, nodeError(node, "tls takes true, false or a map with 'ca', 'certificate' and 'key'")
		}
		settings.Enabled = node.Value == "true"
		return settings, nil
	}
	if node.Kind != yaml.MappingNode {
		return settings, nodeError(node, "tls has to be true, false or a map, for example: tls: { ca: /path/to/ca.pem }")
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "ca":
			settings.CA = ExpandFromEnv(value.Value)
		case "certificate":
			settings.Certificate = ExpandFromEnv(value.Value)
		case "key":
			settings.Key = ExpandFromEnv(value.Value)
		default:
			return settings, nodeError(key, "unknown key in tls: %q\n%s",
				key.Value, suggest(key.Value, []string{"ca", "certificate", "key"}))
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
	return nodeError(node, "literal %s in the scenario: a credential never goes into the file, because the file goes into the repository.\n"+
		"    replace it with:  %s: ${BROKER_PASSWORD}\n"+
		"    and run with:  BROKER_PASSWORD=... braunrate execute scenario.yaml\n"+
		"    a fallback value (${VAR:-something}) does not work either: the fallback would be the secret written in the file", field, field)
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
		addresses := "the target address"
		if len(pair.broker.Addresses) > 0 {
			addresses = strings.Join(pair.broker.Addresses, ", ")
		}
		lines = append(lines, fmt.Sprintf("%s em %s: %s", pair.name, addresses, pair.broker.Describe()))
	}
	return lines
}
