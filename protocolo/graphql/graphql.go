package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/Diegobraun/braunrate/protocolo/transporte"
	"gopkg.in/yaml.v3"
)

const caminhoPadrao = "/graphql"

func init() {
	protocolo.Registrar(Novo(protocolo.OpcoesPadrao()))
}

type Configuracao struct {
	Operacao   string
	Tipo       string
	Consulta   string
	Variaveis  string
	Caminho    string
	Cabecalhos map[string]string
	Timeout    time.Duration
}

func (c *Configuracao) Protocolo() string { return "graphql" }

// A chave e a operacao, nunca a URL: em GraphQL todas as operacoes chegam no
// mesmo endereco, e agregar por URL juntaria a consulta mais barata com a
// mutation mais cara numa linha so.
func (c *Configuracao) ChaveDeAgregacao() string {
	return "graphql " + c.Operacao
}

func (c *Configuracao) Resolver(resolver func(string) string) protocolo.Configuracao {
	copia := *c
	copia.Variaveis = resolver(c.Variaveis)
	copia.Caminho = resolver(c.Caminho)
	copia.Cabecalhos = make(map[string]string, len(c.Cabecalhos))
	for nome, valor := range c.Cabecalhos {
		copia.Cabecalhos[nome] = resolver(valor)
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
	linhas := []string{fmt.Sprintf("%s %s em POST %s", c.Tipo, c.Operacao, c.Caminho)}

	nomes := make([]string, 0, len(c.Cabecalhos))
	for nome := range c.Cabecalhos {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	for _, nome := range nomes {
		linhas = append(linhas, fmt.Sprintf("%s: %s", nome, transporte.EsconderSegredo(nome, c.Cabecalhos[nome])))
	}
	if c.Variaveis != "" && c.Variaveis != "{}" {
		linhas = append(linhas, "variaveis: "+c.Variaveis)
	}
	linhas = append(linhas, "consulta: "+resumirConsulta(c.Consulta))
	return linhas
}

func resumirConsulta(consulta string) string {
	campos := strings.Join(strings.Fields(consulta), " ")
	if len(campos) > 160 {
		return campos[:160] + "…"
	}
	return campos
}

type Protocolo struct {
	cliente *http.Client
}

func Novo(opcoes protocolo.Opcoes) *Protocolo {
	return &Protocolo{cliente: transporte.NovoCliente(opcoes)}
}

func (p *Protocolo) Nome() string { return "graphql" }

func (p *Protocolo) Encerrar() error {
	p.cliente.CloseIdleConnections()
	return nil
}

var padraoDeOperacao = regexp.MustCompile(`(?s)\b(query|mutation|subscription)\s+([A-Za-z_][A-Za-z0-9_]*)`)

func (p *Protocolo) Decodificar(no *yaml.Node) (protocolo.Configuracao, error) {
	if no == nil {
		return nil, errors.New("passo graphql sem configuracao")
	}
	configuracao := Padrao()

	if no.Kind == yaml.ScalarNode {
		configuracao.Consulta = no.Value
		return Finalizar(configuracao)
	}
	if no.Kind != yaml.MappingNode {
		return nil, errors.New(`passo graphql precisa ser a consulta ou um mapa, por exemplo:
  - graphql: |
      query ConsultarPedido { pedido(id: "1") { status } }`)
	}

	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "consulta", "query":
			configuracao.Consulta = valor.Value
		case "operacao":
			configuracao.Operacao = valor.Value
		case "variaveis":
			variaveis, err := lerVariaveis(valor)
			if err != nil {
				return nil, err
			}
			configuracao.Variaveis = variaveis
		case "caminho", "url":
			configuracao.Caminho = valor.Value
		case "cabecalhos":
			if valor.Kind != yaml.MappingNode {
				return nil, errors.New("cabecalhos precisa ser um mapa")
			}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				configuracao.Cabecalhos[valor.Content[i].Value] = valor.Content[i+1].Value
			}
		case "timeout":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, fmt.Errorf("timeout invalido: %q (use 30s, 2m)", valor.Value)
			}
			configuracao.Timeout = duracao
		default:
			return nil, fmt.Errorf("chave desconhecida no passo graphql: %q (use consulta, operacao, variaveis, caminho, cabecalhos ou timeout)", chave.Value)
		}
	}
	return Finalizar(configuracao)
}

// Padrao e Finalizar sao o caminho unico de construcao: a DSL em Go monta a
// mesma configuracao que o YAML monta, incluindo a extracao do nome da operacao.
func Padrao() *Configuracao {
	return &Configuracao{Caminho: caminhoPadrao, Cabecalhos: map[string]string{}, Variaveis: "{}"}
}

func Finalizar(configuracao *Configuracao) (protocolo.Configuracao, error) {
	if strings.TrimSpace(configuracao.Consulta) == "" {
		return nil, errors.New(`passo graphql sem consulta, por exemplo:
  - graphql: |
      query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }`)
	}

	partes := padraoDeOperacao.FindStringSubmatch(configuracao.Consulta)
	if partes != nil {
		configuracao.Tipo = partes[1]
		if configuracao.Operacao == "" {
			configuracao.Operacao = partes[2]
		}
	}
	if configuracao.Tipo == "" {
		configuracao.Tipo = "query"
	}
	if configuracao.Operacao == "" {
		return nil, errors.New(`a operacao graphql precisa de nome: e o nome que vira a linha do relatorio.
Sem nome, todas as operacoes cairiam na mesma linha e a mais cara ficaria escondida na media.
  - graphql: |
      query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }`)
	}
	if configuracao.Caminho == "" {
		configuracao.Caminho = caminhoPadrao
	}
	return configuracao, nil
}

