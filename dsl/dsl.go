// A DSL monta a mesma estrutura que o YAML monta e entrega ao mesmo motor. Por
// isso ela nao interpreta expressao propria: captura, comparacao e limite de SLO
// passam pelas funcoes de cenario que o YAML usa, e nenhum padrao e reescrito
// aqui. Duas interpretacoes da mesma linha viraria numero diferente entre os
// dois publicos, que e exatamente o que a ferramenta promete nao fazer.
package dsl

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Taxa float64

func PorSegundo(valor float64) Taxa { return Taxa(valor) }
func PorMinuto(valor float64) Taxa  { return Taxa(valor / 60) }
func PorHora(valor float64) Taxa    { return Taxa(valor / 3600) }

type Requisicao interface {
	montar() (string, protocol.Configuracao, error)
}

type OpcaoDePasso func(*scenario.Passo) error

type Construtor struct {
	cenario scenario.Cenario
	erros   []error
}

func Novo(nome string) *Construtor {
	return &Construtor{cenario: scenario.Cenario{
		VersaoDoFormato: scenario.VersaoDoFormato,
		Nome:            nome,
		Variaveis:       map[string]string{},
		Carga:           scenario.PlanoDeCarga{Modelo: scenario.ChegadaAberta},
	}}
}

func (c *Construtor) anotar(err error) {
	if err != nil {
		c.erros = append(c.erros, err)
	}
}

func (c *Construtor) Alvo(alvo string) *Construtor {
	c.cenario.Alvo = alvo
	return c
}

func (c *Construtor) Variavel(nome, valor string) *Construtor {
	c.cenario.Variaveis[nome] = scenario.ExpandirDoAmbiente(valor)
	return c
}

type OpcaoDeDados func(*scenario.FonteDeDados)

func Consumo(politica scenario.PoliticaDeConsumo) OpcaoDeDados {
	return func(fonte *scenario.FonteDeDados) { fonte.Consumo = politica }
}

func Semente(semente int64) OpcaoDeDados {
	return func(fonte *scenario.FonteDeDados) { fonte.Semente = semente }
}

func (c *Construtor) DadosDeArquivo(nome, arquivo string, opcoes ...OpcaoDeDados) *Construtor {
	fonte := scenario.FonteDeDados{Nome: nome, Arquivo: arquivo, Consumo: scenario.ConsumoCircular}
	for _, opcao := range opcoes {
		opcao(&fonte)
	}
	c.cenario.Dados = append(c.cenario.Dados, fonte)
	return c
}

func (c *Construtor) DadosGerados(nome string, campos map[string]string, opcoes ...OpcaoDeDados) *Construtor {
	fonte := scenario.FonteDeDados{Nome: nome, Consumo: scenario.ConsumoCircular, Campos: map[string]string{}}
	for campo, receita := range campos {
		fonte.Campos[campo] = receita
	}
	for _, opcao := range opcoes {
		opcao(&fonte)
	}
	if len(fonte.Campos) == 0 {
		c.anotar(fmt.Errorf("a fonte de dados %q precisa de pelo menos um campo em 'gerar'", nome))
	}
	c.cenario.Dados = append(c.cenario.Dados, fonte)
	return c
}

func (c *Construtor) Rampa(de, ate Taxa, durante time.Duration) *Construtor {
	return c.fase(scenario.Fase{Tipo: scenario.FaseRampa, De: float64(de), Ate: float64(ate), Durante: durante})
}

func (c *Construtor) Patamar(taxa Taxa, durante time.Duration) *Construtor {
	return c.fase(scenario.Fase{Tipo: scenario.FasePatamar, Ate: float64(taxa), Durante: durante})
}

func (c *Construtor) Pico(taxa Taxa, durante time.Duration) *Construtor {
	return c.fase(scenario.Fase{Tipo: scenario.FasePico, Ate: float64(taxa), Durante: durante})
}

func (c *Construtor) Constante(taxa Taxa, durante time.Duration) *Construtor {
	return c.fase(scenario.Fase{Tipo: scenario.FaseConstante, Ate: float64(taxa), Durante: durante})
}

func (c *Construtor) fase(fase scenario.Fase) *Construtor {
	c.cenario.Carga.Fases = append(c.cenario.Carga.Fases, fase)
	return c
}

func (c *Construtor) Passo(requisicao Requisicao, opcoes ...OpcaoDePasso) *Construtor {
	passo, err := montarPasso(requisicao, opcoes...)
	if err != nil {
		c.anotar(err)
		return c
	}
	c.cenario.Passos = append(c.cenario.Passos, passo)
	return c
}

func montarPasso(requisicao Requisicao, opcoes ...OpcaoDePasso) (scenario.Passo, error) {
	if requisicao == nil {
		return scenario.Passo{}, errors.New("passo sem requisicao")
	}
	nomeDoProtocolo, configuracao, err := requisicao.montar()
	if err != nil {
		return scenario.Passo{}, err
	}
	passo := scenario.Passo{Protocolo: nomeDoProtocolo, Configuracao: configuracao}
	for _, opcao := range opcoes {
		if err := opcao(&passo); err != nil {
			return scenario.Passo{}, err
		}
	}
	if passo.Nome == "" {
		passo.Nome = configuracao.ChaveDeAgregacao()
	}
	return passo, nil
}

func Nome(nome string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		passo.Nome = nome
		return nil
	}
}

