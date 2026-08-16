package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Diegobraun/braunrate/internal/importer"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/recorder"
	"github.com/Diegobraun/braunrate/internal/report"
	"github.com/Diegobraun/braunrate/internal/report/comparison"
	"github.com/Diegobraun/braunrate/internal/runner"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/server"
	"github.com/Diegobraun/braunrate/internal/testsupport"
	"github.com/Diegobraun/braunrate/internal/texto"
)

const version = "0.4.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "execute":
		os.Exit(execute(os.Args[2:]))
	case "validate":
		os.Exit(validate(os.Args[2:]))
	case "debug":
		os.Exit(debug(os.Args[2:]))
	case "report":
		os.Exit(reportCommand(os.Args[2:]))
	case "compare":
		os.Exit(compare(os.Args[2:]))
	case "new":
		os.Exit(newOne(os.Args[2:]))
	case "import":
		os.Exit(importCommand(os.Args[2:]))
	case "record":
		os.Exit(record(os.Args[2:]))
	case "serve":
		os.Exit(serve(os.Args[2:]))
	case "target":
		os.Exit(serveTarget(os.Args[2:]))
	case "version":
		fmt.Printf("braunrate %s\nprotocolos compilados: %v\n", version, protocol.Registered())
		os.Exit(0)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `braunrate %s

uso:
  braunrate new [cenario.yaml]                  cria um cenario de partida, comentado
  braunrate debug <cenario.yaml>                um usuario, uma iteracao, tudo visivel
  braunrate execute <cenario.yaml> [opcoes]
  braunrate validate <cenario.yaml>
  braunrate import curl "<comando curl>"        gera um cenario a partir de um curl
  braunrate import jmx <plano.jmx>              traduz o subconjunto comum de um plano do JMeter
  braunrate record -output <cenario.yaml>       grava um cenario a partir do que passar pelo proxy
  braunrate report <resultado.json> [opcoes]    gera HTML ou CSV de um resultado ja gravado
  braunrate compare <antes.json> <depois.json> [-html <arquivo.html>]
  braunrate serve [-addr :8080] [-dir ./cenarios]   os mesmos comandos por HTTP, local
  braunrate target [opcoes]
  braunrate version

opcoes de execute:
  -result <arquivo.json>      grava o documento de resultado
  -html <arquivo.html>        grava o relatorio HTML autocontido
  -csv <arquivo.csv>          grava uma linha por passo, para planilha
  -max-concurrent <n>         maximo de requisicoes simultaneas (padrao 20000)
  -late-threshold <dur>       a partir daqui o gerador conta como saturado (padrao 10ms)
  -quiet                      nao imprime progresso durante a execucao
`, version)
}

func execute(args []string) int {
	set := flag.NewFlagSet("execute", flag.ExitOnError)
	resultPath := set.String("result", "", "arquivo JSON de resultado")
	htmlPath := set.String("html", "", "arquivo HTML de relatorio")
	csvPath := set.String("csv", "", "arquivo CSV com uma linha por passo")
	maxInflight := set.Int64("max-concurrent", 20000, "maximo de requisicoes simultaneas antes de desistir de disparar")
	lateThreshold := set.Duration("late-threshold", 10*time.Millisecond, "atraso de disparo a partir do qual o gerador e considerado saturado")
	quiet := set.Bool("quiet", false, "nao imprime progresso")
	baselinePath := set.String("baseline", "", "resultado de uma execucao anterior, para as regras de regressao")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenario")
		return runner.ExitBadFile
	}
	scenarioPath := positional[0]

	c, plan, err := runner.Load(scenarioPath)
	if err != nil {
		return faultExit(err)
	}

	if err := runner.RequireEnvironment(c); err != nil {
		return faultExit(err)
	}

	options := runner.DefaultOptions(version)
	options.MaxInflight = *maxInflight
	options.LateThreshold = *lateThreshold
	options.BaselinePath = *baselinePath
	if !*quiet {
		closed := c.Load.Closed()
		options.OnProgress = func(snapshot metrics.Snapshot, targetRate float64, remaining time.Duration) {
			if closed {
				fmt.Fprintf(os.Stderr, "\r%s", report.ClosedProgressLine(snapshot, c.Load.Users, remaining))
				return
			}
			fmt.Fprintf(os.Stderr, "\r%s", report.ProgressLine(snapshot, targetRate, remaining))
		}
	}

	if c.Load.Closed() {
		fmt.Fprintf(os.Stderr, "executando %q contra %s: %d usuarios em laco fechado durante %s\n",
			c.Name, c.Target, c.Load.Users, plan.Duration())
	} else {
		fmt.Fprintf(os.Stderr, "executando %q contra %s: %s iteracoes em %s\n",
			c.Name, c.Target, humanize(plan.TotalRequests()), plan.Duration())
	}

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	result, err := runner.Execute(runContext, scenarioPath, options)
	if err != nil {
		return faultExit(err)
	}
	if !*quiet {
		fmt.Fprintln(os.Stderr)
	}

	// A failed write to stdout does not change what happened to the target, so
	// the verdict below stands; what cannot happen is the report vanishing in
	// silence.
	if err := report.Summary(os.Stdout, result.Document, result.Document.SLO); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui escrever o resumo: %v\n", err)
	}

	for _, output := range []struct {
		path  string
		write func(string, metrics.Document) error
	}{{*resultPath, runner.WriteJSON}, {*htmlPath, runner.WriteHTML}, {*csvPath, runner.WriteCSV}} {
		if output.path == "" {
			continue
		}
		if err := output.write(output.path, result.Document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return runner.ExitSLO
		}
	}
	if *htmlPath != "" {
		fmt.Fprintf(os.Stderr, "relatorio em %s\n", *htmlPath)
	}

	return result.Exit
}

