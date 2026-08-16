// Package demo is the first thing someone runs. It stands up a target, writes
// a scenario, executes it and explains the numbers while they appear — because
// the audience includes people who never ran a load test, and a report is not
// self-explanatory to someone who has never seen one.
package demo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/selfcheck"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

const (
	rate           = "100/s"
	duration       = "5s"
	runDuration    = 5 * time.Second
	targetLatency  = 5 * time.Millisecond
	freezeAfter    = 2 * time.Second
	freezeDuration = 2 * time.Second
	preferredPort  = "127.0.0.1:8080"
	anyFreePort    = "127.0.0.1:0"
	closedLoopPath = "/pedido"
)

type Options struct {
	WithFailure bool
	Directory   string
	Version     string
	Output      io.Writer
}

func Run(runContext context.Context, options Options) error {
	if options.WithFailure {
		return runFreezing(runContext, options)
	}
	return runHealthy(runContext, options)
}

func runHealthy(runContext context.Context, options Options) error {
	say(options, `
Esta demonstração roda contra um serviço de mentira que sobe aqui mesmo, então
você pode experimentar sem afetar nada.

`)

	target, stop, err := startTarget(options, testsupport.Options{Latency: targetLatency})
	if err != nil {
		return err
	}
	defer stop()

	say(options, `[1/3] Subindo um serviço de exemplo em %s
      Ele responde em ~%d ms, como uma API saudável responderia.

`, address(target), targetLatency.Milliseconds())

	scenarioPath := filepath.Join(options.Directory, "demo.yaml")
	if err := write(scenarioPath, healthyScenario(target)); err != nil {
		return err
	}

	say(options, `[2/3] Rodando: %s requisições por segundo, durante %s.

      Essa é a taxa: o braunrate dispara nesse ritmo esteja o serviço rápido ou
      lento — como usuários de verdade fazem. Ferramentas que esperam a
      resposta anterior antes de mandar a próxima aliviam o sistema justamente
      quando ele está sofrendo.

      O cenário que está rodando ficou em %s, comentado.

`, strings.TrimSuffix(rate, "/s"), duration, scenarioPath)

	result, err := runner.Execute(runContext, scenarioPath, runner.DefaultOptions(options.Version))
	if err != nil {
		return err
	}
	document := result.Document

	say(options, "[3/3] Pronto. O que os números dizem:\n\n")
	sayMeasurement(options, document)
	sayVerdict(options, document)
	sayFixedDataCaveat(options, document, scenarioPath)

	htmlPath := filepath.Join(options.Directory, "demo-relatorio.html")
	if err := runner.WriteHTML(htmlPath, document); err != nil {
		return err
	}

	say(options, `Relatório completo: %s
Os dois arquivos ficaram aqui no diretório atual; apague quando quiser.

Quer ver a ferramenta pegando um problema de verdade?

    braunrate demo --com-falha

`, htmlPath)
	return nil
}

func runFreezing(runContext context.Context, options Options) error {
	say(options, `
Esta demonstração mede o mesmo serviço travado de duas formas, e mostra o que
cada uma reporta.

`)

	freezing := testsupport.Options{
		Latency:     targetLatency,
		FreezeAfter: freezeAfter,
		FreezeFor:   freezeDuration,
	}
	target, stop, err := startTarget(options, freezing)
	if err != nil {
		return err
	}
	defer stop()

	say(options, `[1/4] Subindo um serviço de exemplo em %s, com uma diferença: ele
      trava por %d segundos no meio da execução. É o que um GC longo, um lock
      ou um failover fazem com um serviço de verdade.

`, address(target), int(freezeDuration.Seconds()))

	scenarioPath := filepath.Join(options.Directory, "demo-com-falha.yaml")
	if err := write(scenarioPath, freezingScenario(target)); err != nil {
		return err
	}

	say(options, "[2/4] Rodando o braunrate: %s por segundo durante %s.\n\n",
		strings.TrimSuffix(rate, "/s"), duration)

	result, err := runner.Execute(runContext, scenarioPath, runner.DefaultOptions(options.Version))
	if err != nil {
		return err
	}
	document := result.Document

	say(options, `[3/4] Agora um laço fechado, contra um serviço idêntico que trava igual.
      Laço fechado é como JMeter e Locust medem: a próxima requisição só sai
      depois que a anterior responde.

`)

	closedTarget, stopClosed, err := startTwinTarget(freezing)
	if err != nil {
		return err
	}
	defer stopClosed()
	closed := selfcheck.RunClosedLoop(runContext, closedTarget, closedLoopPath, runDuration)

	open := document.Overall.Reported()
	say(options, `[4/4] Mesma pausa, mesmo tipo de alvo, mesma requisição, duas medições:

      laço fechado (JMeter, Locust):  99%% em até %.1f ms sobre %d requisições
      braunrate (modelo aberto):      99%% em até %.1f ms sobre %d requisições

      %.1f ms escondidos pelo laço fechado.

      O laço fechado não mente por bug. Quando o alvo trava, ele para de
      enviar, e as requisições que deveriam ter partido nunca entram na conta —
      inclusive as que um usuário de verdade teria mandado. O braunrate conta
      do instante em que a requisição deveria ter partido, então a pausa
      aparece.

`, closed.P99, closed.Samples, open.P99, document.Overall.Count, open.P99-closed.P99)

	sayMeasurement(options, document)
	sayVerdict(options, document)
	if result.Exit == runner.ExitSLO {
		say(options, `      Se isto fosse o seu CI, o braunrate teria saído com código 1 e a esteira
      reprovaria. Com a medição de laço fechado, o mesmo critério passaria.

`)
	}

	htmlPath := filepath.Join(options.Directory, "demo-com-falha-relatorio.html")
	if err := runner.WriteHTML(htmlPath, document); err != nil {
		return err
	}
	say(options, "Relatório completo: %s\n\n", htmlPath)
	return nil
}

