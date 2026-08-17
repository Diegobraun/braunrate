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
		return nil, nodeError(node, "capture has to be a map, for example: capture: { invoiceId: $.invoice.id }")
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
	case expression == "body":
		capture.Origin = CaptureBody
	case strings.HasPrefix(expression, "$"):
		capture.Origin = CaptureJSON
	case strings.HasPrefix(expression, "header:"):
		capture.Origin = CaptureHeader
		capture.Expression = strings.TrimSpace(strings.TrimPrefix(expression, "header:"))
	// Um cookie e um cabecalho com estrutura: "header:Set-Cookie" devolveria
	// "sessao=abc; Path=/; HttpOnly", e mandar isso de volta em Cookie: manda
	// tres cookies, dois deles inventados.
	case strings.HasPrefix(expression, "cookie:"):
		capture.Origin = CaptureCookie
		capture.Expression = strings.TrimSpace(strings.TrimPrefix(expression, "cookie:"))
	case strings.HasPrefix(expression, "/") && strings.HasSuffix(expression, "/") && len(expression) > 2:
		capture.Origin = CaptureRegex
		capture.Expression = expression[1 : len(expression)-1]
	default:
		return capture, fmt.Errorf("could not tell where to capture %q from.\n"+
			"    use one of these forms:\n"+
			"      %s: $.path.in.the.json     from a field of the JSON body\n"+
			"      %s: header:X-Request-Id    from a response header\n"+
			"      %s: cookie:session         the value of a cookie from Set-Cookie\n"+
			"      %s: /token=([a-z0-9]+)/    by the first group of the regular expression",
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
		case "from":
			parsed, err := parseCaptureExpression(name, value)
			if err != nil {
				return capture, err
			}
			capture.Origin = parsed.Origin
			capture.Expression = parsed.Expression
		case "default":
			capture.Default = value.Value
			capture.Required = false
		case "required":
			capture.Required = value.Value != "false"
		default:
			return capture, nodeError(key, "unknown key in the capture %q: %q (use from, default or required)", name, key.Value)
		}
	}
	if capture.Origin == "" {
		return capture, nodeError(node, "the capture %q needs 'from', for example: from: $.invoice.id", name)
	}
	return capture, nil
}

func readAssertions(node *yaml.Node) ([]Check, []Assertion, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nil, nodeError(node, "expect has to be a map, for example: expect: { status: 200 }")
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
				return nil, nil, nodeError(value, "invalid status: %q (use a number, for example 200)", value.Value)
			}
			checks = append(checks, Check{Kind: CheckStatus, Status: status})
		case "bodyContains":
			assertions = append(assertions, Assertion{Kind: AssertBodyContains, Value: value.Value, Line: value.Line})
		case "bodyMatches":
			assertions = append(assertions, Assertion{Kind: AssertRegex, Value: value.Value, Line: value.Line})
		case "json":
			if value.Kind != yaml.MappingNode {
				return nil, nil, nodeError(value, "json has to be a map, for example: json: { $.status: PAID }")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				assertion, err := parseComparison(value.Content[i].Value, value.Content[i+1])
				if err != nil {
					return nil, nil, err
				}
				assertion.Kind = AssertJSON
				assertions = append(assertions, assertion)
			}
		case "header":
			if value.Kind != yaml.MappingNode {
				return nil, nil, nodeError(value, "header has to be a map, for example: header: { Content-Type: application/json }")
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				assertions = append(assertions, Assertion{
					Kind: AssertHeader, Target: value.Content[i].Value,
					Operator: OpEqual, Value: value.Content[i+1].Value, Line: value.Content[i].Line,
				})
			}
		default:
			return nil, nil, nodeError(key, "unknown check: %q\n"+
				"    available: status, bodyContains, bodyMatches, json, header", key.Value)
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
	case "exists":
		assertion.Operator = OpExists
		assertion.Value = ""
	case "contains":
		assertion.Operator = OpContains
	}
	if strings.HasPrefix(text, "contains ") {
		assertion.Operator = OpContains
		assertion.Value = strings.TrimSpace(strings.TrimPrefix(text, "contains "))
	}
	return assertion
}