// The exit code of a failure is decided in the runner, so the CLI and the
// server never disagree about what a broken scenario is worth.
func faultExit(err error) int {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	if fault, is := err.(runner.Fault); is {
		return fault.Exit
	}
	return runner.ExitBadFile
}

// The standard flag package stops reading options at the first positional
// argument, so "executar cenario.yaml -html x.html" ignored the option
// silently. Here the list is walked to the end, and options hold before or
// after the file.
func parseArguments(set *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		if err := set.Parse(args); err != nil {
			return positional
		}
		if set.NArg() == 0 {
			return positional
		}
		positional = append(positional, set.Arg(0))
		args = set.Args()[1:]
	}
}

func reportCommand(args []string) int {
	set := flag.NewFlagSet("report", flag.ExitOnError)
	htmlPath := set.String("html", "", "arquivo HTML a gerar")
	csvPath := set.String("csv", "", "arquivo CSV a gerar")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, `informe o resultado gravado, por exemplo:
  braunrate report saida.json -html relatorio.html`)
		return 2
	}
	document, err := runner.ReadDocument(positional[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	if *htmlPath == "" && *csvPath == "" {
		*htmlPath = "relatorio.html"
	}
	if *htmlPath != "" {
		if err := runner.WriteHTML(*htmlPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Printf("relatorio em %s\n", *htmlPath)
	}
	if *csvPath != "" {
		if err := runner.WriteCSV(*csvPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Printf("csv em %s\n", *csvPath)
	}
	return 0
}

func compare(args []string) int {
	set := flag.NewFlagSet("compare", flag.ExitOnError)
	htmlPath := set.String("html", "", "grava a comparacao em HTML autocontido")
	positional := parseArguments(set, args)

	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, `informe as duas execucoes, a antiga primeiro:
  braunrate compare antes.json depois.json`)
		return 2
	}
	before, err := runner.ReadDocument(positional[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	after, err := runner.ReadDocument(positional[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	result := comparison.Compare(before, after)
	if err := report.Comparison(os.Stdout, result); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui escrever a comparacao: %v\n", err)
		return 2
	}
	if *htmlPath != "" {
		if err := runner.WriteComparisonHTML(*htmlPath, result, after.Version); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 2
		}
		fmt.Printf("comparacao em %s\n", *htmlPath)
	}
	if !result.Comparable {
		return 3
	}
	return 0
}

func humanize(value int64) string {
	return fmt.Sprintf("%d", value)
}

func debug(args []string) int {
	set := flag.NewFlagSet("debug", flag.ExitOnError)
	showBody := set.Bool("body", true, "mostra o corpo das respostas")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenario")
		return runner.ExitBadFile
	}
	scenarioPath := positional[0]

	c, _, err := runner.Load(scenarioPath)
	if err != nil {
		return faultExit(err)
	}

	if err := runner.RequireEnvironment(c); err != nil {
		return faultExit(err)
	}

	fmt.Printf("depurando %q contra %s: 1 usuario, 1 iteracao, sem carga\n", c.Name, c.Target)
	for _, line := range scenario.DescribeMessaging(c.Messaging) {
		fmt.Printf("mensageria: %s\n", line)
	}

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	iteration, err := runner.Debug(runContext, scenarioPath, version)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return faultExit(err)
	}

	for index, observation := range iteration.Observations {
		if err := report.Debug(os.Stdout, index+1, observation, *showBody); err != nil {
			fmt.Fprintf(os.Stderr, "nao consegui escrever a depuracao: %v\n", err)
			return runner.ExitBadFile
		}
	}
	if err := report.IterationVars(os.Stdout, iteration.Vars); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui escrever a depuracao: %v\n", err)
		return runner.ExitBadFile
	}

	fmt.Println()
	if !iteration.Complete() {
		if len(iteration.Observations) < len(c.Steps) && !iteration.Failed() {
			fmt.Println("A iteracao nao chegou ao fim.")
		} else {
			fmt.Printf("A iteracao parou no passo %d. Corrija e rode de novo; a carga so vale depois que a iteracao passa.\n",
				len(iteration.Observations))
		}
		return runner.ExitSLO
	}
	fmt.Printf("Iteracao completa: %s, tudo certo. Para rodar com carga:\n  braunrate execute %s\n",
		texto.Count(int64(len(iteration.Observations)), "passo", "passos"), scenarioPath)
	return runner.ExitPassed
}

