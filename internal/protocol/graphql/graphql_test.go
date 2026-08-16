package graphql_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/graphql"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, text string) protocol.Config {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	config, err := graphql.New(protocol.DefaultOptions()).Decode(document.Content[0])
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	return config
}

func decodeErr(t *testing.T, text string) error {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	_, err := graphql.New(protocol.DefaultOptions()).Decode(document.Content[0])
	if err == nil {
		t.Fatal("esperava erro e decodificou")
	}
	return err
}

// In GraphQL every operation arrives at the same address: aggregating by URL
// would put the cheapest query and the most expensive mutation on one row.
func TestAggregationKeyIsOperationNotURL(t *testing.T) {
	config := decode(t, "|\n  query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }\n")
	if config.AggregationKey() != "graphql ConsultarPedido" {
		t.Errorf("chave = %q", config.AggregationKey())
	}
	if config.Protocol() != "graphql" {
		t.Errorf("protocolo = %q", config.Protocol())
	}
}

func TestOperationNameComesFromQuery(t *testing.T) {
	config := decode(t, "consulta: |\n  mutation PagarFatura($f: ID!) { pagarFatura(id: $f) { status } }\n")
	if config.AggregationKey() != "graphql PagarFatura" {
		t.Errorf("chave = %q: o nome da operacao deveria vir da consulta", config.AggregationKey())
	}
}

func TestAnonymousOperationIsRefusedWithTeachingMessage(t *testing.T) {
	err := decodeErr(t, "|\n  { pedido(id: \"1\") { status } }\n")
	if !strings.Contains(err.Error(), "precisa de nome") {
		t.Errorf("mensagem = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "query ConsultarPedido") {
		t.Errorf("a mensagem precisa mostrar a forma certa: %q", err.Error())
	}
}

func TestVarsAreResolvedEachIteration(t *testing.T) {
	config := decode(t, "consulta: |\n  query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }\nvariaveis: { id: \"${assinantes.id}\" }\n")
	resolvida := config.Resolve(func(text string) string {
		return strings.ReplaceAll(text, "${assinantes.id}", "1002")
	})
	describable, knows := resolvida.(protocol.Describable)
	if !knows {
		t.Fatal("a configuracao de graphql precisa saber se descrever para o modo de depuracao")
	}
	if !strings.Contains(strings.Join(describable.Describe(), " "), `{"id":"1002"}`) {
		t.Errorf("variaveis nao resolvidas: %v", describable.Describe())
	}
}

func TestTokenIsNeverPrintedInFullWhenDebugging(t *testing.T) {
	config := decode(t, "consulta: |\n  query ConsultarPedido { pedido { status } }\n")
	withHeader := config.(protocol.WithHeaders).WithHeader("Authorization", "Bearer abcdefghijklmno")
	description := strings.Join(withHeader.(protocol.Describable).Describe(), " ")
	if strings.Contains(description, "abcdefghijklmno") {
		t.Errorf("o token apareceu inteiro na depuracao: %s", description)
	}
}

func runAgainst(t *testing.T, body string, status int) protocol.Response {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)

	config := decode(t, "|\n  query ConsultarPedido { pedido { status } }\n")
	return graphql.New(protocol.DefaultOptions()).Execute(context.Background(), protocol.Request{
		StepName: "consultar",
		Config:   config,
		URLBase:  server.URL,
	})
}

// A GraphQL error arrives with status 200: counting that as success approves a
// service that is answering an error to every request.
func TestBodyErrorWithStatus200CountsAsError(t *testing.T) {
	response := runAgainst(t, `{"errors":[{"message":"pedido nao encontrado","path":["pedido"],"extensions":{"code":"NOT_FOUND"}}]}`, 200)
	if response.Class != protocol.ErrGraphQL {
		t.Fatalf("classe = %q, esperava erro de graphql", response.Class)
	}
	if !strings.Contains(response.Detail, "NOT_FOUND") || !strings.Contains(response.Detail, "em pedido") {
		t.Errorf("o detalhe precisa dizer o codigo e onde falhou: %q", response.Detail)
	}
}

func TestPartialResponseIsErrorAndDeclaredPartial(t *testing.T) {
	response := runAgainst(t, `{"data":{"pedido":null},"errors":[{"message":"sem permissao"}]}`, 200)
	if response.Class != protocol.ErrGraphQL {
		t.Fatalf("classe = %q", response.Class)
	}
	if !strings.HasPrefix(response.Detail, "resposta parcial") {
		t.Errorf("resposta com data e errors precisa ser declarada parcial: %q", response.Detail)
	}
}

func TestResponseWithoutErrorsIsSuccess(t *testing.T) {
	response := runAgainst(t, `{"data":{"pedido":{"status":"ABERTO"}}}`, 200)
	if response.Class != protocol.Success {
		t.Fatalf("classe = %q, detalhe = %q", response.Class, response.Detail)
	}
}

func TestErrorStatusStaysStatusError(t *testing.T) {
	response := runAgainst(t, `{"errors":[{"message":"nao autorizado"}]}`, 401)
	if response.Class != protocol.ErrStatus {
		t.Errorf("classe = %q: erro de transporte continua sendo erro de status", response.Class)
	}
}

func TestNonGraphQLBodyIsNotSuccess(t *testing.T) {
	response := runAgainst(t, `<html>gateway</html>`, 200)
	if response.Class != protocol.ErrGraphQL {
		t.Errorf("classe = %q: pagina HTML com status 200 nao e resposta de GraphQL", response.Class)
	}
}
