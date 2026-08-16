package metrics

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
)

// Version 2 added the serialized histogram every percentile is a projection of.
// Version 1 is still read — the numbers it already carries are the numbers it
// always carried — and only summing needs the histogram.
const ResultFormatVersion = "2"

// ReadableResultFormats is what this binary can open. A format outside the list
// is refused by name instead of being read into fields that do not mean what
// they used to.
var ReadableResultFormats = []string{"1", "2"}

type Document struct {
	FormatVersion string        `json:"versao_do_formato"`
	Tool          string        `json:"ferramenta"`
	Version       string        `json:"versao"`
	Environment   Environment   `json:"ambiente"`
	Run           Run           `json:"execucao"`
	Scheduling    Scheduling    `json:"agendamento"`
	Journey       Journey       `json:"jornada"`
	Steps         []StepResult  `json:"passos"`
	Overall       OverallResult `json:"global"`
	Sanity        Sanity        `json:"sanidade"`
	SLO           Verdict       `json:"slo"`
	Variety       []Variety     `json:"variedade_observada"`
	Warnings      []Warning     `json:"avisos"`
	Series        []Bucket      `json:"series_temporais"`
}

type Environment struct {
	Host      string `json:"maquina"`
	OS        string `json:"sistema_operacional"`
	Arch      string `json:"arquitetura"`
	Cores     int    `json:"nucleos"`
	GoVersion string `json:"versao_do_go"`
	// Protocols compiled into this binary. Without it two binaries with the same
	// version number could produce different results and leave no trace of why
	// (ADR 0004).
	Protocols []string `json:"protocolos_compilados,omitempty"`
}

type Run struct {
	Spec        string           `json:"cenario"`
	Target      string           `json:"alvo"`
	Start       time.Time        `json:"inicio"`
	End         time.Time        `json:"fim"`
	DurationMs  int64            `json:"duracao_ms"`
	Model       string           `json:"modelo_de_chegada"`
	AppliedPlan []AppliedPhase   `json:"plano_aplicado"`
	Users       int              `json:"usuarios,omitzero"`
	ThinkTimeMs int64            `json:"intervalo_entre_iteracoes_ms,omitzero"`
	MaxInflight int64            `json:"maximo_de_requisicoes_simultaneas"`
	Seeds       map[string]int64 `json:"sementes_dos_dados"`
	// Variavel de ambiente que decidiu cada semente. Sem isso, uma semente que
	// veio do ambiente e um numero que ninguem sabe como reproduzir.
	SeedsFrom    map[string]string `json:"sementes_de_ambiente,omitempty"`
	Availability Availability      `json:"valores_disponiveis_por_variavel"`
	AuthObtains  int64             `json:"obtencoes_de_autenticacao"`
	Brokers      []string          `json:"mensageria,omitempty"`
	// How far behind each watched consumer group was left. The time to produce
	// says the broker accepted the message; this says whether the service kept
	// up, and they are different questions.
	ConsumerLag []protocol.ConsumerLag `json:"atraso_do_consumidor,omitempty"`
	// Declared so the report can show a step that never ran. Without it, a step
	// that depended on a capture that failed simply vanished from the table and
	// the reader never learned it existed.
	DeclaredSteps []string `json:"passos_declarados,omitempty"`
}

const ClosedModel = "fechado"

func (d Document) Closed() bool { return d.Run.Model == ClosedModel }

type AppliedPhase struct {
	Kind       string  `json:"tipo"`
	From       float64 `json:"de_por_segundo"`
	To         float64 `json:"ate_por_segundo"`
	DurationMs int64   `json:"duracao_ms"`
}

type Scheduling struct {
	Sent                   int64        `json:"enviadas"`
	Completed              int64        `json:"concluidas"`
	LateDispatches         int64        `json:"despachos_atrasados"`
	DroppedByInflightLimit int64        `json:"descartadas_por_limite_de_voo"`
	LostSamples            int64        `json:"amostras_perdidas"`
	PeakInflight           int64        `json:"pico_em_voo"`
	LateThresholdMs        float64      `json:"limiar_de_atraso_ms"`
	Skew                   Distribution `json:"desvio_de_agendamento"`
}

type Journey struct {
	Started        int64        `json:"iniciadas"`
	Completed      int64        `json:"completas"`
	Latency        Distribution `json:"latencia_corrigida,omitzero"`
	ServiceLatency Distribution `json:"latencia_de_servico,omitzero"`
	Sentence       string       `json:"frase"`
}

// Present exactly when the JSON carries it: the corrected field is omitted when
// it is the zero value, so the same test decides what is read back.
func (j Journey) Reported() Distribution {
	if j.Latency != (Distribution{}) {
		return j.Latency
	}
	return j.ServiceLatency
}

