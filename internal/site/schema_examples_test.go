package site_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/site"
)

// Exemplo publicado que o parser recusa e pior que chave sem exemplo: quem copia
// perde tempo procurando o proprio erro. Cada exemplo entra no menor cenario que
// exercita a chave que ele documenta, e o cenario inteiro passa pelo parser e
// pela validacao.
//
// Chave com exemplo e sem cenario onde encaixa-lo reprova junto: um teste que
// pula em silencio o que nao sabe montar deixa de provar exatamente o exemplo
// novo, que e o que ninguem conferiu ainda.
func TestEverySchemaExampleIsAcceptedByTheParser(t *testing.T) {
	// Os exemplos ensinam a ler credencial e endereco do ambiente, entao o
	// ambiente do teste tem que ter o que eles pedem.
	for name, value := range map[string]string{
		"TARGET": "http://127.0.0.1:8080", "PASSWORD": "x", "API_KEY": "x",
		"KAFKA_USER": "ana", "KAFKA_PASSWORD": "x", "AMQP_USER": "ana", "AMQP_PASSWORD": "x",
		"AMQP_URL": "amqp://127.0.0.1:5672/", "REGION": "us-east-1", "SEED": "42",
	} {
		t.Setenv(name, value)
	}
	examples := schemaExamples(t)
	if len(examples) < 100 {
		t.Fatalf("achei %d exemplos no schema: o teste não estaria provando nada", len(examples))
	}
	for _, example := range examples {
		template, known := hostByExample[example.path][example.encoded]
		if !known {
			template, known = hosts[example.path]
		}
		if !known {
			t.Errorf("%s tem exemplo e não tem cenário onde encaixá-lo: acrescente um em hosts",
				example.path)
			continue
		}
		document := strings.ReplaceAll(template, placeholder, example.encoded)
		specification, err := scenario.Parse([]byte(document))
		if err != nil {
			t.Errorf("o exemplo de %s não carrega: %v\n%s", example.path, err, document)
			continue
		}
		if err := specification.Validate(); err != nil {
			t.Errorf("o exemplo de %s não passa na validação: %v\n%s", example.path, err, document)
		}
	}
}

// A referencia sai do schema, entao chave sem descricao e sem exemplo e chave
// que aparece na pagina como uma linha vazia. O teste trava o resultado da
// auditoria: chave nova nasce documentada ou reprova aqui.
func TestEverySchemaKeyIsDescribedAndExemplified(t *testing.T) {
	keys := documentedKeys(t)
	if len(keys) < 100 {
		t.Fatalf("achei %d chaves no schema: o teste não estaria provando nada", len(keys))
	}
	for _, path := range sortedNames(keys) {
		if !keys[path].described {
			t.Errorf("%s não tem description", path)
		}
		if !keys[path].exemplified {
			t.Errorf("%s não tem examples", path)
		}
	}
}

// Segredo escrito no exemplo ensina o contrario do que a ferramenta recusa na
// validacao, e a documentacao e o lugar de onde o habito sai.
func TestNoSchemaExampleCarriesALiteralCredential(t *testing.T) {
	secretField := regexp.MustCompile(`(?i)"[^"]*(password|secret|token|apikey|api_key)[^"]*"\s*:\s*"([^"]*)"`)
	for _, example := range schemaExamples(t) {
		for _, match := range secretField.FindAllStringSubmatch(example.encoded, -1) {
			value := match[2]
			// "token: $.access_token" e a expressao que diz de onde tirar o valor,
			// e nao o valor: o segredo continua so na resposta do alvo.
			if strings.Contains(value, "${") || strings.HasPrefix(value, "$.") ||
				strings.HasPrefix(value, "header:") || strings.HasPrefix(value, "cookie:") {
				continue
			}
			t.Errorf("o exemplo de %s escreve um valor literal em credencial: %s", example.path, match[0])
		}
	}
}

