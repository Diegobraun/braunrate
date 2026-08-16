package wait

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"gopkg.in/yaml.v3"
)

const timeoutPadrao = 30 * time.Second

func init() {
	protocol.Registrar(Novo(protocol.OpcoesPadrao()))
}

type Configuracao struct {
	Fonte     string
	Topico    string
	Enderecos []string
	Esperado  string
	Campo     string
	Timeout   time.Duration

	Caminho   string
	Ate       Condicao
	Intervalo time.Duration
}

func (c *Configuracao) Protocolo() string { return "aguardar" }

func (c *Configuracao) ChaveDeAgregacao() string {
	if c.Fonte == "http" {
		return "aguardar " + c.Caminho
	}
	return "aguardar " + c.Topico
}

func (c *Configuracao) Resolver(resolver func(string) string) protocol.Configuracao {
	copia := *c
	copia.Topico = resolver(c.Topico)
	copia.Esperado = resolver(c.Esperado)
	copia.Campo = resolver(c.Campo)
	copia.Caminho = resolver(c.Caminho)
	copia.Ate.Valor = resolver(c.Ate.Valor)
	copia.Ate.CorpoContem = resolver(c.Ate.CorpoContem)
	return &copia
}

func (c *Configuracao) Descrever() []string {
	if c.Fonte == "http" {
		intervalo := c.Intervalo
		if intervalo <= 0 {
			intervalo = intervaloPadrao
		}
		return []string{
			fmt.Sprintf("aguardar em GET %s ate %s", c.Caminho, c.Ate.descrever()),
			fmt.Sprintf("sondando a cada %s, desiste depois de %s", intervalo, c.Timeout),
			"a latencia medida tem a granularidade da sondagem",
		}
	}
	onde := "chave da mensagem"
	if c.Campo != "" {
		onde = c.Campo
	}
	linhas := []string{
		fmt.Sprintf("aguardar em %s %s por %s = %q", c.Fonte, c.Topico, onde, c.Esperado),
		"desiste depois de " + c.Timeout.String(),
	}
	if len(c.Enderecos) > 0 {
		linhas = append(linhas, "enderecos: "+strings.Join(c.Enderecos, ", "))
	}
	return linhas
}

type Protocolo struct {
	mutex       sync.Mutex
	assinaturas map[string]*assinatura
	http        *nethttp.Client
}

func Novo(protocol.Opcoes) *Protocolo {
	return &Protocolo{assinaturas: map[string]*assinatura{}}
}

func (p *Protocolo) Nome() string { return "aguardar" }

func (p *Protocolo) Encerrar() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for chave, assinada := range p.assinaturas {
		assinada.encerrar()
		delete(p.assinaturas, chave)
	}
	return nil
}

func (p *Protocolo) Decodificar(no *yaml.Node) (protocol.Configuracao, error) {
	if no == nil || no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo aguardar precisa ser um mapa, por exemplo:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"
      timeout: 30s`)
	}

	configuracao := Padrao()
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "kafka", "amqp":
			configuracao.Fonte = chave.Value
			if err := lerFonte(configuracao, valor); err != nil {
				return nil, err
			}
		case "http":
			configuracao.Fonte = "http"
			if err := lerFonteHTTP(configuracao, valor); err != nil {
				return nil, err
			}
		case "ate":
			if valor.Kind != yaml.MappingNode {
				return nil, errors.New(`ate precisa ser um mapa, por exemplo:
      ate: { $.status: PROCESSADO }
      ate: { status: 200 }`)
			}
			bruto := map[string]string{}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				bruto[valor.Content[i].Value] = valor.Content[i+1].Value
			}
			condicao, err := lerCondicao("ate", bruto)
			if err != nil {
				return nil, err
			}
			configuracao.Ate = condicao
		case "intervalo":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, fmt.Errorf("intervalo invalido: %q (use 200ms, 1s)", valor.Value)
			}
			configuracao.Intervalo = duracao
		case "chave":
			configuracao.Esperado = valor.Value
		case "campo":
			configuracao.Campo = valor.Value
		case "igual_a":
			configuracao.Esperado = valor.Value
		case "timeout":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 30s, 2m)", valor.Value)
			}
			configuracao.Timeout = duracao
		default:
			return nil, fmt.Errorf("chave desconhecida no passo aguardar: %q (use kafka, amqp, http, chave, campo, igual_a, ate, intervalo ou timeout)", chave.Value)
		}
	}

	if err := Validar(configuracao); err != nil {
		return nil, err
	}
	return configuracao, nil
}

func Padrao() *Configuracao {
	return &Configuracao{Timeout: timeoutPadrao}
}

// A correlacao obrigatoria vale igual na DSL: sem ela a medicao pegaria a
// primeira mensagem que aparecesse e mediria o consumidor mais rapido.
func Validar(configuracao *Configuracao) error {
	if configuracao.Fonte == "http" {
		return validarHTTP(configuracao)
	}
	if configuracao.Fonte == "" {
		return errors.New(`o passo aguardar precisa dizer onde esperar, por exemplo:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"`)
	}
	if configuracao.Esperado == "" {
		return errors.New(`o passo aguardar precisa do valor que identifica a mensagem desta iteracao.
Sem isso, qualquer mensagem serviria e a medicao mediria o consumidor mais rapido, nao a cadeia:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"`)
	}
	return nil
}

