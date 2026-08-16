package dsl_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/dsl"
	"github.com/Diegobraun/braunrate/internal/protocol"
	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"gopkg.in/yaml.v3"
)

type equivalencia struct {
	nome string
	yaml string
	dsl  func() (scenario.Cenario, error)
}

// O YAML e a DSL precisam produzir a MESMA estrutura, e nao apenas resultados
// parecidos: o cenario montado e o unico dado que o motor recebe, entao
// igualdade aqui e igualdade de medicao. So o numero da linha e ignorado, que e
// posicao no arquivo e nao existe em codigo Go.
var casos = []equivalencia{
	{
		nome: "http com variaveis, dados, autenticacao por token, capturas e slo",
		yaml: `
nome: Jornada autenticada
alvo: ${BASE:-http://127.0.0.1:8080}

variaveis:
  inquilino: acme

autenticacao:
  tipo: token
  renovar_apos: 25m
  obter:
    http:
      metodo: POST
      caminho: /auth/token
      corpo: { senha: segredo, usuario: ana }
    captura:
      token: $.access_token

dados:
  assinantes:
    arquivo: dados/assinantes.csv
    consumo: circular
  pedidos:
    gerar: { id: uuid, valor: "numero(10,500)" }
    semente: 7

carga:
  perfis:
    - rampa: { de: 10/s, ate: 100/s, durante: 30s }
    - patamar: { taxa: 100/s, durante: 1m }
    - pico: { taxa: 600/m, durante: 10s }
    - constante: { taxa: 36000/h, durante: 5s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    captura:
      faturaId: $.ultimaFatura.id
      requisicao: cabecalho:X-Request-Id
      sessao: /sessao=([a-z0-9]+)/
      codigo: status
      inteiro: corpo
      opcional: { de: $.talvez, padrao: vazio }
    verificar:
      status: 200
      corpo_contem: ABERTO
      corpo_casa: "pedido-[0-9]+"
      json: { $.status: ABERTO, $.total: "> 10", $.cupom: existe, $.tags: contem promo }
      cabecalho: { Content-Type: application/json }

  - http:
      metodo: POST
      caminho: /pedidos
      cabecalhos: { X-Inquilino: "${inquilino}" }
      corpo: { assinante: "${assinantes.id}", total: 199.9 }
      timeout: 2s
      seguir_redirect: false

slo:
  - consultar pedido: { p95: < 150ms, max: < 1s }
  - POST /pedidos: { vazao: "> 50/s" }
  - global: { erros: < 0.1 }
`,
		dsl: func() (scenario.Cenario, error) {
			return dsl.Novo("Jornada autenticada").
				Alvo("${BASE:-http://127.0.0.1:8080}").
				Variavel("inquilino", "acme").
				Autenticacao(dsl.PorToken(
					dsl.POST("/auth/token").Corpo(map[string]any{"usuario": "ana", "senha": "segredo"}),
					dsl.Capturar("token", "$.access_token"),
				).RenovarApos(25*time.Minute)).
				DadosDeArquivo("assinantes", "dados/assinantes.csv", dsl.Consumo(scenario.ConsumoCircular)).
				DadosGerados("pedidos", map[string]string{"id": "uuid", "valor": "numero(10,500)"}, dsl.Semente(7)).
				Rampa(dsl.PorSegundo(10), dsl.PorSegundo(100), 30*time.Second).
				Patamar(dsl.PorSegundo(100), time.Minute).
				Pico(dsl.PorMinuto(600), 10*time.Second).
				Constante(dsl.PorHora(36000), 5*time.Second).
				Passo(dsl.GET("/pedidos/${assinantes.id}"),
					dsl.Nome("consultar pedido"),
					dsl.Capturar("faturaId", "$.ultimaFatura.id"),
					dsl.Capturar("requisicao", "cabecalho:X-Request-Id"),
					dsl.Capturar("sessao", "/sessao=([a-z0-9]+)/"),
					dsl.Capturar("codigo", "status"),
					dsl.Capturar("inteiro", "corpo"),
					dsl.CapturarComPadrao("opcional", "$.talvez", "vazio"),
					dsl.VerificarStatus(200),
					dsl.VerificarCorpoContem("ABERTO"),
					dsl.VerificarCorpoCasa("pedido-[0-9]+"),
					dsl.VerificarJSON("$.status", "ABERTO"),
					dsl.VerificarJSON("$.total", "> 10"),
					dsl.VerificarJSON("$.cupom", "existe"),
					dsl.VerificarJSON("$.tags", "contem promo"),
					dsl.VerificarCabecalho("Content-Type", "application/json"),
				).
				Passo(dsl.POST("/pedidos").
					Cabecalho("X-Inquilino", "${inquilino}").
					Corpo(map[string]any{"assinante": "${assinantes.id}", "total": 199.9}).
					Timeout(2*time.Second).
					SeguirRedirect(false)).
				SLO("consultar pedido", "p95", "< 150ms").
				SLO("consultar pedido", "max", "< 1s").
				SLO("POST /pedidos", "vazao", "> 50/s").
				SLOGlobal("erros", "< 0.1").
				Construir()
		},
	},
	{
		nome: "graphql por operacao",
		yaml: `
nome: Cobranca em GraphQL
alvo: http://127.0.0.1:8080

carga:
  perfis:
    - patamar: { taxa: 50/s, durante: 20s }

cenario:
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }
      variaveis: { id: "${assinantes.id}" }
      caminho: /graphql
      cabecalhos: { X-Origem: braunrate }
      timeout: 3s
    verificar:
      json: { $.data.pedido.status: ABERTO }

  - graphql: |
      mutation PagarFatura($id: ID!) { pagarFatura(id: $id) { status } }

slo:
  - graphql ConsultarPedido: { p99: < 300ms }
`,
		dsl: func() (scenario.Cenario, error) {
			return dsl.Novo("Cobranca em GraphQL").
				Alvo("http://127.0.0.1:8080").
				Patamar(dsl.PorSegundo(50), 20*time.Second).
				Passo(dsl.GraphQL("query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }\n").
					Variaveis(map[string]any{"id": "${assinantes.id}"}).
					Caminho("/graphql").
					Cabecalho("X-Origem", "braunrate").
					Timeout(3*time.Second),
					dsl.VerificarJSON("$.data.pedido.status", "ABERTO")).
				Passo(dsl.GraphQL("mutation PagarFatura($id: ID!) { pagarFatura(id: $id) { status } }\n")).
				SLO("graphql ConsultarPedido", "p99", "< 300ms").
				Construir()
		},
	},
	{
		nome: "kafka com aguardar fechando a cadeia",
		yaml: `
nome: Cadeia assincrona
alvo: 127.0.0.1:9092

dados:
  pedidos:
    gerar: { id: uuid }
    consumo: sequencial

carga:
  perfis:
    - patamar: { taxa: 100/s, durante: 10s }

cenario:
  - kafka:
      topico: pedidos-cadeia
      chave: "${pedidos.id}"
      valor: { pedido: "${pedidos.id}" }
      cabecalhos: { origem: braunrate }
      brokers: [127.0.0.1:9092]
      acks: lider
      timeout: 5s

  - aguardar:
      kafka: { topico: pedidos-processados, brokers: [127.0.0.1:9092] }
      chave: "${pedidos.id}"
      campo: $.pedido
      timeout: 10s

  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }
      intervalo: 200ms
      timeout: 30s

slo:
  - kafka produzir pedidos-cadeia: { p95: < 100ms }
`,
		dsl: func() (scenario.Cenario, error) {
			return dsl.Novo("Cadeia assincrona").
				Alvo("127.0.0.1:9092").
				DadosGerados("pedidos", map[string]string{"id": "uuid"}, dsl.Consumo(scenario.ConsumoSequencial)).
				Patamar(dsl.PorSegundo(100), 10*time.Second).
				Passo(dsl.Kafka("pedidos-cadeia").
					Chave("${pedidos.id}").
					Valor(map[string]any{"pedido": "${pedidos.id}"}).
					Cabecalho("origem", "braunrate").
					Brokers("127.0.0.1:9092").
					Acks("lider").
					Timeout(5*time.Second)).
				Passo(dsl.AguardarKafka("pedidos-processados").
					Enderecos("127.0.0.1:9092").
					Chave("${pedidos.id}").
					Campo("$.pedido").
					Timeout(10*time.Second)).
				Passo(dsl.AguardarHTTP("/pedidos/${pedidos.id}").
					AteJSON("$.status", "PROCESSADO").
					Intervalo(200*time.Millisecond).
					Timeout(30*time.Second)).
				SLO("kafka produzir pedidos-cadeia", "p95", "< 100ms").
				Construir()
		},
	},
	{
		nome: "amqp em fila e em troca com rota",
		yaml: `
nome: Publicacao em RabbitMQ
alvo: amqp://127.0.0.1:5672

dados:
  clientes:
    arquivo: dados/clientes.csv
    consumo: aleatorio

carga:
  perfis:
    - patamar: { taxa: 30/s, durante: 10s }

cenario:
  - amqp:
      fila: pedidos
      identidade: "${clientes.id}"
      corpo: { cliente: "${clientes.id}" }
      cabecalhos: { origem: braunrate }
      url: amqp://127.0.0.1:5672
      persistente: false
      confirmar: false
      timeout: 4s

  - amqp:
      troca: cobranca
      rota: fatura.emitida
      corpo: texto puro

  - aguardar:
      amqp: pedidos-processados
      chave: "${clientes.id}"
`,
		dsl: func() (scenario.Cenario, error) {
			return dsl.Novo("Publicacao em RabbitMQ").
				Alvo("amqp://127.0.0.1:5672").
				DadosDeArquivo("clientes", "dados/clientes.csv", dsl.Consumo(scenario.ConsumoAleatorio)).
				Patamar(dsl.PorSegundo(30), 10*time.Second).
				Passo(dsl.AMQP("pedidos").
					Identidade("${clientes.id}").
					Corpo(map[string]any{"cliente": "${clientes.id}"}).
					Cabecalho("origem", "braunrate").
					URL("amqp://127.0.0.1:5672").
					Persistente(false).
					Confirmar(false).
					Timeout(4 * time.Second)).
				Passo(dsl.Troca("cobranca", "fatura.emitida").Corpo("texto puro")).
				Passo(dsl.AguardarAMQP("pedidos-processados").Chave("${clientes.id}")).
				Construir()
		},
	},
	{
		nome: "autenticacao basica e consumo unico por usuario",
		yaml: `
nome: Basica
alvo: http://127.0.0.1:8080

autenticacao:
  tipo: basica
  usuario: ana
  senha: segredo

dados:
  assinantes:
    arquivo: dados/assinantes.csv
    consumo: unico_por_usuario

carga:
  perfis:
    - patamar: { taxa: 10/s, durante: 5s }

cenario:
  - http: GET /pedidos
`,
		dsl: func() (scenario.Cenario, error) {
			return dsl.Novo("Basica").
				Alvo("http://127.0.0.1:8080").
				Autenticacao(dsl.Basica("ana", "segredo")).
				DadosDeArquivo("assinantes", "dados/assinantes.csv", dsl.Consumo(scenario.ConsumoUnicoPorUsuario)).
				Patamar(dsl.PorSegundo(10), 5*time.Second).
				Passo(dsl.GET("/pedidos")).
				Construir()
		},
	},
	{
		nome: "autenticacao por cabecalho fixo",
		yaml: `
nome: Chave de api
alvo: http://127.0.0.1:8080

autenticacao:
  tipo: cabecalho
  cabecalho: "X-API-Key: ${api_key}"

carga:
  perfis:
    - patamar: { taxa: 10/s, durante: 5s }

cenario:
  - http: DELETE /pedidos/1
`,
		dsl: func() (scenario.Cenario, error) {
			return dsl.Novo("Chave de api").
				Alvo("http://127.0.0.1:8080").
				Autenticacao(dsl.PorCabecalho("X-API-Key: ${api_key}")).
				Patamar(dsl.PorSegundo(10), 5*time.Second).
				Passo(dsl.DELETE("/pedidos/1")).
				Construir()
		},
	},
}

