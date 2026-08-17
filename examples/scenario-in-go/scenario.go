// Package scenarioingo carries the scenario published in the README, as code.
//
// A snippet that lives only inside a fenced block is a snippet nobody compiles:
// this one stopped compiling when the identifiers were translated and stayed
// wrong until someone tried to run it, months later. Here the compiler reads it
// on every build, a test runs it against the built-in target, and another test
// fails if the README drifts from this file.
package scenarioingo

import (
	"time"

	"github.com/Diegobraun/braunrate"
	"github.com/Diegobraun/braunrate/dsl"
)

// README:inicio
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

// README:fim
