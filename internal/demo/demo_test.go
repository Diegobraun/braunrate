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

// O comando existe para quem nunca fez teste de carga: sem arquivo, sem alvo,
// sem segundo terminal. Se ele precisar de preparo, ele nao serve para nada.
func TestTheDemoRunsWithNoPreparationAndExplainsWhatItMeasured(t *testing.T) {
	directory := t.TempDir()
	var output strings.Builder

	if err := Run(context.Background(), Options{Directory: directory, Version: "teste", Output: &output}); err != nil {
		t.Fatalf("a demonstracao nao rodou: %v", err)
	}

	// Os tres conceitos que a demonstracao existe para ensinar. Sem eles ela
	// vira um comando que roda e nao explica, que e o que ja havia.
	for _, taught := range []string{
		"Essa e a taxa",
		"95% em ate",
		"criterio de aceite",
	} {
		if !strings.Contains(output.String(), taught) {
			t.Errorf("a demonstracao nao ensina %q:\n%s", taught, output.String())
		}
	}

	for _, produced := range []string{"demo.yaml", "demo-relatorio.html"} {
		if _, err := os.Stat(filepath.Join(directory, produced)); err != nil {
			t.Errorf("a demonstracao nao deixou %s: %v", produced, err)
		}
	}
	if !strings.Contains(output.String(), filepath.Join(directory, "demo.yaml")) {
		t.Error("a demonstracao criou arquivo e nao disse qual")
	}
}

// Uma ressalva que aparece no relatorio e some da demonstracao ensinaria que
// existe um modo de ver o numero sem o que o desqualifica.
func TestTheDemoRepeatsTheCaveatTheReportRaises(t *testing.T) {
	directory := t.TempDir()
	var output strings.Builder

	if err := Run(context.Background(), Options{Directory: directory, Version: "teste", Output: &output}); err != nil {
		t.Fatalf("a demonstracao nao rodou: %v", err)
	}
	if !strings.Contains(output.String(), "nao tem nenhum valor que varia") {
		t.Errorf("o caminho fixo do cenario da demonstracao passou sem ressalva:\n%s", output.String())
	}
}

func TestTheFailingDemoShowsWhatTheClosedLoopHides(t *testing.T) {
	directory := t.TempDir()
	var output strings.Builder

	options := Options{WithFailure: true, Directory: directory, Version: "teste", Output: &output}
	if err := Run(context.Background(), options); err != nil {
		t.Fatalf("a demonstracao nao rodou: %v", err)
	}

	for _, shown := range []string{
		"escondidos pelo laco fechado",
		"laco fechado (JMeter, Locust)",
		"braunrate (modelo aberto)",
		"FALHA",
	} {
		if !strings.Contains(output.String(), shown) {
			t.Errorf("a demonstracao com falha nao mostra %q:\n%s", shown, output.String())
		}
	}
}

// Os dois cenarios que a demonstracao escreve sao cenarios comuns: quem gostou
// do resultado edita o arquivo e roda contra o proprio servico. Um arquivo que
// a propria ferramenta recusasse tornaria a demonstracao um caminho sem saida.
func TestTheDemoWritesScenariosTheToolAccepts(t *testing.T) {
	for name, content := range map[string]string{
		"demo.yaml":           healthyScenario("http://127.0.0.1:8080"),
		"demo-com-falha.yaml": freezingScenario("http://127.0.0.1:8080"),
	} {
		spec, err := scenario.Parse([]byte(content))
		if err != nil {
			t.Fatalf("%s nao e um cenario valido: %v", name, err)
		}
		if err := spec.Validate(); err != nil {
			t.Fatalf("%s nao passa na validacao: %v", name, err)
		}
	}
}

// 8080 ocupado e o caso comum numa maquina de trabalho, e uma demonstracao que
// morre nele e uma demonstracao que ninguem ve.
func TestTheDemoSurvivesABusyPort(t *testing.T) {
	occupied, err := net.Listen("tcp", preferredPort)
	if err != nil {
		t.Skipf("%s ja esta ocupado por outra coisa, entao o teste nao prova nada: %v", preferredPort, err)
	}
	defer func() { _ = occupied.Close() }()

	var output strings.Builder
	if err := Run(context.Background(), Options{Directory: t.TempDir(), Version: "teste", Output: &output}); err != nil {
		t.Fatalf("a demonstracao morreu com a porta ocupada: %v", err)
	}
	if !strings.Contains(output.String(), "esta ocupado") {
		t.Errorf("o alvo mudou de endereco e a demonstracao nao disse:\n%s", output.String())
	}
}