func TestYAMLEDSLProduzemOMesmoCenario(t *testing.T) {
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			doYAML, err := scenario.Carregar([]byte(caso.yaml))
			if err != nil {
				t.Fatalf("o YAML do caso nao carregou: %v", err)
			}
			if err := doYAML.Validar(); err != nil {
				t.Fatalf("o YAML do caso nao e valido: %v", err)
			}
			daDSL, err := caso.dsl()
			if err != nil {
				t.Fatalf("a DSL do caso nao montou: %v", err)
			}

			esperado, obtido := semLinhas(doYAML), semLinhas(daDSL)
			if reflect.DeepEqual(esperado, obtido) {
				return
			}
			for _, diferenca := range diferencas(esperado, obtido) {
				t.Errorf("%s", diferenca)
			}
		})
	}
}

// Sem esta trava, um protocolo novo nasceria com equivalencia nao verificada e a
// promessa dos dois publicos ficaria valendo so para o que ja existia.
func TestTodoProtocoloRegistradoTemCasoDeEquivalencia(t *testing.T) {
	exercitados := map[string]bool{}
	for _, caso := range casos {
		montado, err := caso.dsl()
		if err != nil {
			t.Fatalf("%s: %v", caso.nome, err)
		}
		for _, passo := range montado.Passos {
			exercitados[passo.Protocolo] = true
		}
	}
	for _, nome := range protocol.Registrados() {
		if !exercitados[nome] {
			t.Errorf("o protocolo %q nao tem caso de equivalencia YAML x DSL", nome)
		}
	}
}

