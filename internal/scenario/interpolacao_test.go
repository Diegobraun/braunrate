package scenario_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/scenario"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
)

// A regra e uma so, e o teste cobre campos de blocos diferentes de proposito:
// se a interpolacao voltar a ser campo a campo, um destes reprova.
func TestEnvironmentVariableWorksInEveryScalarFieldOfTheScenario(t *testing.T) {
	t.Setenv("ALVO", "http://127.0.0.1:9090")
	t.Setenv("TAXA", "120")
	t.Setenv("DURACAO", "3s")
	t.Setenv("TOPICO", "pedidos-homolog")
	t.Setenv("NOME_DO_PASSO", "produzir pedido")

	spec, err := scenario.Parse([]byte(`
name: interpolacao
target: ${ALVO}
messaging:
  kafka:
    brokers: [kafka.homolog:9093]
load:
  profiles:
    - steady: { rate: "${TAXA}/s", duration: "${DURACAO}" }
scenario:
  - kafka: { topic: "${TOPICO}", value: "{}" }
    name: ${NOME_DO_PASSO}
`))
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	if spec.Target != "http://127.0.0.1:9090" {
		t.Fatalf("alvo saiu %q", spec.Target)
	}
	phase := spec.Load.Phases[0]
	if phase.To != 120 || phase.For != 3*time.Second {
		t.Fatalf("taxa e duração não vieram do ambiente: %+v", phase)
	}
	if key := spec.Steps[0].Config.AggregationKey(); !strings.Contains(key, "pedidos-homolog") {
		t.Fatalf("o tópico não veio do ambiente: %q", key)
	}
	if spec.Steps[0].Name != "produzir pedido" {
		t.Fatalf("o nome do passo saiu %q", spec.Steps[0].Name)
	}
}

// Sem valor de reserva, o arquivo do repositorio nao roda em maquina nenhuma
// sem a variavel — e quem escreve o cenario decide se isso e o que quer.
func TestTheDefaultValueAnswersWhenTheEnvironmentIsSilent(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: reserva
target: ${ALVO_SEM_DONO:-http://127.0.0.1:8080}
load:
  profiles:
    - steady: { rate: "${TAXA_SEM_DONO:-50}/s", duration: "${DURACAO_SEM_DONO:-2s}" }
scenario:
  - http: GET /pedidos/1
`))
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	if spec.Target != "http://127.0.0.1:8080" {
		t.Fatalf("alvo saiu %q", spec.Target)
	}
	phase := spec.Load.Phases[0]
	if phase.To != 50 || phase.For != 2*time.Second {
		t.Fatalf("os valores de reserva não chegaram na load: %+v", phase)
	}
}

// "taxa invalida: ${TAXA}/s" manda procurar erro de sintaxe onde falta uma
// variavel de ambiente. A frase que resolve nomeia a variavel.
func TestAFieldLeftWithARawReferenceSaysWhichVariableIsMissing(t *testing.T) {
	_, err := scenario.Parse([]byte(`
name: sem variavel
target: http://127.0.0.1:8080
load:
  profiles:
    - steady: { rate: "${TAXA_QUE_NINGUEM_DEFINIU}/s", duration: 1s }
scenario:
  - http: GET /pedidos/1
`))
	if err == nil {
		t.Fatal("a taxa com referência crua passou")
	}
	for _, fragment := range []string{
		"TAXA_QUE_NINGUEM_DEFINIU is not set",
		"run with TAXA_QUE_NINGUEM_DEFINIU=...",
		"${TAXA_QUE_NINGUEM_DEFINIU:-value}",
	} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("a mensagem não ensina %q: %v", fragment, err)
		}
	}
}

// O alvo era o unico campo que ja aceitava ${VARIAVEL}, e o unico que apagava a
// referencia quando ela nao resolvia: a pessoa escrevia o alvo e a ferramenta
// respondia que faltava alvo.
func TestAnUndefinedTargetVariableIsNamedInsteadOfBecomingAMissingTarget(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: alvo do ambiente
target: ${ALVO_QUE_NINGUEM_DEFINIU}
load:
  profiles:
    - steady: { rate: 10/s, duration: 1s }
scenario:
  - http: GET /pedidos/1
`))
	if err != nil {
		t.Fatalf("o erro do alvo devia sair na validação, não na leitura: %v", err)
	}
	invalid := spec.Validate()
	if invalid == nil {
		t.Fatal("o cenário com alvo sem variável foi aceito")
	}
	problems := invalid.Error()
	if strings.Contains(problems, "o cenário precisa de um alvo") {
		t.Fatalf("a ferramenta apagou a referência e pediu um alvo que a pessoa escreveu:\n%s", problems)
	}
	for _, fragment := range []string{
		"ALVO_QUE_NINGUEM_DEFINIU, which is not set",
		"run with ALVO_QUE_NINGUEM_DEFINIU=...",
	} {
		if !strings.Contains(problems, fragment) {
			t.Fatalf("a mensagem não ensina %q:\n%s", fragment, problems)
		}
	}
}

// A caixa do nome e o que separa ambiente de arquivo e de captura, e continua
// separando: ${pedidos.id} nao pode virar texto na leitura, porque ele muda a
// cada iteracao.
func TestLowercaseReferencesStayForTheEngineToResolvePerIteration(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: caixa
target: http://127.0.0.1:8080
data:
  pedidos:
    generate: { id: numero(1,10) }
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /pedidos/${pedidos.id}
`))
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	if key := spec.Steps[0].Config.AggregationKey(); !strings.Contains(key, "${pedidos.id}") {
		t.Fatalf("a referência por iteração foi resolvida na leitura: %q", key)
	}
}

// Duas excecoes a regra, e as duas porque o texto cru do campo faz parte do que
// ele significa. Com a variavel definida no ambiente, expandir antes esconderia
// a recusa de segredo literal — e a recusa continua sendo o ponto.
func TestCredentialAndSeedKeepTheirRawTextEvenWithTheVariableSet(t *testing.T) {
	t.Setenv("KAFKA_SENHA", "p4ssw0rd")
	t.Setenv("SEMENTE", "8817")

	_, err := scenario.Parse([]byte(`
name: credential
target: http://127.0.0.1:8080
messaging:
  kafka:
    brokers: [kafka.homolog:9093]
    auth: { type: scramSha512, user: ana, password: "${KAFKA_SENHA:-p4ssw0rd}" }
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - kafka: { topic: pedidos, value: "{}" }
`))
	if err == nil {
		t.Fatal("o valor de reserva com segredo literal passou: a recusa leu o texto já expandido")
	}
	if !strings.Contains(err.Error(), "goes into the repository") {
		t.Fatalf("a recusa mudou de motivo: %v", err)
	}

	spec, err := scenario.Parse([]byte(`
name: semente
target: http://127.0.0.1:8080
data:
  pedidos:
    generate: { id: uuid }
    seed: ${SEMENTE:-42}
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /pedidos/${pedidos.id}
`))
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	if spec.Data[0].SeedFrom != "SEMENTE" {
		t.Fatalf("a origem da semente saiu %q: expandir antes apaga de qual variável ela veio", spec.Data[0].SeedFrom)
	}
}
