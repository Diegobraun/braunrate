# Protocols

Five protocols compiled into the binary: `http`, `graphql`, `kafka`, `amqp` and
`await`. `braunrate version` lists the ones your binary has.

What changes from one to the next is not only the syntax: it is **what counts as
a unit of measurement** and **what counts as an error**.

## HTTP

The short form fits on one line; the long one takes a method, headers, a body and
a timeout:

```yaml fragment
scenario:
  - http: GET /orders/${subscribers.id}
    name: look up order
    expect: { status: 200, json: { $.lastInvoice.status: OPEN } }
    capture: { invoiceId: $.lastInvoice.id }

  - name: pay invoice
    http:
      method: POST
      path: "/invoices/${invoiceId}/pay"
      headers: { X-Tenant: "${tenant}" }
      body: { amount: 199.90 }
    expect: { status: 200 }
```

The unit of measurement is the step. An error is a status outside what `expect`
declared, a network failure, a timeout, or an assertion that did not hold. Each
one becomes a class of its own in the report, because "failed 30 times" does not
say where to look.

### An HTTPS target with a private certificate

Corporate staging almost always serves HTTPS with an internal CA, or requires a
client certificate. If the CA is already in the machine's trust store, there is
nothing to declare. When it is not:

```yaml fragment
target: https://api.staging.internal

tls:
  ca: /etc/ssl/staging/ca.pem
```

For mTLS, the client certificate comes along:

```yaml fragment
tls:
  ca: /etc/ssl/staging/ca.pem
  certificate: /etc/ssl/staging/client.pem
  key: /etc/ssl/staging/client.key
```

It applies to `http`, `graphql` and to the polling of `await`. Without the block,
the error says what to declare instead of passing along the x509 text:

```
  look up                    network failure                           30   certificate signed by a CA this machine does…
    certificate signed by a CA this machine does not know — declare tls: { ca: /path/ca.pem }
```

> **Note** The file path takes `${VARIABLE}`. No private key is ever read into
> the scenario: what goes in the file is the path, never the content.

## GraphQL

Paste the query; the operation name becomes the line in the report:

```yaml fragment
scenario:
  - graphql:
      query: |
        query LookUpOrder($id: ID!) {
          order(id: $id) { id status lastInvoice { id status } }
        }
      variables: { id: "${subscribers.id}" }
    expect:
      json: { $.data.order.status: OPEN }
    capture:
      invoiceId: $.data.order.lastInvoice.id

slo:
  - graphql LookUpOrder: { p95: < 150ms }
```

Two things a generic HTTP tool gets wrong in GraphQL, and that are the default
here.

**The operation is the unit of measurement.** Everything in GraphQL arrives at
`POST /graphql`. Aggregating by URL would put the cheapest query and the most
expensive mutation on the same line, and the 99% of the expensive one would
disappear. That is why the key is `graphql LookUpOrder`, and an anonymous
operation is refused while the scenario is read, with the message showing how to
name it.

**An error with status 200 is an error.** The specification says to answer `200`
with `errors` in the body. A real run against the built-in target, where a quarter
of the subscribers do not exist:

```
Failed: the whole scenario had the error rate of 14.28%, above the limit of 0.10%.

Per step
  step                             requests      half       95%       99%     99.9%     worst  errors
  graphql LookUpOrder        (1)      1,625    4.7 ms    5.1 ms    5.4 ms    5.8 ms     14 ms     406
  graphql PayInvoice         (2)      1,219    4.7 ms    5.0 ms    5.2 ms    5.8 ms    6.0 ms       0

Errors
  error in the GraphQL response body (with status 200)  406
```

All 2,844 responses came back with HTTP status 200. A tool that classifies by
status would have reported 0% errors and a green acceptance criterion. A partial
response (`data` and `errors` together) also counts as an error, and the detail
says it was partial.

## Kafka and RabbitMQ

Producing measures the broker accepting the message. What the user feels is the
whole chain, and that is what the `await` step is for:

```yaml fragment
scenario:
  - kafka:
      topic: orders
      key: "${orders.id}"           # a fixed key piles everything into one partition
      value: { order: "${orders.id}", amount: "${orders.amount}" }

  - await:
      kafka: { topic: orders-processed }
      key: "${orders.id}"           # waits for this iteration's message, not just any
      timeout: 10s
```

RabbitMQ follows the same shape:

```yaml fragment
scenario:
  - amqp:
      queue: orders
      messageId: "${orders.id}"
      body: { order: "${orders.id}" }
```

Publishing with confirmation is the default (`acks: all` in Kafka, publisher
confirms in AMQP), **with no batching**: grouping messages would measure the
batch, not the message.

### Why the whole chain

A real run against Kafka with the consumer deliberately overloaded — 15 ms per
message, which is 66/s of capacity, against a load of 100/s:

```
The whole journey
  All 800 journeys reached the end; half took up to 1490 ms and 95% up to 3957 ms,
  counted from the instant they should have started.

Per step
  step                             requests      half       95%       99%     99.9%     worst  errors
  await orders-slow-proces…  (2)        800    1.49 s    3.95 s    4.17 s    4.23 s    4.24 s       0
  kafka produce orders-slo…  (1)        800    1.2 ms    2.2 ms    3.9 ms    179 ms    228 ms       0
```

Producing costs 1.2 ms; the chain costs 3.96 s at the 95%. A tool that measures
only the produce would have reported milliseconds and approved the system.

