package engine

import (
	"sync"
	"time"
)

type Relogio interface {
	Agora() time.Time
	EsperarAte(instante time.Time)
}

type RelogioDoSistema struct{}

func (RelogioDoSistema) Agora() time.Time { return time.Now() }

// Sleep sozinho erra na casa de milissegundos; a espera ativa final e o que
// sustenta o desvio de agendamento abaixo de 100 us em taxa alta. O custo e
// aproximadamente um nucleo dedicado ao agendador, declarado no relatorio.
func (RelogioDoSistema) EsperarAte(instante time.Time) {
	restante := time.Until(instante)
	if restante <= 0 {
		return
	}
	if restante > 2*time.Millisecond {
		time.Sleep(restante - 1500*time.Microsecond)
	}
	for time.Now().Before(instante) {
	}
}

type RelogioVirtual struct {
	mutex   sync.Mutex
	agora   time.Time
	Esperas []time.Time
}

func NovoRelogioVirtual(inicio time.Time) *RelogioVirtual {
	return &RelogioVirtual{agora: inicio}
}

func (r *RelogioVirtual) Agora() time.Time {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.agora
}

func (r *RelogioVirtual) EsperarAte(instante time.Time) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.Esperas = append(r.Esperas, instante)
	if instante.After(r.agora) {
		r.agora = instante
	}
}

func (r *RelogioVirtual) Avancar(duracao time.Duration) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.agora = r.agora.Add(duracao)
}
