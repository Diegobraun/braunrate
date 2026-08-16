package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const bucketDuration = time.Second

// A bucket keeps a full HDR histogram while it can still receive samples, and
// the report reads two quantiles of it. Holding every one of them until the end
// of the run made memory grow about 10 MB per minute, independent of the rate,
// which closed the endurance case: a four hour run asked for gigabytes of
// histograms nobody would read. A bucket that fell this far behind the newest
// one is reduced to what the report consumes and the histogram is released.
// The window is twice the default client timeout, so a request that takes as
// long as the tool allows still lands in an open bucket.
const bucketRetention = 60 * time.Second

type Bucket struct {
	StartEpochMs int64   `json:"startEpochMs"`
	Sent         int64   `json:"sent"`
	Completed    int64   `json:"completed"`
	Errors       int64   `json:"errors"`
	TargetRate   float64 `json:"targetRate"`
	LatencyP50Ms float64 `json:"latencyP50Ms"`
	LatencyP99Ms float64 `json:"latencyP99Ms"`
	// Samples that arrived after this bucket was closed. They are in the step
	// aggregate and in the counters above; what they missed are the two
	// quantiles, and dropping them without saying so would be a number quietly
	// computed over less than it claims.
	LateSamples int64 `json:"samplesOutsideWindow,omitempty"`
	histogram   *hdrhistogram.Histogram
}

type Collector struct {
	mu             sync.Mutex
	aggregates     map[string]*Aggregate
	buckets        map[int64]*Bucket
	newestBucketMs int64
	schedulingSkew *hdrhistogram.Histogram

	start                  time.Time
	Sent                   int64
	Completed              int64
	LateDispatches         int64
	DroppedByInflightLimit int64
	LostSamples            int64
	PeakInflight           int64
	LateThreshold          time.Duration

	variety           map[string]*varietyCounter
	journeys          *hdrhistogram.Histogram
	JourneysStarted   int64
	JourneysCompleted int64

	input    chan Sample
	finished chan struct{}
}

func NewCollector(start time.Time, lateThreshold time.Duration) *Collector {
	return NewCollectorWithCapacity(start, lateThreshold, 16384)
}

func NewCollectorWithCapacity(start time.Time, lateThreshold time.Duration, capacity int) *Collector {
	collector := &Collector{
		aggregates:     map[string]*Aggregate{},
		variety:        map[string]*varietyCounter{},
		buckets:        map[int64]*Bucket{},
		schedulingSkew: hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		journeys:       hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		start:          start,
		LateThreshold:  lateThreshold,
		input:          make(chan Sample, capacity),
		finished:       make(chan struct{}),
	}
	go collector.consume()
	return collector
}

// A single goroutine writes to the histograms: HDR is not safe for concurrent
// writes, and a mutex on the hot path becomes contention at high rates.
func (collector *Collector) consume() {
	defer close(collector.finished)
	for sample := range collector.input {
		collector.apply(sample)
	}
}

func (collector *Collector) apply(sample Sample) {
	collector.mu.Lock()
	defer collector.mu.Unlock()

	key := sample.Step
	aggregate, exists := collector.aggregates[key]
	if !exists {
		aggregate = NewAggregate(sample.Step, sample.Key, sample.Protocol)
		collector.aggregates[key] = aggregate
	}
	aggregate.Record(sample)
	collector.Completed++

	bucket := collector.bucketDe(sample.ScheduledAt)
	bucket.Completed++
	if sample.Class != protocol.Success {
		bucket.Errors++
	}
	if !sample.FinishedAt.IsZero() {
		if bucket.histogram == nil {
			bucket.LateSamples++
		} else {
			save(bucket.histogram, sample.CorrectedLatency())
		}
	}
}

func (collector *Collector) bucketDe(instant time.Time) *Bucket {
	startEpochMs := instant.UnixMilli() - instant.UnixMilli()%bucketDuration.Milliseconds()
	bucket, exists := collector.buckets[startEpochMs]
	if !exists {
		bucket = &Bucket{
			StartEpochMs: startEpochMs,
			histogram:    hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		}
		collector.buckets[startEpochMs] = bucket
	}
	if startEpochMs > collector.newestBucketMs {
		collector.newestBucketMs = startEpochMs
		collector.closeBucketsBefore(startEpochMs - bucketRetention.Milliseconds())
	}
	return bucket
}

func (collector *Collector) closeBucketsBefore(limitEpochMs int64) {
	for key, bucket := range collector.buckets {
		if key < limitEpochMs {
			closeBucket(bucket)
		}
	}
}

// Reduces the bucket to what the report reads. Once this runs the histogram is
// gone and the two quantiles are final.
func closeBucket(bucket *Bucket) {
	if bucket.histogram == nil {
		return
	}
	bucket.LatencyP50Ms = float64(bucket.histogram.ValueAtQuantile(50)) / 1000
	bucket.LatencyP99Ms = float64(bucket.histogram.ValueAtQuantile(99)) / 1000
	bucket.histogram = nil
}

