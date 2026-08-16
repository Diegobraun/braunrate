// Package cenarioemgo carries the scenario published in the README, as code.
//
// A snippet that lives only inside a fenced block is a snippet nobody compiles:
// this one stopped compiling when the identifiers were translated and stayed
// wrong until someone tried to run it, months later. Here the compiler reads it
// on every build, a test runs it against the built-in target, and another test
// fails if the README drifts from this file.
package cenarioemgo

import (
	"time"

	"github.com/Diegobraun/braunrate"
	"github.com/Diegobraun/braunrate/dsl"
)

// README:inicio
// Scenario is the same journey of examples/jornada-autenticada.yaml, written in
// Go: same engine, same metrics, same result document.
func Scenario(alvo string) (braunrate.Scenario, error) {
	return dsl.New("Jornada de cobrança").
		Target(alvo).
		Auth(dsl.WithToken(
			dsl.POST("/auth/token").Body(map[string]any{"usuario": "ana", "senha": "${SENHA:-segredo}"}),
			dsl.Capture("token", "$.access_token"),
		).RefreshAfter(25*time.Minute)).
		DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(dsl.Circular)).
		Ramp(dsl.PerSecond(50), dsl.PerSecond(300), 5*time.Second).
		Steady(dsl.PerSecond(300), 5*time.Second).
		Step(dsl.GET("/pedidos/${assinantes.id}"),
			dsl.Name("consultar pedido"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.ultimaFatura.status", "ABERTA"),
			dsl.Capture("faturaId", "$.ultimaFatura.id")).
		Step(dsl.POST("/faturas/${faturaId}/pagar").
			Body(map[string]any{"valor": 199.90}),
			dsl.Name("pagar fatura"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.status", "PAGA")).
		SLO("consultar pedido", "p95", "< 150ms").
		SLO("pagar fatura", "p95", "< 200ms").
		JourneySLO("p95", "< 2s").
		JourneySLO("p99", "< 5s").
		OverallSLO("erros", "< 0.1").
		Build()
}

// README:fim
