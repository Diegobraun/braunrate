package dsl_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/dsl"
	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"gopkg.in/yaml.v3"
)

type equivalence struct {
	name string
	yaml string
	dsl  func() (scenario.Scenario, error)
}

// O YAML e a DSL precisam produzir a MESMA estrutura, e nao apenas resultados
// parecidos: o cenario montado e o unico dado que o motor recebe, entao
// igualdade aqui e igualdade de medicao. So o numero da linha e ignorado, que e
// posicao no arquivo e nao existe em codigo Go.
var testCases = []equivalence{
	{
		name: "http com variaveis, dados, autenticacao por token, capturas e slo",
		yaml: `
nome: Jornada autenticada
alvo: ${BASE:-http://127.0.0.1:8080}

variaveis:
  inquilino: acme

autenticacao:
  tipo: token
  renovar_apos: 25m
  obter:
    http:
      metodo: POST
      caminho: /auth/token
      corpo: { senha: segredo, usuario: ana }
    captura:
      token: $.access_token

dados:
  assinantes:
    arquivo: dados/assinantes.csv
    consumo: circular
  pedidos:
    gerar: { id: uuid, valor: "numero(10,500)" }
    semente: 7

carga:
  perfis:
    - rampa: { de: 10/s, ate: 100/s, durante: 30s }
    - patamar: { taxa: 100/s, durante: 1m }
    - pico: { taxa: 600/m, durante: 10s }
    - constante: { taxa: 36000/h, durante: 5s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    captura:
      faturaId: $.ultimaFatura.id
      requisicao: cabecalho:X-Request-Id
      sessao: /sessao=([a-z0-9]+)/
      codigo: status
      inteiro: corpo
      opcional: { de: $.talvez, padrao: vazio }
    verificar:
      status: 200
      corpo_contem: ABERTO
      corpo_casa: "pedido-[0-9]+"
      json: { $.status: ABERTO, $.total: "> 10", $.cupom: existe, $.tags: contem promo }
      cabecalho: { Content-Type: application/json }

  - http:
      metodo: POST
      caminho: /pedidos
      cabecalhos: { X-Inquilino: "${inquilino}" }
      corpo: { assinante: "${assinantes.id}", total: 199.9 }
      timeout: 2s
      seguir_redirect: false

slo:
  - consultar pedido: { p95: < 150ms, max: < 1s }
  - POST /pedidos: { vazao: "> 50/s" }
  - global: { erros: < 0.1 }
`,
		dsl: func() (scenario.Scenario, error) {
			return dsl.New("Jornada autenticada").
				Target("${BASE:-http://127.0.0.1:8080}").
				Variable("inquilino", "acme").
				Auth(dsl.WithToken(
					dsl.POST("/auth/token").Body(map[string]any{"usuario": "ana", "senha": "segredo"}),
					dsl.Capture("token", "$.access_token"),
				).RefreshAfter(25*time.Minute)).
				DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(scenario.ConsumeCircular)).
				GeneratedData("pedidos", map[string]string{"id": "uuid", "valor": "numero(10,500)"}, dsl.Seed(7)).
				Ramp(dsl.PerSecond(10), dsl.PerSecond(100), 30*time.Second).
				Plateau(dsl.PerSecond(100), time.Minute).
				Spike(dsl.PerMinute(600), 10*time.Second).
				Constant(dsl.PerHour(36000), 5*time.Second).
				Step(dsl.GET("/pedidos/${assinantes.id}"),
					dsl.Name("consultar pedido"),
					dsl.Capture("faturaId", "$.ultimaFatura.id"),
					dsl.Capture("requisicao", "cabecalho:X-Request-Id"),
					dsl.Capture("sessao", "/sessao=([a-z0-9]+)/"),
					dsl.Capture("codigo", "status"),
					dsl.Capture("inteiro", "corpo"),
					dsl.CaptureWithDefault("opcional", "$.talvez", "vazio"),
					dsl.CheckStatus(200),
					dsl.CheckBodyContains("ABERTO"),
					dsl.CheckBodyMatches("pedido-[0-9]+"),
					dsl.CheckJSON("$.status", "ABERTO"),
					dsl.CheckJSON("$.total", "> 10"),
					dsl.CheckJSON("$.cupom", "existe"),
					dsl.CheckJSON("$.tags", "contem promo"),
					dsl.CheckHeader("Content-Type", "application/json"),
				).
				Step(dsl.POST("/pedidos").
					Header("X-Inquilino", "${inquilino}").
					Body(map[string]any{"assinante": "${assinantes.id}", "total": 199.9}).
					Timeout(2*time.Second).
					SeguirRedirect(false)).
				SLO("consultar pedido", "p95", "< 150ms").
				SLO("consultar pedido", "max", "< 1s").
				SLO("POST /pedidos", "vazao", "> 50/s").
				OverallSLO("erros", "< 0.1").
				Build()
		},
	},
	{
		name: "graphql por operacao",
		yaml: `
nome: Cobranca em GraphQL
alvo: http://127.0.0.1:8080

carga:
  perfis:
    - patamar: { taxa: 50/s, durante: 20s }

cenario:
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }
      variaveis: { id: "${assinantes.id}" }
      caminho: /graphql
      cabecalhos: { X-Origem: braunrate }
      timeout: 3s
    verificar:
      json: { $.data.pedido.status: ABERTO }

  - graphql: |
      mutation PagarFatura($id: ID!) { pagarFatura(id: $id) { status } }

slo:
  - graphql ConsultarPedido: { p99: < 300ms }
`,
		dsl: func() (scenario.Scenario, error) {
			return dsl.New("Cobranca em GraphQL").
				Target("http://127.0.0.1:8080").
				Plateau(dsl.PerSecond(50), 20*time.Second).
				Step(dsl.GraphQL("query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }\n").
					Vars(map[string]any{"id": "${assinantes.id}"}).
					Path("/graphql").
					Header("X-Origem", "braunrate").
					Timeout(3*time.Second),
					dsl.CheckJSON("$.data.pedido.status", "ABERTO")).
				Step(dsl.GraphQL("mutation PagarFatura($id: ID!) { pagarFatura(id: $id) { status } }\n")).
				SLO("graphql ConsultarPedido", "p99", "< 300ms").
				Build()
		},
	},
	{
		name: "kafka com aguardar fechando a cadeia",
		yaml: `
nome: Cadeia assincrona
alvo: 127.0.0.1:9092

dados:
  pedidos:
    gerar: { id: uuid }
    consumo: sequencial

carga:
  perfis:
    - patamar: { taxa: 100/s, durante: 10s }

cenario:
  - kafka:
      topico: pedidos-cadeia
      chave: "${pedidos.id}"
      valor: { pedido: "${pedidos.id}" }
      cabecalhos: { origem: braunrate }
      brokers: [127.0.0.1:9092]
      acks: lider
      timeout: 5s

  - aguardar:
      kafka: { topico: pedidos-processados, brokers: [127.0.0.1:9092] }
      chave: "${pedidos.id}"
      campo: $.pedido
      timeout: 10s

  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }
      intervalo: 200ms
      timeout: 30s

slo:
  - kafka produzir pedidos-cadeia: { p95: < 100ms }
`,
		dsl: func() (scenario.Scenario, error) {
			return dsl.New("Cadeia assincrona").
				Target("127.0.0.1:9092").
				GeneratedData("pedidos", map[string]string{"id": "uuid"}, dsl.Consume(scenario.ConsumeSequential)).
				Plateau(dsl.PerSecond(100), 10*time.Second).
				Step(dsl.Kafka("pedidos-cadeia").
					Key("${pedidos.id}").
					Value(map[string]any{"pedido": "${pedidos.id}"}).
					Header("origem", "braunrate").
					Brokers("127.0.0.1:9092").
					Acks("lider").
					Timeout(5*time.Second)).
				Step(dsl.WaitForKafka("pedidos-processados").
					Addresses("127.0.0.1:9092").
					Key("${pedidos.id}").
					Field("$.pedido").
					Timeout(10*time.Second)).
				Step(dsl.WaitForHTTP("/pedidos/${pedidos.id}").
					UntilJSON("$.status", "PROCESSADO").
					Interval(200*time.Millisecond).
					Timeout(30*time.Second)).
				SLO("kafka produzir pedidos-cadeia", "p95", "< 100ms").
				Build()
		},
	},
	{
		name: "amqp em fila e em troca com rota",
		yaml: `
nome: Publicacao em RabbitMQ
alvo: amqp://127.0.0.1:5672

dados:
  clientes:
    arquivo: dados/clientes.csv
    consumo: aleatorio

carga:
  perfis:
    - patamar: { taxa: 30/s, durante: 10s }

cenario:
  - amqp:
      fila: pedidos
      identidade: "${clientes.id}"
      corpo: { cliente: "${clientes.id}" }
      cabecalhos: { origem: braunrate }
      url: amqp://127.0.0.1:5672
      persistente: false
      confirmar: false
      timeout: 4s

  - amqp:
      troca: cobranca
      rota: fatura.emitida
      corpo: texto puro

  - aguardar:
      amqp: pedidos-processados
      chave: "${clientes.id}"
`,
		dsl: func() (scenario.Scenario, error) {
			return dsl.New("Publicacao em RabbitMQ").
				Target("amqp://127.0.0.1:5672").
				DataFromFile("clientes", "dados/clientes.csv", dsl.Consume(scenario.ConsumeRandom)).
				Plateau(dsl.PerSecond(30), 10*time.Second).
				Step(dsl.AMQP("pedidos").
					Identity("${clientes.id}").
					Body(map[string]any{"cliente": "${clientes.id}"}).
					Header("origem", "braunrate").
					URL("amqp://127.0.0.1:5672").
					Persistent(false).
					Confirm(false).
					Timeout(4 * time.Second)).
				Step(dsl.Exchange("cobranca", "fatura.emitida").Body("texto puro")).
				Step(dsl.WaitForAMQP("pedidos-processados").Key("${clientes.id}")).
				Build()
		},
	},
	{
		name: "autenticacao basica e consumo unico por usuario",
		yaml: `
nome: Basica
alvo: http://127.0.0.1:8080

autenticacao:
  tipo: basica
  usuario: ana
  senha: segredo

dados:
  assinantes:
    arquivo: dados/assinantes.csv
    consumo: unico_por_usuario

carga:
  perfis:
    - patamar: { taxa: 10/s, durante: 5s }

cenario:
  - http: GET /pedidos
`,
		dsl: func() (scenario.Scenario, error) {
			return dsl.New("Basica").
				Target("http://127.0.0.1:8080").
				Auth(dsl.Basic("ana", "segredo")).
				DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(scenario.ConsumeUniquePerUser)).
				Plateau(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.GET("/pedidos")).
				Build()
		},
	},
	{
		name: "autenticacao por cabecalho fixo",
		yaml: `
nome: Chave de api
alvo: http://127.0.0.1:8080

autenticacao:
  tipo: cabecalho
  cabecalho: "X-API-Key: ${api_key}"

carga:
  perfis:
    - patamar: { taxa: 10/s, durante: 5s }

cenario:
  - http: DELETE /pedidos/1
`,
		dsl: func() (scenario.Scenario, error) {
			return dsl.New("Chave de api").
				Target("http://127.0.0.1:8080").
				Auth(dsl.WithHeaderAuth("X-API-Key: ${api_key}")).
				Plateau(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.DELETE("/pedidos/1")).
				Build()
		},
	},
}