type StepResult struct {
	Name           string           `json:"nome"`
	LatencyKind    string           `json:"tipo_de_latencia"`
	Key            string           `json:"chave"`
	Protocol       string           `json:"protocolo"`
	Count          int64            `json:"contagem"`
	Successes      int64            `json:"sucessos"`
	Errors         int64            `json:"erros"`
	Bytes          int64            `json:"bytes"`
	ErrorsByClass  map[string]int64 `json:"erros_por_classe"`
	StatusByCode   map[string]int64 `json:"status_por_codigo"`
	Details        map[string]int64 `json:"detalhes_de_erro"`
	Latency        Distribution     `json:"latencia_corrigida,omitzero"`
	ServiceLatency Distribution     `json:"latencia_de_servico,omitzero"`
	// Proporcao que o cenario pediu para esta alternativa do mix. Zero quando o
	// cenario nao declara mix, e ai todo passo roda em toda iteracao.
	DeclaredShare float64 `json:"proporcao_declarada,omitempty"`
}

func (s StepResult) Reported() Distribution {
	if s.Latency != (Distribution{}) {
		return s.Latency
	}
	return s.ServiceLatency
}

type OverallResult struct {
	Count          int64        `json:"contagem"`
	Successes      int64        `json:"sucessos"`
	Errors         int64        `json:"erros"`
	ErrorRate      float64      `json:"taxa_de_erro"`
	EffectiveRate  float64      `json:"taxa_efetiva_por_segundo"`
	Latency        Distribution `json:"latencia_corrigida,omitzero"`
	ServiceLatency Distribution `json:"latencia_de_servico,omitzero"`
}

func (o OverallResult) Reported() Distribution {
	if o.Latency != (Distribution{}) {
		return o.Latency
	}
	return o.ServiceLatency
}

type Severity string

const (
	SeverityHigh   Severity = "alta"
	SeverityMedium Severity = "media"
	SeverityLow    Severity = "baixa"
)

type Warning struct {
	Kind     string   `json:"tipo"`
	Severity Severity `json:"gravidade"`
	Message  string   `json:"mensagem"`
	Evidence string   `json:"evidencia"`
}

// Valid has a single source of truth, the sanity check. The loop over warnings
// is the fallback for a result file written before the check existed: it
// reproduces exactly the rule that was in force then.
func (d Document) Valid() bool {
	if d.Sanity.Checked {
		return d.Sanity.Valid
	}
	for _, warning := range d.Warnings {
		if warning.Severity == SeverityHigh {
			return false
		}
	}
	return true
}

type DocumentInput struct {
	Version          string
	Protocols        []string
	Spec             string
	Target           string
	Model            string
	Start            time.Time
	End              time.Time
	Phases           []AppliedPhase
	MaxInflight      int64
	Seeds            map[string]int64
	SeedsFrom        map[string]string
	Availability     Availability
	AuthObtains      int64
	ScenarioWarnings []Warning
	Brokers          []string
	DeclaredSteps    []string
	DeclaredShares   map[string]float64
	ConsumerLag      []protocol.ConsumerLag
	PlannedDuration  time.Duration
	PlannedRequests  int64
	Users            int
	ThinkTime        time.Duration
}

func BuildDocument(collector *Collector, input DocumentInput) Document {
	hostname, _ := os.Hostname()
	document := Document{
		FormatVersion: ResultFormatVersion,
		Tool:          "braunrate",
		Version:       input.Version,
		Environment: Environment{
			Host:      hostname,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			Cores:     runtime.NumCPU(),
			GoVersion: runtime.Version(),
			Protocols: input.Protocols,
		},
		Run: Run{
			Spec:          input.Spec,
			Target:        input.Target,
			Start:         input.Start,
			End:           input.End,
			DurationMs:    input.End.Sub(input.Start).Milliseconds(),
			Model:         input.Model,
			AppliedPlan:   input.Phases,
			Users:         input.Users,
			ThinkTimeMs:   input.ThinkTime.Milliseconds(),
			MaxInflight:   input.MaxInflight,
			Seeds:         input.Seeds,
			SeedsFrom:     input.SeedsFrom,
			Availability:  input.Availability,
			AuthObtains:   input.AuthObtains,
			Brokers:       input.Brokers,
			DeclaredSteps: input.DeclaredSteps,
			ConsumerLag:   input.ConsumerLag,
		},
		Scheduling: Scheduling{
			Sent:                   collector.Sent,
			Completed:              collector.Completed,
			LateDispatches:         collector.LateDispatches,
			DroppedByInflightLimit: collector.DroppedByInflightLimit,
			LostSamples:            collector.LostSamples,
			PeakInflight:           collector.PeakInflight,
			LateThresholdMs:        float64(collector.LateThreshold.Microseconds()) / 1000,
			Skew:                   collector.SchedulingSkew(),
		},
		Series: collector.Buckets(),
	}

	aggregates := collector.Aggregates()
	overall := NewAggregate("global", "global", "")
	for _, key := range SortKeys(aggregates) {
		aggregate := aggregates[key]
		overall.Add(aggregate)
		step := convertStep(aggregate)
		step.DeclaredShare = input.DeclaredShares[step.Key]
		document.Steps = append(document.Steps, step)
	}

	duration := input.End.Sub(input.Start).Seconds()
	document.Overall = OverallResult{
		Count:          overall.Count,
		Successes:      overall.Successes,
		Errors:         overall.Errors(),
		Latency:        overall.Distribution(),
		ServiceLatency: overall.ServiceDistribution(),
	}
	if overall.Count > 0 {
		document.Overall.ErrorRate = float64(overall.Errors()) / float64(overall.Count)
	}
	if duration > 0 {
		document.Overall.EffectiveRate = float64(overall.Count) / duration
	}

	document.Journey = Journey{
		Started:   collector.JourneysStarted,
		Completed: collector.JourneysCompleted,
		Latency:   collector.Journeys(),
	}
	if document.Closed() {
		dropCorrectedLatency(&document)
	}
	document.Journey.Sentence = phraseJourney(document.Journey, document.Closed())

	document.Variety = collector.Varieties(input.Availability)
	document.Warnings = append(evaluateWarnings(collector, document), input.ScenarioWarnings...)
	document.Sanity = CheckSanity(document, input)
	return document
}

