package messaging

import (
	"context"
	"fmt"
	"strings"
	"time"

	signer "github.com/aws/aws-msk-iam-sasl-signer-go/signer"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// Transport is built once and shared: the TLS and SASL handshake happens when
// the connection opens, and opening one per message would put the handshake
// inside the latency of the message.
func (broker *Broker) Transport() (*kafka.Transport, error) {
	if broker == nil || !broker.Secured() {
		return nil, nil
	}
	mechanism, err := broker.mechanism()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := broker.TLSConfig()
	if err != nil {
		return nil, err
	}
	return &kafka.Transport{SASL: mechanism, TLS: tlsConfig, DialTimeout: 10 * time.Second}, nil
}

func (broker *Broker) Dialer() (*kafka.Dialer, error) {
	if broker == nil || !broker.Secured() {
		return nil, nil
	}
	mechanism, err := broker.mechanism()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := broker.TLSConfig()
	if err != nil {
		return nil, err
	}
	return &kafka.Dialer{
		Timeout:       10 * time.Second,
		DualStack:     true,
		SASLMechanism: mechanism,
		TLS:           tlsConfig,
	}, nil
}

func (broker *Broker) mechanism() (sasl.Mechanism, error) {
	switch broker.Auth.Kind {
	case NoAuth, External:
		return nil, nil
	case Plain:
		if err := broker.credentialPresent(); err != nil {
			return nil, err
		}
		return plain.Mechanism{Username: broker.Auth.User, Password: broker.Auth.Password}, nil
	case SCRAM256:
		if err := broker.credentialPresent(); err != nil {
			return nil, err
		}
		return scram.Mechanism(scram.SHA256, broker.Auth.User, broker.Auth.Password)
	case SCRAM512:
		if err := broker.credentialPresent(); err != nil {
			return nil, err
		}
		return scram.Mechanism(scram.SHA512, broker.Auth.User, broker.Auth.Password)
	case MSKIAM:
		return mskMechanism{region: broker.Auth.Region}, nil
	}
	return nil, fmt.Errorf("unknown auth type: %q", broker.Auth.Kind)
}

// An empty password is not a password the broker will explain: the scenario
// declared a variable and the variable is not in the environment.
func (broker *Broker) credentialPresent() error {
	if broker.Auth.Password != "" {
		return nil
	}
	if broker.Auth.PasswordVar != "" {
		return fmt.Errorf("the environment variable %s is not set, so the broker password came out empty: run with %s=... in the environment",
			broker.Auth.PasswordVar, broker.Auth.PasswordVar)
	}
	return fmt.Errorf("%s auth with no declared password", broker.Auth.Kind)
}

// The signature is short-lived and the signer refreshes it from the AWS default
// chain, which is why no key is ever asked for in the scenario.
type mskMechanism struct{ region string }

func (mechanism mskMechanism) Name() string { return "AWS_MSK_IAM" }

func (mechanism mskMechanism) Start(runContext context.Context) (sasl.StateMachine, []byte, error) {
	payload, _, err := signer.GenerateAuthToken(runContext, mechanism.region)
	if err != nil {
		return nil, nil, fmt.Errorf("could not sign with IAM in region %s: %w", mechanism.region, err)
	}
	return mskSession{}, []byte(payload), nil
}

type mskSession struct{}

func (mskSession) Next(_ context.Context, _ []byte) (bool, []byte, error) { return true, nil, nil }

// Authentication and authorization are different problems with different
// owners: the first is a wrong credential, the second is a credential without
// permission on that topic. Reporting both as "broker indisponivel" sends the
// person to look at the network.
func ClassifyError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	text := strings.ToLower(err.Error())
	switch {
	// ACCESS_REFUSED comes from RabbitMQ in both roles: with "login" it is a
	// wrong credential, on a resource it is a missing permission.
	case strings.Contains(text, "sasl authentication failed"),
		strings.Contains(text, "authentication failed"),
		strings.Contains(text, "invalid credentials"),
		strings.Contains(text, "sasl_authentication_failed"),
		strings.Contains(text, "unsupported sasl mechanism"),
		strings.Contains(text, "login was refused"),
		strings.Contains(text, "login refused"):
		return "authentication", true
	case strings.Contains(text, "topic authorization failed"),
		strings.Contains(text, "group authorization failed"),
		strings.Contains(text, "cluster authorization failed"),
		strings.Contains(text, "not authorized"),
		strings.Contains(text, "access refused"),
		strings.Contains(text, "access_refused"):
		return "authorization", true
	}
	return "", false
}

// Explain turns the broker error into what to do about it. Without this the
// person reads "EOF" and goes looking at the network.
func Explain(kind string, broker *Broker) string {
	switch kind {
	case "authentication":
		return fmt.Sprintf("the broker refused the credential (%s): check the user and the environment variable holding the password", broker.Describe())
	case "authorization":
		return fmt.Sprintf("the credential was accepted and has no permission on that topic or group (%s): this is an ACL matter on the broker, not a wrong password", broker.Describe())
	}
	return ""
}
