package metrics

import (
	"runtime"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
)

// The time series used to hold a full HDR histogram for every second of the
// run, and the report reads two quantiles of each. Memory grew about 10 MB per
// minute whatever the rate, so a four hour endurance run — the run that finds
// the leak in the target — asked for gigabytes and died before the target did.
//
// Ten simulated minutes, heap measured at three points. Linear growth fails:
// what the last five minutes add has to be a fraction of what the first five
// did, because after the retention window the cost per minute is zero.
func TestTimeSeriesMemoryDoesNotGrowWithTheLengthOfTheRun(t *testing.T) {
	start := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	collector := NewCollector(start, 10*time.Millisecond)
	defer collector.Close()

	feed := func(from, to time.Duration) {
		for second := from; second < to; second += time.Second {
			scheduled := start.Add(second)
			for index := 0; index < 20; index++ {
				collector.apply(Sample{
					Step: "consultar", Key: "GET /pedidos", Protocol: "http",
					ScheduledAt: scheduled.Add(time.Duration(index) * 50 * time.Millisecond),
					SentAt:      scheduled.Add(time.Duration(index) * 50 * time.Millisecond),
					FinishedAt:  scheduled.Add(time.Duration(index)*50*time.Millisecond + 3*time.Millisecond),
					Class:       protocol.Success, Status: 200,
				})
			}
		}
	}

	heap := func() uint64 {
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats.HeapAlloc
	}

	feed(0, time.Minute)
	atOneMinute := heap()

	feed(time.Minute, 5*time.Minute)
	atFiveMinutes := heap()

	feed(5*time.Minute, 10*time.Minute)
	atTenMinutes := heap()

	firstStretch := int64(atFiveMinutes) - int64(atOneMinute)
	secondStretch := int64(atTenMinutes) - int64(atFiveMinutes)

	t.Logf("heap: 1min=%d KB, 5min=%d KB, 10min=%d KB (primeiros 4min=%+d KB, ultimos 5min=%+d KB)",
		atOneMinute/1024, atFiveMinutes/1024, atTenMinutes/1024, firstStretch/1024, secondStretch/1024)

	// Under the previous behaviour both stretches cost the same per minute. The
	// margin is generous on purpose: what fails here is linear growth, not the
	// noise of a few buckets still open inside the window.
	if secondStretch > firstStretch/2 && secondStretch > 8<<20 {
		t.Fatalf("a memória continua crescendo com o tempo de execução: 1min=%d KB, 5min=%d KB, 10min=%d KB",
			atOneMinute/1024, atFiveMinutes/1024, atTenMinutes/1024)
	}
	if atTenMinutes > 64<<20 {
		t.Fatalf("dez minutos de serie temporal ocuparam %d MB de heap", atTenMinutes/(1<<20))
	}
}

// Retention is what makes the release safe: a sample that arrives while its
// bucket is still open goes into the quantiles like any other.
func TestSampleInsideTheWindowStillEntersTheQuantiles(t *testing.T) {
	start := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	collector := NewCollector(start, 10*time.Millisecond)
	defer collector.Close()

	collector.apply(Sample{
		Step: "consultar", ScheduledAt: start, SentAt: start,
		FinishedAt: start.Add(5 * time.Millisecond), Class: protocol.Success,
	})
	// Half a minute later, still inside the window.
	late := start.Add(30 * time.Second)
	collector.apply(Sample{
		Step: "consultar", ScheduledAt: late, SentAt: late,
		FinishedAt: late.Add(5 * time.Millisecond), Class: protocol.Success,
	})
	collector.apply(Sample{
		Step: "consultar", ScheduledAt: start, SentAt: start,
		FinishedAt: start.Add(400 * time.Millisecond), Class: protocol.Success,
	})

	collector.mu.Lock()
	first := collector.buckets[start.UnixMilli()]
	late200 := first.histogram.ValueAtQuantile(99)
	collector.mu.Unlock()

	if late200 < 400_000 {
		t.Fatalf("a amostra que chegou dentro da janela ficou de fora dos quantis: p99 = %d us", late200)
	}
}

// Outside the window the histogram is gone and the sample cannot enter the
// quantiles. It still counts, and the bucket says how many it was — a quantile
// computed over less than it claims, in silence, is the defect this whole
// battery is about.
func TestSampleOutsideTheWindowIsCountedAndDeclared(t *testing.T) {
	start := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	collector := NewCollector(start, 10*time.Millisecond)
	defer collector.Close()

	collector.apply(Sample{
		Step: "consultar", ScheduledAt: start, SentAt: start,
		FinishedAt: start.Add(5 * time.Millisecond), Class: protocol.Success,
	})
	far := start.Add(5 * time.Minute)
	collector.apply(Sample{
		Step: "consultar", ScheduledAt: far, SentAt: far,
		FinishedAt: far.Add(5 * time.Millisecond), Class: protocol.Success,
	})
	collector.apply(Sample{
		Step: "consultar", ScheduledAt: start, SentAt: start,
		FinishedAt: start.Add(9 * time.Second), Class: protocol.Success,
	})

	collector.mu.Lock()
	first := *collector.buckets[start.UnixMilli()]
	collector.mu.Unlock()

	if first.LateSamples != 1 {
		t.Fatalf("a amostra fora da janela não foi declarada: %+v", first)
	}
	if first.Completed != 2 {
		t.Fatalf("a amostra fora da janela deixou de ser contada: %d concluidas", first.Completed)
	}
}
