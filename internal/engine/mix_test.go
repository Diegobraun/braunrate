package engine_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
)

// Sem mix, medir a distribuicao real de operacoes exigia tres cenarios em tres
// processos e tres relatorios que ninguem consegue somar. Aqui a proporcao e do
// arquivo, e o que chega no alvo tem que ser ela.
func TestMixSendsTheDeclaredProportionToTheTarget(t *testing.T) {
	var mutex sync.Mutex
	received := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		received[request.URL.Path]++
		mutex.Unlock()
		_, _ = fmt.Fprint(writer, `{"id":"1"}`)
	}))
	t.Cleanup(server.Close)

	spec, err := scenario.Parse([]byte(fmt.Sprintf(`
nome: Mix
alvo: %s

carga:
  perfis:
    - constante: { taxa: 2000/s, durante: 500ms }

cenario:
  - nome: leve
    peso: 60
    http: GET /leve
  - nome: pesada
    peso: 30
    http: GET /pesada
  - nome: criacao
    peso: 10
    http: GET /criacao
`, server.URL)))
	if err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}

	executor, err := engine.New(spec, engine.DefaultOptions())
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	document := executor.Execute(context.Background())

	mutex.Lock()
	defer mutex.Unlock()
	total := 0
	for _, count := range received {
		total += count
	}
	if total == 0 {
		t.Fatal("nenhuma requisicao chegou ao alvo")
	}
	for path, expected := range map[string]float64{"/leve": 0.6, "/pesada": 0.3, "/criacao": 0.1} {
		observed := float64(received[path]) / float64(total)
		if difference := observed - expected; difference > 0.02 || difference < -0.02 {
			t.Errorf("%s recebeu %.1f%% das chamadas e o arquivo declarou %.0f%%",
				path, observed*100, expected*100)
		}
	}

	// Uma iteracao executa uma alternativa. Somar os passos e somar as iteracoes,
	// e nao tres vezes isso.
	sum := int64(0)
	for _, step := range document.Steps {
		sum += step.Count
	}
	if sum != document.Overall.Count {
		t.Errorf("a soma dos passos (%d) nao bate com o total (%d): alguma iteracao executou mais de uma alternativa",
			sum, document.Overall.Count)
	}
}

// Peso de 60% que virou 45% na execucao e informacao, nao detalhe: e o que
// separa o mix declarado do mix que de fato foi aplicado.
func TestReportShowsDeclaredAndObservedProportionOnlyWhenThereIsMix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = fmt.Fprint(writer, `{"id":"1"}`)
	}))
	t.Cleanup(server.Close)

	run := func(steps string) string {
		t.Helper()
		spec, err := scenario.Parse([]byte(fmt.Sprintf(`
nome: Mix
alvo: %s
carga:
  perfis:
    - constante: { taxa: 500/s, durante: 200ms }
cenario:
%s`, server.URL, steps)))
		if err != nil {
			t.Fatalf("cenario invalido: %v", err)
		}
		executor, err := engine.New(spec, engine.DefaultOptions())
		if err != nil {
			t.Fatalf("motor nao subiu: %v", err)
		}
		var terminal bytes.Buffer
		if err := report.Summary(&terminal, executor.Execute(context.Background()), slo.Verdict{}); err != nil {
			t.Fatalf("relatorio nao saiu: %v", err)
		}
		return terminal.String()
	}

	withMix := run("  - nome: leve\n    peso: 60\n    http: GET /leve\n  - nome: pesada\n    peso: 40\n    http: GET /pesada\n")
	if !strings.Contains(withMix, "Mix declarado e observado") {
		t.Fatalf("o relatorio nao diz qual proporcao foi aplicada:\n%s", withMix)
	}
	if !strings.Contains(withMix, "60.0% declarado") {
		t.Errorf("o relatorio nao mostra a proporcao declarada:\n%s", withMix)
	}
	if !strings.Contains(withMix, "alternativas do mix") {
		t.Errorf("o relatorio nao avisa que o percentil de jornada junta as alternativas:\n%s", withMix)
	}

	withoutMix := run("  - nome: leve\n    http: GET /leve\n")
	if strings.Contains(withoutMix, "Mix declarado") {
		t.Fatalf("o bloco de mix apareceu num cenario sem mix:\n%s", withoutMix)
	}
}
