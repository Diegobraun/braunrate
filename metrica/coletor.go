package metrica

import (
	"runtime"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const duracaoDoBucket = time.Second

type Bucket struct {
	InicioEpochMs int64   `json:"inicio_epoch_ms"`
	Enviadas      int64   `json:"enviadas"`
	Concluidas    int64   `json:"concluidas"`
	Erros         int64   `json:"erros"`
	TaxaAlvo      float64 `json:"taxa_alvo"`
	LatenciaP50Ms float64 `json:"latencia_p50_ms"`
	LatenciaP99Ms float64 `json:"latencia_p99_ms"`
	histograma    *hdrhistogram.Histogram
}

type Coletor struct {
	mutex               sync.Mutex
	agregados           map[string]*Agregado
	buckets             map[int64]*Bucket
	desvioDeAgendamento *hdrhistogram.Histogram

	inicio                    time.Time
	Enviadas                  int64
	Concluidas                int64
	DespachosAtrasados        int64
	DescartadasPorLimiteDeVoo int64
	AmostrasPerdidas          int64
	PicoEmVoo                 int64
	LimiarDeAtraso            time.Duration

	jornadas          *hdrhistogram.Histogram
	JornadasIniciadas int64
	JornadasCompletas int64

	entrada chan Amostra
	pronto  chan struct{}
}

func NovoColetor(inicio time.Time, limiarDeAtraso time.Duration) *Coletor {
	return NovoColetorComCapacidade(inicio, limiarDeAtraso, 16384)
}

func NovoColetorComCapacidade(inicio time.Time, limiarDeAtraso time.Duration, capacidade int) *Coletor {
	c := &Coletor{
		agregados:           map[string]*Agregado{},
		buckets:             map[int64]*Bucket{},
		desvioDeAgendamento: hdrhistogram.New(menorLatenciaUs, maiorLatenciaUs, digitosDePrecisao),
		jornadas:            hdrhistogram.New(menorLatenciaUs, maiorLatenciaUs, digitosDePrecisao),
		inicio:              inicio,
		LimiarDeAtraso:      limiarDeAtraso,
		entrada:             make(chan Amostra, capacidade),
		pronto:              make(chan struct{}),
	}
	go c.consumir()
	return c
}

// Uma goroutine unica grava nos histogramas: HDR nao suporta escrita
// concorrente, e um mutex no caminho quente vira contencao em taxa alta.
func (c *Coletor) consumir() {
	defer close(c.pronto)
	for amostra := range c.entrada {
		c.aplicar(amostra)
	}
}

func (c *Coletor) aplicar(amostra Amostra) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	chave := amostra.Passo
	agregado, existe := c.agregados[chave]
	if !existe {
		agregado = NovoAgregado(amostra.Passo, amostra.Chave, amostra.Protocolo)
		c.agregados[chave] = agregado
	}
	agregado.Registrar(amostra)
	c.Concluidas++

	bucket := c.bucketDe(amostra.InstanteAgendado)
	bucket.Concluidas++
	if amostra.Classe != protocolo.Sucesso {
		bucket.Erros++
	}
	if !amostra.InstanteDeTermino.IsZero() {
		gravar(bucket.histograma, amostra.LatenciaCorrigida())
	}
}

func (c *Coletor) bucketDe(instante time.Time) *Bucket {
	inicioEpochMs := instante.UnixMilli() - instante.UnixMilli()%duracaoDoBucket.Milliseconds()
	bucket, existe := c.buckets[inicioEpochMs]
	if !existe {
		bucket = &Bucket{
			InicioEpochMs: inicioEpochMs,
			histograma:    hdrhistogram.New(menorLatenciaUs, maiorLatenciaUs, digitosDePrecisao),
		}
		c.buckets[inicioEpochMs] = bucket
	}
	return bucket
}

func (c *Coletor) Registrar(amostra Amostra) {
	select {
	case c.entrada <- amostra:
	default:
		c.mutex.Lock()
		c.AmostrasPerdidas++
		c.mutex.Unlock()
	}
}

