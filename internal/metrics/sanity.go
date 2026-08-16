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
	Checked  bool            `json:"verificada"`
	Valid    bool            `json:"valida"`
	Findings []SanityFinding `json:"achados"`
	Sentence string          `json:"frase"`
}

type SanityFinding struct {
	Kind     string `json:"tipo"`
	Message  string `json:"mensagem"`
	Evidence string `json:"evidencia"`
}

// Says nothing about the target: a run where everything failed may well be the
// target falling over, and claiming otherwise would be a second wrong claim.
const invalidSentence = "Resultado invalido: a execucao nao mediu o que se propos a medir. Isto nao e veredito sobre o alvo — e a medicao que nao vale, e por isso nenhuma regra de SLO foi avaliada."

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
		Message:  "nenhuma jornada chegou ao fim, entao o cenario nao exercitou a sequencia que declarou. Rode 'braunrate debug' para ver onde a iteracao para",
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
			Message:  fmt.Sprintf("o passo %q foi declarado e nao registrou nenhuma amostra; ele ficou de fora da medicao", declared),
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
			Message: fmt.Sprintf("todos os %d passos falharam em 100%% das requisicoes; a latencia acima e o tempo que o alvo levou para recusar, nao o tempo do trabalho que o cenario queria medir", ran),
			Evidence: fmt.Sprintf("%s requisicoes, %s erros (%s)",
				thousands(document.Overall.Count), thousands(document.Overall.Errors), dominantClasses(document.Steps)),
		}}
	}
	findings := make([]SanityFinding, 0, len(failed))
	for _, step := range failed {
		findings = append(findings, SanityFinding{
			Kind:     "passo_totalmente_falho",
			Message:  fmt.Sprintf("o passo %q falhou em 100%% das requisicoes; nenhuma resposta bem-sucedida entrou na medicao dele", step.Name),
			Evidence: fmt.Sprintf("%s requisicoes, %s erros (%s)", thousands(step.Count), thousands(step.Errors), dominantClasses([]StepResult{step})),
		})
	}
	return findings
}

// Wall clock is the wrong ruler: a 3 s profile at 20/s schedules its last
// request at 2,95 s, so a complete run ends before the window closes. Dropped
// counts as applied — the loop did get there.
func runShorterThanPlan(document Document, input DocumentInput) []SanityFinding {
	if input.PlannedRequests <= 0 {
		return runShorterThanWindow(document, input)
	}
	applied := document.Scheduling.Sent + document.Scheduling.DroppedByInflightLimit
	if applied >= input.PlannedRequests {
		return nil
	}
	actual := time.Duration(document.Run.DurationMs) * time.Millisecond
	return []SanityFinding{{
		Kind: "execucao_curta",
		Message: fmt.Sprintf("a execucao parou em %s com %s de %s requisicoes do perfil declarado; a carga declarada nao chegou a ser aplicada inteira, e o que ficou medido e so o pedaco que rodou",
			readableDuration(actual), thousands(applied), thousands(input.PlannedRequests)),
		Evidence: fmt.Sprintf("perfil declarado: %s requisicoes em %s; execucao: %s requisicoes em %s",
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
	if actual >= input.PlannedDuration*99/100 {
		return nil
	}
	return []SanityFinding{{
		Kind: "execucao_curta",
		Message: fmt.Sprintf("a execucao parou em %s da janela de %s declarada; a carga declarada nao chegou a ser aplicada inteira, e o que ficou medido e so o pedaco que rodou",
			readableDuration(actual), readableDuration(input.PlannedDuration)),
		Evidence: fmt.Sprintf("%d usuarios em laco fechado, %s jornadas iniciadas", document.Run.Users, thousands(document.Journey.Started)),
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
