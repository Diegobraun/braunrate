package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Diegobraun/braunrate/internal/auth"
	"github.com/Diegobraun/braunrate/internal/correlation"
	"github.com/Diegobraun/braunrate/internal/data"
	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/runtime"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Opcoes struct {
	Versao               string
	MaximoSimultaneas    int64
	LimiarDeAtraso       time.Duration
	Relogio              Relogio
	RaizDeDados          string
	AoProgredir          func(metrics.Instantaneo, float64, time.Duration)
	IntervaloDeProgresso time.Duration
	AoObservarPasso      func(Observacao)
}

func OpcoesPadrao() Opcoes {
	return Opcoes{
		Versao:               "0.2.0",
		MaximoSimultaneas:    20000,
		LimiarDeAtraso:       10 * time.Millisecond,
		Relogio:              RelogioDoSistema{},
		IntervaloDeProgresso: time.Second,
	}
}

// Observacao existe para o modo de depuracao: um usuario, uma iteracao, tudo
// visivel. O caminho de execucao e o mesmo da carga.
type Observacao struct {
	Passo        string
	Chave        string
	Configuracao protocol.Configuracao
	Resposta     protocol.Resposta
	Capturado    map[string]string
	Variaveis    map[string]string
	Falhas       []string
	Classe       protocol.ClasseDeErro
	Duracao      time.Duration
}

type Motor struct {
	cenario      scenario.Cenario
	plano        Plano
	opcoes       Opcoes
	fontes       []data.Fonte
	autenticador *auth.Gerenciador
	coletor      atomic.Pointer[metrics.Coletor]
}

func Novo(c scenario.Cenario, opcoes Opcoes) (*Motor, error) {
	if opcoes.Relogio == nil {
		opcoes.Relogio = RelogioDoSistema{}
	}
	if opcoes.LimiarDeAtraso <= 0 {
		opcoes.LimiarDeAtraso = 10 * time.Millisecond
	}
	if opcoes.MaximoSimultaneas <= 0 {
		opcoes.MaximoSimultaneas = 20000
	}

	m := &Motor{cenario: c, plano: CompilarPlano(c.Carga), opcoes: opcoes}

	for _, fonte := range c.Dados {
		aberta, err := data.Abrir(fonte, opcoes.RaizDeDados)
		if err != nil {
			return nil, err
		}
		m.fontes = append(m.fontes, aberta)
	}

	if c.Autenticacao != nil {
		m.autenticador = auth.Novo(*c.Autenticacao, m.executarPassoDeAutenticacao, opcoes.Relogio)
	}
	return m, nil
}

func (m *Motor) Plano() Plano { return m.plano }

// Depurar roda uma unica iteracao pelo mesmo caminho da carga: mesmo motor,
// mesma resolucao de variavel, mesma captura. So a carga muda.
func (m *Motor) Depurar(ctx context.Context) ([]Observacao, map[string]string, error) {
	valores := runtime.Novo(0, 0, m.cenario.Variaveis)

	for _, fonte := range m.fontes {
		registro, err := fonte.Proximo(0)
		if err != nil {
			return nil, valores.Valores(), err
		}
		valores.DefinirVarios(registro)
	}

	var cabecalhoDeAutenticacao [2]string
	if m.autenticador != nil {
		nome, valor, err := m.autenticador.Cabecalho(ctx, valores)
		if err != nil {
			return nil, valores.Valores(), err
		}
		cabecalhoDeAutenticacao = [2]string{nome, valor}
	}

	var observacoes []Observacao
	instante := m.opcoes.Relogio.Agora()
	for _, passo := range m.cenario.Passos {
		amostra, observacao := m.executarPasso(ctx, passo, instante, valores, cabecalhoDeAutenticacao)
		observacoes = append(observacoes, observacao)
		instante = amostra.InstanteDeTermino
		if amostra.Classe != protocol.Sucesso {
			break
		}
	}
	return observacoes, valores.Valores(), nil
}

func (m *Motor) Cenario() scenario.Cenario { return m.cenario }

func (m *Motor) RaizDeDados() string { return filepath.Clean(m.opcoes.RaizDeDados) }

