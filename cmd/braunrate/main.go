package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
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
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/testsupport"
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
  braunrate compare <antes.json> <depois.json>
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
		return 2
	}
	scenarioPath := positional[0]

	c, err := scenario.ParseFile(scenarioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no cenario: %v\n", err)
		return 2
	}
	if err := c.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	options := engine.DefaultOptions()
	options.Version = version
	options.MaxInflight = *maxInflight
	options.DataRoot = filepath.Dir(scenarioPath)
	options.LateThreshold = *lateThreshold
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

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	m, err := engine.New(c, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	if c.Load.Closed() {
		fmt.Fprintf(os.Stderr, "executando %q contra %s: %d usuarios em laco fechado durante %s\n",
			c.Name, c.Target, c.Load.Users, m.Plan().Duration())
	} else {
		fmt.Fprintf(os.Stderr, "executando %q contra %s: %s iteracoes em %s\n",
			c.Name, c.Target, humanize(m.Plan().TotalRequests()), m.Plan().Duration())
	}

	document := m.Execute(runContext)
	protocol.CloseAll()
	if !*quiet {
		fmt.Fprintln(os.Stderr)
	}
	var baseline *slo.Baseline
	if *baselinePath != "" {
		before, err := readDocument(*baselinePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 2
		}
		baseline = &slo.Baseline{Comparison: comparison.Compare(before, document), Path: *baselinePath}
	}
	if document.Valid() {
		document.SLO = slo.Evaluate(c.SLO, document, baseline)
	}
	// A failed write to stdout does not change what happened to the target, so
	// the verdict below stands; what cannot happen is the report vanishing in
	// silence.
	if err := report.Summary(os.Stdout, document, document.SLO); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui escrever o resumo: %v\n", err)
	}

	if *resultPath != "" {
		if err := writeJSON(*resultPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
	}
	if *htmlPath != "" {
		if err := writeHTML(*htmlPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "relatorio em %s\n", *htmlPath)
	}
	if *csvPath != "" {
		if err := writeCSVFile(*csvPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
	}

	if !document.Valid() {
		return 3
	}
	if !document.SLO.Passed {
		return 1
	}
	return 0
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

func writeJSON(path string, document metrics.Document) error {
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("erro ao gravar resultado: %v", err)
	}
	return nil
}

func writeHTML(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar %s: %v", path, err)
	}
	if err := report.HTML(file, document); err != nil {
		_ = file.Close()
		return fmt.Errorf("erro ao gerar o relatorio HTML: %v", err)
	}
	// Close reports the write the operating system had not flushed yet. Deferred
	// and discarded, a full disk produced a truncated file and a message saying
	// the report was ready.
	if err := file.Close(); err != nil {
		return fmt.Errorf("erro ao fechar %s, o relatorio pode estar incompleto: %v", path, err)
	}
	return nil
}

func writeCSVFile(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar %s: %v", path, err)
	}
	if err := report.CSV(file, document); err != nil {
		_ = file.Close()
		return fmt.Errorf("erro ao gerar o CSV: %v", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("erro ao fechar %s, o CSV pode estar incompleto: %v", path, err)
	}
	return nil
}

