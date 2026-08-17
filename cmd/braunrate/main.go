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
	"github.com/Diegobraun/braunrate/internal/text"
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
	case "help", "-h", "--help":
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
	case "migrate":
		os.Exit(migrate(os.Args[2:]))
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
		fmt.Printf("braunrate %s\ncommit: %s\ndate: %s\ncompiled protocols: %v\n",
			build.Version, build.Commit, build.Date, protocol.Registered())
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "%q is not a braunrate command.\n", os.Args[1])
		if best, found := text.Closest(os.Args[1], commands); found {
			fmt.Fprintf(os.Stderr, "Did you mean %q?\n", best)
		}
		fmt.Fprintln(os.Stderr, "\nEvery command:  braunrate help")
		os.Exit(2)
	}
}

var commands = []string{
	"demo", "new", "migrate", "debug", "execute", "validate", "import", "record",
	"report", "compare", "serve", "ui", "target", "version", "help",
}

// A catalog in alphabetical order serves whoever already knows what they want.
// Nobody arriving now can guess that the path is new, debug and only then
// execute, nor that target exists for whoever has nothing to test against.
func firstScreen(out io.Writer) {
	_, _ = fmt.Fprintf(out, `braunrate %s — load testing that does not lie about its own result

Never used it? See it working in 30 seconds:

    braunrate demo

Already have an API to test? The path is:

    1. braunrate import curl 'curl https://your-api/orders -H "Authorization: ..."'
       (or: braunrate new scenario.yaml, to start from scratch)

    2. braunrate debug scenario.yaml
       runs once and shows everything: what went out, what came back, what failed

    3. braunrate execute scenario.yaml
       now the real load

Every command:  braunrate help
`, build.Version)
}

// Os comandos que recebem um arquivo nao passam por um FlagSet, e sem isto '-h'
// virava nome de arquivo: 'validate -h' procurava um arquivo chamado "-h" e
// 'new -h' escrevia scenario.yaml no diretorio de quem so queria ler a ajuda.
// O site promete que toda opcao aceita -h, e agora aceita.
func askedForHelp(args []string) bool {
	for _, argument := range args {
		switch argument {
		case "-h", "--help", "-help":
			return true
		}
	}
	return false
}

func usage(out io.Writer) {
	_, _ = fmt.Fprintf(out, `braunrate %s

usage:
  braunrate demo [--with-failure]               starts a target, runs a scenario and explains the numbers
  braunrate new [scenario.yaml]                 creates a starting scenario, commented
  braunrate migrate <scenario.yaml|dir>         converts a scenario in the Portuguese format to English
  braunrate debug <scenario.yaml>               one user, one iteration, everything visible
  braunrate execute <scenario.yaml> [options]
  braunrate validate <scenario.yaml>
  braunrate import curl "<curl command>"        generates a scenario from a curl
  braunrate import jmx <plan.jmx>               translates the common subset of a JMeter plan
  braunrate record -output <scenario.yaml>      records a scenario from whatever goes through the proxy
  braunrate report <result.json> [options]      generates HTML or CSV from a result already saved
  braunrate compare <before.json> <after.json> [-html <file.html>]
  braunrate serve [-addr :8080] [-dir ./scenarios]  the same commands over HTTP, local
  braunrate ui [-addr :8080] [-dir ./scenarios]     edit and run the scenarios in the browser
  braunrate target [options]
  braunrate version

execute options:
  -result <file.json>         writes the result document
  -html <file.html>           writes the self-contained HTML report
  -csv <file.csv>             writes one line per step, for a spreadsheet
  -max-concurrent <n>         maximum simultaneous requests (default 20000)
  -late-threshold <dur>       past this the generator did not sustain the rate (default 10ms)
  -quiet                      does not print progress during the run
`, build.Version)
}

