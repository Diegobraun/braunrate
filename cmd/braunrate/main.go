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
	case "executar":
		os.Exit(execute(os.Args[2:]))
	case "validar":
		os.Exit(validate(os.Args[2:]))
	case "depurar":
		os.Exit(debug(os.Args[2:]))
	case "relatorio":
		os.Exit(reportCommand(os.Args[2:]))
	case "comparar":
		os.Exit(compare(os.Args[2:]))
	case "novo":
		os.Exit(newOne(os.Args[2:]))
	case "importar":
		os.Exit(importCommand(os.Args[2:]))
	case "alvo":
		os.Exit(serveTarget(os.Args[2:]))
	case "versao":
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
  braunrate novo [cenario.yaml]         cria um cenario de partida, comentado
  braunrate depurar <cenario.yaml>      um usuario, uma iteracao, tudo visivel
  braunrate executar <cenario.yaml> [opcoes]
  braunrate validar <cenario.yaml>
  braunrate importar curl "<comando curl>"    gera um cenario a partir de um curl
  braunrate importar jmx <plano.jmx>          traduz o subconjunto comum de um plano do JMeter
  braunrate relatorio <resultado.json> [opcoes] gera HTML ou CSV de um resultado ja gravado
  braunrate comparar <antes.json> <depois.json>
  braunrate alvo [opcoes]
  braunrate versao

opcoes de executar:
  -resultado <arquivo.json>   grava o documento de resultado
  -html <arquivo.html>        grava o relatorio HTML autocontido
  -csv <arquivo.csv>          grava uma linha por passo, para planilha
  -maximo-simultaneas <n>     maximo de requisicoes simultaneas (padrao 20000)
  -atraso-tolerado <dur>      a partir daqui o gerador conta como saturado (padrao 10ms)
  -silencioso                 nao imprime progresso durante a execucao
`, version)
}

func execute(args []string) int {
	set := flag.NewFlagSet("executar", flag.ExitOnError)
	resultPath := set.String("resultado", "", "arquivo JSON de resultado")
	htmlPath := set.String("html", "", "arquivo HTML de relatorio")
	csvPath := set.String("csv", "", "arquivo CSV com uma linha por passo")
	maxInflight := set.Int64("maximo-simultaneas", 20000, "maximo de requisicoes simultaneas antes de desistir de disparar")
	lateThreshold := set.Duration("atraso-tolerado", 10*time.Millisecond, "atraso de disparo a partir do qual o gerador e considerado saturado")
	silencioso := set.Bool("silencioso", false, "nao imprime progresso")
	positional := analisar(set, args)

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

	opts := engine.DefaultOptions()
	opts.Version = version
	opts.MaxInflight = *maxInflight
	opts.DataRoot = filepath.Dir(scenarioPath)
	opts.LateThreshold = *lateThreshold
	if !*silencioso {
		opts.OnProgress = func(snapshot metrics.Snapshot, targetRate float64, remaining time.Duration) {
			fmt.Fprintf(os.Stderr, "\r%s", report.ProgressLine(snapshot, targetRate, remaining))
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	m, err := engine.New(c, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "executando %q contra %s: %s iteracoes em %s\n",
		c.Name, c.Target, humanizar(m.Plan().TotalRequests()), m.Plan().Duration())

	document := m.Execute(ctx)
	protocol.CloseAll()
	if !*silencioso {
		fmt.Fprintln(os.Stderr)
	}
	verdict := slo.Evaluate(c.SLO, document)
	document.SLO = verdict
	report.Summary(os.Stdout, document, verdict)

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
	if !verdict.Passed {
		return 1
	}
	return 0
}

// O flag padrao para de ler opcao no primeiro argumento posicional, entao
// "executar cenario.yaml -html x.html" ignorava a opcao em silencio. Aqui a
// lista e percorrida ate o fim, com as opcoes valendo antes ou depois do
// arquivo.
func analisar(set *flag.FlagSet, args []string) []string {
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
	defer file.Close()
	if err := report.HTML(file, document); err != nil {
		return fmt.Errorf("erro ao gerar o relatorio HTML: %v", err)
	}
	return nil
}

func writeCSVFile(path string, document metrics.Document) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar %s: %v", path, err)
	}
	defer file.Close()
	if err := report.CSV(file, document); err != nil {
		return fmt.Errorf("erro ao gerar o CSV: %v", err)
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
		return document, fmt.Errorf("%s nao foi gerado pelo braunrate; use o arquivo de -resultado", path)
	}
	if document.FormatVersion != metrics.VersaoDoFormatoDeResultado {
		return document, fmt.Errorf("%s esta no formato de resultado %q e esta versao le o formato %q",
			path, document.FormatVersion, metrics.VersaoDoFormatoDeResultado)
	}
	return document, nil
}

func reportCommand(args []string) int {
	set := flag.NewFlagSet("relatorio", flag.ExitOnError)
	htmlPath := set.String("html", "", "arquivo HTML a gerar")
	csvPath := set.String("csv", "", "arquivo CSV a gerar")
	positional := analisar(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, `informe o resultado gravado, por exemplo:
  braunrate relatorio saida.json -html relatorio.html`)
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
  braunrate comparar antes.json depois.json`)
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
	report.Comparison(os.Stdout, result)
	if !result.Comparable {
		return 3
	}
	return 0
}

func humanizar(value int64) string {
	return fmt.Sprintf("%d", value)
}

