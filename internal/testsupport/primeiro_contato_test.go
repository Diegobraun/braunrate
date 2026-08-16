package testsupport_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/importer"
	protocolhttp "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

// Enquanto /pedidos exigiu token, a sequencia 'new', 'target', 'execute' —
// o primeiro contato de quem chega — devolvia 401 em toda requisicao.
func TestTheScenarioNewWritesAnswersOnTheEmbeddedTarget(t *testing.T) {
	specification, err := scenario.Parse([]byte(importer.Skeleton()))
	if err != nil {
		t.Fatalf("o esqueleto do 'new' não passa no parser: %v", err)
	}
	first, ok := specification.Steps[0].Config.(*protocolhttp.Config)
	if !ok {
		t.Fatalf("o primeiro passo do esqueleto não é HTTP: %+v", specification.Steps[0].Config)
	}

	server := testsupport.New(testsupport.Options{Latency: time.Millisecond})
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo não subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(server.Address() + first.Path)
	if err != nil {
		t.Fatalf("não consegui pedir %s: %v", first.Path, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("%s respondeu %d sem credencial: %s\n"+
			"o cenário que o 'braunrate new' escreve não declara autenticação",
			first.Path, response.StatusCode, body)
	}
}