func (c *Coletor) RegistrarDespacho(agendado, despacho time.Time, taxaAlvo float64, emVoo int64) {
	atraso := despacho.Sub(agendado)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.Enviadas++
	if emVoo > c.PicoEmVoo {
		c.PicoEmVoo = emVoo
	}
	if atraso > 0 {
		gravar(c.desvioDeAgendamento, atraso)
	} else {
		_ = c.desvioDeAgendamento.RecordValue(menorLatenciaUs)
	}
	if atraso > c.LimiarDeAtraso {
		c.DespachosAtrasados++
	}
	bucket := c.bucketDe(agendado)
	bucket.Enviadas++
	bucket.TaxaAlvo = taxaAlvo
}

// A jornada e a unica metrica que continua contada do instante agendado do
// inicio ao fim: e a que vale para a experiencia do usuario final.
func (c *Coletor) RegistrarJornada(agendado, fim time.Time, completa bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.JornadasIniciadas++
	if completa {
		c.JornadasCompletas++
	}
	gravar(c.jornadas, fim.Sub(agendado))
}

func (c *Coletor) Jornadas() Distribuicao {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return distribuicaoDe(c.jornadas)
}

func (c *Coletor) RegistrarDescartePorLimiteDeVoo() {
	c.mutex.Lock()
	c.DescartadasPorLimiteDeVoo++
	c.mutex.Unlock()
}

func (c *Coletor) Encerrar() {
	close(c.entrada)
	<-c.pronto
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, bucket := range c.buckets {
		bucket.LatenciaP50Ms = float64(bucket.histograma.ValueAtQuantile(50)) / 1000
		bucket.LatenciaP99Ms = float64(bucket.histograma.ValueAtQuantile(99)) / 1000
	}
}

func (c *Coletor) Instantaneo() Instantaneo {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	instantaneo := Instantaneo{
		Enviadas:           c.Enviadas,
		Concluidas:         c.Concluidas,
		DespachosAtrasados: c.DespachosAtrasados,
		PicoEmVoo:          c.PicoEmVoo,
		DesvioP99Ms:        float64(c.desvioDeAgendamento.ValueAtQuantile(99)) / 1000,
	}
	for _, agregado := range c.agregados {
		instantaneo.Erros += agregado.Erros()
		instantaneo.somarLatencia(agregado)
	}
	return instantaneo
}

type Instantaneo struct {
	Enviadas           int64
	Concluidas         int64
	Erros              int64
	DespachosAtrasados int64
	PicoEmVoo          int64
	DesvioP99Ms        float64
	LatenciaP50Ms      float64
	LatenciaP99Ms      float64
	amostrasSomadas    int64
}

func (i *Instantaneo) somarLatencia(agregado *Agregado) {
	distribuicao := agregado.Distribuicao()
	if distribuicao.Amostras == 0 {
		return
	}
	peso := float64(distribuicao.Amostras)
	total := float64(i.amostrasSomadas)
	i.LatenciaP50Ms = (i.LatenciaP50Ms*total + distribuicao.P50*peso) / (total + peso)
	i.LatenciaP99Ms = (i.LatenciaP99Ms*total + distribuicao.P99*peso) / (total + peso)
	i.amostrasSomadas += distribuicao.Amostras
}

func (c *Coletor) Agregados() map[string]*Agregado {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	copia := make(map[string]*Agregado, len(c.agregados))
	for chave, agregado := range c.agregados {
		copia[chave] = agregado
	}
	return copia
}

func (c *Coletor) DesvioDeAgendamento() Distribuicao {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return distribuicaoDe(c.desvioDeAgendamento)
}

func (c *Coletor) Buckets() []Bucket {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	lista := make([]Bucket, 0, len(c.buckets))
	for _, bucket := range c.buckets {
		lista = append(lista, *bucket)
	}
	for i := 0; i < len(lista); i++ {
		for j := i + 1; j < len(lista); j++ {
			if lista[j].InicioEpochMs < lista[i].InicioEpochMs {
				lista[i], lista[j] = lista[j], lista[i]
			}
		}
	}
	return lista
}

func NucleosDisponiveis() int {
	return runtime.NumCPU()
}
