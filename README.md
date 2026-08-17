# braunrate

Load testing with an honest measurement: open arrival model, HDR histogram and
back-pressure detection.

**Documentation: <https://diegobraun.github.io/braunrate/>** — the site is
generated from `docs/guides/` in this repository, and every code block in it goes
through the test suite.

## See it working

```bash
braunrate demo                  # starts a target, runs a scenario and explains every number
braunrate demo --with-failure   # the same, against a target that freezes halfway
```

One command, one terminal, no file first. `braunrate` with no argument says what
the next command is; `braunrate help` lists them all.

## The measurement, in one comparison

The built-in test target freezes for 1 s in the middle of the run. Same pause,
same target, two measurement models:

| Model | 99% of the responses within | Samples |
|---|---|---|
| **braunrate (open arrival, time counted from the scheduled instant)** | **982.0 ms** | 600 |
| Closed loop (one virtual user in sequence, the way JMeter and Locust measure) | 3.7 ms | 728 |

That is 978.3 ms the closed loop never counted. It is not wrong because of a
bug: when the target freezes it simply stops sending, and the requests that
should have gone out never enter the count. That is coordinated omission.

The comparison is an automated test that runs in CI on every push. If the
measurement stops being honest, the build breaks.

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    same 1s freeze against the same target:
      open model (braunrate): p99 982.0 ms over 600 samples
      closed loop:            p99 3.7 ms over 728 samples
      coordinated omission: 978.3 ms the closed loop never counted
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

There are three proofs, each exposing a different blind spot: the freeze above;
the GraphQL that answers an error with status 200; and the asynchronous chain
that costs milliseconds to produce and seconds end to end. All three are on the
[front page of the site](https://diegobraun.github.io/braunrate/), with the real
output of each.

## Installing

Download the binary from the
[release](https://github.com/Diegobraun/braunrate/releases), check the checksum
and run it. There is no runtime to install.

```bash
go install github.com/Diegobraun/braunrate/cmd/braunrate@latest   # if you already have Go
```

The three paths, the first-run warnings on macOS and Windows, the platform table
and what is left out are in
[Installation](https://diegobraun.github.io/braunrate/installation.html).

## Language

The scenario format, the commands and everything the tool prints are in English,
with no language selection: a message that can be switched is a commitment to
keep every sentence in step forever, and a keyed catalogue moves the sentence
away from the code that decides to print it.

Translating the messages is possible, and it will be done if there is demand.
Open an issue saying which language and which surface — terminal, report or web
interface.

The documentation is bilingual: English at
<https://diegobraun.github.io/braunrate/> and Portuguese at
<https://diegobraun.github.io/braunrate/pt-BR/>. The English page is the source;
the Portuguese one declares which version of it was translated, and says on the
page itself when it is behind.

## State

**Phase 8 complete.** Open arrival engine, HTTP, GraphQL, Kafka, RabbitMQ and the
`await` step, correlation, authentication, data, assertions, acceptance criteria
with an exit code, authoring tools (schema in the editor, `debug`, `import curl`,
`import jmx` and `record`), reporting (self-contained HTML, JSON, CSV, comparison
between runs), observed variety, a Go scenario equivalent to the YAML one and
locked by a test, executable from a module outside this one, the closed model
declared, broker authentication with the credential outside the file, and a local
server mode with no logic of its own.

The Phase 0 decision was **Go**, held up by two criteria only: RSS under load
(30 MB against 597 MB for Java with G1, at 10,000/s) and a single static binary,
which for the QA audience means installing by downloading one file. Numbers,
methodology and limits in [medicoes-fase0.md](docs/medicoes-fase0.md); the
decision with the weight of each criterion in
[ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Development

```bash
go build -o braunrate ./cmd/braunrate
go test ./...
go run ./cmd/site -out site      # generates the published documentation
```

The site content lives in [`docs/guides/`](docs/guides), one file per language
(`.en.md` and `.pt-BR.md`); the scenario reference and the index of decisions are
generated from the schema and from the ADRs. Editing the documentation is editing
those files, and the test fails the build if a published code block stops being
valid or if a translation falls behind its English source.

Documents in this repository are written in Portuguese: the ADRs, the commits and
the internal reports record decisions for whoever maintains the tool. What
reaches the user is in English.

## Documentation in the repository

These record decisions for whoever maintains the tool, and they are written in
Portuguese — the titles below are kept in the language of the files. What
reaches the user is in English, and it is on the
[site](https://diegobraun.github.io/braunrate/).

- [Princípios de produto](docs/principios-de-produto.md) — the acceptance criterion of every interface decision
- [Vocabulário](docs/vocabulario.md) — one word per concept, in every text shown to the user
- [Decisões de internacionalização](docs/decisoes-i18n.md)
- [Decisões de experiência de uso](docs/decisoes-experiencia.md)
- [Relatório de experiência de uso](docs/relatorio-experiencia.md)
- [Roteiro](docs/roteiro.md)
- [Arquitetura](docs/arquitetura.md)
- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — the basis of every decision
- [ADRs](docs/adr) — 20 decisions, each with what was refused and the criterion that reopens it
- [API do modo servidor](docs/api-servidor.md) — one curl example per route
- [Scenario schema](docs/braunrate.schema.json) — autocomplete and validation in the editor
- [Example HTML report](docs/exemplo-relatorio.html) — real output of a run that failed the acceptance criterion
- [Bateria adversarial](docs/bateria-adversarial.md) — where the tool fails, lies or frustrates
- [Auditoria de fricção](docs/auditoria-fricao.md) — what the tool demands and does not provide
- [Medição dos protótipos da Fase 0](docs/medicoes-fase0.md)

## License

MIT — Diego Braun.
