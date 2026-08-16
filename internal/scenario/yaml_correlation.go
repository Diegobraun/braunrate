package scenario

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// A forma da expressao decide a origem: "$.campo" e JSON, "cabecalho:X-Id" e
// cabecalho, "/padrao/" e expressao regular. O QA escreve uma linha e pronto.
func readCaptures(no *yaml.Node) ([]Capture, error) {
	if no.Kind != yaml.MappingNode {
		return nil, nodeError(no, "captura precisa ser um mapa, por exemplo: captura: { faturaId: $.fatura.id }")
	}
	captures := make([]Capture, 0, len(no.Content)/2)
	for index := 0; index+1 < len(no.Content); index += 2 {
		name := no.Content[index]
		value := no.Content[index+1]

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

// A leitura da expressao e node-free porque a DSL em Go entra por aqui tambem:
// duas interpretacoes da mesma expressao viraria captura que funciona num
// publico e falha no outro.
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
	case strings.HasPrefix(expression, "/") && strings.HasSuffix(expression, "/") && len(expression) > 2:
		capture.Origin = CaptureRegex
		capture.Expression = expression[1 : len(expression)-1]
	default:
		return capture, fmt.Errorf("nao entendi de onde capturar %q.\n"+
			"    use uma destas formas:\n"+
			"      %s: $.caminho.no.json      captura de um campo do corpo JSON\n"+
			"      %s: cabecalho:X-Request-Id captura de um cabecalho da resposta\n"+
			"      %s: /token=([a-z0-9]+)/    captura pelo primeiro grupo da expressao regular",
			name, name, name, name)
	}
	return capture, nil
}

func parseCaptureExpression(name string, no *yaml.Node) (Capture, error) {
	capture, err := ParseCapture(name, no.Value)
	if err != nil {
		return capture, nodeError(no, "%v", err)
	}
	capture.Line = no.Line
	return capture, nil
}

func readFullCapture(name string, no *yaml.Node) (Capture, error) {
	capture := Capture{Variable: name, Required: true, Line: no.Line}
	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
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
		return capture, nodeError(no, "a captura %q precisa de 'de', por exemplo: de: $.fatura.id", name)
	}
	return capture, nil
}

func readAssertions(no *yaml.Node) ([]Check, []Assertion, error) {
	if no.Kind != yaml.MappingNode {
		return nil, nil, nodeError(no, "verificar precisa ser um mapa, por exemplo: verificar: { status: 200 }")
	}
	var checks []Check
	var assertions []Assertion

	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
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

func parseComparison(target string, no *yaml.Node) (Assertion, error) {
	assertion := ParseComparison(target, no.Value)
	assertion.Line = no.Line
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

func readAuth(no *yaml.Node) (*Auth, error) {
	if no.Kind != yaml.MappingNode {
		return nil, nodeError(no, "autenticacao precisa ser um mapa, por exemplo:\n"+
			"  autenticacao:\n"+
			"    tipo: token\n"+
			"    obter:\n"+
			"      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
			"      captura: { token: $.access_token }")
	}
	auth := &Auth{Kind: AuthToken, Line: no.Line}

	for index := 0; index+1 < len(no.Content); index += 2 {
		key := no.Content[index]
		value := no.Content[index+1]
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
			return nil, nodeError(key, "chave desconhecida em autenticacao: %q\n"+
				"    disponiveis: tipo, obter, renovar_apos, cabecalho, usuario, senha", key.Value)
		}
	}

	if auth.Kind == AuthToken && auth.Obtain == nil {
		return nil, nodeError(no, "autenticacao por token precisa do bloco 'obter' com a requisicao que devolve o token:\n"+
			"    obter:\n"+
			"      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
			"      captura: { token: $.access_token }")
	}
	if auth.Kind == AuthBasic && (auth.User == "" || auth.Password == "") {
		return nil, nodeError(no, "autenticacao basica precisa de usuario e senha, por exemplo:\n"+
			"  autenticacao: { tipo: basica, usuario: ana, senha: \"${SENHA}\" }")
	}
	if auth.Header == "" && auth.Kind != AuthBasic {
		auth.Header = "Authorization: Bearer ${token}"
	}
	return auth, nil
}

func readData(no *yaml.Node) ([]DataSource, error) {
	if no.Kind != yaml.MappingNode {
		return nil, nodeError(no, "dados precisa ser um mapa de fontes, por exemplo:\n"+
			"    dados:\n      assinantes:\n        arquivo: dados/assinantes.csv")
	}
	sources := make([]DataSource, 0, len(no.Content)/2)

	for index := 0; index+1 < len(no.Content); index += 2 {
		name := no.Content[index]
		body := no.Content[index+1]
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
				seed, err := strconv.ParseInt(value.Value, 10, 64)
				if err != nil {
					return nil, nodeError(value, "semente invalida: %q (use um numero inteiro)", value.Value)
				}
				source.Seed = seed
			case "gerar":
				if value.Kind != yaml.MappingNode {
					return nil, nodeError(value, "gerar precisa ser um mapa, por exemplo: gerar: { id: uuid, valor: numero(10,500) }")
				}
				source.Fields = map[string]string{}
				for j := 0; j+1 < len(value.Content); j += 2 {
					source.Fields[value.Content[j].Value] = value.Content[j+1].Value
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

func readSLO(no *yaml.Node) ([]SLORule, error) {
	if no.Kind != yaml.SequenceNode {
		return nil, nodeError(no, "slo precisa ser uma lista, por exemplo:\n"+
			"    slo:\n      - consultar pedido: { p95: < 150ms }\n      - global: { erros: < 0.1 }")
	}
	var rules []SLORule

	for _, item := range no.Content {
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
		Step:     target,
		Overall:  target == "global",
		Metrica:  metric,
		Operator: OpLessOrEqual,
	}

	switch rule.Metrica {
	case "p50", "p75", "p90", "p95", "p99", "p99.9", "max":
		rule.Unit = "ms"
	case "erros":
		rule.Unit = "%"
	case "vazao":
		rule.Unit = "/s"
		rule.Operator = OpGreaterOrEqual
	default:
		return rule, fmt.Errorf("metrica de slo desconhecida: %q\n"+
			"    disponiveis: p50, p75, p90, p95, p99, p99.9, max, erros, vazao", rule.Metrica)
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
			"    exemplos: p95: < 150ms | erros: < 0.1 | vazao: > 500/s", rule.Metrica, err)
	}
	rule.Limit = limit
	return rule, nil
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
	default:
		return strconv.ParseFloat(strings.TrimSuffix(text, "/s"), 64)
	}
}
