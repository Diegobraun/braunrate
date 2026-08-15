package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/segmentio/kafka-go"
	"gopkg.in/yaml.v3"
)

func init() {
	protocolo.Registrar(Novo(protocolo.OpcoesPadrao()))
}

type Configuracao struct {
	Topico     string
	Chave      string
	Valor      []byte
	Cabecalhos map[string]string
	Brokers    []string
	Acks       string
	Timeout    time.Duration
}

func (c *Configuracao) Protocolo() string { return "kafka" }

// A chave e o topico, e nao o broker: quem le o relatorio precisa saber qual
// fluxo de negocio ficou lento, nao qual maquina recebeu o byte.
func (c *Configuracao) ChaveDeAgregacao() string {
	return "kafka produzir " + c.Topico
}

func (c *Configuracao) Resolver(resolver func(string) string) protocolo.Configuracao {
	copia := *c
	copia.Topico = resolver(c.Topico)
	copia.Chave = resolver(c.Chave)
	if len(c.Valor) > 0 {
		copia.Valor = []byte(resolver(string(c.Valor)))
	}
	copia.Cabecalhos = make(map[string]string, len(c.Cabecalhos))
	for nome, valor := range c.Cabecalhos {
		copia.Cabecalhos[nome] = resolver(valor)
	}
	return &copia
}

func (c *Configuracao) Descrever() []string {
	linhas := []string{fmt.Sprintf("produzir em %s (chave %q)", c.Topico, c.Chave)}
	nomes := make([]string, 0, len(c.Cabecalhos))
	for nome := range c.Cabecalhos {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	for _, nome := range nomes {
		linhas = append(linhas, fmt.Sprintf("cabecalho %s: %s", nome, c.Cabecalhos[nome]))
	}
	if len(c.Brokers) > 0 {
		linhas = append(linhas, "brokers: "+strings.Join(c.Brokers, ", "))
	}
	linhas = append(linhas, "acks: "+c.Acks)
	if len(c.Valor) > 0 {
		linhas = append(linhas, "valor: "+string(c.Valor))
	}
	return linhas
}

type Protocolo struct {
	opcoes     protocolo.Opcoes
	mutex      sync.Mutex
	escritores map[string]*kafka.Writer
	particoes  map[string]int64
}

func Novo(opcoes protocolo.Opcoes) *Protocolo {
	return &Protocolo{opcoes: opcoes, escritores: map[string]*kafka.Writer{}, particoes: map[string]int64{}}
}

func (p *Protocolo) Nome() string { return "kafka" }

func (p *Protocolo) Encerrar() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	var ultimo error
	for chave, escritor := range p.escritores {
		if err := escritor.Close(); err != nil {
			ultimo = err
		}
		delete(p.escritores, chave)
	}
	return ultimo
}

// Quantas particoes cada topico tem. E o que permite ao relatorio dizer que
// mandar tudo para uma particao so foi defeito de chave, e nao um topico de
// uma particao.
func (p *Protocolo) Disponiveis() map[string]int64 {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	disponiveis := make(map[string]int64, len(p.particoes))
	for topico, quantas := range p.particoes {
		disponiveis["kafka.particao."+topico] = quantas
	}
	return disponiveis
}

