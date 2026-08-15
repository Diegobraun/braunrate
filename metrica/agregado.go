package metrica

import (
	"sort"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const (
	menorLatenciaUs   = int64(1)
	maiorLatenciaUs   = int64(600_000_000)
	digitosDePrecisao = 3
)

type Amostra struct {
	Passo             string
	Chave             string
	Protocolo         string
	InstanteAgendado  time.Time
	InstanteDeEnvio   time.Time
	InstanteDeTermino time.Time
	Classe            protocolo.ClasseDeErro
	Detalhe           string
	Status            int
	Bytes             int64
}

func (a Amostra) LatenciaCorrigida() time.Duration {
	return a.InstanteDeTermino.Sub(a.InstanteAgendado)
}

func (a Amostra) LatenciaDeServico() time.Duration {
	return a.InstanteDeTermino.Sub(a.InstanteDeEnvio)
}

type Agregado struct {
	Passo             string
	Chave             string
	Protocolo         string
	latenciaCorrigida *hdrhistogram.Histogram
	latenciaDeServico *hdrhistogram.Histogram
	Contagem          int64
	Sucessos          int64
	ErrosPorClasse    map[protocolo.ClasseDeErro]int64
	StatusPorCodigo   map[int]int64
	Bytes             int64
	Detalhes          map[string]int64
}

func NovoAgregado(passo, chave, nomeDoProtocolo string) *Agregado {
	return &Agregado{
		Passo:             passo,
		Chave:             chave,
		Protocolo:         nomeDoProtocolo,
		latenciaCorrigida: hdrhistogram.New(menorLatenciaUs, maiorLatenciaUs, digitosDePrecisao),
		latenciaDeServico: hdrhistogram.New(menorLatenciaUs, maiorLatenciaUs, digitosDePrecisao),
		ErrosPorClasse:    map[protocolo.ClasseDeErro]int64{},
		StatusPorCodigo:   map[int]int64{},
		Detalhes:          map[string]int64{},
	}
}

func (a *Agregado) Registrar(amostra Amostra) {
	a.Contagem++
	a.Bytes += amostra.Bytes
	if amostra.Status > 0 {
		a.StatusPorCodigo[amostra.Status]++
	}
	if amostra.Classe == protocolo.Sucesso {
		a.Sucessos++
	} else {
		a.ErrosPorClasse[amostra.Classe]++
		if amostra.Detalhe != "" && len(a.Detalhes) < 32 {
			a.Detalhes[amostra.Detalhe]++
		}
	}
	if !amostra.InstanteDeTermino.IsZero() {
		gravar(a.latenciaCorrigida, amostra.LatenciaCorrigida())
		if !amostra.InstanteDeEnvio.IsZero() {
			gravar(a.latenciaDeServico, amostra.LatenciaDeServico())
		}
	}
}

func gravar(histograma *hdrhistogram.Histogram, valor time.Duration) {
	microssegundos := valor.Microseconds()
	if microssegundos < menorLatenciaUs {
		microssegundos = menorLatenciaUs
	}
	if microssegundos > maiorLatenciaUs {
		microssegundos = maiorLatenciaUs
	}
	_ = histograma.RecordValue(microssegundos)
}

// Somar existe para viabilizar execucao distribuida sem reescrita: HDR
// histogram e contadores sao mergeaveis, media e percentil pre-calculado nao.
func (a *Agregado) Somar(outro *Agregado) {
	a.latenciaCorrigida.Merge(outro.latenciaCorrigida)
	a.latenciaDeServico.Merge(outro.latenciaDeServico)
	a.Contagem += outro.Contagem
	a.Sucessos += outro.Sucessos
	a.Bytes += outro.Bytes
	for classe, quantidade := range outro.ErrosPorClasse {
		a.ErrosPorClasse[classe] += quantidade
	}
	for status, quantidade := range outro.StatusPorCodigo {
		a.StatusPorCodigo[status] += quantidade
	}
	for detalhe, quantidade := range outro.Detalhes {
		a.Detalhes[detalhe] += quantidade
	}
}

func (a *Agregado) Erros() int64 {
	return a.Contagem - a.Sucessos
}

func (a *Agregado) Distribuicao() Distribuicao {
	return distribuicaoDe(a.latenciaCorrigida)
}

func (a *Agregado) DistribuicaoDeServico() Distribuicao {
	return distribuicaoDe(a.latenciaDeServico)
}

type Distribuicao struct {
	Amostras int64   `json:"amostras"`
	P50      float64 `json:"p50_ms"`
	P75      float64 `json:"p75_ms"`
	P90      float64 `json:"p90_ms"`
	P95      float64 `json:"p95_ms"`
	P99      float64 `json:"p99_ms"`
	P999     float64 `json:"p99_9_ms"`
	Maximo   float64 `json:"max_ms"`
	Minimo   float64 `json:"min_ms"`
	Media    float64 `json:"media_ms"`
}

func distribuicaoDe(histograma *hdrhistogram.Histogram) Distribuicao {
	emMilissegundos := func(microssegundos int64) float64 {
		return float64(microssegundos) / 1000
	}
	return Distribuicao{
		Amostras: histograma.TotalCount(),
		P50:      emMilissegundos(histograma.ValueAtQuantile(50)),
		P75:      emMilissegundos(histograma.ValueAtQuantile(75)),
		P90:      emMilissegundos(histograma.ValueAtQuantile(90)),
		P95:      emMilissegundos(histograma.ValueAtQuantile(95)),
		P99:      emMilissegundos(histograma.ValueAtQuantile(99)),
		P999:     emMilissegundos(histograma.ValueAtQuantile(99.9)),
		Maximo:   emMilissegundos(histograma.Max()),
		Minimo:   emMilissegundos(histograma.Min()),
		Media:    histograma.Mean() / 1000,
	}
}

func OrdenarChaves(agregados map[string]*Agregado) []string {
	chaves := make([]string, 0, len(agregados))
	for chave := range agregados {
		chaves = append(chaves, chave)
	}
	sort.Strings(chaves)
	return chaves
}
