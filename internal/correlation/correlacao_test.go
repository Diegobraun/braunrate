package correlation_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/correlation"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

var respostaDeExemplo = protocol.Resposta{
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
		captura := scenario.Captura{Variavel: "v", Origem: scenario.CapturaDeJSON, Expressao: expressao}
		obtido, err := correlation.Extrair(captura, respostaDeExemplo)
		if err != nil {
			t.Fatalf("%s: erro inesperado: %v", expressao, err)
		}
		if obtido != esperado {
			t.Errorf("%s = %q, esperado %q", expressao, obtido, esperado)
		}
	}
}

func TestCapturaPorCabecalhoIgnoraCaixa(t *testing.T) {
	captura := scenario.Captura{Variavel: "requisicao", Origem: scenario.CapturaDeCabecalho, Expressao: "x-request-id"}
	obtido, err := correlation.Extrair(captura, respostaDeExemplo)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obtido != "req-42" {
		t.Errorf("captura = %q, esperado req-42", obtido)
	}
}

func TestCapturaPorRegexUsaOPrimeiroGrupo(t *testing.T) {
	captura := scenario.Captura{Variavel: "token", Origem: scenario.CapturaDeRegex, Expressao: `"token":"([^"]+)"`}
	obtido, err := correlation.Extrair(captura, respostaDeExemplo)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obtido != "abc123" {
		t.Errorf("captura = %q, esperado abc123", obtido)
	}
}

func TestCapturaQueFalhaExplicaOMotivo(t *testing.T) {
	captura := scenario.Captura{Variavel: "faturaId", Origem: scenario.CapturaDeJSON, Expressao: "$.nao.existe"}
	_, err := correlation.Extrair(captura, respostaDeExemplo)
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
	captura := scenario.Captura{Variavel: "id", Origem: scenario.CapturaDeJSON, Expressao: "$.id"}
	_, err := correlation.Extrair(captura, protocol.Resposta{Corpo: []byte("<html>erro</html>")})
	if err == nil || !strings.Contains(err.Error(), "nao e JSON valido") {
		t.Fatalf("esperava aviso de corpo nao-JSON, recebeu %v", err)
	}
}

func semResolucao(texto string) string { return texto }

func TestAssercaoSobreCampoJSON(t *testing.T) {
	casos := []struct {
		assercao scenario.Assercao
		passa    bool
	}{
		{scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.status", Operador: scenario.OperadorIgual, Valor: "PAGA"}, true},
		{scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.status", Operador: scenario.OperadorIgual, Valor: "ABERTA"}, false},
		{scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.valor", Operador: scenario.OperadorMenor, Valor: "500"}, true},
		{scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.valor", Operador: scenario.OperadorMaior, Valor: "500"}, false},
		{scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.id", Operador: scenario.OperadorExiste}, true},
		{scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.reembolso", Operador: scenario.OperadorExiste}, false},
	}
	for _, caso := range casos {
		err := correlation.Avaliar(caso.assercao, respostaDeExemplo, semResolucao)
		if caso.passa && err != nil {
			t.Errorf("%+v deveria passar, falhou com %v", caso.assercao, err)
		}
		if !caso.passa && err == nil {
			t.Errorf("%+v deveria falhar", caso.assercao)
		}
	}
}

func TestFalhaDeAssercaoDizEsperadoEObtido(t *testing.T) {
	assercao := scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.status",
		Operador: scenario.OperadorIgual, Valor: "ABERTA"}
	err := correlation.Avaliar(assercao, respostaDeExemplo, semResolucao)
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
	if err := correlation.Avaliar(scenario.Assercao{Tipo: scenario.AsserirCorpoContem, Valor: "PAGA"},
		respostaDeExemplo, semResolucao); err != nil {
		t.Errorf("corpo_contem deveria passar: %v", err)
	}
	if err := correlation.Avaliar(scenario.Assercao{Tipo: scenario.AsserirRegex, Valor: `"sku":"x\d"`},
		respostaDeExemplo, semResolucao); err != nil {
		t.Errorf("corpo_casa deveria passar: %v", err)
	}
	if err := correlation.Avaliar(scenario.Assercao{Tipo: scenario.AsserirCorpoContem, Valor: "ESTORNADA"},
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
	assercao := scenario.Assercao{Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.status",
		Operador: scenario.OperadorIgual, Valor: "${statusEsperado}"}
	if err := correlation.Avaliar(assercao, respostaDeExemplo, resolver); err != nil {
		t.Errorf("assercao com variavel deveria passar: %v", err)
	}
}
