package wait_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/wait"
)

func configuracaoHTTP(t *testing.T, corpo string) protocol.Configuracao {
	t.Helper()
	configuracao, err := decodificar(t, corpo)
	if err != nil {
		t.Fatalf("cenario nao decodificou: %v", err)
	}
	return configuracao
}

// Muito sistema assincrono so mostra o efeito por API: sem sondagem, a cadeia
// ponta a ponta nao se mede neles.
func TestEsperaPorHTTPAteOEfeitoAparecer(t *testing.T) {
	var chamadas atomic.Int64
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if chamadas.Add(1) < 3 {
			fmt.Fprint(w, `{"status":"PENDENTE"}`)
			return
		}
		fmt.Fprint(w, `{"status":"PROCESSADO"}`)
	}))
	t.Cleanup(servidor.Close)

	configuracao := configuracaoHTTP(t, `
http: { caminho: /pedidos/1 }
ate: { $.status: PROCESSADO }
intervalo: 20ms
timeout: 2s
`)

	inicio := time.Now()
	resposta := wait.Novo(protocol.OpcoesPadrao()).Executar(context.Background(), protocol.Requisicao{
		URLBase: servidor.URL, Configuracao: configuracao,
	})
	decorrido := time.Since(inicio)

	if resposta.Classe != protocol.Sucesso {
		t.Fatalf("classe = %q, detalhe = %q", resposta.Classe, resposta.Detalhe)
	}
	if chamadas.Load() < 3 {
		t.Errorf("sondou %d vezes; a espera terminou antes do efeito", chamadas.Load())
	}
	if decorrido < 40*time.Millisecond {
		t.Errorf("a espera levou %s: o tempo ate o efeito precisa entrar na medicao", decorrido)
	}
}

func TestEsperaPorHTTPQueEstouraDizOQueViuEQuantoSondou(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"PENDENTE"}`)
	}))
	t.Cleanup(servidor.Close)

	configuracao := configuracaoHTTP(t, `
http: { caminho: /pedidos/1 }
ate: { $.status: PROCESSADO }
intervalo: 20ms
timeout: 120ms
`)

	resposta := wait.Novo(protocol.OpcoesPadrao()).Executar(context.Background(), protocol.Requisicao{
		URLBase: servidor.URL, Configuracao: configuracao,
	})

	if resposta.Classe != protocol.ErroDeTimeout {
		t.Fatalf("classe = %q", resposta.Classe)
	}
	for _, esperado := range []string{"PENDENTE", "sondagens", "$.status"} {
		if !strings.Contains(resposta.Detalhe, esperado) {
			t.Errorf("o detalhe precisa conter %q para a pessoa saber o que aconteceu: %q", esperado, resposta.Detalhe)
		}
	}
}

// Sondar sem condicao mediria o tempo de responder, e nao o tempo ate o efeito.
func TestEsperaPorHTTPSemCondicaoEhRecusadaComExplicacao(t *testing.T) {
	_, err := decodificar(t, "http: { caminho: /pedidos/1 }\n")
	if err == nil {
		t.Fatal("aguardar por http sem 'ate' precisa ser recusado")
	}
	if !strings.Contains(err.Error(), "ate") || !strings.Contains(err.Error(), "efeito") {
		t.Errorf("a mensagem precisa ensinar a forma certa: %v", err)
	}
}
