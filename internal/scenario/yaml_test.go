package scenario_test

import (
	"strings"
	"testing"
	"time"

	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

const minimalScenario = `
name: Jornada de cobrança
target: http://127.0.0.1:8080

load:
  model: open
  profiles:
    - ramp: { from: 1/s, to: 50/s, duration: 1m }
    - steady: { rate: 50/s, duration: 5m }

scenario:
  - http: GET /assinaturas/1
    name: consultar assinatura
    expect: { status: 200 }
`

func TestParseMinimalScenario(t *testing.T) {
	c, err := scenario.Parse([]byte(minimalScenario))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Name != "Jornada de cobrança" {
		t.Errorf("nome = %q", c.Name)
	}
	if c.Load.Model != scenario.OpenArrival {
		t.Errorf("modelo = %q, esperado aberto por padrão", c.Load.Model)
	}
	if len(c.Load.Phases) != 2 {
		t.Fatalf("fases = %d, esperado 2", len(c.Load.Phases))
	}
	if c.Load.Phases[0].From != 1 || c.Load.Phases[0].To != 50 || c.Load.Phases[0].For != time.Minute {
		t.Errorf("rampa lida errado: %+v", c.Load.Phases[0])
	}
	if len(c.Steps) != 1 {
		t.Fatalf("passos = %d", len(c.Steps))
	}
	step := c.Steps[0]
	if step.Name != "consultar assinatura" || step.Protocol != "http" {
		t.Errorf("passo lido errado: %+v", step)
	}
	if step.AggregationKey() != "GET /assinaturas/1" {
		t.Errorf("chave de agregação = %q", step.AggregationKey())
	}
	if len(step.Checks) != 1 || step.Checks[0].Status != 200 {
		t.Errorf("verificacoes lidas errado: %+v", step.Checks)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("cenário deveria ser válido: %v", err)
	}
}

func TestLongFormOfHTTPStep(t *testing.T) {
	input := `
name: com corpo
target: http://127.0.0.1:8080
load:
  profiles:
    - steady: { rate: 10/s, duration: 1s }
scenario:
  - name: criar fatura
    http:
      method: POST
      path: /faturas
      headers: { X-Tenant: acme }
      body: { value: 199.90 }
`
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Steps[0].AggregationKey() != "POST /faturas" {
		t.Errorf("chave = %q", c.Steps[0].AggregationKey())
	}
}

func TestErrorPointsToLine(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		fragment string
		line     int
	}{
		{
			name:     "protocolo desconhecido",
			input:    "name: x\ntarget: http://a\nload:\n  profiles:\n    - steady: { rate: 1/s, duration: 1s }\nscenario:\n  - grpc: /servico\n",
			fragment: "I do not recognize",
			line:     7,
		},
		{
			name:     "invalid rate",
			input:    "name: x\ntarget: http://a\nload:\n  profiles:\n    - steady: { rate: rapido, duration: 1s }\nscenario:\n  - http: GET /\n",
			fragment: "invalid rate",
			line:     5,
		},
		{
			name:     "chave desconhecida no topo",
			input:    "name: x\ntarget: http://a\nturbo: sim\n",
			fragment: "unknown key",
			line:     3,
		},
		{
			name:     "perfil desconhecido",
			input:    "name: x\ntarget: http://a\nload:\n  profiles:\n    - montanha: { rate: 1/s, duration: 1s }\nscenario:\n  - http: GET /\n",
			fragment: "unknown profile kind",
			line:     5,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := scenario.Parse([]byte(testCase.input))
			if err == nil {
				t.Fatal("esperava erro")
			}
			scenarioErr, ok := err.(scenario.ScenarioError)
			if !ok {
				t.Fatalf("esperava ErroDeCenario, recebeu %T: %v", err, err)
			}
			if scenarioErr.Line != testCase.line {
				t.Errorf("linha = %d, esperado %d (%v)", scenarioErr.Line, testCase.line, scenarioErr)
			}
			if !strings.Contains(scenarioErr.Message, testCase.fragment) {
				t.Errorf("mensagem = %q, esperava conter %q", scenarioErr.Message, testCase.fragment)
			}
		})
	}
}

func TestAggregationKeyCarriesNoInterpolatedValue(t *testing.T) {
	input := `
name: agregacao
target: http://127.0.0.1:8080
variables:
  pedidoId: "1"
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /pedidos/${pedidoId}
    capture: { proximo: $.proximo.id }
`
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if key := c.Steps[0].AggregationKey(); key != "GET /pedidos/${pedidoId}" {
		t.Errorf("chave = %q; o relatório agrega pela rota declarada, nunca pela URL com o valor dentro", key)
	}
	if len(c.Steps[0].Captures) != 1 || c.Steps[0].Captures[0].Origin != scenario.CaptureJSON {
		t.Errorf("captura lida errado: %+v", c.Steps[0].Captures)
	}
}

func TestVariableUsesEnvironmentAndDefault(t *testing.T) {
	t.Setenv("TENANT_DE_TESTE", "acme")
	input := `
name: variaveis
target: http://127.0.0.1:8080
variables:
  tenant: ${TENANT_DE_TESTE:-padrao}
  regiao: ${REGIAO_INEXISTENTE_NO_AMBIENTE:-sul}
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /clientes/${tenant}/${regiao}
`
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Vars["tenant"] != "acme" || c.Vars["regiao"] != "sul" {
		t.Fatalf("variáveis = %v", c.Vars)
	}
	if key := c.Steps[0].AggregationKey(); key != "GET /clientes/${tenant}/${regiao}" {
		t.Errorf("chave = %q; a interpolação acontece na execução, não no carregamento", key)
	}
}

func TestValidationReportsProblems(t *testing.T) {
	c := scenario.Spec{}
	err := c.Validate()
	if err == nil {
		t.Fatal("esperava erro de validação")
	}
	for _, fragment := range []string{"name", "target", "step", "profile"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("validação não mencionou %q: %v", fragment, err)
		}
	}
}

func TestDuplicateStepNameIsInvalid(t *testing.T) {
	input := `
name: repetido
target: http://127.0.0.1:8080
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /a
    name: consulta
  - http: GET /b
    name: consulta
`
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "repeated name") {
		t.Fatalf("esperava erro de nome repetido, recebeu %v", err)
	}
}

// A bare number was read as per second. Whoever wrote 100 meaning per minute
// got sixty times the load, no warning, and a report about a load nobody asked
// for.
func TestRateWithoutUnitIsRefusedInsteadOfAssumedPerSecond(t *testing.T) {
	_, err := scenario.Parse([]byte("name: x\ntarget: http://a\nload:\n  profiles:\n    - steady: { rate: 100, duration: 5s }\nscenario:\n  - http: GET /\n"))
	if err == nil {
		t.Fatal("taxa sem unidade foi aceita: 100 virou 100/s sem ninguém ser avisado")
	}
	for _, expected := range []string{"without a unit", "100/s", "100/m", "100/h"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("a mensagem não ensina as unidades: falta %q em\n%s", expected, err)
		}
	}
}

func TestRateThatIsNotANumberKeepsItsOwnMessage(t *testing.T) {
	_, err := scenario.Parse([]byte("name: x\ntarget: http://a\nload:\n  profiles:\n    - steady: { rate: rapido, duration: 5s }\nscenario:\n  - http: GET /\n"))
	if err == nil || !strings.Contains(err.Error(), "invalid rate") {
		t.Fatalf("taxa que não e número passou a sugerir unidades sem sentido: %v", err)
	}
}
