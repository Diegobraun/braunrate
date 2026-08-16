package metrics

import (
	"strings"
	"testing"
	"time"
)

// sane is a run that means something: every declared step produced samples,
// every journey finished, nothing failed, and the load ran the whole declared
// profile. Each case below breaks exactly one of those.
func sane() (Document, DocumentInput) {
	document := Document{
		Run:        Run{DurationMs: 10_500},
		Scheduling: Scheduling{Sent: 100, Completed: 100},
		Journey:    Journey{Started: 100, Completed: 100},
		Steps: []StepResult{
			{Name: "obter token", Count: 100, Successes: 100},
			{Name: "consultar pedido", Count: 100, Successes: 100},
		},
		Overall: OverallResult{Count: 200, Successes: 200},
	}
	input := DocumentInput{
		DeclaredSteps:   []string{"obter token", "consultar pedido"},
		PlannedDuration: 10 * time.Second,
		PlannedRequests: 100,
	}
	return document, input
}

func TestSaneRunIsNotInvalidated(t *testing.T) {
	document, input := sane()
	sanity := CheckSanity(document, input)
	if !sanity.Valid {
		t.Fatalf("execução sadia foi invalidada: %+v", sanity.Findings)
	}
	if !sanity.Checked {
		t.Error("verificação não ficou marcada como feita")
	}
	if sanity.Sentence != "" {
		t.Errorf("execução valida não deveria ter frase: %q", sanity.Sentence)
	}
}

func TestEachEmptyRunIsCaughtAndOnlyByItsOwnCheck(t *testing.T) {
	cases := []struct {
		name    string
		check   string
		kind    string
		mention string
		break_  func(*Document, *DocumentInput)
	}{
		{
			name:    "nenhuma jornada chegou ao fim",
			check:   "incompleteJourney",
			kind:    "incompleteJourney",
			mention: "nenhuma jornada chegou ao fim",
			break_: func(d *Document, _ *DocumentInput) {
				d.Journey.Completed = 0
			},
		},
		{
			name:    "todos os passos falharam",
			check:   "everythingFailed",
			kind:    "everythingFailed",
			mention: "todos os 2 passos falharam",
			break_: func(d *Document, _ *DocumentInput) {
				for index := range d.Steps {
					d.Steps[index].Successes = 0
					d.Steps[index].Errors = d.Steps[index].Count
					d.Steps[index].ErrorsByClass = map[string]int64{"status": d.Steps[index].Count}
				}
				d.Overall.Successes, d.Overall.Errors = 0, d.Overall.Count
			},
		},
		{
			name:    "um passo teve 100% de erro",
			check:   "everythingFailed",
			kind:    "stepFullyFailed",
			mention: `o passo "consultar pedido" falhou em 100% das requisições`,
			break_: func(d *Document, _ *DocumentInput) {
				d.Steps[1].Successes = 0
				d.Steps[1].Errors = d.Steps[1].Count
				d.Steps[1].ErrorsByClass = map[string]int64{"status": d.Steps[1].Count}
				d.Overall.Successes, d.Overall.Errors = 100, 100
			},
		},
		{
			name:    "a execução durou menos que o perfil declarado",
			check:   "shortRun",
			kind:    "shortRun",
			mention: "parou em 4s com 38 de 100 requisições do perfil declarado",
			break_: func(d *Document, _ *DocumentInput) {
				d.Run.DurationMs = 4_000
				d.Scheduling.Sent = 38
			},
		},
		{
			name:    "um passo declarado não registrou amostra",
			check:   "stepWithoutSample",
			kind:    "stepWithoutSample",
			mention: `o passo "consultar pedido" foi declarado e não registrou nenhuma amostra`,
			break_: func(d *Document, _ *DocumentInput) {
				d.Steps = d.Steps[:1]
				d.Overall.Count, d.Overall.Successes = 100, 100
			},
		},
		{
			name:    "variedade colapsada em fonte com varios valores",
			check:   "medicao_invalidada",
			kind:    "missingVariety",
			mention: "um único valor",
			break_: func(d *Document, _ *DocumentInput) {
				d.Variety = []Variety{{Name: "pedidos.id", Distinct: 1, Uses: 200, Available: 500}}
				d.Warnings = VarietyWarnings(d.Variety)
			},
		},
		{
			name:    "gerador saturado",
			check:   "medicao_invalidada",
			kind:    "generatorSaturated",
			mention: "limite de requisições em voo",
			break_: func(d *Document, _ *DocumentInput) {
				d.Scheduling = Scheduling{Sent: 200, DroppedByInflightLimit: 40, PeakInflight: 512}
				d.Warnings = evaluateWarnings(NewCollector(time.Unix(0, 0), time.Second), *d)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			document, input := sane()
			c.break_(&document, &input)

			sanity := CheckSanity(document, input)
			if sanity.Valid {
				t.Fatalf("execução vazia passou como valida")
			}
			finding, found := findingOfKind(sanity, c.kind)
			if !found {
				t.Fatalf("achado %q não apareceu; achados: %+v", c.kind, sanity.Findings)
			}
			if !strings.Contains(finding.Message, c.mention) {
				t.Errorf("mensagem não explica o caso: %q", finding.Message)
			}
			if finding.Evidence == "" {
				t.Error("achado sem evidencia")
			}
			if !strings.Contains(sanity.Sentence, "não mediu o que se propôs a medir") {
				t.Errorf("frase não diz que a execução não mediu o que se propôs: %q", sanity.Sentence)
			}
			if strings.Contains(sanity.Sentence, "falha do alvo") {
				t.Errorf("frase atribui a falha ao alvo: %q", sanity.Sentence)
			}

			// This is the proof the test fails with the code that came before
			// the check: without it, the same empty run passes as valid.
			withoutIt := runSanityChecks(checksExcept(c.check), document, input)
			if !withoutIt.Valid {
				t.Fatalf("sem a verificação %q a execução ainda foi invalidada por %+v; o teste não prova nada",
					c.check, withoutIt.Findings)
			}
		})
	}
}