func readDocument(path string) (metrics.Document, error) {
	var document metrics.Document
	content, err := os.ReadFile(path)
	if err != nil {
		return document, fmt.Errorf("nao consegui ler %s: %v", path, err)
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return document, fmt.Errorf("%s nao e um resultado do braunrate: %v", path, err)
	}
	if document.Tool != "braunrate" {
		return document, fmt.Errorf("%s nao foi gerado pelo braunrate; use o arquivo de -result", path)
	}
	if document.FormatVersion != metrics.VersaoDoFormatoDeResultado {
		return document, fmt.Errorf("%s esta no formato de resultado %q e esta versao le o formato %q",
			path, document.FormatVersion, metrics.VersaoDoFormatoDeResultado)
	}
	return document, nil
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
	document, err := readDocument(positional[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	if *htmlPath == "" && *csvPath == "" {
		*htmlPath = "relatorio.html"
	}
	if *htmlPath != "" {
		if err := writeHTML(*htmlPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Printf("relatorio em %s\n", *htmlPath)
	}
	if *csvPath != "" {
		if err := writeCSVFile(*csvPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Printf("csv em %s\n", *csvPath)
	}
	return 0
}

func compare(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, `informe as duas execucoes, a antiga primeiro:
  braunrate compare antes.json depois.json`)
		return 2
	}
	before, err := readDocument(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	after, err := readDocument(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	result := comparison.Compare(before, after)
	if err := report.Comparison(os.Stdout, result); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui escrever a comparacao: %v\n", err)
		return 2
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
		return 2
	}
	scenarioPath := positional[0]

	c, err := scenario.ParseFile(scenarioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no cenario: %v\n", err)
		return 2
	}
	if err := c.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	options := engine.DefaultOptions()
	options.Version = version
	options.DataRoot = filepath.Dir(scenarioPath)

	m, err := engine.New(c, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	fmt.Printf("depurando %q contra %s: 1 usuario, 1 iteracao, sem carga\n", c.Name, c.Target)
	for _, line := range scenario.DescribeMessaging(c.Messaging) {
		fmt.Printf("mensageria: %s\n", line)
	}

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	observations, vars, err := m.Debug(runContext)
	protocol.CloseAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nnao consegui chegar ao primeiro passo: %v\n", err)
		return 1
	}

	failed := false
	for index, observation := range observations {
		if err := report.Debug(os.Stdout, index+1, observation, *showBody); err != nil {
			fmt.Fprintf(os.Stderr, "nao consegui escrever a depuracao: %v\n", err)
			return 2
		}
		if observation.Class != protocol.Success {
			failed = true
		}
	}
	if err := report.IterationVars(os.Stdout, vars); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui escrever a depuracao: %v\n", err)
		return 2
	}

	fmt.Println()
	if failed {
		fmt.Printf("A iteracao parou no passo %d. Corrija e rode de novo; a carga so vale depois que a iteracao passa.\n", len(observations))
		return 1
	}
	if len(observations) < len(c.Steps) {
		fmt.Println("A iteracao nao chegou ao fim.")
		return 1
	}
	fmt.Printf("Iteracao completa: %d passo(s), tudo certo. Para rodar com carga:\n  braunrate execute %s\n",
		len(observations), scenarioPath)
	return 0
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
	fmt.Fprintf(os.Stderr, "%d requisicoes viraram %d passo(s) em %s\n", len(entries), len(script.Steps), *output)

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
		return 2
	}
	c, err := scenario.ParseFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no cenario: %v\n", err)
		return 2
	}
	if err := c.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	plan := engine.CompilePlan(c.Load)
	if c.Load.Closed() {
		fmt.Printf("Cenario valido: %q, %d passo(s), %d usuarios em laco fechado durante %s.\n",
			c.Name, len(c.Steps), c.Load.Users, plan.Duration())
	} else {
		fmt.Printf("Cenario valido: %q, %d passo(s), %d iteracoes em %s.\n",
			c.Name, len(c.Steps), plan.TotalRequests(), plan.Duration())
	}
	if warning, closed := scenario.ClosedModelWarning(c); closed {
		fmt.Println(warning)
	}
	if len(c.SLO) == 0 {
		fmt.Println("Sem slo declarado: a execucao nunca vai falhar por lentidao. Adicione um bloco 'slo' para virar gate de CI.")
	}
	for _, broker := range scenario.DescribeMessaging(c.Messaging) {
		fmt.Printf("Mensageria: %s\n", broker)
	}
	if len(c.Requires) > 0 {
		fmt.Printf("Depende de infraestrutura externa: %s. Sem isso a execucao nao roda.\n", strings.Join(c.Requires, ", "))
	}
	for _, warning := range scenario.GateWarnings(c) {
		fmt.Println(warning)
	}
	return 0
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
	fmt.Fprintf(os.Stderr, "\natendidas: %d\n", server.Atendidas())
	return 0
}
