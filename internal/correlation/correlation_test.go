package correlation_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/correlation"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

var sampleResponse = protocol.Response{
	Status: 200,
	Body: []byte(`{"id":"a1","ultimaFatura":{"id":"f-99","valor":199.9,"status":"PAGA"},
	"itens":[{"sku":"x1"},{"sku":"x2"}],"token":"abc123"}`),
	Headers: map[string][]string{"X-Request-Id": {"req-42"}, "Content-Type": {"application/json"}},
}

func TestCaptureByJSONPath(t *testing.T) {
	testCases := map[string]string{
		"$.id":                 "a1",
		"$.ultimaFatura.id":    "f-99",
		"$.ultimaFatura.valor": "199.9",
		"$.itens[1].sku":       "x2",
		"ultimaFatura.status":  "PAGA",
	}
	for expression, expected := range testCases {
		capture := scenario.Capture{Variable: "v", Origin: scenario.CaptureJSON, Expression: expression}
		obtained, err := correlation.Extract(capture, sampleResponse)
		if err != nil {
			t.Fatalf("%s: erro inesperado: %v", expression, err)
		}
		if obtained != expected {
			t.Errorf("%s = %q, esperado %q", expression, obtained, expected)
		}
	}
}

func TestHeaderCaptureIgnoresCase(t *testing.T) {
	capture := scenario.Capture{Variable: "requisicao", Origin: scenario.CaptureHeader, Expression: "x-request-id"}
	obtained, err := correlation.Extract(capture, sampleResponse)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obtained != "req-42" {
		t.Errorf("captura = %q, esperado req-42", obtained)
	}
}

func TestRegexCaptureUsesFirstGroup(t *testing.T) {
	capture := scenario.Capture{Variable: "token", Origin: scenario.CaptureRegex, Expression: `"token":"([^"]+)"`}
	obtained, err := correlation.Extract(capture, sampleResponse)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obtained != "abc123" {
		t.Errorf("captura = %q, esperado abc123", obtained)
	}
}

func TestFailedCaptureExplainsWhy(t *testing.T) {
	capture := scenario.Capture{Variable: "faturaId", Origin: scenario.CaptureJSON, Expression: "$.nao.existe"}
	_, err := correlation.Extract(capture, sampleResponse)
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, fragment := range []string{"faturaId", "$.nao.existe", "nao encontrado"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("mensagem %q nao menciona %q", err.Error(), fragment)
		}
	}
}

func TestCaptureOnNonJSONResponseSaysSo(t *testing.T) {
	capture := scenario.Capture{Variable: "id", Origin: scenario.CaptureJSON, Expression: "$.id"}
	_, err := correlation.Extract(capture, protocol.Response{Body: []byte("<html>erro</html>")})
	if err == nil || !strings.Contains(err.Error(), "nao e JSON valido") {
		t.Fatalf("esperava aviso de corpo nao-JSON, recebeu %v", err)
	}
}

func unresolved(text string) string { return text }

func TestAssertionOnJSONField(t *testing.T) {
	testCases := []struct {
		assertion scenario.Assertion
		passes    bool
	}{
		{scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.status", Operator: scenario.OpEqual, Value: "PAGA"}, true},
		{scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.status", Operator: scenario.OpEqual, Value: "ABERTA"}, false},
		{scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.valor", Operator: scenario.OpLess, Value: "500"}, true},
		{scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.valor", Operator: scenario.OpGreater, Value: "500"}, false},
		{scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.id", Operator: scenario.OpExists}, true},
		{scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.reembolso", Operator: scenario.OpExists}, false},
	}
	for _, testCase := range testCases {
		err := correlation.Evaluate(testCase.assertion, sampleResponse, unresolved)
		if testCase.passes && err != nil {
			t.Errorf("%+v deveria passar, falhou com %v", testCase.assertion, err)
		}
		if !testCase.passes && err == nil {
			t.Errorf("%+v deveria falhar", testCase.assertion)
		}
	}
}

func TestAssertionFailureSaysExpectedAndObtained(t *testing.T) {
	assertion := scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.status",
		Operator: scenario.OpEqual, Value: "ABERTA"}
	err := correlation.Evaluate(assertion, sampleResponse, unresolved)
	if err == nil {
		t.Fatal("esperava falha")
	}
	for _, fragment := range []string{"ABERTA", "PAGA", "esperava"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("mensagem %q nao menciona %q", err.Error(), fragment)
		}
	}
}

func TestBodyAndRegexAssertions(t *testing.T) {
	if err := correlation.Evaluate(scenario.Assertion{Kind: scenario.AssertBodyContains, Value: "PAGA"},
		sampleResponse, unresolved); err != nil {
		t.Errorf("corpo_contem deveria passar: %v", err)
	}
	if err := correlation.Evaluate(scenario.Assertion{Kind: scenario.AssertRegex, Value: `"sku":"x\d"`},
		sampleResponse, unresolved); err != nil {
		t.Errorf("corpo_casa deveria passar: %v", err)
	}
	if err := correlation.Evaluate(scenario.Assertion{Kind: scenario.AssertBodyContains, Value: "ESTORNADA"},
		sampleResponse, unresolved); err == nil {
		t.Error("corpo_contem deveria falhar")
	}
}

func TestAssertionUsesResolvedVariable(t *testing.T) {
	resolve := func(text string) string {
		if text == "${statusEsperado}" {
			return "PAGA"
		}
		return text
	}
	assertion := scenario.Assertion{Kind: scenario.AssertJSON, Target: "$.ultimaFatura.status",
		Operator: scenario.OpEqual, Value: "${statusEsperado}"}
	if err := correlation.Evaluate(assertion, sampleResponse, resolve); err != nil {
		t.Errorf("assercao com variavel deveria passar: %v", err)
	}
}