func TestTodaChaveDeTopoTemCasoDeEquivalencia(t *testing.T) {
	usadas := map[string]bool{}
	for _, caso := range casos {
		var documento map[string]any
		if err := yaml.Unmarshal([]byte(caso.yaml), &documento); err != nil {
			t.Fatalf("%s: %v", caso.nome, err)
		}
		for chave := range documento {
			usadas[chave] = true
		}
	}
	for _, chave := range scenario.ChavesDeTopo {
		if !usadas[chave] {
			t.Errorf("a chave de topo %q nao aparece em nenhum caso de equivalencia", chave)
		}
	}
}

func TestTodaFormaDeCenarioTemCasoDeEquivalencia(t *testing.T) {
	origens := map[scenario.OrigemDaCaptura]bool{}
	assercoes := map[scenario.TipoDeAssercao]bool{}
	fases := map[scenario.TipoDeFase]bool{}
	autenticacoes := map[scenario.TipoDeAutenticacao]bool{}
	consumos := map[scenario.PoliticaDeConsumo]bool{}
	metricas := map[string]bool{}
	operadores := map[scenario.Operador]bool{}

	for _, caso := range casos {
		montado, err := caso.dsl()
		if err != nil {
			t.Fatalf("%s: %v", caso.nome, err)
		}
		for _, passo := range montado.Passos {
			for _, captura := range passo.Capturas {
				origens[captura.Origem] = true
			}
			for _, assercao := range passo.Assercoes {
				assercoes[assercao.Tipo] = true
				operadores[assercao.Operador] = true
			}
			if len(passo.Verificacoes) > 0 {
				assercoes[scenario.TipoDeAssercao(scenario.VerificarStatus)] = true
			}
		}
		for _, fase := range montado.Carga.Fases {
			fases[fase.Tipo] = true
		}
		if montado.Autenticacao != nil {
			autenticacoes[montado.Autenticacao.Tipo] = true
		}
		for _, fonte := range montado.Dados {
			consumos[fonte.Consumo] = true
		}
		for _, regra := range montado.SLO {
			metricas[regra.Metrica] = true
		}
	}

	faltando(t, "origem de captura", []scenario.OrigemDaCaptura{
		scenario.CapturaDeJSON, scenario.CapturaDeCabecalho, scenario.CapturaDeRegex,
		scenario.CapturaDeCorpo, scenario.CapturaDeStatus,
	}, origens)
	faltando(t, "tipo de assercao", []scenario.TipoDeAssercao{
		scenario.AsserirCorpoContem, scenario.AsserirJSON, scenario.AsserirRegex,
		scenario.AsserirCabecalho, scenario.TipoDeAssercao(scenario.VerificarStatus),
	}, assercoes)
	faltando(t, "tipo de fase", []scenario.TipoDeFase{
		scenario.FaseRampa, scenario.FasePatamar, scenario.FasePico, scenario.FaseConstante,
	}, fases)
	faltando(t, "tipo de autenticacao", []scenario.TipoDeAutenticacao{
		scenario.AutenticacaoPorToken, scenario.AutenticacaoBasica, scenario.AutenticacaoCabecalho,
	}, autenticacoes)
	faltando(t, "politica de consumo", []scenario.PoliticaDeConsumo{
		scenario.ConsumoCircular, scenario.ConsumoSequencial, scenario.ConsumoAleatorio, scenario.ConsumoUnicoPorUsuario,
	}, consumos)
	faltando(t, "metrica de slo", []string{"p95", "p99", "max", "erros", "vazao"}, metricas)
	faltando(t, "operador de comparacao", []scenario.Operador{
		scenario.OperadorIgual, scenario.OperadorMaior, scenario.OperadorExiste, scenario.OperadorContem,
	}, operadores)
}

