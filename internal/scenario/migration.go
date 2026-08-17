package scenario

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The rename map of ADR 0019 is read from both sides — the parser recognizing
// the old format, and 'braunrate migrate' rewriting it. Two copies would drift.

type Change struct {
	Line int
	From string
	To   string
}

func (change Change) String() string {
	return fmt.Sprintf("line %d: %s -> %s", change.Line, change.From, change.To)
}

var topKeyRenames = map[string]string{
	"nome":         "name",
	"alvo":         "target",
	"requer":       "requires",
	"variaveis":    "variables",
	"autenticacao": "auth",
	"mensageria":   "messaging",
	"dados":        "data",
	"carga":        "load",
	"cenario":      "scenario",
}

var stepKeyRenames = map[string]string{
	"nome":      "name",
	"peso":      "weight",
	"captura":   "capture",
	"verificar": "expect",
	"espera":    "expect",
	"aguardar":  "await",
}

// Asked before printing "unknown key": the old format is not a typo, and the
// suggestion by proximity would send the reader to fix eleven keys by hand.
func RenamedTopKey(key string) (string, bool) {
	replacement, found := topKeyRenames[key]
	return replacement, found
}

func RenamedStepKey(key string) (string, bool) {
	replacement, found := stepKeyRenames[key]
	return replacement, found
}

func outdatedFormat(key, replacement string) string {
	return fmt.Sprintf("this scenario uses the Portuguese format, replaced in 0.6.0 (%q is now %q).\n"+
		"    braunrate migrate <file>\n"+
		"    converts it to the English format, keeping comments and order. No behavior changes.", key, replacement)
}

// Migrate rewrites by position: the document is parsed only to learn which
// line and column holds which key. Re-encoding the tree would have been
// shorter and would have thrown away every comment the author wrote.
func Migrate(content []byte) ([]byte, []Change, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, nil, translateYAMLError(content, err)
	}
	if len(root.Content) == 0 {
		return content, nil, nil
	}
	document := root.Content[0]
	if document.Kind != yaml.MappingNode {
		return content, nil, nil
	}

	rewrite := &rewriter{declaredSteps: declaredStepNames(document)}
	rewrite.mapping(document, topLevel)
	return rewrite.apply(content)
}

type edit struct {
	line   int
	column int
	from   string
	to     string
}

type rewriter struct {
	edits []edit
	// The names the author declared on the steps. A rule pointing at one of them
	// is the author's text even when it starts with a word this map renames:
	// "aguardar o processador" is a step name, "aguardar pedidos" may be the key
	// the await protocol derived.
	declaredSteps map[string]bool
}

func declaredStepNames(document *yaml.Node) map[string]bool {
	names := map[string]bool{}
	for index := 0; index+1 < len(document.Content); index += 2 {
		if document.Content[index].Value != "cenario" && document.Content[index].Value != "scenario" {
			continue
		}
		for _, step := range document.Content[index+1].Content {
			if step.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i+1 < len(step.Content); i += 2 {
				if step.Content[i].Value == "nome" || step.Content[i].Value == "name" {
					names[strings.TrimSpace(step.Content[i+1].Value)] = true
				}
			}
		}
	}
	return names
}

func (rewrite *rewriter) rename(node *yaml.Node, to string) {
	if node == nil || node.Value == to || to == "" {
		return
	}
	rewrite.edits = append(rewrite.edits, edit{line: node.Line, column: node.Column, from: node.Value, to: to})
}

// Plain scalars only: a quoted value would need the quotes counted, and no
// value this migration touches is ever written that way.
func (rewrite *rewriter) renameValue(node *yaml.Node, table map[string]string) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return
	}
	if to, found := table[strings.TrimSpace(node.Value)]; found {
		rewrite.rename(node, to)
	}
}

