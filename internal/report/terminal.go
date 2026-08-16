package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/texto"
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

// No target rate to show: in the closed loop the rate is a result, so what goes
// on screen is what the users are getting, never what they were asked for.
func ClosedProgressLine(snapshot metrics.Snapshot, users int, remaining time.Duration) string {
	return fmt.Sprintf("%d usuarios em laco | concluidas %d | erros %d | metade em %.1f ms | 99%% em %.1f ms | faltam %s",
		users, snapshot.Completed, snapshot.Errors,
		snapshot.LatencyP50Ms, snapshot.LatencyP99Ms, remaining.Round(time.Second))
}

// Summary has two layers: the plain-language sentence says what happened, and
// the number sits right below it for whoever needs it.
func Summary(out io.Writer, document metrics.Document, verdict slo.Verdict) error {
	lines := &lineWriter{out: out}
	write := lines.writef

	write("")
	write("%s — contra %s", document.Run.Spec, document.Run.Target)
	write("")

	if warning, closed := metrics.ClosedLoopWarning(document); closed {
		write("ATENCAO: %s", warning)
		write("")
	}

	if !document.Valid() && document.Sanity.Checked {
		write("%s", document.Sanity.Sentence)
		write("")
		for _, finding := range document.Sanity.Findings {
			write("  - %s", finding.Message)
			write("    %s", finding.Evidence)
		}
		write("")
	}

	if len(verdict.Evaluations) > 0 {
		write("%s", verdict.Sentence)
		write("")
	}

	overall := document.Overall
	overallLatency := overall.Reported()
	journey := document.Journey.Reported()
	duration := (time.Duration(document.Run.DurationMs) * time.Millisecond).Round(100 * time.Millisecond)
	write("O que aconteceu")
	write("  %s requisicoes em %s, %.0f por segundo, %s de erro",
		thousands(overall.Count), duration, overall.EffectiveRate, percentage(overall.ErrorRate*100))
	write("  Metade das respostas em ate %s; 95%% em ate %s; 99%% em ate %s; a pior levou %s",
		milliseconds(overallLatency.P50), milliseconds(overallLatency.P95),
		milliseconds(overallLatency.P99), milliseconds(overallLatency.Max))
	write("")

	if document.Journey.Started > 0 {
		write("A jornada inteira")
		write("  %s", document.Journey.Sentence)
		write("  metade %s | 95%% %s | 99%% %s | pior %s",
			milliseconds(journey.P50), milliseconds(journey.P95),
			milliseconds(journey.P99), milliseconds(journey.Max))
		// Com mix, cada iteracao e uma alternativa: o percentil de jornada passa a
		// juntar populacoes de custo diferente, e quem le procura cauda onde ha
		// mistura. A ferramenta sabe disso e diz, em vez de deixar descobrir.
		if alternatives := mixedAlternatives(document); alternatives > 1 {
			write("  Cada jornada aqui e uma das %d alternativas do mix, entao estes percentis juntam", alternatives)
			write("  populacoes de custo diferente. Para ler cada uma, use a tabela por passo.")
		}
		write("")
	}

	writeStepTable(lines, document)

	if len(verdict.Evaluations) > 0 || len(verdict.Undeclared) > 0 {
		write("SLO")
		for _, evaluation := range verdict.Evaluations {
			mark := "ok  "
			switch {
			case evaluation.Untrustworthy:
				mark = "?    "
			case !evaluation.Passed:
				mark = "FALHA"
			}
			write("  %-5s %s", mark, evaluation.Sentence)
		}
		for _, missing := range verdict.Undeclared {
			write("  --    %s", missing)
		}
		write("")
	}

	errors := errorLines(document)
	if len(errors) > 0 {
		write("Erros")
		write("  %-26s %-34s %10s   %s", "passo", "o que aconteceu", "quantidade", "exemplo")
		for _, line := range errors {
			write("  %-26s %-34s %10s   %s", trim(line.step, 26), trim(line.class, 34), thousands(line.count), trim(line.example, exampleWidth))
		}
		// The column that fits the table was cutting the messages exactly where
		// the cause is: what survived was the URL, which the reader already
		// knew, and what was lost was the part that says what to do.
		for _, line := range errors {
			if len(line.example) > exampleWidth {
				write("    %s", line.example)
			}
		}
		write("")
	}

	writeConsumerLag(lines, document)

	write("Confiabilidade da medicao")
	for _, warning := range document.Warnings {
		if warning.Severity == metrics.SeverityHigh {
			// Already reported at the top, as a sanity finding.
			if document.Sanity.Checked {
				continue
			}
			write("  RESULTADO INVALIDO: %s", warning.Message)
		} else {
			write("  Atencao: %s", warning.Message)
		}
		write("            %s", warning.Evidence)
	}
	if document.Closed() {
		write("  Nao ha agendamento para comparar: a taxa efetiva de %.0f/s foi consequencia do tempo", document.Overall.EffectiveRate)
		write("  de resposta do alvo, nao uma carga declarada. Se o alvo ficar mais lento, a carga cai junto.")
	} else {
		if document.Scheduling.LateDispatches == 0 && document.Scheduling.DroppedByInflightLimit == 0 {
			write("  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.")
		}
		write("  Atraso tipico para disparar: %s; pior caso: %s (o tempo de resposta ja desconta isso)",
			milliseconds(document.Scheduling.Skew.P50), milliseconds(document.Scheduling.Skew.Max))
		hidden := document.Overall.Latency.P99 - document.Overall.ServiceLatency.P99
		if hidden >= 1 {
			write("  Uma ferramenta de laco fechado teria reportado %s a menos no 99%%.", milliseconds(hidden))
		}
	}
	write("")

	write("Ambiente")
	write("  %s %s/%s, %d nucleos | braunrate %s | %s",
		document.Environment.Host, document.Environment.OS, document.Environment.Arch,
		document.Environment.Cores, document.Version, document.Run.Start.Format("2006-01-02 15:04:05"))
	if len(document.Environment.Protocols) > 0 {
		write("  Protocolos compilados: %s", strings.Join(document.Environment.Protocols, ", "))
	}
	for _, broker := range document.Run.Brokers {
		write("  Mensageria: %s", broker)
	}
	for _, variety := range document.Variety {
		if !variety.Notable() {
			continue
		}
		write("  %s", variety.Sentence)
	}
	if len(document.Run.Seeds) > 0 {
		write("  Sementes dos dados: %s (a mesma semente gera os mesmos valores de novo)",
			seeds(document.Run.Seeds, document.Run.SeedsFrom))
		// Semente que veio do ambiente e um numero que ninguem sabe como
		// reproduzir depois — a menos que o relatorio diga a linha de comando que
		// traz de volta este mesmo caso.
		if repeat := repeatWithSeeds(document.Run); repeat != "" {
			write("  Para repetir exatamente estes dados, rode de novo com %s", repeat)
		}
	}
	if document.Run.AuthObtains > 0 {
		write("  Autenticacao obtida %s e reaproveitada por todas as jornadas.",
			texto.Times(document.Run.AuthObtains))
		write("  Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.")
	}
	write("")
	return lines.err
}