func readAuth(node *yaml.Node) (*Auth, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "auth has to be a map, for example:\n"+
			"  auth:\n"+
			"    type: token\n"+
			"    obtain:\n"+
			"      http: { method: POST, path: /auth/token, body: { user: ana, password: \"${PASSWORD}\" } }\n"+
			"      capture: { token: $.access_token }")
	}
	auth := &Auth{Kind: AuthToken, Line: node.Line}

	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		switch key.Value {
		case "type":
			switch value.Value {
			case "token":
				auth.Kind = AuthToken
			case "basic":
				auth.Kind = AuthBasic
			case "header":
				auth.Kind = AuthHeader
			default:
				return nil, nodeError(value, "unknown auth type: %q (use token, basic or header)", value.Value)
			}
		case "obtain":
			step, err := readStep(value)
			if err != nil {
				return nil, err
			}
			step.Name = "obtain auth"
			auth.Obtain = &step
		case "refreshAfter":
			duration, err := time.ParseDuration(value.Value)
			if err != nil {
				return nil, nodeError(value, "invalid refreshAfter: %q (use for example 25m)", value.Value)
			}
			auth.RefreshAfter = duration
		case "header":
			auth.Header = value.Value
		case "user":
			auth.User = value.Value
		case "password":
			auth.Password = value.Value
		default:
			return nil, nodeError(key, "unknown key in auth: %q\n%s", key.Value,
				suggestWithExample(key.Value, []string{"type", "obtain", "refreshAfter", "header", "user", "password"},
					"    'obtain' carries a whole request plus the capture of the token:\n"+
						"      auth:\n"+
						"        type: token\n"+
						"        obtain:\n"+
						"          http: { method: POST, path: /auth/token, body: { user: ana, password: \"${PASSWORD}\" } }\n"+
						"          capture: { token: \"$.access_token\" }\n"+
						"        refreshAfter: 25m"))
		}
	}

	if auth.Kind == AuthToken && auth.Obtain == nil {
		return nil, nodeError(node, "token auth needs the 'obtain' block with the request that returns the token:\n"+
			"    obtain:\n"+
			"      http: { method: POST, path: /auth/token, body: { user: ana, password: \"${PASSWORD}\" } }\n"+
			"      capture: { token: $.access_token }")
	}
	if auth.Kind == AuthBasic && (auth.User == "" || auth.Password == "") {
		return nil, nodeError(node, "basic auth needs a user and a password, for example:\n"+
			"  auth: { type: basic, user: ana, password: \"${PASSWORD}\" }")
	}
	if auth.Header == "" && auth.Kind != AuthBasic {
		auth.Header = "Authorization: Bearer ${token}"
	}
	return auth, nil
}

func readData(node *yaml.Node) ([]DataSource, error) {
	if node.Kind != yaml.MappingNode {
		return nil, nodeError(node, "data has to be a map of sources, for example:\n"+
			"    data:\n      subscribers:\n        file: data/subscribers.csv")
	}
	sources := make([]DataSource, 0, len(node.Content)/2)

	for index := 0; index+1 < len(node.Content); index += 2 {
		name := node.Content[index]
		body := node.Content[index+1]
		if body.Kind != yaml.MappingNode {
			return nil, nodeError(body, "the data source %q needs a map with 'file' or 'generate', for example:\n"+
				"  %s: { file: data/subscribers.csv, consume: circular }", name.Value, name.Value)
		}
		source := DataSource{Name: name.Value, Consume: ConsumeCircular, Line: body.Line}

		for i := 0; i+1 < len(body.Content); i += 2 {
			key := body.Content[i]
			value := body.Content[i+1]
			switch key.Value {
			case "file":
				source.File = value.Value
			case "consume":
				switch value.Value {
				case "sequential":
					source.Consume = ConsumeSequential
				case "random":
					source.Consume = ConsumeRandom
				case "circular":
					source.Consume = ConsumeCircular
				case "uniquePerUser":
					source.Consume = ConsumeUniquePerUser
				default:
					return nil, nodeError(value, "unknown consume mode: %q\n"+
						"    available: circular (default), sequential, random, uniquePerUser", value.Value)
				}
			case "seed":
				seed, origin, err := ReadSeed(value.Value)
				if err != nil {
					return nil, nodeError(value, "%v", err)
				}
				source.Seed, source.SeedFrom = seed, origin
			case "generate":
				if value.Kind != yaml.MappingNode {
					return nil, nodeError(value, "generate has to be a map, for example: generate: { id: uuid, amount: number(10,500) }")
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
				return nil, nodeError(key, "unknown key in the data source %q: %q\n"+
					"    available: file, generate, consume, seed", name.Value, key.Value)
			}
		}
		if source.File == "" && len(source.Fields) == 0 {
			return nil, nodeError(body, "the data source %q needs 'file' (CSV) or 'generate' (synthetic data)", name.Value)
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
		case "type":
			generator.Recipe = value.Value
		case "format":
			generator.Format = value.Value
		case "newEvery":
			switch value.Value {
			case "use":
				generator.PerUse = true
			case "iteration":
				generator.PerUse = false
			default:
				return generator, nodeError(value, "newEvery takes 'iteration' (the default) or 'use': %q\n"+
					"    iteration keeps the same value across both steps of the same journey, which is what an idempotency key needs", value.Value)
			}
		default:
			return generator, nodeError(key, "unknown key in the field %q: %q\n"+
				"    available: type, format, newEvery\n"+
				"    example: %s: { type: pattern, format: \"ORD-######\" }", field, key.Value, field)
		}
	}
	if generator.Recipe == "" {
		return generator, nodeError(node, "the field %q needs 'type', for example: %s: { type: pattern, format: \"ORD-######\" }", field, field)
	}
	if generator.Recipe == "pattern" && generator.Format == "" {
		return generator, nodeError(node, "the field %q is of type pattern and needs 'format', for example: { type: pattern, format: \"ORD-######\" }\n"+
			"    # becomes a digit and @ becomes a letter; everything else comes out literal", field)
	}
	return generator, nil
}

func readSLO(node *yaml.Node) ([]SLORule, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, nodeError(node, "slo has to be a list, for example:\n"+
			"    slo:\n      - look up order: { p95: < 150ms }\n      - global: { errors: < 0.1 }")
	}
	var rules []SLORule

	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, nodeError(item, "every slo rule is a map with the step name (or 'global') and the limits, for example:\n"+
				"  - look up order: { p95: < 150ms }")
		}
		target := item.Content[0]
		limits := item.Content[1]
		if limits.Kind != yaml.MappingNode {
			return nil, nodeError(limits, "the limits of %q have to be a map, for example: { p95: < 150ms, errors: 0 %% }", target.Value)
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
		Metric:   metric,
		Operator: OpLessOrEqual,
	}
	if rule.Scope != ScopeStep {
		rule.Step = ""
	}

	if err := describeMetric(&rule); err != nil {
		return rule, err
	}

	text := strings.TrimSpace(rawLimit)
	rule.Text = rule.Metric + ": " + text

	for _, operator := range []Operator{OpLessOrEqual, OpGreaterOrEqual, OpLess, OpGreater} {
		if strings.HasPrefix(text, string(operator)) {
			rule.Operator = operator
			text = strings.TrimSpace(strings.TrimPrefix(text, string(operator)))
			break
		}
	}

	limit, err := parseLimit(text, rule.Unit)
	if err != nil {
		return rule, fmt.Errorf("invalid limit in %q: %v\n"+
			"    examples: p95: < 150ms | errors: < 0.1 | success: >= 99.9 | throughput: >= 200/s | journeyP95: <= 10%% worse", rule.Metric, err)
	}
	rule.Limit = limit
	return rule, nil
}

