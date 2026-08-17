# Recipes

Situations that show up almost every time, and what to write in the scenario for
each one.

## The endpoint requires a login

Declare once how the token is obtained. braunrate logs in before it starts
measuring and injects the token into every step; no step needs to declare the
header.

```yaml fragment
auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: "${user}", password: "${PASSWORD}" } }
    capture: { token: $.access_token }
  refreshAfter: 25m
```

The password comes from an environment variable. If you write the value into the
file, validation refuses it and explains how to do it right — the scenario goes
into the repository, and the repository keeps it forever.

Two things to know before believing the number:

- the login happens in the preparation, off the clock. If it entered the
  measurement, the first request would carry the cost for all of them.
- **it is one token for the whole run.** If the target has caching by identity, a
  limit per token or sharding by user, the result comes out optimistic. The
  report repeats this every time.

Full file: [`examples/authenticated-journey.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/authenticated-journey.yaml).

### Header, query string and path

`obtain` is a whole request: method, path with a query string, headers and body.
And everything the capture produces becomes a variable in the steps that follow,
so the token goes wherever you write `${token}` — header, query or path.

```yaml fragment
auth:
  type: token
  header: "Authorization: Bearer ${token}"
  obtain:
    http:
      method: POST
      path: /auth/token?origin=braunrate&tenant=${TENANT}
      headers: { X-Client: load-test }
      body: { user: ana, password: "${PASSWORD}" }
    capture: { token: $.access_token }

scenario:
  - http:
      method: POST
      path: /invoices/f-${orders.id}/pay?tk=${token}&tenant=${TENANT}
      headers: { X-Correlation: "${orders.id}" }
    name: pay invoice
```

`braunrate debug` shows what went out, with the secret cut:

```
step 1 — pay invoice   [ok in 5.9ms]
  request:    POST /invoices/f-3958/pay?tk=test-token&tenant=acme
              Authorization: Bearer test-t… (10 characters)
              X-Correlation: 3958
  response:   status 200, 63 bytes
```

> **Note** The automatic injection is always a header, and `header:` takes any
> `Name: value` — the default is `Authorization: Bearer ${token}`. A token in the
> query or in the path is something you write yourself, with `${token}` in the
> step.

Two things that save a round trip:

- **Inside `obtain` only the environment and fixed values fit.** The login
  happens once, before the journeys, so `${orders.id}` does not exist there — and
  validation refuses instead of sending the request with an empty field.
- **`${TENANT}` in upper case comes from the environment with nothing declared.**
  A lower-case name needs `variables`, `data` or `capture`.

### An API key, with no login

When there is no login request — the credential is already in the environment —
the block is one line:

```yaml fragment
auth: { type: header, header: "X-API-Key: ${API_KEY}" }
```

The header goes on every step, and the output shows that it was sent without
showing the value:

```
step 1 — look up order   [ok in 7.4ms]
  request:    GET /orders/1?origin=braunrate
              X-API-Key: ***
```

## Every journey needs data of its own

Two sources: a CSV you already have, or generated values.

```yaml fragment
data:
  subscribers: { file: data/subscribers.csv, consume: circular }
  orders: { generate: { id: uuid, amount: "number(10,500)" } }
```

The CSV becomes `${subscribers.column}`; the generator becomes `${orders.id}`.
Available generators: `uuid`, `sequence`, `number(min,max)`, `integer(min,max)`,
`name`, `email`, `text(n)`, `pattern`, `cpf`, `cnpj`. CPF and CNPJ come out with a
valid check digit, otherwise the target would refuse everything and the test would
measure the path of the refusal.

**One value per journey, not per request.** That is what an idempotency key
needs: the same `transactionId` on both calls of the same journey, a new one on
the next journey. That is the default, with nothing declared:

```yaml fragment
data:
  payment:
    generate: { transactionId: uuid }

scenario:
  - http: { method: POST, path: /orders, headers: { X-Idempotency-Key: "${payment.transactionId}" } }
  - http: { method: GET, path: "/orders/${orderId}", headers: { X-Idempotency-Key: "${payment.transactionId}" } }
```

When the value has to be new on every use, declare it:

```yaml fragment
data:
  keys:
    generate:
      nonce: { type: uuid, newEvery: use }
```

And, if the target requires a specific shape, `pattern` builds it: `#` becomes a
digit, `@` becomes a letter, everything else comes out as it is.

```yaml fragment
data:
  registration:
    generate:
      reference: { type: pattern, format: "ORD-######" }   # ORD-481902
      branch:    { type: pattern, format: "@@-####" }      # KQ-3718
```

There is no `.xlsx` reading. CSV covers the case, and pulling an Excel dependency
into the load engine costs more than it solves.

### Repeating the same run, or varying on purpose

A fixed seed in the file makes CI always run the same case, and a case that
passes a thousand times proves nothing after the first. The seed takes the
environment:

```yaml fragment
data:
  orders:
    generate: { id: uuid, amount: "number(10,500)" }
    seed: ${SEED:-42}
```

Without the variable, it runs with 42 and nothing changes from one day to the
next. With it, the report publishes the seed that ran and the line that brings
the case back:

```
  Data seeds: orders=8817 (from $SEED) (the same seed generates the same values again)
  To repeat exactly this data, run again with SEED=8817
```

## The test has to exercise several routes

Production is not the same call a thousand times. `weight` splits the rate among
alternatives, and each iteration runs one of them:

```yaml fragment
scenario:
  - name: look up order
    weight: 60
    http: { method: GET, path: "/orders/${orders.id}" }
  - name: create order
    weight: 10
    http: { method: POST, path: /orders }
```

The choice is by position in the cycle, not a draw, so two runs of the same file
apply exactly the same mix. The report shows the proportion that came out next to
the declared one:

```
Mix declared and observed
  look up order                60.0% declared     60.0% observed (300 of 500)
  look up invoice              30.0% declared     30.0% observed (150 of 500)
  create order                 10.0% declared     10.0% observed (50 of 500)
```

`weight` chooses which alternative runs, not which step inside a journey. A
scenario with chained captures is a single journey, and validation refuses
`weight` in it.

Full file: [`examples/operation-mix.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/operation-mix.yaml).

### When each customer profile follows a different route

There is no conditional step. Which path each profile walks, and in what
proportion, is business knowledge — no traffic observation reveals it, and
whoever recorded one pass recorded one profile. So the branching goes through the
data: the CSV declares the proportion by the lines it has.

```yaml fragment
data:
  customers: { file: data/customers.csv, consume: circular }

scenario:
  - name: look up limit
    http: { method: GET, path: "/${customers.route}/${customers.id}/limit" }
```

What this shape does not solve yet: both profiles land on a single line of the
report, so an expensive profile shows up as the tail of the whole step.

Full file: [`examples/branching-by-profile.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/branching-by-profile.yaml).

## Where braunrate takes each `${variable}` from

From four places, and from no other:

```yaml fragment
variables:
  tenant: acme                      # fixed in the scenario
  region: "${REGION:-us-east-1}"    # from the environment, with a fallback

data:
  orders: { generate: { id: uuid } }  # and then ${orders.id}

scenario:
  - http: GET /invoices
    capture: { invoiceId: "$.items[0].id" }   # and then ${invoiceId} in the steps that follow
```

A name in UPPER CASE comes from the environment with nothing declared:
`${API_KEY}`, `${KAFKA_PASSWORD}`. It is the same shape `import curl` and `record`
write on their own.

It works in any field of the scenario, not in a chosen list of fields: target,
rate, duration, topic, queue, step name, header, certificate path.

```yaml fragment
target: "${TARGET:-http://127.0.0.1:8080}"
load:
  profiles:
    - steady: { rate: "${RATE:-100}/s", duration: "${DURATION:-1m}" }
```

Two common traps:

- **inside `{ }` the value needs quotes.** YAML reads `{`, `}` and `[` as
  structure, so `path: /orders/${id}` inside an inline map does not load.
- **with no fallback value, validation complains** instead of leaving the field
  empty. Either define the variable, or write `${RATE:-100}`.

## Make the test fail the build

```bash
braunrate execute scenario.yaml -quiet -result=output.json
```

The exit code is enough, there is nothing to read: `0` passed, `1` the acceptance
criterion failed, `2` the scenario file has an error, `3` the run did not measure
what it set out to measure.

If the scenario depends on infrastructure that is not always there, declare it —
the examples loop skips with a visible warning instead of breaking:

```yaml fragment
requires: [kafka]
```

Full file: [`examples/ci.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/ci.yaml).

## Find out whether it got slower than yesterday

Keep the result of one run and pass it as the baseline of the next:

```bash
braunrate execute scenario.yaml -baseline=previous-run.json
```

With a `regression` rule declared, the per-step criterion keeps approving and the
comparison catches what it does not see:

```
  ok    Passed: "look up order" answered 95% within 61 ms, within the limit of 150 ms.
  FAIL  Failed: the response time of 95% of the journeys came out 931.0% worse than previous-run.json, above the limit of 10% worse (from 12 ms to 122 ms).
```

When something outside the service changed between the two — another machine,
another version of braunrate, another arrival model — the rule **does not fail**,
and it says why. Blaming the service for a difference the comparison cannot
attribute to it would be worse than not comparing.

Outside the gate, to look at it calmly:

```bash
braunrate compare before.json after.json -html comparison.html
```

```
It got slower: the whole journey (95%): 215 times slower — from 11 ms to 2435 ms. With 1 caveat about what changed outside the service.

Per step
  step                        95% before   95% after           change
  look up order                   5.8 ms      2.41 s       420x worse
  pay invoice                     5.6 ms       11 ms       2.1x worse

What could explain the difference other than the service
  - both runs used one token for everything; caching or sharding by identity affects them the same way, but it does not disappear from the comparison
  Two runs give no confidence interval: a change below 5% is treated as noise.
```

## When YAML is not enough

A loop over a list, a decision in the middle of the journey, data coming from a
system of yours. The same scenario is written in Go, and runs on the same engine:

```go
// Scenario is the same journey of examples/authenticated-journey.yaml, written
// in Go: same engine, same metrics, same result document.
func Scenario(target string) (braunrate.Scenario, error) {
	return dsl.New("Billing journey").
		Target(target).
		Auth(dsl.WithToken(
			dsl.POST("/auth/token").Body(map[string]any{"user": "ana", "password": "${PASSWORD:-secret}"}),
			dsl.Capture("token", "$.access_token"),
		).RefreshAfter(25*time.Minute)).
		DataFromFile("subscribers", "data/subscribers.csv", dsl.Consume(dsl.Circular)).
		Ramp(dsl.PerSecond(50), dsl.PerSecond(300), 5*time.Second).
		Steady(dsl.PerSecond(300), 5*time.Second).
		Step(dsl.GET("/orders/${subscribers.id}"),
			dsl.Name("look up order"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.lastInvoice.status", "OPEN"),
			dsl.Capture("invoiceId", "$.lastInvoice.id")).
		Step(dsl.POST("/invoices/${invoiceId}/pay").
			Body(map[string]any{"amount": 199.90}),
			dsl.Name("pay invoice"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.status", "PAID")).
		SLO("look up order", "p95", "< 150ms").
		SLO("pay invoice", "p95", "< 200ms").
		JourneySLO("p95", "< 2s").
		JourneySLO("p99", "< 5s").
		OverallSLO("errors", "< 0.1").
		Build()
}
```

This snippet is not an illustration: it is the file
[`examples/scenario-in-go/scenario.go`](https://github.com/Diegobraun/braunrate/blob/main/examples/scenario-in-go/scenario.go),
which CI compiles and runs against the built-in target. A test fails the build if
this page drifts away from the file.

Moving is not rewriting. The DSL interprets nothing on its own:
`"$.lastInvoice.id"` and `"< 150ms"` go through the same functions that read the
YAML, and a test compares the structure of both paths case by case.

What still requires a change in this repository is a new protocol. And, until
v1, the public types are not frozen.
