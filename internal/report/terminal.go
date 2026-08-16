package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/slo"
)

func ProgressLine(snapshot metrics.Snapshot, targetRate float64, remaining time.Duration) string {
	alert := ""
	if snapshot.Sent > 0 {
		proportion := float64(snapshot.LateDispatches) / float64(snapshot.Sent)
		if proportion >= 0.01 {
			alert = fmt.Sprintf("  ATENCAO: o gerador nao esta conseguindo manter a carga (%.1f%% em atraso)", proportion*100)
		}
	}
	return fmt.Sprintf("carga %.0f/s | enviadas %d | concluidas %d | erros %d | metade em %.1f ms | 99%% em %.1f ms | faltam %s%s",
		targetRate, snapshot.Sent, snapshot.Completed, snapshot.Errors,
		snapshot.LatencyP50Ms, snapshot.LatencyP99Ms, remaining.Round(time.Second), alert)
}

// A saida tem duas camadas: a frase em portugues comum diz o que aconteceu, e o
// numero fica logo abaixo para quem precisa dele.
func Summary(out io.Writer, document metrics.Document, verdict slo.Verdict) {
	write := func(format string, args ...any) {
		fmt.Fprintf(out, format+"\n", args...)
	}

	write("")
	write("%s — contra %s", document.Run.Spec, document.Run.Target)
	write("")

	if len(verdict.Evaluations) > 0 {
		write("%s", verdict.Sentence)
		write("")
	}

	overall := document.Overall
	duration := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(100 * time.Millisecond)
	write("O que aconteceu")
	write("  %s requisicoes em %s, %.0f por segundo, %s de erro",
		thousands(overall.Count), duration, overall.EffectiveRate, percentage(overall.ErrorRate*100))
	write("  Metade das respostas em ate %s; 95%% em ate %s; 99%% em ate %s; a pior levou %s",
		milliseconds(overall.Latency.P50), milliseconds(overall.Latency.P95),
		milliseconds(overall.Latency.P99), milliseconds(overall.Latency.Max))
	write("")

	if document.Journey.Started > 0 {
		write("A jornada inteira")
		write("  %s", document.Journey.Sentence)
		write("  metade %s | 95%% %s | 99%% %s | pior %s",
			milliseconds(document.Journey.Latency.P50), milliseconds(document.Journey.Latency.P95),
			milliseconds(document.Journey.Latency.P99), milliseconds(document.Journey.Latency.Max))
		write("")
	}

	write("Por passo")
	write("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7s", "passo", "", "requisicoes", "metade", "95%", "99%", "99,9%", "pior", "erros")
	hasServiceStep := false
	for _, step := range document.Steps {
		mark := "(1)"
		if step.LatencyKind == string(metrics.ServiceLatency) {
			mark = "(2)"
			hasServiceStep = true
		}
		write("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7d",
			trim(step.Name, 26), mark, thousands(step.Count),
			milliseconds(step.Latency.P50), milliseconds(step.Latency.P95),
			milliseconds(step.Latency.P99), milliseconds(step.Latency.P999),
			milliseconds(step.Latency.Max), step.Errors)
	}
	write("")
	write("  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui")
	write("      qualquer atraso e por isso nao esconde travada do alvo.")
	if hasServiceStep {
		write("  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como")
		write("      esse passo depende do valor capturado antes dele, nao existe instante")
		write("      agendado proprio. Para a leitura honesta da jornada, use \"A jornada inteira\".")
	}
	write("")

	if len(verdict.Evaluations) > 0 {
		write("SLO")
		for _, evaluation := range verdict.Evaluations {
			mark := "ok  "
			if !evaluation.Passed {
				mark = "FALHA"
			}
			write("  %-5s %s", mark, evaluation.Sentence)
		}
		write("")
	}

	errors := errorsByClass(document)
	if len(errors) > 0 {
		write("Erros")
		for _, line := range errors {
			write("  %-50s %s", line.class, thousands(line.count))
		}
		write("")
	}

	write("Confiabilidade da medicao")
	for _, warning := range document.Warnings {
		if warning.Severity == metrics.SeverityHigh {
			write("  RESULTADO INVALIDO: %s", warning.Message)
		} else {
			write("  Atencao: %s", warning.Message)
		}
		write("            %s", warning.Evidence)
	}
	if document.Scheduling.LateDispatches == 0 && document.Scheduling.DroppedByInflightLimit == 0 {
		write("  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.")
	}
	write("  Atraso tipico para disparar: %s; pior caso: %s (o tempo de resposta ja desconta isso)",
		milliseconds(document.Scheduling.Skew.P50), milliseconds(document.Scheduling.Skew.Max))
	hidden := document.Overall.Latency.P99 - document.Overall.ServiceLatency.P99
	if hidden >= 1 {
		write("  Uma ferramenta de laco fechado teria reportado %s a menos no 99%%.", milliseconds(hidden))
	}
	write("")

	write("Ambiente")
	write("  %s %s/%s, %d nucleos | braunrate %s | %s",
		document.Environment.Host, document.Environment.OS, document.Environment.Arch,
		document.Environment.Cores, document.Version, document.Run.Start.Format("2006-01-02 15:04:05"))
	for _, variety := range document.Variety {
		write("  %s", variety.Sentence)
	}
	if len(document.Run.Seeds) > 0 {
		write("  Semente das fontes sinteticas: %s (a mesma semente gera os mesmos valores de novo)", seeds(document.Run.Seeds))
	}
	if document.Run.AuthObtains > 0 {
		write("  Autenticacao obtida %d vez(es) e reaproveitada por todas as jornadas.", document.Run.AuthObtains)
		write("  Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.")
	}
	write("")
}