func (m *Motor) Executar(ctx context.Context) metrics.Documento {
	relogio := m.opcoes.Relogio
	inicio := relogio.Agora()
	if err := m.prepararProtocolos(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	coletor := metrics.NovoColetor(inicio, m.opcoes.LimiarDeAtraso)
	m.coletor.Store(coletor)

	var emVoo atomic.Int64
	var grupo sync.WaitGroup
	pararProgresso := make(chan struct{})

	if m.opcoes.AoProgredir != nil {
		go m.acompanhar(coletor, inicio, pararProgresso)
	}

	total := m.plano.TotalDeRequisicoes()
	for indice := int64(0); indice < total; indice++ {
		if ctx.Err() != nil {
			break
		}
		deslocamento := m.plano.InstanteDe(indice)
		agendado := inicio.Add(deslocamento)
		relogio.EsperarAte(agendado)
		despacho := relogio.Agora()

		if emVoo.Load() >= m.opcoes.MaximoSimultaneas {
			coletor.RegistrarDescartePorLimiteDeVoo()
			continue
		}

		atuais := emVoo.Add(1)
		coletor.RegistrarDespacho(agendado, despacho, m.plano.TaxaEm(deslocamento), atuais)

		grupo.Add(1)
		go func(usuarioVirtual int64, agendado time.Time) {
			defer grupo.Done()
			defer emVoo.Add(-1)
			m.executarIteracao(ctx, usuarioVirtual, agendado, coletor)
		}(indice, agendado)
	}

	grupo.Wait()
	close(pararProgresso)
	fim := relogio.Agora()
	coletor.Encerrar()

	return metrics.MontarDocumento(coletor, metrics.EntradaDoDocumento{
		Versao:            m.opcoes.Versao,
		Cenario:           m.cenario.Nome,
		Alvo:              m.cenario.Alvo,
		Modelo:            string(m.cenario.Carga.Modelo),
		Inicio:            inicio,
		Fim:               fim,
		Fases:             m.fasesAplicadas(),
		MaximoSimultaneas: m.opcoes.MaximoSimultaneas,
		Sementes:          m.sementes(),
		Disponibilidade:   m.disponibilidade(),
		Autenticacoes:     m.obtencoesDeAutenticacao(),
		AvisosDoCenario:   m.avisosDoCenario(),
	})
}

// Espera por sondagem mede em degraus do intervalo: o numero e sempre maior ou
// igual ao real, e quem le precisa saber disso antes de comparar com um SLO.
func (m *Motor) avisosDoCenario() []metrics.Aviso {
	var avisos []metrics.Aviso
	for _, passo := range m.cenario.Passos {
		sondagem, sonda := passo.Configuracao.(interface{ IntervaloDeSondagem() time.Duration })
		if !sonda {
			continue
		}
		intervalo := sondagem.IntervaloDeSondagem()
		if intervalo <= 0 {
			continue
		}
		avisos = append(avisos, metrics.Aviso{
			Tipo:      "espera_por_sondagem",
			Gravidade: metrics.GravidadeBaixa,
			Mensagem: fmt.Sprintf("o passo %q espera sondando a cada %s: a latencia dele tem essa granularidade e fica maior que a real, nunca menor",
				passo.Nome, intervalo),
			Evidencia: passo.ChaveDeAgregacao(),
		})
	}
	return avisos
}

// Cada chegada agendada e uma iteracao inteira do cenario: e o que faz o valor
// capturado num passo chegar ao passo seguinte. Se um passo falha, a iteracao
// para — os passos seguintes dependeriam de uma captura que nao aconteceu.
func (m *Motor) executarIteracao(ctx context.Context, usuarioVirtual int64, agendado time.Time, coletor *metrics.Coletor) {
	valores := runtime.Novo(usuarioVirtual, usuarioVirtual, m.cenario.Variaveis)

	for _, fonte := range m.fontes {
		registro, err := fonte.Proximo(usuarioVirtual)
		if err != nil {
			coletor.Registrar(metrics.Amostra{
				Passo: "dados: " + fonte.Nome(), Chave: fonte.Nome(), Protocolo: "dados",
				InstanteAgendado: agendado, InstanteDeEnvio: agendado, InstanteDeTermino: m.opcoes.Relogio.Agora(),
				Classe: protocol.ErroDeConfigacao, Detalhe: err.Error(),
			})
			return
		}
		valores.DefinirVarios(registro)
	}

	var cabecalhoDeAutenticacao [2]string
	if m.autenticador != nil {
		nome, valor, err := m.autenticador.Cabecalho(ctx, valores)
		if err != nil {
			coletor.Registrar(metrics.Amostra{
				Passo: "autenticacao", Chave: "autenticacao", Protocolo: "http",
				InstanteAgendado: agendado, InstanteDeEnvio: agendado, InstanteDeTermino: m.opcoes.Relogio.Agora(),
				Classe: protocol.ErroDeAutenticacao, Detalhe: fmt.Sprintf("%v — alvo %s", err, m.cenario.Alvo),
			})
			return
		}
		cabecalhoDeAutenticacao = [2]string{nome, valor}
	}

	instanteDoPasso := agendado
	completa := true
	for indice, passo := range m.cenario.Passos {
		amostra, _ := m.executarPasso(ctx, passo, instanteDoPasso, valores, cabecalhoDeAutenticacao)
		if indice == 0 {
			amostra.TipoDeLatencia = metrics.LatenciaCorrigida
		} else {
			amostra.TipoDeLatencia = metrics.LatenciaDeServico
		}
		coletor.Registrar(amostra)
		instanteDoPasso = amostra.InstanteDeTermino
		if amostra.Classe != protocol.Sucesso {
			completa = false
			break
		}
	}
	coletor.RegistrarJornada(agendado, instanteDoPasso, completa)
	coletor.RegistrarUsos(valores.Usos())
}

func (m *Motor) executarPasso(ctx context.Context, passo scenario.Passo, agendado time.Time,
	valores *runtime.Contexto, cabecalhoDeAutenticacao [2]string) (metrics.Amostra, Observacao) {

	relogio := m.opcoes.Relogio
	observacao := Observacao{Passo: passo.Nome, Chave: passo.ChaveDeAgregacao(), Capturado: map[string]string{}}
	amostra := metrics.Amostra{
		Passo: passo.Nome, Chave: passo.ChaveDeAgregacao(), Protocolo: passo.Protocolo,
		InstanteAgendado: agendado,
	}

	implementacao, existe := protocol.Buscar(passo.Protocolo)
	if !existe {
		amostra.InstanteDeEnvio = relogio.Agora()
		amostra.InstanteDeTermino = amostra.InstanteDeEnvio
		amostra.Classe = protocol.ErroDeConfigacao
		amostra.Detalhe = "protocolo nao compilado neste binario"
		return amostra, observacao
	}

	configuracao := passo.Configuracao.Resolver(valores.Resolver)
	if cabecalhoDeAutenticacao[0] != "" {
		if comCabecalhos, aceita := configuracao.(protocol.ConfiguracaoComCabecalhos); aceita {
			configuracao = comCabecalhos.ComCabecalho(cabecalhoDeAutenticacao[0], cabecalhoDeAutenticacao[1])
		}
	}
	observacao.Configuracao = configuracao

	amostra.InstanteDeEnvio = relogio.Agora()
	resposta := implementacao.Executar(ctx, protocol.Requisicao{
		NomeDoPasso:  passo.Nome,
		Configuracao: configuracao,
		URLBase:      m.cenario.Alvo,
		Variaveis:    valores.Valores(),
	})
	amostra.InstanteDeTermino = relogio.Agora()
	amostra.Status = resposta.Status
	amostra.Bytes = resposta.Bytes
	amostra.Classe = resposta.Classe
	amostra.Detalhe = resposta.Detalhe
	observacao.Resposta = resposta
	observacao.Duracao = amostra.InstanteDeTermino.Sub(amostra.InstanteDeEnvio)

	if coletor := m.coletor.Load(); coletor != nil && len(resposta.Atributos) > 0 {
		coletor.RegistrarUsos(resposta.Atributos)
	}

	if resposta.Chave != "" {
		amostra.Chave = resposta.Chave
		observacao.Chave = resposta.Chave
	}

	if amostra.Classe == protocol.Sucesso {
		if classe, detalhe := m.verificar(passo, resposta, valores); classe != protocol.Sucesso {
			amostra.Classe = classe
			amostra.Detalhe = detalhe
			observacao.Falhas = append(observacao.Falhas, detalhe)
		}
	}

	if amostra.Classe == protocol.Sucesso {
		for _, captura := range passo.Capturas {
			valor, err := correlation.Extrair(captura, resposta)
			if err != nil {
				if captura.Obrigatoria {
					amostra.Classe = protocol.ErroDeCorrelacao
					amostra.Detalhe = err.Error()
					observacao.Falhas = append(observacao.Falhas, err.Error())
					break
				}
				valor = captura.Padrao
			}
			valores.Definir(captura.Variavel, valor)
			observacao.Capturado[captura.Variavel] = valor
		}
	}

	observacao.Classe = amostra.Classe
	observacao.Variaveis = valores.Valores()
	return amostra, observacao
}

func (m *Motor) verificar(passo scenario.Passo, resposta protocol.Resposta, valores *runtime.Contexto) (protocol.ClasseDeErro, string) {
	for _, verificacao := range passo.Verificacoes {
		switch verificacao.Tipo {
		case scenario.VerificarStatus:
			if resposta.Status != verificacao.Status {
				return protocol.ErroDeStatus, fmt.Sprintf("esperava status %d, recebeu %d", verificacao.Status, resposta.Status)
			}
		case scenario.VerificarCorpo:
			if !bytes.Contains(resposta.Corpo, []byte(verificacao.Texto)) {
				return protocol.ErroDeAssercao, fmt.Sprintf("o corpo nao contem %q", verificacao.Texto)
			}
		}
	}
	for _, assercao := range passo.Assercoes {
		if err := correlation.Avaliar(assercao, resposta, valores.Resolver); err != nil {
			return protocol.ErroDeAssercao, err.Error()
		}
	}
	return protocol.Sucesso, ""
}

func (m *Motor) executarPassoDeAutenticacao(ctx context.Context, passo scenario.Passo, valores *runtime.Contexto) (protocol.Resposta, error) {
	amostra, observacao := m.executarPasso(ctx, passo, m.opcoes.Relogio.Agora(), valores, [2]string{})
	if amostra.Classe != protocol.Sucesso && amostra.Classe != protocol.ErroDeStatus {
		return observacao.Resposta, fmt.Errorf("%s", amostra.Detalhe)
	}
	return observacao.Resposta, nil
}

func (m *Motor) acompanhar(coletor *metrics.Coletor, inicio time.Time, parar <-chan struct{}) {
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

func (m *Motor) fasesAplicadas() []metrics.FaseAplicada {
	fases := make([]metrics.FaseAplicada, 0, len(m.cenario.Carga.Fases))
	for _, fase := range m.cenario.Carga.Fases {
		fases = append(fases, metrics.FaseAplicada{
			Tipo:      string(fase.Tipo),
			De:        fase.TaxaInicial(),
			Ate:       fase.TaxaFinal(),
			DuracaoMs: fase.Durante.Milliseconds(),
		})
	}
	return fases
}

func (m *Motor) prepararProtocolos(ctx context.Context) error {
	valores := runtime.Novo(0, 0, m.cenario.Variaveis)
	for _, passo := range m.cenario.Passos {
		implementacao, existe := protocol.Buscar(passo.Protocolo)
		if !existe {
			continue
		}
		preparador, precisa := implementacao.(protocol.ProtocoloComPreparacao)
		if !precisa {
			continue
		}
		erro := preparador.Preparar(ctx, protocol.Requisicao{
			NomeDoPasso:  passo.Nome,
			Configuracao: passo.Configuracao.Resolver(valores.Resolver),
			URLBase:      m.cenario.Alvo,
		})
		if erro != nil {
			return fmt.Errorf("nao consegui preparar o passo %q: %w", passo.Nome, erro)
		}
	}
	return nil
}

func (m *Motor) disponibilidade() metrics.Disponibilidade {
	disponibilidade := metrics.Disponibilidade{}
	for _, fonte := range m.fontes {
		for nome, quantos := range fonte.Disponiveis() {
			disponibilidade[nome] = quantos
		}
	}
	for _, nome := range protocol.Registrados() {
		implementacao, _ := protocol.Buscar(nome)
		if sabe, tem := implementacao.(protocol.ProtocoloComDisponibilidade); tem {
			for chave, quantos := range sabe.Disponiveis() {
				disponibilidade[chave] = quantos
			}
		}
	}
	// O token e um so por execucao por decisao declarada (ADR 0005), entao um
	// valor unico aqui nao e defeito: a limitacao ja aparece no bloco de
	// ambiente, e repetir como aviso grave abafaria os avisos que importam.
	if m.cenario.Autenticacao != nil && m.cenario.Autenticacao.Obter != nil {
		for _, captura := range m.cenario.Autenticacao.Obter.Capturas {
			disponibilidade[captura.Variavel] = 1
		}
	}
	return disponibilidade
}

// Semente so existe para fonte sintetica: anotar semente de um CSV sugeriria
// que o arquivo foi sorteado, e a frase do relatorio sobre variedade e a
// variedade observada ([ADR 0007]), nunca a semente declarada.
func (m *Motor) sementes() map[string]int64 {
	sementes := map[string]int64{}
	for _, fonte := range m.cenario.Dados {
		if !fonte.Sintetica() {
			continue
		}
		semente := fonte.Semente
		if semente == 0 {
			semente = 1
		}
		sementes[fonte.Nome] = semente
	}
	return sementes
}

func (m *Motor) obtencoesDeAutenticacao() int64 {
	if m.autenticador == nil {
		return 0
	}
	return m.autenticador.Obtencoes
}
