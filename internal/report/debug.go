package report

import (
	"io"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
)

const bodyLimit = 1200

// Debug exists so a broken correlation shows before the load, not after
// minutes of running.
func Debug(out io.Writer, number int, observation engine.Observation, showBody bool) error {
	output := &lineWriter{out: out}
	write := output.writef

	mark := "ok"
	if observation.Class != protocol.Success {
		mark = "FALHOU"
	}
	write("")
	write("passo %d — %s   [%s em %s]", number, observation.Step, mark, observation.Duration.Round(100_000))
	lines := describeConfig(observation.Config)
	write("  requisição: %s", lines[0])
	for _, line := range lines[1:] {
		write("              %s", shorten(line))
	}

	if observation.Response.Status > 0 {
		write("  resposta:   status %d, %d bytes", observation.Response.Status, observation.Response.Bytes)
	}
	if showBody && len(observation.Response.Body) > 0 {
		write("  corpo:      %s", shorten(string(observation.Response.Body)))
	}

	if len(observation.Captured) > 0 {
		write("  capturou:")
		for _, name := range sortNames(observation.Captured) {
			write("    %s = %s", name, shorten(observation.Captured[name]))
		}
	}

	for _, failure := range observation.Failures {
		write("  problema:   %s", failure)
	}
	if observation.Class != protocol.Success && len(observation.Failures) == 0 {
		write("  problema:   %s", className(string(observation.Class)))
		// The detail may explain in more than one line what to do next, and
		// squeezing it into one would bury the part that is not the diagnosis.
		for _, line := range strings.Split(observation.Response.Detail, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				write("              %s", shorten(line))
			}
		}
	}
	return output.err
}

func IterationVars(out io.Writer, vars map[string]string) error {
	output := &lineWriter{out: out}
	output.writef("")
	output.writef("variáveis no fim da iteração")
	for _, name := range sortNames(vars) {
		output.writef("  %s = %s", name, shorten(vars[name]))
	}
	return output.err
}

func describeConfig(config protocol.Config) []string {
	if config == nil {
		return []string{"(não montada)"}
	}
	if describable, knows := config.(protocol.Describable); knows {
		return describable.Describe()
	}
	return []string{config.AggregationKey()}
}

func shorten(text string) string {
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
