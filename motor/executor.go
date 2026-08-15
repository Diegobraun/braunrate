package motor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Diegobraun/braunrate/autenticacao"
	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/contexto"
	"github.com/Diegobraun/braunrate/correlacao"
	"github.com/Diegobraun/braunrate/dados"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/protocolo"
)

type Opcoes struct {
	Versao               string
	MaximoSimultaneas    int64
	LimiarDeAtraso       time.Duration
	Relogio              Relogio
	RaizDeDados          string
	AoProgredir          func(metrica.Instantaneo, float64, time.Duration)
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
	Configuracao protocolo.Configuracao
	Resposta     protocolo.Resposta
	Capturado    map[string]string
	Variaveis    map[string]string
	Falhas       []string
	Classe       protocolo.ClasseDeErro
	Duracao      time.Duration
}

type Motor struct {
	cenario      cenario.Cenario
	plano        Plano
	opcoes       Opcoes
	fontes       []dados.Fonte
	autenticador *autenticacao.Gerenciador
	coletor      atomic.Pointer[metrica.Coletor]
}

func Novo(c cenario.Cenario, opcoes Opcoes) (*Motor, error) {
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
		aberta, err := dados.Abrir(fonte, opcoes.RaizDeDados)
		if err != nil {
			return nil, err
		}
		m.fontes = append(m.fontes, aberta)
	}

	if c.Autenticacao != nil {
		m.autenticador = autenticacao.Novo(*c.Autenticacao, m.executarPassoDeAutenticacao, opcoes.Relogio)
	}
	return m, nil
}

func (m *Motor) Plano() Plano { return m.plano }

// Depurar roda uma unica iteracao pelo mesmo caminho da carga: mesmo motor,
// mesma resolucao de variavel, mesma captura. So a carga muda.
func (m *Motor) Depurar(ctx context.Context) ([]Observacao, map[string]string, error) {
	valores := contexto.Novo(0, 0, m.cenario.Variaveis)

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
		if amostra.Classe != protocolo.Sucesso {
			break
		}
	}
	return observacoes, valores.Valores(), nil
}

func (m *Motor) Cenario() cenario.Cenario { return m.cenario }

func (m *Motor) RaizDeDados() string { return filepath.Clean(m.opcoes.RaizDeDados) }

