# braunrate

```hero
motto: Load testing that does not lie about its own result.
summary: When the system freezes, most tools stop measuring along with it — and the report comes out looking good. braunrate keeps measuring, and shows what happened.
command: braunrate demo
action: Download | installation.html
action: See it on GitHub | https://github.com/Diegobraun/braunrate
facts: single binary | no runtime to install | scenario in YAML, kept in version control
proof: Same service. Same 1-second freeze.
side: Closed-loop tool | 3.5 ms | "everything is fine"
side: braunrate | 972.3 ms | "the user waited 972 ms"
balance: 968.8 ms the other tool never counted.
```

## Start

If you have never run a load test, the shortest path is one command, with no file
to write first:

```bash
braunrate demo
```

No binary yet? [Installation](installation.html) is downloading one file and
unpacking it; there is no runtime to install.

The demo brings up a fake service on your machine, runs a scenario against it and
explains every number as it appears. After that,
[First 15 minutes](first-15-minutes.html) goes from nothing to the first report
of your own service.

| If you want to | Go to |
|---|---|
| see the tool working right now | `braunrate demo` |
| measure your own service | [First 15 minutes](first-15-minutes.html) |
| understand what the numbers mean | [Concepts](concepts.html) |
| write the scenario | [Scenario reference](reference.html) |
| fix an error on the screen | [Troubleshooting](troubleshooting.html) |

## Three pieces of evidence

The three measurements below are real runs, and each one exposes a different way
a load test can produce a number that does not describe the system.

| Evidence | Number | What stays hidden |
|---|---|---|
| Target frozen for 1 s | 972.3 ms against 3.5 ms | **Coordinated omission**: a closed loop stops sending when the target freezes, and the wait disappears from the count |
| [GraphQL with an error inside a 200](protocols.html#graphql) | 406 errors across 2,844 responses, every one of them status 200 | **Errors classified by status**: whoever reads the HTTP code reports 0% errors and a green acceptance criterion |
| [Asynchronous chain](protocols.html#kafka-and-rabbitmq) | 1.2 ms to produce against 3.96 s of journey | **Measuring only the produce**: the broker accepts fast, and the effect the user is waiting for arrives seconds later |

### The first one, in detail

The embedded test target freezes for 1 s in the middle of the run. Same pause,
same target, two models of measurement:

| Model | 99% of the responses within | Samples |
|---|---|---|
| **braunrate (open arrival, time counted from the scheduled instant)** | **972.3 ms** | 600 |
| Closed loop (one virtual user in sequence, the way JMeter and Locust measure) | 3.5 ms | 730 |

That is 968.8 ms hidden by the closed loop. It is not a defect in that tool: when
the target freezes it simply stops sending, and the requests that should have
gone out never enter the count. That is coordinated omission.

The comparison is an automated test that runs in CI on every push. If the
closed loop ever stops hiding the pause, or braunrate stops showing it, the
build breaks:

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    same 1s freeze against the same target:
      open model (braunrate): p99 972.3 ms over 600 samples
      closed loop:            p99 3.5 ms over 730 samples
      coordinated omission: 968.8 ms the closed loop never counted
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

To watch the same thing happen on your machine, without cloning the repository:

```bash
braunrate demo --with-failure
```

## Who it is for

**QA who do not program.** The common case is a ten-line YAML file, with a
default for everything that was not declared. There is always a way in — import a
`curl`, record while browsing, start from an example — because a blank page is
the reason nobody builds a scenario from scratch.

**Teams that keep the test in version control next to the service.** The scenario
is a text file that diffs readably and merges, the same engine runs in the
terminal and in CI, and the acceptance criterion becomes an exit code with no
glue in between.

## Principles

1. **The wait is counted, not dropped.** Open arrival model; response time counted
   from the instant the request *should* have gone out; HDR histogram; an explicit
   warning when the generator did not sustain the rate. Coordinated omission is
   the failure that lets a test pass with 99% within 47 ms while production
   suffers 1.8 s.
2. **Two audiences, one engine.** Declarative YAML for the common case, a Go DSL
   for the complex one — same engine, same metrics, no rewrite when you move.
3. **A business scenario, not just a request.** GraphQL measured per operation;
   Kafka and RabbitMQ with a metric model of their own; an `await` step to measure
   the asynchronous chain end to end.

## Scope

**Inside.** HTTP/HTTPS and REST; GraphQL as a first-class citizen; Kafka and
RabbitMQ (produce and consume); an `await` step with a timeout; correlation,
variables and an authentication flow; CSV with a consumption policy and synthetic
generation with a seed; load profiles and a declared closed model; an acceptance
criterion with an exit code; a self-contained HTML report, JSON, CSV and a
terminal summary; comparison between runs; a `.jmx` importer for the common
subset; an HTTP traffic recorder; a local server mode with no logic of its own;
broker authentication, always with the credential outside the file.

**Outside.** A real browser engine; a managed cloud, a multi-user dashboard, a
team account; scheduling and persistence beyond the files; LDAP, FTP, SMTP,
classic JMS; competing on raw throughput with wrk; distributed execution in v1.

**Known limitations**, with the reason in [Decisions](decisions.html): a protocol
outside the list requires a change in this repository; a single token for the
whole run; and the time of the steps after the first is service time, not
corrected time — the reading that includes the wait is in the "The whole
journey" block.
