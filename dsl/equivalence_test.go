package dsl_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/dsl"
	"github.com/Diegobraun/braunrate/internal/messaging"
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
	dsl  func() (scenario.Spec, error)
}

// The YAML and the DSL have to produce the SAME structure, not merely similar
// results: the built scenario is the only input the engine gets, so equality
// here is equality of measurement. Only the line number is ignored, since that
// is a position in a file and does not exist in Go code.
var testCases = []equivalence{
	{
		name: "http com variáveis, dados, autenticação por token, capturas e slo",
		yaml: `
name: Jornada autenticada
target: ${BASE:-http://127.0.0.1:8080}

variables:
  inquilino: acme

auth:
  type: token
  refreshAfter: 25m
  obtain:
    http:
      method: POST
      path: /auth/token
      body: { password: segredo, user: ana }
    capture:
      token: $.access_token

data:
  assinantes:
    file: dados/assinantes.csv
    consume: circular
  pedidos:
    generate: { id: uuid, value: "numero(10,500)" }
    seed: 7
  pagamento:
    generate:
      referencia: { type: pattern, format: "PED-######" }
      documento: { type: cpf }
      nonce: { type: uuid, newEvery: use }

load:
  profiles:
    - ramp: { from: 10/s, to: 100/s, duration: 30s }
    - steady: { rate: 100/s, duration: 1m }
    - spike: { rate: 600/m, duration: 10s }
    - steady: { rate: 36000/h, duration: 5s }

scenario:
  - http: GET /pedidos/${assinantes.id}
    name: consultar pedido
    capture:
      faturaId: $.ultimaFatura.id
      requestId: header:X-Request-Id
      sessao: /sessao=([a-z0-9]+)/
      cookieDeSessao: cookie:sessao
      codigo: status
      whole: body
      opcional: { from: $.talvez, default: vazio }
    expect:
      status: 200
      bodyContains: ABERTO
      bodyMatches: "pedido-[0-9]+"
      json: { $.status: ABERTO, $.total: "> 10", $.cupom: exists, $.tags: contains promo }
      header: { Content-Type: application/json }

  - http:
      method: POST
      path: /pedidos
      headers: { X-Inquilino: "${inquilino}" }
      body: { assinante: "${assinantes.id}", total: 199.9 }
      timeout: 2s
      followRedirects: false

slo:
  - consultar pedido: { p95: < 150ms, max: < 1s }
  - journey: { p95: < 2s, p99: < 5s }
  - global: { errors: < 0.1, success: ">= 99.9", throughput: "> 50/s" }
  - regression: { journeyP95: "<= 10% worse" }
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Jornada autenticada").
				Target("${BASE:-http://127.0.0.1:8080}").
				Variable("inquilino", "acme").
				Auth(dsl.WithToken(
					dsl.POST("/auth/token").Body(map[string]any{"user": "ana", "password": "segredo"}),
					dsl.Capture("token", "$.access_token"),
				).RefreshAfter(25*time.Minute)).
				DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(scenario.ConsumeCircular)).
				GeneratedData("pedidos", map[string]string{"id": "uuid", "value": "numero(10,500)"}, dsl.Seed(7)).
				GeneratedFields("pagamento", map[string]dsl.Field{
					"referencia": dsl.Pattern("PED-######"),
					"documento":  dsl.Generator("cpf"),
					"nonce":      dsl.Generator("uuid").NewPerUse(),
				}).
				Ramp(dsl.PerSecond(10), dsl.PerSecond(100), 30*time.Second).
				Steady(dsl.PerSecond(100), time.Minute).
				Spike(dsl.PerMinute(600), 10*time.Second).
				Steady(dsl.PerHour(36000), 5*time.Second).
				Step(dsl.GET("/pedidos/${assinantes.id}"),
					dsl.Name("consultar pedido"),
					dsl.Capture("faturaId", "$.ultimaFatura.id"),
					dsl.Capture("requestId", "header:X-Request-Id"),
					dsl.Capture("sessao", "/sessao=([a-z0-9]+)/"),
					dsl.Capture("cookieDeSessao", "cookie:sessao"),
					dsl.Capture("codigo", "status"),
					dsl.Capture("whole", "body"),
					dsl.CaptureWithDefault("opcional", "$.talvez", "vazio"),
					dsl.CheckStatus(200),
					dsl.CheckBodyContains("ABERTO"),
					dsl.CheckBodyMatches("pedido-[0-9]+"),
					dsl.CheckJSON("$.status", "ABERTO"),
					dsl.CheckJSON("$.total", "> 10"),
					dsl.CheckJSON("$.cupom", "exists"),
					dsl.CheckJSON("$.tags", "contains promo"),
					dsl.CheckHeader("Content-Type", "application/json"),
				).
				Step(dsl.POST("/pedidos").
					Header("X-Inquilino", "${inquilino}").
					Body(map[string]any{"assinante": "${assinantes.id}", "total": 199.9}).
					Timeout(2*time.Second).
					FollowRedirects(false)).
				SLO("consultar pedido", "p95", "< 150ms").
				SLO("consultar pedido", "max", "< 1s").
				JourneySLO("p95", "< 2s").
				JourneySLO("p99", "< 5s").
				OverallSLO("errors", "< 0.1").
				OverallSLO("success", ">= 99.9").
				OverallSLO("throughput", "> 50/s").
				RegressionSLO("journeyP95", "<= 10% worse").
				Build()
		},
	},
	{
		name: "graphql por operação",
		yaml: `
name: Cobranca em GraphQL
target: http://127.0.0.1:8080

data:
  assinantes: { file: dados/assinantes.csv, consume: circular }

load:
  profiles:
    - steady: { rate: 50/s, duration: 20s }

scenario:
  - graphql:
      query: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }
      variables: { id: "${assinantes.id}" }
      path: /graphql
      headers: { X-Origem: braunrate }
      timeout: 3s
    expect:
      json: { $.data.pedido.status: ABERTO }

  - graphql: |
      mutation PagarFatura($id: ID!) { pagarFatura(id: $id) { status } }

slo:
  - graphql ConsultarPedido: { p99: < 300ms }
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Cobranca em GraphQL").
				Target("http://127.0.0.1:8080").
				DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(scenario.ConsumeCircular)).
				Steady(dsl.PerSecond(50), 20*time.Second).
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
name: Cadeia assíncrona
target: 127.0.0.1:9092
requires: [kafka]

