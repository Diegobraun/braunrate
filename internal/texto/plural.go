// Package texto holds the small Portuguese-agreement helpers the whole product
// shares. Written out instead of "passo(s)" because the report is read out loud
// in meetings, and "as 1 regras" is the kind of thing that costs trust in
// everything else on the page.
package texto

import "fmt"

// Count writes the number with the noun already agreeing: Count(1, "passo",
// "passos") gives "1 passo".
func Count(quantity int64, singular, plural string) string {
	if quantity == 1 || quantity == -1 {
		return fmt.Sprintf("%d %s", quantity, singular)
	}
	return fmt.Sprintf("%d %s", quantity, plural)
}

// Times is its own helper because "1 uma vez" is what Count would produce: in
// Portuguese the number disappears into the word when there is only one.
func Times(quantity int64) string {
	if quantity == 1 {
		return "uma vez"
	}
	return fmt.Sprintf("%d vezes", quantity)
}

// Pick chooses between two whole phrases, for when the agreement reaches the
// article and the verb too.
func Pick(quantity int64, singular, plural string) string {
	if quantity == 1 || quantity == -1 {
		return singular
	}
	return plural
}
