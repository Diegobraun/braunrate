package protocolo

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

type ClasseDeErro string

const (
	Sucesso          ClasseDeErro = "sucesso"
	ErroDeRede       ClasseDeErro = "rede"
	ErroDeTimeout    ClasseDeErro = "timeout"
	ErroDeStatus     ClasseDeErro = "status"
	ErroDeAssercao   ClasseDeErro = "assercao"
	ErroDeCorrelacao ClasseDeErro = "correlacao"
	ErroDeConfigacao ClasseDeErro = "configuracao"
	ErroDeSaturacao  ClasseDeErro = "saturacao"
	ErroDeGraphQL    ClasseDeErro = "graphql"
)

type Configuracao interface {
	Protocolo() string
	ChaveDeAgregacao() string
	Resolver(func(string) string) Configuracao
}

// Implementada pelos protocolos que sabem se descrever para o modo de
// depuracao, em vez de deixar a struct interna vazar para o usuario.
type ConfiguracaoDescritivel interface {
	Configuracao
	Descrever() []string
}

// Implementada pelos protocolos que aceitam cabecalho: e por aqui que o motor
// injeta autenticacao sem que o protocolo saiba que ela existe.
type ConfiguracaoComCabecalhos interface {
	Configuracao
	ComCabecalho(nome, valor string) Configuracao
}

type Requisicao struct {
	NomeDoPasso  string
	Configuracao Configuracao
	URLBase      string
	Variaveis    map[string]string
}

type Resposta struct {
	Status     int
	Corpo      []byte
	Cabecalhos map[string][]string
	Bytes      int64
	Classe     ClasseDeErro
	Detalhe    string
	Chave      string
}

type Protocolo interface {
	Nome() string
	Decodificar(no *yaml.Node) (Configuracao, error)
	Executar(ctx context.Context, requisicao Requisicao) Resposta
	Encerrar() error
}

var registro = map[string]Protocolo{}

func Registrar(p Protocolo) {
	if _, existe := registro[p.Nome()]; existe {
		panic(fmt.Sprintf("protocolo ja registrado: %s", p.Nome()))
	}
	registro[p.Nome()] = p
}

func Buscar(nome string) (Protocolo, bool) {
	p, existe := registro[nome]
	return p, existe
}

func Registrados() []string {
	nomes := make([]string, 0, len(registro))
	for nome := range registro {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	return nomes
}

func EncerrarTodos() {
	for _, p := range registro {
		_ = p.Encerrar()
	}
}

type Opcoes struct {
	Timeout            time.Duration
	SeguirRedirect     bool
	MaximoDeRedirects  int
	ManterCookies      bool
	ConexoesPorDestino int
}

func OpcoesPadrao() Opcoes {
	return Opcoes{
		Timeout:            30 * time.Second,
		SeguirRedirect:     true,
		MaximoDeRedirects:  10,
		ManterCookies:      false,
		ConexoesPorDestino: 0,
	}
}
