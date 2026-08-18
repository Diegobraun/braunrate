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
		problems = append(problems, "the scenario needs a name")
	}
	if missing := UnresolvedEnvironment(c.Target); len(missing) > 0 {
		problems = append(problems, fmt.Sprintf(
			"the target depends on the environment variable %s, which is not set.\n"+
				"    run with %s=... , or declare a default in the file: target: ${%s:-http://127.0.0.1:8080}",
			strings.Join(missing, ", "), missing[0], missing[0]))
	} else if strings.TrimSpace(c.Target) == "" {
		problems = append(problems, "the scenario needs a target")
	} else if !validTarget(c.Target) {
		problems = append(problems, fmt.Sprintf("invalid target: %q (use https://api.example.com, kafka://127.0.0.1:9092 or amqp://user:password@127.0.0.1:5672/)", c.Target))
	}
	if len(c.Steps) == 0 {
		problems = append(problems, "the scenario needs at least one step")
	}
	switch {
	case c.Load.Closed() && c.Load.Users <= 0:
		problems = append(problems, "the closed model needs at least one user")
	case c.Load.Closed() && c.Load.For <= 0:
		problems = append(problems, "the closed model needs a duration")
	case !c.Load.Closed() && len(c.Load.Phases) == 0:
		problems = append(problems, "the scenario needs at least one load profile")
	}

	seen := map[string]int{}
	for _, step := range c.Steps {
		seen[step.Name]++
		if seen[step.Name] == 2 {
			problems = append(problems, fmt.Sprintf("step with a repeated name: %q (the report aggregates by name)", step.Name))
		}
	}

	problems = append(problems, checkMix(&c)...)
	problems = append(problems, checkBrokers(&c)...)
	problems = append(problems, checkSLOSteps(&c)...)

	for _, phase := range c.Load.Phases {
		if phase.For <= 0 {
			problems = append(problems, fmt.Sprintf("line %d: profile %s with no duration", phase.Line, phase.Kind))
		}
		if phase.Kind != PhaseRamp && phase.To <= 0 {
			problems = append(problems, fmt.Sprintf("line %d: profile %s with no rate", phase.Line, phase.Kind))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid scenario:\n  - %s", strings.Join(problems, "\n  - "))
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

	line := fmt.Sprintf("Warning: 'model: closed' does not declare load, it declares %d loops.\n", spec.Load.Users)
	line += "    Each user only asks again after the previous response: if the target freezes, they stop asking\n"
	line += "    along with it and the delay never shows up in the measurement — the opposite of the open model, which insists on the rate.\n"
	line += fmt.Sprintf("    Approximate rate with those %d users: %.0f/s if the target answers in 100 ms, %.0f/s at 500 ms, %.0f/s at 2s.",
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
			"Warning: the gate measures %d isolated steps and leaves out the whole journey, which is the wait the user feels.\n"+
				"    declare it too:  - journey: { p95: < 2s, p99: < 5s }", len(spec.Steps)))
	}
	if declared[ScopeRegression] {
		warnings = append(warnings, "Warning: there is a regression rule declared; it is only checked with 'braunrate execute ... -baseline=previous-run.json'.")
	}
	return warnings
}

// KnownRequirements is the closed list on purpose: an unknown name would be
// declared, printed and never checked by anyone, which is worse than not
// declaring it.
var KnownRequirements = []string{"kafka", "amqp", "mqtt", "credential"}

func readRequirements(no *yaml.Node) ([]string, error) {
	if no.Kind != yaml.SequenceNode {
		return nil, nodeError(no, "requires has to be a list, for example: requires: [kafka]\n"+
			"    it declares the external infrastructure without which this scenario does not run")
	}
	requirements := make([]string, 0, len(no.Content))
	for _, item := range no.Content {
		if !slices.Contains(KnownRequirements, item.Value) {
			return nil, nodeError(item, "unknown dependency: %q\n%s",
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
			"Warning: the step %q has no value that varies — every request will be identical.\n"+
				"    if the target caches by that key, the number comes out optimistic.\n"+
				"    to make it vary:  data: { orders: { file: orders.csv } }  and then  %s",
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
		return key[:at+1] + "${orders.id}"
	}
	return key + " with ${orders.id}"
}
