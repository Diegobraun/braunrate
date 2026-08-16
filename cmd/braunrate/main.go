package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Diegobraun/braunrate/internal/build"
	"github.com/Diegobraun/braunrate/internal/demo"
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
	"github.com/Diegobraun/braunrate/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		firstScreen(os.Stdout)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "demo":
		os.Exit(runDemo(os.Args[2:]))
	case "ajuda", "help", "-h", "--help":
		usage(os.Stdout)
		os.Exit(0)
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
	case "ui":
		os.Exit(userInterface(os.Args[2:]))
	case "target":
		os.Exit(serveTarget(os.Args[2:]))
	case "version":
		fmt.Printf("braunrate %s\ncommit: %s\ndata: %s\nprotocolos compilados: %v\n",
			build.Version, build.Commit, build.Date, protocol.Registered())
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "%q não é um comando do braunrate.\n", os.Args[1])
		if best, found := texto.Closest(os.Args[1], commands); found {
			fmt.Fprintf(os.Stderr, "Você quis dizer %q?\n", best)
		}
		fmt.Fprintln(os.Stderr, "\nTodos os comandos:  braunrate ajuda")
		os.Exit(2)
	}
}

var commands = []string{
	"demo", "new", "debug", "execute", "validate", "import", "record",
	"report", "compare", "serve", "ui", "target", "version", "ajuda",
}

// A catalog in alphabetical order serves whoever already knows what they want.
// Nobody arriving now can guess that the path is new, debug and only then
// execute, nor that target exists for whoever has nothing to test against.
func firstScreen(out io.Writer) {
	_, _ = fmt.Fprintf(out, `braunrate %s — teste de carga com medição honesta

Nunca usou? Veja funcionando em 30 segundos:

    braunrate demo

Já tem uma API para testar? O caminho é:

    1. braunrate import curl 'curl https://sua-api/pedidos -H "Authorization: ..."'
       (ou: braunrate new cenario.yaml, para começar do zero)

    2. braunrate debug cenario.yaml
       roda uma vez só e mostra tudo: o que foi enviado, o que voltou, o que falhou

    3. braunrate execute cenario.yaml
       agora sim, a carga de verdade

Todos os comandos:  braunrate ajuda
`, build.Version)
}

func usage(out io.Writer) {
	_, _ = fmt.Fprintf(out, `braunrate %s

uso:
  braunrate demo [--com-falha]                  sobe um alvo, roda um cenário e explica os números
  braunrate new [cenario.yaml]                  cria um cenário de partida, comentado
  braunrate debug <cenario.yaml>                um usuário, uma iteração, tudo visível
  braunrate execute <cenario.yaml> [opções]
  braunrate validate <cenario.yaml>
  braunrate import curl "<comando curl>"        gera um cenário a partir de um curl
  braunrate import jmx <plano.jmx>              traduz o subconjunto comum de um plano do JMeter
  braunrate record -output <cenario.yaml>       grava um cenário a partir do que passar pelo proxy
  braunrate report <resultado.json> [opções]    gera HTML ou CSV de um resultado já gravado
  braunrate compare <antes.json> <depois.json> [-html <arquivo.html>]
  braunrate serve [-addr :8080] [-dir ./cenarios]   os mesmos comandos por HTTP, local
  braunrate ui [-addr :8080] [-dir ./cenarios]      edita e roda os cenários no navegador
  braunrate target [opções]
  braunrate version

opções de execute:
  -result <arquivo.json>      grava o documento de resultado
  -html <arquivo.html>        grava o relatório HTML autocontido
  -csv <arquivo.csv>          grava uma linha por passo, para planilha
  -max-concurrent <n>         máximo de requisições simultâneas (padrão 20000)
  -late-threshold <dur>       a partir daqui o gerador não sustentou a taxa (padrão 10ms)
  -quiet                      não imprime progresso durante a execução
`, build.Version)
}

func runDemo(args []string) int {
	set := newFlagSet("demo")
	withFailure := set.Bool("com-falha", false, "roda contra um alvo que trava no meio, e compara com o laço fechado")
	_ = parseArguments(set, args)

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err := demo.Run(runContext, demo.Options{
		WithFailure: *withFailure,
		Directory:   ".",
		Version:     build.Version,
		Output:      os.Stdout,
	})
	if err != nil {
		return faultExit(err)
	}
	return runner.ExitPassed
}

