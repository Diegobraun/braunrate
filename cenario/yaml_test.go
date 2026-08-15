package cenario_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/cenario"
	_ "github.com/Diegobraun/braunrate/protocolo/aguardar"
	_ "github.com/Diegobraun/braunrate/protocolo/amqp"
	_ "github.com/Diegobraun/braunrate/protocolo/graphql"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
	_ "github.com/Diegobraun/braunrate/protocolo/kafka"
)

const cenarioMinimo = `
nome: Jornada de cobranca
alvo: http://127.0.0.1:8080

carga:
  modelo: aberto
  perfis:
    - rampa: { de: 1/s, ate: 50/s, durante: 1m }
    - patamar: { taxa: 50/s, durante: 5m }

cenario:
  - http: GET /assinaturas/1
    nome: consultar assinatura
    verificar: { status: 200 }
`

func TestCarregarCenarioMinimo(t *testing.T) {
	c, err := cenario.Carregar([]byte(cenarioMinimo))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Nome != "Jornada de cobranca" {
		t.Errorf("nome = %q", c.Nome)
	}
	if c.Carga.Modelo != cenario.ChegadaAberta {
		t.Errorf("modelo = %q, esperado aberto por padrao", c.Carga.Modelo)
	}
	if len(c.Carga.Fases) != 2 {
		t.Fatalf("fases = %d, esperado 2", len(c.Carga.Fases))
	}
	if c.Carga.Fases[0].De != 1 || c.Carga.Fases[0].Ate != 50 || c.Carga.Fases[0].Durante != time.Minute {
		t.Errorf("rampa lida errado: %+v", c.Carga.Fases[0])
	}
	if len(c.Passos) != 1 {
		t.Fatalf("passos = %d", len(c.Passos))
	}
	passo := c.Passos[0]
	if passo.Nome != "consultar assinatura" || passo.Protocolo != "http" {
		t.Errorf("passo lido errado: %+v", passo)
	}
	if passo.ChaveDeAgregacao() != "GET /assinaturas/1" {
		t.Errorf("chave de agregacao = %q", passo.ChaveDeAgregacao())
	}
	if len(passo.Verificacoes) != 1 || passo.Verificacoes[0].Status != 200 {
		t.Errorf("verificacoes lidas errado: %+v", passo.Verificacoes)
	}
	if err := c.Validar(); err != nil {
		t.Errorf("cenario deveria ser valido: %v", err)
	}
}

func TestFormaLongaDoPassoHTTP(t *testing.T) {
	entrada := `
nome: com corpo
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - constante: { taxa: 10/s, durante: 1s }
cenario:
  - nome: criar fatura
    http:
      metodo: POST
      caminho: /faturas
      cabecalhos: { X-Tenant: acme }
      corpo: { valor: 199.90 }
`
	c, err := cenario.Carregar([]byte(entrada))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Passos[0].ChaveDeAgregacao() != "POST /faturas" {
		t.Errorf("chave = %q", c.Passos[0].ChaveDeAgregacao())
	}
}

func TestErroApontaLinha(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		trecho  string
		linha   int
	}{
		{
			nome:    "protocolo desconhecido",
			entrada: "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - constante: { taxa: 1/s, durante: 1s }\ncenario:\n  - grpc: /servico\n",
			trecho:  "nao reconheco",
			linha:   7,
		},
		{
			nome:    "taxa invalida",
			entrada: "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - constante: { taxa: rapido, durante: 1s }\ncenario:\n  - http: GET /\n",
			trecho:  "taxa invalida",
			linha:   5,
		},
		{
			nome:    "chave desconhecida no topo",
			entrada: "nome: x\nalvo: http://a\nturbo: sim\n",
			trecho:  "chave desconhecida",
			linha:   3,
		},
		{
			nome:    "perfil desconhecido",
			entrada: "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - montanha: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n",
			trecho:  "tipo de perfil desconhecido",
			linha:   5,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			_, err := cenario.Carregar([]byte(caso.entrada))
			if err == nil {
				t.Fatal("esperava erro")
			}
			erro, ok := err.(cenario.ErroDeCenario)
			if !ok {
				t.Fatalf("esperava ErroDeCenario, recebeu %T: %v", err, err)
			}
			if erro.Linha != caso.linha {
				t.Errorf("linha = %d, esperado %d (%v)", erro.Linha, caso.linha, erro)
			}
			if !strings.Contains(erro.Mensagem, caso.trecho) {
				t.Errorf("mensagem = %q, esperava conter %q", erro.Mensagem, caso.trecho)
			}
		})
	}
}

func TestRecursoDeFaseFuturaFalhaComMensagemQueEnsina(t *testing.T) {
	entrada := "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - constante: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n    peso: 3\n"
	_, err := cenario.Carregar([]byte(entrada))
	if err == nil || !strings.Contains(err.Error(), "entra junto com o GraphQL") {
		t.Fatalf("esperava mensagem explicando quando o recurso chega, recebeu %v", err)
	}
}

func TestChaveDeAgregacaoNaoCarregaValorInterpolado(t *testing.T) {
	entrada := `
nome: agregacao
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - constante: { taxa: 1/s, durante: 1s }
cenario:
  - http: GET /pedidos/${pedidoId}
    captura: { proximo: $.proximo.id }
`
	c, err := cenario.Carregar([]byte(entrada))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if chave := c.Passos[0].ChaveDeAgregacao(); chave != "GET /pedidos/${pedidoId}" {
		t.Errorf("chave = %q; o relatorio agrega pela rota declarada, nunca pela URL com o valor dentro", chave)
	}
	if len(c.Passos[0].Capturas) != 1 || c.Passos[0].Capturas[0].Origem != cenario.CapturaDeJSON {
		t.Errorf("captura lida errado: %+v", c.Passos[0].Capturas)
	}
}

func TestVariavelUsaAmbienteEPadrao(t *testing.T) {
	t.Setenv("TENANT_DE_TESTE", "acme")
	entrada := `
nome: variaveis
alvo: http://127.0.0.1:8080
variaveis:
  tenant: ${TENANT_DE_TESTE:-padrao}
  regiao: ${REGIAO_INEXISTENTE_NO_AMBIENTE:-sul}
carga:
  perfis:
    - constante: { taxa: 1/s, durante: 1s }
cenario:
  - http: GET /clientes/${tenant}/${regiao}
`
	c, err := cenario.Carregar([]byte(entrada))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Variaveis["tenant"] != "acme" || c.Variaveis["regiao"] != "sul" {
		t.Fatalf("variaveis = %v", c.Variaveis)
	}
	if chave := c.Passos[0].ChaveDeAgregacao(); chave != "GET /clientes/${tenant}/${regiao}" {
		t.Errorf("chave = %q; a interpolacao acontece na execucao, nao no carregamento", chave)
	}
}

func TestValidacaoAcusaProblemas(t *testing.T) {
	c := cenario.Cenario{}
	err := c.Validar()
	if err == nil {
		t.Fatal("esperava erro de validacao")
	}
	for _, trecho := range []string{"nome", "alvo", "passo", "perfil"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("validacao nao mencionou %q: %v", trecho, err)
		}
	}
}

func TestPassoComNomeRepetidoEhInvalido(t *testing.T) {
	entrada := `
nome: repetido
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - constante: { taxa: 1/s, durante: 1s }
cenario:
  - http: GET /a
    nome: consulta
  - http: GET /b
    nome: consulta
`
	c, err := cenario.Carregar([]byte(entrada))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := c.Validar(); err == nil || !strings.Contains(err.Error(), "nome repetido") {
		t.Fatalf("esperava erro de nome repetido, recebeu %v", err)
	}
}