func TestYAMLAndDSLProduceSameScenario(t *testing.T) {
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			doYAML, err := scenario.Parse([]byte(testCase.yaml))
			if err != nil {
				t.Fatalf("o YAML do caso nao carregou: %v", err)
			}
			if err := doYAML.Validate(); err != nil {
				t.Fatalf("o YAML do caso nao e valido: %v", err)
			}
			daDSL, err := testCase.dsl()
			if err != nil {
				t.Fatalf("a DSL do caso nao montou: %v", err)
			}

			expected, obtained := withoutLines(doYAML), withoutLines(daDSL)
			if reflect.DeepEqual(expected, obtained) {
				return
			}
			for _, difference := range diferencas(expected, obtained) {
				t.Errorf("%s", difference)
			}
		})
	}
}

// Sem esta trava, um protocolo novo nasceria com equivalencia nao verificada e a
// promessa dos dois publicos ficaria valendo so para o que ja existia.
func TestEveryRegisteredProtocolHasEquivalenceCase(t *testing.T) {
	exercised := map[string]bool{}
	for _, testCase := range testCases {
		built, err := testCase.dsl()
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		for _, step := range built.Steps {
			exercised[step.Protocol] = true
		}
	}
	for _, name := range protocol.Registered() {
		if !exercised[name] {
			t.Errorf("o protocolo %q nao tem caso de equivalencia YAML x DSL", name)
		}
	}
}