func TestSingleFailingStepIsNamedInsteadOfCounted(t *testing.T) {
	document, input := sane()
	document.Steps = []StepResult{{
		Name: "consultar pedido", Count: 60, Errors: 60,
		ErrorsByClass: map[string]int64{"status": 60},
	}}
	document.Overall = OverallResult{Count: 60, Errors: 60}
	input.DeclaredSteps = []string{"consultar pedido"}
	input.PlannedRequests = 60
	document.Scheduling.Sent = 60

	finding, found := findingOfKind(CheckSanity(document, input), "stepFullyFailed")
	if !found {
		t.Fatal("passo único que falhou inteiro não foi apontado pelo nome")
	}
	if strings.Contains(finding.Message, "todos os 1") {
		t.Errorf("concordancia quebrada: %q", finding.Message)
	}
}

// A 3 s profile at 20/s schedules its last request at 2,95 s, so a complete run
// ends before the declared window closes. Reading the wall clock instead of the
// dispatched count invalidated a healthy run.
func TestRunThatEndsBeforeTheDeclaredWindowIsStillValid(t *testing.T) {
	document, input := sane()
	document.Run.DurationMs = 2_960
	document.Scheduling.Sent = 60
	input.PlannedDuration = 3 * time.Second
	input.PlannedRequests = 60

	if sanity := CheckSanity(document, input); !sanity.Valid {
		t.Fatalf("execução completa foi invalidada por %+v", sanity.Findings)
	}
}

// A drop still means the loop reached that instant: the run is invalid for
// saturation, and saying it also stopped short would be a second, wrong reason.
func TestDroppedRequestsDoNotCountAsAShortRun(t *testing.T) {
	document, input := sane()
	document.Scheduling.Sent = 60
	document.Scheduling.DroppedByInflightLimit = 40

	findings := runShorterThanPlan(document, input)
	if len(findings) > 0 {
		t.Fatalf("descarte por saturacao virou execução curta: %+v", findings)
	}
}

func TestVerdictNeverPassesOnAnInvalidRun(t *testing.T) {
	document, input := sane()
	document.Journey.Completed = 0
	document.Sanity = CheckSanity(document, input)

	if document.Valid() {
		t.Fatal("documento com execução vazia se declarou válido")
	}
}

// A result file written before this check existed has no sanity block, and the
// rule in force then was the high-severity warning.
func TestDocumentWithoutSanityBlockFallsBackToWarnings(t *testing.T) {
	document := Document{}
	if !document.Valid() {
		t.Error("documento antigo sem aviso grave foi tratado como inválido")
	}
	document.Warnings = []Warning{{Kind: "generatorSaturated", Severity: SeverityHigh}}
	if document.Valid() {
		t.Error("documento antigo com aviso grave foi tratado como válido")
	}
}

func findingOfKind(sanity Sanity, kind string) (SanityFinding, bool) {
	for _, finding := range sanity.Findings {
		if finding.Kind == kind {
			return finding, true
		}
	}
	return SanityFinding{}, false
}

func checksExcept(name string) []sanityCheck {
	var kept []sanityCheck
	for _, check := range sanityChecks {
		if check.name != name {
			kept = append(kept, check)
		}
	}
	if len(kept) == len(sanityChecks) {
		panic("verificação inexistente: " + name)
	}
	return kept
}

// An absolute threshold where the sibling check had a proportion: a thirty
// minute run that applied 359.999 of 360.000 requests was invalidated for the
// one missing, while the closed model already tolerated 1% of its window.
func TestAlmostCompleteRunIsNotInvalidatedForOneRequest(t *testing.T) {
	document := Document{
		Scheduling: Scheduling{Sent: 359_999},
		Run:        Run{DurationMs: 1_800_000},
		Journey:    Journey{Started: 359_999, Completed: 359_999},
	}
	input := DocumentInput{PlannedRequests: 360_000, PlannedDuration: 30 * time.Minute}

	sanity := CheckSanity(document, input)
	if !sanity.Valid {
		t.Fatalf("execução com uma requisição a menos em 360.000 foi invalidada: %+v", sanity.Findings)
	}
}

// The tolerance is a proportion, not an amnesty: a run that applied a fifth of
// the declared profile still says so.
func TestRunThatAppliedAFifthOfTheProfileIsStillInvalid(t *testing.T) {
	document := Document{
		Scheduling: Scheduling{Sent: 72_000},
		Run:        Run{DurationMs: 360_000},
		Journey:    Journey{Started: 72_000, Completed: 72_000},
	}
	input := DocumentInput{PlannedRequests: 360_000, PlannedDuration: 30 * time.Minute}

	sanity := CheckSanity(document, input)
	if sanity.Valid {
		t.Fatal("execução que aplicou um quinto do perfil declarado passou como valida")
	}
}
