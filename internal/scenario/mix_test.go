package scenario_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/scenario"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
)

func mixSpec(t *testing.T, steps string) (scenario.Spec, error) {
	t.Helper()
	document := "name: mix\ntarget: http://127.0.0.1:8080\nload:\n  profiles:\n    - steady: { rate: 1/s, duration: 1s }\nscenario:\n" + steps
	spec, err := scenario.Parse([]byte(document))
	if err != nil {
		return spec, err
	}
	return spec, spec.Validate()
}

// Distribuicao de operacoes e como carga realista se declara: repetir a mesma
// chamada mede cache, nao sistema. Antes disso, a unica saida era rodar tres
// cenarios em tres processos e tentar somar tres relatorios.
func TestWeightsProduceTheDeclaredProportionOverACycle(t *testing.T) {
	spec, err := mixSpec(t, `  - name: leve
    weight: 60
    http: GET /a
  - name: pesada
    weight: 30
    http: GET /b
  - name: criacao
    weight: 10
    http: GET /c
`)
	if err != nil {
		t.Fatalf("o cenário com mix não passou: %v", err)
	}

	order := scenario.MixOrder(spec)
	counts := map[int]int{}
	for _, step := range order {
		counts[step]++
	}
	if len(order) != 10 {
		t.Fatalf("o ciclo devia ser reduzido pelo máximo divisor comum, e veio com %d posições", len(order))
	}
	for index, expected := range map[int]int{0: 6, 1: 3, 2: 1} {
		if counts[index] != expected {
			t.Errorf("a alternativa %d apareceu %d vezes no ciclo, esperava %d", index, counts[index], expected)
		}
	}
	if share := scenario.DeclaredShare(spec, 0); share != 0.6 {
		t.Errorf("a proporção declarada da primeira alternativa saiu %g, esperava 0.6", share)
	}
}

// Sessenta chamadas de uma operacao seguidas de trinta de outra dao a proporcao
// certa no fim e uma carga que nenhum sistema recebe: durante o primeiro bloco
// a operacao cara nao existe e o alvo aquece um caminho so.
func TestTheCycleInterleavesInsteadOfGroupingByAlternative(t *testing.T) {
	spec, err := mixSpec(t, `  - name: leve
    weight: 3
    http: GET /a
  - name: pesada
    weight: 1
    http: GET /b
`)
	if err != nil {
		t.Fatalf("o cenário com mix não passou: %v", err)
	}

	order := scenario.MixOrder(spec)
	longest, current := 0, 0
	for index, step := range order {
		if index > 0 && step == order[index-1] {
			current++
		} else {
			current = 1
		}
		if current > longest {
			longest = current
		}
	}
	if longest > 2 {
		t.Fatalf("a alternativa se repetiu %d vezes seguidas no ciclo %v", longest, order)
	}
}

// A escolha e por posicao, nao sorteio: o gerador ja e deterministico em quando
// dispara, e duas execucoes do mesmo arquivo precisam disparar a mesma coisa
// para que comparar uma com a outra signifique alguma coisa.
func TestTheSameFileAlwaysProducesTheSameCycle(t *testing.T) {
	steps := `  - name: leve
    weight: 7
    http: GET /a
  - name: pesada
    weight: 5
    http: GET /b
  - name: rara
    weight: 2
    http: GET /c
`
	first, err := mixSpec(t, steps)
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	second, err := mixSpec(t, steps)
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}

	before, after := scenario.MixOrder(first), scenario.MixOrder(second)
	if len(before) != len(after) {
		t.Fatalf("dois ciclos de tamanhos diferentes: %d e %d", len(before), len(after))
	}
	for index := range before {
		if before[index] != after[index] {
			t.Fatalf("a posição %d mudou entre duas leituras do mesmo file: %d e %d", index, before[index], after[index])
		}
	}
}

// Peso escolhe qual alternativa executar, nao qual passo dentro de uma jornada.
// Com cadeia, o passo que usa ${valor} rodaria numa iteracao em que o passo que
// captura nao rodou, e a referencia resolveria para vazio.
func TestWeightInTheMiddleOfACaptureChainIsRefusedWithTheReason(t *testing.T) {
	_, err := mixSpec(t, `  - name: obter token
    weight: 20
    http: { method: POST, path: /auth/token }
    capture: { pedidoId: $.access_token }
  - name: consultar pedido
    weight: 80
    http: GET /pedidos/${pedidoId}
`)
	if err == nil {
		t.Fatal("aceitou peso no meio de uma cadeia de capturas")
	}
	for _, expected := range []string{"consultar pedido", "pedidoId", "obter token", "would resolve to nothing"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("a mensagem não explica por que não pode: falta %q em\n%v", expected, err)
		}
	}
}

// Alternativa sem proporcao nao tem como ser escolhida, e adivinhar um peso
// para ela seria inventar a carga.
func TestWeightOnSomeStepsAndNotOthersIsRefused(t *testing.T) {
	_, err := mixSpec(t, `  - name: leve
    weight: 60
    http: GET /a
  - name: pesada
    http: GET /b
`)
	if err == nil {
		t.Fatal("aceitou peso em um passo só")
	}
	if !strings.Contains(err.Error(), `"pesada"`) {
		t.Fatalf("a mensagem não nomeia o passo sem weight: %v", err)
	}
}

func TestWeightZeroOrNegativeIsRefused(t *testing.T) {
	for _, weight := range []string{"0", "-1", "meio"} {
		if _, err := mixSpec(t, "  - name: leve\n    weight: "+weight+"\n    http: GET /a\n"); err == nil {
			t.Errorf("aceitou peso %q", weight)
		}
	}
}

// Sem mix, todo passo roda em toda iteracao — que e o que sempre aconteceu.
func TestScenarioWithoutWeightsHasNoCycle(t *testing.T) {
	spec, err := mixSpec(t, "  - name: um\n    http: GET /a\n  - name: dois\n    http: GET /b\n")
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}
	if spec.HasMix() {
		t.Fatal("um cenário sem peso foi tratado como mix")
	}
	if scenario.MixOrder(spec) != nil {
		t.Fatal("um cenário sem peso ganhou ciclo de alternativas")
	}
}