func runDemo(args []string) int {
	set := newFlagSet("demo")
	withFailure := set.Bool("with-failure", false, "runs against a target that freezes halfway, and compares with the closed loop")
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
	resultPath := set.String("result", "", "JSON result file")
	htmlPath := set.String("html", "", "HTML report file")
	csvPath := set.String("csv", "", "CSV file with one line per step")
	maxInflight := set.Int64("max-concurrent", 20000, "maximum simultaneous requests before giving up on firing")
	lateThreshold := set.Duration("late-threshold", 10*time.Millisecond, "dispatch delay past which the generator counts as saturated")
	quiet := set.Bool("quiet", false, "does not print progress")
	baselinePath := set.String("baseline", "", "result of a previous run, for the regression rules")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "name the scenario file")
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
		fmt.Fprintf(os.Stderr, "running %q against %s: %d users in a closed loop for %s\n",
			c.Name, c.Target, c.Load.Users, plan.Duration())
	} else {
		fmt.Fprintf(os.Stderr, "running %q against %s: %s iterations in %s\n",
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
		fmt.Fprintf(os.Stderr, "I could not write the summary: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "report at %s\n", *htmlPath)
	}
	// -quiet is the off switch: whoever runs this in a pipeline already knows the
	// next step, and the line turns into log noise. A failed run gets no hint
	// either: the next step there is to fix it, and the SLO block already said what.
	if !*quiet && result.Exit == runner.ExitPassed {
		fmt.Fprintf(os.Stderr, "\n%s\n", nextAfterExecute(scenarioPath, *resultPath, *htmlPath))
	}

	return result.Exit
}

// A number on its own does not answer "did it get better?". What to do next
// depends on what this run already saved, and saying so costs one line.
func nextAfterExecute(scenarioPath, resultPath, htmlPath string) string {
	if resultPath == "" {
		return fmt.Sprintf("To compare against the next run, keep this result:\n  braunrate execute %s -result before.json", scenarioPath)
	}
	if htmlPath == "" {
		return fmt.Sprintf("Next step:\n  braunrate report %s -html report.html\n  braunrate compare %s after.json   (after the next run)", resultPath, resultPath)
	}
	return fmt.Sprintf("After the next run, to know whether it got better:\n  braunrate compare %s after.json", resultPath)
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
// argument, so "execute scenario.yaml -html x.html" ignored the option
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
		fmt.Fprintf(os.Stderr, "%s options:\n", set.Name())
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

	message := fmt.Sprintf("%q does not exist.", "-"+received)
	if best, found := text.Closest(received, known); found {
		message += fmt.Sprintf(" Did you mean %q?\n\n    braunrate %s %s\n",
			"-"+best, set.Name(), strings.Join(corrected(args, received, best), " "))
	} else {
		message += "\n"
	}
	return message + fmt.Sprintf("\nEvery option: braunrate %s -h\n", set.Name())
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
	htmlPath := set.String("html", "", "HTML file to generate")
	csvPath := set.String("csv", "", "CSV file to generate")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, `name the saved result, for example:
  braunrate report result.json -html report.html`)
		return 2
	}
	document, err := runner.ReadDocument(positional[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	if *htmlPath == "" && *csvPath == "" {
		*htmlPath = "report.html"
	}
	if *htmlPath != "" {
		if err := runner.WriteHTML(*htmlPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Printf("report at %s\n", *htmlPath)
		fmt.Printf("\nOpen it in the browser; it is self-contained and fetches nothing from the network.\n")
	}
	if *csvPath != "" {
		if err := runner.WriteCSV(*csvPath, document); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Printf("csv at %s\n", *csvPath)
	}
	return 0
}

func compare(args []string) int {
	set := newFlagSet("compare")
	htmlPath := set.String("html", "", "writes the comparison as self-contained HTML")
	positional := parseArguments(set, args)

	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, `name the two runs, the older one first:
  braunrate compare before.json after.json`)
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
		fmt.Fprintf(os.Stderr, "I could not write the comparison: %v\n", err)
		return 2
	}
	if *htmlPath != "" {
		if err := runner.WriteComparisonHTML(*htmlPath, result, after.Version); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 2
		}
		fmt.Printf("comparison at %s\n", *htmlPath)
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
	showBody := set.Bool("body", true, "shows the response bodies")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "name the scenario file")
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

	fmt.Printf("debugging %q against %s: 1 user, 1 iteration, no load\n", c.Name, c.Target)
	for _, line := range scenario.DescribeMessaging(c.Messaging) {
		fmt.Printf("messaging: %s\n", line)
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
			fmt.Fprintf(os.Stderr, "I could not write the debug output: %v\n", err)
			return runner.ExitBadFile
		}
	}
	if err := report.IterationVars(os.Stdout, iteration.Vars); err != nil {
		fmt.Fprintf(os.Stderr, "I could not write the debug output: %v\n", err)
		return runner.ExitBadFile
	}

	fmt.Println()
	if !iteration.Complete() {
		if len(iteration.Observations) < len(c.Steps) && !iteration.Failed() {
			fmt.Println("The iteration did not reach the end.")
		} else {
			fmt.Printf("The iteration stopped at step %d. Fix it and run again; the load is only worth running after the iteration passes.\n",
				len(iteration.Observations))
		}
		if c.Auth == nil && refusedForCredentials(iteration) {
			fmt.Print(`
The target refused for lack of a credential, and the scenario declares no auth
at all. Declare where the token comes from and braunrate obtains it once and
reuses it across every journey:

  auth:
    type: token
    obtain:
      http: { method: POST, path: /auth/token, body: { user: ana } }
      capture: { token: $.access_token }
`)
		}
		if unreachable(iteration) {
			fmt.Printf("\nNobody answered at %s. Check that the service is up and that the address is right.\n"+
				"If you do not have a service to test yet, start the built-in one in another terminal:\n"+
				"  braunrate target\n", c.Target)
		}
		return runner.ExitSLO
	}
	fmt.Printf("Iteration complete: %s, all good. To run it with load:\n  braunrate execute %s\n",
		text.Count(int64(len(iteration.Observations)), "step", "steps"), scenarioPath)
	return runner.ExitPassed
}