func (collector *Collector) Record(sample Sample) {
	select {
	case collector.input <- sample:
	default:
		collector.mu.Lock()
		collector.LostSamples++
		collector.mu.Unlock()
	}
}

func (collector *Collector) RecordDispatch(scheduled, dispatch time.Time, targetRate float64, inflight int64) {
	delay := dispatch.Sub(scheduled)
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.Sent++
	if inflight > collector.PeakInflight {
		collector.PeakInflight = inflight
	}
	if delay > 0 {
		save(collector.schedulingSkew, delay)
	} else {
		_ = collector.schedulingSkew.RecordValue(minLatencyUs)
	}
	if delay > collector.LateThreshold {
		collector.LateDispatches++
	}
	bucket := collector.bucketDe(scheduled)
	bucket.Sent++
	bucket.TargetRate = targetRate
}

// RecordJourney stores the only metric counted from the scheduled instant end
// to end, which is the one that matches what the end user feels.
func (collector *Collector) RecordJourney(scheduled, end time.Time, complete bool) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.JourneysStarted++
	if complete {
		collector.JourneysCompleted++
	}
	save(collector.journeys, end.Sub(scheduled))
}

// RecordUses counts an iteration's substitutions off the histogram hot path:
// one write per iteration, not one per request.
func (collector *Collector) RecordUses(uses map[string]string) {
	collector.recordUses(uses, nil)
}

// RecordDimensions grava o mesmo que RecordUses e guarda, junto, o que o dono
// da dimensao diz sobre ela ter colapsado. Sem isso, quem escreve o aviso
// precisa reconhecer o nome da dimensao — e ai a medicao passa a conhecer um
// protocolo em particular.
func (collector *Collector) RecordDimensions(uses map[string]string, collapses map[string]protocol.Collapse) {
	collector.recordUses(uses, collapses)
}

func (collector *Collector) recordUses(uses map[string]string, collapses map[string]protocol.Collapse) {
	if len(uses) == 0 {
		return
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	for name, value := range uses {
		counter, exists := collector.variety[name]
		if !exists {
			counter = &varietyCounter{seen: map[string]struct{}{}}
			collector.variety[name] = counter
		}
		counter.record(value)
		if note, declared := collapses[name]; declared && counter.collapse == nil {
			counter.collapse = &note
		}
	}
}

func (collector *Collector) Varieties(available Availability) []Variety {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return buildVarieties(collector.variety, available)
}

func (collector *Collector) Journeys() Distribution {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return distributionOf(collector.journeys)
}

func (collector *Collector) RecordInflightDrop() {
	collector.mu.Lock()
	collector.DroppedByInflightLimit++
	collector.mu.Unlock()
}

func (collector *Collector) Close() {
	close(collector.input)
	<-collector.finished
	collector.mu.Lock()
	defer collector.mu.Unlock()
	for _, bucket := range collector.buckets {
		closeBucket(bucket)
	}
}

func (collector *Collector) Snapshot() Snapshot {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	snapshot := Snapshot{
		Sent:           collector.Sent,
		Completed:      collector.Completed,
		LateDispatches: collector.LateDispatches,
		PeakInflight:   collector.PeakInflight,
		DesvioP99Ms:    float64(collector.schedulingSkew.ValueAtQuantile(99)) / 1000,
	}
	for _, aggregate := range collector.aggregates {
		snapshot.Errors += aggregate.Errors()
		snapshot.addLatency(aggregate)
	}
	return snapshot
}

type Snapshot struct {
	Sent           int64
	Completed      int64
	Errors         int64
	LateDispatches int64
	PeakInflight   int64
	DesvioP99Ms    float64
	LatencyP50Ms   float64
	LatencyP99Ms   float64
	mergedSamples  int64
}

func (snapshot *Snapshot) addLatency(aggregate *Aggregate) {
	distribution := aggregate.Distribution()
	if distribution.Samples == 0 {
		return
	}
	weight := float64(distribution.Samples)
	total := float64(snapshot.mergedSamples)
	snapshot.LatencyP50Ms = (snapshot.LatencyP50Ms*total + distribution.P50*weight) / (total + weight)
	snapshot.LatencyP99Ms = (snapshot.LatencyP99Ms*total + distribution.P99*weight) / (total + weight)
	snapshot.mergedSamples += distribution.Samples
}

func (collector *Collector) Aggregates() map[string]*Aggregate {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	clone := make(map[string]*Aggregate, len(collector.aggregates))
	for key, aggregate := range collector.aggregates {
		clone[key] = aggregate
	}
	return clone
}

func (collector *Collector) SchedulingSkew() Distribution {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return distributionOf(collector.schedulingSkew)
}

func (collector *Collector) Buckets() []Bucket {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	list := make([]Bucket, 0, len(collector.buckets))
	for _, bucket := range collector.buckets {
		list = append(list, *bucket)
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].StartEpochMs < list[i].StartEpochMs {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list
}

func AvailableCores() int {
	return runtime.NumCPU()
}
