package jornada_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/alvo"
	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/motor"
	_ "github.com/Diegobraun/braunrate/protocolo/graphql"
	"github.com/Diegobraun/braunrate/slo"
)

const cenarioGraphQL = `
nome: Cobranca via GraphQL
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
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { id status ultimaFatura { id status } } }
      variaveis: { id: "${assinantes.id}" }
    verificar:
      json: { $.data.pedido.status: ABERTO }
    captura:
      faturaId: $.data.pedido.ultimaFatura.id

  - graphql:
      consulta: |
        mutation PagarFatura($fatura: ID!) { pagarFatura(id: $fatura) { id status } }
      variaveis: { fatura: "${faturaId}" }
    verificar:
      json: { $.data.pagarFatura.status: PAGA }

slo:
  - graphql ConsultarPedido: { p95: < 2s }
  - global: { erros: < 0.1 }
`

func executarGraphQL(t *testing.T, linhas string) (metrica.Documento, slo.Veredito) {
	t.Helper()
	servidor := alvo.Novo(alvo.Opcoes{Latencia: time.Millisecond})
	if err := servidor.Iniciar("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = servidor.Encerrar() })

	raiz := t.TempDir()
	if err := os.WriteFile(filepath.Join(raiz, "assinantes.csv"), []byte(linhas), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}
	caminho := filepath.Join(raiz, "cenario.yaml")
	if err := os.WriteFile(caminho, []byte(fmt.Sprintf(cenarioGraphQL, servidor.Endereco())), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	c, err := cenario.CarregarArquivo(caminho)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	if err := c.Validar(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	opcoes := motor.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	m, err := motor.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())
	return documento, slo.Avaliar(c.SLO, documento)
}

func TestGraphQLRendeUmaLinhaPorOperacao(t *testing.T) {
	documento, veredito := executarGraphQL(t, "id,nome\n1001,ana\n1002,bruno\n")

	if documento.Global.Erros != 0 {
		t.Fatalf("esperava zero erro, obtive %d: %+v", documento.Global.Erros, documento.Passos)
	}
	nomes := map[string]bool{}
	for _, passo := range documento.Passos {
		nomes[passo.Nome] = true
		if passo.Protocolo != "graphql" {
			t.Errorf("passo %q com protocolo %q", passo.Nome, passo.Protocolo)
		}
	}
	if !nomes["graphql ConsultarPedido"] || !nomes["graphql PagarFatura"] {
		t.Fatalf("o relatorio precisa de uma linha por operacao, obtive %v", nomes)
	}
	if len(documento.Passos) != 2 {
		t.Errorf("consulta e mutation nao podem cair na mesma linha: %d passo(s)", len(documento.Passos))
	}
	if !veredito.Passou {
		t.Errorf("SLO deveria passar: %s", veredito.Frase)
	}
}

// O alvo devolve NOT_FOUND com status 200 para identificador terminado em 7,
// que e como o erro de GraphQL chega em producao.
func TestErroDeGraphQLComStatus200DerrubaOSLO(t *testing.T) {
	documento, veredito := executarGraphQL(t, "id,nome\n1007,ana\n")

	consulta := documento.Passos[0]
	if consulta.ErrosPorClasse["graphql"] == 0 {
		t.Fatalf("erro de graphql nao foi contado: %+v", consulta.ErrosPorClasse)
	}
	if consulta.StatusPorCodigo["200"] == 0 {
		t.Errorf("o status HTTP era 200 mesmo: %+v", consulta.StatusPorCodigo)
	}
	if veredito.Passou {
		t.Error("execucao com 100% de erro de graphql nao pode passar no SLO")
	}
	if len(documento.Passos) != 1 {
		t.Errorf("a mutation nao deveria rodar depois do erro: %d passo(s)", len(documento.Passos))
	}
}
