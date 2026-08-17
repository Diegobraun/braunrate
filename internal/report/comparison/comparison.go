package comparison

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/text"
)

// Two runs do not produce a confidence interval. Change below this is treated
// as noise because, with one sample per side, there is no ground to claim
// anything moved: calling 3% a regression would invent precision.
const AcceptedNoise = 0.05

type Comparison struct {
	Before             Identification        `json:"before"`
	After              Identification        `json:"after"`
	Sentence           string                `json:"sentence"`
	Comparable         bool                  `json:"comparable"`
	Caveats            []Caveat              `json:"caveats"`
	Journey            Difference            `json:"journey"`
	Overall            Difference            `json:"global"`
	JourneyPercentiles map[string]Difference `json:"journeyByPercentile"`
	OverallPercentiles map[string]Difference `json:"globalByPercentile"`
	Steps              []StepDifference      `json:"steps"`
	Error              CountDifference       `json:"errorRate"`
}

// Blocking marks a caveat that explains the whole difference by itself. Only
// those take the verdict away from a regression gate; treating every caveat as
// blocking would disable the gate on any authenticated scenario.
type Caveat struct {
	Text     string `json:"text"`
	Blocking bool   `json:"blocksComparison"`
}

type Identification struct {
	Spec    string `json:"scenario"`
	Target  string `json:"target"`
	Start   string `json:"start"`
	Version string `json:"version"`
}

type Difference struct {
	Metric   string  `json:"metric"`
	Before    float64 `json:"beforeMs"`
	After     float64 `json:"afterMs"`
	Change    float64 `json:"change"`
	Direction string  `json:"direction"`
	Sentence  string  `json:"sentence"`
}

type StepDifference struct {
	Step     string     `json:"step"`
	P95      Difference `json:"p95"`
	P99      Difference `json:"p99"`
	New      bool       `json:"new"`
	Vanished bool       `json:"gone"`
}

type CountDifference struct {
	Before   float64 `json:"before"`
	After    float64 `json:"after"`
	Sentence string  `json:"sentence"`
}

const (
	DirectionWorse  = "worse"
	DirectionBetter = "better"
	DirectionSame   = "no difference worth reading"
)

func Compare(before, after metrics.Document) Comparison {
	compared := Comparison{
		Before:     identify(before),
		After:      identify(after),
		Comparable: true,
	}

	compared.Caveats = collectCaveats(before, after)
	if !before.Valid() || !after.Valid() {
		compared.Comparable = false
	}

	compared.Journey = compareDistribution("the whole journey (95%)", before.Journey.Reported().P95, after.Journey.Reported().P95)
	compared.Overall = compareDistribution("all requests (95%)", before.Overall.Reported().P95, after.Overall.Reported().P95)
	compared.JourneyPercentiles = comparePercentiles("the whole journey", before.Journey.Reported(), after.Journey.Reported())
	compared.OverallPercentiles = comparePercentiles("all requests", before.Overall.Reported(), after.Overall.Reported())
	compared.Steps = compareSteps(before, after)
	compared.Error = compareErrors(before, after)
	compared.Sentence = phrase(compared, before, after)
	return compared
}

func identify(document metrics.Document) Identification {
	return Identification{
		Spec:    document.Run.Spec,
		Target:  document.Run.Target,
		Start:   document.Run.Start.Format("2006-01-02 15:04"),
		Version: document.Version,
	}
}

