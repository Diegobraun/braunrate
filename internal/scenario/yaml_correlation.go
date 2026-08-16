package scenario

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The shape of the expression picks the origin: "$.field" is JSON,
// "cabecalho:X-Id" is a header, "/pattern/" is a regular expression. QA writes
// one line and is done.
func readCaptures(node *yaml.Node) ([]Capture, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "captura precisa ser um mapa, por exemplo: captura: { faturaId: $.fatura.id }")
	}
	captures := make([]Capture, 0, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		name := node.Content[index]
		value := node.Content[index+1]

		if value.Kind == yaml.MappingNode {
			capture, err := readFullCapture(name.Value, value)
			if err != nil {
				return nil, err
			}
			captures = append(captures, capture)
			continue
		}

		capture, err := parseCaptureExpression(name.Value, value)
		if err != nil {
			return nil, err
		}
		captures = append(captures, capture)
	}
	return captures, nil
}

// ParseCapture is node-free because the Go DSL comes through here too: two
// readings of the same expression would mean a capture that works for one
// audience and fails for the other.
func ParseCapture(name, text string) (Capture, error) {
	expression := strings.TrimSpace(text)
	capture := Capture{Variable: name, Expression: expression, Required: true}

	switch {
	case expression == "status":
		capture.Origin = CaptureStatus
	case expression == "corpo":
		capture.Origin = CaptureBody
	case strings.HasPrefix(expression, "$"):
		capture.Origin = CaptureJSON
	case strings.HasPrefix(expression, "cabecalho:"):
		capture.Origin = CaptureHeader
		capture.Expression = strings.TrimSpace(strings.TrimPrefix(expression, "cabecalho:"))
	// Um cookie e um cabecalho com estrutura: "cabecalho:Set-Cookie" devolveria
	// "sessao=abc; Path=/; HttpOnly", e mandar isso de volta em Cookie: manda
	// tres cookies, dois deles inventados.
	case strings.HasPrefix(expression, "cookie:"):
		capture.Origin = CaptureCookie
		capture.Expression = strings.TrimSpace(strings.TrimPrefix(expression, "cookie:"))
	case strings.HasPrefix(expression, "/") && strings.HasSuffix(expression, "/") && len(expression) > 2:
		capture.Origin = CaptureRegex
		capture.Expression = expression[1 : len(expression)-1]
	default:
		return capture, fmt.Errorf("nao entendi de onde capturar %q.\n"+
			"    use uma destas formas:\n"+
			"      %s: $.caminho.no.json      captura de um campo do corpo JSON\n"+
			"      %s: cabecalho:X-Request-Id captura de um cabecalho da resposta\n"+
			"      %s: cookie:sessao         captura o valor de um cookie do Set-Cookie\n"+
			"      %s: /token=([a-z0-9]+)/    captura pelo primeiro grupo da expressao regular",
			name, name, name, name, name)
	}
	return capture, nil
}

func parseCaptureExpression(name string, node *yaml.Node) (Capture, error) {
	capture, err := ParseCapture(name, node.Value)
	if err != nil {
		return capture, nodeError(node, "%v", err)
	}
	capture.Line = node.Line
	return capture, nil
}

func readFullCapture(name string, node *yaml.Node) (Capture, error) {
	capture := Capture{Variable: name, Required: true, Line: node.Line}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "de":
			parsed, err := parseCaptureExpression(name, value)
			if err != nil {
				return capture, err
			}
			capture.Origin = parsed.Origin
			capture.Expression = parsed.Expression
		case "padrao":
			capture.Default = value.Value
			capture.Required = false
		case "obrigatoria":
			capture.Required = value.Value != "false"
		default:
			return capture, nodeError(key, "chave desconhecida na captura %q: %q (use de, padrao ou obrigatoria)", name, key.Value)
		}
	}
	if capture.Origin == "" {
		return capture, nodeError(node, "a captura %q precisa de 'de', por exemplo: de: $.fatura.id", name)
	}
	return capture, nil
}

