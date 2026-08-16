package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const bucketDuration = time.Second

type Bucket struct {
	StartEpochMs int64   `json:"inicio_epoch_ms"`
	Sent         int64   `json:"enviadas"`
	Completed    int64   `json:"concluidas"`
	Errors       int64   `json:"erros"`
	TargetRate   float64 `json:"taxa_alvo"`
	LatencyP50Ms float64 `json:"latencia_p50_ms"`
	LatencyP99Ms float64 `json:"latencia_p99_ms"`
	histogram    *hdrhistogram.Histogram
}

type Collector struct {
	mu             sync.Mutex
	aggregates     map[string]*Aggregate
	buckets        map[int64]*Bucket
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

	input  chan Sample
	pronto chan struct{}
}

func NewCollector(start time.Time, lateThreshold time.Duration) *Collector {
	return NewCollectorWithCapacity(start, lateThreshold, 16384)
}

func NewCollectorWithCapacity(start time.Time, lateThreshold time.Duration, capacity int) *Collector {
	c := &Collector{
		aggregates:     map[string]*Aggregate{},
		variety:        map[string]*varietyCounter{},
		buckets:        map[int64]*Bucket{},
		schedulingSkew: hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		journeys:       hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		start:          start,
		LateThreshold:  lateThreshold,
		input:          make(chan Sample, capacity),
		pronto:         make(chan struct{}),
	}
	go c.consumir()
	return c
}

// A single goroutine writes to the histograms: HDR is not safe for concurrent
// writes, and a mutex on the hot path becomes contention at high rates.
func (c *Collector) consumir() {
	defer close(c.pronto)
	for sample := range c.input {
		c.apply(sample)
	}
}

func (c *Collector) apply(sample Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := sample.Step
	aggregate, exists := c.aggregates[key]
	if !exists {
		aggregate = NewAggregate(sample.Step, sample.Key, sample.Protocol)
		c.aggregates[key] = aggregate
	}
	aggregate.Record(sample)
	c.Completed++

	bucket := c.bucketDe(sample.ScheduledAt)
	bucket.Completed++
	if sample.Class != protocol.Success {
		bucket.Errors++
	}
	if !sample.FinishedAt.IsZero() {
		save(bucket.histogram, sample.CorrectedLatency())
	}
}

func (c *Collector) bucketDe(instant time.Time) *Bucket {
	startEpochMs := instant.UnixMilli() - instant.UnixMilli()%bucketDuration.Milliseconds()
	bucket, exists := c.buckets[startEpochMs]
	if !exists {
		bucket = &Bucket{
			StartEpochMs: startEpochMs,
			histogram:    hdrhistogram.New(minLatencyUs, maxLatencyUs, precisionDigits),
		}
		c.buckets[startEpochMs] = bucket
	}
	return bucket
}

func (c *Collector) Record(sample Sample) {
	select {
	case c.input <- sample:
	default:
		c.mu.Lock()
		c.LostSamples++
		c.mu.Unlock()
	}
}

func (c *Collector) RecordDispatch(scheduled, dispatch time.Time, targetRate float64, inflight int64) {
	delay := dispatch.Sub(scheduled)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Sent++
	if inflight > c.PeakInflight {
		c.PeakInflight = inflight
	}
	if delay > 0 {
		save(c.schedulingSkew, delay)
	} else {
		_ = c.schedulingSkew.RecordValue(minLatencyUs)
	}
	if delay > c.LateThreshold {
		c.LateDispatches++
	}
	bucket := c.bucketDe(scheduled)
	bucket.Sent++
	bucket.TargetRate = targetRate
}

// RecordJourney stores the only metric counted from the scheduled instant end
// to end, which is the one that matches what the end user feels.
func (c *Collector) RecordJourney(scheduled, end time.Time, complete bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.JourneysStarted++
	if complete {
		c.JourneysCompleted++
	}
	save(c.journeys, end.Sub(scheduled))
}

// RecordUses counts an iteration's substitutions off the histogram hot path:
// one write per iteration, not one per request.
func (c *Collector) RecordUses(uses map[string]string) {
	if len(uses) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, value := range uses {
		counter, exists := c.variety[name]
		if !exists {
			counter = &varietyCounter{seen: map[string]struct{}{}}
			c.variety[name] = counter
		}
		counter.record(value)
	}
}

func (c *Collector) Varieties(available Availability) []Variety {
	c.mu.Lock()
	defer c.mu.Unlock()
	return buildVarieties(c.variety, available)
}

func (c *Collector) Journeys() Distribution {
	c.mu.Lock()
	defer c.mu.Unlock()
	return distributionOf(c.journeys)
}

func (c *Collector) RecordInflightDrop() {
	c.mu.Lock()
	c.DroppedByInflightLimit++
	c.mu.Unlock()
}

func (c *Collector) Close() {
	close(c.input)
	<-c.pronto
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, bucket := range c.buckets {
		bucket.LatencyP50Ms = float64(bucket.histogram.ValueAtQuantile(50)) / 1000
		bucket.LatencyP99Ms = float64(bucket.histogram.ValueAtQuantile(99)) / 1000
	}
}

func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot := Snapshot{
		Sent:           c.Sent,
		Completed:      c.Completed,
		LateDispatches: c.LateDispatches,
		PeakInflight:   c.PeakInflight,
		DesvioP99Ms:    float64(c.schedulingSkew.ValueAtQuantile(99)) / 1000,
	}
	for _, aggregate := range c.aggregates {
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

func (i *Snapshot) addLatency(aggregate *Aggregate) {
	distribution := aggregate.Distribution()
	if distribution.Samples == 0 {
		return
	}
	weight := float64(distribution.Samples)
	total := float64(i.mergedSamples)
	i.LatencyP50Ms = (i.LatencyP50Ms*total + distribution.P50*weight) / (total + weight)
	i.LatencyP99Ms = (i.LatencyP99Ms*total + distribution.P99*weight) / (total + weight)
	i.mergedSamples += distribution.Samples
}

func (c *Collector) Aggregates() map[string]*Aggregate {
	c.mu.Lock()
	defer c.mu.Unlock()
	clone := make(map[string]*Aggregate, len(c.aggregates))
	for key, aggregate := range c.aggregates {
		clone[key] = aggregate
	}
	return clone
}

func (c *Collector) SchedulingSkew() Distribution {
	c.mu.Lock()
	defer c.mu.Unlock()
	return distributionOf(c.schedulingSkew)
}

func (c *Collector) Buckets() []Bucket {
	c.mu.Lock()
	defer c.mu.Unlock()
	list := make([]Bucket, 0, len(c.buckets))
	for _, bucket := range c.buckets {
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
