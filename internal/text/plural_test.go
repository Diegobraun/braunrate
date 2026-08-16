package text_test

import (
	"testing"

	"github.com/Diegobraun/braunrate/internal/text"
)

func TestCountAgreesWithTheNumber(t *testing.T) {
	cases := []struct {
		quantity int64
		expected string
	}{{0, "0 passos"}, {1, "1 passo"}, {2, "2 passos"}, {1000, "1000 passos"}}

	for _, testCase := range cases {
		if got := text.Count(testCase.quantity, "passo", "passos"); got != testCase.expected {
			t.Errorf("Count(%d) = %q, esperava %q", testCase.quantity, got, testCase.expected)
		}
	}
}

func TestPickChoosesTheWholePhrase(t *testing.T) {
	if got := text.Pick(1, "a única regra foi atendida", "as regras foram atendidas"); got != "a única regra foi atendida" {
		t.Errorf("Pick(1) = %q", got)
	}
	if got := text.Pick(3, "a única regra foi atendida", "as regras foram atendidas"); got != "as regras foram atendidas" {
		t.Errorf("Pick(3) = %q", got)
	}
}

// "1 uma vez" is what Count would produce here: in Portuguese the number
// disappears into the word when there is only one.
func TestTimesSwallowsTheNumberWhenThereIsOnlyOne(t *testing.T) {
	if got := text.Times(1); got != "uma vez" {
		t.Errorf("Times(1) = %q", got)
	}
	if got := text.Times(3); got != "3 vezes" {
		t.Errorf("Times(3) = %q", got)
	}
}
