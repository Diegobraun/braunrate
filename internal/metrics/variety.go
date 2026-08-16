package metrics

import (
	"fmt"
	"sort"
	"strings"
)

// Teto de valores distintos guardados por variavel. O que importa e distinguir
// "um valor so" de "muitos"; contar exatamente um milhao de valores custaria
// memoria proporcional a carga sem mudar nenhuma conclusao.
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

// Disponivel por variavel: quantos valores a fonte que alimenta aquela variavel
// tem para oferecer. E o que permite dizer que usar um valor so foi defeito, e
// nao um cenario que declarou um valor so.
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

// O bug que motivou esta metrica: a autenticacao congelava os dados da primeira
// iteracao e a execucao inteira rodava sobre um assinante so, com o relatorio
// declarando variedade que nao existiu.
//
// A gravidade separa dois casos diferentes: fonte com varios valores e execucao
// com um so e defeito e invalida o resultado; valor fixo declarado no cenario e
// escolha de quem escreveu, e vira aviso de leitura.
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