// O cenario completo no topo da referencia e um cenario de verdade: se ele
// deixar de carregar, a primeira coisa que a pagina mostra estara errada.
func TestTheCompleteScenarioAtTheTopOfTheReferenceRuns(t *testing.T) {
	for _, language := range site.Languages {
		page, err := site.ReferencePage(root, language)
		if err != nil {
			t.Fatalf("%s: a referência não foi gerada: %v", language.Code, err)
		}
		block, found := firstYAMLBlock(page.Markdown)
		if !found {
			t.Fatalf("%s: a referência não abre com um cenário completo", language.Code)
		}
		specification, err := scenario.Parse([]byte(block))
		if err != nil {
			t.Fatalf("%s: o cenário do topo da referência não carrega: %v\n%s", language.Code, err, block)
		}
		if err := specification.Validate(); err != nil {
			t.Fatalf("%s: o cenário do topo da referência não valida: %v\n%s", language.Code, err, block)
		}
	}
}

func firstYAMLBlock(markdown string) (string, bool) {
	_, after, found := strings.Cut(markdown, "```yaml\n")
	if !found {
		return "", false
	}
	block, _, found := strings.Cut(after, "```")
	return block, found
}

type schemaExample struct {
	path string
	// JSON e YAML de fluxo valido, entao o valor entra em uma linha so, seja ele
	// texto, numero, lista ou objeto.
	encoded string
}

func schemaDocument(t *testing.T) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "docs", "braunrate.schema.json"))
	if err != nil {
		t.Fatalf("não consegui ler o schema: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("o schema não carrega: %v", err)
	}
	return document
}

type documentedKey struct {
	described   bool
	exemplified bool
}

func documentedKeys(t *testing.T) map[string]documentedKey {
	t.Helper()
	document := schemaDocument(t)
	keys := map[string]documentedKey{}
	walkKeys(document, document, "", 0, keys)
	return keys
}

// A chave que o usuario escreve e o caminho por onde ela e alcancada, e nao o
// $defs onde ela mora: 'method' documentada uma vez no tipo http vale para
// todos os passos que apontam para ele. Por isso a caminhada resolve o $ref, e
// o limite de profundidade e o que a impede de girar em tipo que se referencia.
func walkKeys(root, node any, path string, depth int, keys map[string]documentedKey) {
	shape, is := resolveRef(root, node).(map[string]any)
	if !is || depth > 12 {
		return
	}
	for name, child := range mapOf(shape["properties"]) {
		full := name
		if path != "" {
			full = path + "." + name
		}
		resolved, _ := resolveRef(root, child).(map[string]any)
		_, described := resolved["description"].(string)
		examples, _ := resolved["examples"].([]any)
		keys[full] = documentedKey{described: described, exemplified: len(examples) > 0}
		walkKeys(root, child, full, depth+1, keys)
	}
	walkKeys(root, shape["items"], path, depth+1, keys)
	walkKeys(root, shape["additionalProperties"], path+".*", depth+1, keys)
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		branches, _ := shape[key].([]any)
		for _, branch := range branches {
			walkKeys(root, branch, path, depth+1, keys)
		}
	}
}

// O que a chave diz sobre si mesma vence o que o tipo diz: 'auth.obtain.http'
// aponta para o tipo http e acrescenta que ali so cabe ambiente e valor fixo.
func resolveRef(root, node any) any {
	for range 10 {
		shape, is := node.(map[string]any)
		if !is {
			return node
		}
		reference, has := shape["$ref"].(string)
		if !has {
			return node
		}
		target := root
		for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
			step, is := target.(map[string]any)
			if !is {
				return node
			}
			target = step[part]
		}
		merged := map[string]any{}
		for name, value := range mapOf(target) {
			merged[name] = value
		}
		for _, name := range []string{"description", "examples", "default"} {
			if value, has := shape[name]; has {
				merged[name] = value
			}
		}
		node = merged
	}
	return node
}

func mapOf(node any) map[string]any {
	shape, _ := node.(map[string]any)
	return shape
}