func seeds(values map[string]int64, origins map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		if origin, fromEnvironment := origins[name]; fromEnvironment {
			parts = append(parts, fmt.Sprintf("%s=%d (de $%s)", name, values[name], origin))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", name, values[name]))
	}
	return strings.Join(parts, ", ")
}

func repeatWithSeeds(run metrics.Run) string {
	names := make([]string, 0, len(run.SeedsFrom))
	for name := range run.SeedsFrom {
		names = append(names, name)
	}
	sort.Strings(names)
	assignments := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		variable := run.SeedsFrom[name]
		if seen[variable] {
			continue
		}
		seen[variable] = true
		assignments = append(assignments, fmt.Sprintf("%s=%d", variable, run.Seeds[name]))
	}
	return strings.Join(assignments, " ")
}

type errorLine struct {
	step    string
	class   string
	count   int64
	example string
}

var classNames = map[string]string{
	"rede":         "falha de rede",
	"timeout":      "tempo esgotado",
	"status":       "status HTTP inesperado",
	"assercao":     "conteudo fora do esperado",
	"correlacao":   "nao consegui capturar um valor",
	"configuracao": "erro de configuracao do cenario",
	"autenticacao": "nao consegui autenticar",
	"autorizacao":  "credencial aceita, sem permissao nesse recurso",
	"mensageria":   "o broker recusou a mensagem",
	"saturacao":    "o gerador nao sustentou a taxa",
	"graphql":      "erro no corpo da resposta GraphQL (com status 200)",
}

// A class with no entry here used to print an empty line, which says less than
// the raw name of the class.
func className(class string) string {
	if name := classNames[class]; name != "" {
		return name
	}
	return class
}

// One line per class was the whole error section: "status HTTP inesperado 60"
// does not say which status, nor in which step, and both are in the JSON.
func errorLines(document metrics.Document) []errorLine {
	var lines []errorLine
	for _, step := range document.Steps {
		for class, count := range step.ErrorsByClass {
			lines = append(lines, errorLine{
				step:    step.Name,
				class:   className(class),
				count:   count,
				example: mostFrequent(step.Details, class),
			})
		}
	}
	sort.Slice(lines, func(first, second int) bool {
		if lines[first].count != lines[second].count {
			return lines[first].count > lines[second].count
		}
		return lines[first].step < lines[second].step
	})
	return lines
}