// A expressao e a mesma do YAML: "$.fatura.id", "cabecalho:X-Id" ou "/regex/".
func Capturar(variavel, expressao string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		captura, err := scenario.MontarCaptura(variavel, expressao)
		if err != nil {
			return err
		}
		passo.Capturas = append(passo.Capturas, captura)
		return nil
	}
}

func CapturarComPadrao(variavel, expressao, padrao string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		captura, err := scenario.MontarCaptura(variavel, expressao)
		if err != nil {
			return err
		}
		captura.Padrao = padrao
		captura.Obrigatoria = false
		passo.Capturas = append(passo.Capturas, captura)
		return nil
	}
}

func VerificarStatus(status int) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		passo.Verificacoes = append(passo.Verificacoes, scenario.Verificacao{Tipo: scenario.VerificarStatus, Status: status})
		return nil
	}
}

func VerificarCorpoContem(trecho string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		passo.Assercoes = append(passo.Assercoes, scenario.Assercao{Tipo: scenario.AsserirCorpoContem, Valor: trecho})
		return nil
	}
}

func VerificarCorpoCasa(padrao string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		passo.Assercoes = append(passo.Assercoes, scenario.Assercao{Tipo: scenario.AsserirRegex, Valor: padrao})
		return nil
	}
}

// A comparacao aceita a mesma escrita do YAML: "PAGA", "> 10", "existe",
// "contem parcial".
func VerificarJSON(caminho, comparacao string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		assercao := scenario.MontarComparacao(caminho, comparacao)
		assercao.Tipo = scenario.AsserirJSON
		passo.Assercoes = append(passo.Assercoes, assercao)
		return nil
	}
}

func VerificarCabecalho(nome, valor string) OpcaoDePasso {
	return func(passo *scenario.Passo) error {
		passo.Assercoes = append(passo.Assercoes, scenario.Assercao{
			Tipo: scenario.AsserirCabecalho, Alvo: nome, Operador: scenario.OperadorIgual, Valor: valor,
		})
		return nil
	}
}

func (c *Construtor) SLO(passo, metrica, limite string) *Construtor {
	regra, err := scenario.MontarRegraDeSLO(passo, metrica, limite)
	if err != nil {
		c.anotar(err)
		return c
	}
	c.cenario.SLO = append(c.cenario.SLO, regra)
	return c
}

func (c *Construtor) SLOGlobal(metrica, limite string) *Construtor {
	return c.SLO("global", metrica, limite)
}

type Autenticador struct {
	autenticacao scenario.Autenticacao
	erro         error
}

func PorToken(requisicao Requisicao, opcoes ...OpcaoDePasso) *Autenticador {
	passo, err := montarPasso(requisicao, opcoes...)
	if err != nil {
		return &Autenticador{erro: err}
	}
	passo.Nome = "obter autenticacao"
	return &Autenticador{autenticacao: scenario.Autenticacao{Tipo: scenario.AutenticacaoPorToken, Obter: &passo}}
}

func Basica(usuario, senha string) *Autenticador {
	return &Autenticador{autenticacao: scenario.Autenticacao{
		Tipo: scenario.AutenticacaoBasica, Usuario: usuario, Senha: senha,
	}}
}

func PorCabecalho(cabecalho string) *Autenticador {
	return &Autenticador{autenticacao: scenario.Autenticacao{
		Tipo: scenario.AutenticacaoCabecalho, Cabecalho: cabecalho,
	}}
}

func (a *Autenticador) RenovarApos(intervalo time.Duration) *Autenticador {
	a.autenticacao.RenovarApos = intervalo
	return a
}

func (a *Autenticador) Cabecalho(cabecalho string) *Autenticador {
	a.autenticacao.Cabecalho = cabecalho
	return a
}

func (c *Construtor) Autenticacao(autenticador *Autenticador) *Construtor {
	if autenticador.erro != nil {
		c.anotar(autenticador.erro)
		return c
	}
	autenticacao := autenticador.autenticacao
	if autenticacao.Tipo == scenario.AutenticacaoPorToken && autenticacao.Obter == nil {
		c.anotar(errors.New("autenticacao por token precisa da requisicao que devolve o token"))
		return c
	}
	if autenticacao.Tipo == scenario.AutenticacaoBasica && (autenticacao.Usuario == "" || autenticacao.Senha == "") {
		c.anotar(errors.New("autenticacao basica precisa de usuario e senha"))
		return c
	}
	if autenticacao.Cabecalho == "" && autenticacao.Tipo != scenario.AutenticacaoBasica {
		autenticacao.Cabecalho = "Authorization: Bearer ${token}"
	}
	c.cenario.Autenticacao = &autenticacao
	return c
}

func (c *Construtor) Construir() (scenario.Cenario, error) {
	montado := c.cenario
	montado.Alvo = scenario.Interpolar(montado.Alvo, montado.Variaveis)
	if len(c.erros) > 0 {
		return montado, fmt.Errorf("cenario invalido:\n  - %s", strings.Join(mensagens(c.erros), "\n  - "))
	}
	if err := montado.Validar(); err != nil {
		return montado, err
	}
	return montado, nil
}

func mensagens(erros []error) []string {
	textos := make([]string, 0, len(erros))
	for _, err := range erros {
		textos = append(textos, err.Error())
	}
	return textos
}