func (rewrite *rewriter) apply(content []byte) ([]byte, []Change, error) {
	if len(rewrite.edits) == 0 {
		return content, nil, nil
	}
	sort.SliceStable(rewrite.edits, func(first, second int) bool {
		if rewrite.edits[first].line != rewrite.edits[second].line {
			return rewrite.edits[first].line < rewrite.edits[second].line
		}
		return rewrite.edits[first].column < rewrite.edits[second].column
	})

	lines := strings.Split(string(content), "\n")
	changes := make([]Change, 0, len(rewrite.edits))
	// Backwards so an earlier edit does not move the column of a later one on
	// the same line, which is the usual case in a flow map.
	for index := len(rewrite.edits) - 1; index >= 0; index-- {
		item := rewrite.edits[index]
		if item.line-1 < 0 || item.line-1 >= len(lines) {
			continue
		}
		runes := []rune(lines[item.line-1])
		start := item.column - 1
		if start < 0 || start+len([]rune(item.from)) > len(runes) {
			continue
		}
		if string(runes[start:start+len([]rune(item.from))]) != item.from {
			continue
		}
		lines[item.line-1] = string(runes[:start]) + item.to + string(runes[start+len([]rune(item.from)):])
		changes = append(changes, Change{Line: item.line, From: item.from, To: item.to})
	}
	for first, last := 0, len(changes)-1; first < last; first, last = first+1, last-1 {
		changes[first], changes[last] = changes[last], changes[first]
	}
	return []byte(strings.Join(lines, "\n")), changes, nil
}

type context int

const (
	topLevel context = iota
	loadBlock
	profileList
	profileBody
	authBlock
	dataBlock
	dataSource
	generateBlock
	generateField
	messagingBlock
	brokerBlock
	brokerAuth
	tlsBlock
	stepList
	stepBlock
	expectBlock
	captureBlock
	captureBody
	httpBlock
	graphqlBlock
	kafkaBlock
	amqpBlock
	awaitBlock
	awaitSource
	awaitHTTP
	sloList
	sloLimits
	opaque
)

// The rename of a key depends on where it sits: 'ate' is 'to' inside a profile
// and 'until' inside await, and 'padrao' is a key named 'default' in a capture
// and a value named 'pattern' in a generator.
var keysByContext = map[context]map[string]string{
	loadBlock: {
		"modelo": "model", "perfis": "profiles", "usuarios": "users",
		"duracao": "duration", "intervalo_entre_iteracoes": "thinkTime",
	},
	profileBody: {"de": "from", "ate": "to", "taxa": "rate", "durante": "duration"},
	authBlock: {
		"tipo": "type", "obter": "obtain", "renovar_apos": "refreshAfter",
		"cabecalho": "header", "usuario": "user", "senha": "password",
	},
	dataSource:    {"arquivo": "file", "consumo": "consume", "semente": "seed", "gerar": "generate"},
	generateField: {"tipo": "type", "formato": "format", "novo_a_cada": "newEvery"},
	brokerBlock:   {"enderecos": "addresses", "autenticacao": "auth"},
	brokerAuth:    {"tipo": "type", "usuario": "user", "senha": "password", "regiao": "region"},
	tlsBlock:      {"certificado": "certificate", "chave": "key"},
	stepBlock:     stepKeyRenames,
	expectBlock:   {"corpo_contem": "bodyContains", "corpo_casa": "bodyMatches", "cabecalho": "header"},
	captureBody:   {"de": "from", "padrao": "default", "obrigatoria": "required"},
	httpBlock: {
		"metodo": "method", "caminho": "path", "cabecalhos": "headers",
		"corpo": "body", "seguir_redirect": "followRedirects",
	},
	graphqlBlock: {"consulta": "query", "operacao": "operation", "variaveis": "variables", "caminho": "path", "cabecalhos": "headers"},
	kafkaBlock: {
		"topico": "topic", "chave": "key", "valor": "value",
		"cabecalhos": "headers", "particao": "partition", "grupo": "group",
	},
	amqpBlock: {
		"troca": "exchange", "rota": "routingKey", "fila": "queue", "corpo": "body",
		"identidade": "messageId", "cabecalhos": "headers", "persistente": "persistent", "confirmar": "confirm",
	},
	awaitBlock:  {"ate": "until", "intervalo": "interval", "chave": "key", "campo": "field", "igual_a": "equals"},
	awaitSource: {"topico": "topic", "fila": "queue", "enderecos": "addresses"},
	awaitHTTP:   {"caminho": "path"},
	sloLimits: {
		"erros": "errors", "sucesso": "success", "vazao": "throughput", "taxa_efetiva": "throughput",
		"jornada_p50": "journeyP50", "jornada_p75": "journeyP75", "jornada_p90": "journeyP90",
		"jornada_p95": "journeyP95", "jornada_p99": "journeyP99", "jornada_max": "journeyMax",
		"global_p50": "globalP50", "global_p75": "globalP75", "global_p90": "globalP90",
		"global_p95": "globalP95", "global_p99": "globalP99", "global_max": "globalMax",
	},
}

