package text_test

import (
	"testing"

	"github.com/Diegobraun/braunrate/internal/text"
)

func TestCountAgreesWithTheNumber(t *testing.T) {
	cases := []struct {
		quantity int64
		expected string
	}{{0, "0 steps"}, {1, "1 step"}, {2, "2 steps"}, {1000, "1,000 steps"},
		{4500000, "4,500,000 steps"}, {-1, "-1 step"}}

	for _, testCase := range cases {
		if got := text.Count(testCase.quantity, "step", "steps"); got != testCase.expected {
			t.Errorf("Count(%d) = %q, esperava %q", testCase.quantity, got, testCase.expected)
		}
	}
}

func TestPickChoosesTheWholePhrase(t *testing.T) {
	if got := text.Pick(1, "the only rule was met", "the rules were met"); got != "the only rule was met" {
		t.Errorf("Pick(1) = %q", got)
	}
	if got := text.Pick(3, "the only rule was met", "the rules were met"); got != "the rules were met" {
		t.Errorf("Pick(3) = %q", got)
	}
}

// "1 time" is what Count would produce here, where the number belongs inside
// the word.
func TestTimesSwallowsTheNumberWhenThereIsOnlyOne(t *testing.T) {
	if got := text.Times(1); got != "once" {
		t.Errorf("Times(1) = %q", got)
	}
	if got := text.Times(3); got != "3 times" {
		t.Errorf("Times(3) = %q", got)
	}
}
