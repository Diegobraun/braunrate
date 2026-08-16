package scenario

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/Diegobraun/braunrate/internal/protocol"
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
		problems = append(problems, "o cenário precisa de um nome")
	}
	if missing := UnresolvedEnvironment(c.Target); len(missing) > 0 {
		problems = append(problems, fmt.Sprintf(
			"o alvo depende da variável de ambiente %s, que não está definida.\n"+
				"    rode com %s=... , ou declare um padrão no arquivo: alvo: ${%s:-http://127.0.0.1:8080}",
			strings.Join(missing, ", "), missing[0], missing[0]))
	} else if strings.TrimSpace(c.Target) == "" {
		problems = append(problems, "o cenário precisa de um alvo")
	} else if !validTarget(c.Target) {
		problems = append(problems, fmt.Sprintf("alvo inválido: %q (use https://api.exemplo.com, kafka://127.0.0.1:9092 ou amqp://usuario:senha@127.0.0.1:5672/)", c.Target))
	}
	if len(c.Steps) == 0 {
		problems = append(problems, "o cenário precisa de pelo menos um passo")
	}
	switch {
	case c.Load.Closed() && c.Load.Users <= 0:
		problems = append(problems, "o modelo fechado precisa de pelo menos um usuário")
	case c.Load.Closed() && c.Load.For <= 0:
		problems = append(problems, "o modelo fechado precisa de duração")
	case !c.Load.Closed() && len(c.Load.Phases) == 0:
		problems = append(problems, "o cenário precisa de pelo menos um perfil de carga")
	}

	seen := map[string]int{}
	for _, step := range c.Steps {
		seen[step.Name]++
		if seen[step.Name] == 2 {
			problems = append(problems, fmt.Sprintf("passo com nome repetido: %q (o relatório agrega por nome)", step.Name))
		}
	}

	problems = append(problems, checkMix(&c)...)
	problems = append(problems, checkBrokers(&c)...)
	problems = append(problems, checkSLOSteps(&c)...)

	for _, phase := range c.Load.Phases {
		if phase.For <= 0 {
			problems = append(problems, fmt.Sprintf("linha %d: perfil %s sem duração", phase.Line, phase.Kind))
		}
		if phase.Kind != PhaseRamp && phase.To <= 0 {
			problems = append(problems, fmt.Sprintf("linha %d: perfil %s sem taxa", phase.Line, phase.Kind))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("cenário inválido:\n  - %s", strings.Join(problems, "\n  - "))
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

	line := fmt.Sprintf("Atenção: 'modelo: fechado' não declara carga, declara %d laços.\n", spec.Load.Users)
	line += "    Cada usuário só pede de novo depois da resposta anterior: se o alvo travar, eles param de pedir\n"
	line += "    junto e o atraso não aparece na medição — é o oposto do modelo aberto, que insiste na taxa.\n"
	line += fmt.Sprintf("    Taxa aproximada com esses %d usuários: %.0f/s se o alvo responder em 100 ms, %.0f/s em 500 ms, %.0f/s em 2s.",
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
			"Atenção: o gate mede %d passos isolados e deixa de fora a jornada inteira, que é o tempo que o usuário espera.\n"+
				"    declare também:  - jornada: { p95: < 2s, p99: < 5s }", len(spec.Steps)))
	}
	if declared[ScopeRegression] {
		warnings = append(warnings, "Atenção: há regra de regressão declarada; ela só é verificada com 'braunrate execute ... -baseline=execucao-anterior.json'.")
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
			"    declara a infraestrutura externa sem a qual este cenário não roda")
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

// A7 of the audit: a literal path never goes through interpolation, so it never
// enters the observed-variety check. The run hits the same URL thousands of
// times, the target answers from cache, and nothing in the report says so —
// exactly the blind spot ADR 0007 closes for data that does vary.
func FixedStepWarnings(spec Spec) []string {
	var warnings []string
	for _, step := range spec.Steps {
		if step.Config == nil || varies(step.Config) {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"Atenção: o passo %q não tem nenhum valor que varia — toda requisição vai ser idêntica.\n"+
				"    se o alvo guardar resposta por essa chave, o número sai otimista.\n"+
				"    para variar:  dados: { pedidos: { arquivo: pedidos.csv } }  e então  %s",
			step.Name, exampleWithVariable(step)))
	}
	return warnings
}

func varies(config protocol.Config) bool {
	describable, describes := config.(interface{ Describe() []string })
	if !describes {
		return strings.Contains(config.AggregationKey(), "${")
	}
	for _, line := range describable.Describe() {
		if strings.Contains(line, "${") {
			return true
		}
	}
	return false
}

func exampleWithVariable(step Step) string {
	key := step.AggregationKey()
	if at := strings.LastIndex(key, "/"); at >= 0 && at+1 < len(key) {
		return key[:at+1] + "${pedidos.id}"
	}
	return key + " com ${pedidos.id}"
}
