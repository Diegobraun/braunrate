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
const ResultFormatVersion = "4"

// ReadableResultFormats is what this binary can open. A format outside the list
// is refused by name instead of being read into fields that do not mean what
// they used to.
var ReadableResultFormats = []string{"3", "4"}

type Document struct {
	FormatVersion string        `json:"formatVersion"`
	Tool          string        `json:"tool"`
	Version       string        `json:"version"`
	Environment   Environment   `json:"environment"`
	Run           Run           `json:"run"`
	Scheduling    Scheduling    `json:"scheduling"`
	Journey       Journey       `json:"journey"`
	Steps         []StepResult  `json:"steps"`
	Overall       OverallResult `json:"global"`
	Sanity        Sanity        `json:"sanity"`
	SLO           Verdict       `json:"slo"`
	Variety       []Variety     `json:"observedVariety"`
	Warnings      []Warning     `json:"warnings"`
	Series        []Bucket      `json:"timeSeries"`
}

type Environment struct {
	Host      string `json:"host"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Cores     int    `json:"cores"`
	GoVersion string `json:"goVersion"`
	// Protocols compiled into this binary. Without it two binaries with the same
	// version number could produce different results and leave no trace of why
	// (ADR 0004).
	Protocols []string `json:"compiledProtocols,omitempty"`
}

type Run struct {
	Spec        string           `json:"scenario"`
	Target      string           `json:"target"`
	Start       time.Time        `json:"start"`
	End         time.Time        `json:"end"`
	DurationMs  int64            `json:"durationMs"`
	Model       string           `json:"arrivalModel"`
	AppliedPlan []AppliedPhase   `json:"appliedPlan"`
	Users       int              `json:"users,omitzero"`
	ThinkTimeMs int64            `json:"thinkTimeMs,omitzero"`
	MaxInflight int64            `json:"maxInflight"`
	Seeds       map[string]int64 `json:"dataSeeds"`
	// Variavel de ambiente que decidiu cada semente. Sem isso, uma semente que
	// veio do ambiente e um numero que ninguem sabe como reproduzir.
	SeedsFrom    map[string]string `json:"seedsFromEnvironment,omitempty"`
	Availability Availability      `json:"availableValuesByVariable"`
	AuthObtains  int64             `json:"authObtains"`
	Brokers      []string          `json:"messaging,omitempty"`
	// How far behind each watched consumer group was left. The time to produce
	// says the broker accepted the message; this says whether the service kept
	// up, and they are different questions.
	ConsumerLag []protocol.ConsumerLag `json:"consumerLag,omitempty"`
	// Declared so the report can show a step that never ran. Without it, a step
	// that depended on a capture that failed simply vanished from the table and
	// the reader never learned it existed.
	DeclaredSteps []string `json:"declaredSteps,omitempty"`
}

const ClosedModel = "closed"

func (d Document) Closed() bool { return d.Run.Model == ClosedModel }

type AppliedPhase struct {
	Kind       string  `json:"kind"`
	From       float64 `json:"fromPerSecond"`
	To         float64 `json:"toPerSecond"`
	DurationMs int64   `json:"durationMs"`
}

type Scheduling struct {
	Sent                   int64        `json:"sent"`
	Completed              int64        `json:"completed"`
	LateDispatches         int64        `json:"lateDispatches"`
	DroppedByInflightLimit int64        `json:"droppedByInflightLimit"`
	LostSamples            int64        `json:"lostSamples"`
	PeakInflight           int64        `json:"peakInflight"`
	LateThresholdMs        float64      `json:"lateThresholdMs"`
	Skew                   Distribution `json:"schedulingSkew"`
}

type Journey struct {
	Started        int64        `json:"started"`
	Completed      int64        `json:"completed"`
	Latency        Distribution `json:"correctedLatency,omitzero"`
	ServiceLatency Distribution `json:"serviceLatency,omitzero"`
	Sentence       string       `json:"sentence"`
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
	Name           string           `json:"name"`
	LatencyKind    string           `json:"latencyKind"`
	Key            string           `json:"key"`
	Protocol       string           `json:"protocol"`
	Count          int64            `json:"count"`
	Successes      int64            `json:"successes"`
	Errors         int64            `json:"errors"`
	Bytes          int64            `json:"bytes"`
	Messages       int64            `json:"messages,omitempty"`
	ErrorsByClass  map[string]int64 `json:"errorsByClass"`
	StatusByCode   map[string]int64 `json:"statusByCode"`
	Details        map[string]int64 `json:"errorDetails"`
	Latency        Distribution     `json:"correctedLatency,omitzero"`
	ServiceLatency Distribution     `json:"serviceLatency,omitzero"`
	// Proporcao que o cenario pediu para esta alternativa do mix. Zero quando o
	// cenario nao declara mix, e ai todo passo roda em toda iteracao.
	DeclaredShare float64 `json:"declaredShare,omitempty"`
}

func (s StepResult) Reported() Distribution {
	if s.Latency != (Distribution{}) {
		return s.Latency
	}
	return s.ServiceLatency
}

type OverallResult struct {
	Count          int64        `json:"count"`
	Successes      int64        `json:"successes"`
	Errors         int64        `json:"errors"`
	ErrorRate      float64      `json:"errorRate"`
	EffectiveRate  float64      `json:"effectiveRatePerSecond"`
	Latency        Distribution `json:"correctedLatency,omitzero"`
	ServiceLatency Distribution `json:"serviceLatency,omitzero"`
}

func (o OverallResult) Reported() Distribution {
	if o.Latency != (Distribution{}) {
		return o.Latency
	}
	return o.ServiceLatency
}

type Severity string

// O valor vai para o JSON, que e lido por automacao. Ficou em portugues depois
// que o resto da saida virou ingles, e quem escrevesse severity == "high" nao
// receberia erro nenhum: receberia zero achados, em silencio.
const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

// Resultado gravado antes desta versao traz o valor em portugues. Recusar o
// arquivo tiraria justamente a comparacao com a execucao de ontem, que e o
// motivo de guardar resultado; traduzir na leitura mantem os dois lados.
var severityBefore = map[Severity]Severity{"alta": SeverityHigh, "media": SeverityMedium, "baixa": SeverityLow}

// TranslateOlderValues is called at the single read point, so a value that
// changed spelling between formats is normalized once instead of at every place
// that switches on it.
func (document *Document) TranslateOlderValues() {
	for index, warning := range document.Warnings {
		if current, older := severityBefore[warning.Severity]; older {
			document.Warnings[index].Severity = current
		}
	}
}

type Warning struct {
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Evidence string   `json:"evidence"`
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
	return fmt.Sprintf("This test used %d users in a closed loop. If the target freezes, the users stop asking "+
		"and the delay never shows up in the numbers. The response time below may look better than what a real user feels.",
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
		Messages:       a.Messages,
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
			Kind:     "generatorSaturated",
			Severity: SeverityHigh,
			Message:  "the generator hit the in-flight limit and stopped sending scheduled requests; the result does not hold",
			Evidence: fmt.Sprintf("%d requests dropped, peak of %d in flight", scheduling.DroppedByInflightLimit, scheduling.PeakInflight),
		})
	}

	if scheduling.Sent > 0 {
		proportion := lateProportion(scheduling)
		if proportion >= lateDispatchLimit {
			warnings = append(warnings, Warning{
				Kind:     "generatorSaturated",
				Severity: SeverityHigh,
				Message:  "the generator did not sustain the target rate: dispatches went out after their scheduled instant; the result does not hold",
				Evidence: fmt.Sprintf("%.2f%% of the dispatches went out more than %.1f ms late (p99 skew of %.1f ms)", proportion*100, scheduling.LateThresholdMs, scheduling.Skew.P99),
			})
		} else if scheduling.LateDispatches > 0 {
			warnings = append(warnings, Warning{
				Kind:     "generatorOccasionallyLate",
				Severity: SeverityLow,
				Message:  "there was occasional dispatch delay, below 1% of the requests",
				Evidence: fmt.Sprintf("%d late dispatches out of %d (p99 skew of %.1f ms)", scheduling.LateDispatches, scheduling.Sent, scheduling.Skew.P99),
			})
		}
	}

	if scheduling.LostSamples > 0 {
		warnings = append(warnings, Warning{
			Kind:     "lostSamples",
			Severity: SeverityHigh,
			Message:  "the metric collector did not keep up with the volume and lost samples; the distribution is incomplete",
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
		message := "the target response time grew over the run while dispatch stayed on time; the degradation belongs to the target, not to the generator"
		if document.Closed() {
			message = "the target response time grew over the run; in a closed loop that also drags the load down, so the drop in rate is part of the same event, not a second finding"
		}
		return Warning{
			Kind:     "targetDegraded",
			Severity: SeverityMedium,
			Message:  message,
			Evidence: fmt.Sprintf("p99 per second went from %.1f ms to %.1f ms", first, worst),
		}, true
	}
	return Warning{}, false
}

func ReadableClass(class protocol.ErrorClass) string {
	switch class {
	case protocol.ErrNetwork:
		return "network failure"
	case protocol.ErrTimeout:
		return "timeout"
	case protocol.ErrStatus:
		return "unexpected HTTP status"
	case protocol.ErrAssertion:
		return "functional assertion"
	case protocol.ErrCorrelation:
		return "lost correlation"
	case protocol.ErrSaturation:
		return "generator saturation"
	case protocol.ErrGraphQL:
		return "error in the GraphQL response body"
	default:
		return string(class)
	}
}

func phraseJourney(journey Journey, closed bool) string {
	if journey.Started == 0 {
		return "No journey ran."
	}
	counted := "counted from the instant they should have started"
	if closed {
		counted = "counted from when the virtual user started the journey, which is only after finishing the previous one"
	}
	latency := journey.Reported()
	if journey.Completed < journey.Started {
		return fmt.Sprintf("%d of %d journeys reached the end; half of them took up to %.0f ms and 95%% up to %.0f ms, %s.",
			journey.Completed, journey.Started, latency.P50, latency.P95, counted)
	}
	return fmt.Sprintf("All %d journeys reached the end; half took up to %.0f ms and 95%% up to %.0f ms, %s.",
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