func debug(args []string) int {
	set := flag.NewFlagSet("depurar", flag.ExitOnError)
	showBody := set.Bool("corpo", true, "mostra o corpo das respostas")
	positional := analisar(set, args)

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

	opts := engine.DefaultOptions()
	opts.Version = version
	opts.DataRoot = filepath.Dir(scenarioPath)

	m, err := engine.New(c, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	fmt.Printf("depurando %q contra %s: 1 usuario, 1 iteracao, sem carga\n", c.Name, c.Target)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	observations, vars, err := m.Debug(ctx)
	protocol.CloseAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nnao consegui chegar ao primeiro passo: %v\n", err)
		return 1
	}

	failed := false
	for index, observation := range observations {
		report.Debug(os.Stdout, index+1, observation, *showBody)
		if observation.Class != protocol.Success {
			failed = true
		}
	}
	report.IterationVars(os.Stdout, vars)

	fmt.Println()
	if failed {
		fmt.Printf("A iteracao parou no passo %d. Corrija e rode de novo; a carga so vale depois que a iteracao passa.\n", len(observations))
		return 1
	}
	if len(observations) < len(c.Steps) {
		fmt.Println("A iteracao nao chegou ao fim.")
		return 1
	}
	fmt.Printf("Iteracao completa: %d passo(s), tudo certo. Para rodar com carga:\n  braunrate executar %s\n",
		len(observations), scenarioPath)
	return 0
}

// O comando curl chega como um argumento so, cheio de aspas, e a pessoa cola
// a opcao antes ou depois dele; o flag padrao para de ler no primeiro
// argumento posicional e perderia a opcao colada no fim.
func newOne(args []string) int {
	destination := "cenario.yaml"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		destination = args[0]
	}
	if _, err := os.Stat(destination); err == nil {
		fmt.Fprintf(os.Stderr, "%s ja existe; escolha outro nome:\n  braunrate novo outro-cenario.yaml\n", destination)
		return 2
	}
	if err := os.WriteFile(destination, []byte(importer.Skeleton()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "nao consegui gravar %s: %v\n", destination, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "cenario de partida em %s: troque o alvo e o caminho pelo seu servico.\n", destination)
	fmt.Fprintf(os.Stderr, "\nProximo passo, antes de qualquer carga:\n  braunrate depurar %s\n", destination)
	return 0
}

func importCommand(args []string) int {
	out := ""
	var rest []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-saida" || arg == "--saida":
			if index+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "a opcao -saida ficou sem o nome do arquivo")
				return 2
			}
			index++
			out = args[index]
		case strings.HasPrefix(arg, "-saida="):
			out = strings.TrimPrefix(arg, "-saida=")
		case strings.HasPrefix(arg, "--saida="):
			out = strings.TrimPrefix(arg, "--saida=")
		default:
			rest = append(rest, arg)
		}
	}

	if len(rest) < 1 || (rest[0] != "curl" && rest[0] != "jmx") {
		fmt.Fprintln(os.Stderr, `informe o que importar. Hoje existem dois formatos:
  braunrate importar curl "curl -X POST https://exemplo/pedidos -d '{}'" -saida cenario.yaml
  pbpaste | braunrate importar curl
  braunrate importar jmx plano.jmx -saida cenario.yaml`)
		return 2
	}

	var importResult importer.Import
	var err error
	if rest[0] == "jmx" {
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "informe o arquivo .jmx:\n  braunrate importar jmx plano.jmx -saida cenario.yaml")
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
		fmt.Fprintf(os.Stderr, "\nProximo passo, antes de qualquer carga:\n  braunrate depurar %s\n", destination)
	} else {
		fmt.Fprintln(os.Stderr, "\nProximo passo: grave com -saida cenario.yaml e rode 'braunrate depurar cenario.yaml'.")
	}
	return 0
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
	fmt.Printf("Cenario valido: %q, %d passo(s), %d iteracoes em %s.\n",
		c.Name, len(c.Steps), plan.TotalRequests(), plan.Duration())
	if len(c.SLO) == 0 {
		fmt.Println("Sem slo declarado: a execucao nunca vai falhar por lentidao. Adicione um bloco 'slo' para virar gate de CI.")
	}
	return 0
}

func serveTarget(args []string) int {
	set := flag.NewFlagSet("alvo", flag.ExitOnError)
	address := set.String("endereco", "127.0.0.1:8080", "endereco de escuta")
	latency := set.Duration("latencia", 5*time.Millisecond, "latencia fixa por requisicao")
	jitter := set.Duration("jitter", 0, "variacao aleatoria somada a latencia")
	freezeAfter := set.Duration("congelar-apos", 0, "instante em que o alvo congela")
	freezeFor := set.Duration("congelar-por", 0, "duracao do congelamento")
	brokers := set.String("kafka", "", "brokers do Kafka para subir tambem o processador assincrono")
	input := set.String("entrada", "pedidos", "topico consumido pelo processador")
	out := set.String("saida", "pedidos-processados", "topico publicado pelo processador")
	processorDelay := set.Duration("atraso-do-processador", 20*time.Millisecond, "quanto o processador demora por mensagem")
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
		if err := processor.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "erro ao subir o processador: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "processador assincrono: %s -> %s, %s por mensagem\n", *input, *out, *processorDelay)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	_ = server.Close()
	if processor != nil {
		_ = processor.Close()
		fmt.Fprintf(os.Stderr, "\nmensagens processadas: %d", processor.Processed())
	}
	fmt.Fprintf(os.Stderr, "\natendidas: %d\n", server.Atendidas())
	return 0
}
