package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

func init() {
	protocolo.Registrar(Novo(protocolo.OpcoesPadrao()))
}

type Configuracao struct {
	Troca       string
	Rota        string
	Fila        string
	Corpo       []byte
	Identidade  string
	Cabecalhos  map[string]string
	URL         string
	Persistente bool
	Confirmar   bool
	Timeout     time.Duration
}

func (c *Configuracao) Protocolo() string { return "amqp" }

// A chave e a rota do negocio, e nao a conexao: e o que aparece no relatorio
// quando uma rota especifica fica lenta.
func (c *Configuracao) ChaveDeAgregacao() string {
	destino := c.Rota
	if c.Troca != "" {
		destino = c.Troca + "/" + c.Rota
	}
	return "amqp publicar " + destino
}

func (c *Configuracao) Resolver(resolver func(string) string) protocolo.Configuracao {
	copia := *c
	copia.Troca = resolver(c.Troca)
	copia.Rota = resolver(c.Rota)
	copia.Fila = resolver(c.Fila)
	copia.Identidade = resolver(c.Identidade)
	if len(c.Corpo) > 0 {
		copia.Corpo = []byte(resolver(string(c.Corpo)))
	}
	copia.Cabecalhos = make(map[string]string, len(c.Cabecalhos))
	for nome, valor := range c.Cabecalhos {
		copia.Cabecalhos[nome] = resolver(valor)
	}
	return &copia
}

func (c *Configuracao) Descrever() []string {
	linhas := []string{fmt.Sprintf("publicar em troca %q com rota %q", c.Troca, c.Rota)}
	if c.Identidade != "" {
		linhas = append(linhas, "identidade da mensagem: "+c.Identidade)
	}
	if c.Confirmar {
		linhas = append(linhas, "espera confirmacao do broker")
	}
	if len(c.Corpo) > 0 {
		linhas = append(linhas, "corpo: "+string(c.Corpo))
	}
	return linhas
}

type Protocolo struct {
	mutex    sync.Mutex
	conexoes map[string]*conexao
}

type conexao struct {
	ligacao  *amqp.Connection
	mutex    sync.Mutex
	canais   chan *amqp.Channel
	confirma bool
}

func Novo(protocolo.Opcoes) *Protocolo {
	return &Protocolo{conexoes: map[string]*conexao{}}
}

func (p *Protocolo) Nome() string { return "amqp" }

func (p *Protocolo) Encerrar() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for endereco, aberta := range p.conexoes {
		close(aberta.canais)
		for canal := range aberta.canais {
			_ = canal.Close()
		}
		_ = aberta.ligacao.Close()
		delete(p.conexoes, endereco)
	}
	return nil
}