func seeds(values map[string]int64) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, values[name]))
	}
	return strings.Join(parts, ", ")
}

type errorLine struct {
	class string
	count int64
}

var classNames = map[string]string{
	"rede":         "falha de rede",
	"timeout":      "tempo esgotado",
	"status":       "status HTTP inesperado",
	"assercao":     "conteudo fora do esperado",
	"correlacao":   "nao consegui capturar um valor",
	"configuracao": "erro de configuracao do cenario",
	"autenticacao": "nao consegui autenticar",
	"saturacao":    "gerador saturado",
	"graphql":      "erro no corpo da resposta GraphQL (com status 200)",
}

func errorsByClass(document metrics.Document) []errorLine {
	total := map[string]int64{}
	for _, step := range document.Steps {
		for class, count := range step.ErrorsByClass {
			name := classNames[class]
			if name == "" {
				name = class
			}
			total[name] += count
		}
	}
	lines := make([]errorLine, 0, len(total))
	for class, count := range total {
		lines = append(lines, errorLine{class: class, count: count})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].count > lines[j].count })
	return lines
}

func milliseconds(value float64) string {
	switch {
	case value >= 1000:
		return fmt.Sprintf("%.2f s", value/1000)
	case value >= 10:
		return fmt.Sprintf("%.0f ms", value)
	case value >= 1:
		return fmt.Sprintf("%.1f ms", value)
	default:
		return fmt.Sprintf("%.3f ms", value)
	}
}

func percentage(value float64) string {
	if value == 0 {
		return "0%"
	}
	if value < 0.01 {
		return fmt.Sprintf("%.4f%%", value)
	}
	return fmt.Sprintf("%.2f%%", value)
}

func thousands(value int64) string {
	text := fmt.Sprintf("%d", value)
	if len(text) <= 3 {
		return text
	}
	var parts []string
	for len(text) > 3 {
		parts = append([]string{text[len(text)-3:]}, parts...)
		text = text[:len(text)-3]
	}
	parts = append([]string{text}, parts...)
	return strings.Join(parts, ".")
}

func trim(text string, size int) string {
	if len(text) <= size {
		return text
	}
	return strings.TrimSpace(text[:size-1]) + "…"
}