// A 401 with no authentication block is not a mystery to whoever wrote the
// scenario knowing the tool has one. It is a wall to everyone else, and the
// body of the answer says "missing token" without saying where to declare it.
func refusedForCredentials(iteration runner.Iteration) bool {
	for _, observation := range iteration.Observations {
		if observation.Response.Status == http.StatusUnauthorized || observation.Response.Status == http.StatusForbidden {
			return true
		}
	}
	return false
}

// "network failure / connection refused" says what the operating system saw, not
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
	if askedForHelp(args) {
		fmt.Println("braunrate new [scenario.yaml]    creates a starting scenario, commented")
		fmt.Println("\n  With no name it writes scenario.yaml in the current folder.")
		fmt.Println("  The file comes with the $schema line, so the editor completes the keys.")
		return 0
	}
	destination := "scenario.yaml"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		destination = args[0]
	}
	if _, err := os.Stat(destination); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists; pick another name:\n  braunrate new another-scenario.yaml\n", destination)
		return 2
	}
	if err := os.WriteFile(destination, []byte(importer.Skeleton()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "I could not write %s: %v\n", destination, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "starting scenario at %s: swap the target and the path for your service.\n", destination)
	fmt.Fprintf(os.Stderr, "\nNext step, before any load:\n  braunrate debug %s\n", destination)
	return 0
}

func importCommand(args []string) int {
	out := ""
	var rest []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-output":
			if index+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "the -output option came with no file name")
				return 2
			}
			index++
			out = args[index]
		case strings.HasPrefix(arg, "-output="):
			out = strings.TrimPrefix(arg, "-output=")
		default:
			rest = append(rest, arg)
		}
	}

	if len(rest) < 1 || (rest[0] != "curl" && rest[0] != "jmx") {
		fmt.Fprintln(os.Stderr, `say what to import. There are two formats today:
  braunrate import curl "curl -X POST https://example/orders -d '{}'" -output scenario.yaml
  pbpaste | braunrate import curl
  braunrate import jmx plan.jmx -output scenario.yaml`)
		return 2
	}

	var importResult importer.Import
	var err error
	if rest[0] == "jmx" {
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "name the .jmx file:\n  braunrate import jmx plan.jmx -output scenario.yaml")
			return 2
		}
		content, readErr := os.ReadFile(rest[1])
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "I could not read %s: %v\n", rest[1], readErr)
			return 2
		}
		importResult, err = importer.FromJMX(content)
	} else {
		command := strings.Join(rest[1:], " ")
		if strings.TrimSpace(command) == "" {
			read, readErr := io.ReadAll(os.Stdin)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "I could not read the command from standard input: %v\n", readErr)
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
		fmt.Fprintf(os.Stderr, "I generated a scenario I do not accept myself; that is my defect, not your file's:\n%v\n", err)
		return 1
	}

	destination := out
	if destination == "" {
		fmt.Print(importResult.YAML)
	} else {
		if err := os.WriteFile(destination, []byte(importResult.YAML), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "I could not write %s: %v\n", destination, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "scenario written to %s\n", destination)
	}

	for _, warning := range importResult.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	if destination != "" {
		fmt.Fprintf(os.Stderr, "\nNext step, before any load:\n  braunrate debug %s\n", destination)
	} else {
		fmt.Fprintln(os.Stderr, "\nNext step: save it with -output scenario.yaml and run 'braunrate debug scenario.yaml'.")
	}
	return 0
}

