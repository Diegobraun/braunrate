// Package text holds the small plural-agreement helpers the whole product
// shares. Written out instead of "step(s)" because the report is read out loud
// in meetings, and "1 rules were met" is the kind of thing that costs trust in
// everything else on the page.
package text

import "fmt"

// Count writes the number with the noun already agreeing: Count(1, "step",
// "steps") gives "1 step".
func Count(quantity int64, singular, plural string) string {
	if quantity == 1 || quantity == -1 {
		return fmt.Sprintf("%d %s", quantity, singular)
	}
	return fmt.Sprintf("%d %s", quantity, plural)
}

// Times is its own helper because "1 once" is what Count would produce: the
// number disappears into the word when there is only one.
func Times(quantity int64) string {
	if quantity == 1 {
		return "once"
	}
	return fmt.Sprintf("%d times", quantity)
}

// Pick chooses between two whole phrases, for when the agreement reaches the
// article and the verb too.
func Pick(quantity int64, singular, plural string) string {
	if quantity == 1 || quantity == -1 {
		return singular
	}
	return plural
}
