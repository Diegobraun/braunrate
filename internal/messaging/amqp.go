package messaging

import (
	"fmt"
	"net/url"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DialAMQP keeps the credential out of the address that gets printed: user and
// password go in the connection configuration, never glued into the URI that
// ends up in a log line.
func (broker *Broker) DialAMQP(address string) (*amqp.Connection, error) {
	config := amqp.Config{Properties: amqp.NewConnectionProperties()}
	config.Properties.SetClientConnectionName("braunrate")

	if broker != nil && broker.Auth.Kind == Plain {
		config.SASL = []amqp.Authentication{&amqp.PlainAuth{
			Username: broker.Auth.User, Password: broker.Auth.Password,
		}}
	}
	if broker != nil && broker.Auth.Kind == External {
		config.SASL = []amqp.Authentication{&amqp.ExternalAuth{}}
	}

	tlsConfig, err := broker.TLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		config.TLSClientConfig = tlsConfig
		address = strings.Replace(address, "amqp://", "amqps://", 1)
	}
	return amqp.DialConfig(address, config)
}

// SafeAddress is what may be printed. An amqp URI carries the password in the
// userinfo, and that address travels to the terminal, the HTML and the JSON.
func SafeAddress(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.User == nil {
		return address
	}
	user := parsed.User.Username()
	parsed.User = url.User(user)
	return parsed.String()
}

func (broker *Broker) SupportsAMQP() error {
	if broker == nil || broker.Auth.Kind == NoAuth || broker.Auth.Kind == Plain || broker.Auth.Kind == External {
		return nil
	}
	return fmt.Errorf("o RabbitMQ não usa %q: os tipos disponíveis são sasl_plain (usuário e senha) e certificado (mTLS)", broker.Auth.Kind)
}
