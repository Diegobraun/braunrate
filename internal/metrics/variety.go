package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Diegobraun/braunrate/internal/texto"
)

// Cap on distinct values kept per variable. What matters is telling "a single
// value" from "many"; counting a million exactly would cost memory proportional
// to the load without changing any conclusion.
const distinctValuesCap = 1024

type Variety struct {
	Name      string `json:"nome"`
	Distinct  int64  `json:"valores_distintos"`
	Uses      int64  `json:"usos"`
	Available int64  `json:"valores_disponiveis"`
	Capped    bool   `json:"limitado_pelo_teto"`
	// Range is what a count of distinct values cannot say: a thousand different
	// ids that all belong to one customer exercise one slice of the target, and
	// the count alone reads as full coverage (ADR 0007).
	Range *Range `json:"faixa,omitempty"`
	// Shapes carries the body shapes themselves, not a count of them: "2 formas"
	// tells nobody which two, and the difference between them is the whole point.
	Shapes   []string `json:"formas_observadas,omitempty"`
	Sentence string   `json:"frase"`
}

// Range describes where the values landed. Numbers get the interval they
// covered; text gets the prefix they all share, which is how a single customer,
// tenant or region hides behind values that are all different.
type Range struct {
	Kind   string  `json:"tipo"`
	Min    float64 `json:"minimo,omitempty"`
	Max    float64 `json:"maximo,omitempty"`
	Prefix string  `json:"prefixo_comum,omitempty"`
}

const (
	NumericRange = "numerica"
	PrefixRange  = "prefixo"
)

type varietyCounter struct {
	seen   map[string]struct{}
	uses   int64
	capped bool

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
		return fmt.Sprintf("1 unico valor de %s em %s usos", variety.Name, thousands(variety.Uses))
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
		texto.Count(count, "forma", "formas"), step, thousands(variety.Uses))
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
		return fmt.Sprintf(", todos comecando com %q", interval.Prefix)
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
				Message: fmt.Sprintf("a carga inteira usou o mesmo valor de %s; se o alvo guardar resposta por esse valor, o numero fica otimista",
					variety.Name),
				Evidence: fmt.Sprintf("%s: 1 valor em %s usos", variety.Name, thousands(variety.Uses)),
			})
			continue
		}

		message := fmt.Sprintf("a execucao inteira rodou com um unico valor de %s, embora a fonte tenha mais; o alvo pode ter respondido de cache, e o resultado nao representa a carga declarada",
			variety.Name)
		severity := SeverityHigh
		if strings.HasPrefix(variety.Name, "kafka.particao.") {
			message = fmt.Sprintf("toda a carga caiu numa particao so de %s; o resto do cluster ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao",
				strings.TrimPrefix(strings.TrimPrefix(variety.Name, "kafka.particao.consumida."), "kafka.particao."))
		}
		// A partition the scenario asked for is not the same defect: nobody
		// forgot to vary the key, and telling them to vary it sends them looking
		// for a bug they did not write. The concentration is still worth saying,
		// because the number that comes out is not production shape.
		if strings.HasPrefix(variety.Name, "kafka.particao.declarada.") {
			message = fmt.Sprintf("toda a carga caiu na particao declarada de %s: o resto do cluster ficou parado e este numero nao representa producao — e o de uma particao, nao o do topico. Tire 'particao' do passo para distribuir",
				strings.TrimPrefix(variety.Name, "kafka.particao.declarada."))
			severity = SeverityMedium
		}

		evidence := fmt.Sprintf("%s tinha %d valores disponiveis e a execucao usou 1, em %s usos",
			variety.Name, variety.Available, thousands(variety.Uses))
		if variety.Available < 0 {
			evidence = fmt.Sprintf("%s e gerada por iteracao e mesmo assim repetiu o mesmo valor em %s usos",
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
