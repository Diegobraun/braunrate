package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	"gopkg.in/yaml.v3"
)

func init() {
	protocol.Registrar(Novo(protocol.OpcoesPadrao()))
}

type Configuracao struct {
	Metodo         string
	Caminho        string
	Cabecalhos     map[string]string
	Corpo          []byte
	TipoDeConteudo string
	Timeout        time.Duration
	SeguirRedirect *bool
}

func (c *Configuracao) Protocolo() string { return "http" }

func (c *Configuracao) ChaveDeAgregacao() string {
	return fmt.Sprintf("%s %s", c.Metodo, c.Caminho)
}

func (c *Configuracao) Resolver(resolver func(string) string) protocol.Configuracao {
	copia := *c
	copia.Caminho = resolver(c.Caminho)
	copia.Cabecalhos = make(map[string]string, len(c.Cabecalhos))
	for nome, valor := range c.Cabecalhos {
		copia.Cabecalhos[nome] = resolver(valor)
	}
	if len(c.Corpo) > 0 {
		copia.Corpo = []byte(resolver(string(c.Corpo)))
	}
	return &copia
}

func (c *Configuracao) ComCabecalho(nome, valor string) protocol.Configuracao {
	copia := *c
	copia.Cabecalhos = make(map[string]string, len(c.Cabecalhos)+1)
	for chave, conteudo := range c.Cabecalhos {
		copia.Cabecalhos[chave] = conteudo
	}
	copia.Cabecalhos[nome] = valor
	return &copia
}

func (c *Configuracao) Descrever() []string {
	linhas := []string{fmt.Sprintf("%s %s", c.Metodo, c.Caminho)}

	nomes := make([]string, 0, len(c.Cabecalhos))
	for nome := range c.Cabecalhos {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	for _, nome := range nomes {
		linhas = append(linhas, fmt.Sprintf("%s: %s", nome, transport.EsconderSegredo(nome, c.Cabecalhos[nome])))
	}
	if c.TipoDeConteudo != "" {
		linhas = append(linhas, "Content-Type: "+c.TipoDeConteudo)
	}
	if len(c.Corpo) > 0 {
		linhas = append(linhas, "corpo: "+string(c.Corpo))
	}
	if c.Timeout > 0 {
		linhas = append(linhas, "timeout: "+c.Timeout.String())
	}
	return linhas
}

type Protocolo struct {
	cliente *http.Client
	opcoes  protocol.Opcoes
}

func Novo(opcoes protocol.Opcoes) *Protocolo {
	return &Protocolo{cliente: transport.NovoCliente(opcoes), opcoes: opcoes}
}

func (p *Protocolo) Nome() string { return "http" }

func (p *Protocolo) Encerrar() error {
	p.cliente.CloseIdleConnections()
	return nil
}

func (p *Protocolo) Decodificar(no *yaml.Node) (protocol.Configuracao, error) {
	if no == nil {
		return nil, errors.New("passo http sem configuracao")
	}
	configuracao := Padrao()

	if no.Kind == yaml.ScalarNode {
		partes := strings.Fields(no.Value)
		switch len(partes) {
		case 1:
			configuracao.Caminho = partes[0]
		case 2:
			configuracao.Metodo = strings.ToUpper(partes[0])
			configuracao.Caminho = partes[1]
		default:
			return nil, fmt.Errorf("forma curta do passo http deve ser \"METODO /caminho\", recebido %q", no.Value)
		}
		return configuracao, nil
	}

	if no.Kind != yaml.MappingNode {
		return nil, errors.New("passo http precisa ser um texto ou um mapa")
	}

	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "metodo":
			configuracao.Metodo = strings.ToUpper(valor.Value)
		case "caminho", "url":
			configuracao.Caminho = valor.Value
		case "cabecalhos":
			if valor.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				configuracao.Cabecalhos[valor.Content[i].Value] = valor.Content[i+1].Value
			}
		case "corpo":
			corpo, tipo, err := lerCorpo(valor)
			if err != nil {
				return nil, err
			}
			configuracao.Corpo = corpo
			configuracao.TipoDeConteudo = tipo
		case "timeout":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q", valor.Value)
			}
			configuracao.Timeout = duracao
		case "seguir_redirect":
			seguir := valor.Value == "true"
			configuracao.SeguirRedirect = &seguir
		default:
			return nil, fmt.Errorf("chave desconhecida no passo http: %q", chave.Value)
		}
	}

	if err := Validar(configuracao); err != nil {
		return nil, err
	}
	return configuracao, nil
}