// Comparing two runs only holds when both measured the same thing the same
// way; each difference here can explain the whole change on its own.
func collectCaveats(before, after metrics.Document) []Caveat {
	var caveats []Caveat
	blocking := func(format string, args ...any) {
		caveats = append(caveats, Caveat{Text: fmt.Sprintf(format, args...), Blocking: true})
	}

	if before.Run.Spec != after.Run.Spec {
		blocking("the scenarios are different: %q and %q", before.Run.Spec, after.Run.Spec)
	}
	if before.Run.Target != after.Run.Target {
		blocking("the targets are different: %s and %s", before.Run.Target, after.Run.Target)
	}
	if before.Environment.Host != after.Environment.Host || before.Environment.Cores != after.Environment.Cores {
		blocking("the generating machines are different: %s with %d cores and %s with %d cores",
			before.Environment.Host, before.Environment.Cores, after.Environment.Host, after.Environment.Cores)
	}
	if planSummary(before) != planSummary(after) {
		blocking("the load plans are different: %s and %s", planSummary(before), planSummary(after))
	}
	if before.Version != after.Version {
		blocking("the runs used different versions of braunrate: %s and %s", before.Version, after.Version)
	}
	if before.Run.Model != after.Run.Model {
		blocking("the arrival models are different: %s and %s. Closed-loop latency does not compare with latency counted from the scheduled instant — the second includes a delay the first never gets to record",
			before.Run.Model, after.Run.Model)
	}
	if !before.Valid() {
		blocking("the previous run has an invalid result and its number does not work as a baseline")
	}
	if !after.Valid() {
		blocking("the new run has an invalid result and its number does not work as a comparison")
	}
	if before.Run.AuthObtains > 0 || after.Run.AuthObtains > 0 {
		caveats = append(caveats, Caveat{Text: "both runs used one token for everything; caching or sharding by identity affects them the same way, but it does not disappear from the comparison"})
	}
	return caveats
}

func planSummary(document metrics.Document) string {
	if len(document.Run.AppliedPlan) == 0 {
		return "sem plano declarado"
	}
	summary := ""
	for index, phase := range document.Run.AppliedPlan {
		if index > 0 {
			summary += " + "
		}
		summary += fmt.Sprintf("%s up to %.0f/s for %ds", phase.Kind, phase.To, phase.DurationMs/1000)
	}
	return summary
}

func comparePercentiles(name string, before, after metrics.Distribution) map[string]Difference {
	pairs := map[string][2]float64{
		"p50":   {before.P50, after.P50},
		"p75":   {before.P75, after.P75},
		"p90":   {before.P90, after.P90},
		"p95":   {before.P95, after.P95},
		"p99":   {before.P99, after.P99},
		"p99.9": {before.P999, after.P999},
		"max":   {before.Max, after.Max},
	}
	byPercentile := make(map[string]Difference, len(pairs))
	for percentile, pair := range pairs {
		byPercentile[percentile] = compareDistribution(fmt.Sprintf("%s (%s)", name, percentile), pair[0], pair[1])
	}
	return byPercentile
}

func compareDistribution(name string, before, after float64) Difference {
	difference := Difference{Metric: name, Before: before, After: after, Direction: DirectionSame}
	if before > 0 {
		difference.Change = (after - before) / before
	}
	switch {
	case math.Abs(difference.Change) < AcceptedNoise:
		difference.Direction = DirectionSame
	case difference.Change > 0:
		difference.Direction = DirectionWorse
	default:
		difference.Direction = DirectionBetter
	}
	difference.Sentence = phraseDifference(difference)
	return difference
}

func phraseDifference(difference Difference) string {
	if difference.Before == 0 && difference.After == 0 {
		return fmt.Sprintf("%s: no sample in either run.", difference.Metric)
	}
	if difference.Direction == DirectionSame {
		return fmt.Sprintf("%s: %.0f ms against %.0f ms — a difference within the noise of two runs.",
			difference.Metric, difference.Before, difference.After)
	}
	verb := "slower"
	if difference.Direction == DirectionBetter {
		verb = "faster"
	}
	return fmt.Sprintf("%s: %s %s — from %.0f ms to %.0f ms.",
		difference.Metric, Magnitude(difference), verb, difference.Before, difference.After)
}

// Past two times, percentages stop being readable: "6994% slower" forces the
// reader to divide in their head to get to "70 times".
func Magnitude(difference Difference) string {
	if difference.Direction == DirectionSame {
		return "no difference"
	}
	if difference.Before <= 0 || difference.After <= 0 {
		return fmt.Sprintf("%.0f%%", math.Abs(difference.Change)*100)
	}
	greater, lesser := difference.After, difference.Before
	if lesser > greater {
		greater, lesser = lesser, greater
	}
	times := greater / lesser
	if times < 2 {
		return fmt.Sprintf("%.0f%%", math.Abs(difference.Change)*100)
	}
	if times < 10 {
		return fmt.Sprintf("%.1f times", times)
	}
	return fmt.Sprintf("%.0f times", times)
}