// Sondar sem condicao mediria a primeira resposta, e nao o efeito: o passo
// terminaria antes de o sistema fazer o que tinha de fazer.
func validarHTTP(configuracao *Configuracao) error {
	if configuracao.Caminho == "" {
		return errors.New(`o passo aguardar por http precisa do caminho, por exemplo:
  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }`)
	}
	if configuracao.Ate.vazia() {
		return errors.New(`o passo aguardar por http precisa de 'ate': sem condicao, a primeira resposta encerraria a espera
e a medicao seria do tempo de responder, nao do tempo ate o efeito acontecer:
  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }`)
	}
	return nil
}

func lerFonteHTTP(configuracao *Configuracao, no *yaml.Node) error {
	if no.Kind == yaml.ScalarNode {
		configuracao.Caminho = no.Value
		return nil
	}
	if no.Kind != yaml.MappingNode {
		return errors.New("aguardar.http precisa ser o caminho ou um mapa com 'caminho'")
	}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "caminho", "url":
			configuracao.Caminho = valor.Value
		default:
			return fmt.Errorf("chave desconhecida em aguardar.http: %q (use caminho)", chave.Value)
		}
	}
	return nil
}

func lerFonte(configuracao *Configuracao, no *yaml.Node) error {
	if no.Kind == yaml.ScalarNode {
		configuracao.Topico = no.Value
		return nil
	}
	if no.Kind != yaml.MappingNode {
		return fmt.Errorf("a fonte %q precisa ser o nome do topico ou um mapa", configuracao.Fonte)
	}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "topico", "fila":
			configuracao.Topico = valor.Value
		case "brokers", "url", "enderecos":
			if valor.Kind == yaml.ScalarNode {
				configuracao.Enderecos = strings.Split(valor.Value, ",")
				continue
			}
			for _, item := range valor.Content {
				configuracao.Enderecos = append(configuracao.Enderecos, item.Value)
			}
		default:
			return fmt.Errorf("chave desconhecida em aguardar.%s: %q (use topico ou brokers)", configuracao.Fonte, chave.Value)
		}
	}
	if configuracao.Topico == "" {
		return fmt.Errorf("aguardar.%s sem topico", configuracao.Fonte)
	}
	return nil
}

// A assinatura abre antes da carga: o offset de leitura e fixado agora, e nao
// depois que a primeira mensagem ja foi produzida.
func (p *Protocolo) Preparar(_ context.Context, requisicao protocol.Requisicao) error {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok || configuracao.Fonte == "http" {
		return nil
	}
	_, err := p.assinar(configuracao, requisicao.URLBase)
	return err
}

func (p *Protocolo) Executar(ctx context.Context, requisicao protocol.Requisicao) protocol.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocol.Resposta{Classe: protocol.ErroDeConfigacao, Detalhe: "configuracao nao e de aguardar"}
	}

	if configuracao.Fonte == "http" {
		return p.esperarPorHTTP(ctx, requisicao, configuracao)
	}

	assinada, err := p.assinar(configuracao, requisicao.URLBase)
	if err != nil {
		return protocol.Resposta{Classe: protocol.ErroDeConfigacao, Detalhe: err.Error()}
	}

	timeout := configuracao.Timeout
	if timeout <= 0 {
		timeout = timeoutPadrao
	}

	mensagem, chegou := assinada.esperar(ctx, configuracao.Esperado, timeout)
	if !chegou {
		return protocol.Resposta{
			Classe: protocol.ErroDeTimeout,
			Detalhe: fmt.Sprintf("a mensagem com %s=%q nao chegou em %s no topico %s",
				ondeProcura(configuracao), configuracao.Esperado, timeout, configuracao.Topico),
		}
	}

	return protocol.Resposta{
		Corpo:     mensagem.corpo,
		Bytes:     int64(len(mensagem.corpo)),
		Classe:    protocol.Sucesso,
		Atributos: mensagem.atributos,
	}
}

func ondeProcura(configuracao *Configuracao) string {
	if configuracao.Campo != "" {
		return configuracao.Campo
	}
	return "chave"
}

func (p *Protocolo) assinar(configuracao *Configuracao, alvo string) (*assinatura, error) {
	enderecos := configuracao.Enderecos
	if len(enderecos) == 0 {
		enderecos = enderecosDoAlvo(alvo)
	}
	if len(enderecos) == 0 {
		return nil, fmt.Errorf("aguardar em %s sem endereco: declare 'brokers' (kafka) ou 'url' (amqp) no passo, ou aponte o alvo do cenario para o broker", configuracao.Fonte)
	}

	chave := configuracao.Fonte + "|" + strings.Join(enderecos, ",") + "|" + configuracao.Topico

	p.mutex.Lock()
	defer p.mutex.Unlock()
	if existente, tem := p.assinaturas[chave]; tem {
		return existente, nil
	}

	nova, err := abrirAssinatura(configuracao, enderecos)
	if err != nil {
		return nil, err
	}
	p.assinaturas[chave] = nova
	return nova, nil
}

func enderecosDoAlvo(alvo string) []string {
	if alvo == "" || strings.HasPrefix(alvo, "http://") || strings.HasPrefix(alvo, "https://") {
		return nil
	}
	return strings.Split(strings.TrimSuffix(alvo, "/"), ",")
}
