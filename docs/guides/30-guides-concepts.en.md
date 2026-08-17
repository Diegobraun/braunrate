# Concepts

Five ideas. None of them is theory: each one matches a line that shows up in the
report, and knowing how to read them is the difference between trusting the
number and repeating the number.

## Rate, and why the open model

**Rate** is how many requests per second the generator fires. In braunrate it is
a decision of yours, declared in the file, and the generator insists on it even
when the target takes its time:

```yaml fragment
load:
  profiles:
    - ramp: { from: 100/s, to: 800/s, duration: 5s }
    - steady: { rate: 800/s, duration: 10s }
    - spike: { rate: 2000/s, duration: 3s }
```

That is the **open model**, and it is the default. The other way to describe load
is the **closed loop**: N virtual users, each one asking again only after the
previous response arrived. It is how JMeter and Locust measure.

The difference matters exactly when it matters most. In a closed loop, if the
target freezes, the users stop asking along with it, and the delay never enters
the count. A real user does not do that: they arrive when they were going to
arrive, and they wait.

braunrate counts the response time **from the instant the request should have
gone out**, not from when it did. That is why a freeze in the target shows up in
the number instead of disappearing from it. The name for this is coordinated
omission, and you can see both sides on your machine with
`braunrate demo --with-failure`.

### The closed model exists, declared

```yaml fragment
load:
  model: closed
  users: 200
  duration: 5m
  thinkTime: 1s
```

**It serves** when the real limit is on the session, not on arrival: a connection
pool, a per-seat licence, a queue with a fixed number of workers, or when you are
reproducing a JMeter plan written in threads.

**It lies** when the question is "does the target hold X per second?". Same
target, frozen for 3 s in the middle of 12 s — on the left 100/s in the open
model, on the right 10 users in a closed loop:

```
1,200 requests, 100 per second             850 requests, 70 per second
half 6.7 ms | 95% 2.41 s | worst 3.01 s    half 6.9 ms | 95% 7.9 ms | worst 2.96 s
```

The 95% fell from **2.41 s to 7.9 ms**. The closed loop got no arithmetic wrong:
it measured with precision an event it stopped causing itself. Look at the rate
as well, 100/s against 70/s — in a capacity test, that is the load that should
have kept coming.

That is why the report of a closed model opens with a warning, always, even when
everything passes; the JSON document **has no** corrected-time field, because
with no scheduled instant there is nothing to correct; and `braunrate compare`
refuses to compare an open run with a closed one.

## "95% of the responses within X"

The report shows no average. An average hides things: if 95 responses take 5 ms
and 5 take 2 seconds, the average reads 105 ms and nobody notices the five slow
ones.

"95% within 6.2 ms" means 5% of the people waited longer than 6.2 ms. The
published cuts are half (50%), 95%, 99%, 99.9% and the worst.

### The time of step 2 onwards is not corrected

Only the first step has a scheduled instant of its own. The ones after it depend
on a value captured before them, so they start when the previous step finishes.
This real run, against the built-in target frozen for 1 s in the middle, shows the
size of the problem:

```
Per step
  step                             requests      half       95%       99%     99.9%     worst  errors
  look up order              (1)      2,400    5.5 ms    416 ms    900 ms    1.00 s    1.01 s       0
  pay invoice                (2)      2,400    5.3 ms    7.2 ms     12 ms     15 ms    1.01 s       0

  (1) time counted from the instant the request should have gone out — it includes
      any delay, and for that reason it does not hide a freeze in the target.
  (2) plain response time, counted from when the previous step finished. Because
      that step depends on a value captured before it, it has no scheduled
      instant of its own. For the honest reading of the journey, use "The whole journey".
```

Look at `pay invoice`: **7.2 ms at the 95%**, with the target frozen for a whole
second. On its own, that number is the same kind of lie a closed-loop tool
produces.

> **Important** The reading that counts for whoever uses the system is **The
> whole journey**, counted from the scheduled instant. In the same run above, it
> shows 428 ms.

