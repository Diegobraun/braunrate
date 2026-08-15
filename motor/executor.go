package motor

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/protocolo"
)

type Opcoes struct {
	Versao               string
	LimiteDeVoo          int64
	LimiarDeAtraso       time.Duration
	Relogio              Relogio
	AoProgredir          func(metrica.Instantaneo, float64, time.Duration)
	IntervaloDeProgresso time.Duration
}

func OpcoesPadrao() Opcoes {
	return Opcoes{
		Versao:               "0.1.0",
		LimiteDeVoo:          20000,
		LimiarDeAtraso:       10 * time.Millisecond,
		Relogio:              RelogioDoSistema{},
		IntervaloDeProgresso: time.Second,
	}
}

type Motor struct {
	cenario cenario.Cenario
	plano   Plano
	opcoes  Opcoes
}

func Novo(c cenario.Cenario, opcoes Opcoes) *Motor {
	if opcoes.Relogio == nil {
		opcoes.Relogio = RelogioDoSistema{}
	}
	if opcoes.LimiarDeAtraso <= 0 {
		opcoes.LimiarDeAtraso = 10 * time.Millisecond
	}
	return &Motor{cenario: c, plano: CompilarPlano(c.Carga), opcoes: opcoes}
}

func (m *Motor) Plano() Plano { return m.plano }

func (m *Motor) Executar(ctx context.Context) metrica.Documento {
	relogio := m.opcoes.Relogio
	inicio := relogio.Agora()
	coletor := metrica.NovoColetor(inicio, m.opcoes.LimiarDeAtraso)

	var emVoo atomic.Int64
	var grupo sync.WaitGroup
	pararProgresso := make(chan struct{})

	if m.opcoes.AoProgredir != nil {
		go m.acompanhar(coletor, inicio, pararProgresso)
	}

	total := m.plano.TotalDeRequisicoes()
	quantidadeDePassos := int64(len(m.cenario.Passos))

	for indice := int64(0); indice < total; indice++ {
		if ctx.Err() != nil {
			break
		}
		deslocamento := m.plano.InstanteDe(indice)
		agendado := inicio.Add(deslocamento)
		relogio.EsperarAte(agendado)
		despacho := relogio.Agora()

		if emVoo.Load() >= m.opcoes.LimiteDeVoo {
			coletor.RegistrarDescartePorLimiteDeVoo()
			continue
		}

		passo := m.cenario.Passos[indice%quantidadeDePassos]
		atuais := emVoo.Add(1)
		coletor.RegistrarDespacho(agendado, despacho, m.plano.TaxaEm(deslocamento), atuais)

		grupo.Add(1)
		go func(passo cenario.Passo, agendado time.Time) {
			defer grupo.Done()
			defer emVoo.Add(-1)
			coletor.Registrar(m.executarPasso(ctx, passo, agendado))
		}(passo, agendado)
	}

	grupo.Wait()
	close(pararProgresso)
	fim := relogio.Agora()
	coletor.Encerrar()

	return metrica.MontarDocumento(coletor, metrica.EntradaDoDocumento{
		Versao:      m.opcoes.Versao,
		Cenario:     m.cenario.Nome,
		Alvo:        m.cenario.Alvo,
		Modelo:      string(m.cenario.Carga.Modelo),
		Inicio:      inicio,
		Fim:         fim,
		Fases:       m.fasesAplicadas(),
		LimiteDeVoo: m.opcoes.LimiteDeVoo,
	})
}

// A instrumentacao vive aqui, e nao dentro do protocolo: e o que garante que
// HTTP, GraphQL e Kafka produzam metrica comparavel (ADR 0003).
func (m *Motor) executarPasso(ctx context.Context, passo cenario.Passo, agendado time.Time) metrica.Amostra {
	relogio := m.opcoes.Relogio
	implementacao, existe := protocolo.Buscar(passo.Protocolo)
	envio := relogio.Agora()
	if !existe {
		return metrica.Amostra{
			Passo: passo.Nome, Chave: passo.ChaveDeAgregacao(), Protocolo: passo.Protocolo,
			InstanteAgendado: agendado, InstanteDeEnvio: envio, InstanteDeTermino: relogio.Agora(),
			Classe: protocolo.ErroDeConfigacao, Detalhe: "protocolo nao compilado neste binario",
		}
	}

	resposta := implementacao.Executar(ctx, protocolo.Requisicao{
		NomeDoPasso:  passo.Nome,
		Configuracao: passo.Configuracao,
		URLBase:      m.cenario.Alvo,
		Variaveis:    m.cenario.Variaveis,
	})
	termino := relogio.Agora()

	classe := resposta.Classe
	detalhe := resposta.Detalhe
	if classe == protocolo.Sucesso {
		if falha, motivo := verificar(passo.Verificacoes, resposta); falha != protocolo.Sucesso {
			classe = falha
			detalhe = motivo
		}
	}

	chave := resposta.Chave
	if chave == "" {
		chave = passo.ChaveDeAgregacao()
	}

	return metrica.Amostra{
		Passo:             passo.Nome,
		Chave:             chave,
		Protocolo:         passo.Protocolo,
		InstanteAgendado:  agendado,
		InstanteDeEnvio:   envio,
		InstanteDeTermino: termino,
		Classe:            classe,
		Detalhe:           detalhe,
		Status:            resposta.Status,
		Bytes:             resposta.Bytes,
	}
}

func verificar(verificacoes []cenario.Verificacao, resposta protocolo.Resposta) (protocolo.ClasseDeErro, string) {
	for _, verificacao := range verificacoes {
		switch verificacao.Tipo {
		case cenario.VerificarStatus:
			if resposta.Status != verificacao.Status {
				return protocolo.ErroDeStatus, fmt.Sprintf("esperava status %d, recebeu %d", verificacao.Status, resposta.Status)
			}
		case cenario.VerificarCorpo:
			if !bytes.Contains(resposta.Corpo, []byte(verificacao.Texto)) {
				return protocolo.ErroDeAssercao, fmt.Sprintf("corpo nao contem %q", verificacao.Texto)
			}
		}
	}
	return protocolo.Sucesso, ""
}

func (m *Motor) acompanhar(coletor *metrica.Coletor, inicio time.Time, parar <-chan struct{}) {
	intervalo := m.opcoes.IntervaloDeProgresso
	if intervalo <= 0 {
		intervalo = time.Second
	}
	tique := time.NewTicker(intervalo)
	defer tique.Stop()
	for {
		select {
		case <-parar:
			return
		case <-tique.C:
			decorrido := time.Since(inicio)
			restante := m.plano.Duracao() - decorrido
			if restante < 0 {
				restante = 0
			}
			m.opcoes.AoProgredir(coletor.Instantaneo(), m.plano.TaxaEm(decorrido), restante)
		}
	}
}

func (m *Motor) fasesAplicadas() []metrica.FaseAplicada {
	fases := make([]metrica.FaseAplicada, 0, len(m.cenario.Carga.Fases))
	for _, fase := range m.cenario.Carga.Fases {
		fases = append(fases, metrica.FaseAplicada{
			Tipo:      string(fase.Tipo),
			De:        fase.TaxaInicial(),
			Ate:       fase.TaxaFinal(),
			DuracaoMs: fase.Durante.Milliseconds(),
		})
	}
	return fases
}
