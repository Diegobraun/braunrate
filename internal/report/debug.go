package report

import (
	"io"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
)

const bodyLimit = 1200

// Debug exists so a broken correlation shows before the load, not after
// minutes of running.
func Debug(out io.Writer, number int, observation engine.Observation, showBody bool) error {
	output := &lineWriter{out: out}
	write := output.writef

	mark := "ok"
	if observation.Class != protocol.Success {
		mark = "FAILED"
	}
	write("")
	write("step %d — %s   [%s in %s]", number, observation.Step, mark, observation.Duration.Round(100_000))
	lines := describeConfig(observation.Config)
	write("  request:    %s", lines[0])
	for _, line := range lines[1:] {
		write("              %s", shorten(line))
	}

	if observation.Response.Status > 0 {
		write("  response:   status %d, %d bytes", observation.Response.Status, observation.Response.Bytes)
	}
	if showBody && len(observation.Response.Body) > 0 {
		write("  body:       %s", shorten(transport.MaskBody(string(observation.Response.Body))))
	}

	if len(observation.Captured) > 0 {
		write("  captured:")
		captured := MaskedVars(observation.Captured)
		for _, name := range sortNames(captured) {
			write("    %s = %s", name, shorten(captured[name]))
		}
	}

	for _, failure := range observation.Failures {
		write("  problem:    %s", failure)
	}
	if observation.Class != protocol.Success && len(observation.Failures) == 0 {
		write("  problem:    %s", className(string(observation.Class)))
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

// A scenario with no capture and no declared variable ends the iteration with
// nothing to show, and the header printed over the empty space read like a
// section that failed to fill.
func IterationVars(out io.Writer, vars map[string]string) error {
	if len(vars) == 0 {
		return nil
	}
	output := &lineWriter{out: out}
	output.writef("")
	output.writef("variables at the end of the iteration")
	masked := MaskedVars(vars)
	for _, name := range sortNames(masked) {
		output.writef("  %s = %s", name, shorten(masked[name]))
	}
	return output.err
}

// MaskedVars is the single place the variables of an iteration lose their
// secrets. O terminal e o modo servidor entregam o mesmo mapa por caminhos
// diferentes, e o servidor montava a resposta dele sem passar por corte nenhum.
func MaskedVars(vars map[string]string) map[string]string {
	masked := make(map[string]string, len(vars))
	for name, value := range vars {
		masked[name] = transport.MaskSecret(name, value)
	}
	return masked
}

func describeConfig(config protocol.Config) []string {
	if config == nil {
		return []string{"(not built)"}
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
