// Package braunrate e a superficie publica para rodar um cenario de dentro de
// outro modulo Go — o par que faltava para o pacote dsl, que era publico e
// inutil de fora: ele devolve um cenario que so o proprio modulo conseguia
// executar.
//
// O caminho e o mesmo do CLI, sem excecao: mesma validacao, mesmo motor, mesmo
// documento de resultado, mesma avaliacao de SLO. Um segundo caminho aqui
// significaria um numero para quem escreve YAML e outro para quem escreve Go,
// que e o que o ADR 0009 proibe.
//
// O que esta fora, de proposito: escrever protocolo novo. A interface de
// protocolo continua em internal/ e vira contrato publico versionado a partir
// da v1 (ADR 0004).
package braunrate

import (
	"context"
	"io"
	"time"

	"github.com/Diegobraun/braunrate/internal/build"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"

	// Os protocolos compilados se registram por efeito de import, e de fora do
	// modulo ninguem alcanca internal/. Sem estes imports, um cenario montado em
	// Go falharia com "protocolo nao compilado neste binario".
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/grpc"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/mqtt"
	_ "github.com/Diegobraun/braunrate/internal/protocol/sse"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	_ "github.com/Diegobraun/braunrate/internal/protocol/websocket"
)

// Scenario e o que o dsl monta e o que o YAML produz: a mesma estrutura, que e
// a unica entrada do motor.
type Scenario = scenario.Spec

// Result e o documento de resultado, do qual relatorio, JSON, CSV e comparacao
// sao projecoes (ADR 0003 §6).
type Result = metrics.Document

// Verdict e o veredito de SLO ja avaliado sobre o resultado.
type Verdict = slo.Verdict

type Options struct {
	// Version aparece no bloco de ambiente do relatorio e no documento. Duas
	// execucoes com versoes diferentes nao sao comparaveis sem ressalva, e e por
	// isso que ela viaja com o numero. Vazio usa a versao deste binario.
	Version string
	// DataRoot e a pasta a partir da qual os caminhos de 'dados' sao resolvidos.
	// Vazio significa o diretorio de trabalho.
	DataRoot string
	// BaselinePath e o resultado anterior contra o qual comparar, quando o
	// cenario declara criterio de regressao.
	BaselinePath  string
	MaxInflight   int64
	LateThreshold time.Duration
}

// Load le e valida um cenario em YAML sem executar.
func Load(path string) (Scenario, error) {
	spec, _, err := runner.Load(path)
	return spec, err
}

// Parse le um cenario em YAML ja em memoria.
func Parse(content []byte) (Scenario, error) {
	spec, err := scenario.Parse(content)
	if err != nil {
		return spec, err
	}
	return spec, spec.Validate()
}

// Run executa e devolve o resultado com o veredito de SLO ja dentro dele.
//
// Um resultado invalido — a execucao que nao mediu o que se propos a medir —
// volta com Valid() falso e sem veredito, porque nao ha o que aprovar ou
// reprovar. Consulte Passed antes de decidir.
func Run(runContext context.Context, spec Scenario, options Options) (Result, error) {
	declaredVersion := options.Version
	if declaredVersion == "" {
		declaredVersion = build.Version
	}
	runnerOptions := runner.DefaultOptions(declaredVersion)
	if options.MaxInflight > 0 {
		runnerOptions.MaxInflight = options.MaxInflight
	}
	if options.LateThreshold > 0 {
		runnerOptions.LateThreshold = options.LateThreshold
	}
	runnerOptions.BaselinePath = options.BaselinePath

	result, err := runner.ExecuteSpec(runContext, spec, options.DataRoot, runnerOptions)
	return result.Document, err
}

// Passed diz se o resultado pode aprovar: uma medicao invalida nunca aprova,
// mesmo com todos os percentis dentro do limite.
func Passed(result Result) bool {
	return result.Valid() && result.SLO.Passed
}

// ExitCode e o mesmo codigo que o CLI devolve: 0 passou, 1 SLO reprovou,
// 3 medicao invalida.
func ExitCode(result Result) int {
	switch {
	case !result.Valid():
		return runner.ExitInvalid
	case !result.SLO.Passed:
		return runner.ExitSLO
	}
	return runner.ExitPassed
}

// Summary escreve o mesmo resumo de terminal que o CLI escreve.
func Summary(out io.Writer, result Result) error {
	return report.Summary(out, result, result.SLO)
}

// HTML escreve o mesmo relatorio autocontido que o CLI escreve.
func HTML(out io.Writer, result Result) error {
	return report.HTML(out, result)
}
