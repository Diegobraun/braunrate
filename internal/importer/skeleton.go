package importer

import "strings"

// Skeleton exists because from an empty folder there was no path to a first
// scenario: every command takes a file and none created one. The comments in
// it are for someone reading this YAML for the first time, which is why they
// show the shape of the blocks nearly every scenario needs.
func Skeleton() string {
	return `# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
# With the editor's YAML extension, the line above turns on key autocomplete.

name: My first scenario
target: http://127.0.0.1:8080

# Arrival rate, in requests per second. Not a number of users: the generator
# fires on schedule even when the target is slow, which is what keeps the
# measurement from hiding a freeze.
load:
  profiles:
    - ramp: { from: 1/s, to: 20/s, duration: 30s }
    - steady: { rate: 20/s, duration: 1m }

scenario:
  - http: GET /orders/1
    name: look up order
    expect: { status: 200 }

# Acceptance criterion. Without this block the run reports and never fails.
slo:
  - look up order: { p95: < 200ms }
  - global: { errors: < 1 }

# Data that varies per iteration. A fixed value makes the target answer from
# cache and the number comes out optimistic; the report states the variety that
# actually happened.
# data:
#   subscribers: { file: subscribers.csv, consume: circular }
#
# scenario:
#   - http: GET /orders/${subscribers.id}

# Log in once, token reused by the iterations that follow.
# auth:
#   type: token
#   obtain:
#     http: { method: POST, path: /auth/token, body: { user: ana, password: "${PASSWORD}" } }
#     capture: { token: $.access_token }

# A step does not have to be HTTP. These are the other protocols, and every
# report lists which ones exist in the binary you are running.
` + commented(ProtocolShapes())
}

// ProtocolShapes is what the skeleton shows commented out. It lives apart from
// the text so a test can put it through the parser: a shape that does not parse
// teaches the wrong thing to exactly the person who has no other reference.
func ProtocolShapes() string {
	return `scenario:
  - graphql:
      query: |
        query LookUpOrder($id: ID!) { order(id: $id) { status } }
      variables: { id: "${subscribers.id}" }

  - kafka: { topic: orders, key: "${subscribers.id}", value: { order: "${subscribers.id}" } }
  - amqp:  { queue: orders, body: { order: "${subscribers.id}" } }

  # Waits for the effect of the previous step to show up. Its latency has the
  # granularity of the polling, and the report says so.
  - await:
      kafka: { topic: orders-processed }
      key: "${subscribers.id}"
      timeout: 10s
`
}

func commented(text string) string {
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			out.WriteString("#\n")
			continue
		}
		out.WriteString("# " + line + "\n")
	}
	return out.String()
}