func sayMeasurement(options Options, document metrics.Document) {
	overall := document.Overall
	latency := overall.Reported()
	elapsed := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(100 * time.Millisecond)
	say(options, `  %d requisições em %s, %.0f por segundo, %.2f%% de erro
  Metade das respostas em até %.1f ms; 95%% em até %.1f ms; a pior levou %.0f ms

      Repare que não existe média nessa linha. Média esconde: se 95 respostas
      levam 5 ms e 5 levam 2 segundos, a média dá 105 ms e ninguém percebe as
      cinco lentas. "95%% em até %.1f ms" quer dizer que 5%% das pessoas
      esperaram mais que isso.

`, overall.Count, elapsed, overall.EffectiveRate, overall.ErrorRate*100,
		latency.P50, latency.P95, latency.Max, latency.P95)
}

func sayVerdict(options Options, document metrics.Document) {
	if !document.Valid() && document.Sanity.Checked {
		say(options, "  %s\n", document.Sanity.Sentence)
		for _, finding := range document.Sanity.Findings {
			say(options, "      %s\n", finding.Message)
		}
		say(options, `
      Resultado inválido não e o mesmo que resultado ruim: quer dizer que a
      execução não mediu o que se propôs, e nenhum número acima vale como
      resposta.

`)
		return
	}
	for _, evaluation := range document.SLO.Evaluations {
		mark := "ok  "
		if !evaluation.Passed {
			mark = "FALHA"
		}
		say(options, "  %-5s %s\n", mark, evaluation.Sentence)
	}
	if len(document.SLO.Evaluations) > 0 {
		say(options, `
      Isso é um critério de aceite: um limite que você declara no arquivo. Se
      estourar, o braunrate sai com código 1 — dá para usar direto no seu CI.

`)
	}
}

// The report already raises this; repeating it here is not decoration. Someone
// reading their first report does not know that a fixed path measures the
// cache, and the demo is exactly where that is learned.
func sayFixedDataCaveat(options Options, document metrics.Document, scenarioPath string) {
	for _, warning := range document.Warnings {
		if warning.Kind != "passo_sem_variacao" && warning.Kind != "valor_fixo" && warning.Kind != "variedade_ausente" {
			continue
		}
		say(options, `  Uma ressalva que o próprio relatório levanta:
      %s
      Requisição sempre igual mede o cache do alvo, não o alvo. Em %s, troque
      /pedidos/1 por /pedidos/${id} e declare de onde ${id} vem.

`, warning.Message, scenarioPath)
		return
	}
}

// A busy 8080 is the common case on a developer machine, and a demo that dies
// on it is a demo nobody sees. Any free port serves; what cannot happen is the
// address changing without the person being told, because the scenario file on
// disk carries it.
func startTarget(options Options, targetOptions testsupport.Options) (string, func(), error) {
	server := testsupport.New(targetOptions)
	if err := server.Start(preferredPort); err != nil {
		if err := server.Start(anyFreePort); err != nil {
			return "", nil, fmt.Errorf("não consegui subir o alvo de exemplo: %w", err)
		}
		say(options, "      (%s está ocupado, então o alvo subiu em %s)\n\n",
			preferredPort, address(server.Address()))
	}
	return server.Address(), func() { _ = server.Close() }, nil
}

// The twin the closed loop drives never appears in a file and never gets
// asked for by address, so it takes whatever port is free without a word about
// it.
func startTwinTarget(targetOptions testsupport.Options) (string, func(), error) {
	server := testsupport.New(targetOptions)
	if err := server.Start(anyFreePort); err != nil {
		return "", nil, fmt.Errorf("não consegui subir o segundo alvo de exemplo: %w", err)
	}
	return server.Address(), func() { _ = server.Close() }, nil
}

func address(target string) string { return strings.TrimPrefix(target, "http://") }

func write(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("não consegui gravar %s: %w", path, err)
	}
	return nil
}

func say(options Options, format string, args ...any) {
	if options.Output == nil {
		return
	}
	_, _ = fmt.Fprintf(options.Output, format, args...)
}
