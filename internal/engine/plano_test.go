package engine_test

import (
	"math"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func planoDe(fases ...scenario.Fase) engine.Plano {
	return engine.CompilarPlano(scenario.PlanoDeCarga{Modelo: scenario.ChegadaAberta, Fases: fases})
}

func TestTaxaConstanteAgendaEmIntervaloFixo(t *testing.T) {
	plano := planoDe(scenario.Fase{Tipo: scenario.FaseConstante, Ate: 100, Durante: 2 * time.Second})

	if total := plano.TotalDeRequisicoes(); total != 200 {
		t.Fatalf("total = %d, esperado 200", total)
	}
	for _, indice := range []int64{0, 1, 50, 199} {
		esperado := time.Duration(indice) * 10 * time.Millisecond
		if obtido := plano.InstanteDe(indice); obtido != esperado {
			t.Errorf("InstanteDe(%d) = %v, esperado %v", indice, obtido, esperado)
		}
	}
}

func TestRampaAgendaPelaIntegralDaTaxa(t *testing.T) {
	plano := planoDe(scenario.Fase{Tipo: scenario.FaseRampa, De: 0, Ate: 100, Durante: 10 * time.Second})

	if total := plano.TotalDeRequisicoes(); total != 500 {
		t.Fatalf("total = %d, esperado 500 (area do triangulo)", total)
	}
	if taxa := plano.TaxaEm(5 * time.Second); math.Abs(taxa-50) > 0.001 {
		t.Errorf("TaxaEm(5s) = %v, esperado 50", taxa)
	}

	metade := plano.InstanteDe(250)
	esperado := 7071 * time.Millisecond
	if diferenca := metade - esperado; diferenca > 5*time.Millisecond || diferenca < -5*time.Millisecond {
		t.Errorf("metade das requisicoes em %v, esperado ~%v", metade, esperado)
	}
}

func TestPerfisEmSequenciaSomamDuracaoEQuantidade(t *testing.T) {
	plano := planoDe(
		scenario.Fase{Tipo: scenario.FaseRampa, De: 0, Ate: 100, Durante: 2 * time.Second},
		scenario.Fase{Tipo: scenario.FasePatamar, Ate: 100, Durante: 3 * time.Second},
		scenario.Fase{Tipo: scenario.FasePico, Ate: 500, Durante: 1 * time.Second},
	)

	if duracao := plano.Duracao(); duracao != 6*time.Second {
		t.Errorf("duracao = %v, esperado 6s", duracao)
	}
	if total := plano.TotalDeRequisicoes(); total != 100+300+500 {
		t.Errorf("total = %d, esperado 900", total)
	}
	if taxa := plano.TaxaEm(5*time.Second + 500*time.Millisecond); taxa != 500 {
		t.Errorf("taxa no pico = %v, esperado 500", taxa)
	}
}

func TestInstantesSaoMonotonicos(t *testing.T) {
	plano := planoDe(
		scenario.Fase{Tipo: scenario.FaseRampa, De: 10, Ate: 200, Durante: 3 * time.Second},
		scenario.Fase{Tipo: scenario.FasePatamar, Ate: 200, Durante: 2 * time.Second},
	)

	anterior := time.Duration(-1)
	for indice := int64(0); indice < plano.TotalDeRequisicoes(); indice++ {
		instante := plano.InstanteDe(indice)
		if instante < anterior {
			t.Fatalf("instante %d (%v) veio antes do anterior (%v)", indice, instante, anterior)
		}
		if instante > plano.Duracao() {
			t.Fatalf("instante %d (%v) passou da duracao do plano (%v)", indice, instante, plano.Duracao())
		}
		anterior = instante
	}
}
