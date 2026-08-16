package scenario

import (
	"fmt"
	"net/url"
	"strings"
)

// A messaging broker has no mandatory scheme: "127.0.0.1:9092" is what people
// paste from a docker-compose, and refusing it would ask them to learn a syntax
// Kafka itself does not use.
func validTarget(target string) bool {
	if address, err := url.Parse(target); err == nil && address.Scheme != "" && address.Host != "" {
		return true
	}
	maquina, port, found := strings.Cut(target, ":")
	if !found || maquina == "" || port == "" {
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
	if len(c.Load.Phases) == 0 {
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

// GateWarnings reports what a declared gate leaves out. A scenario with several
// steps and only step rules approves each piece and says nothing about the wait
// the user actually feels, which is the sum of them.
func GateWarnings(c Spec) []string {
	if len(c.SLO) == 0 {
		return nil
	}
	declared := map[SLOScope]bool{}
	for _, rule := range c.SLO {
		declared[rule.Scope] = true
	}

	var warnings []string
	if len(c.Steps) > 1 && !declared[ScopeJourney] {
		warnings = append(warnings, fmt.Sprintf(
			"Atencao: o gate mede %d passos isolados e deixa de fora a jornada inteira, que e o tempo que o usuario espera.\n"+
				"    declare tambem:  - jornada: { p95: < 2s, p99: < 5s }", len(c.Steps)))
	}
	if declared[ScopeRegression] {
		warnings = append(warnings, "Atencao: ha regra de regressao declarada; ela so e verificada com 'braunrate execute ... -baseline=execucao-anterior.json'.")
	}
	return warnings
}