func faltando[T comparable](t *testing.T, assunto string, esperados []T, vistos map[T]bool) {
	t.Helper()
	for _, esperado := range esperados {
		if !vistos[esperado] {
			t.Errorf("%s %v nao aparece em nenhum caso de equivalencia YAML x DSL", assunto, esperado)
		}
	}
}

func semLinhas(c scenario.Cenario) scenario.Cenario {
	copia := c
	copia.Passos = nil
	for _, passo := range c.Passos {
		copia.Passos = append(copia.Passos, passoSemLinhas(passo))
	}
	copia.Dados = nil
	for _, fonte := range c.Dados {
		fonte.Linha = 0
		copia.Dados = append(copia.Dados, fonte)
	}
	copia.SLO = nil
	for _, regra := range c.SLO {
		regra.Linha = 0
		copia.SLO = append(copia.SLO, regra)
	}
	copia.Carga.Fases = nil
	for _, fase := range c.Carga.Fases {
		fase.Linha = 0
		copia.Carga.Fases = append(copia.Carga.Fases, fase)
	}
	if c.Autenticacao != nil {
		autenticacao := *c.Autenticacao
		autenticacao.Linha = 0
		if autenticacao.Obter != nil {
			obter := passoSemLinhas(*autenticacao.Obter)
			autenticacao.Obter = &obter
		}
		copia.Autenticacao = &autenticacao
	}
	return copia
}

