package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	case "target":
		os.Exit(serveTarget(os.Args[2:]))
	case "version":
		fmt.Printf("braunrate %s\ncommit: %s\ndata: %s\nprotocolos compilados: %v\n",
			build.Version, build.Commit, build.Date, protocol.Registered())
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "%q nao e um comando do braunrate.\n", os.Args[1])
		if best, found := texto.Closest(os.Args[1], commands); found {
			fmt.Fprintf(os.Stderr, "Voce quis dizer %q?\n", best)
		}
		fmt.Fprintln(os.Stderr, "\nTodos os comandos:  braunrate ajuda")
		os.Exit(2)
	}
}

var commands = []string{
	"demo", "new", "debug", "execute", "validate", "import", "record",
	"report", "compare", "serve", "target", "version", "ajuda",
}

// A catalog in alphabetical order serves whoever already knows what they want.
// Nobody arriving now can guess that the path is new, debug and only then
// execute, nor that target exists for whoever has nothing to test against.
func firstScreen(out io.Writer) {
	_, _ = fmt.Fprintf(out, `braunrate %s — teste de carga com medicao honesta

Nunca usou? Veja funcionando em 30 segundos:

    braunrate demo

Ja tem uma API para testar? O caminho e:

    1. braunrate import curl 'curl https://sua-api/pedidos -H "Authorization: ..."'
       (ou: braunrate new cenario.yaml, para comecar do zero)

    2. braunrate debug cenario.yaml
       roda uma vez so e mostra tudo: o que foi enviado, o que voltou, o que falhou

    3. braunrate execute cenario.yaml
       agora sim, a carga de verdade

Todos os comandos:  braunrate ajuda
`, build.Version)
}

func usage(out io.Writer) {
	_, _ = fmt.Fprintf(out, `braunrate %s

uso:
  braunrate demo [--com-falha]                  sobe um alvo, roda um cenario e explica os numeros
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
  -late-threshold <dur>       a partir daqui o gerador nao sustentou a taxa (padrao 10ms)
  -quiet                      nao imprime progresso durante a execucao
`, build.Version)
}

func runDemo(args []string) int {
	set := newFlagSet("demo")
	withFailure := set.Bool("com-falha", false, "roda contra um alvo que trava no meio, e compara com o laco fechado")
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
		return fmt.Sprintf("Para comparar com a proxima execucao, guarde este resultado:\n  braunrate execute %s -result antes.json", scenarioPath)
	}
	if htmlPath == "" {
		return fmt.Sprintf("Proximo passo:\n  braunrate report %s -html relatorio.html\n  braunrate compare %s depois.json   (depois da proxima execucao)", resultPath, resultPath)
	}
	return fmt.Sprintf("Depois da proxima execucao, para saber se melhorou:\n  braunrate compare %s depois.json", resultPath)
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
		fmt.Fprintf(os.Stderr, "opcoes de %s:\n", set.Name())
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

	message := fmt.Sprintf("%q nao existe.", "-"+received)
	if best, found := texto.Closest(received, known); found {
		message += fmt.Sprintf(" Voce quis dizer %q?\n\n    braunrate %s %s\n",
			"-"+best, set.Name(), strings.Join(corrected(args, received, best), " "))
	} else {
		message += "\n"
	}
	return message + fmt.Sprintf("\nTodas as opcoes: braunrate %s -h\n", set.Name())
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
		fmt.Printf("\nAbra no navegador; ele e autocontido e nao busca nada na rede.\n")
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
	set := newFlagSet("debug")
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

	iteration, err := runner.Debug(runContext, scenarioPath, build.Version)
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
		if c.Auth == nil && refusedForCredentials(iteration) {
			fmt.Print(`
O alvo recusou por credencial, e o cenario nao declara autenticacao nenhuma.
Declare de onde vem o token e o braunrate obtem uma vez e reaproveita em todas
as jornadas:

  autenticacao:
    tipo: token
    obter:
      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
      captura: { token: $.access_token }
`)
		}
		if unreachable(iteration) {
			fmt.Printf("\nNinguem atendeu em %s. Confira se o servico esta no ar e se o endereco esta certo.\n"+
				"Se voce ainda nao tem um servico para testar, suba o embutido em outro terminal:\n"+
				"  braunrate target\n", c.Target)
		}
		return runner.ExitSLO
	}
	fmt.Printf("Iteracao completa: %s, tudo certo. Para rodar com carga:\n  braunrate execute %s\n",
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
	set := newFlagSet("record")
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
	fmt.Printf("\nAntes de rodar a carga, veja se o cenario faz o que voce espera:\n  braunrate debug %s\n", args[0])
	return runner.ExitPassed
}

func serve(args []string) int {
	set := newFlagSet("serve")
	address := set.String("addr", "127.0.0.1:8080", "endereco de escuta")
	directory := set.String("dir", ".", "diretorio com os cenarios servidos")
	concurrent := set.Bool("concurrent", false, "permite mais de uma execucao ao mesmo tempo, aceitando a contaminacao da medicao")
	_ = parseArguments(set, args)

	if info, err := os.Stat(*directory); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s nao e um diretorio que eu consiga ler; -dir aponta para onde estao os cenarios\n", *directory)
		return runner.ExitBadFile
	}

	options := server.DefaultOptions(build.Version)
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
	set := newFlagSet("target")
	address := set.String("address", "127.0.0.1:8080", "endereco de escuta")
	latency := set.Duration("latency", 5*time.Millisecond, "latencia fixa por requisicao")
	jitter := set.Duration("jitter", 0, "variacao aleatoria somada a latencia")
	freezeAfter := set.Duration("freeze-after", 0, "instante em que o alvo congela")
	freezeFor := set.Duration("freeze-for", 0, "duracao do congelamento")
	brokers := set.String("kafka", "", "brokers do Kafka para subir tambem o processador assincrono")
	input := set.String("input", "pedidos", "topico consumido pelo processador")
	out := set.String("output", "pedidos-processados", "topico publicado pelo processador")
	processorDelay := set.Duration("processor-delay", 20*time.Millisecond, "quanto o processador demora por mensagem")
	raw := set.Bool("raw", false, "alvo minimo: responde sem interpretar a requisicao, para medir o teto do gerador")
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
		fmt.Fprintf(os.Stderr, "erro ao subir alvo: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "alvo de teste em %s (responde em %s)\n", server.Address(), *latency)
	fmt.Fprintf(os.Stderr, "\nEm outro terminal, aponte um cenario para ca:\n  braunrate execute examples/ci.yaml\n"+
		"Se voce so quer ver a ferramenta funcionando, nao precisa deste comando:\n  braunrate demo\n\n")

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

// O alvo minimo existe para medir o gerador, nao para exercitar cenario: ele
// nao roteia, nao le metodo e responde 200 a qualquer coisa. Medir o teto contra
// o alvo completo mede o par gerador+alvo, que e a ressalva que a Fase 0 ja
// tinha registrado e nunca teve como remover.
func serveRawTarget(address string) int {
	server := testsupport.NewRaw()
	if err := server.Start(address); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao subir alvo minimo: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "alvo minimo em %s: responde 200 sem interpretar a requisicao\n", server.Address())
	fmt.Fprintln(os.Stderr, "use so para medir o teto do gerador — ele nao valida rota, metodo nem corpo")

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-runContext.Done()
	_ = server.Close()
	fmt.Fprintf(os.Stderr, "\natendidas: %d\n", server.Served())
	return 0
}
