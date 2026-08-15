package cenario

import (
	"fmt"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
)

const VersaoDoFormato = "1"

type Cenario struct {
	VersaoDoFormato string
	Nome            string
	Alvo            string
	Variaveis       map[string]string
	Carga           PlanoDeCarga
	Passos          []Passo
}

type Passo struct {
	Nome         string
	Protocolo    string
	Configuracao protocolo.Configuracao
	Verificacoes []Verificacao
	Linha        int
}

func (p Passo) ChaveDeAgregacao() string {
	if p.Configuracao == nil {
		return p.Nome
	}
	return p.Configuracao.ChaveDeAgregacao()
}

type Verificacao struct {
	Tipo   TipoDeVerificacao
	Status int
	Texto  string
}

type TipoDeVerificacao string

const (
	VerificarStatus TipoDeVerificacao = "status"
	VerificarCorpo  TipoDeVerificacao = "corpo_contem"
)

type ModeloDeChegada string

const (
	ChegadaAberta  ModeloDeChegada = "aberto"
	ChegadaFechada ModeloDeChegada = "fechado"
)

type PlanoDeCarga struct {
	Modelo ModeloDeChegada
	Fases  []Fase
}

type TipoDeFase string

const (
	FaseRampa     TipoDeFase = "rampa"
	FasePatamar   TipoDeFase = "patamar"
	FasePico      TipoDeFase = "pico"
	FaseConstante TipoDeFase = "constante"
)

type Fase struct {
	Tipo    TipoDeFase
	De      float64
	Ate     float64
	Durante time.Duration
	Linha   int
}

func (f Fase) TaxaInicial() float64 {
	if f.Tipo == FaseRampa {
		return f.De
	}
	return f.Ate
}

func (f Fase) TaxaFinal() float64 {
	return f.Ate
}

func (c Cenario) Duracao() time.Duration {
	var total time.Duration
	for _, fase := range c.Carga.Fases {
		total += fase.Durante
	}
	return total
}

func (c Cenario) BuscarPasso(nome string) (Passo, error) {
	for _, passo := range c.Passos {
		if passo.Nome == nome {
			return passo, nil
		}
	}
	return Passo{}, fmt.Errorf("passo nao encontrado: %q", nome)
}