// The closed loop schedules nothing, so there is no instant to count from. The
// field is absent rather than zero: a zero there reads as "no delay hidden",
// which is the one thing a closed loop cannot claim.
func dropCorrectedLatency(document *Document) {
	document.Overall.Latency = Distribution{}
	document.Journey.ServiceLatency = document.Journey.Latency
	document.Journey.Latency = Distribution{}
	for index := range document.Steps {
		document.Steps[index].Latency = Distribution{}
	}
}

func ClosedLoopWarning(document Document) (string, bool) {
	if !document.Closed() {
		return "", false
	}
	return fmt.Sprintf("Este teste usou %d usuários em laço fechado. Se o alvo travar, os usuários param de pedir "+
		"e o atraso não aparece nos números. O tempo de resposta abaixo pode estar melhor do que o usuário real sente.",
		document.Run.Users), true
}

func convertStep(a *Aggregate) StepResult {
	errorsByClass := map[string]int64{}
	for class, count := range a.ErrorsByClass {
		errorsByClass[string(class)] = count
	}
	statusPorCodigo := map[string]int64{}
	for status, count := range a.StatusByCode {
		statusPorCodigo[fmt.Sprintf("%d", status)] = count
	}
	return StepResult{
		Name:           a.Step,
		LatencyKind:    string(a.LatencyKind),
		Key:            a.Key,
		Protocol:       a.Protocol,
		Count:          a.Count,
		Successes:      a.Successes,
		Errors:         a.Errors(),
		Bytes:          a.Bytes,
		ErrorsByClass:  errorsByClass,
		StatusByCode:   statusPorCodigo,
		Details:        a.Details,
		Latency:        a.Distribution(),
		ServiceLatency: a.ServiceDistribution(),
	}
}

// Past this, the histogram is measuring the generator, not the target.
const lateDispatchLimit = 0.01

func lateProportion(scheduling Scheduling) float64 {
	if scheduling.Sent == 0 {
		return 0
	}
	return float64(scheduling.LateDispatches) / float64(scheduling.Sent)
}

func dispatchWasLate(scheduling Scheduling) bool {
	return lateProportion(scheduling) >= lateDispatchLimit || scheduling.DroppedByInflightLimit > 0
}