func (p *Protocolo) Decodificar(no *yaml.Node) (protocolo.Configuracao, error) {
	if no == nil || no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo amqp precisa ser um mapa, por exemplo:
  - amqp:
      fila: pedidos
      corpo: { id: "${assinantes.id}" }`)
	}

	configuracao := &Configuracao{Cabecalhos: map[string]string{}, Persistente: true, Confirmar: true}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "troca":
			configuracao.Troca = valor.Value
		case "rota":
			configuracao.Rota = valor.Value
		case "fila":
			configuracao.Fila = valor.Value
			if configuracao.Rota == "" {
				configuracao.Rota = valor.Value
			}
		case "corpo":
			corpo, err := lerCorpo(valor)
			if err != nil {
				return nil, err
			}
			configuracao.Corpo = corpo
		case "identidade":
			configuracao.Identidade = valor.Value
		case "cabecalhos":
			if valor.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				configuracao.Cabecalhos[valor.Content[i].Value] = valor.Content[i+1].Value
			}
		case "url":
			configuracao.URL = valor.Value
		case "persistente":
			configuracao.Persistente = valor.Value == "true"
		case "confirmar":
			configuracao.Confirmar = valor.Value == "true"
		case "timeout":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 5s, 30s)", valor.Value)
			}
			configuracao.Timeout = duracao
		default:
			return nil, fmt.Errorf("chave desconhecida no passo amqp: %q (use fila, troca, rota, corpo, identidade, cabecalhos, url, persistente, confirmar ou timeout)", chave.Value)
		}
	}

	if configuracao.Rota == "" && configuracao.Fila == "" {
		return nil, errors.New(`passo amqp sem destino: declare 'fila' (caso comum) ou 'troca' com 'rota'.
  - amqp: { fila: pedidos, corpo: { id: "${assinantes.id}" } }`)
	}
	if len(configuracao.Corpo) == 0 {
		return nil, errors.New(`passo amqp sem corpo: uma mensagem vazia nao exercita o consumidor.
  - amqp: { fila: pedidos, corpo: { id: "${assinantes.id}" } }`)
	}
	return configuracao, nil
}

func lerCorpo(no *yaml.Node) ([]byte, error) {
	if no.Kind == yaml.ScalarNode {
		return []byte(no.Value), nil
	}
	var estrutura any
	if err := no.Decode(&estrutura); err != nil {
		return nil, fmt.Errorf("corpo invalido: %v", err)
	}
	corpo, err := json.Marshal(estrutura)
	if err != nil {
		return nil, fmt.Errorf("corpo nao serializa para JSON: %v", err)
	}
	return corpo, nil
}

func (p *Protocolo) Executar(ctx context.Context, requisicao protocolo.Requisicao) protocolo.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: "configuracao nao e de amqp"}
	}

	endereco := configuracao.URL
	if endereco == "" {
		endereco = requisicao.URLBase
	}
	if endereco == "" || strings.HasPrefix(endereco, "http") {
		return protocolo.Resposta{
			Classe:  protocolo.ErroDeConfigacao,
			Detalhe: "sem endereco: declare 'url' no passo ou aponte o alvo do cenario para amqp://usuario:senha@host:5672/",
		}
	}

	aberta, err := p.conexaoDe(normalizar(endereco), configuracao)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeRede, Detalhe: resumir(err.Error())}
	}

	canal, err := aberta.pegarCanal()
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeRede, Detalhe: resumir(err.Error())}
	}
	defer aberta.devolverCanal(canal)

	if configuracao.Timeout > 0 {
		var cancelar context.CancelFunc
		ctx, cancelar = context.WithTimeout(ctx, configuracao.Timeout)
		defer cancelar()
	}

	entrega := amqp.Publishing{
		Body:        configuracao.Corpo,
		ContentType: "application/json",
		MessageId:   configuracao.Identidade,
		Timestamp:   time.Now(),
	}
	if configuracao.Persistente {
		entrega.DeliveryMode = amqp.Persistent
	}
	if len(configuracao.Cabecalhos) > 0 {
		entrega.Headers = amqp.Table{}
		for nome, valor := range configuracao.Cabecalhos {
			entrega.Headers[nome] = valor
		}
	}

	confirmacao, err := canal.PublishWithDeferredConfirmWithContext(ctx, configuracao.Troca, configuracao.Rota, false, false, entrega)
	if err != nil {
		return protocolo.Resposta{Classe: classificar(err), Detalhe: resumir(err.Error())}
	}

	// Sem esperar a confirmacao, o tempo medido seria o de escrever no socket,
	// e nao o de o broker aceitar a mensagem — mediria a rede local.
	if configuracao.Confirmar && confirmacao != nil {
		aceita, err := confirmacao.WaitContext(ctx)
		if err != nil {
			return protocolo.Resposta{Classe: classificar(err), Detalhe: resumir(err.Error())}
		}
		if !aceita {
			return protocolo.Resposta{
				Classe:  protocolo.ErroDeMensageria,
				Detalhe: fmt.Sprintf("o broker recusou a mensagem para a rota %q", configuracao.Rota),
			}
		}
	}

	return protocolo.Resposta{
		Bytes:     int64(len(configuracao.Corpo)),
		Classe:    protocolo.Sucesso,
		Atributos: map[string]string{"amqp.rota": configuracao.Rota},
	}
}

func (p *Protocolo) conexaoDe(endereco string, configuracao *Configuracao) (*conexao, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if existente, tem := p.conexoes[endereco]; tem {
		return existente, nil
	}

	ligacao, err := amqp.Dial(endereco)
	if err != nil {
		return nil, err
	}
	nova := &conexao{ligacao: ligacao, canais: make(chan *amqp.Channel, 64), confirma: configuracao.Confirmar}

	if configuracao.Fila != "" {
		canal, err := ligacao.Channel()
		if err != nil {
			_ = ligacao.Close()
			return nil, err
		}
		if _, err := canal.QueueDeclare(configuracao.Fila, true, false, false, false, nil); err != nil {
			_ = canal.Close()
			_ = ligacao.Close()
			return nil, fmt.Errorf("nao consegui declarar a fila %q: %v", configuracao.Fila, err)
		}
		_ = canal.Close()
	}

	p.conexoes[endereco] = nova
	return nova, nil
}

// Um canal AMQP nao e seguro para uso concorrente, entao cada requisicao pega
// um do pool e devolve; abrir um canal por mensagem custaria um ida e volta a
// mais dentro da medicao.
func (c *conexao) pegarCanal() (*amqp.Channel, error) {
	select {
	case canal := <-c.canais:
		if canal != nil && !canal.IsClosed() {
			return canal, nil
		}
	default:
	}

	canal, err := c.ligacao.Channel()
	if err != nil {
		return nil, err
	}
	if c.confirma {
		if err := canal.Confirm(false); err != nil {
			_ = canal.Close()
			return nil, err
		}
	}
	return canal, nil
}

func (c *conexao) devolverCanal(canal *amqp.Channel) {
	if canal == nil || canal.IsClosed() {
		return
	}
	select {
	case c.canais <- canal:
	default:
		_ = canal.Close()
	}
}

func normalizar(endereco string) string {
	if strings.HasPrefix(endereco, "amqp://") || strings.HasPrefix(endereco, "amqps://") {
		return endereco
	}
	return "amqp://" + endereco
}

func classificar(err error) protocolo.ClasseDeErro {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocolo.ErroDeTimeout
	}
	texto := err.Error()
	if strings.Contains(texto, "timeout") || strings.Contains(texto, "deadline") {
		return protocolo.ErroDeTimeout
	}
	if strings.Contains(texto, "closed") || strings.Contains(texto, "connection") {
		return protocolo.ErroDeRede
	}
	return protocolo.ErroDeMensageria
}

func resumir(texto string) string {
	texto = strings.Join(strings.Fields(texto), " ")
	if len(texto) > 140 {
		return texto[:140] + "…"
	}
	return texto
}
