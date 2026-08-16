package engine

import (
	"math"
	"time"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

type faseCompilada struct {
	taxaInicial   float64
	taxaFinal     float64
	duracao       time.Duration
	inicio        time.Duration
	acumuladoAtes float64
}

type Plano struct {
	fases              []faseCompilada
	duracao            time.Duration
	totalDeRequisicoes int64
}

// O instante agendado sai da inversao da integral da taxa, e nao de um
// contador incremental: contador acumula erro e faz a taxa efetiva derivar
// da taxa declarada ao longo de uma execucao longa.
func CompilarPlano(plano scenario.PlanoDeCarga) Plano {
	compilado := Plano{}
	var inicio time.Duration
	var acumulado float64

	for _, fase := range plano.Fases {
		f := faseCompilada{
			taxaInicial:   fase.TaxaInicial(),
			taxaFinal:     fase.TaxaFinal(),
			duracao:       fase.Durante,
			inicio:        inicio,
			acumuladoAtes: acumulado,
		}
		acumulado += quantidadeNaFase(f, fase.Durante)
		inicio += fase.Durante
		compilado.fases = append(compilado.fases, f)
	}

	compilado.duracao = inicio
	compilado.totalDeRequisicoes = int64(math.Floor(acumulado))
	return compilado
}

func quantidadeNaFase(f faseCompilada, ate time.Duration) float64 {
	if f.duracao <= 0 {
		return 0
	}
	t := ate.Seconds()
	d := f.duracao.Seconds()
	inclinacao := (f.taxaFinal - f.taxaInicial) / d
	return f.taxaInicial*t + inclinacao*t*t/2
}

func (p Plano) Duracao() time.Duration { return p.duracao }

func (p Plano) TotalDeRequisicoes() int64 { return p.totalDeRequisicoes }

func (p Plano) TaxaEm(instante time.Duration) float64 {
	for _, fase := range p.fases {
		if instante < fase.inicio || instante >= fase.inicio+fase.duracao {
			continue
		}
		decorrido := (instante - fase.inicio).Seconds()
		inclinacao := (fase.taxaFinal - fase.taxaInicial) / fase.duracao.Seconds()
		return fase.taxaInicial + inclinacao*decorrido
	}
	return 0
}

func (p Plano) InstanteDe(indice int64) time.Duration {
	alvo := float64(indice)
	for posicao, fase := range p.fases {
		naFase := alvo - fase.acumuladoAtes
		ultima := posicao == len(p.fases)-1
		if !ultima && naFase >= quantidadeNaFase(fase, fase.duracao) {
			continue
		}
		instante := fase.inicio + instanteNaFase(fase, naFase)
		if instante > p.duracao {
			return p.duracao
		}
		return instante
	}
	return p.duracao
}

func instanteNaFase(f faseCompilada, quantidade float64) time.Duration {
	if quantidade <= 0 {
		return 0
	}
	d := f.duracao.Seconds()
	inclinacao := (f.taxaFinal - f.taxaInicial) / d
	var segundos float64
	if math.Abs(inclinacao) < 1e-12 {
		segundos = quantidade / f.taxaInicial
	} else {
		a := inclinacao / 2
		b := f.taxaInicial
		segundos = (-b + math.Sqrt(b*b+4*a*quantidade)) / (2 * a)
	}
	return time.Duration(segundos * float64(time.Second))
}