func (m *Motor) Executar(ctx context.Context) metrica.Documento {
	relogio := m.opcoes.Relogio
	inicio := relogio.Agora()
	if err := m.prepararProtocolos(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	coletor := metrica.NovoColetor(inicio, m.opcoes.LimiarDeAtraso)
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

	return metrica.MontarDocumento(coletor, metrica.EntradaDoDocumento{
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
	})
}

// Cada chegada agendada e uma iteracao inteira do cenario: e o que faz o valor
// capturado num passo chegar ao passo seguinte. Se um passo falha, a iteracao
// para — os passos seguintes dependeriam de uma captura que nao aconteceu.
func (m *Motor) executarIteracao(ctx context.Context, usuarioVirtual int64, agendado time.Time, coletor *metrica.Coletor) {
	valores := contexto.Novo(usuarioVirtual, usuarioVirtual, m.cenario.Variaveis)

	for _, fonte := range m.fontes {
		registro, err := fonte.Proximo(usuarioVirtual)
		if err != nil {
			coletor.Registrar(metrica.Amostra{
				Passo: "dados: " + fonte.Nome(), Chave: fonte.Nome(), Protocolo: "dados",
				InstanteAgendado: agendado, InstanteDeEnvio: agendado, InstanteDeTermino: m.opcoes.Relogio.Agora(),
				Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error(),
			})
			return
		}
		valores.DefinirVarios(registro)
	}

	var cabecalhoDeAutenticacao [2]string
	if m.autenticador != nil {
		nome, valor, err := m.autenticador.Cabecalho(ctx, valores)
		if err != nil {
			coletor.Registrar(metrica.Amostra{
				Passo: "autenticacao", Chave: "autenticacao", Protocolo: "http",
				InstanteAgendado: agendado, InstanteDeEnvio: agendado, InstanteDeTermino: m.opcoes.Relogio.Agora(),
				Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error(),
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
			amostra.TipoDeLatencia = metrica.LatenciaCorrigida
		} else {
			amostra.TipoDeLatencia = metrica.LatenciaDeServico
		}
		coletor.Registrar(amostra)
		instanteDoPasso = amostra.InstanteDeTermino
		if amostra.Classe != protocolo.Sucesso {
			completa = false
			break
		}
	}
	coletor.RegistrarJornada(agendado, instanteDoPasso, completa)
	coletor.RegistrarUsos(valores.Usos())
}

func (m *Motor) executarPasso(ctx context.Context, passo cenario.Passo, agendado time.Time,
	valores *contexto.Contexto, cabecalhoDeAutenticacao [2]string) (metrica.Amostra, Observacao) {

	relogio := m.opcoes.Relogio
	observacao := Observacao{Passo: passo.Nome, Chave: passo.ChaveDeAgregacao(), Capturado: map[string]string{}}
	amostra := metrica.Amostra{
		Passo: passo.Nome, Chave: passo.ChaveDeAgregacao(), Protocolo: passo.Protocolo,
		InstanteAgendado: agendado,
	}

	implementacao, existe := protocolo.Buscar(passo.Protocolo)
	if !existe {
		amostra.InstanteDeEnvio = relogio.Agora()
		amostra.InstanteDeTermino = amostra.InstanteDeEnvio
		amostra.Classe = protocolo.ErroDeConfigacao
		amostra.Detalhe = "protocolo nao compilado neste binario"
		return amostra, observacao
	}

	configuracao := passo.Configuracao.Resolver(valores.Resolver)
	if cabecalhoDeAutenticacao[0] != "" {
		if comCabecalhos, aceita := configuracao.(protocolo.ConfiguracaoComCabecalhos); aceita {
			configuracao = comCabecalhos.ComCabecalho(cabecalhoDeAutenticacao[0], cabecalhoDeAutenticacao[1])
		}
	}
	observacao.Configuracao = configuracao

	amostra.InstanteDeEnvio = relogio.Agora()
	resposta := implementacao.Executar(ctx, protocolo.Requisicao{
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

	if amostra.Classe == protocolo.Sucesso {
		if classe, detalhe := m.verificar(passo, resposta, valores); classe != protocolo.Sucesso {
			amostra.Classe = classe
			amostra.Detalhe = detalhe
			observacao.Falhas = append(observacao.Falhas, detalhe)
		}
	}

	if amostra.Classe == protocolo.Sucesso {
		for _, captura := range passo.Capturas {
			valor, err := correlacao.Extrair(captura, resposta)
			if err != nil {
				if captura.Obrigatoria {
					amostra.Classe = protocolo.ErroDeCorrelacao
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

func (m *Motor) verificar(passo cenario.Passo, resposta protocolo.Resposta, valores *contexto.Contexto) (protocolo.ClasseDeErro, string) {
	for _, verificacao := range passo.Verificacoes {
		switch verificacao.Tipo {
		case cenario.VerificarStatus:
			if resposta.Status != verificacao.Status {
				return protocolo.ErroDeStatus, fmt.Sprintf("esperava status %d, recebeu %d", verificacao.Status, resposta.Status)
			}
		case cenario.VerificarCorpo:
			if !bytes.Contains(resposta.Corpo, []byte(verificacao.Texto)) {
				return protocolo.ErroDeAssercao, fmt.Sprintf("o corpo nao contem %q", verificacao.Texto)
			}
		}
	}
	for _, assercao := range passo.Assercoes {
		if err := correlacao.Avaliar(assercao, resposta, valores.Resolver); err != nil {
			return protocolo.ErroDeAssercao, err.Error()
		}
	}
	return protocolo.Sucesso, ""
}

func (m *Motor) executarPassoDeAutenticacao(ctx context.Context, passo cenario.Passo, valores *contexto.Contexto) (protocolo.Resposta, error) {
	amostra, observacao := m.executarPasso(ctx, passo, m.opcoes.Relogio.Agora(), valores, [2]string{})
	if amostra.Classe != protocolo.Sucesso && amostra.Classe != protocolo.ErroDeStatus {
		return observacao.Resposta, fmt.Errorf("%s", amostra.Detalhe)
	}
	return observacao.Resposta, nil
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

func (m *Motor) prepararProtocolos(ctx context.Context) error {
	valores := contexto.Novo(0, 0, m.cenario.Variaveis)
	for _, passo := range m.cenario.Passos {
		implementacao, existe := protocolo.Buscar(passo.Protocolo)
		if !existe {
			continue
		}
		preparador, precisa := implementacao.(protocolo.ProtocoloComPreparacao)
		if !precisa {
			continue
		}
		erro := preparador.Preparar(ctx, protocolo.Requisicao{
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

func (m *Motor) disponibilidade() metrica.Disponibilidade {
	disponibilidade := metrica.Disponibilidade{}
	for _, fonte := range m.fontes {
		for nome, quantos := range fonte.Disponiveis() {
			disponibilidade[nome] = quantos
		}
	}
	for _, nome := range protocolo.Registrados() {
		implementacao, _ := protocolo.Buscar(nome)
		if sabe, tem := implementacao.(protocolo.ProtocoloComDisponibilidade); tem {
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
