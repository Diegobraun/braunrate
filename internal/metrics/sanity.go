package metrics

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Sanity answers, before any SLO is read, whether the result means anything.
// Checked separates a verified run from a document written by an older version,
// which has no sanity block at all.
type Sanity struct {
	Checked  bool            `json:"checked"`
	Valid    bool            `json:"valid"`
	Findings []SanityFinding `json:"findings"`
	Sentence string          `json:"sentence"`
}

type SanityFinding struct {
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	Evidence string `json:"evidence"`
}

// Says nothing about the target: a run where everything failed may well be the
// target falling over, and claiming otherwise would be a second wrong claim.
const invalidSentence = "Resultado inválido: a execução não mediu o que se propôs a medir. Isto não é veredito sobre o alvo — é a medição que não vale, e por isso nenhuma regra de SLO foi avaliada."

// What counts as the declared load having been applied. Below it the run
// measured a piece of the profile and says so.
const appliedTolerance = 99

type sanityCheck struct {
	name string
	run  func(Document, DocumentInput) []SanityFinding
}

var sanityChecks = []sanityCheck{
	{"jornada_incompleta", noJourneyCompleted},
	{"passo_sem_amostra", declaredStepWithoutSamples},
	{"tudo_falhou", everythingFailed},
	{"execucao_curta", runShorterThanPlan},
	{"medicao_invalidada", invalidatingWarnings},
}

func CheckSanity(document Document, input DocumentInput) Sanity {
	return runSanityChecks(sanityChecks, document, input)
}

func runSanityChecks(checks []sanityCheck, document Document, input DocumentInput) Sanity {
	sanity := Sanity{Checked: true, Valid: true}
	for _, check := range checks {
		sanity.Findings = append(sanity.Findings, check.run(document, input)...)
	}
	if len(sanity.Findings) > 0 {
		sanity.Valid = false
		sanity.Sentence = invalidSentence
	}
	return sanity
}

func noJourneyCompleted(document Document, _ DocumentInput) []SanityFinding {
	if document.Journey.Started == 0 || document.Journey.Completed > 0 {
		return nil
	}
	return []SanityFinding{{
		Kind:     "jornada_incompleta",
		Message:  "nenhuma jornada chegou ao fim, então o cenário não exercitou a sequência que declarou. Rode 'braunrate debug' para ver onde a iteração para",
		Evidence: fmt.Sprintf("%s jornadas iniciadas, 0 completas", thousands(document.Journey.Started)),
	}}
}

func declaredStepWithoutSamples(document Document, input DocumentInput) []SanityFinding {
	if len(input.DeclaredSteps) == 0 {
		return nil
	}
	withSamples := map[string]bool{}
	for _, step := range document.Steps {
		if step.Count > 0 {
			withSamples[step.Name] = true
		}
	}
	var findings []SanityFinding
	for _, declared := range input.DeclaredSteps {
		if withSamples[declared] {
			continue
		}
		findings = append(findings, SanityFinding{
			Kind:     "passo_sem_amostra",
			Message:  fmt.Sprintf("o passo %q foi declarado e não registrou nenhuma amostra; ele ficou de fora da medição", declared),
			Evidence: fmt.Sprintf("passos com amostra: %s", listOrNone(sortedNames(withSamples))),
		})
	}
	return findings
}

// Latency of a step that always failed is the time the target took to refuse,
// never the time of the work the scenario meant to measure.
func everythingFailed(document Document, _ DocumentInput) []SanityFinding {
	var failed []StepResult
	var ran int
	for _, step := range document.Steps {
		if step.Count == 0 {
			continue
		}
		ran++
		if step.Errors == step.Count {
			failed = append(failed, step)
		}
	}
	if ran == 0 || len(failed) == 0 {
		return nil
	}
	if len(failed) == ran && ran > 1 {
		return []SanityFinding{{
			Kind:    "tudo_falhou",
			Message: fmt.Sprintf("todos os %d passos falharam em 100%% das requisições; o tempo de resposta acima é o tempo que o alvo levou para recusar, não o tempo do trabalho que o cenário queria medir", ran),
			Evidence: fmt.Sprintf("%s requisições, %s erros (%s)",
				thousands(document.Overall.Count), thousands(document.Overall.Errors), dominantClasses(document.Steps)),
		}}
	}
	findings := make([]SanityFinding, 0, len(failed))
	for _, step := range failed {
		findings = append(findings, SanityFinding{
			Kind:     "passo_totalmente_falho",
			Message:  fmt.Sprintf("o passo %q falhou em 100%% das requisições; nenhuma resposta bem-sucedida entrou na medição dele", step.Name),
			Evidence: fmt.Sprintf("%s requisições, %s erros (%s)", thousands(step.Count), thousands(step.Errors), dominantClasses([]StepResult{step})),
		})
	}
	return findings
}