func record(args []string) int {
	set := newFlagSet("record")
	output := set.String("output", "", "scenario file to write")
	address := set.String("addr", "127.0.0.1:8888", "address the proxy listens on")
	hosts := set.String("host", "", "hosts to record, comma separated (default: the first one that shows up)")
	ignore := set.String("ignore", "", "path fragments to drop, comma separated")
	parseArguments(set, args)

	if *output == "" {
		fmt.Fprintln(os.Stderr, "say where to write the scenario:\n  braunrate record -output scenario.yaml")
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
		fmt.Fprintf(os.Stderr, `recording at %s

Point the client at this proxy and walk through the flow you want to measure:
  browser:  set the HTTP proxy to %s
  curl:     curl -x http://%s http://your-service/orders
  terminal: export http_proxy=http://%s

Two things this recorder does not know, and you do:
  the load and the slo come out as a starting guess, not as a measurement — tune them before using it as a gate
  a sequence recorded once is not the production mix: the proportion between routes is your call

Ctrl+C stops and writes the scenario.
`, listening, listening, listening, listening)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	fmt.Fprintln(os.Stderr)

	entries := proxy.Entries()
	for _, line := range recorder.DroppedLines(proxy.Dropped()) {
		fmt.Fprintf(os.Stderr, "dropped %s\n", line)
	}
	for host, count := range proxy.Tunneled() {
		fmt.Fprintf(os.Stderr, "I did not record %d HTTPS connection(s) to %s: recording inside TLS needs a certificate authority of our own, which is out of scope\n", count, host)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "no request recorded; I will not write an empty scenario")
		fmt.Fprintln(os.Stderr, "if the traffic was HTTPS, point the client at the HTTP address of the service, or use -host to allow the right domain")
		return 1
	}

	prefix := strings.TrimSuffix(filepath.Base(*output), filepath.Ext(*output))
	script, files := recorder.Build(entries, prefix)
	generated := importer.RenderYAML(script)

	if _, err := scenario.Parse([]byte(generated.YAML)); err != nil {
		fmt.Fprintf(os.Stderr, "I recorded a scenario I do not accept myself; that is my defect, not your browsing:\n%v\n", err)
		return 1
	}
	if err := os.WriteFile(*output, []byte(generated.YAML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "I could not write %s: %v\n", *output, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%s became %s in %s\n",
		text.Count(int64(len(entries)), "request", "requests"),
		text.Count(int64(len(script.Steps)), "step", "steps"), *output)

	directory := filepath.Dir(*output)
	for index, file := range files {
		path := filepath.Join(directory, script.Data[index].File)
		if err := os.WriteFile(path, []byte(file.CSV()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "I could not write %s: %v\n", path, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "%d observed value(s) of %s in %s\n", len(file.Values), file.Name, path)
	}

	for _, warning := range generated.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	fmt.Fprintf(os.Stderr, "\nNext step, before any load:\n  braunrate debug %s\n", *output)
	return 0
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func validate(args []string) int {
	if askedForHelp(args) {
		fmt.Println("braunrate validate <scenario.yaml>    reads the scenario and says what it will do")
		fmt.Println("\n  Loads the file, refuses what the engine would refuse, and prints how many")
		fmt.Println("  iterations it plans. It does not send a single request.")
		return 0
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "name the scenario file")
		return runner.ExitBadFile
	}
	c, plan, err := runner.Load(args[0])
	if err != nil {
		return faultExit(err)
	}
	for _, line := range runner.Describe(c, plan) {
		fmt.Println(line)
	}
	fmt.Printf("\nBefore running the load, check that the scenario does what you expect:\n  braunrate debug %s\n", args[0])
	return runner.ExitPassed
}

func serve(args []string) int {
	set := newFlagSet("serve")
	address := set.String("addr", "127.0.0.1:8080", "address to listen on")
	directory := set.String("dir", ".", "directory holding the scenarios served")
	concurrent := set.Bool("concurrent", false, "allows more than one run at a time, accepting that the measurements contaminate each other")
	_ = parseArguments(set, args)

	if info, err := os.Stat(*directory); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s is not a directory I can read; -dir points at where the scenarios are\n", *directory)
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
		fmt.Fprintf(os.Stderr, "the server stopped: %v\n", err)
		return runner.ExitBadFile
	}
	return runner.ExitPassed
}

// A busy port is the first error for whoever starts two processes, and "bind:
// address already in use" does not say what to do to someone who never picked
// a port.
func portInUse(command, flagName, address string, err error) int {
	if !errors.Is(err, syscall.EADDRINUSE) {
		fmt.Fprintf(os.Stderr, "I could not listen on %s: %v\n", address, err)
		return runner.ExitBadFile
	}
	fmt.Fprintf(os.Stderr, "%s is already taken by another process. Pick another port:\n"+
		"  braunrate %s %s 127.0.0.1:8081\n", address, command, flagName)
	return runner.ExitBadFile
}

// The interface is the same server as 'serve', with writing enabled and the
// screens mounted at the root: whatever it can do, it does by editing the file
// under -dir.
func userInterface(args []string) int {
	set := newFlagSet("ui")
	address := set.String("addr", "127.0.0.1:8080", "address to listen on")
	directory := set.String("dir", ".", "directory holding the scenarios edited")
	concurrent := set.Bool("concurrent", false, "allows more than one run at a time, accepting that the measurements contaminate each other")
	open := set.Bool("open", true, "opens the browser on the interface")
	_ = parseArguments(set, args)

	if info, err := os.Stat(*directory); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s is not a directory I can read; -dir points at where the scenarios are\n", *directory)
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

// Opening the browser is a convenience: if the system does not know how, the
// address is already on screen and the interface stays up.
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
	address := set.String("address", "127.0.0.1:8080", "address to listen on")
	latency := set.Duration("latency", 5*time.Millisecond, "fixed latency per request")
	jitter := set.Duration("jitter", 0, "random variation added to the latency")
	freezeAfter := set.Duration("freeze-after", 0, "instant at which the target freezes")
	freezeFor := set.Duration("freeze-for", 0, "how long the freeze lasts")
	brokers := set.String("kafka", "", "Kafka brokers, to also start the async processor")
	input := set.String("input", "orders", "topic the processor consumes")
	out := set.String("output", "orders-processed", "topic the processor publishes to")
	processorDelay := set.Duration("processor-delay", 20*time.Millisecond, "how long the processor takes per message")
	raw := set.Bool("raw", false, "minimal target: answers without reading the request, to measure the generator ceiling")
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
	fmt.Fprintf(os.Stderr, "test target at %s (answers in %s)\n", server.Address(), *latency)
	fmt.Fprintf(os.Stderr, "\nIn another terminal, write a scenario and run it against this target:\n"+
		"  braunrate new scenario.yaml\n  braunrate debug scenario.yaml\n"+
		"If you only want to see the tool working, you do not need this command:\n  braunrate demo\n\n")

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
			fmt.Fprintf(os.Stderr, "WARNING: the async processor did not start: %v\n", err)
			fmt.Fprintln(os.Stderr, "         the HTTP target stays up; a scenario with 'await' will fail on timeout")
			processor = nil
		} else {
			fmt.Fprintf(os.Stderr, "async processor: %s -> %s, %s per message\n", *input, *out, *processorDelay)
		}
	}

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-runContext.Done()
	_ = server.Close()
	if processor != nil {
		_ = processor.Close()
		fmt.Fprintf(os.Stderr, "\nmessages processed: %d", processor.Processed())
	}
	fmt.Fprintf(os.Stderr, "\nserved: %d\n", server.Served())
	return 0
}