func scopeOf(target string) SLOScope {
	switch target {
	case "global":
		return ScopeOverall
	case "journey":
		return ScopeJourney
	case "regression":
		return ScopeRegression
	default:
		return ScopeStep
	}
}

// Each metric carries the direction that reads naturally: "sucesso: 99.9" means
// at least that, the way "erros: 0.1" means at most that.
func describeMetric(rule *SLORule) error {
	if rule.Scope == ScopeRegression {
		if !isRegressionMetric(rule.Metric) {
			return fmt.Errorf("unknown regression metric: %q\n"+
				"    available: journeyP50, journeyP95, journeyP99, globalP95, globalP99\n"+
				"    example: - regression: { journeyP95: <= 10%% worse }", rule.Metric)
		}
		rule.Unit = "% worse"
		return nil
	}

	switch rule.Metric {
	case "p50", "p75", "p90", "p95", "p99", "p99.9", "max":
		rule.Unit = "ms"
	case "errors":
		rule.Unit = "%"
	case "success":
		rule.Unit = "%"
		rule.Operator = OpGreaterOrEqual
	case "throughput":
		if rule.Scope != ScopeOverall {
			return fmt.Errorf("%q only exists in global, because it is the rate of the whole run\n"+
				"    write:  - global: { %s: >= 200/s }", rule.Metric, rule.Metric)
		}
		rule.Unit = "/s"
		rule.Operator = OpGreaterOrEqual
	default:
		return fmt.Errorf("unknown slo metric: %q\n"+
			"    available: p50, p75, p90, p95, p99, p99.9, max, errors, success, throughput", rule.Metric)
	}

	if rule.Scope == ScopeJourney && (rule.Metric == "errors" || rule.Metric == "success") {
		return fmt.Errorf("%q does not exist in journey: a journey that does not reach the end already invalidates the run\n"+
			"    for the error rate write:  - global: { %s: ... }", rule.Metric, rule.Metric)
	}
	return nil
}

func isRegressionMetric(metric string) bool {
	for _, prefix := range []string{"journey", "global"} {
		percentile, found := strings.CutPrefix(metric, prefix)
		if !found {
			continue
		}
		switch percentile {
		case "P50", "P75", "P90", "P95", "P99", "P99.9", "Max":
			return true
		}
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
	case "% worse":
		text = strings.TrimSpace(strings.TrimSuffix(text, "worse"))
		return strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
	default:
		return strconv.ParseFloat(strings.TrimSuffix(text, "/s"), 64)
	}
}
