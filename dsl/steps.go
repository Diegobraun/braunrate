package dsl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/amqp"
	"github.com/Diegobraun/braunrate/internal/protocol/graphql"
	protocoloHTTP "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/protocol/kafka"
	"github.com/Diegobraun/braunrate/internal/protocol/wait"
)

// O corpo declarado como estrutura vira JSON pelo mesmo caminho do YAML
// (json.Marshal do mapa lido), entao os bytes enviados sao byte a byte os
// mesmos nos dois publicos.
func serialize(body any) ([]byte, string, error) {
	switch value := body.(type) {
	case nil:
		return nil, "", nil
	case string:
		return []byte(value), "text/plain", nil
	case []byte:
		return value, "text/plain", nil
	default:
		content, err := json.Marshal(value)
		if err != nil {
			return nil, "", fmt.Errorf("corpo nao serializa para JSON: %v", err)
		}
		return content, "application/json", nil
	}
}

type HTTPStep struct {
	config *protocoloHTTP.Config
	err    error
}

func HTTP(method, path string) *HTTPStep {
	config := protocoloHTTP.Default()
	config.Method = strings.ToUpper(method)
	config.Path = path
	return &HTTPStep{config: config}
}

func GET(path string) *HTTPStep    { return HTTP(http.MethodGet, path) }
func POST(path string) *HTTPStep   { return HTTP(http.MethodPost, path) }
func PUT(path string) *HTTPStep    { return HTTP(http.MethodPut, path) }
func PATCH(path string) *HTTPStep  { return HTTP(http.MethodPatch, path) }
func DELETE(path string) *HTTPStep { return HTTP(http.MethodDelete, path) }

func (p *HTTPStep) Header(name, value string) *HTTPStep {
	p.config.Headers[name] = value
	return p
}

func (p *HTTPStep) Body(body any) *HTTPStep {
	content, kind, err := serialize(body)
	if err != nil {
		p.err = err
		return p
	}
	p.config.Body = content
	p.config.ContentType = kind
	return p
}

func (p *HTTPStep) Timeout(timeout time.Duration) *HTTPStep {
	p.config.Timeout = timeout
	return p
}

func (p *HTTPStep) SeguirRedirect(seguir bool) *HTTPStep {
	p.config.SeguirRedirect = &seguir
	return p
}

func (p *HTTPStep) build() (string, protocol.Config, error) {
	if p.err != nil {
		return "", nil, p.err
	}
	if err := protocoloHTTP.Validate(p.config); err != nil {
		return "", nil, err
	}
	return "http", p.config, nil
}

type GraphQLStep struct {
	config *graphql.Config
	err    error
}

func GraphQL(query string) *GraphQLStep {
	config := graphql.Default()
	config.Query = query
	return &GraphQLStep{config: config}
}

func (p *GraphQLStep) Operation(name string) *GraphQLStep {
	p.config.Operation = name
	return p
}

func (p *GraphQLStep) Vars(vars any) *GraphQLStep {
	content, err := json.Marshal(vars)
	if err != nil {
		p.err = fmt.Errorf("variaveis de graphql nao serializam para JSON: %v", err)
		return p
	}
	p.config.Vars = string(content)
	return p
}

func (p *GraphQLStep) Path(path string) *GraphQLStep {
	p.config.Path = path
	return p
}

func (p *GraphQLStep) Header(name, value string) *GraphQLStep {
	p.config.Headers[name] = value
	return p
}

func (p *GraphQLStep) Timeout(timeout time.Duration) *GraphQLStep {
	p.config.Timeout = timeout
	return p
}

func (p *GraphQLStep) build() (string, protocol.Config, error) {
	if p.err != nil {
		return "", nil, p.err
	}
	config, err := graphql.Finish(p.config)
	if err != nil {
		return "", nil, err
	}
	return "graphql", config, nil
}

type KafkaStep struct {
	config *kafka.Config
	err    error
}

func Kafka(topic string) *KafkaStep {
	config := kafka.Default()
	config.Topic = topic
	return &KafkaStep{config: config}
}

func (p *KafkaStep) Key(key string) *KafkaStep {
	p.config.Key = key
	return p
}

func (p *KafkaStep) Value(value any) *KafkaStep {
	content, _, err := serialize(value)
	if err != nil {
		p.err = err
		return p
	}
	p.config.Value = content
	return p
}

func (p *KafkaStep) Header(name, value string) *KafkaStep {
	p.config.Headers[name] = value
	return p
}

