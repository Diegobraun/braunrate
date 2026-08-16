package report

import (
	"io"
	"strings"

	"github.com/Diegobraun/braunrate/internal/report/comparison"
)

func Comparison(out io.Writer, c comparison.Comparison) error {
	lines := &lineWriter{out: out}
	write := lines.writef

	write("")
	write("%s", c.Sentence)
	write("")
	write("Comparing")
	write("  before: %s against %s, at %s", c.Before.Spec, c.Before.Target, c.Before.Start)
	write("  after:  %s against %s, at %s", c.After.Spec, c.After.Target, c.After.Start)
	write("")

	if c.Comparable {
		write("The whole journey")
		write("  %s", c.Journey.Sentence)
		write("")

		write("Per step")
		write("  %-26s %11s %11s %16s", "step", "95% before", "95% after", "change")
		for _, step := range c.Steps {
			note := ""
			switch {
			case step.New:
				note = "  (new step)"
			case step.Vanished:
				note = "  (gone)"
			}
			write("  %-26s %11s %11s %16s%s", trim(step.Step, 26),
				milliseconds(step.P95.Before), milliseconds(step.P95.After),
				change(step.P95), note)
		}
		write("")
		write("Errors")
		write("  %s", c.Error.Sentence)
		write("")
	}

	write("What could explain the difference other than the service")
	if len(c.Caveats) == 0 {
		write("  %s", noCaveatSentence)
	}
	for _, caveat := range c.Caveats {
		if caveat.Blocking {
			write("  - %s (this alone explains the difference)", caveat.Text)
			continue
		}
		write("  - %s", caveat.Text)
	}
	write("  Two runs give no confidence interval: a change below %.0f%% is treated as noise.", comparison.AcceptedNoise*100)
	write("")
	return lines.err
}

func change(difference comparison.Difference) string {
	if difference.Direction == comparison.DirectionSame {
		return "noise"
	}
	magnitude := strings.Replace(comparison.Magnitude(difference), " times", "x", 1)
	if difference.Direction == comparison.DirectionBetter {
		return magnitude + " better"
	}
	return magnitude + " worse"
}

// "Nothing" was an absolute claim over five fields it had checked. Replacing the
// whole CSV between two runs changed the p95 by 15x and the comparison still
// said nothing but the service could explain it.
const noCaveatSentence = "Nothing that can be compared differs: scenario, target, machine, load plan and version are the same. The contents of the data files are not on this list — if they changed between the two, the difference may be theirs."