// The curl command arrives as a single argument full of quotes, and people
// paste the option before or after it; the standard flag package stops at the
// first positional argument and would drop an option pasted at the end.
func newOne(args []string) int {
	destination := "cenario.yaml"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		destination = args[0]
	}
	if _, err := os.Stat(destination); err == nil {
		fmt.Fprintf(os.Stderr, "%s ja existe; escolha outro nome:\n  braunrate new outro-cenario.yaml\n", destination)
		return 2
	}
	if err := os.WriteFile(destination, []byte(importer.Skeleton()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui gravar %s: %v\n", destination, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "cenario de partida em %s: troque o alvo e o caminho pelo seu servico.\n", destination)
	fmt.Fprintf(os.Stderr, "\nProximo passo, antes de qualquer carga:\n  braunrate debug %s\n", destination)
	return 0
}

func importCommand(args []string) int {
	out := ""
	var rest []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-output" || arg == "--saida":
			if index+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "a opcao -output ficou sem o nome do arquivo")
				return 2
			}
			index++
			out = args[index]
		case strings.HasPrefix(arg, "-output="):
			out = strings.TrimPrefix(arg, "-output=")
		case strings.HasPrefix(arg, "--saida="):
			out = strings.TrimPrefix(arg, "--saida=")
		default:
			rest = append(rest, arg)
		}
	}

	if len(rest) < 1 || (rest[0] != "curl" && rest[0] != "jmx") {
		fmt.Fprintln(os.Stderr, `informe o que importar. Hoje existem dois formatos:
  braunrate import curl "curl -X POST https://exemplo/pedidos -d '{}'" -output cenario.yaml
  pbpaste | braunrate import curl
  braunrate import jmx plano.jmx -output cenario.yaml`)
		return 2
	}

	var importResult importer.Import
	var err error
	if rest[0] == "jmx" {
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "informe o arquivo .jmx:\n  braunrate import jmx plano.jmx -output cenario.yaml")
			return 2
		}
		content, readErr := os.ReadFile(rest[1])
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "nao consegui ler %s: %v\n", rest[1], readErr)
			return 2
		}
		importResult, err = importer.FromJMX(content)
	} else {
		command := strings.Join(rest[1:], " ")
		if strings.TrimSpace(command) == "" {
			read, readErr := io.ReadAll(os.Stdin)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "nao consegui ler o comando da entrada padrao: %v\n", readErr)
				return 2
			}
			command = string(read)
		}
		importResult, err = importer.FromCurl(command)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	if _, err := scenario.Parse([]byte(importResult.YAML)); err != nil {
		fmt.Fprintf(os.Stderr, "gerei um cenario que eu mesmo nao aceito; isso e defeito meu, nao do seu arquivo:\n%v\n", err)
		return 1
	}

	destination := out
	if destination == "" {
		fmt.Print(importResult.YAML)
	} else {
		if err := os.WriteFile(destination, []byte(importResult.YAML), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "nao consegui gravar %s: %v\n", destination, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "cenario gravado em %s\n", destination)
	}

	for _, warning := range importResult.Warnings {
		fmt.Fprintf(os.Stderr, "atencao: %s\n", warning)
	}
	if destination != "" {
		fmt.Fprintf(os.Stderr, "\nProximo passo, antes de qualquer carga:\n  braunrate debug %s\n", destination)
	} else {
		fmt.Fprintln(os.Stderr, "\nProximo passo: grave com -output cenario.yaml e rode 'braunrate debug cenario.yaml'.")
	}
	return 0
}

