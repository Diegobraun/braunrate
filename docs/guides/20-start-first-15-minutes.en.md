# First 15 minutes

From nothing to a report of your own service, read and understood. If you have
never run a load test, do not skip the first two steps: they explain the terms
the rest of the path uses.

## 1. See it working

```bash
braunrate demo
```

No file, no target and no second terminal. The command brings up a fake service
at `127.0.0.1:8080`, runs a scenario against it and explains every number:

```
[2/3] Running: 100 requests per second, for 5s.

      That is the rate: braunrate fires at that pace whether the service is
      fast or slow — the way real users do. Tools that wait for the previous
      response before sending the next one go easy on the system exactly when
      it is struggling.

[3/3] Done. What the numbers say:

  500 requests in 5s, 100 per second, 0.00% of them errors
  Half the responses within 6.5 ms; 95% within 7.0 ms; the worst took 14 ms

  ok    Passed: the whole scenario had the error rate of 0.00%, within the limit of 0.10%.
```

When it finishes, two files are left in the directory: `demo.yaml`, the commented
scenario that just ran, and `demo-report.html`, the full report. Open both.

To watch the tool catch a real problem:

```bash
braunrate demo --with-failure
```

## 2. Understand what you have just read

Five ideas, and no other one is needed to start. Each has the long explanation in
[Concepts](concepts.html):

| Idea | In one line |
|---|---|
| **rate** | how many requests per second the generator fires, whether the target is fast or slow |
| **"95% of the responses within X"** | 5% of the people waited longer than X; the average is not in the report because it hides the tail |
| **acceptance criterion** | the limit you declare in the `slo` block; go over it and the command exits with code 1 and CI fails |
| **fixed data distorts** | a thousand identical requests measure the target cache, and the report warns when the scenario varies nothing |
| **invalid result** | the run did not measure what it set out to; exit code 3, and none of its numbers count as an answer |

## 3. Start from your own service

Do not start from a blank page. Copy a `curl` from the browser network panel and
turn it into a scenario:

```bash
braunrate import curl 'curl https://your-api/orders/9912 -H "Authorization: Bearer abc.def"' -output scenario.yaml
```

The token becomes `${TOKEN}`, read from the environment, and never reaches the
repository.

The other ways in, for when there is no `curl` at hand:

| You have | Command |
|---|---|
| a JMeter plan | `braunrate import jmx plan.jmx -output scenario.yaml` |
| the browser open on the flow | `braunrate record -output scenario.yaml` |
| nothing yet | `braunrate new scenario.yaml` |

> **Note** The JMeter importer translates the common subset and lists in the
> terminal what was left out, instead of translating half of it quietly.

## 4. Run it once before running it a thousand times

```bash
braunrate debug scenario.yaml
```

One iteration, one user, no load. It shows what was sent, what came back, what
was captured and where it stopped:

```
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

Iteration complete: 2 steps, all good. To run it with load:
  braunrate execute scenario.yaml
```

> **Important** Load is only worth running after the iteration passes. Finding
> out the correlation broke ten minutes into the load is the mistake JMeter
> taught everyone to make.

## 5. Declare what you consider acceptable

Without an `slo` block the scenario runs and reports, but it does not work as a
gate. With it, the exit code decides:

```yaml fragment
slo:
  - look up order: { p95: < 150ms }   # one step
  - journey: { p95: < 2s }            # the whole wait, end to end
  - global: { errors: < 0.1 }         # the entire run
```

A gate made only of per-step rules approves every piece and says nothing about
the wait the user feels, which is the sum of them. `braunrate validate` warns you
when your gate looks like that.

## 6. Run it with load and read the result

```bash
braunrate execute scenario.yaml -html=report.html -result=output.json
```

Read it in this order:

1. **The first sentence.** "Passed", "Failed" or "Invalid result". If it is the
   third one, stop here: nothing below counts.
2. **"What happened"** — how many requests, the throughput, the error rate.
   Throughput well below the declared rate has two opposite causes, and the tool
   says which one it was.
3. **"The whole journey"** — the time the user waits end to end. It is the
   reading that counts when the scenario has more than one step.
4. **"How trustworthy the measurement is"** — whether the generator fell behind,
   whether the target degraded over time, whether some data never varied. This is
   where the caveats that change the reading of everything above show up.
5. **"SLO"** — the verdict, rule by rule, and also what you did **not** declare.

## 7. Turn it into a CI gate

```bash
braunrate execute scenario.yaml -quiet -result=output.json
```

| Exit code | Meaning |
|---|---|
| `0` | passed |
| `1` | the acceptance criterion failed |
| `2` | an error in the scenario file |
| `3` | invalid result: the run did not measure what it set out to measure |

The full recipe, comparing against the previous run, is in
[Recipes](recipes.html#make-the-test-fail-the-build).
