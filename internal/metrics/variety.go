package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/text"
)

// Cap on distinct values kept per variable. What matters is telling "a single
// value" from "many"; counting a million exactly would cost memory proportional
// to the load without changing any conclusion.
const distinctValuesCap = 1024

type Variety struct {
	Name      string `json:"name"`
	Distinct  int64  `json:"distinctValues"`
	Uses      int64  `json:"uses"`
	Available int64  `json:"availableValues"`
	Capped    bool   `json:"cappedByLimit"`
	// Range is what a count of distinct values cannot say: a thousand different
	// ids that all belong to one customer exercise one slice of the target, and
	// the count alone reads as full coverage (ADR 0007).
	Range *Range `json:"range,omitempty"`
	// Shapes carries the body shapes themselves, not a count of them: "2 formas"
	// tells nobody which two, and the difference between them is the whole point.
	Shapes   []string `json:"observedShapes,omitempty"`
	Sentence string   `json:"sentence"`
	// O que o protocolo dono desta dimensao diz sobre ela ter colapsado. A
	// medicao decide se avisa e com que gravidade; o dominio vem de quem sabe.
	Collapse *protocol.Collapse `json:"collapse,omitempty"`
}

// Range describes where the values landed. Numbers get the interval they
// covered; text gets the prefix they all share, which is how a single customer,
// tenant or region hides behind values that are all different.
type Range struct {
	Kind   string  `json:"kind"`
	Min    float64 `json:"min,omitempty"`
	Max    float64 `json:"max,omitempty"`
	Prefix string  `json:"commonPrefix,omitempty"`
}

const (
	NumericRange = "numerica"
	PrefixRange  = "prefixo"
)

type varietyCounter struct {
	seen     map[string]struct{}
	uses     int64
	capped   bool
	collapse *protocol.Collapse

	numeric  bool
	first    bool
	min, max float64
	prefix   string
}

func (c *varietyCounter) record(value string) {
	c.uses++
	c.span(value)
	if c.capped {
		return
	}
	if len(c.seen) >= distinctValuesCap {
		c.capped = true
		return
	}
	c.seen[value] = struct{}{}
}

// The span is kept for every use, not only for the distinct ones kept below the
// cap: a run of a million requests still has a first and a last value, and the
// interval they covered is exactly what the cap throws away.
func (c *varietyCounter) span(value string) {
	number, isNumber := parseNumber(value)
	if !c.first {
		c.first = true
		c.numeric = isNumber
		c.min, c.max = number, number
		c.prefix = value
		return
	}
	if isNumber && c.numeric {
		if number < c.min {
			c.min = number
		}
		if number > c.max {
			c.max = number
		}
	} else {
		c.numeric = false
	}
	c.prefix = commonPrefix(c.prefix, value)
}