func sortedNames(keys map[string]documentedKey) []string {
	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func schemaExamples(t *testing.T) []schemaExample {
	t.Helper()
	document := schemaDocument(t)
	var found []schemaExample
	collect(t, document, "", &found)
	sort.Slice(found, func(first, second int) bool {
		if found[first].path != found[second].path {
			return found[first].path < found[second].path
		}
		return found[first].encoded < found[second].encoded
	})
	return found
}

func collect(t *testing.T, node any, path string, found *[]schemaExample) {
	t.Helper()
	shape, is := node.(map[string]any)
	if !is {
		return
	}
	if examples, has := shape["examples"].([]any); has && path != "" {
		for _, example := range examples {
			// Sem desligar o escape de HTML, "< 150ms" viraria "\u003c 150ms" e o
			// cenario montado teria um limite que o parser nao reconhece.
			var buffer bytes.Buffer
			encoder := json.NewEncoder(&buffer)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(example); err != nil {
				t.Fatalf("não consegui serializar o exemplo de %s: %v", path, err)
			}
			*found = append(*found, schemaExample{path: path, encoded: strings.TrimSpace(buffer.String())})
		}
	}
	for _, key := range []string{"properties", "$defs"} {
		children, has := shape[key].(map[string]any)
		if !has {
			continue
		}
		for name, child := range children {
			next := name
			switch {
			case key == "$defs":
				next = "$" + name
			case path != "":
				next = path + "." + name
			}
			collect(t, child, next, found)
		}
	}
	collect(t, shape["items"], path, found)
	// O que vale para "qualquer chave" tem exemplo proprio, e ele nao e exemplo
	// da chave que o contem: o exemplo de uma captura e o lado direito de uma
	// linha dela, nao o bloco inteiro.
	collect(t, shape["additionalProperties"], path+".*", found)
	if branches, has := shape["oneOf"].([]any); has {
		for _, branch := range branches {
			collect(t, branch, path, found)
		}
	}
	for _, key := range []string{"anyOf", "allOf"} {
		if branches, has := shape[key].([]any); has {
			for _, branch := range branches {
				collect(t, branch, path, found)
			}
		}
	}
}

const placeholder = "«»"

const (
	head      = "name: Order lookup\ntarget: http://127.0.0.1:8080\n"
	someLoad  = "load:\n  profiles:\n    - steady: { rate: 100/s, duration: 1m }\n"
	someSteps = "scenario:\n  - http: GET /orders/1001\n    name: look up order\n"
	base      = head + someLoad + someSteps

	// Os exemplos de passo referenciam dados e falam com broker, e sem as duas
	// declaracoes a validacao reprovaria a falta delas em vez do exemplo.
	stepHead = head +
		"variables:\n  tenant: acme\n" +
		"data:\n  orders: { generate: { id: uuid, amount: \"number(10,500)\" } }\n" +
		"  subscribers: { file: data/subscribers.csv }\n" +
		"messaging:\n  kafka: { brokers: [127.0.0.1:9092] }\n  amqp: { brokers: [\"amqp://127.0.0.1:5672/\"] }\n"
)

func step(body string) string  { return stepHead + someLoad + "scenario:\n  - " + body + "\n" }
func extra(body string) string { return base + body + "\n" }

// Exemplo cuja forma decide o cenario: um limite de tempo nao encaixa numa
// regra de vazao, e o modelo fechado nao aceita o bloco de perfis.
var hostByExample = map[string]map[string]string{
	"$limit": {
		`"< 150ms"`: extra("slo:\n  - look up order: { p95: \"< 150ms\" }"),
		`"<= 2s"`:   extra("slo:\n  - journey: { p95: \"<= 2s\" }"),
		`"> 250/s"`: extra("slo:\n  - global: { throughput: \"> 250/s\" }"),
		`"< 0.1"`:   extra("slo:\n  - global: { errors: \"< 0.1\" }"),
	},
	"$load.model": {
		`"open"`:   head + "load:\n  model: open\n  profiles:\n    - steady: { rate: 100/s, duration: 1m }\n" + someSteps,
		`"closed"`: head + "load:\n  model: closed\n  users: 10\n  duration: 1m\n" + someSteps,
	},
}

// O menor cenario que exercita cada chave. Escrito a mao porque um gerador
// generico monta combinacao que o formato recusa de proposito — 'file' com
// 'generate', 'profiles' com 'users' — e o teste passaria a reprovar o formato
// em vez do exemplo.
var hosts = map[string]string{
	// topo
	"name":      "name: " + placeholder + "\ntarget: http://127.0.0.1:8080\n" + someLoad + someSteps,
	"target":    "name: Order lookup\ntarget: " + placeholder + "\n" + someLoad + someSteps,
	"requires":  extra("requires: " + placeholder),
	"variables": extra("variables: " + placeholder),
	"data":      extra("data: " + placeholder),
	"auth":      extra("auth: " + placeholder),
	"tls":       extra("tls: " + placeholder),
	"messaging": extra("messaging: " + placeholder),
	"slo":       extra("slo: " + placeholder),
	"load":      head + "load: " + placeholder + "\n" + someSteps,
	"scenario":  head + someLoad + "scenario: " + placeholder + "\n",

	"messaging.kafka": extra("messaging:\n  kafka: " + placeholder),
	"messaging.amqp":  extra("messaging:\n  amqp: " + placeholder),

	// auth
	"$auth.type":           extra(`auth: { type: ` + placeholder + `, header: "X-API-Key: ${API_KEY}", obtain: { http: { method: POST, path: /auth/token }, capture: { token: $.access_token } } }`),
	"$auth.user":           extra(`auth: { type: basic, user: ` + placeholder + `, password: "${PASSWORD}" }`),
	"$auth.password":       extra(`auth: { type: basic, user: ana, password: ` + placeholder + ` }`),
	"$auth.header":         extra(`auth: { type: token, header: ` + placeholder + `, obtain: { http: { method: POST, path: /auth/token }, capture: { token: $.access_token } } }`),
	"$auth.refreshAfter":   extra(`auth: { type: token, refreshAfter: ` + placeholder + `, obtain: { http: { method: POST, path: /auth/token }, capture: { token: $.access_token } } }`),
	"$auth.obtain":         extra(`auth: { type: token, obtain: ` + placeholder + ` }`),
	"$auth.obtain.http":    extra(`auth: { type: token, obtain: { http: ` + placeholder + `, capture: { token: $.access_token } } }`),
	"$auth.obtain.capture": extra(`auth: { type: token, obtain: { http: { method: POST, path: /auth/token }, capture: ` + placeholder + ` } }`),

	// tls
	"$tls.ca":          extra(`tls: { ca: ` + placeholder + ` }`),
	"$tls.certificate": extra(`tls: { ca: /etc/ssl/staging/ca.pem, certificate: ` + placeholder + `, key: /etc/ssl/staging/client.key }`),
	"$tls.key":         extra(`tls: { ca: /etc/ssl/staging/ca.pem, certificate: /etc/ssl/staging/client.pem, key: ` + placeholder + ` }`),

	// broker
	"$broker.brokers":       extra(`messaging: { kafka: { brokers: ` + placeholder + ` } }`),
	"$broker.tls":           extra(`messaging: { kafka: { brokers: [127.0.0.1:9092], tls: ` + placeholder + ` } }`),
	"$broker.auth":          extra(`messaging: { kafka: { brokers: [127.0.0.1:9092], auth: ` + placeholder + ` } }`),
	"$broker.auth.type":     extra(`messaging: { kafka: { brokers: [127.0.0.1:9092], auth: { type: ` + placeholder + `, region: us-east-1, user: "${KAFKA_USER}", password: "${KAFKA_PASSWORD}" } } }`),
	"$broker.auth.user":     extra(`messaging: { kafka: { brokers: [127.0.0.1:9092], auth: { type: scramSha512, user: ` + placeholder + `, password: "${KAFKA_PASSWORD}" } } }`),
	"$broker.auth.password": extra(`messaging: { kafka: { brokers: [127.0.0.1:9092], auth: { type: scramSha512, user: "${KAFKA_USER}", password: ` + placeholder + ` } } }`),
	"$broker.auth.region":   extra(`messaging: { kafka: { brokers: [127.0.0.1:9092], auth: { type: mskIam, region: ` + placeholder + ` } } }`),

	// dados
	"$dataSource.file":          extra(`data: { subscribers: { file: ` + placeholder + ` } }`),
	"$dataSource.consume":       extra(`data: { subscribers: { file: data/subscribers.csv, consume: ` + placeholder + ` } }`),
	"$dataSource.generate":      extra(`data: { orders: { generate: ` + placeholder + ` } }`),
	"$dataSource.seed":          extra(`data: { orders: { generate: { id: uuid }, seed: ` + placeholder + ` } }`),
	"$generatedField":           extra(`data: { orders: { generate: { id: ` + placeholder + ` } } }`),
	"$generatedField.type":      extra(`data: { orders: { generate: { id: { type: ` + placeholder + `, format: "ORD-######" } } } }`),
	"$generatedField.format":    extra(`data: { orders: { generate: { reference: { type: pattern, format: ` + placeholder + ` } } } }`),
	"$generatedField.newEvery":  extra(`data: { orders: { generate: { id: { type: uuid, newEvery: ` + placeholder + ` } } } }`),
	"$detailedCapture.from":     step(`http: GET /auth/token` + "\n    capture: { token: { from: " + placeholder + " } }"),
	"$detailedCapture.default":  step(`http: GET /auth/token` + "\n    capture: { token: { from: $.access_token, default: " + placeholder + " } }"),
	"$detailedCapture.required": step(`http: GET /auth/token` + "\n    capture: { token: { from: $.access_token, required: " + placeholder + " } }"),

	// passo
	"$step.name":      step(`http: GET /orders/1001` + "\n    name: " + placeholder),
	"$step.weight":    head + someLoad + "scenario:\n  - http: GET /orders/1001\n    name: look up order\n    weight: " + placeholder + "\n  - http: POST /orders\n    name: create order\n    weight: 40\n",
	"$step.capture":   step(`http: GET /orders/1001` + "\n    capture: " + placeholder),
	"$step.capture.*": step(`http: GET /orders/1001` + "\n    capture: { invoiceId: " + placeholder + " }"),
	"$step.expect":    step(`http: GET /orders/1001` + "\n    expect: " + placeholder),
	"$step.http":      step(`http: ` + placeholder),
	"$step.graphql":   step(`graphql: ` + placeholder),
	"$step.kafka":     step(`kafka: ` + placeholder),
	"$step.amqp":      step(`amqp: ` + placeholder),
	"$step.await":     step(`await: ` + placeholder),

	// http
	"$http.method":          step(`http: { method: ` + placeholder + `, path: /orders/1001 }`),
	"$http.path":            step("http: GET /orders/1001\n    capture: { invoiceId: $.lastInvoice.id }\n  - http: { method: GET, path: " + placeholder + " }"),
	"$http.url":             step(`http: { method: GET, url: ` + placeholder + ` }`),
	"$http.headers":         step(`http: { method: GET, path: /orders/1001, headers: ` + placeholder + ` }`),
	"$http.body":            step(`http: { method: POST, path: /orders, body: ` + placeholder + ` }`),
	"$http.timeout":         step(`http: { method: GET, path: /orders/1001, timeout: ` + placeholder + ` }`),
	"$http.followRedirects": step(`http: { method: GET, path: /orders/1001, followRedirects: ` + placeholder + ` }`),

	// graphql
	"$graphql.query":     step(`graphql: { query: ` + placeholder + ` }`),
	"$graphql.operation": step(`graphql: { query: "query LookUpOrder($id: ID!) { order(id: $id) { id } }", operation: ` + placeholder + `, variables: { id: "1001" } }`),
	"$graphql.variables": step(`graphql: { query: "query LookUpOrder($id: ID!) { order(id: $id) { id } }", variables: ` + placeholder + ` }`),
	"$graphql.path":      step(`graphql: { query: "query LookUpOrder { order { id } }", path: ` + placeholder + ` }`),
	"$graphql.url":       step(`graphql: { query: "query LookUpOrder { order { id } }", url: ` + placeholder + ` }`),
	"$graphql.headers":   step(`graphql: { query: "query LookUpOrder { order { id } }", headers: ` + placeholder + ` }`),
	"$graphql.timeout":   step(`graphql: { query: "query LookUpOrder { order { id } }", timeout: ` + placeholder + ` }`),

	// kafka
	"$kafka.topic":     step(`kafka: { topic: ` + placeholder + `, value: { order: "1001" } }`),
	"$kafka.brokers":   step(`kafka: { topic: orders, brokers: ` + placeholder + `, value: { order: "1001" } }`),
	"$kafka.key":       step(`kafka: { topic: orders, key: ` + placeholder + `, value: { order: "1001" } }`),
	"$kafka.value":     step(`kafka: { topic: orders, value: ` + placeholder + ` }`),
	"$kafka.headers":   step(`kafka: { topic: orders, value: { order: "1001" }, headers: ` + placeholder + ` }`),
	"$kafka.partition": step(`kafka: { topic: orders, partition: ` + placeholder + `, value: { order: "1001" } }`),
	"$kafka.group":     step(`kafka: { topic: orders, group: ` + placeholder + `, value: { order: "1001" } }`),
	"$kafka.acks":      step(`kafka: { topic: orders, acks: ` + placeholder + `, value: { order: "1001" } }`),
	"$kafka.timeout":   step(`kafka: { topic: orders, timeout: ` + placeholder + `, value: { order: "1001" } }`),

	// amqp
	"$amqp.url":        step(`amqp: { url: ` + placeholder + `, queue: orders, body: { order: "1001" } }`),
	"$amqp.queue":      step(`amqp: { queue: ` + placeholder + `, body: { order: "1001" } }`),
	"$amqp.exchange":   step(`amqp: { exchange: ` + placeholder + `, routingKey: orders.created, body: { order: "1001" } }`),
	"$amqp.routingKey": step(`amqp: { exchange: orders, routingKey: ` + placeholder + `, body: { order: "1001" } }`),
	"$amqp.messageId":  step(`amqp: { queue: orders, messageId: ` + placeholder + `, body: { order: "1001" } }`),
	"$amqp.body":       step(`amqp: { queue: orders, body: ` + placeholder + ` }`),
	"$amqp.headers":    step(`amqp: { queue: orders, body: { order: "1001" }, headers: ` + placeholder + ` }`),
	"$amqp.persistent": step(`amqp: { queue: orders, body: { order: "1001" }, persistent: ` + placeholder + ` }`),
	"$amqp.confirm":    step(`amqp: { queue: orders, body: { order: "1001" }, confirm: ` + placeholder + ` }`),
	"$amqp.timeout":    step(`amqp: { queue: orders, body: { order: "1001" }, timeout: ` + placeholder + ` }`),

	// await
	"$await.http":          step(`await: { http: ` + placeholder + `, until: { status: 200 } }`),
	"$await.http.path":     step(`await: { http: { path: ` + placeholder + ` }, until: { status: 200 } }`),
	"$await.http.url":      step(`await: { http: { url: ` + placeholder + ` }, until: { status: 200 } }`),
	"$await.kafka":         step(`await: { kafka: ` + placeholder + `, key: "1001" }`),
	"$await.kafka.topic":   step(`await: { kafka: { topic: ` + placeholder + ` }, key: "1001" }`),
	"$await.kafka.brokers": step(`await: { kafka: { topic: orders-processed, brokers: ` + placeholder + ` }, key: "1001" }`),
	"$await.amqp":          step(`await: { amqp: ` + placeholder + `, key: "1001" }`),
	"$await.amqp.queue":    step(`await: { amqp: { queue: ` + placeholder + ` }, key: "1001" }`),
	"$await.amqp.topic":    step(`await: { amqp: { topic: ` + placeholder + ` }, key: "1001" }`),
	"$await.amqp.url":      step(`await: { amqp: { queue: orders-processed, url: ` + placeholder + ` }, key: "1001" }`),
	"$await.key":           step(`await: { kafka: { topic: orders-processed }, key: ` + placeholder + ` }`),
	"$await.until":         step(`await: { http: { path: /orders/1001 }, until: ` + placeholder + ` }`),
	"$await.field":         step(`await: { kafka: { topic: orders-processed }, key: "1001", field: ` + placeholder + `, equals: PROCESSED }`),
	"$await.equals":        step(`await: { kafka: { topic: orders-processed }, key: "1001", field: $.status, equals: ` + placeholder + ` }`),
	"$await.interval":      step(`await: { http: { path: /orders/1001 }, until: { status: 200 }, interval: ` + placeholder + ` }`),
	"$await.timeout":       step(`await: { http: { path: /orders/1001 }, until: { status: 200 }, timeout: ` + placeholder + ` }`),

	// expect
	"$expect.status":       step(`http: GET /orders/1001` + "\n    expect: { status: " + placeholder + " }"),
	"$expect.json":         step(`http: GET /orders/1001` + "\n    expect: { json: " + placeholder + " }"),
	"$expect.bodyContains": step(`http: GET /orders/1001` + "\n    expect: { bodyContains: " + placeholder + " }"),
	"$expect.bodyMatches":  step(`http: GET /orders/1001` + "\n    expect: { bodyMatches: " + placeholder + " }"),
	"$expect.header":       step(`http: GET /orders/1001` + "\n    expect: { header: " + placeholder + " }"),

	// carga
	"$load.profiles":  head + "load:\n  profiles: " + placeholder + "\n" + someSteps,
	"$load.model":     head + "load:\n  model: " + placeholder + "\n  users: 10\n  duration: 1m\n" + someSteps,
	"$load.users":     head + "load:\n  model: closed\n  users: " + placeholder + "\n  duration: 1m\n" + someSteps,
	"$load.duration":  head + "load:\n  model: closed\n  users: 10\n  duration: " + placeholder + "\n" + someSteps,
	"$load.thinkTime": head + "load:\n  model: closed\n  users: 10\n  duration: 1m\n  thinkTime: " + placeholder + "\n" + someSteps,

	"$profile.ramp":            head + "load:\n  profiles:\n    - ramp: " + placeholder + "\n" + someSteps,
	"$profile.steady":          head + "load:\n  profiles:\n    - steady: " + placeholder + "\n" + someSteps,
	"$profile.spike":           head + "load:\n  profiles:\n    - spike: " + placeholder + "\n" + someSteps,
	"$profile.ramp.from":       head + "load:\n  profiles:\n    - ramp: { from: " + placeholder + ", to: 800/s, duration: 5s }\n" + someSteps,
	"$profile.ramp.to":         head + "load:\n  profiles:\n    - ramp: { from: 100/s, to: " + placeholder + ", duration: 5s }\n" + someSteps,
	"$profile.ramp.duration":   head + "load:\n  profiles:\n    - ramp: { from: 100/s, to: 800/s, duration: " + placeholder + " }\n" + someSteps,
	"$profile.steady.rate":     head + "load:\n  profiles:\n    - steady: { rate: " + placeholder + ", duration: 1m }\n" + someSteps,
	"$profile.steady.duration": head + "load:\n  profiles:\n    - steady: { rate: 100/s, duration: " + placeholder + " }\n" + someSteps,
	"$profile.spike.rate":      head + "load:\n  profiles:\n    - spike: { rate: " + placeholder + ", duration: 3s }\n" + someSteps,
	"$profile.spike.duration":  head + "load:\n  profiles:\n    - spike: { rate: 2000/s, duration: " + placeholder + " }\n" + someSteps,
	"$rate":                    head + "load:\n  profiles:\n    - steady: { rate: " + placeholder + ", duration: 1m }\n" + someSteps,
	"$duration":                head + "load:\n  profiles:\n    - steady: { rate: 100/s, duration: " + placeholder + " }\n" + someSteps,

	// slo
	"$limit":           extra(`slo:` + "\n  - look up order: { p95: " + placeholder + " }"),
	"$sloRule.*.p50":   extra(`slo:` + "\n  - look up order: { p50: " + placeholder + " }"),
	"$sloRule.*.p75":   extra(`slo:` + "\n  - look up order: { p75: " + placeholder + " }"),
	"$sloRule.*.p90":   extra(`slo:` + "\n  - look up order: { p90: " + placeholder + " }"),
	"$sloRule.*.p95":   extra(`slo:` + "\n  - look up order: { p95: " + placeholder + " }"),
	"$sloRule.*.p99":   extra(`slo:` + "\n  - look up order: { p99: " + placeholder + " }"),
	"$sloRule.*.p99.9": extra(`slo:` + "\n  - look up order: { p99.9: " + placeholder + " }"),
	"$sloRule.*.max":   extra(`slo:` + "\n  - look up order: { max: " + placeholder + " }"),
}