func TestEveryTopKeyHasEquivalenceCase(t *testing.T) {
	used := map[string]bool{}
	for _, testCase := range testCases {
		var document map[string]any
		if err := yaml.Unmarshal([]byte(testCase.yaml), &document); err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		for key := range document {
			used[key] = true
		}
	}
	for _, key := range scenario.TopKeys {
		if !used[key] {
			t.Errorf("a chave de topo %q nao aparece em nenhum caso de equivalencia", key)
		}
	}
}

func TestEveryScenarioShapeHasEquivalenceCase(t *testing.T) {
	origins := map[scenario.CaptureOrigin]bool{}
	assertions := map[scenario.AssertionKind]bool{}
	phases := map[scenario.PhaseKind]bool{}
	authObtains := map[scenario.AuthKind]bool{}
	consumePolicies := map[scenario.ConsumePolicy]bool{}
	metrics := map[string]bool{}
	operators := map[scenario.Operator]bool{}

	for _, testCase := range testCases {
		built, err := testCase.dsl()
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		for _, step := range built.Steps {
			for _, capture := range step.Captures {
				origins[capture.Origin] = true
			}
			for _, assertion := range step.Assertions {
				assertions[assertion.Kind] = true
				operators[assertion.Operator] = true
			}
			if len(step.Checks) > 0 {
				assertions[scenario.AssertionKind(scenario.CheckStatus)] = true
			}
		}
		for _, phase := range built.Load.Phases {
			phases[phase.Kind] = true
		}
		if built.Auth != nil {
			authObtains[built.Auth.Kind] = true
		}
		for _, source := range built.Data {
			consumePolicies[source.Consume] = true
		}
		for _, rule := range built.SLO {
			metrics[rule.Metrica] = true
		}
	}

	missing(t, "origem de captura", []scenario.CaptureOrigin{
		scenario.CaptureJSON, scenario.CaptureHeader, scenario.CaptureRegex,
		scenario.CaptureBody, scenario.CaptureStatus,
	}, origins)
	missing(t, "tipo de assercao", []scenario.AssertionKind{
		scenario.AssertBodyContains, scenario.AssertJSON, scenario.AssertRegex,
		scenario.AssertHeader, scenario.AssertionKind(scenario.CheckStatus),
	}, assertions)
	missing(t, "tipo de fase", []scenario.PhaseKind{
		scenario.PhaseRamp, scenario.PhasePlateau, scenario.PhaseSpike, scenario.PhaseConstant,
	}, phases)
	missing(t, "tipo de autenticacao", []scenario.AuthKind{
		scenario.AuthToken, scenario.AuthBasic, scenario.AuthHeader,
	}, authObtains)
	missing(t, "politica de consumo", []scenario.ConsumePolicy{
		scenario.ConsumeCircular, scenario.ConsumeSequential, scenario.ConsumeRandom, scenario.ConsumeUniquePerUser,
	}, consumePolicies)
	missing(t, "metrica de slo", []string{"p95", "p99", "max", "erros", "vazao"}, metrics)
	missing(t, "operador de comparacao", []scenario.Operator{
		scenario.OpEqual, scenario.OpGreater, scenario.OpExists, scenario.OpContains,
	}, operators)
}

