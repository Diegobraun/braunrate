package scenario_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

func parseWithSteps(t *testing.T, blocks, step string) (scenario.Spec, error) {
	t.Helper()
	return scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8080
` + blocks + `
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - ` + step + `
`))
}

// Silence was the whole problem: the field went out empty, the target answered
// 401, and nothing connected the two.
func TestUndeclaredVariableIsRefusedWithTheLineAndWhereToDeclareIt(t *testing.T) {
	_, err := parseWithSteps(t, "", "http: GET /pedidos/${variavel_que_ninguem_declarou}")
	if err == nil {
		t.Fatal("a variável inventada passou na validação")
	}

	message := err.Error()
	for _, fragment := range []string{
		"I do not know where ${variavel_que_ninguem_declarou} comes from",
		"variables:", "capture:", "data:", "UPPERCASE",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("a mensagem não ensina %q: %v", fragment, err)
		}
	}
	if position, is := err.(scenario.ScenarioError); !is || position.Line == 0 || position.Column == 0 {
		t.Fatalf("o erro não aponta linha e coluna: %#v", err)
	}
}

// Pointing at the start of the line would send the person to the first
// reference when the broken one is the third.
func TestTheColumnPointsAtTheReferenceThatFailedNotAtTheLine(t *testing.T) {
	_, err := parseWithSteps(t, "variables:\n  a: 1\n  b: 2\n",
		"http: GET /${a}/${b}/${quebrada}")
	position, is := err.(scenario.ScenarioError)
	if !is {
		t.Fatalf("erro sem posição: %v", err)
	}
	if position.Column < 25 {
		t.Fatalf("a coluna %d aponta antes da referência quebrada, que começa depois de ${a} e ${b}", position.Column)
	}
}

func TestSimilarNameIsSuggested(t *testing.T) {
	_, err := parseWithSteps(t, "variables:\n  faturaId: 7\n", "http: GET /faturas/${faturald}")
	if err == nil || !strings.Contains(err.Error(), `did you mean "faturaId"?`) {
		t.Fatalf("a sugestao não apareceu: %v", err)
	}
}

func TestEveryDeclaredOriginResolves(t *testing.T) {
	blocks := `
variables:
  tenant: acme

data:
  pedidos: { generate: { id: { type: pattern, format: "PED-######" } } }
  assinantes: { file: dados/assinantes.csv }
`
	step := `http: GET /${tenant}/${pedidos.id}/${assinantes.qualquer_coluna}/${faturaId}
    capture: { faturaId: $.fatura.id }`

	if _, err := parseWithSteps(t, blocks, step); err != nil {
		t.Fatalf("origem declarada foi recusada: %v", err)
	}
}

// The tool writes ${API_KEY} itself, in 'import curl' and in the messaging
// examples. Checking the environment instead of the case would make 'validate'
// impossible on a machine without the secret.
func TestUpperCaseNameComesFromTheEnvironmentWithoutBeingDeclared(t *testing.T) {
	if _, err := parseWithSteps(t, "", `http: { method: GET, path: /pedidos, headers: { Authorization: "Bearer ${API_KEY}" } }`); err != nil {
		t.Fatalf("referência de ambiente foi recusada: %v", err)
	}
}

func TestDefaultValueAlwaysResolves(t *testing.T) {
	if _, err := parseWithSteps(t, "", "http: GET /${nunca_declarada:-1}"); err != nil {
		t.Fatalf("referência com reserva foi recusada: %v", err)
	}
}

func TestUnknownDataSourceSaysWhichOnesExist(t *testing.T) {
	_, err := parseWithSteps(t, "data:\n  assinantes: { file: dados/assinantes.csv }\n",
		"http: GET /${assinante.id}")
	if err == nil || !strings.Contains(err.Error(), `did you mean "assinantes"?`) {
		t.Fatalf("a fonte parecida não foi sugerida: %v", err)
	}
}

// A CSV brings its columns from the file, so only a synthetic source can have
// its field names checked here.
func TestSyntheticSourceChecksTheFieldAndTheCSVDoesNot(t *testing.T) {
	_, err := parseWithSteps(t, "data:\n  pedidos: { generate: { id: { type: pattern, format: \"PED-######\" } } }\n",
		"http: GET /${pedidos.identificador}")
	if err == nil || !strings.Contains(err.Error(), `the source "pedidos" does not generate the field "identificador"`) {
		t.Fatalf("o campo inexistente da fonte sintetica passou: %v", err)
	}

	if _, err := parseWithSteps(t, "data:\n  assinantes: { file: dados/assinantes.csv }\n",
		"http: GET /${assinantes.coluna_que_so_o_arquivo_conhece}"); err != nil {
		t.Fatalf("coluna de CSV foi recusada sem o arquivo ter sido lido: %v", err)
	}
}

// A7 of the audit: a literal path never goes through interpolation, so it never
// enters the observed-variety check. The run hits the same URL thousands of
// times and nothing in the report says so.
func TestStepWithNothingVaryingIsWarnedAbout(t *testing.T) {
	spec, err := parseWithSteps(t, "", "http: GET /pedidos/1\n    name: consultar pedido")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	warnings := strings.Join(scenario.FixedStepWarnings(spec), "\n")
	if !strings.Contains(warnings, "consultar pedido") || !strings.Contains(warnings, "identical") {
		t.Fatalf("o passo fixo não foi avisado:\n%s", warnings)
	}
	if !strings.Contains(warnings, "${orders.id}") {
		t.Fatalf("o aviso não mostra como variar:\n%s", warnings)
	}
}

func TestStepThatVariesIsNotWarnedAbout(t *testing.T) {
	spec, err := parseWithSteps(t, "data:\n  pedidos: { file: pedidos.csv }\n",
		"http: GET /pedidos/${pedidos.id}\n    name: consultar pedido")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	if warnings := scenario.FixedStepWarnings(spec); len(warnings) > 0 {
		t.Fatalf("passo que varia foi avisado como fixo: %v", warnings)
	}
}

// A interface precisa dar valor ao ${TOKEN} sem escrever o segredo no arquivo.
// ResolveWith injeta o valor de sessao onde a variavel de ambiente entraria: o
// arquivo mantem ${TOKEN}, o nome deixa de contar como ausente, e a resolucao em
// tempo de execucao o encontra. Ver ADR 0021.
func TestResolveWithSatisfiesAMissingVariableWithoutTouchingTheFile(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8080

load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

auth:
  type: header
  header: "Authorization: Bearer ${TOKEN}"

scenario:
  - http: GET /pedido
    name: pedido
`))
	if err != nil {
		t.Fatalf("o cenário não carregou: %v", err)
	}
	if len(spec.MissingEnvironment) != 1 || spec.MissingEnvironment[0] != "TOKEN" {
		t.Fatalf("TOKEN deveria faltar antes de fornecer: %v", spec.MissingEnvironment)
	}

	spec.ResolveWith(map[string]string{"TOKEN": "s3cr3t"})

	if len(spec.MissingEnvironment) != 0 {
		t.Errorf("TOKEN ainda consta como ausente depois de fornecido: %v", spec.MissingEnvironment)
	}
	if spec.Vars["TOKEN"] != "s3cr3t" {
		t.Errorf("o valor de sessão não chegou aos valores da execução: %v", spec.Vars)
	}
	// O texto do cabeçalho continua ${TOKEN}: o segredo nunca entra no arquivo.
	if spec.Auth == nil || !strings.Contains(spec.Auth.Header, "${TOKEN}") {
		t.Errorf("o cabeçalho perdeu o ${TOKEN}: %+v", spec.Auth)
	}
}

