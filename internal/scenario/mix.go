package scenario

import (
	"fmt"
	"sort"
	"strings"
)

// Mix ponderado: cada iteracao executa uma alternativa, escolhida pela posicao
// dela no ciclo e nao sorteada. O ADR 0016 explica por que a escolha e
// deterministica e por que o peso nao entra no meio de uma cadeia de capturas.

// HasMix diz se o cenario declara mix. Sem mix, todo passo roda em toda
// iteracao, que e o comportamento que sempre existiu.
func (spec Spec) HasMix() bool {
	for _, step := range spec.Steps {
		if step.Weight > 0 {
			return true
		}
	}
	return false
}

// MixOrder devolve, para cada posicao de um ciclo, o indice do passo que roda
// ali. O ciclo se repete: a iteracao N executa a alternativa da posicao
// N % len(ordem). Devolve nil quando nao ha mix.
//
// A ordem intercala em vez de agrupar. Sessenta chamadas de uma operacao
// seguidas de trinta de outra teriam a proporcao certa no fim e uma carga que
// nenhum sistema recebe: durante os primeiros sessenta segundos a operacao
// cara nao existe, e o alvo aquece um caminho so.
func MixOrder(spec Spec) []int {
	if !spec.HasMix() {
		return nil
	}
	weights := make([]int, len(spec.Steps))
	for index, step := range spec.Steps {
		weights[index] = step.Weight
	}
	divisor := weights[0]
	for _, weight := range weights[1:] {
		divisor = greatestCommonDivisor(divisor, weight)
	}

	type slot struct {
		position float64
		step     int
		copy     int
	}
	var slots []slot
	total := 0
	for _, weight := range weights {
		total += weight / divisor
	}
	for index, weight := range weights {
		share := weight / divisor
		for copyIndex := range share {
			slots = append(slots, slot{
				position: (float64(copyIndex) + 0.5) * float64(total) / float64(share),
				step:     index,
				copy:     copyIndex,
			})
		}
	}
	sort.Slice(slots, func(first, second int) bool {
		if slots[first].position != slots[second].position {
			return slots[first].position < slots[second].position
		}
		if slots[first].step != slots[second].step {
			return slots[first].step < slots[second].step
		}
		return slots[first].copy < slots[second].copy
	})

	order := make([]int, 0, len(slots))
	for _, item := range slots {
		order = append(order, item.step)
	}
	return order
}

// DeclaredShare e a proporcao que o arquivo pediu para aquele passo. Sem mix,
// todo passo roda em toda iteracao e a proporcao e 1.
func DeclaredShare(spec Spec, index int) float64 {
	if !spec.HasMix() {
		return 1
	}
	total := 0
	for _, step := range spec.Steps {
		total += step.Weight
	}
	if total == 0 {
		return 0
	}
	return float64(spec.Steps[index].Weight) / float64(total)
}

func greatestCommonDivisor(first, second int) int {
	for second != 0 {
		first, second = second, first%second
	}
	return first
}

func checkMix(spec *Spec) []string {
	declared, silent := 0, []string{}
	for _, step := range spec.Steps {
		if step.Weight > 0 {
			declared++
			continue
		}
		silent = append(silent, fmt.Sprintf("%q", step.Name))
	}
	if declared == 0 {
		return nil
	}
	var problems []string
	if len(silent) > 0 {
		problems = append(problems, fmt.Sprintf(
			"the scenario declares weight on %d step(s) and not on %s: weight is the proportion between alternatives, and an alternative with no proportion has no way of being picked.\n"+
				"    declare o peso de todos os passos, ou de nenhum",
			declared, strings.Join(silent, ", ")))
	}
	if chained := chainedCaptures(*spec); chained != "" {
		problems = append(problems, chained)
	}
	return problems
}

// Peso escolhe qual alternativa executar, nao qual passo dentro dela. Com uma
// cadeia de capturas, o passo que usa ${valor} pode rodar numa iteracao em que
// o passo que captura nao rodou — e ai a referencia resolve para vazio, que e o
// defeito 3.7.a por outro caminho.
func chainedCaptures(spec Spec) string {
	produced := map[string]int{}
	for index, step := range spec.Steps {
		for _, capture := range step.Captures {
			produced[capture.Variable] = index
		}
	}
	if len(produced) == 0 {
		return ""
	}
	for index, step := range spec.Steps {
		for _, text := range textsOf(step.Config) {
			for _, used := range referencesIn(text) {
				origin, born := produced[used.name]
				if !born || origin == index {
					continue
				}
				return fmt.Sprintf(
					"the step %q has a weight and uses ${%s}, which the step %q captures: weight picks which alternative to run, not which step inside a journey,\n"+
						"    so the iteration that runs %q may not have run %q, and the reference would resolve to nothing.\n"+
						"    a chain of captures is one single journey: drop the weight from it, or split the alternatives into separate scenarios",
					spec.Steps[index].Name, used.name, spec.Steps[origin].Name,
					spec.Steps[index].Name, spec.Steps[origin].Name)
			}
		}
	}
	return ""
}