func (p *KafkaStep) Brokers(brokers ...string) *KafkaStep {
	p.config.Brokers = brokers
	return p
}

func (p *KafkaStep) Acks(acks string) *KafkaStep {
	switch acks {
	case "todos", "lider", "nenhum":
		p.config.Acks = acks
	default:
		p.err = fmt.Errorf("acks desconhecido: %q (use todos, lider ou nenhum)", acks)
	}
	return p
}

func (p *KafkaStep) Timeout(timeout time.Duration) *KafkaStep {
	p.config.Timeout = timeout
	return p
}

func (p *KafkaStep) build() (string, protocol.Config, error) {
	if p.err != nil {
		return "", nil, p.err
	}
	if err := kafka.Validate(p.config); err != nil {
		return "", nil, err
	}
	return "kafka", p.config, nil
}

type AMQPStep struct {
	config *amqp.Config
	err    error
}

func AMQP(queue string) *AMQPStep {
	config := amqp.Default()
	config.Queue = queue
	config.Route = queue
	return &AMQPStep{config: config}
}

func Exchange(exchange, route string) *AMQPStep {
	config := amqp.Default()
	config.Exchange = exchange
	config.Route = route
	return &AMQPStep{config: config}
}

func (p *AMQPStep) Body(body any) *AMQPStep {
	content, _, err := serialize(body)
	if err != nil {
		p.err = err
		return p
	}
	p.config.Body = content
	return p
}

func (p *AMQPStep) Identity(identity string) *AMQPStep {
	p.config.Identity = identity
	return p
}

func (p *AMQPStep) Header(name, value string) *AMQPStep {
	p.config.Headers[name] = value
	return p
}

func (p *AMQPStep) URL(url string) *AMQPStep {
	p.config.URL = url
	return p
}

func (p *AMQPStep) Persistent(persistent bool) *AMQPStep {
	p.config.Persistent = persistent
	return p
}

func (p *AMQPStep) Confirm(confirm bool) *AMQPStep {
	p.config.Confirm = confirm
	return p
}

func (p *AMQPStep) Timeout(timeout time.Duration) *AMQPStep {
	p.config.Timeout = timeout
	return p
}

func (p *AMQPStep) build() (string, protocol.Config, error) {
	if p.err != nil {
		return "", nil, p.err
	}
	if err := amqp.Validate(p.config); err != nil {
		return "", nil, err
	}
	return "amqp", p.config, nil
}

type WaitStep struct {
	config *wait.Config
}

func WaitForKafka(topic string) *WaitStep {
	config := wait.Default()
	config.Source = "kafka"
	config.Topic = topic
	return &WaitStep{config: config}
}

// Espera por HTTP existe para o sistema que so mostra o efeito por API: sem
// isto, a cadeia ponta a ponta nao se mede nele.
func WaitForHTTP(path string) *WaitStep {
	config := wait.Default()
	config.Source = "http"
	config.Path = path
	return &WaitStep{config: config}
}

func WaitForAMQP(queue string) *WaitStep {
	config := wait.Default()
	config.Source = "amqp"
	config.Topic = queue
	return &WaitStep{config: config}
}

// A correlacao e obrigatoria pelo mesmo motivo do YAML: esperar qualquer
// mensagem mediria o consumidor mais rapido, e nao a jornada desta iteracao.
func (p *WaitStep) Key(expected string) *WaitStep {
	p.config.Expected = expected
	return p
}

func (p *WaitStep) Field(field string) *WaitStep {
	p.config.Field = field
	return p
}

func (p *WaitStep) Addresses(addresses ...string) *WaitStep {
	p.config.Addresses = addresses
	return p
}

func (p *WaitStep) UntilJSON(path, value string) *WaitStep {
	p.config.To = wait.Condition{Path: path, Value: value}
	return p
}

func (p *WaitStep) UntilStatus(status int) *WaitStep {
	p.config.To = wait.Condition{Status: status}
	return p
}

func (p *WaitStep) UntilBodyContains(fragment string) *WaitStep {
	p.config.To = wait.Condition{BodyContains: fragment}
	return p
}

func (p *WaitStep) Interval(interval time.Duration) *WaitStep {
	p.config.Interval = interval
	return p
}

func (p *WaitStep) Timeout(timeout time.Duration) *WaitStep {
	p.config.Timeout = timeout
	return p
}

func (p *WaitStep) build() (string, protocol.Config, error) {
	if err := wait.Validate(p.config); err != nil {
		return "", nil, err
	}
	return "aguardar", p.config, nil
}
