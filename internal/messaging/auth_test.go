package messaging

import (
	"strings"
	"testing"
)

// Authentication and authorization are fixed in different places: one is an
// environment variable, the other is an ACL in the broker. Reporting both as a
// network problem sends the person to look at the firewall.
func TestCredentialErrorsAreToldApartFromEachOtherAndFromTheNetwork(t *testing.T) {
	cases := []struct {
		message string
		kind    string
		is      bool
	}{
		{"SASL Authentication failed", "autenticacao", true},
		{"[58] SASL Authentication Failed: invalid credentials", "autenticacao", true},
		{"Unsupported SASL mechanism", "autenticacao", true},
		{"[29] Topic Authorization Failed", "autorizacao", true},
		{"[30] Group Authorization Failed", "autorizacao", true},
		{"ACCESS_REFUSED - Login was refused", "autenticacao", true},
		{"dial tcp 10.0.0.1:9093: connect: connection refused", "", false},
		{"context deadline exceeded", "", false},
	}

	for _, c := range cases {
		kind, credential := ClassifyError(errorOf(c.message))
		if credential != c.is || kind != c.kind {
			t.Fatalf("%q virou (%q, %v), esperava (%q, %v)", c.message, kind, credential, c.kind, c.is)
		}
	}
}

type textError string

func (err textError) Error() string { return string(err) }

func errorOf(text string) error { return textError(text) }

func TestExplanationSaysWhereToFixItWithoutShowingTheSecret(t *testing.T) {
	broker := &Broker{Auth: Auth{Kind: SCRAM512, User: "ana", Password: "p4ssw0rd-secreta", PasswordVar: "KAFKA_SENHA"}}

	for _, kind := range []string{"autenticacao", "autorizacao"} {
		explanation := Explain(kind, broker)
		if strings.Contains(explanation, "p4ssw0rd-secreta") {
			t.Fatalf("a senha vazou na explicacao de %s: %s", kind, explanation)
		}
		if !strings.Contains(explanation, "usuario ana") {
			t.Fatalf("a explicacao de %s nao diz o usuario: %s", kind, explanation)
		}
	}
	if !strings.Contains(Explain("autorizacao", broker), "ACL") {
		t.Fatal("a autorizacao nao aponta a ACL, que e onde se resolve")
	}
}

// The signer is exercised without AWS: what is verified here is that no key was
// ever asked for, and that the region reaches the mechanism.
func TestMSKMechanismCarriesTheRegionAndAsksForNoKey(t *testing.T) {
	broker := &Broker{Auth: Auth{Kind: MSKIAM, Region: "us-east-1"}, TLS: TLS{Enabled: true}}

	mechanism, err := broker.mechanism()
	if err != nil {
		t.Fatalf("mecanismo nao montou: %v", err)
	}
	if mechanism.Name() != "AWS_MSK_IAM" {
		t.Fatalf("nome do mecanismo: %q", mechanism.Name())
	}
	signer, isMSK := mechanism.(mskMechanism)
	if !isMSK || signer.region != "us-east-1" {
		t.Fatalf("a regiao nao chegou ao assinador: %+v", mechanism)
	}
	if broker.Auth.Password != "" || broker.Auth.PasswordVar != "" {
		t.Fatal("msk_iam guardou credencial, e ele nunca deve pedir uma")
	}
}

// A variable that is not set turns into an empty password, and the broker
// refuses it without saying which variable was missing.
func TestUnsetVariableIsReportedByNameInsteadOfBecomingAnEmptyPassword(t *testing.T) {
	broker := &Broker{Auth: Auth{Kind: SCRAM512, User: "ana", PasswordVar: "KAFKA_SENHA"}}

	_, err := broker.mechanism()
	if err == nil {
		t.Fatal("senha vazia passou como se fosse credencial")
	}
	if !strings.Contains(err.Error(), "KAFKA_SENHA") {
		t.Fatalf("o erro nao diz qual variavel falta: %v", err)
	}
}

func TestTransportIsNilWhenThereIsNothingToSecure(t *testing.T) {
	transport, err := (*Broker)(nil).Transport()
	if err != nil || transport != nil {
		t.Fatalf("broker sem autenticacao montou transporte: %v, %v", transport, err)
	}
	plain := &Broker{Addresses: []string{"127.0.0.1:9092"}}
	transport, err = plain.Transport()
	if err != nil || transport != nil {
		t.Fatalf("broker local montou transporte: %v, %v", transport, err)
	}
}

// An amqp URI carries the password in the userinfo, and that address is printed
// in the terminal, the HTML and the JSON.
func TestAMQPAddressLosesThePasswordBeforeBeingPrinted(t *testing.T) {
	safe := SafeAddress("amqp://ana:p4ssw0rd@rabbit.homolog:5672/producao")

	if strings.Contains(safe, "p4ssw0rd") {
		t.Fatalf("a senha ficou no endereco: %s", safe)
	}
	if !strings.Contains(safe, "ana") || !strings.Contains(safe, "rabbit.homolog:5672") {
		t.Fatalf("o endereco perdeu o que serve para ler: %s", safe)
	}
}
