package scenario

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"gopkg.in/yaml.v3"
)

type ScenarioError struct {
	File    string
	Line    int
	Column  int
	Message string
}

func (scenarioError ScenarioError) Error() string {
	if scenarioError.File == "" {
		return fmt.Sprintf("linha %d: %s", scenarioError.Line, scenarioError.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s", scenarioError.File, scenarioError.Line, scenarioError.Column, scenarioError.Message)
}

func nodeError(node *yaml.Node, format string, args ...any) error {
	line, column := 0, 0
	if node != nil {
		line, column = node.Line, node.Column
	}
	return ScenarioError{Line: line, Column: column, Message: fmt.Sprintf(format, args...)}
}

// Listed here because the published schema is tested against them: a key that
// exists on only one side becomes autocomplete the parser refuses.
var (
	TopKeys  = []string{"nome", "alvo", "requer", "variaveis", "autenticacao", "dados", "carga", "cenario", "slo"}
	StepKeys = []string{"nome", "captura", "verificar", "espera"}
)

func ParseFile(path string) (Spec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
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
		return Spec{}, err
	}
	if len(root.Content) == 0 {
		return Spec{}, ScenarioError{Line: 1, Message: "cenario vazio"}
	}
	document := root.Content[0]
	if document.Kind != yaml.MappingNode {
		return Spec{}, nodeError(document, "o cenario precisa ser um mapa de chaves, comecando por:\n"+
			"  nome: Consulta de pedidos\n"+
			"  alvo: http://127.0.0.1:8080")
	}

	spec := Spec{
		FormatVersion: FormatVersion,
		Vars:          map[string]string{},
		Load:          LoadPlan{Model: OpenArrival},
	}

	for index := 0; index+1 < len(document.Content); index += 2 {
		key := document.Content[index]
		value := document.Content[index+1]
		switch key.Value {
		case "nome":
			spec.Name = value.Value
		case "alvo":
			spec.Target = value.Value
		case "variaveis":
			vars, err := readVars(value)
			if err != nil {
				return spec, err
			}
			spec.Vars = vars
		case "carga":
			load, err := readLoad(value)
			if err != nil {
				return spec, err
			}
			spec.Load = load
		case "cenario":
			steps, err := readSteps(value)
			if err != nil {
				return spec, err
			}
			spec.Steps = steps
		case "autenticacao":
			auth, err := readAuth(value)
			if err != nil {
				return spec, err
			}
			spec.Auth = auth
		case "dados":
			sources, err := readData(value)
			if err != nil {
				return spec, err
			}
			spec.Data = sources
		case "requer":
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
			return spec, nodeError(key, "chave desconhecida no topo do cenario: %q\n%s",
				key.Value, sugerir(key.Value, TopKeys))
		}
	}

	spec.Target = Interpolate(spec.Target, spec.Vars)
	return spec, nil
}

func readVars(node *yaml.Node) (map[string]string, error) {
	vars := map[string]string{}
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "variaveis precisa ser um mapa, por exemplo:\n"+
			"  variaveis:\n"+
			"    usuario: ${USUARIO:-ana}")
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		name := node.Content[index].Value
		vars[name] = ExpandFromEnv(node.Content[index+1].Value)
	}
	return vars, nil
}