// Padrao e Validar existem para que a DSL em Go entre pelo mesmo lugar que o
// YAML: um padrao que so um dos dois aplicasse viraria diferenca de medicao
// entre os dois publicos.
func Padrao() *Configuracao {
	return &Configuracao{Metodo: http.MethodGet, Cabecalhos: map[string]string{}}
}

func Validar(configuracao *Configuracao) error {
	if configuracao.Caminho == "" {
		return errors.New("passo http sem caminho")
	}
	return nil
}

func lerCorpo(no *yaml.Node) ([]byte, string, error) {
	if no.Kind == yaml.ScalarNode {
		return []byte(no.Value), "text/plain", nil
	}
	var estrutura any
	if err := no.Decode(&estrutura); err != nil {
		return nil, "", fmt.Errorf("corpo invalido: %v", err)
	}
	corpo, err := json.Marshal(estrutura)
	if err != nil {
		return nil, "", fmt.Errorf("corpo nao serializa para JSON: %v", err)
	}
	return corpo, "application/json", nil
}

func (p *Protocolo) Executar(ctx context.Context, requisicao protocol.Requisicao) protocol.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocol.Resposta{Classe: protocol.ErroDeConfigacao, Detalhe: "configuracao nao e de http"}
	}

	endereco, err := transport.MontarURL(requisicao.URLBase, configuracao.Caminho)
	if err != nil {
		return protocol.Resposta{Classe: protocol.ErroDeConfigacao, Detalhe: err.Error()}
	}

	var corpo io.Reader
	if len(configuracao.Corpo) > 0 {
		corpo = bytes.NewReader(configuracao.Corpo)
	}

	if configuracao.Timeout > 0 {
		var cancelar context.CancelFunc
		ctx, cancelar = context.WithTimeout(ctx, configuracao.Timeout)
		defer cancelar()
	}

	pedido, err := http.NewRequestWithContext(ctx, configuracao.Metodo, endereco, corpo)
	if err != nil {
		return protocol.Resposta{Classe: protocol.ErroDeConfigacao, Detalhe: err.Error()}
	}
	if configuracao.TipoDeConteudo != "" {
		pedido.Header.Set("Content-Type", configuracao.TipoDeConteudo)
	}
	for nome, valor := range configuracao.Cabecalhos {
		pedido.Header.Set(nome, valor)
	}

	resposta, err := p.cliente.Do(pedido)
	if err != nil {
		return protocol.Resposta{Classe: transport.Classificar(err), Detalhe: transport.ResumirErro(err)}
	}
	defer resposta.Body.Close()

	conteudo, err := io.ReadAll(resposta.Body)
	if err != nil {
		return protocol.Resposta{Status: resposta.StatusCode, Classe: transport.Classificar(err), Detalhe: transport.ResumirErro(err)}
	}

	classe := protocol.Sucesso
	detalhe := ""
	if resposta.StatusCode >= 400 {
		classe = protocol.ErroDeStatus
		detalhe = fmt.Sprintf("status %d", resposta.StatusCode)
	}

	return protocol.Resposta{
		Status:     resposta.StatusCode,
		Corpo:      conteudo,
		Cabecalhos: resposta.Header,
		Bytes:      int64(len(conteudo)),
		Classe:     classe,
		Detalhe:    detalhe,
	}
}
