package metrics

import (
	"fmt"
	"sort"
	"strings"
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
	Sentence  string `json:"frase"`
}

type varietyCounter struct {
	seen   map[string]struct{}
	uses   int64
	capped bool
}

func (c *varietyCounter) record(value string) {
	c.uses++
	if c.capped {
		return
	}
	if len(c.seen) >= distinctValuesCap {
		c.capped = true
		return
	}
	c.seen[value] = struct{}{}
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
		variety.Sentence = phraseVariety(variety)
		varieties = append(varieties, variety)
	}
	return varieties
}

func phraseVariety(v Variety) string {
	if v.Capped {
		return fmt.Sprintf("mais de %d valores distintos de %s em %s usos", distinctValuesCap-1, v.Name, thousands(v.Uses))
	}
	if v.Distinct == 1 {
		return fmt.Sprintf("1 unico valor de %s em %s usos", v.Name, thousands(v.Uses))
	}
	return fmt.Sprintf("%d valores distintos de %s em %s usos", v.Distinct, v.Name, thousands(v.Uses))
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
		if strings.HasPrefix(variety.Name, "kafka.particao.") {
			message = fmt.Sprintf("toda a carga caiu numa particao so de %s; o resto do cluster ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao",
				strings.TrimPrefix(strings.TrimPrefix(variety.Name, "kafka.particao.consumida."), "kafka.particao."))
		}

		evidence := fmt.Sprintf("%s tinha %d valores disponiveis e a execucao usou 1, em %s usos",
			variety.Name, variety.Available, thousands(variety.Uses))
		if variety.Available < 0 {
			evidence = fmt.Sprintf("%s e gerada por iteracao e mesmo assim repetiu o mesmo valor em %s usos",
				variety.Name, thousands(variety.Uses))
		}
		warnings = append(warnings, Warning{
			Kind:     "variedade_ausente",
			Severity: SeverityHigh,
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