func execute(args []string) int {
	set := newFlagSet("execute")
	resultPath := set.String("result", "", "arquivo JSON de resultado")
	htmlPath := set.String("html", "", "arquivo HTML de relatório")
	csvPath := set.String("csv", "", "arquivo CSV com uma linha por passo")
	maxInflight := set.Int64("max-concurrent", 20000, "máximo de requisições simultâneas antes de desistir de disparar")
	lateThreshold := set.Duration("late-threshold", 10*time.Millisecond, "atraso de disparo a partir do qual o gerador é considerado saturado")
	quiet := set.Bool("quiet", false, "não imprime progresso")
	baselinePath := set.String("baseline", "", "resultado de uma execução anterior, para as regras de regressão")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenário")
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

	options := runner.DefaultOptions(build.Version)
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
		fmt.Fprintf(os.Stderr, "executando %q contra %s: %d usuários em laço fechado durante %s\n",
			c.Name, c.Target, c.Load.Users, plan.Duration())
	} else {
		fmt.Fprintf(os.Stderr, "executando %q contra %s: %s iterações em %s\n",
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
		fmt.Fprintf(os.Stderr, "não consegui escrever o resumo: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "relatório em %s\n", *htmlPath)
	}
	// -quiet e o desligamento: quem roda em esteira ja sabe o proximo passo, e
	// a linha vira ruido em log. Execucao reprovada tambem nao ganha a dica: o
	// proximo passo ali e corrigir, e o bloco de SLO ja disse o que.
	if !*quiet && result.Exit == runner.ExitPassed {
		fmt.Fprintf(os.Stderr, "\n%s\n", nextAfterExecute(scenarioPath, *resultPath, *htmlPath))
	}

	return result.Exit
}

// Um numero sozinho nao responde "melhorou?". A proxima coisa a fazer depende
// do que esta execucao ja gravou, e dizer isso custa uma linha.
func nextAfterExecute(scenarioPath, resultPath, htmlPath string) string {
	if resultPath == "" {
		return fmt.Sprintf("Para comparar com a próxima execução, guarde este resultado:\n  braunrate execute %s -result antes.json", scenarioPath)
	}
	if htmlPath == "" {
		return fmt.Sprintf("Próximo passo:\n  braunrate report %s -html relatório.html\n  braunrate compare %s depois.json   (depois da próxima execução)", resultPath, resultPath)
	}
	return fmt.Sprintf("Depois da próxima execução, para saber se melhorou:\n  braunrate compare %s depois.json", resultPath)
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

// ContinueOnError with the output silenced, so a wrong option gets the
// suggestion below instead of the ten-line dump the flag package prints.
func newFlagSet(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

// The standard flag package stops reading options at the first positional
// argument, so "executar cenario.yaml -html x.html" ignored the option
// silently. Here the list is walked to the end, and options hold before or
// after the file.
func parseArguments(set *flag.FlagSet, args []string) []string {
	var positional []string
	original := args
	for {
		if err := set.Parse(args); err != nil {
			// Exiting here is what flag.ExitOnError already did; what changed is
			// what gets printed on the way out.
			refuseFlag(set, original, err)
			return positional
		}
		if set.NArg() == 0 {
			return positional
		}
		positional = append(positional, set.Arg(0))
		args = set.Args()[1:]
	}
}

func refuseFlag(set *flag.FlagSet, args []string, err error) {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintf(os.Stderr, "opções de %s:\n", set.Name())
		set.SetOutput(os.Stderr)
		set.PrintDefaults()
		os.Exit(0)
	}
	fmt.Fprint(os.Stderr, unknownFlagMessage(set, args, err))
	os.Exit(runner.ExitBadFile)
}

const notDefined = "flag provided but not defined: -"

// The tool already knew how to answer this: the suggestion by edit distance
// existed for scenario keys while "braunrate target -addr" got the whole option
// list. Same posture, same wording.
func unknownFlagMessage(set *flag.FlagSet, args []string, err error) string {
	_, received, isUnknown := strings.Cut(err.Error(), notDefined)
	if !isUnknown {
		return err.Error() + "\n"
	}

	var known []string
	set.VisitAll(func(f *flag.Flag) { known = append(known, f.Name) })

	message := fmt.Sprintf("%q não existe.", "-"+received)
	if best, found := texto.Closest(received, known); found {
		message += fmt.Sprintf(" Você quis dizer %q?\n\n    braunrate %s %s\n",
			"-"+best, set.Name(), strings.Join(corrected(args, received, best), " "))
	} else {
		message += "\n"
	}
	return message + fmt.Sprintf("\nTodas as opções: braunrate %s -h\n", set.Name())
}

// The command comes back ready to paste, with the typo replaced and everything
// else the person had typed still in place.
func corrected(args []string, received, best string) []string {
	fixed := make([]string, 0, len(args))
	for _, argument := range args {
		name := strings.TrimLeft(argument, "-")
		value := ""
		if cut, after, found := strings.Cut(name, "="); found {
			name, value = cut, "="+after
		}
		if name == received {
			argument = "-" + best + value
		}
		fixed = append(fixed, argument)
	}
	return fixed
}

func reportCommand(args []string) int {
	set := newFlagSet("report")
	htmlPath := set.String("html", "", "arquivo HTML a gerar")
	csvPath := set.String("csv", "", "arquivo CSV a gerar")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, `informe o resultado gravado, por exemplo:
  braunrate report saída.json -html relatório.html`)
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
		fmt.Printf("relatório em %s\n", *htmlPath)
		fmt.Printf("\nAbra no navegador; ele é autocontido e não busca nada na rede.\n")
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
	set := newFlagSet("compare")
	htmlPath := set.String("html", "", "grava a comparação em HTML autocontido")
	positional := parseArguments(set, args)

	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, `informe as duas execuções, a antiga primeiro:
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
		fmt.Fprintf(os.Stderr, "não consegui escrever a comparação: %v\n", err)
		return 2
	}
	if *htmlPath != "" {
		if err := runner.WriteComparisonHTML(*htmlPath, result, after.Version); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 2
		}
		fmt.Printf("comparação em %s\n", *htmlPath)
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
	set := newFlagSet("debug")
	showBody := set.Bool("body", true, "mostra o corpo das respostas")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenário")
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

	fmt.Printf("depurando %q contra %s: 1 usuário, 1 iteração, sem carga\n", c.Name, c.Target)
	for _, line := range scenario.DescribeMessaging(c.Messaging) {
		fmt.Printf("mensageria: %s\n", line)
	}

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	iteration, err := runner.Debug(runContext, scenarioPath, build.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return faultExit(err)
	}

	for index, observation := range iteration.Observations {
		if err := report.Debug(os.Stdout, index+1, observation, *showBody); err != nil {
			fmt.Fprintf(os.Stderr, "não consegui escrever a depuracao: %v\n", err)
			return runner.ExitBadFile
		}
	}
	if err := report.IterationVars(os.Stdout, iteration.Vars); err != nil {
		fmt.Fprintf(os.Stderr, "não consegui escrever a depuracao: %v\n", err)
		return runner.ExitBadFile
	}

	fmt.Println()
	if !iteration.Complete() {
		if len(iteration.Observations) < len(c.Steps) && !iteration.Failed() {
			fmt.Println("A iteração não chegou ao fim.")
		} else {
			fmt.Printf("A iteração parou no passo %d. Corrija e rode de novo; a carga só vale depois que a iteração passa.\n",
				len(iteration.Observations))
		}
		if c.Auth == nil && refusedForCredentials(iteration) {
			fmt.Print(`
O alvo recusou por credencial, e o cenário não declara autenticação nenhuma.
Declare de onde vem o token e o braunrate obtém uma vez e reaproveita em todas
as jornadas:

  autenticacao:
    tipo: token
    obter:
      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
      captura: { token: $.access_token }
`)
		}
		if unreachable(iteration) {
			fmt.Printf("\nNinguém atendeu em %s. Confira se o serviço está no ar e se o endereço está certo.\n"+
				"Se você ainda não tem um serviço para testar, suba o embutido em outro terminal:\n"+
				"  braunrate target\n", c.Target)
		}
		return runner.ExitSLO
	}
	fmt.Printf("Iteração completa: %s, tudo certo. Para rodar com carga:\n  braunrate execute %s\n",
		texto.Count(int64(len(iteration.Observations)), "passo", "passos"), scenarioPath)
	return runner.ExitPassed
}

// A 401 with no authentication block is not a mystery to whoever wrote the
// scenario knowing the tool has one. It is a wall to everyone else, and the
// body of the answer says "token ausente" without saying where to declare it.
func refusedForCredentials(iteration runner.Iteration) bool {
	for _, observation := range iteration.Observations {
		if observation.Response.Status == http.StatusUnauthorized || observation.Response.Status == http.StatusForbidden {
			return true
		}
	}
	return false
}

// "falha de rede / connection refused" says what the operating system saw, not
// what to do about it — and whoever is on their first scenario has no reason to
// know that a target has to be running somewhere.
func unreachable(iteration runner.Iteration) bool {
	for _, observation := range iteration.Observations {
		if observation.Class == protocol.ErrNetwork {
			return true
		}
	}
	return false
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
		fmt.Fprintf(os.Stderr, "%s já existe; escolha outro nome:\n  braunrate new outro-cenario.yaml\n", destination)
		return 2
	}
	if err := os.WriteFile(destination, []byte(importer.Skeleton()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "não consegui gravar %s: %v\n", destination, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "cenário de partida em %s: troque o alvo e o caminho pelo seu serviço.\n", destination)
	fmt.Fprintf(os.Stderr, "\nPróximo passo, antes de qualquer carga:\n  braunrate debug %s\n", destination)
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
				fmt.Fprintln(os.Stderr, "a opção -output ficou sem o nome do arquivo")
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
			fmt.Fprintf(os.Stderr, "não consegui ler %s: %v\n", rest[1], readErr)
			return 2
		}
		importResult, err = importer.FromJMX(content)
	} else {
		command := strings.Join(rest[1:], " ")
		if strings.TrimSpace(command) == "" {
			read, readErr := io.ReadAll(os.Stdin)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "não consegui ler o comando da entrada padrão: %v\n", readErr)
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
		fmt.Fprintf(os.Stderr, "gerei um cenário que eu mesmo não aceito; isso é defeito meu, não do seu arquivo:\n%v\n", err)
		return 1
	}

	destination := out
	if destination == "" {
		fmt.Print(importResult.YAML)
	} else {
		if err := os.WriteFile(destination, []byte(importResult.YAML), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "não consegui gravar %s: %v\n", destination, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "cenário gravado em %s\n", destination)
	}

	for _, warning := range importResult.Warnings {
		fmt.Fprintf(os.Stderr, "atenção: %s\n", warning)
	}
	if destination != "" {
		fmt.Fprintf(os.Stderr, "\nPróximo passo, antes de qualquer carga:\n  braunrate debug %s\n", destination)
	} else {
		fmt.Fprintln(os.Stderr, "\nPróximo passo: grave com -output cenario.yaml e rode 'braunrate debug cenario.yaml'.")
	}
	return 0
}

func record(args []string) int {
	set := newFlagSet("record")
	output := set.String("output", "", "arquivo de cenário a gravar")
	address := set.String("addr", "127.0.0.1:8888", "endereço onde o proxy escuta")
	hosts := set.String("host", "", "hosts a gravar, separados por virgula (padrão: o primeiro que aparecer)")
	ignore := set.String("ignore", "", "trechos de caminho a descartar, separados por virgula")
	parseArguments(set, args)

	if *output == "" {
		fmt.Fprintln(os.Stderr, "informe onde gravar o cenário:\n  braunrate record -output cenario.yaml")
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

Aponte o cliente para este proxy e navegue pelo fluxo que você quer medir:
  navegador: configure o proxy HTTP para %s
  curl:      curl -x http://%s http://seu-servico/pedidos
  terminal:  export http_proxy=http://%s

Duas coisas que este gravador não sabe, e você sabe:
  a carga e o slo saem como chute de partida, não como medição — ajuste antes de usar como gate
  uma sequência gravada uma vez não é o mix de produção: a proporção entre as rotas e sua decisão

Ctrl+C encerra e escreve o cenário.
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
		fmt.Fprintf(os.Stderr, "não gravei %d conexão(ões) HTTPS para %s: gravar dentro de TLS exige autoridade certificadora própria, que está fora do escopo\n", count, host)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "nenhuma requisição gravada; não vou escrever um cenário vazio")
		fmt.Fprintln(os.Stderr, "se o tráfego era HTTPS, aponte o cliente para o endereço HTTP do serviço, ou use -host para liberar o domínio certo")
		return 1
	}

	prefix := strings.TrimSuffix(filepath.Base(*output), filepath.Ext(*output))
	script, files := recorder.Build(entries, prefix)
	generated := importer.RenderYAML(script)

	if _, err := scenario.Parse([]byte(generated.YAML)); err != nil {
		fmt.Fprintf(os.Stderr, "gravei um cenário que eu mesmo não aceito; isso é defeito meu, não da sua navegação:\n%v\n", err)
		return 1
	}
	if err := os.WriteFile(*output, []byte(generated.YAML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "não consegui gravar %s: %v\n", *output, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%s viraram %s em %s\n",
		texto.Count(int64(len(entries)), "requisição", "requisições"),
		texto.Count(int64(len(script.Steps)), "passo", "passos"), *output)

	directory := filepath.Dir(*output)
	for index, file := range files {
		path := filepath.Join(directory, script.Data[index].File)
		if err := os.WriteFile(path, []byte(file.CSV()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "não consegui gravar %s: %v\n", path, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "%d valor(es) observado(s) de %s em %s\n", len(file.Values), file.Name, path)
	}

	for _, warning := range generated.Warnings {
		fmt.Fprintf(os.Stderr, "atenção: %s\n", warning)
	}
	fmt.Fprintf(os.Stderr, "\nPróximo passo, antes de qualquer carga:\n  braunrate debug %s\n", *output)
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
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenário")
		return runner.ExitBadFile
	}
	c, plan, err := runner.Load(args[0])
	if err != nil {
		return faultExit(err)
	}
	for _, line := range runner.Describe(c, plan) {
		fmt.Println(line)
	}
	fmt.Printf("\nAntes de rodar a carga, veja se o cenário faz o que você espera:\n  braunrate debug %s\n", args[0])
	return runner.ExitPassed
}

func serve(args []string) int {
	set := newFlagSet("serve")
	address := set.String("addr", "127.0.0.1:8080", "endereço de escuta")
	directory := set.String("dir", ".", "diretorio com os cenários servidos")
	concurrent := set.Bool("concurrent", false, "permite mais de uma execução ao mesmo tempo, aceitando a contaminação da medição")
	_ = parseArguments(set, args)

	if info, err := os.Stat(*directory); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s não é um diretório que eu consiga ler; -dir aponta para onde estão os cenários\n", *directory)
		return runner.ExitBadFile
	}

	options := server.DefaultOptions(build.Version)
	options.Address = *address
	options.Directory = *directory
	options.Concurrent = *concurrent

	httpServer := server.New(options)
	listener, err := httpServer.Bind()
	if err != nil {
		return portInUse("serve", "-addr", *address, err)
	}
	for _, line := range httpServer.StartupWarning() {
		fmt.Fprintln(os.Stderr, line)
	}
	if err := httpServer.ServeOn(listener); err != nil {
		fmt.Fprintf(os.Stderr, "o servidor parou: %v\n", err)
		return runner.ExitBadFile
	}
	return runner.ExitPassed
}

// A porta ocupada e o primeiro erro de quem sobe dois processos, e "bind:
// address already in use" nao diz o que fazer a quem nunca escolheu porta.
func portInUse(command, flagName, address string, err error) int {
	if !errors.Is(err, syscall.EADDRINUSE) {
		fmt.Fprintf(os.Stderr, "não consegui escutar em %s: %v\n", address, err)
		return runner.ExitBadFile
	}
	fmt.Fprintf(os.Stderr, "%s já está ocupado por outro processo. Escolha outra porta:\n"+
		"  braunrate %s %s 127.0.0.1:8081\n", address, command, flagName)
	return runner.ExitBadFile
}

// A interface e o mesmo servidor do 'serve', com a gravacao ligada e as telas
// montadas na raiz: o que ela sabe fazer, ela faz editando o arquivo de -dir.
func userInterface(args []string) int {
	set := newFlagSet("ui")
	address := set.String("addr", "127.0.0.1:8080", "endereço de escuta")
	directory := set.String("dir", ".", "diretorio com os cenários editados")
	concurrent := set.Bool("concurrent", false, "permite mais de uma execução ao mesmo tempo, aceitando a contaminação da medição")
	open := set.Bool("open", true, "abre o navegador na interface")
	_ = parseArguments(set, args)

	if info, err := os.Stat(*directory); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s não é um diretório que eu consiga ler; -dir aponta para onde estão os cenários\n", *directory)
		return runner.ExitBadFile
	}

	options := server.DefaultOptions(build.Version)
	options.Address = *address
	options.Directory = *directory
	options.Concurrent = *concurrent
	options.Writable = true
	options.UI = ui.Handler()

	httpServer := server.New(options)
	listener, err := httpServer.Bind()
	if err != nil {
		return portInUse("ui", "-addr", *address, err)
	}
	for _, line := range httpServer.StartupWarning() {
		fmt.Fprintln(os.Stderr, line)
	}
	if *open {
		openBrowser("http://" + *address)
	}
	if err := httpServer.ServeOn(listener); err != nil {
		fmt.Fprintf(os.Stderr, "o servidor parou: %v\n", err)
		return runner.ExitBadFile
	}
	return runner.ExitPassed
}

// Abrir o navegador e conveniencia: se o sistema nao souber abrir, o endereco
// ja esta impresso na tela e a interface continua no ar.
func openBrowser(address string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	_ = command.Start()
}

func serveTarget(args []string) int {
	set := newFlagSet("target")
	address := set.String("address", "127.0.0.1:8080", "endereço de escuta")
	latency := set.Duration("latency", 5*time.Millisecond, "latência fixa por requisição")
	jitter := set.Duration("jitter", 0, "variação aleatória somada a latência")
	freezeAfter := set.Duration("freeze-after", 0, "instante em que o alvo congela")
	freezeFor := set.Duration("freeze-for", 0, "duração do congelamento")
	brokers := set.String("kafka", "", "brokers do Kafka para subir também o processador assíncrono")
	input := set.String("input", "pedidos", "tópico consumido pelo processador")
	out := set.String("output", "pedidos-processados", "tópico publicado pelo processador")
	processorDelay := set.Duration("processor-delay", 20*time.Millisecond, "quanto o processador demora por mensagem")
	raw := set.Bool("raw", false, "alvo mínimo: responde sem interpretar a requisição, para medir o teto do gerador")
	_ = parseArguments(set, args)

	if *raw {
		return serveRawTarget(*address)
	}

	server := testsupport.New(testsupport.Options{
		Latency:     *latency,
		Jitter:      *jitter,
		FreezeAfter: *freezeAfter,
		FreezeFor:   *freezeFor,
	})
	if err := server.Start(*address); err != nil {
		return portInUse("target", "-address", *address, err)
	}
	fmt.Fprintf(os.Stderr, "alvo de teste em %s (responde em %s)\n", server.Address(), *latency)
	fmt.Fprintf(os.Stderr, "\nEm outro terminal, escreva um cenário e rode contra este alvo:\n"+
		"  braunrate new cenario.yaml\n  braunrate debug cenario.yaml\n"+
		"Se você só quer ver a ferramenta funcionando, não precisa deste comando:\n  braunrate demo\n\n")

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
			fmt.Fprintf(os.Stderr, "ATENÇÃO: o processador assíncrono não subiu: %v\n", err)
			fmt.Fprintln(os.Stderr, "         o alvo HTTP continua no ar; cenário com 'aguardar' vai falhar por timeout")
			processor = nil
		} else {
			fmt.Fprintf(os.Stderr, "processador assíncrono: %s -> %s, %s por mensagem\n", *input, *out, *processorDelay)
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

// O alvo minimo existe para medir o gerador, nao para exercitar cenario: ele
// nao roteia, nao le metodo e responde 200 a qualquer coisa. Medir o teto contra
// o alvo completo mede o par gerador+alvo, que e a ressalva que a Fase 0 ja
// tinha registrado e nunca teve como remover.
func serveRawTarget(address string) int {
	server := testsupport.NewRaw()
	if err := server.Start(address); err != nil {
		return portInUse("target -raw", "-address", address, err)
	}
	fmt.Fprintf(os.Stderr, "alvo mínimo em %s: responde 200 sem interpretar a requisição\n", server.Address())
	fmt.Fprintln(os.Stderr, "use só para medir o teto do gerador — ele não valida rota, metodo nem corpo")

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-runContext.Done()
	_ = server.Close()
	fmt.Fprintf(os.Stderr, "\natendidas: %d\n", server.Served())
	return 0
}