func readAssertions(node *yaml.Node) ([]Check, []Assertion, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nil, nodeError(node, "verificar precisa ser um mapa, por exemplo: verificar: { status: 200 }")
	}
	var checks []Check
	var assertions []Assertion

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "status":
			status, err := strconv.Atoi(strings.TrimSpace(value.Value))
			if err != nil {
				return nil, nil, nodeError(value, "status invalido: %q (use um numero, por exemplo 200)", value.Value)
			}
			checks = append(checks, Check{Kind: CheckStatus, Status: status})
		case "corpo_contem":
			assertions = append(assertions, Assertion{Kind: AssertBodyContains, Value: value.Value, Line: value.Line})
		case "corpo_casa":
			assertions = append(assertions, Assertion{Kind: AssertRegex, Value: value.Value, Line: value.Line})
		case "json":
			if value.Kind != yaml.MappingNode {
				return nil, nil, nodeError(value, "json precisa ser um mapa, por exemplo: json: { $.status: PAGA }")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				assertion, err := parseComparison(value.Content[i].Value, value.Content[i+1])
				if err != nil {
					return nil, nil, err
				}
				assertion.Kind = AssertJSON
				assertions = append(assertions, assertion)
			}
		case "cabecalho":
			if value.Kind != yaml.MappingNode {
				return nil, nil, nodeError(value, "cabecalho precisa ser um mapa, por exemplo: cabecalho: { Content-Type: application/json }")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				assertions = append(assertions, Assertion{
					Kind: AssertHeader, Target: value.Content[i].Value,
					Operator: OpEqual, Value: value.Content[i+1].Value, Line: value.Content[i].Line,
				})
			}
		default:
			return nil, nil, nodeError(key, "verificacao desconhecida: %q\n"+
				"    disponiveis: status, corpo_contem, corpo_casa, json, cabecalho", key.Value)
		}
	}
	return checks, assertions, nil
}

func parseComparison(target string, node *yaml.Node) (Assertion, error) {
	assertion := ParseComparison(target, node.Value)
	assertion.Line = node.Line
	return assertion, nil
}

func ParseComparison(target, raw string) Assertion {
	text := strings.TrimSpace(raw)
	assertion := Assertion{Target: target, Operator: OpEqual, Value: text}

	for _, operator := range []Operator{OpLessOrEqual, OpGreaterOrEqual, OpNotEqual,
		OpLess, OpGreater} {
		if strings.HasPrefix(text, string(operator)) {
			assertion.Operator = operator
			assertion.Value = strings.TrimSpace(strings.TrimPrefix(text, string(operator)))
			return assertion
		}
	}
	switch text {
	case "existe":
		assertion.Operator = OpExists
		assertion.Value = ""
	case "contem":
		assertion.Operator = OpContains
	}
	if strings.HasPrefix(text, "contem ") {
		assertion.Operator = OpContains
		assertion.Value = strings.TrimSpace(strings.TrimPrefix(text, "contem "))
	}
	return assertion
}

func readAuth(node *yaml.Node) (*Auth, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "autenticacao precisa ser um mapa, por exemplo:\n"+
			"  autenticacao:\n"+
			"    tipo: token\n"+
			"    obter:\n"+
			"      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
			"      captura: { token: $.access_token }")
	}
	auth := &Auth{Kind: AuthToken, Line: node.Line}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "tipo":
			switch value.Value {
			case "token":
				auth.Kind = AuthToken
			case "basica":
				auth.Kind = AuthBasic
			case "cabecalho":
				auth.Kind = AuthHeader
			default:
				return nil, nodeError(value, "tipo de autenticacao desconhecido: %q (use token, basica ou cabecalho)", value.Value)
			}
		case "obter":
			step, err := readStep(value)
			if err != nil {
				return nil, err
			}
			step.Name = "obter autenticacao"
			auth.Obtain = &step
		case "renovar_apos":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, nodeError(value, "renovar_apos invalido: %q (use por exemplo 25m)", value.Value)
			}
			auth.RefreshAfter = duration
		case "cabecalho":
			auth.Header = value.Value
		case "usuario":
			auth.User = value.Value
		case "senha":
			auth.Password = value.Value
		default:
			return nil, nodeError(key, "chave desconhecida em autenticacao: %q\n%s", key.Value,
				suggestWithExample(key.Value, []string{"tipo", "obter", "renovar_apos", "cabecalho", "usuario", "senha"},
					"    'obter' carrega uma requisicao inteira mais a captura do token:\n"+
						"      autenticacao:\n"+
						"        tipo: token\n"+
						"        obter:\n"+
						"          http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
						"          captura: { token: \"$.access_token\" }\n"+
						"        renovar_apos: 25m"))
		}
	}

	if auth.Kind == AuthToken && auth.Obtain == nil {
		return nil, nodeError(node, "autenticacao por token precisa do bloco 'obter' com a requisicao que devolve o token:\n"+
			"    obter:\n"+
			"      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
			"      captura: { token: $.access_token }")
	}
	if auth.Kind == AuthBasic && (auth.User == "" || auth.Password == "") {
		return nil, nodeError(node, "autenticacao basica precisa de usuario e senha, por exemplo:\n"+
			"  autenticacao: { tipo: basica, usuario: ana, senha: \"${SENHA}\" }")
	}
	if auth.Header == "" && auth.Kind != AuthBasic {
		auth.Header = "Authorization: Bearer ${token}"
	}
	return auth, nil
}

