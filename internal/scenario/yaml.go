package scenario

import (
	"fmt"
	"os"
	"regexp"
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

func (e ScenarioError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("linha %d: %s", e.Line, e.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
}

func nodeError(no *yaml.Node, format string, args ...any) error {
	line, column := 0, 0
	if no != nil {
		line, column = no.Line, no.Column
	}
	return ScenarioError{Line: line, Column: column, Message: fmt.Sprintf(format, args...)}
}

// Listadas aqui porque o schema publicado e testado contra elas: chave que
// existe so num dos dois lados vira autocompletar que o parser recusa.
var (
	TopKeys  = []string{"nome", "alvo", "variaveis", "autenticacao", "dados", "carga", "cenario", "slo"}
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

	c := Spec{
		FormatVersion: FormatVersion,
		Vars:          map[string]string{},
		Load:          LoadPlan{Model: OpenArrival},
	}

	for index := 0; index+1 < len(document.Content); index += 2 {
		key := document.Content[index]
		value := document.Content[index+1]
		switch key.Value {
		case "nome":
			c.Name = value.Value
		case "alvo":
			c.Target = value.Value
		case "variaveis":
			vars, err := readVars(value)
			if err != nil {
				return c, err
			}
			c.Vars = vars
		case "carga":
			load, err := readLoad(value)
			if err != nil {
				return c, err
			}
			c.Load = load
		case "cenario":
			steps, err := readSteps(value)
			if err != nil {
				return c, err
			}
			c.Steps = steps
		case "autenticacao":
			auth, err := readAuth(value)
			if err != nil {
				return c, err
			}
			c.Auth = auth
		case "dados":
			sources, err := readData(value)
			if err != nil {
				return c, err
			}
			c.Data = sources
		case "slo":
			rules, err := readSLO(value)
			if err != nil {
				return c, err
			}
			c.SLO = rules
		default:
			return c, nodeError(key, "chave desconhecida no topo do cenario: %q\n%s",
				key.Value, sugerir(key.Value, TopKeys))
		}
	}

	c.Target = Interpolate(c.Target, c.Vars)
	return c, nil
}

func readVars(no *yaml.Node) (map[string]string, error) {
	vars := map[string]string{}
	if no.Kind != yaml.MappingNode {
		return nil, nodeError(no, "variaveis precisa ser um mapa, por exemplo:\n"+
			"  variaveis:\n"+
			"    usuario: ${USUARIO:-ana}")
	}
	for index := 0; index+1 < len(no.Content); index += 2 {
		name := no.Content[index].Value
		vars[name] = ExpandFromEnv(no.Content[index+1].Value)
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

func readLoad(no *yaml.Node) (LoadPlan, error) {
	plan := LoadPlan{Model: OpenArrival}
	if no.Kind != yaml.MappingNode {
		return plan, nodeError(no, "carga precisa ser um mapa, por exemplo:\n"+
			"  carga:\n"+
			"    perfis:\n"+
			"      - patamar: { taxa: 300/s, durante: 1m }")
	}
	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
		switch key.Value {
		case "modelo":
			switch value.Value {
			case string(OpenArrival):
				plan.Model = OpenArrival
			case string(ClosedArrival):
				return plan, nodeError(value, "modelo fechado ainda nao e suportado; o padrao e aberto")
			default:
				return plan, nodeError(value, "modelo de carga desconhecido: %q (o unico modelo e 'aberto', e ele e o padrao: pode omitir a linha)", value.Value)
			}
		case "perfis":
			if value.Kind != yaml.SequenceNode {
				return plan, nodeError(value, "perfis precisa ser uma lista, um trecho por linha:\n"+
					"  perfis:\n"+
					"    - rampa: { de: 50/s, ate: 300/s, durante: 30s }\n"+
					"    - patamar: { taxa: 300/s, durante: 5m }")
			}
			for _, itemNode := range value.Content {
				phase, err := readPhase(itemNode)
				if err != nil {
					return plan, err
				}
				plan.Phases = append(plan.Phases, phase)
			}
		default:
			return plan, nodeError(key, "chave desconhecida em carga: %q\n%s", key.Value, sugerir(key.Value, []string{"modelo", "perfis"}))
		}
	}
	return plan, nil
}

func readPhase(no *yaml.Node) (Phase, error) {
	if no.Kind != yaml.MappingNode || len(no.Content) < 2 {
		return Phase{}, nodeError(no, "cada perfil precisa ser um mapa com um tipo (rampa, patamar, pico, constante), por exemplo:\n"+
			"  - patamar: { taxa: 300/s, durante: 5m }")
	}
	kindNode := no.Content[0]
	body := no.Content[1]
	phase := Phase{Line: no.Line}

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
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return phase, nodeError(value, "duracao invalida: %q (use 30s, 5m, 1h30m)", value.Value)
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

func readRate(no *yaml.Node) (float64, error) {
	text := strings.TrimSpace(no.Value)
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
		return 0, nodeError(no, "taxa invalida: %q (use por exemplo 50/s)", no.Value)
	}
	if value <= 0 {
		return 0, nodeError(no, "taxa precisa ser maior que zero")
	}
	return value / divisor, nil
}

func readSteps(no *yaml.Node) ([]Step, error) {
	if no.Kind != yaml.SequenceNode {
		return nil, nodeError(no, "cenario precisa ser uma lista de passos, um por linha:\n"+
			"  cenario:\n"+
			"    - http: GET /pedidos/1")
	}
	steps := make([]Step, 0, len(no.Content))
	for _, itemNode := range no.Content {
		step, err := readStep(itemNode)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func readStep(no *yaml.Node) (Step, error) {
	step := Step{Line: no.Line}
	if no.Kind != yaml.MappingNode {
		return step, nodeError(no, "cada passo precisa ser um mapa, por exemplo:\n"+
			"  - http: GET /pedidos/1\n"+
			"    nome: consultar pedido")
	}
	var configNode *yaml.Node
	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
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
		return step, nodeError(no, "passo sem protocolo (compilados: %s), por exemplo:\n"+
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
