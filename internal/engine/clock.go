package engine

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	WaitUntil(instant time.Time)
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// Sleep alone is off by milliseconds; the final spin is what keeps scheduling
// skew under 100 us at high rates. The cost is roughly one core dedicated to
// the scheduler, and it is declared in the report.
func (SystemClock) WaitUntil(instant time.Time) {
	remaining := time.Until(instant)
	if remaining <= 0 {
		return
	}
	if remaining > 2*time.Millisecond {
		time.Sleep(remaining - 1500*time.Microsecond)
	}
	for time.Now().Before(instant) {
	}
}

type VirtualClock struct {
	mu      sync.Mutex
	now     time.Time
	Esperas []time.Time
}

func NewVirtualClock(start time.Time) *VirtualClock {
	return &VirtualClock{now: start}
}

func (r *VirtualClock) Now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.now
}

func (r *VirtualClock) WaitUntil(instant time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Esperas = append(r.Esperas, instant)
	if instant.After(r.now) {
		r.now = instant
	}
}

func (r *VirtualClock) Advance(duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = r.now.Add(duration)
}
