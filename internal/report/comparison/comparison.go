package comparison

import (
	"fmt"
	"math"
	"sort"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/texto"
)

// Two runs do not produce a confidence interval. Change below this is treated
// as noise because, with one sample per side, there is no ground to claim
// anything moved: calling 3% a regression would invent precision.
const AcceptedNoise = 0.05

type Comparison struct {
	Before             Identification        `json:"antes"`
	After              Identification        `json:"depois"`
	Sentence           string                `json:"frase"`
	Comparable         bool                  `json:"comparavel"`
	Caveats            []Caveat              `json:"ressalvas"`
	Journey            Difference            `json:"jornada"`
	Overall            Difference            `json:"global"`
	JourneyPercentiles map[string]Difference `json:"jornada_por_percentil"`
	OverallPercentiles map[string]Difference `json:"global_por_percentil"`
	Steps              []StepDifference      `json:"passos"`
	Error              CountDifference       `json:"taxa_de_erro"`
}

// Blocking marks a caveat that explains the whole difference by itself. Only
// those take the verdict away from a regression gate; treating every caveat as
// blocking would disable the gate on any authenticated scenario.
type Caveat struct {
	Text     string `json:"texto"`
	Blocking bool   `json:"impede_comparacao"`
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
	compared := Comparison{
		Before:     identify(before),
		After:      identify(after),
		Comparable: true,
	}

	compared.Caveats = collectCaveats(before, after)
	if !before.Valid() || !after.Valid() {
		compared.Comparable = false
	}

	compared.Journey = compareDistribution("jornada inteira (95%)", before.Journey.Reported().P95, after.Journey.Reported().P95)
	compared.Overall = compareDistribution("todas as requisicoes (95%)", before.Overall.Reported().P95, after.Overall.Reported().P95)
	compared.JourneyPercentiles = comparePercentiles("jornada inteira", before.Journey.Reported(), after.Journey.Reported())
	compared.OverallPercentiles = comparePercentiles("todas as requisicoes", before.Overall.Reported(), after.Overall.Reported())
	compared.Steps = compareSteps(before, after)
	compared.Error = compareErrors(before, after)
	compared.Sentence = phrase(compared, before, after)
	return compared
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
func collectCaveats(before, after metrics.Document) []Caveat {
	var caveats []Caveat
	blocking := func(format string, args ...any) {
		caveats = append(caveats, Caveat{Text: fmt.Sprintf(format, args...), Blocking: true})
	}

	if before.Run.Spec != after.Run.Spec {
		blocking("os cenarios sao diferentes: %q e %q", before.Run.Spec, after.Run.Spec)
	}
	if before.Run.Target != after.Run.Target {
		blocking("os alvos sao diferentes: %s e %s", before.Run.Target, after.Run.Target)
	}
	if before.Environment.Host != after.Environment.Host || before.Environment.Cores != after.Environment.Cores {
		blocking("as maquinas geradoras sao diferentes: %s com %d nucleos e %s com %d nucleos",
			before.Environment.Host, before.Environment.Cores, after.Environment.Host, after.Environment.Cores)
	}
	if planSummary(before) != planSummary(after) {
		blocking("os planos de carga sao diferentes: %s e %s", planSummary(before), planSummary(after))
	}
	if before.Version != after.Version {
		blocking("as execucoes usaram versoes diferentes do braunrate: %s e %s", before.Version, after.Version)
	}
	if before.Run.Model != after.Run.Model {
		blocking("os modelos de chegada sao diferentes: %s e %s. Latencia de laco fechado nao se compara com latencia contada do instante agendado — a segunda inclui um atraso que a primeira nao chega a registrar",
			before.Run.Model, after.Run.Model)
	}
	if !before.Valid() {
		blocking("a execucao anterior tem resultado invalido e o numero dela nao vale como base")
	}
	if !after.Valid() {
		blocking("a execucao nova tem resultado invalido e o numero dela nao vale como comparacao")
	}
	if before.Run.AuthObtains > 0 || after.Run.AuthObtains > 0 {
		caveats = append(caveats, Caveat{Text: "as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao"})
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

func phrase(compared Comparison, before, after metrics.Document) string {
	if !compared.Comparable {
		return "Nao da para comparar: pelo menos uma das execucoes tem resultado invalido porque o gerador saturou."
	}

	main := compared.Journey
	if before.Journey.Started == 0 || after.Journey.Started == 0 {
		main = compared.Overall
	}

	prefix := "Sem mudanca que valha leitura"
	if main.Direction == DirectionWorse {
		prefix = "Ficou mais lento"
	}
	if main.Direction == DirectionBetter {
		prefix = "Ficou mais rapido"
	}

	sentence := fmt.Sprintf("%s: %s", prefix, main.Sentence)
	if compared.Error.After != compared.Error.Before {
		sentence += " " + compared.Error.Sentence
	}
	if len(compared.Caveats) > 0 {
		sentence += fmt.Sprintf(" Com %s que %s explicar a diferenca sozinha%s.",
			texto.Count(int64(len(compared.Caveats)), "ressalva", "ressalvas"),
			texto.Pick(int64(len(compared.Caveats)), "pode", "podem"),
			texto.Pick(int64(len(compared.Caveats)), "", "s"))
	}
	return sentence
}
