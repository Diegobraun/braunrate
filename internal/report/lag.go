package report

import (
	"fmt"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/text"
)

// The phrasing of the lag lives here because the terminal and the HTML answer
// the same question from the same numbers: two wordings of one measurement is
// two readings of it.
func lagSentences(lag protocol.ConsumerLag) (headline, note string) {
	switch {
	case lag.Problem != "":
		return fmt.Sprintf("I could not measure it — %s", lag.Problem), ""
	case lag.Readings == 0:
		return "no reading in the period", ""
	}
	headline = fmt.Sprintf("at the worst moment %s behind; at the end, %s",
		messages(lag.Max), messages(lag.Final))
	// The measurement is the distance between the high watermark and the
	// committed offset. Why the distance grew — a consumer that could not keep
	// up, one that stopped, one that was rebalancing — is not in it, and the
	// sentence used to name the first of the three as if it had been checked.
	if lag.Final > 0 {
		note = "The consumer ended the run behind. The lag says the distance, not the cause: a slow consumer, a stopped one and one rebalancing all produce the same number."
	}
	return headline, note
}

func messages(quantity int64) string {
	return thousands(quantity) + " " + text.Pick(quantity, "message", "messages")
}