// Wall clock is the wrong ruler: a 3 s profile at 20/s schedules its last
// request at 2,95 s, so a complete run ends before the window closes. Dropped
// counts as applied — the loop did get there.
//
// The tolerance is the same the closed model already uses for the same
// question. Demanding every single request invalidated a thirty minute run for
// missing one of 360.000 — an absolute threshold where the sibling check had a
// proportion, and the same shape as the defect that let a 98% broken run pass.
func runShorterThanPlan(document Document, input DocumentInput) []SanityFinding {
	if input.PlannedRequests <= 0 {
		return runShorterThanWindow(document, input)
	}
	applied := document.Scheduling.Sent + document.Scheduling.DroppedByInflightLimit
	if applied >= input.PlannedRequests*appliedTolerance/100 {
		return nil
	}
	actual := time.Duration(document.Run.DurationMs) * time.Millisecond
	return []SanityFinding{{
		Kind: "execucao_curta",
		Message: fmt.Sprintf("a execução parou em %s com %s de %s requisições do perfil declarado; a carga declarada não chegou a ser aplicada inteira, e o que ficou medido é só o pedaço que rodou",
			readableDuration(actual), thousands(applied), thousands(input.PlannedRequests)),
		Evidence: fmt.Sprintf("perfil declarado: %s requisições em %s; execução: %s requisições em %s",
			thousands(input.PlannedRequests), readableDuration(input.PlannedDuration), thousands(applied), readableDuration(actual)),
	}}
}

// The closed loop has no request count to compare against: it runs until the
// window closes, so a run that stopped early is one that was interrupted.
func runShorterThanWindow(document Document, input DocumentInput) []SanityFinding {
	if !document.Closed() || input.PlannedDuration <= 0 {
		return nil
	}
	actual := time.Duration(document.Run.DurationMs) * time.Millisecond
	if actual >= input.PlannedDuration*appliedTolerance/100 {
		return nil
	}
	return []SanityFinding{{
		Kind: "execucao_curta",
		Message: fmt.Sprintf("a execução parou em %s da janela de %s declarada; a carga declarada não chegou a ser aplicada inteira, e o que ficou medido é só o pedaço que rodou",
			readableDuration(actual), readableDuration(input.PlannedDuration)),
		Evidence: fmt.Sprintf("%d usuários em laço fechado, %s jornadas iniciadas", document.Run.Users, thousands(document.Journey.Started)),
	}}
}

func invalidatingWarnings(document Document, _ DocumentInput) []SanityFinding {
	var findings []SanityFinding
	for _, warning := range document.Warnings {
		if warning.Severity != SeverityHigh {
			continue
		}
		findings = append(findings, SanityFinding{
			Kind:     warning.Kind,
			Message:  warning.Message,
			Evidence: warning.Evidence,
		})
	}
	return findings
}

func dominantClasses(steps []StepResult) string {
	total := map[string]int64{}
	for _, step := range steps {
		for class, count := range step.ErrorsByClass {
			total[class] += count
		}
	}
	if len(total) == 0 {
		return "sem classe registrada"
	}
	names := make([]string, 0, len(total))
	for class := range total {
		names = append(names, class)
	}
	sort.Slice(names, func(i, j int) bool {
		if total[names[i]] != total[names[j]] {
			return total[names[i]] > total[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s: %s", name, thousands(total[name])))
	}
	return strings.Join(parts, ", ")
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "nenhum"
	}
	return strings.Join(names, ", ")
}

func readableDuration(value time.Duration) string {
	if value >= time.Second {
		return value.Round(100 * time.Millisecond).String()
	}
	return value.Round(time.Millisecond).String()
}
