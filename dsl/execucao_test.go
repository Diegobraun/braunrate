package dsl_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/dsl"
	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

const gemeoEmYAML = `
nome: Consulta de pedido
alvo: %s

carga:
  perfis:
    - constante: { taxa: 100/s, durante: 1s }

cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }

slo:
  - consultar pedido: { p95: < 1s }
`

// A equivalencia estrutural ja esta travada; este teste fecha o outro lado da
// promessa: o cenario escrito em Go roda no mesmo motor e sai com as mesmas
// chaves de agregacao e o mesmo veredito que o YAML gemeo.
func TestCenarioEmGoRodaComOMesmoMotorEAsMesmasChaves(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ABERTO"}`)
	}))
	t.Cleanup(servidor.Close)

	doYAML, err := scenario.Carregar([]byte(fmt.Sprintf(gemeoEmYAML, servidor.URL)))
	if err != nil {
		t.Fatalf("yaml nao carregou: %v", err)
	}
	daDSL, err := dsl.Novo("Consulta de pedido").
		Alvo(servidor.URL).
		Constante(dsl.PorSegundo(100), time.Second).
		Passo(dsl.GET("/pedidos/1"), dsl.Nome("consultar pedido"), dsl.VerificarStatus(200)).
		SLO("consultar pedido", "p95", "< 1s").
		Construir()
	if err != nil {
		t.Fatalf("dsl nao montou: %v", err)
	}

	pelaYAML := executar(t, doYAML)
	pelaDSL := executar(t, daDSL)

	if len(pelaYAML.Passos) != len(pelaDSL.Passos) {
		t.Fatalf("quantidade de passos: yaml %d, dsl %d", len(pelaYAML.Passos), len(pelaDSL.Passos))
	}
	for indice := range pelaYAML.Passos {
		if pelaYAML.Passos[indice].Chave != pelaDSL.Passos[indice].Chave {
			t.Errorf("chave de agregacao: yaml %q, dsl %q", pelaYAML.Passos[indice].Chave, pelaDSL.Passos[indice].Chave)
		}
	}
	if pelaYAML.SLO.Passou != pelaDSL.SLO.Passou {
		t.Errorf("veredito de slo: yaml %v, dsl %v", pelaYAML.SLO.Passou, pelaDSL.SLO.Passou)
	}
	if pelaDSL.Global.Contagem == 0 {
		t.Error("o cenario em Go nao executou nenhuma requisicao")
	}
}

func executar(t *testing.T, c scenario.Cenario) metrics.Documento {
	t.Helper()
	opcoes := engine.OpcoesPadrao()
	opcoes.RaizDeDados = t.TempDir()
	m, err := engine.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	return m.Executar(context.Background())
}
