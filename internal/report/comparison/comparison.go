package comparison

import (
	"fmt"
	"math"
	"sort"

	"github.com/Diegobraun/braunrate/internal/metrics"
)

// Two runs do not produce a confidence interval. Change below this is treated
// as noise because, with one sample per side, there is no ground to claim
// anything moved: calling 3% a regression would invent precision.
const AcceptedNoise = 0.05

type Comparison struct {
	Before     Identification   `json:"antes"`
	After      Identification   `json:"depois"`
	Sentence   string           `json:"frase"`
	Comparable bool             `json:"comparavel"`
	Caveats    []string         `json:"ressalvas"`
	Journey    Difference       `json:"jornada"`
	Overall    Difference       `json:"global"`
	Steps      []StepDifference `json:"passos"`
	Error      CountDifference  `json:"taxa_de_erro"`
}

type Identification struct {
	Spec    string `json:"cenario"`
	Target  string `json:"alvo"`
	Start   string `json:"inicio"`
	Version string `json:"versao"`
}

type Difference struct {
	Metrica   string  `json:"metrica"`
	Before    float64 `json:"antes_ms"`
	After     float64 `json:"depois_ms"`
	Change    float64 `json:"variacao"`
	Direction string  `json:"sentido"`
	Sentence  string  `json:"frase"`
}

type StepDifference struct {
	Step     string     `json:"passo"`
	P95      Difference `json:"p95"`
	P99      Difference `json:"p99"`
	New      bool       `json:"novo"`
	Vanished bool       `json:"sumiu"`
}

type CountDifference struct {
	Before   float64 `json:"antes"`
	After    float64 `json:"depois"`
	Sentence string  `json:"frase"`
}

const (
	DirectionWorse  = "piorou"
	DirectionBetter = "melhorou"
	DirectionSame   = "sem diferenca que valha leitura"
)

func Compare(before, after metrics.Document) Comparison {
	c := Comparison{
		Before:     identify(before),
		After:      identify(after),
		Comparable: true,
	}

	c.Caveats = collectCaveats(before, after)
	if !before.Valid() || !after.Valid() {
		c.Comparable = false
	}

	c.Journey = compareDistribution("jornada inteira (95%)", before.Journey.Latency.P95, after.Journey.Latency.P95)
	c.Overall = compareDistribution("todas as requisicoes (95%)", before.Overall.Latency.P95, after.Overall.Latency.P95)
	c.Steps = compareSteps(before, after)
	c.Error = compareErrors(before, after)
	c.Sentence = phrase(c, before, after)
	return c
}

func identify(document metrics.Document) Identification {
	return Identification{
		Spec:    document.Run.Spec,
		Target:  document.Run.Target,
		Start:   document.Run.Start.Format("02/01/2006 15:04"),
		Version: document.Version,
	}
}

// Comparing two runs only holds when both measured the same thing the same
// way; each difference here can explain the whole change on its own.
func collectCaveats(before, after metrics.Document) []string {
	var caveats []string

	if before.Run.Spec != after.Run.Spec {
		caveats = append(caveats, fmt.Sprintf("os cenarios sao diferentes: %q e %q", before.Run.Spec, after.Run.Spec))
	}
	if before.Run.Target != after.Run.Target {
		caveats = append(caveats, fmt.Sprintf("os alvos sao diferentes: %s e %s", before.Run.Target, after.Run.Target))
	}
	if before.Environment.Host != after.Environment.Host || before.Environment.Cores != after.Environment.Cores {
		caveats = append(caveats, fmt.Sprintf("as maquinas geradoras sao diferentes: %s com %d nucleos e %s com %d nucleos",
			before.Environment.Host, before.Environment.Cores, after.Environment.Host, after.Environment.Cores))
	}
	if planSummary(before) != planSummary(after) {
		caveats = append(caveats, fmt.Sprintf("os planos de carga sao diferentes: %s e %s", planSummary(before), planSummary(after)))
	}
	if before.Version != after.Version {
		caveats = append(caveats, fmt.Sprintf("as execucoes usaram versoes diferentes do braunrate: %s e %s", before.Version, after.Version))
	}
	if before.Run.AuthObtains > 0 || after.Run.AuthObtains > 0 {
		caveats = append(caveats, "as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao")
	}
	if !before.Valid() {
		caveats = append(caveats, "a execucao anterior tem resultado invalido: o gerador saturou e o numero dela nao vale como base")
	}
	if !after.Valid() {
		caveats = append(caveats, "a execucao nova tem resultado invalido: o gerador saturou e o numero dela nao vale como comparacao")
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
		summary += fmt.Sprintf("%s ate %.0f/s por %ds", phase.Kind, phase.To, phase.DurationMs/1000)
	}
	return summary
}