func parseNumber(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func commonPrefix(first, second string) string {
	limit := len(first)
	if len(second) < limit {
		limit = len(second)
	}
	for index := 0; index < limit; index++ {
		if first[index] != second[index] {
			return first[:index]
		}
	}
	return first[:limit]
}

// A prefix only says something when it takes up most of what the values are:
// two ids that happen to start with the same digit share nothing worth
// reporting, and saying it would bury the cases that matter.
const meaningfulPrefix = 4

func (c *varietyCounter) observedRange() *Range {
	if !c.first || c.uses < 2 {
		return nil
	}
	if c.numeric {
		return &Range{Kind: NumericRange, Min: c.min, Max: c.max}
	}
	if len(c.prefix) >= meaningfulPrefix {
		return &Range{Kind: PrefixRange, Prefix: c.prefix}
	}
	return nil
}

// Availability maps a variable to how many values its source can offer. It is
// what tells a defect (one value used out of many) from a scenario that
// declared a single value on purpose.
type Availability map[string]int64

const UnknownAvailability = int64(-1)

func buildVarieties(counters map[string]*varietyCounter, available Availability) []Variety {
	names := make([]string, 0, len(counters))
	for name := range counters {
		names = append(names, name)
	}
	sort.Strings(names)

	varieties := make([]Variety, 0, len(names))
	for _, name := range names {
		counter := counters[name]
		variety := Variety{
			Name:     name,
			Distinct: int64(len(counter.seen)),
			Uses:     counter.uses,
			Capped:   counter.capped,
		}
		if counter.capped {
			variety.Distinct = distinctValuesCap
		}
		variety.Collapse = counter.collapse
		if howMany, knows := available[name]; knows {
			variety.Available = howMany
		}
		if variety.Distinct > 1 || variety.Capped {
			// A single value has no range to report: "all between 7 and 7" says
			// less than the sentence about the single value already says.
			variety.Range = counter.observedRange()
		}
		if bodyShape(name) {
			variety.Shapes = shapesSeen(counter)
			variety.Range = nil
		}
		variety.Sentence = phraseVariety(variety)
		varieties = append(varieties, variety)
	}
	return varieties
}

func phraseVariety(variety Variety) string {
	if bodyShape(variety.Name) {
		return phraseShape(variety)
	}
	if variety.Capped {
		return fmt.Sprintf("mais de %d valores distintos de %s em %s usos%s",
			distinctValuesCap-1, variety.Name, thousands(variety.Uses), phraseRange(variety.Range))
	}
	if variety.Distinct == 1 {
		return fmt.Sprintf("1 single value of %s across %s uses", variety.Name, thousands(variety.Uses))
	}
	return fmt.Sprintf("%d valores distintos de %s em %s usos%s",
		variety.Distinct, variety.Name, thousands(variety.Uses), phraseRange(variety.Range))
}

func phraseShape(variety Variety) string {
	step := strings.TrimPrefix(variety.Name, BodyShapeName)
	count := variety.Distinct
	if variety.Capped {
		count = distinctValuesCap
	}
	sentence := fmt.Sprintf("%s de corpo em %q, em %s envios",
		text.Count(count, "forma", "formas"), step, thousands(variety.Uses))
	if count == 1 && len(variety.Shapes) == 1 {
		return sentence + ": " + variety.Shapes[0]
	}
	return sentence
}

func phraseRange(interval *Range) string {
	if interval == nil {
		return ""
	}
	if interval.Kind == PrefixRange {
		return fmt.Sprintf(", all starting with %q", interval.Prefix)
	}
	if interval.Min == interval.Max {
		return fmt.Sprintf(", todos iguais a %s", number(interval.Min))
	}
	return fmt.Sprintf(", entre %s e %s", number(interval.Min), number(interval.Max))
}

func number(value float64) string {
	if value == float64(int64(value)) {
		return thousands(int64(value))
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// VarietyWarnings reports load that concentrated on a single value.
//
// The bug behind this metric: auth froze the first iteration's data and the
// whole run went against one subscriber, while the report claimed a variety
// that never happened.
//
// Severity separates two different cases: a source with many values and a run
// with one is a defect and invalidates the result; a fixed value declared in
// the scenario is the author's choice, and becomes a reading warning.
func VarietyWarnings(varieties []Variety) []Warning {
	var warnings []Warning
	for _, variety := range varieties {
		// One body shape is the normal case, not concentration: the shape comes
		// from the scenario, and repeating it back as a defect would be the same
		// noise ADR 0007 refuses for a source that only has one value.
		if bodyShape(variety.Name) {
			warnings = append(warnings, emptyFieldWarnings(variety)...)
			continue
		}
		if variety.Distinct != 1 || variety.Uses < 2 {
			continue
		}
		if variety.Available == 1 {
			continue
		}

		if variety.Available == 0 {
			warnings = append(warnings, Warning{
				Kind:     "valor_fixo",
				Severity: SeverityMedium,
				Message: fmt.Sprintf("the whole load used the same value of %s; if the target caches by that value, the number comes out optimistic",
					variety.Name),
				Evidence: fmt.Sprintf("%s: 1 valor em %s usos", variety.Name, thousands(variety.Uses)),
			})
			continue
		}

		message := fmt.Sprintf("the whole run went with a single value of %s, even though the source has more; the target may have answered from cache, and the result does not represent the declared load",
			variety.Name)
		severity := SeverityHigh
		// Uma concentracao que o cenario pediu nao e o mesmo defeito: ninguem
		// esqueceu de variar, e mandar variar manda procurar um defeito que a
		// pessoa nao escreveu. Continua valendo dizer, porque o numero que sai
		// nao tem a forma de producao.
		if note := variety.Collapse; note != nil {
			message = fmt.Sprintf("toda a carga caiu em %s: %s. %s", note.Subject, note.Meaning, note.Remedy)
			if note.Declared {
				severity = SeverityMedium
			}
		}

		evidence := fmt.Sprintf("%s had %d available values and the run used 1, across %s uses",
			variety.Name, variety.Available, thousands(variety.Uses))
		if variety.Available < 0 {
			evidence = fmt.Sprintf("%s is generated per iteration and still repeated the same value across %s uses",
				variety.Name, thousands(variety.Uses))
		}
		warnings = append(warnings, Warning{
			Kind:     "variedade_ausente",
			Severity: severity,
			Message:  message,
			Evidence: evidence,
		})
	}
	return warnings
}

func thousands(value int64) string {
	text := fmt.Sprintf("%d", value)
	if len(text) <= 3 {
		return text
	}
	var parts []string
	for len(text) > 3 {
		parts = append([]string{text[len(text)-3:]}, parts...)
		text = text[:len(text)-3]
	}
	parts = append([]string{text}, parts...)
	return join(parts, ".")
}

func join(parts []string, separator string) string {
	out := ""
	for index, part := range parts {
		if index > 0 {
			out += separator
		}
		out += part
	}
	return out
}