func record(args []string) int {
	set := flag.NewFlagSet("record", flag.ExitOnError)
	output := set.String("output", "", "arquivo de cenario a gravar")
	address := set.String("addr", "127.0.0.1:8888", "endereco onde o proxy escuta")
	hosts := set.String("host", "", "hosts a gravar, separados por virgula (padrao: o primeiro que aparecer)")
	ignore := set.String("ignore", "", "trechos de caminho a descartar, separados por virgula")
	parseArguments(set, args)

	if *output == "" {
		fmt.Fprintln(os.Stderr, "informe onde gravar o cenario:\n  braunrate record -output cenario.yaml")
		return 2
	}

	proxy := recorder.New(recorder.Options{
		Address: *address,
		Hosts:   splitList(*hosts),
		Ignore:  splitList(*ignore),
	})

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err := proxy.Serve(runContext, func(listening string) {
		fmt.Fprintf(os.Stderr, `gravando em %s

Aponte o cliente para este proxy e navegue pelo fluxo que voce quer medir:
  navegador: configure o proxy HTTP para %s
  curl:      curl -x http://%s http://seu-servico/pedidos
  terminal:  export http_proxy=http://%s

Duas coisas que este gravador nao sabe, e voce sabe:
  a carga e o slo saem como chute de partida, nao como medicao — ajuste antes de usar como gate
  uma sequencia gravada uma vez nao e o mix de producao: a proporcao entre as rotas e sua decisao

Ctrl+C encerra e escreve o cenario.
`, listening, listening, listening, listening)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	fmt.Fprintln(os.Stderr)

	entries := proxy.Entries()
	for _, line := range recorder.DroppedLines(proxy.Dropped()) {
		fmt.Fprintf(os.Stderr, "descartei %s\n", line)
	}
	for host, count := range proxy.Tunneled() {
		fmt.Fprintf(os.Stderr, "nao gravei %d conexao(oes) HTTPS para %s: gravar dentro de TLS exige autoridade certificadora propria, que esta fora do escopo\n", count, host)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "nenhuma requisicao gravada; nao vou escrever um cenario vazio")
		fmt.Fprintln(os.Stderr, "se o trafego era HTTPS, aponte o cliente para o endereco HTTP do servico, ou use -host para liberar o dominio certo")
		return 1
	}

	prefix := strings.TrimSuffix(filepath.Base(*output), filepath.Ext(*output))
	script, files := recorder.Build(entries, prefix)
	generated := importer.RenderYAML(script)

	if _, err := scenario.Parse([]byte(generated.YAML)); err != nil {
		fmt.Fprintf(os.Stderr, "gravei um cenario que eu mesmo nao aceito; isso e defeito meu, nao da sua navegacao:\n%v\n", err)
		return 1
	}
	if err := os.WriteFile(*output, []byte(generated.YAML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui gravar %s: %v\n", *output, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%s viraram %s em %s\n",
		texto.Count(int64(len(entries)), "requisicao", "requisicoes"),
		texto.Count(int64(len(script.Steps)), "passo", "passos"), *output)

	directory := filepath.Dir(*output)
	for index, file := range files {
		path := filepath.Join(directory, script.Data[index].File)
		if err := os.WriteFile(path, []byte(file.CSV()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "nao consegui gravar %s: %v\n", path, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "%d valor(es) observado(s) de %s em %s\n", len(file.Values), file.Name, path)
	}

	for _, warning := range generated.Warnings {
		fmt.Fprintf(os.Stderr, "atencao: %s\n", warning)
	}
	fmt.Fprintf(os.Stderr, "\nProximo passo, antes de qualquer carga:\n  braunrate debug %s\n", *output)
	return 0
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func validate(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenario")
		return runner.ExitBadFile
	}
	c, plan, err := runner.Load(args[0])
	if err != nil {
		return faultExit(err)
	}
	for _, line := range runner.Describe(c, plan) {
		fmt.Println(line)
	}
	return runner.ExitPassed
}

func serve(args []string) int {
	set := flag.NewFlagSet("serve", flag.ExitOnError)
	address := set.String("addr", "127.0.0.1:8080", "endereco de escuta")
	directory := set.String("dir", ".", "diretorio com os cenarios servidos")
	concurrent := set.Bool("concurrent", false, "permite mais de uma execucao ao mesmo tempo, aceitando a contaminacao da medicao")
	_ = parseArguments(set, args)

	if info, err := os.Stat(*directory); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s nao e um diretorio que eu consiga ler; -dir aponta para onde estao os cenarios\n", *directory)
		return runner.ExitBadFile
	}

	options := server.DefaultOptions(version)
	options.Address = *address
	options.Directory = *directory
	options.Concurrent = *concurrent

	httpServer := server.New(options)
	for _, line := range httpServer.StartupWarning() {
		fmt.Fprintln(os.Stderr, line)
	}
	if err := httpServer.Listen(); err != nil {
		fmt.Fprintf(os.Stderr, "o servidor parou: %v\n", err)
		return runner.ExitBadFile
	}
	return runner.ExitPassed
}

func serveTarget(args []string) int {
	set := flag.NewFlagSet("target", flag.ExitOnError)
	address := set.String("address", "127.0.0.1:8080", "endereco de escuta")
	latency := set.Duration("latency", 5*time.Millisecond, "latencia fixa por requisicao")
	jitter := set.Duration("jitter", 0, "variacao aleatoria somada a latencia")
	freezeAfter := set.Duration("freeze-after", 0, "instante em que o alvo congela")
	freezeFor := set.Duration("freeze-for", 0, "duracao do congelamento")
	brokers := set.String("kafka", "", "brokers do Kafka para subir tambem o processador assincrono")
	input := set.String("input", "pedidos", "topico consumido pelo processador")
	out := set.String("output", "pedidos-processados", "topico publicado pelo processador")
	processorDelay := set.Duration("processor-delay", 20*time.Millisecond, "quanto o processador demora por mensagem")
	_ = set.Parse(args)

	server := testsupport.New(testsupport.Options{
		Latency:     *latency,
		Jitter:      *jitter,
		FreezeAfter: *freezeAfter,
		FreezeFor:   *freezeFor,
	})
	if err := server.Start(*address); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao subir alvo: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "alvo de teste em %s (latencia %s)\n", server.Address(), *latency)

	var processor *testsupport.Processor
	if *brokers != "" {
		processor = testsupport.NewProcessor(testsupport.ProcessorOptions{
			Brokers: strings.Split(*brokers, ","),
			Input:   *input,
			Output:  *out,
			Delay:   *processorDelay,
		})
		// The HTTP target stays up even if the async processor does not: killing
		// everything here turned one broken piece into every scenario failing
		// with an authentication error, which points at the wrong place.
		if err := processor.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "ATENCAO: o processador assincrono nao subiu: %v\n", err)
			fmt.Fprintln(os.Stderr, "         o alvo HTTP continua no ar; cenario com 'aguardar' vai falhar por timeout")
			processor = nil
		} else {
			fmt.Fprintf(os.Stderr, "processador assincrono: %s -> %s, %s por mensagem\n", *input, *out, *processorDelay)
		}
	}

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-runContext.Done()
	_ = server.Close()
	if processor != nil {
		_ = processor.Close()
		fmt.Fprintf(os.Stderr, "\nmensagens processadas: %d", processor.Processed())
	}
	fmt.Fprintf(os.Stderr, "\natendidas: %d\n", server.Served())
	return 0
}
