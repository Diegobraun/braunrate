# Troubleshooting

This page lists the most common errors by the text that shows up on the screen,
the cause of each one and what to do. Every message quoted here is real output
from the tool.

| Symptom | Likely cause |
|---|---|
| [`connection refused`](#the-target-did-not-answer) | the target is not up, or the address is wrong |
| [`status 401` or `403` on everything](#every-request-comes-back-401-or-403) | the scenario declares no authentication |
| [`Invalid result`, code 3](#invalid-result-and-exit-code-3) | the run did not measure what it set out to measure |
| [`I do not know where ${...} comes from`](#a-variable-reference-with-no-origin) | a reference with no declared origin |
| [`the environment variable X is not set`](#an-environment-variable-that-is-not-set) | the variable is missing, or a fallback value is |
| [`literal password in the scenario`](#a-secret-written-into-the-file) | a credential written into the file |
| [`unknown key at the top of the scenario`](#an-unknown-key) | a typo, or a key that does not exist |
| [`certificate signed by a CA this machine does not know`](#an-internal-ca-certificate) | the internal CA was not declared |
| [step 2 times that look too low](#the-times-of-the-second-step-look-too-low) | from the second step on, the time is service time |
| [a warning that nothing varies](#the-report-warns-that-no-value-varies) | the scenario always sends the same request |
| [the system blocks the binary](#macos-or-windows-blocks-the-binary) | there is no code signature |
| [`braunrate version` answers `dev`](#the-version-shows-up-as-dev) | a binary compiled locally |
| [`409` in server mode](#server-mode-answers-409) | a run is already in progress |

## The target did not answer

```
step 1 — get orders 1   [FAILED in 3.5ms]
  request:    GET /orders/1
  problem:    network failure
              connection refused

Nobody answered at http://127.0.0.1:9. Check that the service is up and that the address is right.
If you do not have a service to test yet, start the built-in one in another terminal:
  braunrate target
```

**Cause.** Nothing is listening on the address declared in `target`.

**Fix.** Check the address and bring the service up. If you do not have a service
to point at yet, braunrate carries one:

```bash
braunrate target -latency=5ms
```

> **Tip** To just try the tool out, `braunrate demo` needs no target at all: it
> brings the target up, runs and explains the result in one command.

## Every request comes back 401 or 403

```
  response:   status 401, 36 bytes
  body:       {"error":"missing or invalid token"}
  problem:    unexpected HTTP status
              status 401

The target refused for lack of a credential, and the scenario declares no auth
at all.
```

**Cause.** The target requires a credential and the scenario has no `auth` block.

**Fix.** Declare how the token is obtained. braunrate logs in once, in the
preparation, and injects the token into every step:

```yaml fragment
auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: ana } }
    capture: { token: $.access_token }
```

If the block is already there and 401 still comes back, run `braunrate debug`: it
shows the request that obtains the token and the value that was captured.

## Invalid result, and exit code 3

**Cause.** The sanity check decided the run did not measure what it set out to
measure. No acceptance-criterion rule is ever evaluated.

**Fix.** The message itself says which of the
[six cases](concepts.html#invalid-result) happened. The two most common ones:

- **no journey reached the end.** Some step always fails. Run `braunrate debug`
  to see where the iteration stops.
- **the generator did not sustain the rate.** The machine doing the generating
  could not keep up. Lower the rate, or generate from a bigger machine.

> **Important** Code 3 is not a verdict on the target. Code 1 means "the target
> did not meet the criterion"; code 3 means "this run is no good for claiming
> anything".

## A variable reference with no origin

```
error in the scenario: scenario.yaml:7:26: I do not know where ${invoiceld} comes from.
    declare where it comes from:
      variables: { invoiceld: value }                # fixed in the scenario
      variables: { invoiceld: "${INVOICELD:-fallback}" }  # from the environment, with a fallback
      capture: { invoiceld: $.field }                # from an earlier response
      data: { orders: { file: orders.csv } }  # and then ${orders.invoiceld}
    an UPPERCASE name comes from the environment with nothing to declare: ${INVOICELD}
```

**Cause.** The reference does not come from `variables`, from `data`, from an
earlier `capture` or from the environment in UPPER CASE.

**Fix.** Correct the name, or declare the origin. The refusal exists because this
reference used to become empty text quietly: the request went out with a blank
field, the target answered 401 or 404, and nothing in the output tied one thing
to the other.

## An environment variable that is not set

```
error in the scenario: scenario.yaml:5:23: invalid rate: "${RATE}/s" (use for example 50/s)
    the environment variable RATE is not set, so this field kept the raw reference.
    run with RATE=... , or declare a default in the file: ${RATE:-value}
```

**Cause.** The field references a variable that does not exist in the environment
and has no fallback value.

**Fix.** Set the variable on the run, or write the fallback into the file:

```bash
RATE=100 braunrate execute scenario.yaml
```

## A secret written into the file

```
error in the scenario: staging.yaml:6:65: literal password in the scenario: a credential never goes into the file, because the file goes into the repository.
    replace it with:  password: ${BROKER_PASSWORD}
    and run with:  BROKER_PASSWORD=... braunrate execute scenario.yaml
    a fallback value (${VAR:-something}) does not work either: the fallback would be the secret written in the file
```

**Cause.** A credential field has a literal value.

**Fix.** Replace it with the name of an environment variable. A fallback value
(`${VAR:-something}`) is refused as well, because the fallback would be the
secret itself written into the file.

> **Note** There is no way to turn this refusal off. A credential only comes in
> through an environment variable or through the standard cloud chain.

## An unknown key

```
error in the scenario: scenario.yaml:3:1: unknown key at the top of the scenario: "carg"
    available: name, target, requires, variables, auth, tls, messaging, data, load, scenario, slo
    a minimal scenario has four of them:
      name: Order lookup
      target: http://127.0.0.1:8080
      load: { profiles: [ { steady: { rate: 100/s, duration: 1m } } ] }
      scenario:
        - http: GET /orders/1
```

**Cause.** The key does not exist, or has a typo.

**Fix.** The full list is in [Scenario reference](reference.html). For the editor
to complete the keys and point at the error before you run, put this line at the
top of the file:

```yaml fragment
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
```

## An internal CA certificate

```
look up      network failure   30    certificate signed by a CA this machine does…
  certificate signed by a CA this machine does not know — declare tls: { ca: /path/ca.pem }
```

**Cause.** The target serves HTTPS with an authority the machine does not know.

**Fix.** Declare the CA at the top of the scenario — see
[Protocols](protocols.html#an-https-target-with-a-private-certificate).

> **Note** There is no option to turn certificate verification off. A
> self-signed certificate works as its own CA, and the option would be the easy
> way out for whoever does not want to configure it.

## The times of the second step look too low

**Cause.** It is not a mistake in the tool: only the first step has a scheduled
instant of its own. From the second on, the time is counted from when the
previous step finished, because the step depends on a value captured before it.

**Fix.** Read the **"The whole journey"** block, which counts from the instant the
journey should have started. The full explanation is in
[Concepts](concepts.html#the-time-of-step-2-onwards-is-not-corrected).

## The report warns that no value varies

```
Warning: the step "look up order" has no value that varies — every request will be identical.
    if the target caches by that key, the number comes out optimistic.
    to make it vary:  data: { orders: { file: orders.csv } }  and then  GET /orders/${orders.id}
```

**Cause.** The step has no `${}` reference, so every request is the same.

**Fix.** Replace the fixed value with a reference and declare where it comes
from, in [Recipes](recipes.html#every-journey-needs-data-of-its-own).

> **Note** If the fixed path is deliberate, as in a smoke test, the warning is
> still correct and invalidates nothing.

## macOS or Windows blocks the binary

**Cause.** There is no code signature and no notarization, so both systems warn
that the developer cannot be verified.

**Fix.** The instruction to clear it is in
[Installation](installation.html#1-download-the-release-binary), along with the
reason the signature does not exist.

## The version shows up as `dev`

**Cause.** The binary was compiled locally or installed with `go install`, and
the version is only injected into the release artefact.

**Fix.** For a real version number, download the release archive.

> **Warning** The version travels inside the result document, and
> `braunrate compare` comes out with no verdict when the two runs used different
> versions.

## Server mode answers 409

**Cause.** A run is already in progress.

**Fix.** Wait for it to finish. Two runs on the same machine fight over the CPU
that has to dispatch at the scheduled instant, and neither of them measures what
it set out to measure. The answer says how to accept the contamination, if that
is what you want.
