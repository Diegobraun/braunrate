package transport_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
)

// Kafka and AMQP could declare a private CA and HTTP could not, which is the
// protocol every scenario starts from. A corporate homologation behind its own
// CA could not be measured at all.
func TestClientReachesATargetWhoseCAOnlyTheScenarioKnows(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()

	semCA := transport.NewClient(protocol.Options{})
	if _, err := semCA.Get(server.URL); err == nil {
		t.Fatal("o cliente sem CA aceitou um certificado que nao tem por que confiar")
	}

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	comCA := transport.NewClient(protocol.Options{TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}})
	response, err := comCA.Get(server.URL)
	if err != nil {
		t.Fatalf("o cliente com a CA declarada nao alcancou o alvo: %v", err)
	}
	_ = response.Body.Close()
}

// The raw x509 text is long, ends in the part that matters, and said nothing
// about the way out.
func TestTLSFailureSaysWhatToDeclare(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()

	_, err := transport.NewClient(protocol.Options{}).Get(server.URL)
	if err == nil {
		t.Fatal("esperava falha de certificado")
	}
	summary := transport.SummarizeError(err)
	for _, expected := range []string{"CA", "tls:", "ca:"} {
		if !strings.Contains(summary, expected) {
			t.Errorf("a mensagem nao ensina a saida: falta %q em %q", expected, summary)
		}
	}
}