// The minimal target exists to measure the generator, not to exercise a
// scenario: it does not route, does not read the method and answers 200 to
// anything. Measuring the ceiling against the full target measures the
// generator+target pair, which is the caveat Phase 0 had already registered.
func serveRawTarget(address string) int {
	server := testsupport.NewRaw()
	if err := server.Start(address); err != nil {
		return portInUse("target -raw", "-address", address, err)
	}
	fmt.Fprintf(os.Stderr, "minimal target at %s: answers 200 without reading the request\n", server.Address())
	fmt.Fprintln(os.Stderr, "use it only to measure the generator ceiling — it checks no route, method or body")

	runContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-runContext.Done()
	_ = server.Close()
	fmt.Fprintf(os.Stderr, "\nserved: %d\n", server.Served())
	return 0
}

// Without a migration the format change locks everyone into 0.5.0, including
// whoever wrote this. Overwriting in place with a .bak beside it is the default
// because the usual case is a folder with thirty scenarios and one command.
func migrate(args []string) int {
	set := newFlagSet("migrate")
	output := set.String("output", "", "writes the converted scenario to this file instead of overwriting")
	dryRun := set.Bool("dry-run", false, "shows what would change and writes nothing")
	positional := parseArguments(set, args)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, `name the scenario or the folder to convert:
  braunrate migrate scenario.yaml
  braunrate migrate ./scenarios/ -dry-run`)
		return runner.ExitBadFile
	}

	files, err := scenariosUnder(positional[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return runner.ExitBadFile
	}
	if *output != "" && len(files) > 1 {
		fmt.Fprintf(os.Stderr, "-output writes one file, and %s holds %d scenarios; drop -output to convert them in place\n",
			positional[0], len(files))
		return runner.ExitBadFile
	}

	converted := 0
	for _, path := range files {
		changed, err := migrateOne(path, *output, *dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return runner.ExitBadFile
		}
		if changed {
			converted++
		}
	}
	if converted == 0 {
		fmt.Fprintln(os.Stderr, "\nnothing to convert: every scenario is already in the English format.")
		return runner.ExitPassed
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "\n%s would change. Run without -dry-run to write.\n", text.Count(int64(converted), "file", "files"))
		return runner.ExitPassed
	}
	// With -output the converted scenario is somewhere else, and pointing the
	// next step at the original would send the reader to validate the file that
	// was not converted.
	next := files[0]
	if *output != "" {
		next = *output
	}
	fmt.Fprintf(os.Stderr, "\n%s converted. Next step:\n  braunrate validate %s\n",
		text.Count(int64(converted), "file", "files"), next)
	return runner.ExitPassed
}