func (p *Protocolo) Decodificar(no *yaml.Node) (protocolo.Configuracao, error) {
	if no == nil || no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo kafka precisa ser um mapa, por exemplo:
  - kafka:
      topico: pedidos
      chave: "${assinantes.id}"
      valor: { id: "${assinantes.id}", total: 199.90 }`)
	}

	configuracao := &Configuracao{Cabecalhos: map[string]string{}, Acks: "todos"}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "topico":
			configuracao.Topico = valor.Value
		case "chave":
			configuracao.Chave = valor.Value
		case "valor":
			corpo, err := lerValor(valor)
			if err != nil {
				return nil, err
			}
			configuracao.Valor = corpo
		case "cabecalhos":
			if valor.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				configuracao.Cabecalhos[valor.Content[i].Value] = valor.Content[i+1].Value
			}
		case "brokers":
			if valor.Kind == yaml.ScalarNode {
				configuracao.Brokers = strings.Split(valor.Value, ",")
				break
			}
			for _, item := range valor.Content {
				configuracao.Brokers = append(configuracao.Brokers, item.Value)
			}
		case "acks":
			switch valor.Value {
			case "todos", "lider", "nenhum":
				configuracao.Acks = valor.Value
			default:
				return nil, fmt.Errorf("acks desconhecido: %q (use todos, lider ou nenhum)", valor.Value)
			}
		case "timeout":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 5s, 30s)", valor.Value)
			}
			configuracao.Timeout = duracao
		default:
			return nil, fmt.Errorf("chave desconhecida no passo kafka: %q (use topico, chave, valor, cabecalhos, brokers, acks ou timeout)", chave.Value)
		}
	}

	if configuracao.Topico == "" {
		return nil, errors.New(`passo kafka sem topico, por exemplo:
  - kafka: { topico: pedidos, valor: { id: "${assinantes.id}" } }`)
	}
	if len(configuracao.Valor) == 0 {
		return nil, errors.New(`passo kafka sem valor: uma mensagem vazia nao exercita o consumidor.
  - kafka: { topico: pedidos, valor: { id: "${assinantes.id}" } }`)
	}
	return configuracao, nil
}

func lerValor(no *yaml.Node) ([]byte, error) {
	if no.Kind == yaml.ScalarNode {
		return []byte(no.Value), nil
	}
	var estrutura any
	if err := no.Decode(&estrutura); err != nil {
		return nil, fmt.Errorf("valor invalido: %v", err)
	}
	corpo, err := json.Marshal(estrutura)
	if err != nil {
		return nil, fmt.Errorf("valor nao serializa para JSON: %v", err)
	}
	return corpo, nil
}

func (p *Protocolo) Executar(ctx context.Context, requisicao protocolo.Requisicao) protocolo.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: "configuracao nao e de kafka"}
	}

	brokers := configuracao.Brokers
	if len(brokers) == 0 {
		brokers = brokersDoAlvo(requisicao.URLBase)
	}
	if len(brokers) == 0 {
		return protocolo.Resposta{
			Classe:  protocolo.ErroDeConfigacao,
			Detalhe: "sem broker: declare 'brokers' no passo ou aponte o alvo do cenario para kafka://host:9092",
		}
	}

	escritor, err := p.escritorDe(brokers, configuracao)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}

	if configuracao.Timeout > 0 {
		var cancelar context.CancelFunc
		ctx, cancelar = context.WithTimeout(ctx, configuracao.Timeout)
		defer cancelar()
	}

	mensagem := kafka.Message{Value: configuracao.Valor}
	if configuracao.Chave != "" {
		mensagem.Key = []byte(configuracao.Chave)
	}
	for nome, valor := range configuracao.Cabecalhos {
		mensagem.Headers = append(mensagem.Headers, kafka.Header{Key: nome, Value: []byte(valor)})
	}

	if err := escritor.WriteMessages(ctx, mensagem); err != nil {
		return protocolo.Resposta{Classe: classificar(err), Detalhe: resumir(err.Error())}
	}

	particao := p.particaoDe(brokers, configuracao.Topico, mensagem.Key)
	resposta := protocolo.Resposta{
		Bytes:  int64(len(configuracao.Valor) + len(mensagem.Key)),
		Classe: protocolo.Sucesso,
	}
	if particao >= 0 {
		resposta.Atributos = map[string]string{
			"kafka.particao." + configuracao.Topico: strconv.Itoa(particao),
		}
	}
	return resposta
}

func (p *Protocolo) escritorDe(brokers []string, configuracao *Configuracao) (*kafka.Writer, error) {
	chave := strings.Join(brokers, ",") + "|" + configuracao.Topico + "|" + configuracao.Acks

	p.mutex.Lock()
	defer p.mutex.Unlock()
	if escritor, existe := p.escritores[chave]; existe {
		return escritor, nil
	}

	// Sem lote e sem espera: o braunrate mede o tempo ate o broker confirmar a
	// mensagem daquela chegada agendada. Agrupar mensagens melhoraria a vazao e
	// mediria o lote, nao a mensagem.
	escritor := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  configuracao.Topico,
		Balancer:               &kafka.Hash{},
		BatchSize:              1,
		BatchTimeout:           time.Millisecond,
		RequiredAcks:           acksDe(configuracao.Acks),
		AllowAutoTopicCreation: true,
		Async:                  false,
	}
	p.escritores[chave] = escritor

	if _, medido := p.particoes[configuracao.Topico]; !medido {
		if quantas := contarParticoes(brokers, configuracao.Topico); quantas > 0 {
			p.particoes[configuracao.Topico] = int64(quantas)
		}
	}
	return escritor, nil
}

func acksDe(acks string) kafka.RequiredAcks {
	switch acks {
	case "nenhum":
		return kafka.RequireNone
	case "lider":
		return kafka.RequireOne
	default:
		return kafka.RequireAll
	}
}

// A particao e calculada com o mesmo balanceador usado no envio: o kafka-go nao
// devolve a particao escolhida, e a alternativa seria nao declarar nada sobre
// distribuicao — que e justamente onde a carga fica otimista sem ninguem ver.
func (p *Protocolo) particaoDe(brokers []string, topico string, chave []byte) int {
	p.mutex.Lock()
	quantas, conhecida := p.particoes[topico]
	p.mutex.Unlock()
	if !conhecida || quantas <= 0 {
		return -1
	}
	if len(chave) == 0 {
		return -1
	}
	balanceador := &kafka.Hash{}
	lista := make([]int, 0, quantas)
	for indice := 0; indice < int(quantas); indice++ {
		lista = append(lista, indice)
	}
	return balanceador.Balance(kafka.Message{Key: chave}, lista...)
}

func contarParticoes(brokers []string, topico string) int {
	conexao, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return 0
	}
	defer conexao.Close()
	particoes, err := conexao.ReadPartitions(topico)
	if err != nil {
		return 0
	}
	return len(particoes)
}

func brokersDoAlvo(alvo string) []string {
	if alvo == "" {
		return nil
	}
	endereco := strings.TrimPrefix(strings.TrimPrefix(alvo, "kafka://"), "tcp://")
	endereco = strings.TrimSuffix(endereco, "/")
	if strings.HasPrefix(alvo, "http://") || strings.HasPrefix(alvo, "https://") {
		return nil
	}
	if endereco == "" {
		return nil
	}
	return strings.Split(endereco, ",")
}

func classificar(err error) protocolo.ClasseDeErro {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocolo.ErroDeTimeout
	}
	texto := err.Error()
	if strings.Contains(texto, "timeout") || strings.Contains(texto, "deadline") {
		return protocolo.ErroDeTimeout
	}
	if strings.Contains(texto, "connection") || strings.Contains(texto, "dial") || strings.Contains(texto, "EOF") {
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
