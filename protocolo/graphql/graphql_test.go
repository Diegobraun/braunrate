package graphql_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/Diegobraun/braunrate/protocolo/graphql"
	"gopkg.in/yaml.v3"
)

func decodificar(t *testing.T, texto string) protocolo.Configuracao {
	t.Helper()
	var documento yaml.Node
	if err := yaml.Unmarshal([]byte(texto), &documento); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	configuracao, err := graphql.Novo(protocolo.OpcoesPadrao()).Decodificar(documento.Content[0])
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	return configuracao
}

func erroAoDecodificar(t *testing.T, texto string) error {
	t.Helper()
	var documento yaml.Node
	if err := yaml.Unmarshal([]byte(texto), &documento); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	_, err := graphql.Novo(protocolo.OpcoesPadrao()).Decodificar(documento.Content[0])
	if err == nil {
		t.Fatal("esperava erro e decodificou")
	}
	return err
}

// Em GraphQL toda operacao chega no mesmo endereco: agregar por URL juntaria a
// consulta mais barata com a mutation mais cara numa linha so.
func TestAChaveDeAgregacaoEhAOperacaoENaoAURL(t *testing.T) {
	configuracao := decodificar(t, "|\n  query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }\n")
	if configuracao.ChaveDeAgregacao() != "graphql ConsultarPedido" {
		t.Errorf("chave = %q", configuracao.ChaveDeAgregacao())
	}
	if configuracao.Protocolo() != "graphql" {
		t.Errorf("protocolo = %q", configuracao.Protocolo())
	}
}

func TestNomeDaOperacaoSaiDaPropriaConsulta(t *testing.T) {
	configuracao := decodificar(t, "consulta: |\n  mutation PagarFatura($f: ID!) { pagarFatura(id: $f) { status } }\n")
	if configuracao.ChaveDeAgregacao() != "graphql PagarFatura" {
		t.Errorf("chave = %q: o nome da operacao deveria vir da consulta", configuracao.ChaveDeAgregacao())
	}
}

func TestOperacaoAnonimaEhRecusadaComMensagemQueEnsina(t *testing.T) {
	err := erroAoDecodificar(t, "|\n  { pedido(id: \"1\") { status } }\n")
	if !strings.Contains(err.Error(), "precisa de nome") {
		t.Errorf("mensagem = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "query ConsultarPedido") {
		t.Errorf("a mensagem precisa mostrar a forma certa: %q", err.Error())
	}
}

func TestVariaveisSaoResolvidasAcadaIteracao(t *testing.T) {
	configuracao := decodificar(t, "consulta: |\n  query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }\nvariaveis: { id: \"${assinantes.id}\" }\n")
	resolvida := configuracao.Resolver(func(texto string) string {
		return strings.ReplaceAll(texto, "${assinantes.id}", "1002")
	})
	descritivel, sabe := resolvida.(protocolo.ConfiguracaoDescritivel)
	if !sabe {
		t.Fatal("a configuracao de graphql precisa saber se descrever para o modo de depuracao")
	}
	if !strings.Contains(strings.Join(descritivel.Descrever(), " "), `{"id":"1002"}`) {
		t.Errorf("variaveis nao resolvidas: %v", descritivel.Descrever())
	}
}

func TestTokenNaoApareceInteiroNaDepuracao(t *testing.T) {
	configuracao := decodificar(t, "consulta: |\n  query ConsultarPedido { pedido { status } }\n")
	comCabecalho := configuracao.(protocolo.ConfiguracaoComCabecalhos).ComCabecalho("Authorization", "Bearer abcdefghijklmno")
	descricao := strings.Join(comCabecalho.(protocolo.ConfiguracaoDescritivel).Descrever(), " ")
	if strings.Contains(descricao, "abcdefghijklmno") {
		t.Errorf("o token apareceu inteiro na depuracao: %s", descricao)
	}
}

func executarContra(t *testing.T, corpo string, status int) protocolo.Resposta {
	t.Helper()
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, corpo)
	}))
	t.Cleanup(servidor.Close)

	configuracao := decodificar(t, "|\n  query ConsultarPedido { pedido { status } }\n")
	return graphql.Novo(protocolo.OpcoesPadrao()).Executar(context.Background(), protocolo.Requisicao{
		NomeDoPasso:  "consultar",
		Configuracao: configuracao,
		URLBase:      servidor.URL,
	})
}

// O erro de GraphQL chega com status 200: contar isso como sucesso e aprovar um
// servico que esta respondendo erro em todas as requisicoes.
func TestErroNoCorpoComStatus200ContaComoErro(t *testing.T) {
	resposta := executarContra(t, `{"errors":[{"message":"pedido nao encontrado","path":["pedido"],"extensions":{"code":"NOT_FOUND"}}]}`, 200)
	if resposta.Classe != protocolo.ErroDeGraphQL {
		t.Fatalf("classe = %q, esperava erro de graphql", resposta.Classe)
	}
	if !strings.Contains(resposta.Detalhe, "NOT_FOUND") || !strings.Contains(resposta.Detalhe, "em pedido") {
		t.Errorf("o detalhe precisa dizer o codigo e onde falhou: %q", resposta.Detalhe)
	}
}

func TestRespostaParcialEhErroEhDeclaradaComoParcial(t *testing.T) {
	resposta := executarContra(t, `{"data":{"pedido":null},"errors":[{"message":"sem permissao"}]}`, 200)
	if resposta.Classe != protocolo.ErroDeGraphQL {
		t.Fatalf("classe = %q", resposta.Classe)
	}
	if !strings.HasPrefix(resposta.Detalhe, "resposta parcial") {
		t.Errorf("resposta com data e errors precisa ser declarada parcial: %q", resposta.Detalhe)
	}
}

func TestRespostaSemErroEhSucesso(t *testing.T) {
	resposta := executarContra(t, `{"data":{"pedido":{"status":"ABERTO"}}}`, 200)
	if resposta.Classe != protocolo.Sucesso {
		t.Fatalf("classe = %q, detalhe = %q", resposta.Classe, resposta.Detalhe)
	}
}

func TestStatusDeErroContinuaSendoErroDeStatus(t *testing.T) {
	resposta := executarContra(t, `{"errors":[{"message":"nao autorizado"}]}`, 401)
	if resposta.Classe != protocolo.ErroDeStatus {
		t.Errorf("classe = %q: erro de transporte continua sendo erro de status", resposta.Classe)
	}
}

func TestCorpoQueNaoEhGraphQLNaoPassaComoSucesso(t *testing.T) {
	resposta := executarContra(t, `<html>gateway</html>`, 200)
	if resposta.Classe != protocolo.ErroDeGraphQL {
		t.Errorf("classe = %q: pagina HTML com status 200 nao e resposta de GraphQL", resposta.Classe)
	}
}