func missing[T comparable](t *testing.T, subject string, expected []T, seen map[T]bool) {
	t.Helper()
	for _, expected := range expected {
		if !seen[expected] {
			t.Errorf("%s %v nao aparece em nenhum caso de equivalencia YAML x DSL", subject, expected)
		}
	}
}

func withoutLines(c scenario.Scenario) scenario.Scenario {
	clone := c
	clone.Steps = nil
	for _, step := range c.Steps {
		clone.Steps = append(clone.Steps, stepWithoutLines(step))
	}
	clone.Data = nil
	for _, source := range c.Data {
		source.Line = 0
		clone.Data = append(clone.Data, source)
	}
	clone.SLO = nil
	for _, rule := range c.SLO {
		rule.Line = 0
		clone.SLO = append(clone.SLO, rule)
	}
	clone.Load.Phases = nil
	for _, phase := range c.Load.Phases {
		phase.Line = 0
		clone.Load.Phases = append(clone.Load.Phases, phase)
	}
	if c.Auth != nil {
		auth := *c.Auth
		auth.Line = 0
		if auth.Obtain != nil {
			obtain := stepWithoutLines(*auth.Obtain)
			auth.Obtain = &obtain
		}
		clone.Auth = &auth
	}
	return clone
}

func stepWithoutLines(step scenario.Step) scenario.Step {
	clone := step
	clone.Line = 0
	clone.Captures = nil
	for _, capture := range step.Captures {
		capture.Line = 0
		clone.Captures = append(clone.Captures, capture)
	}
	clone.Assertions = nil
	for _, assertion := range step.Assertions {
		assertion.Line = 0
		clone.Assertions = append(clone.Assertions, assertion)
	}
	return clone
}

func diferencas(expected, obtained scenario.Scenario) []string {
	var findings []string
	compare := func(field string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			findings = append(findings, field+":\n  yaml: "+format(a)+"\n   dsl: "+format(b))
		}
	}
	compare("nome", expected.Name, obtained.Name)
	compare("alvo", expected.Target, obtained.Target)
	compare("variaveis", expected.Vars, obtained.Vars)
	compare("autenticacao", expected.Auth, obtained.Auth)
	compare("dados", expected.Data, obtained.Data)
	compare("carga", expected.Load, obtained.Load)
	compare("slo", expected.SLO, obtained.SLO)
	if len(expected.Steps) != len(obtained.Steps) {
		compare("quantidade de passos", len(expected.Steps), len(obtained.Steps))
		return findings
	}
	for index := range expected.Steps {
		compare(format(index)+" passo", expected.Steps[index], obtained.Steps[index])
	}
	return findings
}

func format(value any) string {
	text, err := yaml.Marshal(value)
	if err != nil {
		return "<nao formatavel>"
	}
	return string(text)
}
