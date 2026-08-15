package aguardar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"gopkg.in/yaml.v3"
)

const timeoutPadrao = 30 * time.Second

func init() {
	protocolo.Registrar(Novo(protocolo.OpcoesPadrao()))
}

type Configuracao struct {
	Fonte     string
	Topico    string
	Enderecos []string
	Esperado  string
	Campo     string
	Timeout   time.Duration
}

func (c *Configuracao) Protocolo() string { return "aguardar" }

func (c *Configuracao) ChaveDeAgregacao() string {
	return "aguardar " + c.Topico
}

func (c *Configuracao) Resolver(resolver func(string) string) protocolo.Configuracao {
	copia := *c
	copia.Topico = resolver(c.Topico)
	copia.Esperado = resolver(c.Esperado)
	copia.Campo = resolver(c.Campo)
	return &copia
}

func (c *Configuracao) Descrever() []string {
	onde := "chave da mensagem"
	if c.Campo != "" {
		onde = c.Campo
	}
	return []string{
		fmt.Sprintf("aguardar em %s %s por %s = %q", c.Fonte, c.Topico, onde, c.Esperado),
		"desiste depois de " + c.Timeout.String(),
		"enderecos: " + strings.Join(c.Enderecos, ", "),
	}
}

type Protocolo struct {
	mutex       sync.Mutex
	assinaturas map[string]*assinatura
}

func Novo(protocolo.Opcoes) *Protocolo {
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

func (p *Protocolo) Decodificar(no *yaml.Node) (protocolo.Configuracao, error) {
	if no == nil || no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo aguardar precisa ser um mapa, por exemplo:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"
      timeout: 30s`)
	}

	configuracao := &Configuracao{Timeout: timeoutPadrao}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "kafka", "amqp":
			configuracao.Fonte = chave.Value
			if err := lerFonte(configuracao, valor); err != nil {
				return nil, err
			}
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
			return nil, fmt.Errorf("chave desconhecida no passo aguardar: %q (use kafka, amqp, chave, campo, igual_a ou timeout)", chave.Value)
		}
	}

	if configuracao.Fonte == "" {
		return nil, errors.New(`o passo aguardar precisa dizer onde esperar, por exemplo:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"`)
	}
	if configuracao.Esperado == "" {
		return nil, errors.New(`o passo aguardar precisa do valor que identifica a mensagem desta iteracao.
Sem isso, qualquer mensagem serviria e a medicao mediria o consumidor mais rapido, nao a cadeia:
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidoId}"`)
	}
	return configuracao, nil
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
func (p *Protocolo) Preparar(_ context.Context, requisicao protocolo.Requisicao) error {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return nil
	}
	_, err := p.assinar(configuracao, requisicao.URLBase)
	return err
}

func (p *Protocolo) Executar(ctx context.Context, requisicao protocolo.Requisicao) protocolo.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: "configuracao nao e de aguardar"}
	}

	assinada, err := p.assinar(configuracao, requisicao.URLBase)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}

	timeout := configuracao.Timeout
	if timeout <= 0 {
		timeout = timeoutPadrao
	}

	mensagem, chegou := assinada.esperar(ctx, configuracao.Esperado, timeout)
	if !chegou {
		return protocolo.Resposta{
			Classe: protocolo.ErroDeTimeout,
			Detalhe: fmt.Sprintf("a mensagem com %s=%q nao chegou em %s no topico %s",
				ondeProcura(configuracao), configuracao.Esperado, timeout, configuracao.Topico),
		}
	}

	return protocolo.Resposta{
		Corpo:     mensagem.corpo,
		Bytes:     int64(len(mensagem.corpo)),
		Classe:    protocolo.Sucesso,
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
