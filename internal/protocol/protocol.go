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

	// Auth failure gets its own class because falling into "configuracao" sent
	// people looking for a defect in the scenario when the target was down.
	ErrAuth ErrorClass = "autenticacao"
)

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
// load starts. Without it the first iteration's message can arrive before
// anyone waits for it, and the timeout would be braunrate's, not the service's.
type Preparable interface {
	Prepare(runContext context.Context, request Request) error
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