var (
	loadModels     = map[string]string{"aberto": "open", "fechado": "closed"}
	profileKinds   = map[string]string{"rampa": "ramp", "patamar": "steady", "pico": "spike", "constante": "steady"}
	authKinds      = map[string]string{"basica": "basic", "cabecalho": "header"}
	consumeKinds   = map[string]string{"sequencial": "sequential", "aleatorio": "random", "unico_por_usuario": "uniquePerUser"}
	generatorKinds = map[string]string{
		"sequencia": "sequence", "numero": "number", "inteiro": "integer",
		"nome": "name", "texto": "text", "padrao": "pattern",
	}
	newEveryKinds = map[string]string{"iteracao": "iteration", "uso": "use"}
	brokerKinds   = map[string]string{
		"sasl_plain": "saslPlain", "scram_sha256": "scramSha256",
		"scram_sha512": "scramSha512", "msk_iam": "mskIam", "certificado": "certificate",
	}
	acksKinds        = map[string]string{"todos": "all", "lider": "leader", "nenhum": "none"}
	requirementKinds = map[string]string{"credencial": "credential"}
	sloScopes        = map[string]string{"jornada": "journey", "regressao": "regression"}
	comparisons      = map[string]string{"existe": "exists", "contem": "contains"}
	// A step with no declared name reports under the key its protocol derives,
	// and those three prefixes changed with the format. The rest of the key is
	// the topic or the path the author wrote, and stays.
	derivedStepKeys = map[string]string{
		"kafka produzir ": "kafka produce ", "amqp publicar ": "amqp publish ", "aguardar ": "await ",
	}
)

func (rewrite *rewriter) mapping(node *yaml.Node, where context) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		rewrite.pair(key, value, where)
	}
}

func (rewrite *rewriter) sequence(node *yaml.Node, where context) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range node.Content {
		switch where {
		case profileList:
			rewrite.profile(item)
		case stepList:
			rewrite.mapping(item, stepBlock)
		case sloList:
			rewrite.sloRule(item)
		}
	}
}

func (rewrite *rewriter) profile(node *yaml.Node) {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content) < 2 {
		return
	}
	rewrite.renameValue(node.Content[0], profileKinds)
	rewrite.mapping(node.Content[1], profileBody)
}

func (rewrite *rewriter) sloRule(node *yaml.Node) {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content) < 2 {
		return
	}
	// The target of a rule is a step name the author wrote; only the reserved
	// scopes and the keys a protocol derived for an unnamed step are ours to
	// rename. Leaving the derived ones alone left every messaging scenario
	// converted and invalid, pointing at a step that no longer answers by that
	// name.
	rewrite.renameValue(node.Content[0], sloScopes)
	rewrite.renameDerivedStep(node.Content[0])
	rewrite.mapping(node.Content[1], sloLimits)
	for index := 1; index < len(node.Content); index += 2 {
		limits := node.Content[index]
		if limits.Kind != yaml.MappingNode {
			continue
		}
		for i := 1; i < len(limits.Content); i += 2 {
			limit := limits.Content[i]
			if strings.Contains(limit.Value, "pior") {
				rewrite.rename(limit, strings.ReplaceAll(limit.Value, "pior", "worse"))
			}
		}
	}
}

func (rewrite *rewriter) renameDerivedStep(node *yaml.Node) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return
	}
	name := strings.TrimSpace(node.Value)
	if rewrite.declaredSteps[name] {
		return
	}
	for from, to := range derivedStepKeys {
		if rest, found := strings.CutPrefix(name, from); found {
			rewrite.rename(node, to+rest)
			return
		}
	}
}

func (rewrite *rewriter) pair(key, value *yaml.Node, where context) {
	name := key.Value
	if table, has := keysByContext[where]; has {
		if to, found := table[name]; found {
			rewrite.rename(key, to)
		}
	}
	if where == topLevel {
		if to, found := topKeyRenames[name]; found {
			rewrite.rename(key, to)
		}
	}
	rewrite.descend(name, value, where)
}

