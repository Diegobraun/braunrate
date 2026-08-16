package metrics

import (
	"strings"
	"testing"
	"time"
)

// dispatches feeds the collector a run whose dispatch delays are chosen, not
// measured: no sleeping, no wall clock, no retry. The same input always gives
// the same verdict, which is the point — the flake this test came from was in
// the machine, never in the rule.
func dispatches(total, late int, delay time.Duration) *Collector {
	start := time.Unix(1_700_000_000, 0).UTC()
	collector := NewCollector(start, 10*time.Millisecond)
	for index := 0; index < total; index++ {
		scheduled := start.Add(time.Duration(index) * time.Millisecond)
		punctual := scheduled.Add(time.Microsecond)
		if index < late {
			punctual = scheduled.Add(delay)
		}
		collector.RecordDispatch(scheduled, punctual, 1000, 1)
	}
	return collector
}

func warningOfKind(warnings []Warning, kind string) (Warning, bool) {
	for _, warning := range warnings {
		if warning.Kind == kind {
			return warning, true
		}
	}
	return Warning{}, false
}

// The flake that started this: examples/graphql-cobranca.yaml alternated between
// exit 0 and exit 3 on a busy machine. Neither run lost a journey and neither
// had an error — the only difference was how many dispatches crossed the late
// threshold, right around the 1% line. The rule is a cliff by design, and this
// test pins both sides of it.
func TestSaturationVerdictSitsExactlyOnOnePercent(t *testing.T) {
	cases := []struct {
		name    string
		total   int
		late    int
		kind    string
		invalid bool
	}{
		{"abaixo de 1% e atraso pontual", 1000, 9, "gerador_com_atraso_pontual", false},
		{"exatamente 1% ja invalida", 1000, 10, "gerador_saturado", true},
		{"acima de 1% invalida", 1000, 40, "gerador_saturado", true},
		{"nenhum atraso nao gera aviso", 1000, 0, "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			collector := dispatches(c.total, c.late, 30*time.Millisecond)
			collector.Close()
			document := BuildDocument(collector, DocumentInput{PlannedRequests: int64(c.total)})

			if document.Valid() == c.invalid {
				t.Fatalf("%d de %d atrasados: valido=%v, esperado invalido=%v; achados: %+v",
					c.late, c.total, document.Valid(), c.invalid, document.Sanity.Findings)
			}
			if c.kind == "" {
				if _, had := warningOfKind(document.Warnings, "gerador_saturado"); had {
					t.Error("execucao pontual acusou saturacao")
				}
				return
			}
			warning, had := warningOfKind(document.Warnings, c.kind)
			if !had {
				t.Fatalf("aviso %q nao apareceu; avisos: %+v", c.kind, document.Warnings)
			}
			if warning.Evidence == "" {
				t.Error("aviso sem evidencia")
			}
		})
	}
}

// Two sentences in the same report contradicted each other: the generator had
// missed 4% of its dispatches, and right below, that dispatch had stayed
// punctual. The second one is the claim this test protects.
func TestDegradationIsNotBlamedOnTheTargetWhenDispatchSlipped(t *testing.T) {
	growing := []Bucket{
		{LatencyP99Ms: 10}, {LatencyP99Ms: 20}, {LatencyP99Ms: 40}, {LatencyP99Ms: 90},
	}

	punctual := Document{
		Scheduling: Scheduling{Sent: 1000, LateDispatches: 0},
		Series:     growing,
	}
	warning, had := detectTargetDegradation(punctual)
	if !had {
		t.Fatal("com despacho pontual, a degradacao do alvo precisa ser apontada")
	}
	if !strings.Contains(warning.Message, "despacho continuou pontual") {
		t.Errorf("mensagem mudou sem o teste acompanhar: %q", warning.Message)
	}

	slipped := punctual
	slipped.Scheduling.LateDispatches = 40
	if _, had := detectTargetDegradation(slipped); had {
		t.Error("com 4% dos despachos atrasados, o relatorio afirmou que o despacho continuou pontual")
	}

	dropped := punctual
	dropped.Scheduling.DroppedByInflightLimit = 1
	if _, had := detectTargetDegradation(dropped); had {
		t.Error("com requisicao descartada por saturacao, a degradacao nao pode ser atribuida ao alvo")
	}
}
