package scenario

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/texto"
	"gopkg.in/yaml.v3"
)

type ScenarioError struct {
	File    string
	Line    int
	Column  int
	Message string
}

func (scenarioError ScenarioError) Error() string {
	// A scenario written in Go has no line: printing "line 0" would send the
	// reader looking for a file that does not exist.
	if scenarioError.Line == 0 {
		return scenarioError.Message
	}
	if scenarioError.File == "" {
		return fmt.Sprintf("line %d: %s", scenarioError.Line, scenarioError.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s", scenarioError.File, scenarioError.Line, scenarioError.Column, scenarioError.Message)
}

func nodeError(node *yaml.Node, format string, args ...any) error {
	line, column := 0, 0
	if node != nil {
		line, column = node.Line, node.Column
	}
	return ScenarioError{
		Line:    line,
		Column:  column,
		Message: fmt.Sprintf(format, args...) + missingEnvironmentHint(node),
	}
}

// Listed here because the published schema is tested against them: a key that
// exists on only one side becomes autocomplete the parser refuses.
var (
	TopKeys  = []string{"name", "target", "requires", "variables", "auth", "tls", "messaging", "data", "load", "scenario", "slo"}
	StepKeys = []string{"name", "weight", "capture", "expect"}
)

func ParseFile(path string) (Spec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, readError(path, err)
	}
	c, err := Parse(content)
	if err, ok := err.(ScenarioError); ok {
		err.File = path
		return c, err
	}
	return c, err
}

func Parse(content []byte) (Spec, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return Spec{}, translateYAMLError(content, err)
	}
	if len(root.Content) == 0 {
		return Spec{}, ScenarioError{Line: 1, Message: "empty scenario"}
	}
	document := root.Content[0]
	if document.Kind != yaml.MappingNode {
		return Spec{}, nodeError(document, "a scenario has to be a map of keys, starting with:\n"+
			"  name: Order lookup\n"+
			"  target: http://127.0.0.1:8080")
	}

	// Antes de ler qualquer campo: ${VARIAVEL} do ambiente vale em todo campo
	// escalar, e nao em uma lista de campos escolhidos a dedo.
	expandEnvironment(document)

	spec := Spec{
		FormatVersion: FormatVersion,
		Vars:          map[string]string{},
		Load:          LoadPlan{Model: OpenArrival},
	}

	for index := 0; index+1 < len(document.Content); index += 2 {
		key := document.Content[index]
		value := document.Content[index+1]
		switch key.Value {
		case "name":
			spec.Name = value.Value
		case "target":
			spec.Target = value.Value
		case "variables":
			vars, err := readVars(value)
			if err != nil {
				return spec, err
			}
			spec.Vars = vars
		case "load":
			load, err := readLoad(value)
			if err != nil {
				return spec, err
			}
			spec.Load = load
		case "scenario":
			steps, err := readSteps(value)
			if err != nil {
				return spec, err
			}
			spec.Steps = steps
		case "auth":
			auth, err := readAuth(value)
			if err != nil {
				return spec, err
			}
			spec.Auth = auth
		case "data":
			sources, err := readData(value)
			if err != nil {
				return spec, err
			}
			spec.Data = sources
		case "tls":
			settings, err := readBrokerTLS(value)
			if err != nil {
				return spec, err
			}
			spec.TLS = &settings
		case "messaging":
			settings, err := readMessaging(value)
			if err != nil {
				return spec, err
			}
			spec.Messaging = settings
		case "requires":
			requirements, err := readRequirements(value)
			if err != nil {
				return spec, err
			}
			spec.Requires = requirements
		case "slo":
			rules, err := readSLO(value)
			if err != nil {
				return spec, err
			}
			spec.SLO = rules
		default:
			if replacement, isOld := RenamedTopKey(key.Value); isOld {
				return spec, nodeError(key, "%s", outdatedFormat(key.Value, replacement))
			}
			return spec, nodeError(key, "unknown key at the top of the scenario: %q\n%s",
				key.Value, suggestWithExample(key.Value, TopKeys, "    a minimal scenario has four of them:\n"+
					"      name: Order lookup\n"+
					"      target: http://127.0.0.1:8080\n"+
					"      load: { profiles: [ { steady: { rate: 100/s, duration: 1m } } ] }\n"+
					"      scenario:\n"+
					"        - http: GET /orders/1"))
		}
	}

	spec.Target = interpolateKnown(spec.Target, spec.Vars)
	if err := checkReferences(document, &spec); err != nil {
		return spec, err
	}
	return spec, nil
}

