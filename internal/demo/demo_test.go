package demo

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// O comando existe para quem nunca fez teste de load: sem arquivo, sem alvo,
// sem segundo terminal. Se ele precisar de preparo, ele nao serve para nada.
func TestTheDemoRunsWithNoPreparationAndExplainsWhatItMeasured(t *testing.T) {
	t.Cleanup(rememberElsewhere(t.TempDir()))
	directory := t.TempDir()
	var output strings.Builder

	if err := Run(context.Background(), Options{Directory: directory, Version: "teste", Output: &output}); err != nil {
		t.Fatalf("a demonstracao não rodou: %v", err)
	}

	// Os tres conceitos que a demonstracao existe para ensinar. Sem eles ela
	// vira um comando que roda e nao explica, que e o que ja havia.
	for _, taught := range []string{
		"That is the rate",
		"95% within",
		"acceptance criterion",
	} {
		if !strings.Contains(output.String(), taught) {
			t.Errorf("a demonstracao não ensina %q:\n%s", taught, output.String())
		}
	}

	for _, produced := range []string{"demo.yaml", "demo-report.html"} {
		if _, err := os.Stat(filepath.Join(directory, produced)); err != nil {
			t.Errorf("a demonstracao não deixou %s: %v", produced, err)
		}
	}
	if !strings.Contains(output.String(), filepath.Join(directory, "demo.yaml")) {
		t.Error("a demonstracao criou arquivo e não disse qual")
	}
}

// Uma ressalva que aparece no relatorio e some da demonstracao ensinaria que
// existe um modo de ver o numero sem o que o desqualifica.
func TestTheDemoRepeatsTheCaveatTheReportRaises(t *testing.T) {
	t.Cleanup(rememberElsewhere(t.TempDir()))
	directory := t.TempDir()
	var output strings.Builder

	if err := Run(context.Background(), Options{Directory: directory, Version: "teste", Output: &output}); err != nil {
		t.Fatalf("a demonstracao não rodou: %v", err)
	}
	if !strings.Contains(output.String(), "has no value that varies") {
		t.Errorf("o caminho fixo do cenário da demonstracao passou sem ressalva:\n%s", output.String())
	}
}

func TestTheFailingDemoShowsWhatTheClosedLoopHides(t *testing.T) {
	t.Cleanup(rememberElsewhere(t.TempDir()))
	directory := t.TempDir()
	var output strings.Builder

	options := Options{WithFailure: true, Directory: directory, Version: "teste", Output: &output}
	if err := Run(context.Background(), options); err != nil {
		t.Fatalf("a demonstracao não rodou: %v", err)
	}

	for _, shown := range []string{
		"the closed loop never counted",
		"closed loop (JMeter, Locust)",
		"braunrate (open model)",
		"FAIL",
	} {
		if !strings.Contains(output.String(), shown) {
			t.Errorf("a demonstracao com falha não mostra %q:\n%s", shown, output.String())
		}
	}
}

// Os dois cenarios que a demonstracao escreve sao cenarios comuns: quem gostou
// do resultado edita o arquivo e roda contra o proprio servico. Um arquivo que
// a propria ferramenta recusasse tornaria a demonstracao um caminho sem saida.
func TestTheDemoWritesScenariosTheToolAccepts(t *testing.T) {
	t.Cleanup(rememberElsewhere(t.TempDir()))
	for name, content := range map[string]string{
		"demo.yaml":           healthyScenario("http://127.0.0.1:8080"),
		"demo-com-falha.yaml": freezingScenario("http://127.0.0.1:8080"),
	} {
		spec, err := scenario.Parse([]byte(content))
		if err != nil {
			t.Fatalf("%s não e um cenário válido: %v", name, err)
		}
		if err := spec.Validate(); err != nil {
			t.Fatalf("%s não passa na validação: %v", name, err)
		}
	}
}

// 8080 ocupado e o caso comum numa maquina de trabalho, e uma demonstracao que
// morre nele e uma demonstracao que ninguem ve.
func TestTheDemoSurvivesABusyPort(t *testing.T) {
	t.Cleanup(rememberElsewhere(t.TempDir()))
	occupied, err := net.Listen("tcp", preferredPort)
	if err != nil {
		t.Skipf("%s já esta ocupado por outra coisa, então o teste não prova nada: %v", preferredPort, err)
	}
	defer func() { _ = occupied.Close() }()

	var output strings.Builder
	if err := Run(context.Background(), Options{Directory: t.TempDir(), Version: "teste", Output: &output}); err != nil {
		t.Fatalf("a demonstracao morreu com a porta ocupada: %v", err)
	}
	if !strings.Contains(output.String(), "is busy") {
		t.Errorf("o alvo mudou de endereço e a demonstracao não disse:\n%s", output.String())
	}
}

// A explicacao ao lado de cada numero salva quem nunca rodou um teste de carga e
// e ruido para quem ja rodou. Uma bandeira nao resolve: quem se irrita com a
// explicacao e quem nao vai ler a ajuda para descobrir que ela existe.
func TestTheSecondRunKeepsTheNumbersAndDropsTheLesson(t *testing.T) {
	t.Cleanup(rememberElsewhere(t.TempDir()))

	var first, second strings.Builder
	for _, output := range []*strings.Builder{&first, &second} {
		if err := Run(context.Background(), Options{Directory: t.TempDir(), Version: "teste", Output: output}); err != nil {
			t.Fatalf("a demonstração não rodou: %v", err)
		}
	}

	for _, lesson := range []string{"That is the rate", "An average hides things", "acceptance criterion"} {
		if !strings.Contains(first.String(), lesson) {
			t.Errorf("a primeira execução não ensina %q", lesson)
		}
		if strings.Contains(second.String(), lesson) {
			t.Errorf("a segunda execução repete %q para quem já leu", lesson)
		}
	}
	for _, number := range []string{"requests in", "Half the responses within", "Full report:"} {
		if !strings.Contains(second.String(), number) {
			t.Errorf("a segunda execução perdeu %q, que é medição e não explicação", number)
		}
	}
	if !strings.Contains(second.String(), "braunrate demo --explain") {
		t.Error("a segunda execução encolhe sem dizer por quê nem como voltar")
	}

	var asked strings.Builder
	if err := Run(context.Background(), Options{Explain: true, Directory: t.TempDir(), Version: "teste", Output: &asked}); err != nil {
		t.Fatalf("a demonstração não rodou: %v", err)
	}
	if !strings.Contains(asked.String(), "That is the rate") {
		t.Error("--explain não traz a explicação de volta")
	}
}
