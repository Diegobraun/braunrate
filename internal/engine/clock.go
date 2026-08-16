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

// Sleep sozinho erra na casa de milissegundos; a espera ativa final e o que
// sustenta o desvio de agendamento abaixo de 100 us em taxa alta. O custo e
// aproximadamente um nucleo dedicado ao agendador, declarado no relatorio.
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
