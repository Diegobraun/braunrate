package testsupport_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/testsupport"
)

// O alvo minimo nao interpreta a requisicao, entao a unica coisa que pode estar
// errada nele e o quadro da resposta: um Content-Length que nao bate faz o
// cliente fechar a conexao a cada chamada, e ai o alvo barato vira o mais caro
// de todos — uma conexao nova por requisicao.
func TestRawTargetAnswersHTTPAndKeepsTheConnection(t *testing.T) {
	server := testsupport.NewRaw()
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo minimo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	client := &http.Client{Timeout: 5 * time.Second}
	address := "http://" + server.Address()

	for attempt := range 20 {
		response, err := client.Get(fmt.Sprintf("%s/pedidos/%d", address, attempt))
		if err != nil {
			t.Fatalf("chamada %d falhou: %v", attempt, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("nao consegui ler o corpo: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("chamada %d veio com status %d", attempt, response.StatusCode)
		}
		if string(body) != `{"id":1,"status":"OK"}` {
			t.Fatalf("corpo inesperado: %q", body)
		}
	}

	if served := server.Served(); served != 20 {
		t.Fatalf("o alvo contou %d chamadas e foram 20", served)
	}
}

// Se ele respondesse por leitura em vez de por requisicao, duas requisicoes que
// chegassem no mesmo pacote receberiam uma resposta so — e o cliente ficaria
// esperando a segunda para sempre.
func TestRawTargetAnswersOncePerRequestEvenWhenTheyArriveTogether(t *testing.T) {
	server := testsupport.NewRaw()
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo minimo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	transport := &http.Transport{MaxIdleConnsPerHost: 1}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	address := "http://" + server.Address() + "/x"

	for range 5 {
		response, err := client.Get(address)
		if err != nil {
			t.Fatalf("chamada falhou: %v", err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	if served := server.Served(); served != 5 {
		t.Fatalf("o alvo respondeu %d vezes para 5 requisicoes", served)
	}
}
