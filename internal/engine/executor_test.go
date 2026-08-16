package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"gopkg.in/yaml.v3"
)

type configuracaoFalsa struct{ chave string }

func (c configuracaoFalsa) Protocolo() string        { return "falso" }
func (c configuracaoFalsa) ChaveDeAgregacao() string { return c.chave }

func (c configuracaoFalsa) Resolver(func(string) string) protocol.Configuracao { return c }

type protocoloFalso struct {
	nome     string
	entrou   chan struct{}
	libera   chan struct{}
	chamadas chan struct{}
}

func (p *protocoloFalso) Nome() string { return p.nome }

func (p *protocoloFalso) Decodificar(*yaml.Node) (protocol.Configuracao, error) {
	return configuracaoFalsa{chave: "falso"}, nil
}

func (p *protocoloFalso) Encerrar() error { return nil }

func (p *protocoloFalso) Executar(context.Context, protocol.Requisicao) protocol.Resposta {
	if p.entrou != nil {
		select {
		case p.entrou <- struct{}{}:
		default:
		}
	}
	if p.chamadas != nil {
		p.chamadas <- struct{}{}
	}
	if p.libera != nil {
		<-p.libera
	}
	return protocol.Resposta{Status: 200, Classe: protocol.Sucesso, Bytes: 7}
}

func registrarFalso(t *testing.T, nome string, falso *protocoloFalso) {
	t.Helper()
	falso.nome = nome
	protocol.Registrar(falso)
}

func cenarioFalso(nome string, taxa float64, duracao time.Duration) scenario.Cenario {
	return scenario.Cenario{
		Nome: "teste",
		Alvo: "http://alvo.invalido",
		Carga: scenario.PlanoDeCarga{
			Modelo: scenario.ChegadaAberta,
			Fases:  []scenario.Fase{{Tipo: scenario.FaseConstante, Ate: taxa, Durante: duracao}},
		},
		Passos: []scenario.Passo{{
			Nome:         "passo falso",
			Protocolo:    nome,
			Configuracao: configuracaoFalsa{chave: "falso"},
		}},
	}
}

func TestDespachoSegueOInstanteAgendadoComRelogioInjetado(t *testing.T) {
	registrarFalso(t, "falso-pontual", &protocoloFalso{})
	relogio := engine.NovoRelogioVirtual(time.Unix(1_700_000_000, 0))

	opcoes := engine.OpcoesPadrao()
	opcoes.Relogio = relogio
	opcoes.MaximoSimultaneas = 1000

	m, err := engine.Novo(cenarioFalso("falso-pontual", 100, time.Second), opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())

	if documento.Agendamento.Enviadas != 100 {
		t.Errorf("enviadas = %d, esperado 100", documento.Agendamento.Enviadas)
	}
	if documento.Agendamento.DespachosAtrasados != 0 {
		t.Errorf("despachos atrasados = %d, esperado 0 com relogio virtual", documento.Agendamento.DespachosAtrasados)
	}
	if documento.Global.Contagem != 100 {
		t.Errorf("contagem = %d, esperado 100", documento.Global.Contagem)
	}
	if !documento.ResultadoValido() {
		t.Errorf("resultado deveria ser valido, avisos: %+v", documento.Avisos)
	}
}

func TestLimiteDeVooDescartaEInvalidaOResultado(t *testing.T) {
	falso := &protocoloFalso{entrou: make(chan struct{}, 1), libera: make(chan struct{})}
	registrarFalso(t, "falso-preso", falso)

	opcoes := engine.OpcoesPadrao()
	opcoes.Relogio = engine.NovoRelogioVirtual(time.Unix(1_700_000_000, 0))
	opcoes.MaximoSimultaneas = 1

	concluido := make(chan struct{})
	var documento = make(chan any, 1)
	go func() {
		m, err := engine.Novo(cenarioFalso("falso-preso", 3, time.Second), opcoes)
		if err != nil {
			panic(err)
		}
		documento <- m.Executar(context.Background())
		close(concluido)
	}()

	<-falso.entrou
	close(falso.libera)
	<-concluido

	resultado := (<-documento).(interface {
		ResultadoValido() bool
	})

	if resultado.ResultadoValido() {
		t.Fatal("resultado com descarte por limite de voo nao pode ser valido")
	}
}

func TestVerificacaoDeStatusClassificaErro(t *testing.T) {
	registrarFalso(t, "falso-status", &protocoloFalso{})
	c := cenarioFalso("falso-status", 10, time.Second)
	c.Passos[0].Verificacoes = []scenario.Verificacao{{Tipo: scenario.VerificarStatus, Status: 201}}

	opcoes := engine.OpcoesPadrao()
	opcoes.Relogio = engine.NovoRelogioVirtual(time.Unix(1_700_000_000, 0))

	m, err := engine.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())

	if documento.Global.Erros != documento.Global.Contagem {
		t.Fatalf("esperava todas as requisicoes como erro de status, obtido %d de %d",
			documento.Global.Erros, documento.Global.Contagem)
	}
	if documento.Passos[0].ErrosPorClasse["status"] == 0 {
		t.Errorf("erro nao foi classificado como status: %+v", documento.Passos[0].ErrosPorClasse)
	}
}
