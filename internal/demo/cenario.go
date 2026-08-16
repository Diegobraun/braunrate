package demo

import "fmt"

// The demo runs a file it wrote, instead of a Spec built in Go: principle 1 of
// the product says the scenario is the truth, and a demonstration that runs
// something with no file behind it teaches that a secret path exists — and
// leaves whoever liked the result with nothing to edit.
func healthyScenario(target string) string {
	return fmt.Sprintf(`# Written by 'braunrate demo'. It is an ordinary scenario: point the target at
# your service, edit the steps and run it with 'braunrate execute'.
name: Demonstration
target: %s

# The built-in target asks for a token, the way a real API would. Without this
# block the whole run takes 401 and comes out invalid.
auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: ana } }
    capture: { token: $.access_token }

# rate: how many requests per second braunrate fires. It fires at that pace
# whether the target is fast or slow, which is what real users do.
load:
  profiles:
    - steady: { rate: %s, duration: %s }

scenario:
  # Fixed path: every request will be identical, and the report says the number
  # comes out optimistic. To measure the service instead of its cache, swap it
  # for /orders/${id} and declare where ${id} comes from.
  - http: GET /orders/1
    name: look up order
    expect: { status: 200 }

# acceptance criterion: if it goes over, 'braunrate execute' exits with code 1
# and your CI fails.
slo:
  - global: { errors: < 0.1 }
`, target, rate, duration)
}

// The failing demo sends exactly the request the closed loop sends, with no
// authentication in the way: the whole point is that the two measurements
// differ because of when the request is counted, not because of what it asked
// for.
func freezingScenario(target string) string {
	return fmt.Sprintf(`# Written by 'braunrate demo --with-failure'. The target of this demonstration
# freezes on purpose halfway through, the way a long GC or a failover would.
name: Demonstration with a failure
target: %s

load:
  profiles:
    - steady: { rate: %s, duration: %s }

scenario:
  - http: GET /order
    name: look up order
    expect: { status: 200 }

# acceptance criterion: this scenario exists to blow through it. On a
# closed-loop tool the same freeze would go unnoticed and the criterion would
# approve the run.
slo:
  - global: { errors: < 0.1 }
  - global: { p95: < 100ms }
`, target, rate, duration)
}
