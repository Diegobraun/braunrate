// Package messaging holds how braunrate reaches a broker that is not the one on
// localhost: SASL, TLS and the AWS credential chain. It is one place on purpose
// — the producer step, the consumer of the wait step and the report all have to
// agree on what the connection is, and on what may be printed about it.
package messaging

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

type Kind string

const (
	NoAuth   Kind = ""
	Plain    Kind = "saslPlain"
	SCRAM256 Kind = "scramSha256"
	SCRAM512 Kind = "scramSha512"
	MSKIAM   Kind = "mskIam"
	External Kind = "certificate"
)

// KnownKinds is the closed list: an unknown name would be declared, printed and
// never checked by anyone.
var KnownKinds = []Kind{Plain, SCRAM256, SCRAM512, MSKIAM, External}

type Auth struct {
	Kind Kind
	User string
	// UserVar e PasswordVar guardam o nome da variavel de ambiente que o cenario
	// declarou, para uma variavel ausente ser reportada pelo nome em vez de virar
	// um campo vazio que o broker recusa sem explicar. O nome tambem separa a
	// credencial de broker — que se resolve na leitura do arquivo, do ambiente —
	// do ${TOKEN} de HTTP, que a interface pode preencher na sessao (ADR 0021).
	UserVar     string
	PasswordVar string
	Password    string
	Region      string
}

type TLS struct {
	Enabled     bool
	CA          string
	Certificate string
	Key         string
}

type Broker struct {
	Addresses []string
	Auth      Auth
	TLS       TLS
	Line      int
}

// Settings is the `messaging` block. A nil pointer means the scenario talks to
// a broker with no credentials, which stays the default.
type Settings struct {
	Kafka *Broker
	AMQP  *Broker
}

func (settings *Settings) BrokerFor(protocolName string) *Broker {
	if settings == nil {
		return nil
	}
	switch protocolName {
	case "kafka":
		return settings.Kafka
	case "amqp":
		return settings.AMQP
	}
	return nil
}

func (broker *Broker) Secured() bool {
	return broker != nil && (broker.Auth.Kind != NoAuth || broker.TLS.Enabled)
}

// Describe is what may appear on a screen, in the HTML and in the JSON: kind of
// authentication and user, never the secret. There is no code path that prints
// a broker any other way.
func (broker *Broker) Describe() string {
	if broker == nil || !broker.Secured() {
		return "no authentication"
	}
	var parts []string
	switch broker.Auth.Kind {
	case NoAuth:
	case MSKIAM:
		parts = append(parts, fmt.Sprintf("mskIam (region %s, credential from the standard AWS chain)", broker.Auth.Region))
	case External:
		parts = append(parts, "client certificate")
	default:
		user := broker.Auth.User
		if user == "" {
			user = "no user"
		}
		parts = append(parts, fmt.Sprintf("%s, user %s", broker.Auth.Kind, user))
	}
	if broker.TLS.Enabled {
		tlsPart := "TLS"
		if broker.TLS.CA != "" {
			tlsPart += " with a private CA"
		}
		if broker.TLS.Certificate != "" {
			tlsPart += " and a client certificate"
		}
		parts = append(parts, tlsPart)
	}
	return strings.Join(parts, " + ")
}

func (broker *Broker) TLSConfig() (*tls.Config, error) {
	if broker == nil {
		return nil, nil
	}
	return broker.TLS.Config()
}

// Config is the same for a broker and for the HTTP target: one way to declare a
// private CA and a client certificate, one way to read them.
func (settings TLS) Config() (*tls.Config, error) {
	if !settings.Enabled {
		return nil, nil
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}

	if settings.CA != "" {
		pem, err := os.ReadFile(settings.CA)
		if err != nil {
			return nil, fmt.Errorf("could not read the CA at %s: %w", settings.CA, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("the file %s has no valid PEM certificate", settings.CA)
		}
		config.RootCAs = pool
	}

	if settings.Certificate != "" || settings.Key != "" {
		if settings.Certificate == "" || settings.Key == "" {
			return nil, fmt.Errorf("a client certificate needs both files: 'certificate' and 'key'")
		}
		pair, err := tls.LoadX509KeyPair(settings.Certificate, settings.Key)
		if err != nil {
			return nil, fmt.Errorf("could not load the certificate/key pair: %w", err)
		}
		config.Certificates = []tls.Certificate{pair}
	}
	return config, nil
}
