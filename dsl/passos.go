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
func serializar(corpo any) ([]byte, string, error) {
	switch valor := corpo.(type) {
	case nil:
		return nil, "", nil
	case string:
		return []byte(valor), "text/plain", nil
	case []byte:
		return valor, "text/plain", nil
	default:
		conteudo, err := json.Marshal(valor)
		if err != nil {
			return nil, "", fmt.Errorf("corpo nao serializa para JSON: %v", err)
		}
		return conteudo, "application/json", nil
	}
}

type PassoHTTP struct {
	configuracao *protocoloHTTP.Configuracao
	erro         error
}

func HTTP(metodo, caminho string) *PassoHTTP {
	configuracao := protocoloHTTP.Padrao()
	configuracao.Metodo = strings.ToUpper(metodo)
	configuracao.Caminho = caminho
	return &PassoHTTP{configuracao: configuracao}
}

func GET(caminho string) *PassoHTTP    { return HTTP(http.MethodGet, caminho) }
func POST(caminho string) *PassoHTTP   { return HTTP(http.MethodPost, caminho) }
func PUT(caminho string) *PassoHTTP    { return HTTP(http.MethodPut, caminho) }
func PATCH(caminho string) *PassoHTTP  { return HTTP(http.MethodPatch, caminho) }
func DELETE(caminho string) *PassoHTTP { return HTTP(http.MethodDelete, caminho) }

func (p *PassoHTTP) Cabecalho(nome, valor string) *PassoHTTP {
	p.configuracao.Cabecalhos[nome] = valor
	return p
}

func (p *PassoHTTP) Corpo(corpo any) *PassoHTTP {
	conteudo, tipo, err := serializar(corpo)
	if err != nil {
		p.erro = err
		return p
	}
	p.configuracao.Corpo = conteudo
	p.configuracao.TipoDeConteudo = tipo
	return p
}

func (p *PassoHTTP) Timeout(timeout time.Duration) *PassoHTTP {
	p.configuracao.Timeout = timeout
	return p
}

func (p *PassoHTTP) SeguirRedirect(seguir bool) *PassoHTTP {
	p.configuracao.SeguirRedirect = &seguir
	return p
}

func (p *PassoHTTP) montar() (string, protocol.Configuracao, error) {
	if p.erro != nil {
		return "", nil, p.erro
	}
	if err := protocoloHTTP.Validar(p.configuracao); err != nil {
		return "", nil, err
	}
	return "http", p.configuracao, nil
}

type PassoGraphQL struct {
	configuracao *graphql.Configuracao
	erro         error
}

func GraphQL(consulta string) *PassoGraphQL {
	configuracao := graphql.Padrao()
	configuracao.Consulta = consulta
	return &PassoGraphQL{configuracao: configuracao}
}

func (p *PassoGraphQL) Operacao(nome string) *PassoGraphQL {
	p.configuracao.Operacao = nome
	return p
}

func (p *PassoGraphQL) Variaveis(variaveis any) *PassoGraphQL {
	conteudo, err := json.Marshal(variaveis)
	if err != nil {
		p.erro = fmt.Errorf("variaveis de graphql nao serializam para JSON: %v", err)
		return p
	}
	p.configuracao.Variaveis = string(conteudo)
	return p
}

func (p *PassoGraphQL) Caminho(caminho string) *PassoGraphQL {
	p.configuracao.Caminho = caminho
	return p
}

func (p *PassoGraphQL) Cabecalho(nome, valor string) *PassoGraphQL {
	p.configuracao.Cabecalhos[nome] = valor
	return p
}

func (p *PassoGraphQL) Timeout(timeout time.Duration) *PassoGraphQL {
	p.configuracao.Timeout = timeout
	return p
}

func (p *PassoGraphQL) montar() (string, protocol.Configuracao, error) {
	if p.erro != nil {
		return "", nil, p.erro
	}
	configuracao, err := graphql.Finalizar(p.configuracao)
	if err != nil {
		return "", nil, err
	}
	return "graphql", configuracao, nil
}

type PassoKafka struct {
	configuracao *kafka.Configuracao
	erro         error
}

func Kafka(topico string) *PassoKafka {
	configuracao := kafka.Padrao()
	configuracao.Topico = topico
	return &PassoKafka{configuracao: configuracao}
}

func (p *PassoKafka) Chave(chave string) *PassoKafka {
	p.configuracao.Chave = chave
	return p
}

func (p *PassoKafka) Valor(valor any) *PassoKafka {
	conteudo, _, err := serializar(valor)
	if err != nil {
		p.erro = err
		return p
	}
	p.configuracao.Valor = conteudo
	return p
}

func (p *PassoKafka) Cabecalho(nome, valor string) *PassoKafka {
	p.configuracao.Cabecalhos[nome] = valor
	return p
}

func (p *PassoKafka) Brokers(brokers ...string) *PassoKafka {
	p.configuracao.Brokers = brokers
	return p
}