func passoSemLinhas(passo scenario.Passo) scenario.Passo {
	copia := passo
	copia.Linha = 0
	copia.Capturas = nil
	for _, captura := range passo.Capturas {
		captura.Linha = 0
		copia.Capturas = append(copia.Capturas, captura)
	}
	copia.Assercoes = nil
	for _, assercao := range passo.Assercoes {
		assercao.Linha = 0
		copia.Assercoes = append(copia.Assercoes, assercao)
	}
	return copia
}

func diferencas(esperado, obtido scenario.Cenario) []string {
	var achados []string
	comparar := func(campo string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			achados = append(achados, campo+":\n  yaml: "+formatar(a)+"\n   dsl: "+formatar(b))
		}
	}
	comparar("nome", esperado.Nome, obtido.Nome)
	comparar("alvo", esperado.Alvo, obtido.Alvo)
	comparar("variaveis", esperado.Variaveis, obtido.Variaveis)
	comparar("autenticacao", esperado.Autenticacao, obtido.Autenticacao)
	comparar("dados", esperado.Dados, obtido.Dados)
	comparar("carga", esperado.Carga, obtido.Carga)
	comparar("slo", esperado.SLO, obtido.SLO)
	if len(esperado.Passos) != len(obtido.Passos) {
		comparar("quantidade de passos", len(esperado.Passos), len(obtido.Passos))
		return achados
	}
	for indice := range esperado.Passos {
		comparar(formatar(indice)+" passo", esperado.Passos[indice], obtido.Passos[indice])
	}
	return achados
}

func formatar(valor any) string {
	texto, err := yaml.Marshal(valor)
	if err != nil {
		return "<nao formatavel>"
	}
	return string(texto)
}
