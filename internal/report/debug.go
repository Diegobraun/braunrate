package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
)

const bodyLimit = 1200

// Debug exists so a broken correlation shows before the load, not after
// minutes of running.
func Debug(out io.Writer, number int, observation engine.Observation, showBody bool) {
	write := func(format string, args ...any) {
		fmt.Fprintf(out, format+"\n", args...)
	}

	mark := "ok"
	if observation.Class != protocol.Success {
		mark = "FALHOU"
	}
	write("")
	write("passo %d — %s   [%s em %s]", number, observation.Step, mark, observation.Duration.Round(100_000))
	lines := describeConfig(observation.Config)
	write("  requisicao: %s", lines[0])
	for _, line := range lines[1:] {
		write("              %s", encurtar(line))
	}

	if observation.Response.Status > 0 {
		write("  resposta:   status %d, %d bytes", observation.Response.Status, observation.Response.Bytes)
	}
	if showBody && len(observation.Response.Body) > 0 {
		write("  corpo:      %s", encurtar(string(observation.Response.Body)))
	}

	if len(observation.Captured) > 0 {
		write("  capturou:")
		for _, name := range sortNames(observation.Captured) {
			write("    %s = %s", name, encurtar(observation.Captured[name]))
		}
	}

	for _, failure := range observation.Failures {
		write("  problema:   %s", failure)
	}
	if observation.Class != protocol.Success && len(observation.Failures) == 0 {
		write("  problema:   %s", classNames[string(observation.Class)])
	}
}

func IterationVars(out io.Writer, vars map[string]string) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "variaveis no fim da iteracao")
	for _, name := range sortNames(vars) {
		fmt.Fprintf(out, "  %s = %s\n", name, encurtar(vars[name]))
	}
}

func describeConfig(config protocol.Config) []string {
	if config == nil {
		return []string{"(nao montada)"}
	}
	if describable, knows := config.(protocol.Describable); knows {
		return describable.Describe()
	}
	return []string{config.AggregationKey()}
}

func encurtar(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len(text) > bodyLimit {
		return text[:bodyLimit] + "…"
	}
	return text
}

func sortNames(values map[string]string) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