func readData(node *yaml.Node) ([]DataSource, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "dados precisa ser um mapa de fontes, por exemplo:\n"+
			"    dados:\n      assinantes:\n        arquivo: dados/assinantes.csv")
	}
	sources := make([]DataSource, 0, len(node.Content)/2)

	for index := 0; index+1 < len(node.Content); index += 2 {
		name := node.Content[index]
		body := node.Content[index+1]
		if body.Kind != yaml.MappingNode {
			return nil, nodeError(body, "a fonte de dados %q precisa de um mapa com 'arquivo' ou 'gerar', por exemplo:\n"+
				"  %s: { arquivo: dados/assinantes.csv, consumo: circular }", name.Value, name.Value)
		}
		source := DataSource{Name: name.Value, Consume: ConsumeCircular, Line: body.Line}

		for i := 0; i+1 < len(body.Content); i += 2 {
			key := body.Content[i]
			value := body.Content[i+1]
			switch key.Value {
			case "arquivo":
				source.File = value.Value
			case "consumo":
				switch value.Value {
				case "sequencial":
					source.Consume = ConsumeSequential
				case "aleatorio":
					source.Consume = ConsumeRandom
				case "circular":
					source.Consume = ConsumeCircular
				case "unico_por_usuario":
					source.Consume = ConsumeUniquePerUser
				default:
					return nil, nodeError(value, "consumo desconhecido: %q\n"+
						"    disponiveis: circular (padrao), sequencial, aleatorio, unico_por_usuario", value.Value)
				}
			case "semente":
				seed, origin, err := ReadSeed(value.Value)
				if err != nil {
					return nil, nodeError(value, "%v", err)
				}
				source.Seed, source.SeedFrom = seed, origin
			case "gerar":
				if value.Kind != yaml.MappingNode {
					return nil, nodeError(value, "gerar precisa ser um mapa, por exemplo: gerar: { id: uuid, valor: numero(10,500) }")
				}
				source.Fields = map[string]Generator{}
				for j := 0; j+1 < len(value.Content); j += 2 {
					generator, err := readGenerator(value.Content[j].Value, value.Content[j+1])
					if err != nil {
						return nil, err
					}
					source.Fields[value.Content[j].Value] = generator
				}
			default:
				return nil, nodeError(key, "chave desconhecida na fonte de dados %q: %q\n"+
					"    disponiveis: arquivo, gerar, consumo, semente", name.Value, key.Value)
			}
		}
		if source.File == "" && len(source.Fields) == 0 {
			return nil, nodeError(body, "a fonte de dados %q precisa de 'arquivo' (CSV) ou 'gerar' (dado sintetico)", name.Value)
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func readGenerator(field string, node *yaml.Node) (Generator, error) {
	if node.Kind != yaml.MappingNode {
		return ParseGenerator(node.Value), nil
	}
	generator := Generator{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		switch key.Value {
		case "tipo":
			generator.Recipe = value.Value
		case "formato":
			generator.Format = value.Value
		case "novo_a_cada":
			switch value.Value {
			case "uso":
				generator.PerUse = true
			case "iteracao":
				generator.PerUse = false
			default:
				return generator, nodeError(value, "novo_a_cada aceita 'iteracao' (padrao) ou 'uso': %q\n"+
					"    iteracao mantem o mesmo valor nos dois passos da mesma jornada, que e o caso da chave de idempotencia", value.Value)
			}
		default:
			return generator, nodeError(key, "chave desconhecida no campo %q: %q\n"+
				"    disponiveis: tipo, formato, novo_a_cada\n"+
				"    exemplo: %s: { tipo: padrao, formato: \"PED-######\" }", field, key.Value, field)
		}
	}
	if generator.Recipe == "" {
		return generator, nodeError(node, "o campo %q precisa de 'tipo', por exemplo: %s: { tipo: padrao, formato: \"PED-######\" }", field, field)
	}
	if generator.Recipe == "padrao" && generator.Format == "" {
		return generator, nodeError(node, "o campo %q e do tipo padrao e precisa de 'formato', por exemplo: { tipo: padrao, formato: \"PED-######\" }\n"+
			"    # vira digito e @ vira letra; o resto sai literal", field)
	}
	return generator, nil
}

func readSLO(node *yaml.Node) ([]SLORule, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, nodeError(node, "slo precisa ser uma lista, por exemplo:\n"+
			"    slo:\n      - consultar pedido: { p95: < 150ms }\n      - global: { erros: < 0.1 }")
	}
	var rules []SLORule

	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, nodeError(item, "cada regra de slo e um mapa com o nome do passo (ou 'global') e os limites, por exemplo:\n"+
				"  - consultar pedido: { p95: < 150ms }")
		}
		target := item.Content[0]
		limits := item.Content[1]
		if limits.Kind != yaml.MappingNode {
			return nil, nodeError(limits, "os limites de %q precisam ser um mapa, por exemplo: { p95: < 150ms, erros: 0 %% }", target.Value)
		}

		for i := 0; i+1 < len(limits.Content); i += 2 {
			rule, err := readSLORule(target.Value, limits.Content[i], limits.Content[i+1])
			if err != nil {
				return nil, err
			}
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func readSLORule(target string, metricNode, limitNode *yaml.Node) (SLORule, error) {
	rule, err := ParseSLORule(target, metricNode.Value, limitNode.Value)
	if err != nil {
		return rule, nodeError(limitNode, "%v", err)
	}
	rule.Line = metricNode.Line
	return rule, nil
}

func ParseSLORule(target, metric, rawLimit string) (SLORule, error) {
	rule := SLORule{
		Scope:    scopeOf(target),
		Step:     target,
		Metrica:  metric,
		Operator: OpLessOrEqual,
	}
	if rule.Scope != ScopeStep {
		rule.Step = ""
	}

	if err := describeMetric(&rule); err != nil {
		return rule, err
	}

	text := strings.TrimSpace(rawLimit)
	rule.Text = rule.Metrica + ": " + text

	for _, operator := range []Operator{OpLessOrEqual, OpGreaterOrEqual, OpLess, OpGreater} {
		if strings.HasPrefix(text, string(operator)) {
			rule.Operator = operator
			text = strings.TrimSpace(strings.TrimPrefix(text, string(operator)))
			break
		}
	}

	limit, err := parseLimit(text, rule.Unit)
	if err != nil {
		return rule, fmt.Errorf("limite invalido em %q: %v\n"+
			"    exemplos: p95: < 150ms | erros: < 0.1 | sucesso: >= 99.9 | taxa_efetiva: >= 200/s | jornada_p95: <= 10%% pior", rule.Metrica, err)
	}
	rule.Limit = limit
	return rule, nil
}

func scopeOf(target string) SLOScope {
	switch target {
	case "global":
		return ScopeOverall
	case "jornada":
		return ScopeJourney
	case "regressao":
		return ScopeRegression
	default:
		return ScopeStep
	}
}

// Each metric carries the direction that reads naturally: "sucesso: 99.9" means
// at least that, the way "erros: 0.1" means at most that.
func describeMetric(rule *SLORule) error {
	if rule.Scope == ScopeRegression {
		if !isRegressionMetric(rule.Metrica) {
			return fmt.Errorf("metrica de regressao desconhecida: %q\n"+
				"    disponiveis: jornada_p50, jornada_p95, jornada_p99, global_p95, global_p99\n"+
				"    exemplo: - regressao: { jornada_p95: <= 10%% pior }", rule.Metrica)
		}
		rule.Unit = "% pior"
		return nil
	}

	switch rule.Metrica {
	case "p50", "p75", "p90", "p95", "p99", "p99.9", "max":
		rule.Unit = "ms"
	case "erros":
		rule.Unit = "%"
	case "sucesso":
		rule.Unit = "%"
		rule.Operator = OpGreaterOrEqual
	case "vazao", "taxa_efetiva":
		if rule.Scope != ScopeOverall {
			return fmt.Errorf("%q so existe em global, porque e a taxa da execucao inteira\n"+
				"    escreva:  - global: { %s: >= 200/s }", rule.Metrica, rule.Metrica)
		}
		rule.Unit = "/s"
		rule.Operator = OpGreaterOrEqual
	default:
		return fmt.Errorf("metrica de slo desconhecida: %q\n"+
			"    disponiveis: p50, p75, p90, p95, p99, p99.9, max, erros, sucesso, taxa_efetiva", rule.Metrica)
	}

	if rule.Scope == ScopeJourney && (rule.Metrica == "erros" || rule.Metrica == "sucesso") {
		return fmt.Errorf("%q nao existe em jornada: jornada que nao chega ao fim ja invalida a execucao\n"+
			"    para taxa de erro escreva:  - global: { %s: ... }", rule.Metrica, rule.Metrica)
	}
	return nil
}

func isRegressionMetric(metric string) bool {
	prefix, percentile, found := strings.Cut(metric, "_")
	if !found || (prefix != "jornada" && prefix != "global") {
		return false
	}
	switch percentile {
	case "p50", "p75", "p90", "p95", "p99", "p99.9", "max":
		return true
	}
	return false
}

func parseLimit(text, unit string) (float64, error) {
	text = strings.TrimSpace(text)
	switch unit {
	case "ms":
		duration, err := time.ParseDuration(text)
		if err != nil {
			value, numberErr := strconv.ParseFloat(text, 64)
			if numberErr != nil {
				return 0, err
			}
			return value, nil
		}
		return float64(duration.Microseconds()) / 1000, nil
	case "%":
		return strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
	case "% pior":
		text = strings.TrimSpace(strings.TrimSuffix(text, "pior"))
		return strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
	default:
		return strconv.ParseFloat(strings.TrimSuffix(text, "/s"), 64)
	}
}