func (p *PassoKafka) Acks(acks string) *PassoKafka {
	switch acks {
	case "todos", "lider", "nenhum":
		p.configuracao.Acks = acks
	default:
		p.erro = fmt.Errorf("acks desconhecido: %q (use todos, lider ou nenhum)", acks)
	}
	return p
}

func (p *PassoKafka) Timeout(timeout time.Duration) *PassoKafka {
	p.configuracao.Timeout = timeout
	return p
}

func (p *PassoKafka) montar() (string, protocol.Configuracao, error) {
	if p.erro != nil {
		return "", nil, p.erro
	}
	if err := kafka.Validar(p.configuracao); err != nil {
		return "", nil, err
	}
	return "kafka", p.configuracao, nil
}

type PassoAMQP struct {
	configuracao *amqp.Configuracao
	erro         error
}

func AMQP(fila string) *PassoAMQP {
	configuracao := amqp.Padrao()
	configuracao.Fila = fila
	configuracao.Rota = fila
	return &PassoAMQP{configuracao: configuracao}
}

func Troca(troca, rota string) *PassoAMQP {
	configuracao := amqp.Padrao()
	configuracao.Troca = troca
	configuracao.Rota = rota
	return &PassoAMQP{configuracao: configuracao}
}

func (p *PassoAMQP) Corpo(corpo any) *PassoAMQP {
	conteudo, _, err := serializar(corpo)
	if err != nil {
		p.erro = err
		return p
	}
	p.configuracao.Corpo = conteudo
	return p
}

func (p *PassoAMQP) Identidade(identidade string) *PassoAMQP {
	p.configuracao.Identidade = identidade
	return p
}

func (p *PassoAMQP) Cabecalho(nome, valor string) *PassoAMQP {
	p.configuracao.Cabecalhos[nome] = valor
	return p
}

func (p *PassoAMQP) URL(url string) *PassoAMQP {
	p.configuracao.URL = url
	return p
}

func (p *PassoAMQP) Persistente(persistente bool) *PassoAMQP {
	p.configuracao.Persistente = persistente
	return p
}

func (p *PassoAMQP) Confirmar(confirmar bool) *PassoAMQP {
	p.configuracao.Confirmar = confirmar
	return p
}

func (p *PassoAMQP) Timeout(timeout time.Duration) *PassoAMQP {
	p.configuracao.Timeout = timeout
	return p
}

func (p *PassoAMQP) montar() (string, protocol.Configuracao, error) {
	if p.erro != nil {
		return "", nil, p.erro
	}
	if err := amqp.Validar(p.configuracao); err != nil {
		return "", nil, err
	}
	return "amqp", p.configuracao, nil
}

type PassoAguardar struct {
	configuracao *wait.Configuracao
}

func AguardarKafka(topico string) *PassoAguardar {
	configuracao := wait.Padrao()
	configuracao.Fonte = "kafka"
	configuracao.Topico = topico
	return &PassoAguardar{configuracao: configuracao}
}

// Espera por HTTP existe para o sistema que so mostra o efeito por API: sem
// isto, a cadeia ponta a ponta nao se mede nele.
func AguardarHTTP(caminho string) *PassoAguardar {
	configuracao := wait.Padrao()
	configuracao.Fonte = "http"
	configuracao.Caminho = caminho
	return &PassoAguardar{configuracao: configuracao}
}

func AguardarAMQP(fila string) *PassoAguardar {
	configuracao := wait.Padrao()
	configuracao.Fonte = "amqp"
	configuracao.Topico = fila
	return &PassoAguardar{configuracao: configuracao}
}

// A correlacao e obrigatoria pelo mesmo motivo do YAML: esperar qualquer
// mensagem mediria o consumidor mais rapido, e nao a jornada desta iteracao.
func (p *PassoAguardar) Chave(esperado string) *PassoAguardar {
	p.configuracao.Esperado = esperado
	return p
}

func (p *PassoAguardar) Campo(campo string) *PassoAguardar {
	p.configuracao.Campo = campo
	return p
}

func (p *PassoAguardar) Enderecos(enderecos ...string) *PassoAguardar {
	p.configuracao.Enderecos = enderecos
	return p
}

func (p *PassoAguardar) AteJSON(caminho, valor string) *PassoAguardar {
	p.configuracao.Ate = wait.Condicao{Caminho: caminho, Valor: valor}
	return p
}

func (p *PassoAguardar) AteStatus(status int) *PassoAguardar {
	p.configuracao.Ate = wait.Condicao{Status: status}
	return p
}

func (p *PassoAguardar) AteCorpoContem(trecho string) *PassoAguardar {
	p.configuracao.Ate = wait.Condicao{CorpoContem: trecho}
	return p
}

func (p *PassoAguardar) Intervalo(intervalo time.Duration) *PassoAguardar {
	p.configuracao.Intervalo = intervalo
	return p
}

func (p *PassoAguardar) Timeout(timeout time.Duration) *PassoAguardar {
	p.configuracao.Timeout = timeout
	return p
}

func (p *PassoAguardar) montar() (string, protocol.Configuracao, error) {
	if err := wait.Validar(p.configuracao); err != nil {
		return "", nil, err
	}
	return "aguardar", p.configuracao, nil
}
