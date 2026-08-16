package scenario

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// A messaging broker has no mandatory scheme: "127.0.0.1:9092" is what people
// paste from a docker-compose, and refusing it would ask them to learn a syntax
// Kafka itself does not use.
func validTarget(target string) bool {
	if address, err := url.Parse(target); err == nil && address.Scheme != "" && address.Host != "" {
		return true
	}
	hostname, port, found := strings.Cut(target, ":")
	if !found || hostname == "" || port == "" {
		return false
	}
	for _, char := range port {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (c Spec) Validate() error {
	var problems []string

	if strings.TrimSpace(c.Name) == "" {
		problems = append(problems, "o cenario precisa de um nome")
	}
	if strings.TrimSpace(c.Target) == "" {
		problems = append(problems, "o cenario precisa de um alvo")
	} else if !validTarget(c.Target) {
		problems = append(problems, fmt.Sprintf("alvo invalido: %q (use https://api.exemplo.com, kafka://127.0.0.1:9092 ou amqp://usuario:senha@127.0.0.1:5672/)", c.Target))
	}
	if len(c.Steps) == 0 {
		problems = append(problems, "o cenario precisa de pelo menos um passo")
	}
	switch {
	case c.Load.Closed() && c.Load.Users <= 0:
		problems = append(problems, "o modelo fechado precisa de pelo menos um usuario")
	case c.Load.Closed() && c.Load.For <= 0:
		problems = append(problems, "o modelo fechado precisa de duracao")
	case !c.Load.Closed() && len(c.Load.Phases) == 0:
		problems = append(problems, "o cenario precisa de pelo menos um perfil de carga")
	}

	seen := map[string]int{}
	for _, step := range c.Steps {
		seen[step.Name]++
		if seen[step.Name] == 2 {
			problems = append(problems, fmt.Sprintf("passo com nome repetido: %q (o relatorio agrega por nome)", step.Name))
		}
	}

	for _, phase := range c.Load.Phases {
		if phase.For <= 0 {
			problems = append(problems, fmt.Sprintf("linha %d: perfil %s sem duracao", phase.Line, phase.Kind))
		}
		if phase.Kind != PhaseRamp && phase.To <= 0 {
			problems = append(problems, fmt.Sprintf("linha %d: perfil %s sem taxa", phase.Line, phase.Kind))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("cenario invalido:\n  - %s", strings.Join(problems, "\n  - "))
}

// The rate is shown at three response times because that is the whole point: in
// the closed model it is the target that decides the load, so a single number
// would be the very promise this model cannot keep.
func ClosedModelWarning(spec Spec) (string, bool) {
	if !spec.Load.Closed() {
		return "", false
	}
	think := spec.Load.ThinkTime.Seconds()
	rate := func(response float64) float64 { return float64(spec.Load.Users) / (think + response) }

	line := fmt.Sprintf("Atencao: 'modelo: fechado' nao declara carga, declara %d lacos.\n", spec.Load.Users)
	line += "    Cada usuario so pede de novo depois da resposta anterior: se o alvo travar, eles param de pedir\n"
	line += "    junto e o atraso nao aparece na medicao — e o oposto do modelo aberto, que insiste na taxa.\n"
	line += fmt.Sprintf("    Taxa aproximada com esses %d usuarios: %.0f/s se o alvo responder em 100 ms, %.0f/s em 500 ms, %.0f/s em 2s.",
		spec.Load.Users, rate(0.1), rate(0.5), rate(2))
	return line, true
}

// GateWarnings reports what a declared gate leaves out. A scenario with several
// steps and only step rules approves each piece and says nothing about the wait
// the user actually feels, which is the sum of them.
func GateWarnings(spec Spec) []string {
	if len(spec.SLO) == 0 {
		return nil
	}
	declared := map[SLOScope]bool{}
	for _, rule := range spec.SLO {
		declared[rule.Scope] = true
	}

	var warnings []string
	if len(spec.Steps) > 1 && !declared[ScopeJourney] {
		warnings = append(warnings, fmt.Sprintf(
			"Atencao: o gate mede %d passos isolados e deixa de fora a jornada inteira, que e o tempo que o usuario espera.\n"+
				"    declare tambem:  - jornada: { p95: < 2s, p99: < 5s }", len(spec.Steps)))
	}
	if declared[ScopeRegression] {
		warnings = append(warnings, "Atencao: ha regra de regressao declarada; ela so e verificada com 'braunrate execute ... -baseline=execucao-anterior.json'.")
	}
	return warnings
}

// KnownRequirements is the closed list on purpose: an unknown name would be
// declared, printed and never checked by anyone, which is worse than not
// declaring it.
var KnownRequirements = []string{"kafka", "amqp", "credencial"}

func readRequirements(no *yaml.Node) ([]string, error) {
	if no.Kind != yaml.SequenceNode {
		return nil, nodeError(no, "requer precisa ser uma lista, por exemplo: requer: [kafka]\n"+
			"    declara a infraestrutura externa sem a qual este cenario nao roda")
	}
	requirements := make([]string, 0, len(no.Content))
	for _, item := range no.Content {
		if !slices.Contains(KnownRequirements, item.Value) {
			return nil, nodeError(item, "dependencia desconhecida: %q\n%s",
				item.Value, suggest(item.Value, KnownRequirements))
		}
		requirements = append(requirements, item.Value)
	}
	return requirements, nil
}
