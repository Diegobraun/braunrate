package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"gopkg.in/yaml.v3"
)

func init() {
	protocolo.Registrar(Novo(protocolo.OpcoesPadrao()))
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

func (c *Configuracao) Resolver(resolver func(string) string) protocolo.Configuracao {
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

func (c *Configuracao) ComCabecalho(nome, valor string) protocolo.Configuracao {
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
		linhas = append(linhas, fmt.Sprintf("%s: %s", nome, esconderSegredo(nome, c.Cabecalhos[nome])))
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

// Token e senha aparecem cortados: a saida de depuracao costuma ir parar em
// ticket e em captura de tela.
func esconderSegredo(nome, valor string) string {
	nomeMinusculo := strings.ToLower(nome)
	if nomeMinusculo != "authorization" && !strings.Contains(nomeMinusculo, "token") &&
		!strings.Contains(nomeMinusculo, "senha") && !strings.Contains(nomeMinusculo, "secret") {
		return valor
	}
	prefixo, resto, encontrou := strings.Cut(valor, " ")
	if !encontrou {
		prefixo, resto = "", valor
	}
	if len(resto) <= 6 {
		return strings.TrimSpace(prefixo + " ***")
	}
	return strings.TrimSpace(prefixo + " " + resto[:6] + "… (" + fmt.Sprint(len(resto)) + " caracteres)")
}

type Protocolo struct {
	cliente *http.Client
	opcoes  protocolo.Opcoes
}

func Novo(opcoes protocolo.Opcoes) *Protocolo {
	transporte := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   65536,
		MaxConnsPerHost:       opcoes.ConexoesPorDestino,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	cliente := &http.Client{Transport: transporte, Timeout: opcoes.Timeout}
	if opcoes.ManterCookies {
		if jarra, err := cookiejar.New(nil); err == nil {
			cliente.Jar = jarra
		}
	}
	if !opcoes.SeguirRedirect {
		cliente.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		maximo := opcoes.MaximoDeRedirects
		cliente.CheckRedirect = func(_ *http.Request, anteriores []*http.Request) error {
			if len(anteriores) >= maximo {
				return fmt.Errorf("mais de %d redirects", maximo)
			}
			return nil
		}
	}
	return &Protocolo{cliente: cliente, opcoes: opcoes}
}

func (p *Protocolo) Nome() string { return "http" }

func (p *Protocolo) Encerrar() error {
	p.cliente.CloseIdleConnections()
	return nil
}

func (p *Protocolo) Decodificar(no *yaml.Node) (protocolo.Configuracao, error) {
	if no == nil {
		return nil, errors.New("passo http sem configuracao")
	}
	configuracao := &Configuracao{Metodo: http.MethodGet, Cabecalhos: map[string]string{}}

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

	if configuracao.Caminho == "" {
		return nil, errors.New("passo http sem caminho")
	}
	return configuracao, nil
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

func (p *Protocolo) Executar(ctx context.Context, requisicao protocolo.Requisicao) protocolo.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: "configuracao nao e de http"}
	}

	endereco, err := montarURL(requisicao.URLBase, configuracao.Caminho)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
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
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}
	if configuracao.TipoDeConteudo != "" {
		pedido.Header.Set("Content-Type", configuracao.TipoDeConteudo)
	}
	for nome, valor := range configuracao.Cabecalhos {
		pedido.Header.Set(nome, valor)
	}

	resposta, err := p.cliente.Do(pedido)
	if err != nil {
		return protocolo.Resposta{Classe: classificar(err), Detalhe: resumirErro(err)}
	}
	defer resposta.Body.Close()

	conteudo, err := io.ReadAll(resposta.Body)
	if err != nil {
		return protocolo.Resposta{Status: resposta.StatusCode, Classe: classificar(err), Detalhe: resumirErro(err)}
	}

	classe := protocolo.Sucesso
	detalhe := ""
	if resposta.StatusCode >= 400 {
		classe = protocolo.ErroDeStatus
		detalhe = fmt.Sprintf("status %d", resposta.StatusCode)
	}

	return protocolo.Resposta{
		Status:     resposta.StatusCode,
		Corpo:      conteudo,
		Cabecalhos: resposta.Header,
		Bytes:      int64(len(conteudo)),
		Classe:     classe,
		Detalhe:    detalhe,
	}
}

func montarURL(base, caminho string) (string, error) {
	if strings.HasPrefix(caminho, "http://") || strings.HasPrefix(caminho, "https://") {
		return caminho, nil
	}
	if base == "" {
		return "", fmt.Errorf("passo com caminho relativo %q e cenario sem alvo", caminho)
	}
	enderecoBase, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("alvo invalido: %q", base)
	}
	relativo, err := url.Parse(caminho)
	if err != nil {
		return "", fmt.Errorf("caminho invalido: %q", caminho)
	}
	return enderecoBase.ResolveReference(relativo).String(), nil
}

func classificar(err error) protocolo.ClasseDeErro {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocolo.ErroDeTimeout
	}
	var erroDeRede net.Error
	if errors.As(err, &erroDeRede) && erroDeRede.Timeout() {
		return protocolo.ErroDeTimeout
	}
	return protocolo.ErroDeRede
}

func resumirErro(err error) string {
	texto := err.Error()
	for _, padrao := range []string{"connection refused", "connection reset", "no such host",
		"too many open files", "cannot assign requested address", "context deadline exceeded",
		"EOF", "broken pipe"} {
		if strings.Contains(texto, padrao) {
			return padrao
		}
	}
	if len(texto) > 120 {
		return texto[:120]
	}
	return texto
}
