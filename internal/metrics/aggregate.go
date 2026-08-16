package metrics

import (
	"sort"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const (
	minLatencyUs    = int64(1)
	maxLatencyUs    = int64(600_000_000)
	precisionDigits = 3
)

type LatencyKind string

const (
	// The step that opens the iteration counts from the scheduled instant, so
	// it is protected against coordinated omission. The following ones count
	// from when the previous step ended: they have no scheduled instant.
	CorrectedLatency LatencyKind = "corrigida"
	ServiceLatency   LatencyKind = "servico"
)

type Sample struct {
	LatencyKind LatencyKind
	Step        string
	Key         string
	Protocol    string
	ScheduledAt time.Time
	SentAt      time.Time
	FinishedAt  time.Time
	Class       protocol.ErrorClass
	Detail      string
	Status      int
	Bytes       int64
}

func (a Sample) CorrectedLatency() time.Duration {
	return a.FinishedAt.Sub(a.ScheduledAt)
}

func (a Sample) ServiceLatency() time.Duration {
	return a.FinishedAt.Sub(a.SentAt)
}

type Aggregate struct {
	Step             string
	Key              string
	Protocol         string
	LatencyKind      LatencyKind
	correctedLatency *hdrhistogram.Histogram
	serviceLatency   *hdrhistogram.Histogram
	Count            int64
	Successes        int64
	ErrorsByClass    map[protocol.ErrorClass]int64
	StatusByCode     map[int]int64
	Bytes            int64
	Details          map[string]int64
}

func NewAggregate(step, key, protocolName string) *Aggregate {
	return &Aggregate{
		Step:             step,
		Key:              key,
		Protocol:         protocolName,
		correctedLatency: hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		serviceLatency:   hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		ErrorsByClass:    map[protocol.ErrorClass]int64{},
		StatusByCode:     map[int]int64{},
		Details:          map[string]int64{},
	}
}

func (a *Aggregate) Record(sample Sample) {
	if a.LatencyKind == "" {
		a.LatencyKind = sample.LatencyKind
	}
	a.Count++
	a.Bytes += sample.Bytes
	if sample.Status > 0 {
		a.StatusByCode[sample.Status]++
	}
	if sample.Class == protocol.Success {
		a.Successes++
	} else {
		a.ErrorsByClass[sample.Class]++
		if sample.Detail != "" && len(a.Details) < 32 {
			a.Details[sample.Detail]++
		}
	}
	if !sample.FinishedAt.IsZero() {
		save(a.correctedLatency, sample.CorrectedLatency())
		if !sample.SentAt.IsZero() {
			save(a.serviceLatency, sample.ServiceLatency())
		}
	}
}

func save(histogram *hdrhistogram.Histogram, value time.Duration) {
	microseconds := value.Microseconds()
	if microseconds < minLatencyUs {
		microseconds = minLatencyUs
	}
	if microseconds > maxLatencyUs {
		microseconds = maxLatencyUs
	}
	_ = histogram.RecordValue(microseconds)
}

// Add merges another aggregate into this one. It exists so distributed
// execution needs no rewrite: HDR histograms and counters merge, means and
// precomputed percentiles do not.
func (a *Aggregate) Add(other *Aggregate) {
	a.correctedLatency.Merge(other.correctedLatency)
	a.serviceLatency.Merge(other.serviceLatency)
	a.Count += other.Count
	a.Successes += other.Successes
	a.Bytes += other.Bytes
	for class, count := range other.ErrorsByClass {
		a.ErrorsByClass[class] += count
	}
	for status, count := range other.StatusByCode {
		a.StatusByCode[status] += count
	}
	for detail, count := range other.Details {
		a.Details[detail] += count
	}
}

func (a *Aggregate) Errors() int64 {
	return a.Count - a.Successes
}

func (a *Aggregate) Distribution() Distribution {
	return distributionOf(a.correctedLatency)
}

func (a *Aggregate) ServiceDistribution() Distribution {
	return distributionOf(a.serviceLatency)
}

type Distribution struct {
	Samples int64   `json:"amostras"`
	P50     float64 `json:"p50_ms"`
	P75     float64 `json:"p75_ms"`
	P90     float64 `json:"p90_ms"`
	P95     float64 `json:"p95_ms"`
	P99     float64 `json:"p99_ms"`
	P999    float64 `json:"p99_9_ms"`
	Max     float64 `json:"max_ms"`
	Minimum float64 `json:"min_ms"`
	Mean    float64 `json:"media_ms"`
}

func distributionOf(histogram *hdrhistogram.Histogram) Distribution {
	inMilliseconds := func(microseconds int64) float64 {
		return float64(microseconds) / 1000
	}
	return Distribution{
		Samples: histogram.TotalCount(),
		P50:     inMilliseconds(histogram.ValueAtQuantile(50)),
		P75:     inMilliseconds(histogram.ValueAtQuantile(75)),
		P90:     inMilliseconds(histogram.ValueAtQuantile(90)),
		P95:     inMilliseconds(histogram.ValueAtQuantile(95)),
		P99:     inMilliseconds(histogram.ValueAtQuantile(99)),
		P999:    inMilliseconds(histogram.ValueAtQuantile(99.9)),
		Max:     inMilliseconds(histogram.Max()),
		Minimum: inMilliseconds(histogram.Min()),
		Mean:    histogram.Mean() / 1000,
	}
}

func SortKeys(aggregates map[string]*Aggregate) []string {
	keys := make([]string, 0, len(aggregates))
	for key := range aggregates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