func migrateOne(path, output string, dryRun bool) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("could not read %s: %v", path, err)
	}
	rewritten, changes, err := scenario.Migrate(content)
	if err != nil {
		return false, fmt.Errorf("%s: %v", path, err)
	}
	if len(changes) == 0 {
		fmt.Fprintf(os.Stderr, "%s: already in the English format\n", path)
		return false, nil
	}

	fmt.Fprintf(os.Stderr, "%s: %s\n", path, text.Count(int64(len(changes)), "key changes", "keys change"))
	for _, change := range changes {
		fmt.Fprintf(os.Stderr, "  %s\n", change)
	}
	if dryRun {
		return true, nil
	}

	// The converted file goes through the new parser before anything is written:
	// a migration that produces a scenario the tool refuses is worse than no
	// migration, because the original is gone by then.
	converted, err := scenario.Parse(rewritten)
	if err != nil {
		return false, fmt.Errorf("converted %s into a scenario I do not accept myself; that is my defect, not your file's:\n%v", path, err)
	}
	// A step with no declared name takes the name of what it does, and that name
	// changed with the format: an slo rule pointing at "aguardar pedidos" no
	// longer matches "await pedidos". Renaming the rule would be guessing at the
	// author's text, so what the migration does is say so.
	if err := converted.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "  the converted scenario does not validate yet:\n%v\n", err)
	}

	destination := output
	if destination == "" {
		destination = path
		if err := os.WriteFile(path+".bak", content, 0o644); err != nil {
			return false, fmt.Errorf("could not write the backup %s.bak: %v", path, err)
		}
		fmt.Fprintf(os.Stderr, "  original kept at %s.bak\n", path)
	}
	if err := os.WriteFile(destination, rewritten, 0o644); err != nil {
		return false, fmt.Errorf("could not write %s: %v", destination, err)
	}
	return true, nil
}

func scenariosUnder(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("could not find %s", path)
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("could not read the folder %s: %v", path, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if extension := filepath.Ext(entry.Name()); extension != ".yaml" && extension != ".yml" {
			continue
		}
		files = append(files, filepath.Join(path, entry.Name()))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .yaml file in %s", path)
	}
	return files, nil
}