var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.]*)(?::-([^}]*))?\}`)

func ExpandFromEnv(text string) string {
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		if value, definida := os.LookupEnv(parts[1]); definida {
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
		if value, definida := os.LookupEnv(parts[1]); definida {
			return value
		}
		return parts[2]
	})
}

const closedExample = "  carga:\n" +
	"    modelo: fechado\n" +
	"    usuarios: 200\n" +
	"    duracao: 5m\n" +
	"    intervalo_entre_iteracoes: 1s"

var loadKeys = []string{"modelo", "perfis", "usuarios", "duracao", "intervalo_entre_iteracoes"}

func readLoad(node *yaml.Node) (LoadPlan, error) {
	plan := LoadPlan{Model: OpenArrival}
	if node.Kind != yaml.MappingNode {
		return plan, nodeError(node, "carga precisa ser um mapa, por exemplo:\n"+
			"  carga:\n"+
			"    perfis:\n"+
			"      - patamar: { taxa: 300/s, durante: 1m }")
	}

	// Collected before being judged: the model may be declared after the key
	// that only makes sense under it, and reading in order blames the wrong line.
	nodes := map[string]*yaml.Node{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		if !slices.Contains(loadKeys, key.Value) {
			return plan, nodeError(key, "chave desconhecida em carga: %q\n%s", key.Value, sugerir(key.Value, loadKeys))
		}
		nodes[key.Value] = value
	}

	if model, declared := nodes["modelo"]; declared {
		switch model.Value {
		case string(OpenArrival):
			plan.Model = OpenArrival
		case string(ClosedArrival):
			plan.Model = ClosedArrival
		default:
			return plan, nodeError(model, "modelo de carga desconhecido: %q (os modelos sao 'aberto', que e o padrao, e 'fechado')", model.Value)
		}
	}

	if plan.Model == ClosedArrival {
		return readClosedLoad(plan, nodes)
	}
	return readOpenLoad(plan, nodes)
}

func readOpenLoad(plan LoadPlan, nodes map[string]*yaml.Node) (LoadPlan, error) {
	for _, key := range []string{"usuarios", "duracao", "intervalo_entre_iteracoes"} {
		if node, declared := nodes[key]; declared {
			return plan, nodeError(node, "%q so existe no modelo fechado; no modelo aberto a carga se declara por taxa de chegada:\n"+
				"  carga:\n"+
				"    perfis:\n"+
				"      - patamar: { taxa: 300/s, durante: 5m }", key)
		}
	}

	profiles, declared := nodes["perfis"]
	if !declared {
		return plan, nil
	}
	if profiles.Kind != yaml.SequenceNode {
		return plan, nodeError(profiles, "perfis precisa ser uma lista, um trecho por linha:\n"+
			"  perfis:\n"+
			"    - rampa: { de: 50/s, ate: 300/s, durante: 30s }\n"+
			"    - patamar: { taxa: 300/s, durante: 5m }")
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
	if profiles, declared := nodes["perfis"]; declared {
		return plan, nodeError(profiles, "modelo fechado nao usa 'perfis': no laco fechado a taxa e consequencia do tempo de resposta do alvo, nao uma declaracao sua. Declare quantos usuarios e por quanto tempo:\n"+closedExample)
	}

	users, declared := nodes["usuarios"]
	if !declared {
		return plan, nodeError(nodes["modelo"], "modelo fechado precisa de 'usuarios': e o numero de lacos simultaneos, cada um esperando a resposta antes da proxima iteracao\n"+closedExample)
	}
	count, err := strconv.Atoi(users.Value)
	if err != nil || count <= 0 {
		return plan, nodeError(users, "usuarios precisa ser um inteiro maior que zero, recebeu %q", users.Value)
	}
	plan.Users = count

	span, declared := nodes["duracao"]
	if !declared {
		return plan, nodeError(nodes["modelo"], "modelo fechado precisa de 'duracao': sem taxa declarada, e ela que diz quando a execucao termina\n"+closedExample)
	}
	plan.For, err = readDuration(span)
	if err != nil {
		return plan, err
	}
	if plan.For <= 0 {
		return plan, nodeError(span, "duracao precisa ser maior que zero, recebeu %q", span.Value)
	}

	if think, declared := nodes["intervalo_entre_iteracoes"]; declared {
		plan.ThinkTime, err = readDuration(think)
		if err != nil {
			return plan, err
		}
		if plan.ThinkTime < 0 {
			return plan, nodeError(think, "intervalo_entre_iteracoes nao pode ser negativo, recebeu %q", think.Value)
		}
	}
	return plan, nil
}

func readPhase(node *yaml.Node) (Phase, error) {
	if node.Kind != yaml.MappingNode || len(node.Content) < 2 {
		return Phase{}, nodeError(node, "cada perfil precisa ser um mapa com um tipo (rampa, patamar, pico, constante), por exemplo:\n"+
			"  - patamar: { taxa: 300/s, durante: 5m }")
	}
	kindNode := node.Content[0]
	body := node.Content[1]
	phase := Phase{Line: node.Line}

	switch kindNode.Value {
	case "rampa":
		phase.Kind = PhaseRamp
	case "patamar":
		phase.Kind = PhasePlateau
	case "pico":
		phase.Kind = PhaseSpike
	case "constante":
		phase.Kind = PhaseConstant
	default:
		return phase, nodeError(kindNode, "tipo de perfil desconhecido: %q\n%s\nexemplo: - patamar: { taxa: 300/s, durante: 5m }",
			kindNode.Value, sugerir(kindNode.Value, []string{"rampa", "patamar", "pico", "constante"}))
	}

	if body.Kind != yaml.MappingNode {
		return phase, nodeError(body, "o perfil %q precisa de um mapa de parametros, por exemplo: %s: { taxa: 300/s, durante: 5m }", kindNode.Value, kindNode.Value)
	}
	for index := 0; index+1 < len(body.Content); index += 2 {
		key := body.Content[index]
		value := body.Content[index+1]
		switch key.Value {
		case "de":
			rate, err := readRate(value)
			if err != nil {
				return phase, err
			}
			phase.From = rate
		case "ate", "taxa":
			rate, err := readRate(value)
			if err != nil {
				return phase, err
			}
			phase.To = rate
		case "durante":
			duration, err := readDuration(value)
			if err != nil {
				return phase, err
			}
			phase.For = duration
		default:
			return phase, nodeError(key, "chave desconhecida no perfil %q: %q\n%s", kindNode.Value, key.Value,
				sugerir(key.Value, []string{"de", "ate", "taxa", "durante"}))
		}
	}
	if phase.Kind == PhaseRamp && phase.From == 0 && phase.To == 0 {
		return phase, nodeError(body, "rampa precisa de 'de' e 'ate', por exemplo: - rampa: { de: 50/s, ate: 300/s, durante: 30s }")
	}
	return phase, nil
}

func readDuration(node *yaml.Node) (time.Duration, error) {
	duration, err := time.ParseDuration(node.Value)
	if err != nil {
		return 0, nodeError(node, "duracao invalida: %q (use 30s, 5m, 1h30m)", node.Value)
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
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, nodeError(node, "taxa invalida: %q (use por exemplo 50/s)", node.Value)
	}
	if value <= 0 {
		return 0, nodeError(node, "taxa precisa ser maior que zero")
	}
	return value / divisor, nil
}

func readSteps(node *yaml.Node) ([]Step, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, nodeError(node, "cenario precisa ser uma lista de passos, um por linha:\n"+
			"  cenario:\n"+
			"    - http: GET /pedidos/1")
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
		return step, nodeError(node, "cada passo precisa ser um mapa, por exemplo:\n"+
			"  - http: GET /pedidos/1\n"+
			"    nome: consultar pedido")
	}
	var configNode *yaml.Node
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "nome":
			step.Name = value.Value
		case "verificar", "espera":
			checks, assertions, err := readAssertions(value)
			if err != nil {
				return step, err
			}
			step.Checks = checks
			step.Assertions = assertions
		case "captura":
			captures, err := readCaptures(value)
			if err != nil {
				return step, err
			}
			step.Captures = captures
		case "peso":
			return step, nodeError(key, "a chave %q ainda nao existe: mix ponderado de operacoes entra junto com o GraphQL", key.Value)
		default:
			if _, exists := protocol.Lookup(key.Value); !exists {
				return step, nodeError(key, "nao reconheco %q como tipo de passo\n%s",
					key.Value, sugerir(key.Value, append(protocol.Registered(), StepKeys...)))
			}
			if step.Protocol != "" {
				return step, nodeError(key, "o passo declara mais de um protocolo: %q e %q", step.Protocol, key.Value)
			}
			step.Protocol = key.Value
			configNode = value
		}
	}
	if step.Protocol == "" {
		return step, nodeError(node, "passo sem protocolo (compilados: %s), por exemplo:\n"+
			"  - http: GET /pedidos/1", strings.Join(protocol.Registered(), ", "))
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

func sugerir(received string, valid []string) string {
	best, shortestDistance := "", 1<<30
	for _, valid := range valid {
		distance := editDistance(strings.ToLower(received), strings.ToLower(valid))
		if distance < shortestDistance {
			best, shortestDistance = valid, distance
		}
	}
	lines := ""
	if best != "" && shortestDistance <= 3 {
		lines += fmt.Sprintf("    voce quis dizer %q?\n", best)
	}
	return lines + "    disponiveis: " + strings.Join(valid, ", ")
}

func editDistance(first, second string) int {
	previous := make([]int, len(second)+1)
	current := make([]int, len(second)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(first); i++ {
		current[0] = i
		for j := 1; j <= len(second); j++ {
			cost := 1
			if first[i-1] == second[j-1] {
				cost = 0
			}
			current[j] = min(min(current[j-1]+1, previous[j]+1), previous[j-1]+cost)
		}
		copy(previous, current)
	}
	return previous[len(second)]
}