data:
  pedidos:
    generate: { id: uuid }
    consume: sequential

load:
  profiles:
    - steady: { rate: 100/s, duration: 10s }

scenario:
  - kafka:
      topic: pedidos-cadeia
      key: "${pedidos.id}"
      value: { pedido: "${pedidos.id}" }
      headers: { origem: braunrate }
      brokers: [127.0.0.1:9092]
      acks: leader
      timeout: 5s
      partition: 2
      group: cobranca

  - await:
      kafka: { topic: pedidos-processados, brokers: [127.0.0.1:9092] }
      key: "${pedidos.id}"
      field: $.pedido
      timeout: 10s

  - await:
      http: { path: "/pedidos/${pedidos.id}" }
      until: { $.status: PROCESSADO }
      interval: 200ms
      timeout: 30s

slo:
  - kafka produce pedidos-cadeia: { p95: < 100ms }
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Cadeia assíncrona").
				Target("127.0.0.1:9092").
				Requires("kafka").
				GeneratedData("pedidos", map[string]string{"id": "uuid"}, dsl.Consume(scenario.ConsumeSequential)).
				Steady(dsl.PerSecond(100), 10*time.Second).
				Step(dsl.Kafka("pedidos-cadeia").
					Key("${pedidos.id}").
					Value(map[string]any{"pedido": "${pedidos.id}"}).
					Header("origem", "braunrate").
					Brokers("127.0.0.1:9092").
					Acks("leader").
					Timeout(5*time.Second).
					Partition(2).
					Group("cobranca")).
				Step(dsl.WaitForKafka("pedidos-processados").
					Addresses("127.0.0.1:9092").
					Key("${pedidos.id}").
					Field("$.pedido").
					Timeout(10*time.Second)).
				Step(dsl.WaitForHTTP("/pedidos/${pedidos.id}").
					UntilJSON("$.status", "PROCESSADO").
					Interval(200*time.Millisecond).
					Timeout(30*time.Second)).
				SLO("kafka produce pedidos-cadeia", "p95", "< 100ms").
				Build()
		},
	},
	{
		name: "amqp em fila e em troca com rota",
		yaml: `
name: Publicação em RabbitMQ
target: amqp://127.0.0.1:5672

data:
  clientes:
    file: dados/clientes.csv
    consume: random

load:
  profiles:
    - steady: { rate: 30/s, duration: 10s }

scenario:
  - amqp:
      queue: pedidos
      messageId: "${clientes.id}"
      body: { cliente: "${clientes.id}" }
      headers: { origem: braunrate }
      url: amqp://127.0.0.1:5672
      persistent: false
      confirm: false
      timeout: 4s

  - amqp:
      exchange: cobranca
      routingKey: fatura.emitida
      body: texto puro

  - await:
      amqp: pedidos-processados
      key: "${clientes.id}"
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Publicação em RabbitMQ").
				Target("amqp://127.0.0.1:5672").
				DataFromFile("clientes", "dados/clientes.csv", dsl.Consume(scenario.ConsumeRandom)).
				Steady(dsl.PerSecond(30), 10*time.Second).
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
		name: "autenticação básica e consumo único por usuário",
		yaml: `
name: Basica
target: http://127.0.0.1:8080

auth:
  type: basic
  user: ana
  password: segredo

data:
  assinantes:
    file: dados/assinantes.csv
    consume: uniquePerUser

load:
  profiles:
    - steady: { rate: 10/s, duration: 5s }

scenario:
  - http: GET /pedidos
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Basica").
				Target("http://127.0.0.1:8080").
				Auth(dsl.Basic("ana", "segredo")).
				DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(scenario.ConsumeUniquePerUser)).
				Steady(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.GET("/pedidos")).
				Build()
		},
	},
	{
		name: "autenticação por cabeçalho fixo",
		yaml: `
name: Chave de api
target: http://127.0.0.1:8080

variables:
  api_key: "${API_KEY:-chave-de-teste}"

auth:
  type: header
  header: "X-API-Key: ${api_key}"

load:
  profiles:
    - steady: { rate: 10/s, duration: 5s }

scenario:
  - http: DELETE /pedidos/1
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Chave de api").
				Target("http://127.0.0.1:8080").
				Variable("api_key", "${API_KEY:-chave-de-teste}").
				Auth(dsl.WithHeaderAuth("X-API-Key: ${api_key}")).
				Steady(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.DELETE("/pedidos/1")).
				Build()
		},
	},
	{
		name: "semente vinda do ambiente",
		yaml: `
name: Semente do ambiente
target: http://127.0.0.1:8080

data:
  pedidos:
    generate: { id: uuid }
    seed: ${SEMENTE_DE_TESTE:-42}

load:
  profiles:
    - steady: { rate: 10/s, duration: 5s }

scenario:
  - http: GET /pedidos/${pedidos.id}
    name: consultar pedido
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Semente do ambiente").
				Target("http://127.0.0.1:8080").
				GeneratedData("pedidos", map[string]string{"id": "uuid"}, dsl.SeedFromEnv("SEMENTE_DE_TESTE", 42)).
				Steady(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.GET("/pedidos/${pedidos.id}"), dsl.Name("consultar pedido")).
				Build()
		},
	},
	{
		name: "mix ponderado de operações",
		yaml: `
name: Mix de operações
target: http://127.0.0.1:8080

load:
  profiles:
    - steady: { rate: 100/s, duration: 5s }

scenario:
  - http: GET /pedidos
    name: consulta leve
    weight: 60
  - http: GET /pedidos/1/detalhe
    name: consulta pesada
    weight: 30
  - http: { method: POST, path: /pedidos }
    name: criacao
    weight: 10
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Mix de operações").
				Target("http://127.0.0.1:8080").
				Steady(dsl.PerSecond(100), 5*time.Second).
				Step(dsl.GET("/pedidos"), dsl.Name("consulta leve"), dsl.Weight(60)).
				Step(dsl.GET("/pedidos/1/detalhe"), dsl.Name("consulta pesada"), dsl.Weight(30)).
				Step(dsl.POST("/pedidos"), dsl.Name("criacao"), dsl.Weight(10)).
				Build()
		},
	},
	{
		name: "alvo https com CA própria",
		yaml: `
name: Homologacao atrás de CA própria
target: https://api.homolog.interno

tls: { ca: /etc/ssl/ca-interna.pem, certificate: /etc/ssl/cliente.pem, key: /etc/ssl/cliente.key }

load:
  profiles:
    - steady: { rate: 10/s, duration: 5s }

scenario:
  - http: GET /pedidos/1
    name: consultar pedido
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Homologacao atrás de CA própria").
				Target("https://api.homolog.interno").
				TargetTLS(messaging.TLS{
					CA:          "/etc/ssl/ca-interna.pem",
					Certificate: "/etc/ssl/cliente.pem",
					Key:         "/etc/ssl/cliente.key",
				}).
				Steady(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.GET("/pedidos/1"), dsl.Name("consultar pedido")).
				Build()
		},
	},
	{
		name: "mensageria autenticada",
		yaml: `
name: Cobranca autenticada
target: kafka.homolog:9093

requires: [kafka, credential]

messaging:
  kafka:
    brokers: [kafka.homolog:9093]
    auth: { type: scramSha512, user: "${KAFKA_USUARIO}", password: "${KAFKA_SENHA}" }
    tls: { ca: /etc/ssl/ca.pem }

load:
  profiles:
    - steady: { rate: 10/s, duration: 5s }

scenario:
  - kafka: { topic: pedidos, value: "{}" }
    name: publicar pedido
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Cobranca autenticada").
				Target("kafka.homolog:9093").
				Requires("kafka", "credential").
				KafkaBroker(dsl.BrokerAt("kafka.homolog:9093").
					SCRAM512("${KAFKA_USUARIO}", "${KAFKA_SENHA}").
					CA("/etc/ssl/ca.pem")).
				Steady(dsl.PerSecond(10), 5*time.Second).
				Step(dsl.Kafka("pedidos").Value("{}"), dsl.Name("publicar pedido")).
				Build()
		},
	},
	{
		name: "modelo fechado",
		yaml: `
name: Laço fechado
target: http://127.0.0.1:8080

load:
  model: closed
  users: 200
  duration: 5m
  thinkTime: 1s

scenario:
  - http: GET /pedidos
`,
		dsl: func() (scenario.Spec, error) {
			return dsl.New("Laço fechado").
				Target("http://127.0.0.1:8080").
				ClosedLoop(200, 5*time.Minute, time.Second).
				Step(dsl.GET("/pedidos")).
				Build()
		},
	},
}

func TestYAMLAndDSLProduceSameScenario(t *testing.T) {
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			doYAML, err := scenario.Parse([]byte(testCase.yaml))
			if err != nil {
				t.Fatalf("o YAML do caso não carregou: %v", err)
			}
			if err := doYAML.Validate(); err != nil {
				t.Fatalf("o YAML do caso não e válido: %v", err)
			}
			daDSL, err := testCase.dsl()
			if err != nil {
				t.Fatalf("a DSL do caso não montou: %v", err)
			}

			expected, obtained := withoutLines(doYAML), withoutLines(daDSL)
			if reflect.DeepEqual(expected, obtained) {
				return
			}
			differences := diferencas(expected, obtained)
			if len(differences) == 0 {
				t.Fatalf("os dois cenários diferem e a comparação campo a campo não viu onde: "+
					"um campo novo de scenario.Spec entrou sem entrar em diferencas().\n  yaml: %s\n   dsl: %s",
					format(expected), format(obtained))
			}
			for _, difference := range differences {
				t.Errorf("%s", difference)
			}
		})
	}
}

// Without this lock a new protocol would be born with unverified equivalence
// and the two-audience promise would only hold for what already existed.
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
			t.Errorf("o protocolo %q não tem caso de equivalencia YAML x DSL", name)
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
			t.Errorf("a chave de topo %q não aparece em nenhum caso de equivalencia", key)
		}
	}
}

func TestEveryScenarioShapeHasEquivalenceCase(t *testing.T) {
	origins := map[scenario.CaptureOrigin]bool{}
	assertions := map[scenario.AssertionKind]bool{}
	phases := map[scenario.PhaseKind]bool{}
	authObtains := map[scenario.AuthKind]bool{}
	models := map[scenario.ArrivalModel]bool{}
	consumePolicies := map[scenario.ConsumePolicy]bool{}
	metrics := map[string]bool{}
	scopes := map[scenario.SLOScope]bool{}
	generators := map[string]bool{}
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
		models[built.Load.Model] = true
		for _, phase := range built.Load.Phases {
			phases[phase.Kind] = true
		}
		if built.Auth != nil {
			authObtains[built.Auth.Kind] = true
		}
		for _, source := range built.Data {
			consumePolicies[source.Consume] = true
			for _, field := range source.Fields {
				generators[field.Recipe] = true
				if field.PerUse {
					generators["newEvery: use"] = true
				}
			}
		}
		for _, rule := range built.SLO {
			metrics[rule.Metric] = true
			scopes[rule.Scope] = true
		}
	}

	missing(t, "origem de captura", []scenario.CaptureOrigin{
		scenario.CaptureJSON, scenario.CaptureHeader, scenario.CaptureCookie,
		scenario.CaptureRegex, scenario.CaptureBody, scenario.CaptureStatus,
	}, origins)
	missing(t, "tipo de assercao", []scenario.AssertionKind{
		scenario.AssertBodyContains, scenario.AssertJSON, scenario.AssertRegex,
		scenario.AssertHeader, scenario.AssertionKind(scenario.CheckStatus),
	}, assertions)
	missing(t, "tipo de fase", []scenario.PhaseKind{
		scenario.PhaseRamp, scenario.PhaseSteady, scenario.PhaseSpike, scenario.PhaseSteady,
	}, phases)
	missing(t, "modelo de chegada", []scenario.ArrivalModel{
		scenario.OpenArrival, scenario.ClosedArrival,
	}, models)
	missing(t, "tipo de autenticação", []scenario.AuthKind{
		scenario.AuthToken, scenario.AuthBasic, scenario.AuthHeader,
	}, authObtains)
	missing(t, "politica de consumo", []scenario.ConsumePolicy{
		scenario.ConsumeCircular, scenario.ConsumeSequential, scenario.ConsumeRandom, scenario.ConsumeUniquePerUser,
	}, consumePolicies)
	missing(t, "métrica de slo", []string{"p95", "p99", "max", "errors", "success", "throughput", "journeyP95"}, metrics)
	missing(t, "gerador de dados", []string{"uuid", "pattern", "cpf", "newEvery: use"}, generators)
	missing(t, "escopo de slo", []scenario.SLOScope{
		scenario.ScopeStep, scenario.ScopeOverall, scenario.ScopeJourney, scenario.ScopeRegression,
	}, scopes)
	missing(t, "operador de comparação", []scenario.Operator{
		scenario.OpEqual, scenario.OpGreater, scenario.OpExists, scenario.OpContains,
	}, operators)
}

func missing[T comparable](t *testing.T, subject string, expected []T, seen map[T]bool) {
	t.Helper()
	for _, expected := range expected {
		if !seen[expected] {
			t.Errorf("%s %v não aparece em nenhum caso de equivalencia YAML x DSL", subject, expected)
		}
	}
}

func withoutLines(c scenario.Spec) scenario.Spec {
	clone := c
	// What the machine had, not what the file says: the YAML side records which
	// environment variables were missing when it was read, and the DSL has no
	// file to read.
	clone.MissingEnvironment = nil
	clone.Steps = nil
	for _, step := range c.Steps {
		clone.Steps = append(clone.Steps, stepWithoutLines(step))
	}
	if c.Messaging != nil {
		settings := *c.Messaging
		if settings.Kafka != nil {
			broker := *settings.Kafka
			broker.Line = 0
			settings.Kafka = &broker
		}
		if settings.AMQP != nil {
			broker := *settings.AMQP
			broker.Line = 0
			settings.AMQP = &broker
		}
		clone.Messaging = &settings
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

func diferencas(expected, obtained scenario.Spec) []string {
	var findings []string
	compare := func(field string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			findings = append(findings, field+":\n  yaml: "+format(a)+"\n   dsl: "+format(b))
		}
	}
	compare("nome", expected.Name, obtained.Name)
	compare("alvo", expected.Target, obtained.Target)
	compare("requer", expected.Requires, obtained.Requires)
	compare("variaveis", expected.Vars, obtained.Vars)
	compare("autenticacao", expected.Auth, obtained.Auth)
	compare("mensageria", expected.Messaging, obtained.Messaging)
	compare("tls", expected.TLS, obtained.TLS)
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
		return "<não formatavel>"
	}
	return string(text)
}

// The ADR 0009 promise is that adding a key to the YAML without adding it to
// the DSL breaks the build. The locks above cover the shape of the scenario,
// not the options of each protocol — and `particao` and `grupo` went into the
// Kafka step without a DSL method, in silence. This one walks the fields of
// every protocol config and demands that each of them be exercised somewhere.
func TestEveryProtocolConfigFieldHasEquivalenceCase(t *testing.T) {
	touched := map[string]map[string]bool{}
	shapes := map[string]reflect.Type{}

	for _, testCase := range testCases {
		built, err := testCase.dsl()
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		for _, step := range built.Steps {
			value := reflect.ValueOf(step.Config)
			for value.Kind() == reflect.Pointer {
				if value.IsNil() {
					break
				}
				value = value.Elem()
			}
			if value.Kind() != reflect.Struct {
				continue
			}
			shapes[step.Protocol] = value.Type()
			if touched[step.Protocol] == nil {
				touched[step.Protocol] = map[string]bool{}
			}
			for index := 0; index < value.NumField(); index++ {
				if !value.Field(index).IsZero() {
					touched[step.Protocol][value.Type().Field(index).Name] = true
				}
			}
		}
	}

	for protocolName, shape := range shapes {
		for index := 0; index < shape.NumField(); index++ {
			field := shape.Field(index)
			if !field.IsExported() {
				continue
			}
			if !touched[protocolName][field.Name] {
				t.Errorf("o campo %s.%s nunca aparece num caso de equivalencia: se a DSL não souber declarar, o cenário em Go não consegue dizer o que o YAML diz",
					protocolName, field.Name)
			}
		}
	}
}

// The undeclared-variable refusal was born reading the YAML text, so it never
// reached the scenario written in Go: the same file refused in one public was
// accepted in the other, and the request went out with an empty field. ADR 0002
// says validation runs on the model, one place, same message for both.
func TestUndeclaredVariableIsRefusedInTheDSLToo(t *testing.T) {
	cases := map[string]func() (scenario.Spec, error){
		"no caminho": func() (scenario.Spec, error) {
			return dsl.New("x").Target("http://127.0.0.1:8080").
				Steady(dsl.PerSecond(1), time.Second).
				Step(dsl.GET("/pedidos/${nao_declarada}"), dsl.Name("consultar")).Build()
		},
		"no corpo": func() (scenario.Spec, error) {
			return dsl.New("x").Target("http://127.0.0.1:8080").
				Steady(dsl.PerSecond(1), time.Second).
				Step(dsl.POST("/pedidos").Body(map[string]any{"cupom": "${nao_declarada}"}), dsl.Name("criar")).Build()
		},
		"no cabecalho": func() (scenario.Spec, error) {
			return dsl.New("x").Target("http://127.0.0.1:8080").
				Steady(dsl.PerSecond(1), time.Second).
				Step(dsl.GET("/pedidos").Header("X-Inquilino", "${nao_declarada}"), dsl.Name("consultar")).Build()
		},
		"na chave do kafka": func() (scenario.Spec, error) {
			return dsl.New("x").Target("127.0.0.1:9092").
				Steady(dsl.PerSecond(1), time.Second).
				Step(dsl.Kafka("pedidos").Key("${nao_declarada}").Value(map[string]any{"id": "1"}), dsl.Name("publicar")).Build()
		},
	}

	for name, build := range cases {
		_, err := build()
		if err == nil {
			t.Errorf("%s: a DSL aceitou ${nao_declarada}; o mesmo cenário em YAML e recusado", name)
			continue
		}
		if !strings.Contains(err.Error(), "I do not know where ${nao_declarada} comes from") {
			t.Errorf("%s: a mensagem não e a mesma do YAML: %v", name, err)
		}
		if strings.Contains(err.Error(), "line 0") {
			t.Errorf("%s: cenário em Go não tem linha para apontar: %v", name, err)
		}
	}
}

// The same rule cannot start refusing what the scenario does declare: a name
// from the environment, a captured value, a field of a declared source.
func TestDeclaredVariablesKeepPassingInTheDSL(t *testing.T) {
	_, err := dsl.New("x").Target("http://127.0.0.1:8080").
		Variable("inquilino", "acme").
		GeneratedData("pedidos", map[string]string{"id": "uuid"}).
		Steady(dsl.PerSecond(1), time.Second).
		Step(dsl.GET("/pedidos/${pedidos.id}"),
			dsl.Name("consultar"),
			dsl.Capture("faturaId", "$.ultimaFatura.id")).
		Step(dsl.POST("/faturas/${faturaId}").
			Header("X-Inquilino", "${inquilino}").
			Body(map[string]any{"chave": "${API_KEY}"}), dsl.Name("pagar")).
		Build()
	if err != nil {
		t.Fatalf("cenário com tudo declarado foi recusado: %v", err)
	}
}