func compareSteps(before, after metrics.Document) []StepDifference {
	byName := func(document metrics.Document) map[string]metrics.StepResult {
		table := map[string]metrics.StepResult{}
		for _, step := range document.Steps {
			table[step.Name] = step
		}
		return table
	}
	deAntes, deDepois := byName(before), byName(after)

	names := map[string]bool{}
	for name := range deAntes {
		names[name] = true
	}
	for name := range deDepois {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var diferencas []StepDifference
	for _, name := range sorted {
		previous, existiaAntes := deAntes[name]
		newOne, existsNow := deDepois[name]
		difference := StepDifference{
			Step:     name,
			New:      !existiaAntes,
			Vanished: !existsNow,
			P95:      compareDistribution(name+" (95%)", previous.Reported().P95, newOne.Reported().P95),
			P99:      compareDistribution(name+" (99%)", previous.Reported().P99, newOne.Reported().P99),
		}
		diferencas = append(diferencas, difference)
	}
	return diferencas
}

func compareErrors(before, after metrics.Document) CountDifference {
	difference := CountDifference{Before: before.Overall.ErrorRate * 100, After: after.Overall.ErrorRate * 100}
	switch {
	case difference.Before == 0 && difference.After == 0:
		difference.Sentence = "Neither run had errors."
	case difference.After > difference.Before:
		difference.Sentence = fmt.Sprintf("The error rate went up from %.2f%% to %.2f%%.", difference.Before, difference.After)
	case difference.After < difference.Before:
		difference.Sentence = fmt.Sprintf("The error rate went down from %.2f%% to %.2f%%.", difference.Before, difference.After)
	default:
		difference.Sentence = fmt.Sprintf("The error rate stayed at %.2f%% in both.", difference.Before)
	}
	return difference
}

func phrase(compared Comparison, before, after metrics.Document) string {
	if !compared.Comparable {
		return invalidPhrase(before, after)
	}

	main := compared.Journey
	if before.Journey.Started == 0 || after.Journey.Started == 0 {
		main = compared.Overall
	}

	prefix := "No change worth reading"
	if main.Direction == DirectionWorse {
		prefix = "It got slower"
	}
	if main.Direction == DirectionBetter {
		prefix = "It got faster"
	}

	sentence := fmt.Sprintf("%s: %s", prefix, main.Sentence)
	if compared.Error.After != compared.Error.Before {
		sentence += " " + compared.Error.Sentence
	}
	// Only a blocking caveat explains the difference by itself. Saying it of
	// every caveat made the sentence claim more than the comparison knows, and
	// the field that tells the two apart exists exactly for this.
	var blocking int64
	for _, caveat := range compared.Caveats {
		if caveat.Blocking {
			blocking++
		}
	}
	switch {
	case blocking > 0:
		sentence += fmt.Sprintf(" With %s that could explain the difference on %s own.",
			text.Count(blocking, "caveat", "caveats"),
			text.Pick(blocking, "its", "their"))
	case len(compared.Caveats) > 0:
		sentence += fmt.Sprintf(" With %s about what changed outside the service.",
			text.Count(int64(len(compared.Caveats)), "caveat", "caveats"))
	}
	return sentence
}

// The reason a comparison does not stand is in the sanity check of whichever
// run failed it. Naming saturation for every case put a cause on the screen
// that the run had not reported.
func invalidPhrase(before, after metrics.Document) string {
	var reasons []string
	if !before.Valid() {
		reasons = append(reasons, "the previous one "+firstFinding(before))
	}
	if !after.Valid() {
		reasons = append(reasons, "the new one "+firstFinding(after))
	}
	return "There is nothing to compare, because one of the runs does not hold as a measurement: " + strings.Join(reasons, "; ") + "."
}

func firstFinding(document metrics.Document) string {
	for _, finding := range document.Sanity.Findings {
		return finding.Message
	}
	for _, warning := range document.Warnings {
		if warning.Severity == metrics.SeverityHigh {
			return warning.Message
		}
	}
	return "has an invalid result"
}
