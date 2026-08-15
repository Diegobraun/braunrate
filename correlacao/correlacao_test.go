package correlacao_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/correlacao"
	"github.com/Diegobraun/braunrate/protocolo"
)

var respostaDeExemplo = protocolo.Resposta{
	Status: 200,
	Corpo: []byte(`{"id":"a1","ultimaFatura":{"id":"f-99","valor":199.9,"status":"PAGA"},
	"itens":[{"sku":"x1"},{"sku":"x2"}],"token":"abc123"}`),
	Cabecalhos: map[string][]string{"X-Request-Id": {"req-42"}, "Content-Type": {"application/json"}},
}

func TestCapturaPorJSONPath(t *testing.T) {
	casos := map[string]string{
		"$.id":                 "a1",
		"$.ultimaFatura.id":    "f-99",
		"$.ultimaFatura.valor": "199.9",
		"$.itens[1].sku":       "x2",
		"ultimaFatura.status":  "PAGA",
	}
	for expressao, esperado := range casos {
		captura := cenario.Captura{Variavel: "v", Origem: cenario.CapturaDeJSON, Expressao: expressao}
		obtido, err := correlacao.Extrair(captura, respostaDeExemplo)
		if err != nil {
			t.Fatalf("%s: erro inesperado: %v", expressao, err)
		}
		if obtido != esperado {
			t.Errorf("%s = %q, esperado %q", expressao, obtido, esperado)
		}
	}
}

func TestCapturaPorCabecalhoIgnoraCaixa(t *testing.T) {
	captura := cenario.Captura{Variavel: "requisicao", Origem: cenario.CapturaDeCabecalho, Expressao: "x-request-id"}
	obtido, err := correlacao.Extrair(captura, respostaDeExemplo)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obtido != "req-42" {
		t.Errorf("captura = %q, esperado req-42", obtido)
	}
}

func TestCapturaPorRegexUsaOPrimeiroGrupo(t *testing.T) {
	captura := cenario.Captura{Variavel: "token", Origem: cenario.CapturaDeRegex, Expressao: `"token":"([^"]+)"`}
	obtido, err := correlacao.Extrair(captura, respostaDeExemplo)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obtido != "abc123" {
		t.Errorf("captura = %q, esperado abc123", obtido)
	}
}

func TestCapturaQueFalhaExplicaOMotivo(t *testing.T) {
	captura := cenario.Captura{Variavel: "faturaId", Origem: cenario.CapturaDeJSON, Expressao: "$.nao.existe"}
	_, err := correlacao.Extrair(captura, respostaDeExemplo)
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, trecho := range []string{"faturaId", "$.nao.existe", "nao encontrado"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("mensagem %q nao menciona %q", err.Error(), trecho)
		}
	}
}

func TestCapturaEmRespostaQueNaoEhJSONDizIsso(t *testing.T) {
	captura := cenario.Captura{Variavel: "id", Origem: cenario.CapturaDeJSON, Expressao: "$.id"}
	_, err := correlacao.Extrair(captura, protocolo.Resposta{Corpo: []byte("<html>erro</html>")})
	if err == nil || !strings.Contains(err.Error(), "nao e JSON valido") {
		t.Fatalf("esperava aviso de corpo nao-JSON, recebeu %v", err)
	}
}

func semResolucao(texto string) string { return texto }

func TestAssercaoSobreCampoJSON(t *testing.T) {
	casos := []struct {
		assercao cenario.Assercao
		passa    bool
	}{
		{cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.status", Operador: cenario.OperadorIgual, Valor: "PAGA"}, true},
		{cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.status", Operador: cenario.OperadorIgual, Valor: "ABERTA"}, false},
		{cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.valor", Operador: cenario.OperadorMenor, Valor: "500"}, true},
		{cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.valor", Operador: cenario.OperadorMaior, Valor: "500"}, false},
		{cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.id", Operador: cenario.OperadorExiste}, true},
		{cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.reembolso", Operador: cenario.OperadorExiste}, false},
	}
	for _, caso := range casos {
		err := correlacao.Avaliar(caso.assercao, respostaDeExemplo, semResolucao)
		if caso.passa && err != nil {
			t.Errorf("%+v deveria passar, falhou com %v", caso.assercao, err)
		}
		if !caso.passa && err == nil {
			t.Errorf("%+v deveria falhar", caso.assercao)
		}
	}
}

func TestFalhaDeAssercaoDizEsperadoEObtido(t *testing.T) {
	assercao := cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.status",
		Operador: cenario.OperadorIgual, Valor: "ABERTA"}
	err := correlacao.Avaliar(assercao, respostaDeExemplo, semResolucao)
	if err == nil {
		t.Fatal("esperava falha")
	}
	for _, trecho := range []string{"ABERTA", "PAGA", "esperava"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("mensagem %q nao menciona %q", err.Error(), trecho)
		}
	}
}

func TestAssercaoDeCorpoERegex(t *testing.T) {
	if err := correlacao.Avaliar(cenario.Assercao{Tipo: cenario.AsserirCorpoContem, Valor: "PAGA"},
		respostaDeExemplo, semResolucao); err != nil {
		t.Errorf("corpo_contem deveria passar: %v", err)
	}
	if err := correlacao.Avaliar(cenario.Assercao{Tipo: cenario.AsserirRegex, Valor: `"sku":"x\d"`},
		respostaDeExemplo, semResolucao); err != nil {
		t.Errorf("corpo_casa deveria passar: %v", err)
	}
	if err := correlacao.Avaliar(cenario.Assercao{Tipo: cenario.AsserirCorpoContem, Valor: "ESTORNADA"},
		respostaDeExemplo, semResolucao); err == nil {
		t.Error("corpo_contem deveria falhar")
	}
}

func TestAssercaoUsaVariavelResolvida(t *testing.T) {
	resolver := func(texto string) string {
		if texto == "${statusEsperado}" {
			return "PAGA"
		}
		return texto
	}
	assercao := cenario.Assercao{Tipo: cenario.AsserirJSON, Alvo: "$.ultimaFatura.status",
		Operador: cenario.OperadorIgual, Valor: "${statusEsperado}"}
	if err := correlacao.Avaliar(assercao, respostaDeExemplo, resolver); err != nil {
		t.Errorf("assercao com variavel deveria passar: %v", err)
	}
}
