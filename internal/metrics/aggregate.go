package metrics

import (
	"fmt"
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
	CorrectedLatency LatencyKind = "corrected"
	ServiceLatency   LatencyKind = "service"
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
	Messages    int64
}

func (sample Sample) CorrectedLatency() time.Duration {
	return sample.FinishedAt.Sub(sample.ScheduledAt)
}

func (sample Sample) ServiceLatency() time.Duration {
	return sample.FinishedAt.Sub(sample.SentAt)
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
	Messages         int64
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

func (aggregate *Aggregate) Record(sample Sample) {
	if aggregate.LatencyKind == "" {
		aggregate.LatencyKind = sample.LatencyKind
	}
	aggregate.Count++
	aggregate.Bytes += sample.Bytes
	aggregate.Messages += sample.Messages
	if sample.Status > 0 {
		aggregate.StatusByCode[sample.Status]++
	}
	if sample.Class == protocol.Success {
		aggregate.Successes++
	} else {
		aggregate.ErrorsByClass[sample.Class]++
		if sample.Detail != "" && len(aggregate.Details) < 32 {
			aggregate.Details[sample.Detail]++
		}
	}
	if !sample.FinishedAt.IsZero() {
		save(aggregate.correctedLatency, sample.CorrectedLatency())
		if !sample.SentAt.IsZero() {
			save(aggregate.serviceLatency, sample.ServiceLatency())
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
func (aggregate *Aggregate) Add(other *Aggregate) {
	aggregate.correctedLatency.Merge(other.correctedLatency)
	aggregate.serviceLatency.Merge(other.serviceLatency)
	aggregate.Count += other.Count
	aggregate.Successes += other.Successes
	aggregate.Bytes += other.Bytes
	aggregate.Messages += other.Messages
	for class, count := range other.ErrorsByClass {
		aggregate.ErrorsByClass[class] += count
	}
	for status, count := range other.StatusByCode {
		aggregate.StatusByCode[status] += count
	}
	for detail, count := range other.Details {
		aggregate.Details[detail] += count
	}
}

func (aggregate *Aggregate) Errors() int64 {
	return aggregate.Count - aggregate.Successes
}

func (aggregate *Aggregate) Distribution() Distribution {
	return distributionOf(aggregate.correctedLatency)
}

func (aggregate *Aggregate) ServiceDistribution() Distribution {
	return distributionOf(aggregate.serviceLatency)
}

type Distribution struct {
	Samples int64   `json:"samples"`
	P50     float64 `json:"p50Ms"`
	P75     float64 `json:"p75Ms"`
	P90     float64 `json:"p90Ms"`
	P95     float64 `json:"p95Ms"`
	P99     float64 `json:"p99Ms"`
	P999    float64 `json:"p999Ms"`
	Max     float64 `json:"maxMs"`
	Minimum float64 `json:"minMs"`
	Mean    float64 `json:"meanMs"`
	// The histogram every field above is a projection of, in the HDR V2
	// compressed encoding. Percentiles and means do not add, so this is the only
	// field through which two documents can be summed — which is what ADR 0003
	// §5 promised and the format did not deliver.
	Histogram string `json:"histogram,omitempty"`
}

// Merged returns the distribution of the two histograms added. It is exact:
// adding HDR histograms adds their counts bucket by bucket, and the percentiles
// come out of the sum, never out of the two sets of percentiles.
func (distribution Distribution) Merged(other Distribution) (Distribution, error) {
	first, err := decodeHistogram(distribution.Histogram)
	if err != nil {
		return Distribution{}, err
	}
	second, err := decodeHistogram(other.Histogram)
	if err != nil {
		return Distribution{}, err
	}
	switch {
	case first == nil:
		return other, nil
	case second == nil:
		return distribution, nil
	}
	first.Merge(second)
	return distributionOf(first), nil
}

func encodeHistogram(histogram *hdrhistogram.Histogram) string {
	if histogram == nil || histogram.TotalCount() == 0 {
		return ""
	}
	encoded, err := histogram.Encode(hdrhistogram.V2CompressedEncodingCookieBase)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func decodeHistogram(encoded string) (*hdrhistogram.Histogram, error) {
	if encoded == "" {
		return nil, nil
	}
	histogram, err := hdrhistogram.Decode([]byte(encoded))
	if err != nil {
		return nil, fmt.Errorf("histograma ilegivel no resultado: %w", err)
	}
	return histogram, nil
}

func distributionOf(histogram *hdrhistogram.Histogram) Distribution {
	inMilliseconds := func(microseconds int64) float64 {
		return float64(microseconds) / 1000
	}
	return Distribution{
		Samples:   histogram.TotalCount(),
		P50:       inMilliseconds(histogram.ValueAtQuantile(50)),
		P75:       inMilliseconds(histogram.ValueAtQuantile(75)),
		P90:       inMilliseconds(histogram.ValueAtQuantile(90)),
		P95:       inMilliseconds(histogram.ValueAtQuantile(95)),
		P99:       inMilliseconds(histogram.ValueAtQuantile(99)),
		P999:      inMilliseconds(histogram.ValueAtQuantile(99.9)),
		Max:       inMilliseconds(histogram.Max()),
		Minimum:   inMilliseconds(histogram.Min()),
		Mean:      histogram.Mean() / 1000,
		Histogram: encodeHistogram(histogram),
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
