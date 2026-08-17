# Commands

`braunrate` with no argument shows the path; `braunrate help` lists everything.
Every option takes `-h`, and an option written wrong gets the right one back:

```
$ braunrate target -addr :8080
"-addr" does not exist. Did you mean "-address"?

    braunrate target -address :8080

Every option: braunrate target -h
```

| Command | What for |
|---|---|
| [`demo`](#demo) | see the tool working without preparing anything |
| [`new`](#new) | write a scenario skeleton |
| [`migrate`](#migrate) | convert a scenario in the Portuguese format |
| [`import`](#import) | start from a `curl` or a JMeter plan |
| [`record`](#record) | record the flow while browsing |
| [`validate`](#validate) | check the file without running it |
| [`debug`](#debug) | run one iteration and see what happens |
| [`execute`](#execute) | run it with load |
| [`report`](#report) | generate HTML or CSV from a result already saved |
| [`compare`](#compare) | compare two runs |
| [`serve`](#serve) | expose the CLI as local HTTP |
| [`ui`](#ui) | edit and run the scenarios in the browser |
| [`target`](#target) | bring up the built-in test target |
| [`version`](#version) | version, commit and compiled protocols |

## `demo`

```bash
braunrate demo
braunrate demo --with-failure
```

Brings up the built-in target, writes the scenario it is going to run, executes
it and explains every number. It needs no file, no target and no second terminal.
`--with-failure` runs against a target that freezes in the middle and measures the
same freeze two ways.

It leaves `demo.yaml` and `demo-report.html` in the current directory, and says
that it did.

## `new`

```bash
braunrate new scenario.yaml
```

Writes a commented skeleton, and never overwrites an existing file. It is the
rare path: importing a `curl` is almost always better.

## `migrate`

```bash
braunrate migrate scenario.yaml
braunrate migrate ./scenarios -dry-run
braunrate migrate scenario.yaml -output scenario-en.yaml
```

Converts a scenario written in the Portuguese format, replaced in 0.6.0, keeping
comments and the order of the keys. It lists line by line what changed, leaves
the original as `.bak`, and refuses a file that has already been converted. No
behaviour changes: it is a rename.

## `import`

```bash
braunrate import curl "curl 'https://api.example.com/v1/orders/9912' -X POST -H 'Authorization: Bearer abc.def' -d '{\"amount\": 199.90}'" -output scenario.yaml
pbpaste | braunrate import curl
braunrate import jmx plan.jmx -output scenario.yaml
```

Out of the `curl` comes a scenario that already loads, with a starting load and
acceptance criterion, and three warnings:

```
scenario written to scenario.yaml
warning: the path has a fixed value: with a single value the target answers from cache and the number comes out optimistic. Swap it for ${data.column} and declare a 'data' block
warning: the header Authorization became ${token}: run with TOKEN=... in the environment, so a credential does not get versioned
warning: the load and slo numbers are a starting guess, not a measurement: tune them before using this as a gate

Next step, before any load:
  braunrate debug scenario.yaml
```

Out of the `.jmx` the translation is partial, and what was left out is listed in
the terminal. An importer that swallows the file quietly hands over a scenario
that measures something else:

| Translated | Not translated (comes out declared) |
|---|---|
| `HTTPSamplerProxy` (method, path, domain, body) | Controllers (If, While, Loop), timers |
| `HeaderManager`, with the credential becoming an environment variable | JSR223/BeanShell scripts |
| `CSVDataSet` (file and recycling) | JDBC, JMS and other non-HTTP samplers |
| `ThreadGroup`, as a **warning**, never as a rate | Assertions: every step comes out with `status: 200` |
| `JSONPostProcessor` and `RegexExtractor`, as a capture instruction | JMeter `${__...}` functions |

> **Important** A thread never becomes a rate. In JMeter a thread only sends
> after the previous response arrived: 50 threads are 50/s if the target answers
> in 1 s and 5/s if it answers in 10 s. Converting quietly would import
> coordinated omission along with the plan.

```
warning: the group "Users" declares 50 threads, ramp of 30s, 300s of duration: a thread count does not
turn into an arrival rate, because a thread only sends after the previous response. The 'load' block
came out as a guess; swap it for the rate you want to sustain (requests per second)
```

## `record`

```bash
braunrate record -output scenario.yaml
# point the browser or curl at the proxy, walk the flow, Ctrl+C
```

The JMeter recorder transcribes: it records the token of that session and order
`9912`, and on the second run the scenario breaks. This one does four more
things, and declares each of them:

```
dropped 1 an outside domain (example.com)
dropped 1 a static asset
3 requests became 2 steps in scenario.yaml
2 observed value(s) of orders_id in scenario-orders-id.csv
warning: the field "password" of the body became ${password}: run with PASSWORD=... in the environment, so a credential does not get versioned
warning: the recorded sequence is a single pass: the production mix has other proportions between the routes
warning: the load and slo numbers are a starting guess, not a measurement: tune them before using this as a gate

Next step, before any load:
  braunrate debug scenario.yaml
```

> **Note** Recording inside HTTPS requires braunrate to issue a certificate and
> your machine to trust it, and touching the system trust store is not something
> a load tool should automate quietly. The connection is forwarded so the client
> keeps working, and what was not recorded shows up on screen by host. Mobile app
> traffic is out of v1, because of certificate pinning.

## `validate`

```bash
braunrate validate scenario.yaml
```

Reads and checks without running anything. It says how many iterations the
scenario would produce, warns about what you did not declare, and points at the
next step:

```
Valid scenario: "Journey with new criteria", 2 steps, 500 iterations in 5s.
Warning: the gate measures 2 isolated steps and leaves out the whole journey, which is the wait the user feels.
    declare it too:  - journey: { p95: < 2s, p99: < 5s }

Before running the load, check that the scenario does what you expect:
  braunrate debug scenario.yaml
```

## `debug`

```bash
braunrate debug scenario.yaml
```

One user, one iteration, no load. It shows the request, the response, the
capture, the variables and where it stopped. It is where a broken correlation
shows up, before the ten minutes of load and not after:

```
$ braunrate debug examples/authenticated-journey.yaml
debugging "Billing journey" against http://127.0.0.1:8080: 1 user, 1 iteration, no load

step 1 — look up order   [ok in 6.8ms]
  request:    GET /orders/1001
              Authorization: Bearer test-t… (10 characters)
  response:   status 200, 91 bytes
  body:       {"id":"1001","status":"OPEN","lastInvoice":{"id":"f-1001","amount":199.90,"status":"OPEN"}}
  captured:
    invoiceId = f-1001

step 2 — pay invoice   [ok in 6.1ms]
  request:    POST /invoices/f-1001/pay
              Authorization: Bearer test-t… (10 characters)
  response:   status 200, 63 bytes
  body:       {"id":"f-1001","status":"PAID","paidAt":"2026-08-15T00:00:00Z"}

variables at the end of the iteration
  invoiceId = f-1001
  token = test-token

Iteration complete: 2 steps, all good. To run it with load:
  braunrate execute examples/authenticated-journey.yaml
```

## `execute`

```bash
braunrate execute scenario.yaml
braunrate execute scenario.yaml -html=report.html -result=output.json -csv=steps.csv
braunrate execute scenario.yaml -baseline=previous-run.json
braunrate execute scenario.yaml -quiet
```

| Option | What it does |
|---|---|
| `-result <file.json>` | writes the result document, which is what `compare` and `report` read afterwards |
| `-html <file.html>` | a self-contained report, which opens on a closed network and survives attached to a ticket |
| `-csv <file.csv>` | one line per step, for a spreadsheet |
| `-baseline <file.json>` | a previous run, for the `regression` rules |
| `-max-concurrent <n>` | maximum simultaneous requests before it gives up firing |
| `-late-threshold <dur>` | past this the generator counts as not having sustained the rate |
| `-quiet` | prints neither progress nor the next-step hint |

## `report`

```bash
braunrate report output.json -html report.html
braunrate report output.json -csv steps.csv
```

Generates the output from a result already saved, without running anything again.

## `compare`

```bash
braunrate compare before.json after.json
braunrate compare before.json after.json -html comparison.html
```

It says what changed, lists everything that changed outside the service (machine,
load plan, version, scenario) and refuses to compare when either run does not
hold as a measurement. Exit code `3` when no verdict is possible.

## `serve`

```bash
braunrate serve -addr 127.0.0.1:8080 -dir ./scenarios
```

```
braunrate serve at http://127.0.0.1:8080, serving scenarios from ./scenarios
No authentication and no TLS: anyone who reaches this port can fire load at the targets of the scenarios.
It was made to run on 127.0.0.1. Exposing it on another interface is a separate decision, and it has not been made.

To see what it is serving:
  curl http://127.0.0.1:8080/scenarios
```

Validate, debug, execute, follow, list, fetch the JSON and the HTML, compare two
runs: what the CLI already does, and nothing beyond that. Every route ends in the
same code the terminal uses, and a test fails the build if the two stop producing
the same document.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/runs
curl -sN http://127.0.0.1:8080/runs/r001/stream
```

> **Warning** It is one run at a time, by default. Two runs on the same machine
> fight over the CPU that has to dispatch at the scheduled instant, and neither
> of them measures what it set out to measure. The second one answers `409` and
> says how to accept the contamination.

The YAML is still the truth. There is no database: the scenarios are the files in
`-dir`, and the runs live in the memory of the process.

## `ui`

```bash
braunrate ui -dir ./scenarios
braunrate ui -addr 127.0.0.1:8080 -dir ./scenarios -open=false
```

```
braunrate ui at http://127.0.0.1:8080, editing the scenarios in ./scenarios
No authentication and no TLS: anyone who reaches this port can fire load at the targets of the scenarios.
It was made to run on 127.0.0.1. Exposing it on another interface is a separate decision, and it has not been made.
Writing enabled: whoever reaches this port can change the scenario files in ./scenarios.

Open it in the browser:
  http://127.0.0.1:8080
```

It opens in the browser the same thing the CLI does: it lists the scenarios of
the directory, edits the file, validates while you type, runs one iteration,
executes with load and shows the report. The equivalent terminal command sits at
the top of the screen, on every screen.

> **Important** The interface is an editor of the file, not a form that generates
> one. The text you save is what the terminal reads, with the comments you wrote,
> and editing the file from outside keeps working. The reason is in
> [ADR 0018](decisions.html).

`-open=false` does not open the browser by itself, for whoever runs it on a
remote terminal or does not want the window to come up.

## `target`

```bash
braunrate target -latency=5ms
braunrate target -freeze-after=2s -freeze-for=2s
braunrate target -raw
braunrate target -kafka=127.0.0.1:9092 -input=orders -output=orders-processed
```

The built-in test target, for whoever does not have a service to point at yet.
`-raw` brings up a minimal target that answers without interpreting the request,
to measure the ceiling of the generator; measuring the ceiling against the full
target would measure the generator+target pair.

| Route | Credential |
|---|---|
| `/orders`, `/orders/{id}` | does not ask: it is the step `braunrate new` writes, and it runs on the first try |
| `/invoices/{id}`, `/graphql` | ask for a token obtained at `POST /auth/token` |
| `/auth/token`, `/health` | do not ask |

> **Tip** To exercise the authenticated journey, point a step at `/invoices` and
> declare the `auth` block — that is what the examples in the repository do.

## `version`

```bash
braunrate version
```

```
braunrate 0.6.0
commit: e1dca9c279e1ec653ea52cec1f4325e04ec21599
date: 2026-08-17T02:01:38Z
compiled protocols: [amqp await graphql http kafka]
```

The compiled protocols show up because two binaries with the same version could
produce different results with no trace of the reason.
