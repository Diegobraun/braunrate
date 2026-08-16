package journey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

const scenarioWithAuthAndData = `
nome: Rotacao de dados
alvo: %s

autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
    captura: { token: $.access_token }

dados:
  assinantes:
    arquivo: assinantes.csv
    consumo: circular

carga:
  perfis:
    - constante: { taxa: 50/s, durante: 1s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
`

// Auth kept the whole context of the first iteration and reinjected it into
// the following ones: the entire load hit the first CSV row while the report
// claimed the data varied.
func TestAuthDoesNotFreezeFirstIterationData(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_, _ = fmt.Fprint(w, `{"access_token":"token-de-teste"}`)
			return
		}
		mu.Lock()
		seen[filepath.Base(r.URL.Path)]++
		mu.Unlock()
		_, _ = fmt.Fprint(w, `{"status":"ABERTO"}`)
	}))
	t.Cleanup(server.Close)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "assinantes.csv"),
		[]byte("id,nome\n1001,ana\n1002,bruno\n1003,carla\n"), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}
	path := filepath.Join(root, "cenario.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(scenarioWithAuthAndData, server.URL)), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	c, err := scenario.ParseFile(path)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	opts := engine.DefaultOptions()
	opts.DataRoot = root
	m, err := engine.New(c, opts)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := m.Execute(context.Background())
	if document.Overall.Errors != 0 {
		t.Fatalf("esperava zero erro, obtive %d", document.Overall.Errors)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, identifier := range []string{"1001", "1002", "1003"} {
		if seen[identifier] == 0 {
			t.Errorf("o assinante %s nunca foi usado; os dados nao rodaram: %v", identifier, seen)
		}
	}
}