// The detail map holds every distinct message; the most frequent one is the one
// worth a line, and the count already says how many there were.
func mostFrequent(details map[string]int64, class string) string {
	best, most := "", int64(0)
	for detail, count := range details {
		if count > most || (count == most && detail < best) {
			best, most = detail, count
		}
	}
	if best == "" {
		return class
	}
	return strings.Join(strings.Fields(best), " ")
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

// Wide enough for a short cause, narrow enough to keep the table readable.
const exampleWidth = 44

func trim(text string, size int) string {
	if len(text) <= size {
		return text
	}
	return strings.TrimSpace(text[:size-1]) + "…"
}

// The header over an empty table says "there is nothing here" in the least
// useful way there is.
func writeStepTable(output *lineWriter, document metrics.Document) {
	write := output.writef
	never := metrics.StepsThatNeverRan(document)

	write("Por passo")
	if len(document.Steps) == 0 && len(never) == 0 {
		write("  Nenhum passo registrou amostra: a execucao nao chegou a medir nada.")
		write("  Rode 'braunrate debug' para ver onde a iteracao para.")
		write("")
		return
	}

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
			milliseconds(step.Reported().P50), milliseconds(step.Reported().P95),
			milliseconds(step.Reported().P99), milliseconds(step.Reported().P999),
			milliseconds(step.Reported().Max), step.Errors)
	}
	// A step that never ran used to vanish from here, and whoever read the
	// report never found out it existed.
	for _, name := range never {
		write("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7s",
			trim(name, 26), "", "0", "\u2014", "\u2014", "\u2014", "\u2014", "\u2014", "\u2014")
	}
	if len(never) > 0 {
		write("")
		write("  Passo com traco nunca chegou a executar: a iteracao parou antes dele. O motivo")
		write("  esta em \"Erros\", no passo que falhou primeiro.")
	}
	write("")

	if document.Closed() {
		write("  (2) tempo de resposta puro. No laco fechado nao existe instante agendado: o")
		write("      usuario virtual so pede de novo depois da resposta anterior, entao nenhum")
		write("      atraso de fila aparece nestes numeros.")
	} else {
		write("  (1) tempo contado do instante em que a requisicao deveria ter partido \u2014 inclui")
		write("      qualquer atraso e por isso nao esconde travada do alvo.")
		if hasServiceStep {
			write("  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como")
			write("      esse passo depende do valor capturado antes dele, nao existe instante")
			write("      agendado proprio. Para a leitura honesta da jornada, use \"A jornada inteira\".")
		}
	}
	write("")
	writeMix(output, document)
}

// Peso de 60% que virou 45% na execucao e informacao, nao detalhe: a proporcao
// e o que faz a carga ser um mix e nao tres cenarios, e uma proporcao que nao
// se cumpriu muda o que o numero significa. So aparece quando o cenario declara
// mix — sem mix, todo passo roda em toda iteracao e a proporcao seria 100% em
// todas as linhas.
func mixedAlternatives(document metrics.Document) int {
	declared := 0
	for _, step := range document.Steps {
		if step.DeclaredShare > 0 {
			declared++
		}
	}
	return declared
}

func writeMix(output *lineWriter, document metrics.Document) {
	total := int64(0)
	declared := false
	for _, step := range document.Steps {
		total += step.Count
		if step.DeclaredShare > 0 {
			declared = true
		}
	}
	if !declared || total == 0 {
		return
	}
	write := output.writef
	write("Mix declarado e observado")
	for _, step := range document.Steps {
		if step.DeclaredShare <= 0 {
			continue
		}
		observed := float64(step.Count) / float64(total)
		write("  %-26s %6.1f%% declarado   %6.1f%% observado (%s de %s)",
			trim(step.Name, 26), step.DeclaredShare*100, observed*100,
			thousands(step.Count), thousands(total))
	}
	write("")
}

// Producing fast says the broker accepted the message. Whether the service kept
// up is a different number, and it is the one that decides if the chain held.
func writeConsumerLag(output *lineWriter, document metrics.Document) {
	if len(document.Run.ConsumerLag) == 0 {
		return
	}
	write := output.writef
	write("Atraso do consumidor")
	for _, lag := range document.Run.ConsumerLag {
		headline, note := lagSentences(lag)
		write("  grupo %s em %s: %s", lag.Group, lag.Topic, headline)
		if note != "" {
			write("  %s", note)
		}
	}
	write("")
}