func TestResolveWithLeavesOtherMissingVariablesAlone(t *testing.T) {
	spec := scenario.Spec{MissingEnvironment: []string{"OUTRA", "TOKEN"}}
	spec.ResolveWith(map[string]string{"TOKEN": "s3cr3t"})
	if len(spec.MissingEnvironment) != 1 || spec.MissingEnvironment[0] != "OUTRA" {
		t.Errorf("só TOKEN deveria sair da lista: %v", spec.MissingEnvironment)
	}
}

// Credencial de broker se resolve na leitura, do ambiente, e nao pelo runtime.
// A interface nao pode fingir que a preenche: injetar limparia o aviso sem
// entregar o valor, e o run comecaria para falhar na conexao. Ver ADR 0021.
// A sessao preenche credencial de broker onde a variavel de ambiente entraria, e
// o valor chega ao campo de conexao que o cliente le — nao aos Vars de HTTP.
func TestBrokerCredentialsAreFilledFromTheSession(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8080

messaging:
  kafka:
    brokers: ["kafka.staging:9093"]
    auth:
      type: scramSha512
      user: "${KAFKA_USER}"
      password: "${KAFKA_PASSWORD}"

auth:
  type: header
  header: "Authorization: Bearer ${TOKEN}"

load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - kafka: { topic: pedidos, key: "k", value: { id: "1" } }
    name: publicar
`))
	if err != nil {
		t.Fatalf("cenário não carregou: %v", err)
	}

	got := map[string]bool{}
	for _, name := range spec.SessionCredentials() {
		got[name] = true
	}
	if !got["TOKEN"] || !got["KAFKA_USER"] || !got["KAFKA_PASSWORD"] {
		t.Fatalf("a sessão deveria poder preencher HTTP e broker: %v", spec.SessionCredentials())
	}

	spec.ResolveWith(map[string]string{"KAFKA_PASSWORD": "x", "KAFKA_USER": "y", "TOKEN": "t"})

	if spec.Messaging.Kafka.Auth.User != "y" || spec.Messaging.Kafka.Auth.Password != "x" {
		t.Errorf("a credencial de broker da sessão não chegou à conexão: user=%q password=%q",
			spec.Messaging.Kafka.Auth.User, spec.Messaging.Kafka.Auth.Password)
	}
	if len(spec.MissingEnvironment) != 0 {
		t.Errorf("nada deveria faltar após a sessão preencher tudo: %v", spec.MissingEnvironment)
	}
}