func evaluateWarnings(collector *Collector, document Document) []Warning {
	var warnings []Warning
	scheduling := document.Scheduling

	if scheduling.DroppedByInflightLimit > 0 {
		warnings = append(warnings, Warning{
			Kind:     "gerador_saturado",
			Severity: SeverityHigh,
			Message:  "o gerador atingiu o limite de requisições em voo e deixou de enviar requisições agendadas; o resultado não vale",
			Evidence: fmt.Sprintf("%d requisições descartadas, pico de %d em voo", scheduling.DroppedByInflightLimit, scheduling.PeakInflight),
		})
	}

	if scheduling.Sent > 0 {
		proportion := lateProportion(scheduling)
		if proportion >= lateDispatchLimit {
			warnings = append(warnings, Warning{
				Kind:     "gerador_saturado",
				Severity: SeverityHigh,
				Message:  "o gerador não sustentou a taxa alvo: despachos sairam depois do instante agendado; o resultado não vale",
				Evidence: fmt.Sprintf("%.2f%% dos despachos atrasaram mais de %.1f ms (desvio p99 de %.1f ms)", proportion*100, scheduling.LateThresholdMs, scheduling.Skew.P99),
			})
		} else if scheduling.LateDispatches > 0 {
			warnings = append(warnings, Warning{
				Kind:     "gerador_com_atraso_pontual",
				Severity: SeverityLow,
				Message:  "houve atraso pontual de despacho, abaixo de 1% das requisições",
				Evidence: fmt.Sprintf("%d despachos atrasados de %d (desvio p99 de %.1f ms)", scheduling.LateDispatches, scheduling.Sent, scheduling.Skew.P99),
			})
		}
	}

	if scheduling.LostSamples > 0 {
		warnings = append(warnings, Warning{
			Kind:     "amostras_perdidas",
			Severity: SeverityHigh,
			Message:  "o coletor de métrica não acompanhou o volume e perdeu amostras; a distribuição está incompleta",
			Evidence: fmt.Sprintf("%d amostras perdidas", scheduling.LostSamples),
		})
	}

	if warning, had := detectTargetDegradation(document); had {
		warnings = append(warnings, warning)
	}

	warnings = append(warnings, VarietyWarnings(document.Variety)...)

	return warnings
}

// The warning claims dispatch stayed punctual, so punctuality is checked before
// it is stated: without this the report printed both "4% of dispatches were
// late" and "dispatch stayed punctual". With the generator slipping, the two
// causes cannot be told apart from outside.
func detectTargetDegradation(document Document) (Warning, bool) {
	series := document.Series
	if len(series) < 4 {
		return Warning{}, false
	}
	if dispatchWasLate(document.Scheduling) {
		return Warning{}, false
	}
	first := series[0].LatencyP99Ms
	if first <= 0 {
		first = series[1].LatencyP99Ms
	}
	worst := 0.0
	for _, bucket := range series {
		if bucket.LatencyP99Ms > worst {
			worst = bucket.LatencyP99Ms
		}
	}
	if first > 0 && worst >= 3*first {
		message := "o tempo de resposta do alvo cresceu ao longo da execução enquanto o despacho continuou pontual; a degradação é do alvo, não do gerador"
		if document.Closed() {
			message = "o tempo de resposta do alvo cresceu ao longo da execução; no laço fechado isso também derruba a carga, então a queda de taxa é parte do mesmo evento, não um segundo achado"
		}
		return Warning{
			Kind:     "alvo_degradado",
			Severity: SeverityMedium,
			Message:  message,
			Evidence: fmt.Sprintf("p99 por segundo passou de %.1f ms para %.1f ms", first, worst),
		}, true
	}
	return Warning{}, false
}

func ReadableClass(class protocol.ErrorClass) string {
	switch class {
	case protocol.ErrNetwork:
		return "falha de rede"
	case protocol.ErrTimeout:
		return "timeout"
	case protocol.ErrStatus:
		return "status HTTP inesperado"
	case protocol.ErrAssertion:
		return "assercao funcional"
	case protocol.ErrCorrelation:
		return "correlação perdida"
	case protocol.ErrSaturation:
		return "saturacao do gerador"
	case protocol.ErrGraphQL:
		return "erro no corpo da resposta GraphQL"
	default:
		return string(class)
	}
}

func phraseJourney(journey Journey, closed bool) string {
	if journey.Started == 0 {
		return "Nenhuma jornada foi executada."
	}
	counted := "contados do instante em que deveriam ter comecado"
	if closed {
		counted = "contados de quando o usuário virtual começou a jornada, que é só depois de ter terminado a anterior"
	}
	latency := journey.Reported()
	if journey.Completed < journey.Started {
		return fmt.Sprintf("%d de %d jornadas chegaram ao fim; metade delas levou até %.0f ms e 95%% até %.0f ms, %s.",
			journey.Completed, journey.Started, latency.P50, latency.P95, counted)
	}
	return fmt.Sprintf("Todas as %d jornadas chegaram ao fim; metade levou até %.0f ms e 95%% até %.0f ms, %s.",
		journey.Started, latency.P50, latency.P95, counted)
}

// StepsThatNeverRan is the difference between what the scenario declared and
// what the measurement saw. It is empty for a result written before the field
// existed, which is why the report treats it as extra information and not as
// the source of the step list.
func StepsThatNeverRan(document Document) []string {
	if len(document.Run.DeclaredSteps) == 0 {
		return nil
	}
	measured := map[string]bool{}
	for _, step := range document.Steps {
		measured[step.Name] = true
	}
	var missing []string
	for _, declared := range document.Run.DeclaredSteps {
		if !measured[declared] {
			missing = append(missing, declared)
		}
	}
	return missing
}