## Acceptance criterion

It is the limit you declare and that becomes an exit code. Four scopes:

```yaml fragment
slo:
  - look up order: { p95: < 150ms }                 # one step
  - journey: { p95: < 2s, p99: < 5s }               # the whole wait
  - global: { success: ">= 99.9", throughput: ">= 90/s" }
  - regression: { journeyP95: "<= 10% worse" }      # against a previous run
```

The report also shows **what was not declared**, because a gate that only
measures parts approves every piece while saying nothing about the whole wait:

```
SLO
  FAIL  Failed: "look up order" answered 95% within 416 ms, above the limit of 150 ms.
  ok    Passed: the whole journey answered 95% within 428 ms, within the limit of 2000 ms.
  ok    Passed: the whole scenario had the success rate of 100.00%, at the minimum of 99.90%.
  --    steps with no criterion declared (1 of 2): pay invoice
  --    regression: no criterion declared — the gate approves without comparing against the previous run
```

None of this is mandatory: a scenario with no `slo` block still runs and still
reports, it just does not work as a gate.

## Variety of the data

Repeating the same request a thousand times measures the target cache, not the
target. The report publishes **what happened**, not what was declared:

```
Environment
  4 distinct values of orders.id across 100 uses, between 1,001 and 1,004
  1 single value of token across 100 uses
```

A count of distinct values answers "one value or many"; it does not answer
**where** the values landed, and a thousand different ids belonging to the same
customer exercise a single slice of the target. That is why the line also carries
the range and the common prefix.

If the source has several values and the run used only one, the result is
**invalid**:

```
INVALID RESULT: the whole load landed on a single partition of orders-chain; the rest of the
cluster stood still and the number does not represent production. Make the message key vary per iteration
            kafka.partition.orders-chain had 4 available values and the run used 1, across 60 uses
```

That rule was born from a defect of ours: authentication froze the data of the
first iteration, and every authenticated run with a CSV ran over the first line
while the report announced variety that never existed.

## Invalid result

Every run goes through a sanity check **before** the acceptance criterion is
read. It does not ask whether the target did well; it asks whether the run
measured what it set out to measure. When the answer is no, the criterion is
never even evaluated and the command exits with **code 3**:

```
Invalid result: the run did not measure what it set out to measure. This is not a verdict on the
target — it is the measurement that does not hold, and that is why no SLO rule was evaluated.

  - no journey reached the end, so the scenario never exercised the sequence it declared. Run
    'braunrate debug' to see where the iteration stops
    60 journeys started, 0 completed
  - the step "look up order" failed on 100% of the requests; no successful response entered its
    measurement
    60 requests, 60 errors (status: 60)
```

The six cases that invalidate:

| Case | Why the number does not hold |
|---|---|
| no journey reached the end | the scenario never exercised the sequence it declared |
| every step failed, or one step failed on 100% | the time measured is the time to refuse, not the time to do |
| the declared load was not applied in full | only the piece that ran got measured |
| a declared step recorded no sample | it stayed out of the measurement |
| variety collapsed on a source with several values | the target may have answered from cache |
| the generator did not sustain the rate | the numbers measure the generator, not the target |

> **Warning** Code 3 is not code 1. `1` means "the target did not meet the
> criterion"; `3` means "this run is no good for claiming anything".

The check applies always, with or without an `slo` block. It was born from three
defects of the same family: data frozen on the first iteration, `examples/ci.yaml`
itself running 100% of 401 and passing green since Phase 1, and the variety that
was only checked when somebody asked. All three were runs that were
syntactically perfect, semantically empty, with the whole suite green.

## A permanent caveat: one token for the whole run

The engine logs in once and reuses the credential across every journey, and that
does not exist in production. If the target has caching by identity, rate limiting
by token or sharding by user, the number comes out optimistic, or it fails with a
429 that would not have happened. The report declares this caveat on every run
with authentication.
