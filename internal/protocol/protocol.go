package protocol

import (
	"context"
	"fmt"
	"github.com/Diegobraun/braunrate/internal/messaging"
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

	// Auth failure gets its own class because falling into "configuracao" sent
	// people looking for a defect in the scenario when the target was down.
	ErrAuth ErrorClass = "autenticacao"

	// A credential that was accepted and has no permission is not a wrong
	// password: one is fixed in the environment, the other in the broker ACL.
	ErrAuthorization ErrorClass = "autorizacao"
)

// ErrorClasses exists so the report can be checked against the whole list: a
// class the report has no phrase for used to print an empty line.
var ErrorClasses = []ErrorClass{
	ErrNetwork, ErrTimeout, ErrStatus, ErrAssertion, ErrCorrelation,
	ErrConfig, ErrSaturation, ErrGraphQL, ErrMessaging, ErrAuth, ErrAuthorization,
}

type Config interface {
	Protocol() string
	AggregationKey() string
	Resolve(func(string) string) Config
}

// Describable is implemented by protocols that can describe themselves for
// debug mode, instead of leaking the internal struct to the user.
type Describable interface {
	Config
	Describe() []string
}

// WithHeaders is implemented by protocols that accept headers: it is how the
// engine injects auth without the protocol knowing auth exists.
type WithHeaders interface {
	Config
	WithHeader(name, value string) Config
}

type Request struct {
	StepName  string
	Config    Config
	URLBase   string
	Vars      map[string]string
	Messaging *messaging.Settings
}

type Response struct {
	Status  int
	Body    []byte
	Headers map[string][]string
	Bytes   int64
	Class   ErrorClass
	Detail  string
	Key     string
	// Facts about the destination only the protocol knows, feeding observed
	// variety: Kafka partition, AMQP queue. Empty for protocols with nothing
	// to declare, and then it costs nothing.
	Attributes map[string]string
}

// WithAvailability is implemented by protocols that know how many distinct
// destinations exist server-side: without it the report cannot tell a
// single-partition defect from a single-partition topic.
type WithAvailability interface {
	Available() map[string]int64
}

// Preparable is implemented by protocols that must be listening before the
// load starts, or that pay a cost to open — a subscription, a TLS and SASL
// handshake, a schema negotiation. Everything done here happens before the run
// clock starts, so it never lands in any percentile.
//
// A protocol that does not implement this is asserting that opening costs the
// same as operating. When that assertion was wrong it invalidated the run
// twice: the consumer subscription in phase 5 and the handshake in phase 7 both
// pushed the first scheduled instants into the past, and the run declared
// itself saturated for a delay the generator never caused. See ADR 0003 §7.
type Preparable interface {
	Prepare(runContext context.Context, request Request) error
}

// ConsumerLag is what a protocol may report about a consumer group it was asked
// to watch. The engine records it, like every other number: a protocol declares
// what it measured and never writes to the report itself (ADR 0003 §3).
type ConsumerLag struct {
	Group       string        `json:"grupo"`
	Topic       string        `json:"topico"`
	Max         int64         `json:"atraso_maximo"`
	Final       int64         `json:"atraso_no_fim"`
	ByPartition map[int]int64 `json:"atraso_maximo_por_particao"`
	Readings    int           `json:"leituras"`
	Problem     string        `json:"problema,omitempty"`
}

// WithConsumerLag is implemented by protocols that watched a consumer group
// while the load ran.
type WithConsumerLag interface {
	ConsumerLag() []ConsumerLag
}

type Protocol interface {
	Name() string
	Decode(node *yaml.Node) (Config, error)
	Execute(runContext context.Context, request Request) Response
	Close() error
}

var record = map[string]Protocol{}

func Register(implementation Protocol) {
	if _, exists := record[implementation.Name()]; exists {
		panic(fmt.Sprintf("protocolo ja registrado: %s", implementation.Name()))
	}
	record[implementation.Name()] = implementation
}

func Lookup(name string) (Protocol, bool) {
	implementation, exists := record[name]
	return implementation, exists
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
	for _, implementation := range record {
		_ = implementation.Close()
	}
}

type Options struct {
	Timeout         time.Duration
	FollowRedirects bool
	MaxRedirects    int
	KeepCookies     bool
	ConnsPerHost    int
}

func DefaultOptions() Options {
	return Options{
		Timeout:         30 * time.Second,
		FollowRedirects: true,
		MaxRedirects:    10,
		KeepCookies:     false,
		ConnsPerHost:    0,
	}
}
