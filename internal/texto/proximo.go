package texto

import "strings"

// Closest answers which of the valid words the received one is probably a typo
// of. It lived inside the scenario parser, which is why "braunrate target -addr"
// answered with the whole option list while "carga: { taxaa: ... }" answered
// with the right key: the same product had the answer in one place and not in
// the other.
func Closest(received string, valid []string) (string, bool) {
	best, shortestDistance := "", 1<<30
	for _, candidate := range valid {
		distance := editDistance(strings.ToLower(received), strings.ToLower(candidate))
		if distance < shortestDistance {
			best, shortestDistance = candidate, distance
		}
	}
	// A fixed distance of three turns "taxa" into "voce quis dizer rampa?",
	// which is not a typo of anything. The tolerance grows with the word,
	// because a long word survives more typing than a short one.
	return best, best != "" && shortestDistance <= tolerance(received)
}

func tolerance(received string) int {
	switch {
	case len(received) <= 4:
		return 1
	case len(received) <= 8:
		return 2
	}
	return 3
}

func editDistance(first, second string) int {
	previous := make([]int, len(second)+1)
	current := make([]int, len(second)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(first); i++ {
		current[0] = i
		for j := 1; j <= len(second); j++ {
			cost := 1
			if first[i-1] == second[j-1] {
				cost = 0
			}
			current[j] = min(min(current[j-1]+1, previous[j]+1), previous[j-1]+cost)
		}
		copy(previous, current)
	}
	return previous[len(second)]
}