func lerVariaveis(no *yaml.Node) (string, error) {
	if no.Kind == yaml.ScalarNode {
		return no.Value, nil
	}
	var estrutura any
	if err := no.Decode(&estrutura); err != nil {
		return "", fmt.Errorf("variaveis invalidas: %v", err)
	}
	conteudo, err := json.Marshal(estrutura)
	if err != nil {
		return "", fmt.Errorf("variaveis nao serializam para JSON: %v", err)
	}
	return string(conteudo), nil
}

type corpoDeRequisicao struct {
	Consulta      string          `json:"query"`
	OperationName string          `json:"operationName,omitempty"`
	Variaveis     json.RawMessage `json:"variables,omitempty"`
}

type corpoDeResposta struct {
	Data   json.RawMessage `json:"data"`
	Erros  []erroGraphQL   `json:"errors"`
	Extras json.RawMessage `json:"extensions,omitempty"`
}

type erroGraphQL struct {
	Mensagem  string `json:"message"`
	Caminho   []any  `json:"path"`
	Extensoes struct {
		Codigo string `json:"code"`
	} `json:"extensions"`
}

func (p *Protocolo) Executar(ctx context.Context, requisicao protocolo.Requisicao) protocolo.Resposta {
	configuracao, ok := requisicao.Configuracao.(*Configuracao)
	if !ok {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: "configuracao nao e de graphql"}
	}

	endereco, err := transporte.MontarURL(requisicao.URLBase, configuracao.Caminho)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}

	corpo := corpoDeRequisicao{Consulta: configuracao.Consulta, OperationName: configuracao.Operacao}
	if variaveis := strings.TrimSpace(configuracao.Variaveis); variaveis != "" && variaveis != "{}" {
		if !json.Valid([]byte(variaveis)) {
			return protocolo.Resposta{
				Classe:  protocolo.ErroDeConfigacao,
				Detalhe: "as variaveis nao formaram JSON valido depois da interpolacao: " + resumir(variaveis),
			}
		}
		corpo.Variaveis = json.RawMessage(variaveis)
	}
	serializado, err := json.Marshal(corpo)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}

	if configuracao.Timeout > 0 {
		var cancelar context.CancelFunc
		ctx, cancelar = context.WithTimeout(ctx, configuracao.Timeout)
		defer cancelar()
	}

	pedido, err := http.NewRequestWithContext(ctx, http.MethodPost, endereco, bytes.NewReader(serializado))
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}
	pedido.Header.Set("Content-Type", "application/json")
	pedido.Header.Set("Accept", "application/json")
	for nome, valor := range configuracao.Cabecalhos {
		pedido.Header.Set(nome, valor)
	}

	resposta, err := p.cliente.Do(pedido)
	if err != nil {
		return protocolo.Resposta{Classe: transporte.Classificar(err), Detalhe: transporte.ResumirErro(err)}
	}
	defer resposta.Body.Close()

	conteudo, err := io.ReadAll(resposta.Body)
	if err != nil {
		return protocolo.Resposta{Status: resposta.StatusCode, Classe: transporte.Classificar(err), Detalhe: transporte.ResumirErro(err)}
	}

	saida := protocolo.Resposta{
		Status:     resposta.StatusCode,
		Corpo:      conteudo,
		Cabecalhos: resposta.Header,
		Bytes:      int64(len(conteudo)),
		Classe:     protocolo.Sucesso,
	}

	if resposta.StatusCode >= 400 {
		saida.Classe = protocolo.ErroDeStatus
		saida.Detalhe = fmt.Sprintf("status %d", resposta.StatusCode)
		return saida
	}

	classe, detalhe := classificarCorpo(conteudo)
	saida.Classe = classe
	saida.Detalhe = detalhe
	return saida
}

// O erro de GraphQL chega com status 200: tratar o passo como sucesso porque o
// HTTP deu 200 e o jeito mais comum de um teste de carga aprovar um servico
// que esta respondendo erro em todas as requisicoes.
func classificarCorpo(conteudo []byte) (protocolo.ClasseDeErro, string) {
	var corpo corpoDeResposta
	if err := json.Unmarshal(conteudo, &corpo); err != nil {
		return protocolo.ErroDeGraphQL, "a resposta nao e JSON de GraphQL: " + resumir(string(conteudo))
	}
	if len(corpo.Erros) == 0 {
		if len(corpo.Data) == 0 || string(corpo.Data) == "null" {
			return protocolo.ErroDeGraphQL, "resposta sem data e sem errors"
		}
		return protocolo.Sucesso, ""
	}

	primeiro := corpo.Erros[0]
	detalhe := primeiro.Mensagem
	if primeiro.Extensoes.Codigo != "" {
		detalhe = primeiro.Extensoes.Codigo + ": " + detalhe
	}
	if caminho := formatarCaminho(primeiro.Caminho); caminho != "" {
		detalhe += " (em " + caminho + ")"
	}
	if len(corpo.Erros) > 1 {
		detalhe = fmt.Sprintf("%s (+%d erro(s))", detalhe, len(corpo.Erros)-1)
	}
	if len(corpo.Data) > 0 && string(corpo.Data) != "null" {
		detalhe = "resposta parcial — " + detalhe
	}
	return protocolo.ErroDeGraphQL, resumir(detalhe)
}

func formatarCaminho(caminho []any) string {
	if len(caminho) == 0 {
		return ""
	}
	partes := make([]string, 0, len(caminho))
	for _, item := range caminho {
		partes = append(partes, fmt.Sprint(item))
	}
	return strings.Join(partes, ".")
}

func resumir(texto string) string {
	texto = strings.Join(strings.Fields(texto), " ")
	if len(texto) > 140 {
		return texto[:140] + "…"
	}
	return texto
}
