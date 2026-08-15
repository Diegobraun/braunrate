package jornada_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/motor"
)

const cenarioComAutenticacaoEDados = `
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

// A autenticacao guardava o contexto inteiro da primeira iteracao e o
// reinjetava nas seguintes: toda a carga caia sobre a primeira linha do CSV
// enquanto o relatorio afirmava que os dados variavam.
func TestAutenticacaoNaoCongelaOsDadosDaPrimeiraIteracao(t *testing.T) {
	var mutex sync.Mutex
	vistos := map[string]int{}

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			fmt.Fprint(w, `{"access_token":"token-de-teste"}`)
			return
		}
		mutex.Lock()
		vistos[filepath.Base(r.URL.Path)]++
		mutex.Unlock()
		fmt.Fprint(w, `{"status":"ABERTO"}`)
	}))
	t.Cleanup(servidor.Close)

	raiz := t.TempDir()
	if err := os.WriteFile(filepath.Join(raiz, "assinantes.csv"),
		[]byte("id,nome\n1001,ana\n1002,bruno\n1003,carla\n"), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}
	caminho := filepath.Join(raiz, "cenario.yaml")
	if err := os.WriteFile(caminho, []byte(fmt.Sprintf(cenarioComAutenticacaoEDados, servidor.URL)), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	c, err := cenario.CarregarArquivo(caminho)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	opcoes := motor.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	m, err := motor.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())
	if documento.Global.Erros != 0 {
		t.Fatalf("esperava zero erro, obtive %d", documento.Global.Erros)
	}

	mutex.Lock()
	defer mutex.Unlock()
	for _, identificador := range []string{"1001", "1002", "1003"} {
		if vistos[identificador] == 0 {
			t.Errorf("o assinante %s nunca foi usado; os dados nao rodaram: %v", identificador, vistos)
		}
	}
}
