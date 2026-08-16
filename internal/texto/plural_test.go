package texto_test

import (
	"testing"

	"github.com/Diegobraun/braunrate/internal/texto"
)

func TestCountAgreesWithTheNumber(t *testing.T) {
	cases := []struct {
		quantity int64
		expected string
	}{{0, "0 passos"}, {1, "1 passo"}, {2, "2 passos"}, {1000, "1000 passos"}}

	for _, testCase := range cases {
		if got := texto.Count(testCase.quantity, "passo", "passos"); got != testCase.expected {
			t.Errorf("Count(%d) = %q, esperava %q", testCase.quantity, got, testCase.expected)
		}
	}
}

func TestPickChoosesTheWholePhrase(t *testing.T) {
	if got := texto.Pick(1, "a unica regra foi atendida", "as regras foram atendidas"); got != "a unica regra foi atendida" {
		t.Errorf("Pick(1) = %q", got)
	}
	if got := texto.Pick(3, "a unica regra foi atendida", "as regras foram atendidas"); got != "as regras foram atendidas" {
		t.Errorf("Pick(3) = %q", got)
	}
}

// "1 uma vez" is what Count would produce here: in Portuguese the number
// disappears into the word when there is only one.
func TestTimesSwallowsTheNumberWhenThereIsOnlyOne(t *testing.T) {
	if got := texto.Times(1); got != "uma vez" {
		t.Errorf("Times(1) = %q", got)
	}
	if got := texto.Times(3); got != "3 vezes" {
		t.Errorf("Times(3) = %q", got)
	}
}