func readVars(node *yaml.Node) (map[string]string, error) {
	vars := map[string]string{}
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "variables has to be a map, for example:\n"+
			"  variables:\n"+
			"    user: ${USER:-ana}")
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		name := node.Content[index].Value
		if err := refuseLiteralVariable(name, node.Content[index+1]); err != nil {
			return nil, err
		}
		vars[name] = ExpandFromEnv(node.Content[index+1].Value)
	}
	return vars, nil
}

// The raw error from the operating system says nothing about what to do next,
// in a product where every other message does.
func readError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ScenarioError{File: path, Message: fmt.Sprintf("I could not find the file %s.\n"+
			"    to start a scenario from scratch:  braunrate new %s\n"+
			"    to see the ones nearby:  ls *.yaml", path, path)}
	case errors.Is(err, fs.ErrPermission):
		return ScenarioError{File: path, Message: fmt.Sprintf("I am not allowed to read %s", path)}
	}
	return ScenarioError{File: path, Message: fmt.Sprintf("I could not read %s: %v", path, err)}
}

var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.]*)(?::-([^}]*))?\}`)

func ExpandFromEnv(text string) string {
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		if value, defined := os.LookupEnv(parts[1]); defined {
			return value
		}
		return parts[2]
	})
}

func Interpolate(text string, vars map[string]string) string {
	if text == "" {
		return text
	}
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		if value, exists := vars[parts[1]]; exists {
			return value
		}
		if value, defined := os.LookupEnv(parts[1]); defined {
			return value
		}
		return parts[2]
	})
}

const closedExample = "  load:\n" +
	"    model: closed\n" +
	"    users: 200\n" +
	"    duration: 5m\n" +
	"    thinkTime: 1s"

var loadKeys = []string{"model", "profiles", "users", "duration", "thinkTime"}

func readLoad(node *yaml.Node) (LoadPlan, error) {
	plan := LoadPlan{Model: OpenArrival}
	if node.Kind != yaml.MappingNode {
		return plan, nodeError(node, "load has to be a map, for example:\n"+
			"  load:\n"+
			"    profiles:\n"+
			"      - steady: { rate: 300/s, duration: 1m }")
	}

	// Collected before being judged: the model may be declared after the key
	// that only makes sense under it, and reading in order blames the wrong line.
	nodes := map[string]*yaml.Node{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		if !slices.Contains(loadKeys, key.Value) {
			return plan, nodeError(key, "unknown key in load: %q\n%s", key.Value,
				suggestWithExample(key.Value, loadKeys, "    example:\n"+
					"      load:\n"+
					"        profiles:\n"+
					"          - ramp: { from: 10/s, to: 200/s, duration: 30s }\n"+
					"          - steady: { rate: 200/s, duration: 5m }"))
		}
		nodes[key.Value] = value
	}

	if model, declared := nodes["model"]; declared {
		switch model.Value {
		case string(OpenArrival):
			plan.Model = OpenArrival
		case string(ClosedArrival):
			plan.Model = ClosedArrival
		default:
			return plan, nodeError(model, "unknown load model: %q (the models are 'open', which is the default, and 'closed')", model.Value)
		}
	}

	if plan.Model == ClosedArrival {
		return readClosedLoad(plan, nodes)
	}
	return readOpenLoad(plan, nodes)
}

func readOpenLoad(plan LoadPlan, nodes map[string]*yaml.Node) (LoadPlan, error) {
	for _, key := range []string{"users", "duration", "thinkTime"} {
		if node, declared := nodes[key]; declared {
			return plan, nodeError(node, "%q only exists in the closed model; in the open model the load is declared as an arrival rate:\n"+
				"  load:\n"+
				"    profiles:\n"+
				"      - steady: { rate: 300/s, duration: 5m }", key)
		}
	}

	profiles, declared := nodes["profiles"]
	if !declared {
		return plan, nil
	}
	if profiles.Kind != yaml.SequenceNode {
		return plan, nodeError(profiles, "profiles has to be a list, one stretch per line:\n"+
			"  profiles:\n"+
			"    - ramp: { from: 50/s, to: 300/s, duration: 30s }\n"+
			"    - steady: { rate: 300/s, duration: 5m }")
	}
	for _, item := range profiles.Content {
		phase, err := readPhase(item)
		if err != nil {
			return plan, err
		}
		plan.Phases = append(plan.Phases, phase)
	}
	return plan, nil
}

func readClosedLoad(plan LoadPlan, nodes map[string]*yaml.Node) (LoadPlan, error) {
	if profiles, declared := nodes["profiles"]; declared {
		return plan, nodeError(profiles, "the closed model does not use 'profiles': in a closed loop the rate is a consequence of how fast the target answers, not something you declare. Say how many users and for how long:\n"+closedExample)
	}

	users, declared := nodes["users"]
	if !declared {
		return plan, nodeError(nodes["model"], "the closed model needs 'users': the number of simultaneous loops, each waiting for the response before the next iteration\n"+closedExample)
	}
	count, err := strconv.Atoi(users.Value)
	if err != nil || count <= 0 {
		return plan, nodeError(users, "users has to be an integer greater than zero, got %q", users.Value)
	}
	plan.Users = count

	span, declared := nodes["duration"]
	if !declared {
		return plan, nodeError(nodes["model"], "the closed model needs 'duration': with no declared rate, it is what says when the run ends\n"+closedExample)
	}
	plan.For, err = readDuration(span)
	if err != nil {
		return plan, err
	}
	if plan.For <= 0 {
		return plan, nodeError(span, "duration has to be greater than zero, got %q", span.Value)
	}

	if think, declared := nodes["thinkTime"]; declared {
		plan.ThinkTime, err = readDuration(think)
		if err != nil {
			return plan, err
		}
		if plan.ThinkTime < 0 {
			return plan, nodeError(think, "thinkTime cannot be negative, got %q", think.Value)
		}
	}
	return plan, nil
}

func readPhase(node *yaml.Node) (Phase, error) {
	if node.Kind != yaml.MappingNode || len(node.Content) < 2 {
		return Phase{}, nodeError(node, "every profile has to be a map with a kind (ramp, steady, spike), for example:\n"+
			"  - steady: { rate: 300/s, duration: 5m }")
	}
	kindNode := node.Content[0]
	body := node.Content[1]
	phase := Phase{Line: node.Line}

	switch kindNode.Value {
	case "ramp":
		phase.Kind = PhaseRamp
	case "steady":
		phase.Kind = PhasePlateau
	case "spike":
		phase.Kind = PhaseSpike
	default:
		return phase, nodeError(kindNode, "unknown profile kind: %q\n%s\nexample: - steady: { rate: 300/s, duration: 5m }",
			kindNode.Value, suggest(kindNode.Value, []string{"ramp", "steady", "spike"}))
	}

	if body.Kind != yaml.MappingNode {
		return phase, nodeError(body, "the profile %q needs a map of parameters, for example: %s: { rate: 300/s, duration: 5m }", kindNode.Value, kindNode.Value)
	}
	for index := 0; index+1 < len(body.Content); index += 2 {
		key := body.Content[index]
		value := body.Content[index+1]
		switch key.Value {
		case "from":
			rate, err := readRate(value)
			if err != nil {
				return phase, err
			}
			phase.From = rate
		case "to", "rate":
			rate, err := readRate(value)
			if err != nil {
				return phase, err
			}
			phase.To = rate
		case "duration":
			duration, err := readDuration(value)
			if err != nil {
				return phase, err
			}
			phase.For = duration
		default:
			return phase, nodeError(key, "unknown key in the %q profile: %q\n%s", kindNode.Value, key.Value,
				suggestWithExample(key.Value, []string{"from", "to", "rate", "duration"},
					"    ramp uses from/to/duration; steady and spike use rate/duration:\n"+
						"      - ramp: { from: 10/s, to: 200/s, duration: 30s }\n"+
						"      - steady: { rate: 200/s, duration: 5m }"))
		}
	}
	if phase.Kind == PhaseRamp && phase.From == 0 && phase.To == 0 {
		return phase, nodeError(body, "ramp needs 'from' and 'to', for example: - ramp: { from: 50/s, to: 300/s, duration: 30s }")
	}
	return phase, nil
}

func readDuration(node *yaml.Node) (time.Duration, error) {
	duration, err := time.ParseDuration(node.Value)
	if err != nil {
		return 0, nodeError(node, "invalid duration: %q (use 30s, 5m, 1h30m)", node.Value)
	}
	return duration, nil
}

func readRate(node *yaml.Node) (float64, error) {
	text := strings.TrimSpace(node.Value)
	divisor := 1.0
	switch {
	case strings.HasSuffix(text, "/s"):
		text = strings.TrimSuffix(text, "/s")
	case strings.HasSuffix(text, "/m"):
		text = strings.TrimSuffix(text, "/m")
		divisor = 60
	case strings.HasSuffix(text, "/h"):
		text = strings.TrimSuffix(text, "/h")
		divisor = 3600
	default:
		// A bare number was read as per second: whoever meant per minute got
		// sixty times the load and no warning.
		if _, err := strconv.ParseFloat(text, 64); err == nil {
			return 0, nodeError(node, "rate without a unit: %q\n"+
				"    say over which interval: %s/s (per second), %s/m (per minute) or %s/h (per hour)", text, text, text, text)
		}
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, nodeError(node, "invalid rate: %q (use for example 50/s)", node.Value)
	}
	if value <= 0 {
		return 0, nodeError(node, "rate has to be greater than zero")
	}
	return value / divisor, nil
}

func readSteps(node *yaml.Node) ([]Step, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, nodeError(node, "scenario has to be a list of steps, one per line:\n"+
			"  scenario:\n"+
			"    - http: GET /orders/1")
	}
	steps := make([]Step, 0, len(node.Content))
	for _, itemNode := range node.Content {
		step, err := readStep(itemNode)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func readStep(node *yaml.Node) (Step, error) {
	step := Step{Line: node.Line}
	if node.Kind != yaml.MappingNode {
		return step, nodeError(node, "every step has to be a map, for example:\n"+
			"  - http: GET /orders/1\n"+
			"    name: look up order")
	}
	var configNode *yaml.Node
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "name":
			step.Name = value.Value
		case "expect":
			checks, assertions, err := readAssertions(value)
			if err != nil {
				return step, err
			}
			step.Checks = checks
			step.Assertions = assertions
		case "capture":
			captures, err := readCaptures(value)
			if err != nil {
				return step, err
			}
			step.Captures = captures
		case "weight":
			weight, err := strconv.Atoi(strings.TrimSpace(value.Value))
			if err != nil || weight <= 0 {
				return step, nodeError(value, "weight has to be an integer greater than zero, for example: weight: 60")
			}
			step.Weight = weight
		default:
			if _, exists := protocol.Lookup(key.Value); !exists {
				if replacement, isOld := RenamedStepKey(key.Value); isOld {
					return step, nodeError(key, "%s", outdatedFormat(key.Value, replacement))
				}
				return step, nodeError(key, "I do not recognize %q as a kind of step\n%s",
					key.Value, suggestWithExample(key.Value, append(protocol.Registered(), StepKeys...),
						"    a step is a map with the protocol and what it carries:\n"+
							"      - http: GET /orders/1\n"+
							"        name: look up order\n"+
							"        expect: { status: 200 }\n"+
							"        capture: { invoiceId: \"$.lastInvoice.id\" }"))
			}
			if step.Protocol != "" {
				return step, nodeError(key, "the step declares more than one protocol: %q and %q", step.Protocol, key.Value)
			}
			step.Protocol = key.Value
			configNode = value
		}
	}
	if step.Protocol == "" {
		return step, nodeError(node, "step with no protocol (compiled in: %s), for example:\n"+
			"  - http: GET /orders/1", strings.Join(protocol.Registered(), ", "))
	}
	implementation, _ := protocol.Lookup(step.Protocol)
	config, err := implementation.Decode(configNode)
	if err != nil {
		if _, alreadyScenarioError := err.(ScenarioError); !alreadyScenarioError {
			return step, nodeError(configNode, "%v", err)
		}
		return step, err
	}
	step.Config = config
	if step.Name == "" {
		step.Name = config.AggregationKey()
	}
	return step, nil
}

// Knowing that "profiles" exists does not teach that "profiles" is a list of
// maps with a profile kind inside. Where the shape is not obvious from the
// name, the message carries the shape.
func suggestWithExample(received string, valid []string, example string) string {
	return suggest(received, valid) + "\n" + example
}

func suggest(received string, valid []string) string {
	lines := ""
	if best, found := texto.Closest(received, valid); found {
		lines += fmt.Sprintf("    did you mean %q?\n", best)
	}
	return lines + "    available: " + strings.Join(valid, ", ")
}