func (rewrite *rewriter) descend(name string, value *yaml.Node, where context) {
	switch where {
	case topLevel:
		switch name {
		case "carga", "load":
			rewrite.mapping(value, loadBlock)
		case "cenario", "scenario":
			rewrite.sequence(value, stepList)
		case "autenticacao", "auth":
			rewrite.mapping(value, authBlock)
		case "dados", "data":
			rewrite.mapping(value, dataBlock)
		case "mensageria", "messaging":
			rewrite.mapping(value, messagingBlock)
		case "tls":
			rewrite.mapping(value, tlsBlock)
		case "slo":
			rewrite.sequence(value, sloList)
		case "requer", "requires":
			rewrite.list(value, requirementKinds)
		}
	case loadBlock:
		switch name {
		case "modelo", "model":
			rewrite.renameValue(value, loadModels)
		case "perfis", "profiles":
			rewrite.sequence(value, profileList)
		}
	case authBlock:
		switch name {
		case "tipo", "type":
			rewrite.renameValue(value, authKinds)
		case "obter", "obtain":
			rewrite.mapping(value, stepBlock)
		}
	case dataBlock:
		rewrite.mapping(value, dataSource)
	case dataSource:
		switch name {
		case "consumo", "consume":
			rewrite.renameValue(value, consumeKinds)
		case "gerar", "generate":
			rewrite.mapping(value, generateBlock)
		}
	case generateBlock:
		if value.Kind == yaml.MappingNode {
			rewrite.mapping(value, generateField)
			return
		}
		rewrite.generatorShorthand(value)
	case generateField:
		switch name {
		case "tipo", "type":
			rewrite.renameValue(value, generatorKinds)
		case "novo_a_cada", "newEvery":
			rewrite.renameValue(value, newEveryKinds)
		}
	case messagingBlock:
		rewrite.mapping(value, brokerBlock)
	case brokerBlock:
		switch name {
		case "autenticacao", "auth":
			rewrite.mapping(value, brokerAuth)
		case "tls":
			rewrite.mapping(value, tlsBlock)
		}
	case brokerAuth:
		if name == "tipo" || name == "type" {
			rewrite.renameValue(value, brokerKinds)
		}
	case stepBlock:
		switch name {
		case "verificar", "espera", "expect":
			rewrite.mapping(value, expectBlock)
		case "captura", "capture":
			rewrite.mapping(value, captureBlock)
		case "http":
			rewrite.mapping(value, httpBlock)
		case "graphql":
			rewrite.mapping(value, graphqlBlock)
		case "kafka":
			rewrite.mapping(value, kafkaBlock)
		case "amqp":
			rewrite.mapping(value, amqpBlock)
		case "aguardar", "await":
			rewrite.mapping(value, awaitBlock)
		}
	case expectBlock:
		if name == "json" {
			rewrite.comparisons(value)
		}
	case captureBlock:
		if value.Kind == yaml.MappingNode {
			rewrite.mapping(value, captureBody)
			return
		}
		rewrite.captureExpression(value)
	case captureBody:
		if name == "de" || name == "from" {
			rewrite.captureExpression(value)
		}
	case kafkaBlock:
		if name == "acks" {
			rewrite.renameValue(value, acksKinds)
		}
	case awaitBlock:
		switch name {
		case "kafka", "amqp":
			rewrite.mapping(value, awaitSource)
		case "http":
			rewrite.mapping(value, awaitHTTP)
		case "ate", "until":
			rewrite.comparisons(value)
		}
	}
}

func (rewrite *rewriter) list(node *yaml.Node, table map[string]string) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range node.Content {
		rewrite.renameValue(item, table)
	}
}

func (rewrite *rewriter) comparisons(node *yaml.Node) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for index := 1; index < len(node.Content); index += 2 {
		rewrite.renameValue(node.Content[index], comparisons)
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == "corpo_contem" {
			rewrite.rename(node.Content[index], "bodyContains")
		}
	}
}

func (rewrite *rewriter) captureExpression(node *yaml.Node) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return
	}
	switch {
	case node.Value == "corpo":
		rewrite.rename(node, "body")
	case strings.HasPrefix(node.Value, "cabecalho:"):
		rewrite.rename(node, "header:"+strings.TrimPrefix(node.Value, "cabecalho:"))
	}
}

func (rewrite *rewriter) generatorShorthand(node *yaml.Node) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return
	}
	recipe, arguments, hasArguments := strings.Cut(node.Value, "(")
	to, found := generatorKinds[strings.TrimSpace(recipe)]
	if !found {
		return
	}
	if hasArguments {
		rewrite.rename(node, to+"("+arguments)
		return
	}
	rewrite.rename(node, to)
}
