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

// A body declared as a structure becomes JSON through the same path the YAML
// takes (json.Marshal of the parsed map), so the bytes on the wire are
// identical for both audiences.
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

func (step *HTTPStep) Header(name, value string) *HTTPStep {
	step.config.Headers[name] = value
	return step
}

func (step *HTTPStep) Body(body any) *HTTPStep {
	content, kind, err := serialize(body)
	if err != nil {
		step.err = err
		return step
	}
	step.config.Body = content
	step.config.ContentType = kind
	return step
}

func (step *HTTPStep) Timeout(timeout time.Duration) *HTTPStep {
	step.config.Timeout = timeout
	return step
}

func (step *HTTPStep) FollowRedirects(follow bool) *HTTPStep {
	step.config.FollowRedirects = &follow
	return step
}

func (step *HTTPStep) build() (string, protocol.Config, error) {
	if step.err != nil {
		return "", nil, step.err
	}
	if err := protocoloHTTP.Validate(step.config); err != nil {
		return "", nil, err
	}
	return "http", step.config, nil
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

func (step *GraphQLStep) Operation(name string) *GraphQLStep {
	step.config.Operation = name
	return step
}

func (step *GraphQLStep) Vars(vars any) *GraphQLStep {
	content, err := json.Marshal(vars)
	if err != nil {
		step.err = fmt.Errorf("variaveis de graphql nao serializam para JSON: %v", err)
		return step
	}
	step.config.Vars = string(content)
	return step
}

func (step *GraphQLStep) Path(path string) *GraphQLStep {
	step.config.Path = path
	return step
}

func (step *GraphQLStep) Header(name, value string) *GraphQLStep {
	step.config.Headers[name] = value
	return step
}

func (step *GraphQLStep) Timeout(timeout time.Duration) *GraphQLStep {
	step.config.Timeout = timeout
	return step
}

func (step *GraphQLStep) build() (string, protocol.Config, error) {
	if step.err != nil {
		return "", nil, step.err
	}
	config, err := graphql.Finish(step.config)
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

func (step *KafkaStep) Key(key string) *KafkaStep {
	step.config.Key = key
	return step
}

func (step *KafkaStep) Value(value any) *KafkaStep {
	content, _, err := serialize(value)
	if err != nil {
		step.err = err
		return step
	}
	step.config.Value = content
	return step
}

func (step *KafkaStep) Header(name, value string) *KafkaStep {
	step.config.Headers[name] = value
	return step
}

func (step *KafkaStep) Brokers(brokers ...string) *KafkaStep {
	step.config.Brokers = brokers
	return step
}

func (step *KafkaStep) Acks(acks string) *KafkaStep {
	switch acks {
	case "todos", "lider", "nenhum":
		step.config.Acks = acks
	default:
		step.err = fmt.Errorf("acks desconhecido: %q (use todos, lider ou nenhum)", acks)
	}
	return step
}

func (step *KafkaStep) Timeout(timeout time.Duration) *KafkaStep {
	step.config.Timeout = timeout
	return step
}

// Partition sends the whole load of this step to one partition, ignoring the
// key. It concentrates on purpose, and the report says so.
func (step *KafkaStep) Partition(partition int) *KafkaStep {
	if partition < 0 {
		step.err = fmt.Errorf("particao invalida: %d (use um numero, como 0 ou 3)", partition)
		return step
	}
	step.config.Partition = &partition
	return step
}

// Group names a consumer group to watch while the load runs. It only observes:
// entering the group would take a partition away from the service being
// measured.
func (step *KafkaStep) Group(group string) *KafkaStep {
	step.config.Group = group
	return step
}

func (step *KafkaStep) build() (string, protocol.Config, error) {
	if step.err != nil {
		return "", nil, step.err
	}
	if err := kafka.Validate(step.config); err != nil {
		return "", nil, err
	}
	return "kafka", step.config, nil
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

func (step *AMQPStep) Body(body any) *AMQPStep {
	content, _, err := serialize(body)
	if err != nil {
		step.err = err
		return step
	}
	step.config.Body = content
	return step
}

func (step *AMQPStep) Identity(identity string) *AMQPStep {
	step.config.Identity = identity
	return step
}

func (step *AMQPStep) Header(name, value string) *AMQPStep {
	step.config.Headers[name] = value
	return step
}

func (step *AMQPStep) URL(url string) *AMQPStep {
	step.config.URL = url
	return step
}

func (step *AMQPStep) Persistent(persistent bool) *AMQPStep {
	step.config.Persistent = persistent
	return step
}

func (step *AMQPStep) Confirm(confirm bool) *AMQPStep {
	step.config.Confirm = confirm
	return step
}

func (step *AMQPStep) Timeout(timeout time.Duration) *AMQPStep {
	step.config.Timeout = timeout
	return step
}

func (step *AMQPStep) build() (string, protocol.Config, error) {
	if step.err != nil {
		return "", nil, step.err
	}
	if err := amqp.Validate(step.config); err != nil {
		return "", nil, err
	}
	return "amqp", step.config, nil
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

// WaitForHTTP exists for systems that only expose the effect over an API:
// without it the end-to-end chain cannot be measured on them.
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

// Key is required for the same reason it is in the YAML: waiting for any
// message would time the fastest consumer, not this iteration's journey.
func (step *WaitStep) Key(expected string) *WaitStep {
	step.config.Expected = expected
	return step
}

func (step *WaitStep) Field(field string) *WaitStep {
	step.config.Field = field
	return step
}

func (step *WaitStep) Addresses(addresses ...string) *WaitStep {
	step.config.Addresses = addresses
	return step
}

func (step *WaitStep) UntilJSON(path, value string) *WaitStep {
	step.config.To = wait.Condition{Path: path, Value: value}
	return step
}

func (step *WaitStep) UntilStatus(status int) *WaitStep {
	step.config.To = wait.Condition{Status: status}
	return step
}

func (step *WaitStep) UntilBodyContains(fragment string) *WaitStep {
	step.config.To = wait.Condition{BodyContains: fragment}
	return step
}

func (step *WaitStep) Interval(interval time.Duration) *WaitStep {
	step.config.Interval = interval
	return step
}

func (step *WaitStep) Timeout(timeout time.Duration) *WaitStep {
	step.config.Timeout = timeout
	return step
}

func (step *WaitStep) build() (string, protocol.Config, error) {
	if err := wait.Validate(step.config); err != nil {
		return "", nil, err
	}
	return "aguardar", step.config, nil
}
