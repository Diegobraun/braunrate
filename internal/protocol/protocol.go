package protocol

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

type ErrorClass string

const (
	Success        ErrorClass = "sucesso"
	ErrNetwork     ErrorClass = "rede"
	ErrTimeout     ErrorClass = "timeout"
	ErrStatus      ErrorClass = "status"
	ErrAssertion   ErrorClass = "assercao"
	ErrCorrelation ErrorClass = "correlacao"
	ErrConfig      ErrorClass = "configuracao"
	ErrSaturation  ErrorClass = "saturacao"
	ErrGraphQL     ErrorClass = "graphql"
	ErrMessaging   ErrorClass = "mensageria"

	// Falha ao autenticar tem classe propria porque cair em "configuracao"
	// mandava procurar defeito no cenario quando o alvo e que estava fora do ar.
	ErrAuth ErrorClass = "autenticacao"
)

type Config interface {
	Protocol() string
	AggregationKey() string
	Resolve(func(string) string) Config
}

// Implementada pelos protocolos que sabem se descrever para o modo de
// depuracao, em vez de deixar a struct interna vazar para o usuario.
type Describable interface {
	Config
	Describe() []string
}

// Implementada pelos protocolos que aceitam cabecalho: e por aqui que o motor
// injeta autenticacao sem que o protocolo saiba que ela existe.
type WithHeaders interface {
	Config
	WithHeader(name, value string) Config
}

type Request struct {
	StepName string
	Config   Config
	URLBase  string
	Vars     map[string]string
}

type Response struct {
	Status  int
	Body    []byte
	Headers map[string][]string
	Bytes   int64
	Class   ErrorClass
	Detail  string
	Key     string
	// Fatos sobre o destino que so o protocolo conhece e que entram na
	// variedade observada: particao do Kafka, fila do AMQP. Vazio nos
	// protocolos que nao tem nada a declarar, e ai nao custa nada.
	Attributes map[string]string
}

// Implementada pelo protocolo que sabe quantos destinos distintos existem do
// lado do servidor: sem isso o relatorio nao consegue dizer que usar uma
// particao so foi defeito, e nao um topico de uma particao.
type WithAvailability interface {
	Available() map[string]int64
}

// Implementada pelo protocolo que precisa estar ouvindo antes de a carga
// comecar. Sem isso, a mensagem da primeira iteracao pode chegar antes de
// existir quem a espere, e o timeout seria do braunrate, nao do servico.
type Preparable interface {
	Prepare(ctx context.Context, request Request) error
}

type Protocol interface {
	Name() string
	Decode(no *yaml.Node) (Config, error)
	Execute(ctx context.Context, request Request) Response
	Close() error
}

var record = map[string]Protocol{}

func Record(p Protocol) {
	if _, exists := record[p.Name()]; exists {
		panic(fmt.Sprintf("protocolo ja registrado: %s", p.Name()))
	}
	record[p.Name()] = p
}

func Lookup(name string) (Protocol, bool) {
	p, exists := record[name]
	return p, exists
}

func Registered() []string {
	names := make([]string, 0, len(record))
	for name := range record {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func CloseAll() {
	for _, p := range record {
		_ = p.Close()
	}
}

type Options struct {
	Timeout        time.Duration
	SeguirRedirect bool
	MaxRedirects   int
	KeepCookies    bool
	ConnsPerHost   int
}

func DefaultOptions() Options {
	return Options{
		Timeout:        30 * time.Second,
		SeguirRedirect: true,
		MaxRedirects:   10,
		KeepCookies:    false,
		ConnsPerHost:   0,
	}
}