### How far behind the consumer fell

When the consumer is a real service, the end-to-end chain is not always
available. What can be read straight from the broker is the **group lag**:

```yaml fragment
scenario:
  - kafka:
      topic: orders
      group: billing               # only observes; consumes nothing
      key: "${orders.id}"
      value: { order: "${orders.id}" }
```

```
Consumer lag
  group demo-lag-group on demo-lag: at its worst 885 messages behind; at the end, 885 messages
  The consumer finished the run behind. The lag says the distance, not the cause: a slow consumer, a stopped one and one rebalancing produce the same number.
```

Both numbers are read from the broker (high watermark minus the group's committed
offset), never counted on this side: a message this generator did not send still
weighs on the service. If the offset cannot be read, the report says it could not
measure, because a zero there would claim the consumer was up to date.

To send the whole load of a step to a single partition — a hot-partition test, a
specific replica — there is `partition: N`. It ignores the key, and the report
marks the run as deliberately concentrated.

### How much the generator holds while producing

With no batching, one message per scheduled arrival and with confirmation, the
maximum rate is lower than that of tools which group. The number, measured:

| Topic | Last valid rate | half / 95% of the produce | First invalid rate |
|---|---|---|---|
| 6 partitions | **15,000 msg/s** | 1.4 ms / 41 ms | 18,000/s (4,884 dropped) |
| 1 partition | **5,000 msg/s** | 0.2 ms / 56 ms | 8,000/s (8,059 dropped) |

At 15,000/s the scheduling drift stayed at 0.001 ms typical and 0.56 ms worst
case: what saturated first was not the braunrate scheduler, it was the confirmed
delivery path to the broker. The number in the table is the ceiling of the
generator+broker pair on this machine.

> **Warning** Measurement environment: Apple M2 Pro, 10 cores, Redpanda v24.2.7
> on loopback on the same host, 10 s per run. A remote broker, real replication
> and a bigger message change the number. Measure it in your environment before
> quoting this one.

### Pointing at a real broker

A credential **never goes into the file**: only the name of an environment
variable or the standard cloud chain. The scenario goes into the repository, and
the repository keeps it forever.

```yaml fragment
messaging:
  kafka:
    brokers: [kafka.staging:9093]
    auth: { type: scramSha512, user: "${KAFKA_USER}", password: "${KAFKA_PASSWORD}" }
    tls: { ca: /etc/ssl/staging/ca.pem }
```

```bash
KAFKA_USER=ana KAFKA_PASSWORD=... braunrate validate staging.yaml
```

```
Valid scenario: "Orders in staging", 1 step, 6000 iterations in 2m0s.
Messaging: kafka at kafka.staging:9093: scramSha512, user ana + TLS with a private CA
```

Accepted types: `saslPlain`, `scramSha256`, `scramSha512`, `mskIam` and
`certificate` (mTLS). `tls: true` turns TLS on with the system authorities;
`tls: { ca: ... }` uses an internal one.

**AWS MSK with IAM.** There is no key field, and there is not going to be one:

```yaml fragment
messaging:
  kafka:
    brokers: [b-1.msk.example:9098, b-2.msk.example:9098]
    auth: { type: mskIam, region: us-east-1 }
```

The signature comes from the standard AWS chain: `AWS_ACCESS_KEY_ID`,
`AWS_PROFILE`, or the machine role. TLS turns itself on, because port 9098 does
not accept anything else.

A password written into the file fails validation, and the message teaches the
way out:

```
error in the scenario: staging.yaml:6:65: literal password in the scenario: a credential never goes into the file, because the file goes into the repository.
    replace it with:  password: ${BROKER_PASSWORD}
    and run with:  BROKER_PASSWORD=... braunrate execute scenario.yaml
    a fallback value (${VAR:-something}) does not work either: the fallback would be the secret written in the file
```

The terminal, the HTML, the JSON and the debug output show the authentication
type and the user, never the secret. A wrong password becomes an error of class
`authentication` and a missing permission becomes `authorization`; neither turns
into "broker unavailable", which would send you to look at the firewall.

**Out for now:** OAUTHBEARER. **Out, with a reason:** a managed cloud service
(SQS, SNS, Kinesis, EventBridge, Service Bus, Pub/Sub) is not a broker you point
at, it is an SDK with delivery and billing semantics of its own; it would come in
as a new protocol, not as an authentication type.

## `await`

It measures the time until the effect happens, not the time to answer. There are
two forms: by message, shown above, and by polling an API, for when the
asynchronous system does not publish the result to a topic:

```yaml fragment
scenario:
  - kafka:
      topic: orders
      key: "${orders.id}"
      value: { order: "${orders.id}" }

  - await:
      http: { path: "/orders/${orders.id}" }
      until: { $.status: PROCESSED }      # or { status: 200 }, or { bodyContains: PAID }
      interval: 200ms
      timeout: 30s
```

The granularity is declared, not hidden. Measuring by polling only measures in
steps of the interval: the value always comes out greater than or equal to the
real one, never smaller. That is why the report carries the line *"the step X
waits by polling every 200ms: its time has that granularity and comes out larger
than the real one, never smaller"*, and `braunrate debug` shows the same thing
before any load.

Without `until` the step is refused: the first response would end the wait, and
the measurement would be of the time to answer instead of the time until the
effect happens.
