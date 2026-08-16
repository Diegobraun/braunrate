package scenario_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/scenario"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
)

func seedSpec(t *testing.T, declared string) (scenario.Spec, error) {
	t.Helper()
	document := `name: semente
target: http://127.0.0.1:8080
data:
  pedidos:
    generate: { id: uuid }
    seed: ` + declared + `
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /pedidos/${pedidos.id}
    name: consultar
`
	return scenario.Parse([]byte(document))
}

// Semente fixa no arquivo faz o CI rodar sempre o mesmo caso, e um caso que
// passa mil vezes nao prova mais nada depois da primeira.
func TestSeedComesFromTheEnvironmentWhenTheVariableIsSet(t *testing.T) {
	t.Setenv("SEMENTE", "8817")

	spec, err := seedSpec(t, "${SEMENTE:-42}")
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	if spec.Data[0].Seed != 8817 {
		t.Fatalf("a semente saiu %d e o ambiente dizia 8817", spec.Data[0].Seed)
	}
	if spec.Data[0].SeedFrom != "SEMENTE" {
		t.Fatalf("a origem da semente saiu %q e devia ser SEMENTE", spec.Data[0].SeedFrom)
	}
	if origins := scenario.SeedsFromEnvironment(spec); origins["pedidos"] != "SEMENTE" {
		t.Fatalf("a origem não chegou ao que o relatório publica: %+v", origins)
	}
}

// Sem a variavel declarada, o comportamento e o de antes. Um arquivo que ja
// existe nao pode mudar de resultado porque a ferramenta ganhou um recurso.
func TestWithoutTheVariableTheDefaultIsUsedAndNothingIsAnnounced(t *testing.T) {
	spec, err := seedSpec(t, "${SEMENTE_QUE_NINGUEM_DEFINIU:-42}")
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	if spec.Data[0].Seed != 42 {
		t.Fatalf("a semente saiu %d e o padrão do arquivo era 42", spec.Data[0].Seed)
	}
	if spec.Data[0].SeedFrom != "" {
		t.Fatalf("declarou origem de ambiente para uma semente que veio do file: %q", spec.Data[0].SeedFrom)
	}
	if scenario.SeedsFromEnvironment(spec) != nil {
		t.Fatal("o relatório ganharia uma linha de reproducao que não tem o que reproduzir")
	}
}

func TestLiteralSeedKeepsWorking(t *testing.T) {
	spec, err := seedSpec(t, "7")
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	if spec.Data[0].Seed != 7 || spec.Data[0].SeedFrom != "" {
		t.Fatalf("a semente literal mudou de significado: %+v", spec.Data[0])
	}
}

// Errar a variavel e o caso comum: exportar SEMENTE=hoje e ver a execucao rodar
// com a semente padrao seria pior que a recusa, porque ninguem descobre.
func TestSeedFromTheEnvironmentThatIsNotANumberIsRefusedNamingTheVariable(t *testing.T) {
	t.Setenv("SEMENTE", "hoje")

	_, err := seedSpec(t, "${SEMENTE:-42}")
	if err == nil {
		t.Fatal("aceitou uma semente que não e número")
	}
	if !strings.Contains(err.Error(), "$SEMENTE") || !strings.Contains(err.Error(), `"hoje"`) {
		t.Fatalf("a mensagem não diz qual variável esta errada nem com que value: %v", err)
	}
}

// A semente so muda o que sai de quem usa aleatoriedade. Publicar a semente de
// um CSV circular seria ruido no bloco que a pessoa le para reproduzir.
func TestOnlySourcesThatUseTheSeedReportIt(t *testing.T) {
	cases := []struct {
		name   string
		source scenario.DataSource
		uses   bool
	}{
		{"sintetica", scenario.DataSource{Fields: map[string]scenario.Generator{"id": {Recipe: "uuid"}}}, true},
		{"csv aleatório", scenario.DataSource{File: "x.csv", Consume: scenario.ConsumeRandom}, true},
		{"csv circular", scenario.DataSource{File: "x.csv", Consume: scenario.ConsumeCircular}, false},
	}
	for _, testCase := range cases {
		if testCase.source.UsesSeed() != testCase.uses {
			t.Errorf("%s: UsesSeed devolveu %v", testCase.name, testCase.source.UsesSeed())
		}
	}
}