func compareDistribution(name string, before, after float64) Difference {
	difference := Difference{Metrica: name, Before: before, After: after, Direction: DirectionSame}
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
		return fmt.Sprintf("%s: sem amostra nas duas execucoes.", difference.Metrica)
	}
	if difference.Direction == DirectionSame {
		return fmt.Sprintf("%s: %.0f ms contra %.0f ms — diferenca dentro do ruido de duas execucoes.",
			difference.Metrica, difference.Before, difference.After)
	}
	verb := "mais lento"
	if difference.Direction == DirectionBetter {
		verb = "mais rapido"
	}
	return fmt.Sprintf("%s: %s %s — de %.0f ms para %.0f ms.",
		difference.Metrica, Magnitude(difference), verb, difference.Before, difference.After)
}

// Past two times, percentages stop being readable: "6994% slower" forces the
// reader to divide in their head to get to "70 times".
func Magnitude(difference Difference) string {
	if difference.Direction == DirectionSame {
		return "sem diferenca"
	}
	if difference.Before <= 0 || difference.After <= 0 {
		return fmt.Sprintf("%.0f%%", math.Abs(difference.Change)*100)
	}
	greater, lesser := difference.After, difference.Before
	if lesser > greater {
		greater, lesser = lesser, greater
	}
	vezes := greater / lesser
	if vezes < 2 {
		return fmt.Sprintf("%.0f%%", math.Abs(difference.Change)*100)
	}
	if vezes < 10 {
		return fmt.Sprintf("%.1f vezes", vezes)
	}
	return fmt.Sprintf("%.0f vezes", vezes)
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
			P95:      compareDistribution(name+" (95%)", previous.Latency.P95, newOne.Latency.P95),
			P99:      compareDistribution(name+" (99%)", previous.Latency.P99, newOne.Latency.P99),
		}
		diferencas = append(diferencas, difference)
	}
	return diferencas
}

func compareErrors(before, after metrics.Document) CountDifference {
	difference := CountDifference{Before: before.Overall.ErrorRate * 100, After: after.Overall.ErrorRate * 100}
	switch {
	case difference.Before == 0 && difference.After == 0:
		difference.Sentence = "Nenhuma das duas execucoes teve erro."
	case difference.After > difference.Before:
		difference.Sentence = fmt.Sprintf("A taxa de erro subiu de %.2f%% para %.2f%%.", difference.Before, difference.After)
	case difference.After < difference.Before:
		difference.Sentence = fmt.Sprintf("A taxa de erro caiu de %.2f%% para %.2f%%.", difference.Before, difference.After)
	default:
		difference.Sentence = fmt.Sprintf("A taxa de erro ficou em %.2f%% nas duas.", difference.Before)
	}
	return difference
}

func phrase(c Comparison, before, after metrics.Document) string {
	if !c.Comparable {
		return "Nao da para comparar: pelo menos uma das execucoes tem resultado invalido porque o gerador saturou."
	}

	main := c.Journey
	if before.Journey.Started == 0 || after.Journey.Started == 0 {
		main = c.Overall
	}

	prefix := "Sem mudanca que valha leitura"
	if main.Direction == DirectionWorse {
		prefix = "Ficou mais lento"
	}
	if main.Direction == DirectionBetter {
		prefix = "Ficou mais rapido"
	}

	sentence := fmt.Sprintf("%s: %s", prefix, main.Sentence)
	if c.Error.After != c.Error.Before {
		sentence += " " + c.Error.Sentence
	}
	if len(c.Caveats) > 0 {
		sentence += fmt.Sprintf(" Com %d ressalva(s) que podem explicar a diferenca sozinhas.", len(c.Caveats))
	}
	return sentence
}
