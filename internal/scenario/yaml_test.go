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

func TestParseMinimalScenario(t *testing.T) {
	c, err := scenario.Parse([]byte(minimalScenario))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Name != "Jornada de cobranca" {
		t.Errorf("nome = %q", c.Name)
	}
	if c.Load.Model != scenario.OpenArrival {
		t.Errorf("modelo = %q, esperado aberto por padrao", c.Load.Model)
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
		t.Errorf("chave de agregacao = %q", step.AggregationKey())
	}
	if len(step.Checks) != 1 || step.Checks[0].Status != 200 {
		t.Errorf("verificacoes lidas errado: %+v", step.Checks)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("cenario deveria ser valido: %v", err)
	}
}

func TestLongFormOfHTTPStep(t *testing.T) {
	input := `
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
			input:    "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - constante: { taxa: 1/s, durante: 1s }\ncenario:\n  - grpc: /servico\n",
			fragment: "nao reconheco",
			line:     7,
		},
		{
			name:     "taxa invalida",
			input:    "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - constante: { taxa: rapido, durante: 1s }\ncenario:\n  - http: GET /\n",
			fragment: "taxa invalida",
			line:     5,
		},
		{
			name:     "chave desconhecida no topo",
			input:    "nome: x\nalvo: http://a\nturbo: sim\n",
			fragment: "chave desconhecida",
			line:     3,
		},
		{
			name:     "perfil desconhecido",
			input:    "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - montanha: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n",
			fragment: "tipo de perfil desconhecido",
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
nome: agregacao
alvo: http://127.0.0.1:8080
variaveis:
  pedidoId: "1"
carga:
  perfis:
    - constante: { taxa: 1/s, durante: 1s }
cenario:
  - http: GET /pedidos/${pedidoId}
    captura: { proximo: $.proximo.id }
`
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if key := c.Steps[0].AggregationKey(); key != "GET /pedidos/${pedidoId}" {
		t.Errorf("chave = %q; o relatorio agrega pela rota declarada, nunca pela URL com o valor dentro", key)
	}
	if len(c.Steps[0].Captures) != 1 || c.Steps[0].Captures[0].Origin != scenario.CaptureJSON {
		t.Errorf("captura lida errado: %+v", c.Steps[0].Captures)
	}
}

func TestVariableUsesEnvironmentAndDefault(t *testing.T) {
	t.Setenv("TENANT_DE_TESTE", "acme")
	input := `
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
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Vars["tenant"] != "acme" || c.Vars["regiao"] != "sul" {
		t.Fatalf("variaveis = %v", c.Vars)
	}
	if key := c.Steps[0].AggregationKey(); key != "GET /clientes/${tenant}/${regiao}" {
		t.Errorf("chave = %q; a interpolacao acontece na execucao, nao no carregamento", key)
	}
}

func TestValidationReportsProblems(t *testing.T) {
	c := scenario.Spec{}
	err := c.Validate()
	if err == nil {
		t.Fatal("esperava erro de validacao")
	}
	for _, fragment := range []string{"nome", "alvo", "passo", "perfil"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("validacao nao mencionou %q: %v", fragment, err)
		}
	}
}

func TestDuplicateStepNameIsInvalid(t *testing.T) {
	input := `
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
	c, err := scenario.Parse([]byte(input))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "nome repetido") {
		t.Fatalf("esperava erro de nome repetido, recebeu %v", err)
	}
}

// A bare number was read as per second. Whoever wrote 100 meaning per minute
// got sixty times the load, no warning, and a report about a load nobody asked
// for.
func TestRateWithoutUnitIsRefusedInsteadOfAssumedPerSecond(t *testing.T) {
	_, err := scenario.Parse([]byte("nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - patamar: { taxa: 100, durante: 5s }\ncenario:\n  - http: GET /\n"))
	if err == nil {
		t.Fatal("taxa sem unidade foi aceita: 100 virou 100/s sem ninguem ser avisado")
	}
	for _, expected := range []string{"sem unidade", "100/s", "100/m", "100/h"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("a mensagem nao ensina as unidades: falta %q em\n%s", expected, err)
		}
	}
}

func TestRateThatIsNotANumberKeepsItsOwnMessage(t *testing.T) {
	_, err := scenario.Parse([]byte("nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - patamar: { taxa: rapido, durante: 5s }\ncenario:\n  - http: GET /\n"))
	if err == nil || !strings.Contains(err.Error(), "taxa invalida") {
		t.Fatalf("taxa que nao e numero passou a sugerir unidades sem sentido: %v", err)
	}
}
