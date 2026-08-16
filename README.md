# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR histogram e deteccao de back-pressure.

## Demonstracao de honestidade de medicao

O alvo de teste embutido congela por 1 s no meio da execucao. Mesma pausa, mesmo alvo, dois modelos de medicao:

| Modelo | p99 reportado | Amostras |
|---|---|---|
| **braunrate (chegada aberta, latencia contada do instante agendado)** | **976,4 ms** | 600 |
| Laco fechado (um usuario virtual em sequencia, como JMeter e Locust medem) | 3,3 ms | 793 |

**973,1 ms escondidos pelo laco fechado.** O laco fechado nao mente por bug: quando o alvo trava, ele simplesmente para de enviar, e as requisicoes que deveriam ter partido nunca entram na conta. E a omissao coordenada.

Isso nao e alegacao de marketing: e um teste automatizado que roda no CI a cada push. Se a medicao mentir, o build quebra.

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestMeasurementReflectsTargetFreeze
    modelo aberto: p50 2.7 ms | p99 976.9 ms | max 1005.6 ms | n 600
--- PASS: TestMeasurementReflectsTargetFreeze (3.01s)
=== RUN   TestTargetFreezeIsNotConfusedWithGeneratorSaturation
    aviso correto: a latencia do alvo cresceu ao longo da execucao enquanto o despacho
    continuou pontual; a degradacao e do alvo, nao do gerador | p99 por segundo passou
    de 3.2 ms para 996.9 ms
--- PASS: TestTargetFreezeIsNotConfusedWithGeneratorSaturation (3.01s)
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 976.4 ms sobre 600 amostras
      laco fechado:              p99 3.3 ms sobre 793 amostras
      omissao coordenada: 973.1 ms escondidos pelo laco fechado
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

Reproduza na sua maquina: `go test ./internal/selfcheck/... -v`.

### Tres provas, tres pontos cegos

Esta e a primeira das tres execucoes reais que sustentam a tese. Cada uma expoe um ponto cego diferente, e nenhuma delas e opiniao:

| Prova | Numero | Ponto cego que expoe |
|---|---|---|
| Alvo congelado por 1 s (acima) | 976,4 ms contra 3,3 ms | **Omissao coordenada**: laco fechado para de enviar quando o alvo trava, e a espera some da conta |
| [GraphQL com erro em 200](#graphql) | 406 erros em 2.844 respostas, todas com status 200 | **Erro classificado por status**: quem le o codigo HTTP reporta 0% de erro e SLO verde |
| [Cadeia assincrona](#mensageria-e-cadeia-assincrona) | 0,9 ms para produzir contra 4,87 s de jornada | **Medir so a producao**: o broker aceita rapido, e o efeito que o usuario espera chega segundos depois |

## Estado

**Fase 6 concluida** — motor de chegada aberta, HTTP, GraphQL, Kafka, RabbitMQ e passo `aguardar`, correlacao, autenticacao, dados, assercoes, SLO com codigo de saida, ferramentas de autoria (schema no editor, `depurar`, `importar curl` e `importar jmx`), relatorio (HTML autocontido, JSON, CSV, comparacao entre execucoes), variedade observada e **cenario em Go equivalente ao YAML, travado por teste**.

Decisao da Fase 0: **Go**, sustentada por dois criterios apenas — RSS sob carga (30 MB contra 597 MB do Java com G1, a 10.000/s) e binario unico estatico, que para o publico de QA significa instalar baixando um arquivo. Startup, precisao de agendamento e modo de falha apareceram na primeira analise com peso que nao aguentam, e estao marcados como nao-criterio no ADR. Numeros, metodologia e limites em [medicoes-fase0.md](docs/medicoes-fase0.md); a decisao com os pesos de cada criterio em [ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Como usar

```bash
go build -o braunrate ./cmd/braunrate

braunrate alvo -latencia=5ms &                     # alvo de teste embutido
braunrate validar examples/http-basico.yaml        # valida sem executar
braunrate depurar examples/http-basico.yaml        # uma iteracao, tudo visivel
braunrate executar examples/http-basico.yaml       # executa e resume no terminal
braunrate executar examples/http-basico.yaml -html=relatorio.html -resultado=saida.json
braunrate comparar antes.json depois.json          # o que mudou entre duas execucoes
```

Cenario minimo:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
nome: Consulta de pedidos
alvo: http://127.0.0.1:8080

carga:
  modelo: aberto
  perfis:
    - rampa: { de: 100/s, ate: 800/s, durante: 5s }
    - patamar: { taxa: 800/s, durante: 10s }
    - pico: { taxa: 2000/s, durante: 3s }

cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }
```

Codigo de saida: `0` passou, `1` **falhou o SLO**, `2` erro de cenario, `3` **resultado invalido** — o gerador saturou e o numero nao vale.

Cenario com autenticacao, correlacao, dados e SLO — o exemplo completo esta em [`examples/jornada-autenticada.yaml`](examples/jornada-autenticada.yaml):

```yaml
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: "${usuario}", senha: "${SENHA}" } }
    captura: { token: $.access_token }
  renovar_apos: 25m

dados:
  assinantes: { arquivo: dados/assinantes.csv, consumo: circular }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    verificar: { status: 200, json: { $.ultimaFatura.status: ABERTA } }
    captura: { faturaId: $.ultimaFatura.id }

slo:
  - consultar pedido: { p95: < 150ms }
  - global: { erros: < 0.1 }
```

Saida real dessa execucao:

```
Jornada de cobranca — contra http://127.0.0.1:8080

Passou: as 3 regras de SLO foram atendidas.

O que aconteceu
  4.750 requisicoes em 10s, 475 por segundo, 0% de erro
  Metade das respostas em ate 5.4 ms; 95% em ate 5.8 ms; 99% em ate 6.2 ms; a pior levou 14 ms

A jornada inteira
  Todas as 2375 jornadas chegaram ao fim; metade levou ate 11 ms e 95% ate 12 ms, contados do instante em que deveriam ter comecado.
  metade 11 ms | 95% 12 ms | 99% 12 ms | pior 20 ms

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)      2.375    5.4 ms    5.9 ms    6.3 ms     13 ms     14 ms       0
  pagar fatura               (2)      2.375    5.3 ms    5.8 ms    6.1 ms    6.9 ms     12 ms       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.
  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como
      esse passo depende do valor capturado antes dele, nao existe instante
      agendado proprio. Para a leitura honesta da jornada, use "A jornada inteira".

SLO
  ok    Passou: "consultar pedido" teve latencia p95 de 6 ms, dentro do limite de 150 ms.
  ok    Passou: "pagar fatura" teve latencia p95 de 6 ms, dentro do limite de 200 ms.
  ok    Passou: o cenario inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.

Confiabilidade da medicao
  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.
  Atraso tipico para disparar: 0.001 ms; pior caso: 5.4 ms (o tempo de resposta ja desconta isso)

Ambiente
  Mac darwin/arm64, 10 nucleos | braunrate 0.4.0 | 2026-08-16 00:54:30
  4 valores distintos de assinantes.id em 2.375 usos
  4 valores distintos de faturaId em 2.375 usos
  1 unico valor de token em 2.375 usos
  Autenticacao obtida 1 vez(es) e reaproveitada por todas as jornadas.
  Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.
```

## Escrever um cenario

**Autocompletar no editor.** Todo exemplo comeca com esta linha, e ela vale para o seu arquivo tambem:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
```

Com a extensao YAML do VS Code (ou qualquer editor com yaml-language-server), o editor passa a completar as chaves, mostrar a explicacao de cada uma e marcar erro antes de rodar. O [schema](docs/braunrate.schema.json) tem teste que quebra o build se ele oferecer chave que o parser recusa, ou esquecer chave que o parser aceita.

**Comecar de um curl** em vez de comecar do zero:

```bash
braunrate importar curl "curl 'https://api.exemplo.com/v1/pedidos/9912' -X POST -H 'Authorization: Bearer abc.def' -d '{\"valor\": 199.90}'" -saida cenario.yaml
```

Sai um cenario que ja carrega, com carga e SLO de partida, e tres avisos honestos no terminal: o token virou variavel (`${token}`, lida de `TOKEN` no ambiente) e nao vai para o repositorio; o id fixo no caminho faz o alvo responder de cache; os numeros de carga e SLO sao chute, nao medicao.

**Comecar de um plano do JMeter**, para quem tem suite pronta:

```bash
braunrate importar jmx plano.jmx -saida cenario.yaml
```

**A traducao e parcial e o que ficou de fora sai listado no terminal**, um elemento por vez, porque importador que engole o arquivo inteiro em silencio entrega um cenario que mede outra coisa:

| Traduzido | Nao traduzido (sai declarado) |
|---|---|
| `HTTPSamplerProxy` (metodo, caminho, dominio, corpo) | Controladores (If, While, Loop), temporizadores |
| `HeaderManager`, com credencial virando variavel de ambiente | Scripts JSR223/BeanShell |
| `CSVDataSet` (arquivo e reciclagem) | Samplers de JDBC, JMS e outros nao-HTTP |
| `ThreadGroup`, como **aviso**, nunca como taxa | Assercoes: todo passo sai com `status: 200` |
| `JSONPostProcessor` e `RegexExtractor`, como instrucao de captura | Funcoes `${__...}` do JMeter |

**Thread nunca vira taxa.** No JMeter uma thread so envia depois que a resposta anterior chegou: 50 threads sao 50/s se o alvo responde em 1 s e 5/s se responde em 10 s. Converter em silencio importaria a omissao coordenada junto com o plano, entao o bloco `carga` sai como chute declarado e o aviso diz o que o `.jmx` tinha:

```
atencao: o grupo "Usuarios" declara 50 threads, rampa de 30s, 300s de duracao: numero de
thread nao vira taxa de chegada, porque thread so envia depois da resposta anterior. O bloco
'carga' ficou com um chute; troque pela taxa que voce quer sustentar (requisicoes por segundo)
atencao: o .jmx captura "faturaId" de "$.ultimaFatura.id": declare no passo que produz o valor,
como captura: { faturaId: $.ultimaFatura.id }
atencao: 1 elemento(s) do .jmx nao foram traduzidos e ficaram de fora do cenario:
BeanShellPreProcessor (1). Confira se algum deles mudava o que era medido
```

**Ver a iteracao antes da carga**, que e onde a correlacao quebrada aparece:

```
$ braunrate depurar examples/jornada-autenticada.yaml
depurando "Jornada de cobranca" contra http://127.0.0.1:8080: 1 usuario, 1 iteracao, sem carga

passo 1 — consultar pedido   [ok em 3.4ms]
  requisicao: GET /pedidos/1001
              Authorization: Bearer token-… (14 caracteres)
  resposta:   status 200, 95 bytes
  corpo:      {"id":"1001","status":"ABERTO","ultimaFatura":{"id":"f-1001","valor":199.90,"status":"ABERTA"}}
  capturou:
    faturaId = f-1001

passo 2 — pagar fatura   [ok em 3.7ms]
  requisicao: POST /faturas/f-1001/pagar
              Authorization: Bearer token-… (14 caracteres)
              Content-Type: application/json
              corpo: {"valor":199.9}
  resposta:   status 200, 63 bytes

variaveis no fim da iteracao
  assinantes.id = 1001

Iteracao completa: 2 passo(s), tudo certo. Para rodar com carga:
  braunrate executar examples/jornada-autenticada.yaml
```

## O mesmo cenario em Go

Quando o cenario passa do que o YAML expressa — laco sobre uma lista, decisao no meio da jornada, dado vindo de um sistema seu — o mesmo cenario se escreve em Go, com o mesmo motor e as mesmas metricas:

```go
c, err := dsl.Novo("Jornada autenticada").
	Alvo("https://api.exemplo.com").
	Autenticacao(dsl.PorToken(
		dsl.POST("/auth/token").Corpo(map[string]any{"usuario": "ana", "senha": "${SENHA}"}),
		dsl.Capturar("token", "$.access_token"),
	).RenovarApos(25 * time.Minute)).
	DadosDeArquivo("assinantes", "dados/assinantes.csv").
	Rampa(dsl.PorSegundo(10), dsl.PorSegundo(100), 30*time.Second).
	Patamar(dsl.PorSegundo(100), time.Minute).
	Passo(dsl.GET("/pedidos/${assinantes.id}"),
		dsl.Nome("consultar pedido"),
		dsl.VerificarStatus(200),
		dsl.Capturar("faturaId", "$.ultimaFatura.id")).
	SLO("consultar pedido", "p95", "< 150ms").
	SLOGlobal("erros", "< 0.1").
	Construir()

m, err := motor.Novo(c, motor.OpcoesPadrao())
documento := m.Executar(context.Background())
```

**Migrar de YAML para Go nao e reescrever.** A DSL nao interpreta nada por conta propria: `"$.ultimaFatura.id"`, `"> 10"` e `"< 150ms"` sao lidos pelas mesmas funcoes que leem o YAML, e cada protocolo aplica seus padroes num lugar so. Um teste compara a estrutura inteira dos dois caminhos, caso a caso, e falha se um protocolo registrado, uma chave de topo ou uma forma de cenario ficar sem caso de equivalencia:

```
$ go test ./dsl/ -run TestYAMLEDSL -v
--- PASS: TestYAMLEDSLProduzemOMesmoCenario (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/http_com_variaveis,_dados,_autenticacao_por_token,_capturas_e_slo (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/graphql_por_operacao (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/kafka_com_aguardar_fechando_a_cadeia (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/amqp_em_fila_e_em_troca_com_rota (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/autenticacao_basica_e_consumo_unico_por_usuario (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/autenticacao_por_cabecalho_fixo (0.00s)
```

## O relatorio

```bash
braunrate executar examples/jornada-autenticada.yaml -html=relatorio.html -resultado=saida.json -csv=passos.csv
braunrate relatorio saida.json -html=relatorio.html     # gera depois, a partir do resultado gravado
braunrate comparar ontem.json hoje.json                 # o que mudou entre duas execucoes
```

O topo do HTML e uma frase, nao uma tabela. [Exemplo real](docs/exemplo-relatorio.html) (baixe e abra: o GitHub nao renderiza HTML), gerado da execucao abaixo. O arquivo nao busca script, fonte nem imagem: abre em rede fechada e sobrevive anexado em ticket.

## Como ler o numero (e onde ele e otimista)

Duas coisas mudam a leitura do relatorio, e as duas estao impressas na saida em vez de escondidas na documentacao.

**Latencia do passo 2 em diante nao e corrigida.** So o primeiro passo tem instante agendado proprio; os seguintes dependem de um valor capturado antes deles e por isso comecam quando o passo anterior termina. Esta execucao real, contra o alvo embutido congelado por 1 s no meio, mostra o tamanho do problema:

```
Falhou: "consultar pedido" teve latencia p95 de 598 ms, acima do limite de 150 ms.

A jornada inteira
  Todas as 2375 jornadas chegaram ao fim; metade levou ate 81 ms e 95% ate 675 ms, contados do instante em que deveriam ter comecado.
  metade 81 ms | 95% 675 ms | 99% 1.03 s | pior 1.09 s

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)      2.375     40 ms    598 ms    954 ms    1.03 s    1.04 s       0
  pagar fatura               (2)      2.375     40 ms     43 ms     43 ms    1.04 s    1.04 s       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.
  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como
      esse passo depende do valor capturado antes dele, nao existe instante
      agendado proprio. Para a leitura honesta da jornada, use "A jornada inteira".

Confiabilidade da medicao
  Atencao: a latencia do alvo cresceu ao longo da execucao enquanto o despacho continuou pontual; a degradacao e do alvo, nao do gerador
            p99 por segundo passou de 64.1 ms para 1039.4 ms
```

Repare no `pagar fatura`: **43 ms no 95%**, com o alvo congelado por um segundo inteiro no meio da execucao. Sozinho, esse numero e o mesmo tipo de mentira que uma ferramenta de laco fechado produz. A jornada inteira, contada do instante agendado, mostra **675 ms** — e essa e a leitura que vale para quem usa o sistema.

**Um token para a execucao inteira.** Hoje o motor faz login uma vez e reaproveita a credencial em todas as jornadas — isso nao existe em producao. Se o alvo tiver cache por identidade, rate limit por token ou sharding por usuario, o numero fica otimista (ou, no caso do rate limit, falha por 429 que nao aconteceria). O relatorio declara isso em toda execucao com autenticacao. `pool de tokens` e `token por usuario virtual` sao evolucao prevista, com a forma do YAML ja desenhada no [ADR 0005](docs/adr/0005-identidade-e-token.md).

## GraphQL

Cole a consulta; o nome da operacao vira a linha do relatorio:

```yaml
cenario:
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) {
          pedido(id: $id) { id status ultimaFatura { id status } }
        }
      variaveis: { id: "${assinantes.id}" }
    verificar:
      json: { $.data.pedido.status: ABERTO }
    captura:
      faturaId: $.data.pedido.ultimaFatura.id

slo:
  - graphql ConsultarPedido: { p95: < 150ms }
```

Duas coisas que uma ferramenta de HTTP generico erra em GraphQL, e que aqui sao o padrao:

**A operacao e a unidade de medida.** Tudo em GraphQL chega em `POST /graphql`. Agregar por URL colocaria a consulta mais barata e a mutation mais cara na mesma linha, e o p99 da mais cara sumiria na media. Por isso a chave e `graphql ConsultarPedido`, e operacao anonima e recusada na leitura do cenario — com a mensagem mostrando como dar nome.

**Erro com status 200 e erro.** A especificacao manda responder `200` com `errors` no corpo. Execucao real contra o alvo embutido, onde um quarto dos assinantes nao existe:

```
Falhou: o cenario inteiro teve taxa de erro de 14.28%, acima do limite de 0.10%.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  graphql ConsultarPedido    (1)      1.625    4.7 ms    5.1 ms    5.4 ms    5.8 ms     14 ms     406
  graphql PagarFatura        (2)      1.219    4.7 ms    5.0 ms    5.2 ms    5.8 ms    6.0 ms       0

Erros
  erro no corpo da resposta GraphQL (com status 200)  406
```

**Todas as 2.844 respostas vieram com status HTTP 200.** Uma ferramenta que classifica por status teria reportado 0% de erro e SLO verde. Resposta parcial (`data` e `errors` juntos) tambem conta como erro, e o detalhe diz que foi parcial. Detalhes em [ADR 0006](docs/adr/0006-graphql-como-unidade-de-medida.md).

## Mensageria e cadeia assincrona

Produzir mede o broker aceitando a mensagem. O que o usuario sente e a cadeia inteira — e para isso existe o passo `aguardar`:

```yaml
cenario:
  - kafka:
      topico: pedidos
      chave: "${pedidos.id}"          # chave fixa concentra tudo numa particao
      valor: { pedido: "${pedidos.id}", valor: "${pedidos.valor}" }

  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidos.id}"          # espera a mensagem desta iteracao, nao qualquer uma
      timeout: 10s
```

Execucao real contra Kafka, com um processador que leva 15 ms por mensagem e uma carga de 100/s:

```
Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  aguardar pedidos-processa… (2)        800    1.78 s    4.87 s    5.26 s    5.30 s    5.32 s       0
  kafka produzir pedidos-ca… (1)        800  0.915 ms    1.9 ms    4.9 ms     14 ms     27 ms       0

A jornada inteira
  Todas as 800 jornadas chegaram ao fim; metade levou ate 1778 ms e 95% ate 4874 ms, contados do instante em que deveriam ter comecado.
```

**Produzir custa 0,9 ms; a cadeia custa 4,87 s no 95%.** Uma ferramenta que so mede a producao teria reportado sub-milissegundo e aprovado o sistema. O consumidor nao acompanha 100/s, a fila cresce, e so a medicao ponta a ponta mostra isso.

Detalhes das decisoes: [ADR 0008](docs/adr/0008-mensageria-e-cadeia-assincrona.md). Publicacao com confirmacao e o padrao (`acks: todos` no Kafka, publisher confirms no AMQP), sem lote — agrupar mensagens mediria o lote, nao a mensagem.

### Quanto o gerador aguenta produzindo (medido)

Sem lote, uma mensagem por chegada agendada e com confirmacao, a vazao maxima e menor que a de ferramentas que agrupam. O numero, medido:

| Topico | Ultima taxa valida | p50 / p95 da producao | Primeira taxa invalida |
|---|---|---|---|
| 6 particoes | **15.000 msg/s** | 1,4 ms / 41 ms | 18.000/s (4.884 descartadas) |
| 1 particao | **5.000 msg/s** | 0,2 ms / 56 ms | 8.000/s (8.059 descartadas) |

Em 15.000/s o desvio de agendamento ficou em 0,001 ms tipico e 0,56 ms no pior caso, com pico de 861 mensagens em voo: **quem saturou primeiro nao foi o escalonador do braunrate, foi o caminho de entrega confirmada contra o broker.** Por isso o numero da tabela e o teto do par gerador+broker nesta maquina, e nao um teto puro do gerador.

**Ambiente:** Apple M2 Pro, 10 nucleos, Redpanda v24.2.7 em loopback no mesmo host, 10 s por execucao, um passo de producao por iteracao. Broker remoto, replicacao real e mensagem maior mudam o numero. Meça no seu ambiente antes de citar este.

Se o gerador nao sustentar a producao, o resultado sai **invalido com codigo de saida 3**, exatamente como em HTTP — a deteccao de back-pressure acontece no escalonador, antes de qualquer protocolo, e esta coberta por teste com Kafka de verdade (`TestGeradorSaturadoProduzindoInvalidaOResultado`).

### Quando o efeito so aparece por API

Nem todo sistema assincrono publica o resultado num topico: muitos so mostram o efeito numa consulta. Para esses, `aguardar` sonda ate a condicao valer:

```yaml
  - kafka:
      topico: pedidos
      chave: "${pedidos.id}"
      valor: { pedido: "${pedidos.id}" }

  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }     # ou { status: 200 }, ou { corpo_contem: PAGO }
      intervalo: 200ms
      timeout: 30s
```

**A granularidade e declarada, nao escondida.** Medir por sondagem so mede em degraus do intervalo: o valor sai sempre maior ou igual ao real, nunca menor. Por isso o relatorio traz a linha *"o passo X espera sondando a cada 200ms: a latencia dele tem essa granularidade e fica maior que a real, nunca menor"*, e `braunrate depurar` mostra o mesmo antes de qualquer carga. Intervalo menor mede mais fino e pesa mais no alvo — a escolha e de quem escreve, com o efeito impresso.

Sem `ate` o passo e recusado: a primeira resposta encerraria a espera e a medicao seria do tempo de responder, nao do tempo ate o efeito acontecer.

RabbitMQ segue a mesma forma:

```yaml
  - amqp:
      fila: pedidos
      identidade: "${pedidos.id}"
      corpo: { pedido: "${pedidos.id}" }
```

## Variedade observada: o relatorio diz o que aconteceu, nao o que foi declarado

```
Ambiente
  4 valores distintos de kafka.particao.pedidos-cadeia em 800 usos
  800 valores distintos de pedidos.id em 800 usos
```

Se a fonte tem varios valores e a execucao usou um so, o resultado e **invalido** e o comando sai com codigo 3:

```
RESULTADO INVALIDO: toda a carga caiu numa particao so de pedidos-cadeia; o resto do cluster
ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao
            kafka.particao.pedidos-cadeia tinha 4 valores disponiveis e a execucao usou 1, em 60 usos
```

Isso nasceu de um bug nosso: a autenticacao congelava os dados da primeira iteracao e toda execucao autenticada com CSV rodava sobre a primeira linha, com o relatorio anunciando variedade que nao existiu. A medicao passa a verificar isso em caminho, corpo, cabecalho, variavel de GraphQL e chave de mensagem — um ponto so de instrumentacao, entao protocolo novo entra coberto ([ADR 0007](docs/adr/0007-variedade-observada.md)).

## Comparar duas execucoes

```
$ braunrate comparar antes.json depois.json

Ficou mais lento: jornada inteira (95%): 71 vezes mais lento — de 10 ms para 675 ms. Com 2 ressalva(s) que podem explicar a diferenca sozinhas.

Por passo
  passo                        95% antes  95% depois         variacao
  consultar pedido                4.9 ms      598 ms        123x pior
  pagar fatura                    4.8 ms       43 ms        8.9x pior

O que pode explicar a diferenca sem ser o servico
  - as execucoes usaram versoes diferentes do braunrate: 0.2.0 e 0.3.0
  - as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao
  Duas execucoes nao dao intervalo de confianca: variacao abaixo de 5% e tratada como ruido.
```

A comparacao nunca chama de regressao o que pode ser ruido, lista tudo que mudou fora do servico (maquina, plano de carga, versao, cenario), e se recusa a comparar quando alguma das duas execucoes teve o gerador saturado.

## O que existe hoje

| Recurso | Estado |
|---|---|
| Motor de chegada aberta, latencia do instante agendado | pronto |
| Perfis: rampa, patamar, pico, taxa constante | pronto |
| HDR histogram, agregados mergeaveis, series alinhadas ao epoch | pronto |
| Deteccao de back-pressure com causa provavel (gerador x alvo) | pronto |
| Limite de requisicoes em voo, com descarte declarado no relatorio | pronto |
| HTTP: verbos, cabecalhos, corpo JSON, redirect, timeout, cookies | pronto |
| YAML com erro apontando linha e coluna | pronto |
| Resumo de terminal e progresso ao vivo | pronto |
| Correlacao em uma linha: JSON, cabecalho e regex | pronto |
| Autenticacao por token com renovacao, e basica | pronto |
| Dados: CSV com politica de consumo e geracao com semente | pronto |
| Assercoes funcionais e SLO por passo e global, com codigo de saida | pronto |
| Tempo total da jornada, contado do instante agendado | pronto |
| Autoria: schema no editor, `depurar`, `importar curl`, erros que ensinam | pronto |
| Relatorio HTML autocontido, com veredito em uma frase | pronto |
| JSON versionado, CSV por passo, comparacao entre execucoes | pronto |
| GraphQL: uma linha por operacao, erro em 200 contado como erro | pronto |
| Kafka e RabbitMQ com entrega confirmada, sem lote | pronto |
| Passo `aguardar`: mede a cadeia assincrona ponta a ponta | pronto |
| Variedade observada, com resultado invalido quando a carga concentra | pronto |
| Cenario em Go, com equivalencia YAML x DSL travada por teste | pronto |
| `importar jmx`: requisicao, cabecalho, CSV e correlacao do plano do JMeter | parcial, declarado |

## Por que existe

Tres razoes, nesta ordem:

1. **Medicao honesta por padrao.** Modelo de chegada aberto; latencia contada a partir do instante em que a requisicao *deveria* ter partido; HDR histogram; aviso explicito quando o gerador nao sustentou a taxa alvo. A omissao coordenada e a falha que faz teste passar com p99 de 47 ms enquanto producao sofre 1,8 s.
2. **Dois publicos, um motor.** YAML declarativo para o caso comum, DSL para o complexo — mesmo motor, mesmas metricas, sem reescrita ao migrar.
3. **Cenario de negocio, nao so requisicao.** GraphQL medido por operacao; Kafka e RabbitMQ com modelo de metrica proprio; passo `aguardar` para medir a cadeia assincrona ponta a ponta.

## Escopo

**Dentro:** HTTP/HTTPS e REST; GraphQL de primeira classe; Kafka e RabbitMQ (produzir e consumir); passo `aguardar` com timeout; correlacao, variaveis e fluxo de autenticacao; CSV com politica de consumo e geracao sintetica com semente; perfis de carga (rampa, patamar, pico, taxa constante); SLO com codigo de saida; relatorio HTML autocontido, JSON, CSV e resumo de terminal; comparacao entre execucoes; importador de `.jmx` para o subconjunto comum.

**Limitacao conhecida:** protocolo fora da lista acima exige recompilar o binario — a mesma friccao que o k6 tem. E consequencia da escolha de Go ([ADR 0004](docs/adr/0004-extensao-de-protocolo.md)), esta declarada aqui de proposito, e o processo de build reprodutivel para protocolo fora-de-arvore sera documentado. Avro e Schema Registry sao mais fracos em Go que na JVM e ficam para depois da v1.

**Limitacao conhecida:** um unico token para a execucao inteira, com a consequencia declarada no relatorio ([ADR 0005](docs/adr/0005-identidade-e-token.md)). E a latencia dos passos seguintes ao primeiro e tempo de servico, nao latencia corrigida — a leitura honesta da jornada esta no bloco "A jornada inteira".

**Fora:** motor de browser real; nuvem gerenciada, dashboard multiusuario, conta de time; LDAP, FTP, SMTP, JMS classico; competir em vazao bruta com wrk; execucao distribuida na v1 — a arquitetura nao pode impedi-la, mas ela nao entra agora.

## Documentacao

- [Principios de produto](docs/principios-de-produto.md) — criterio de aceitacao de toda decisao de interface
- [Roteiro](docs/roteiro.md)
- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — base de todas as decisoes
- [Arquitetura](docs/arquitetura.md)
- [ADR 0001 — linguagem e runtime](docs/adr/0001-linguagem-e-runtime.md)
- [ADR 0002 — modelo de cenario](docs/adr/0002-modelo-de-cenario.md)
- [ADR 0003 — modelo de execucao e metrica](docs/adr/0003-modelo-de-execucao-e-metrica.md)
- [ADR 0004 — extensao de protocolo](docs/adr/0004-extensao-de-protocolo.md)
- [ADR 0005 — identidade e token](docs/adr/0005-identidade-e-token.md)
- [ADR 0006 — GraphQL como unidade de medida](docs/adr/0006-graphql-como-unidade-de-medida.md)
- [ADR 0007 — variedade observada](docs/adr/0007-variedade-observada.md)
- [ADR 0008 — mensageria e cadeia assincrona](docs/adr/0008-mensageria-e-cadeia-assincrona.md)
- [ADR 0009 — equivalencia entre YAML e DSL](docs/adr/0009-equivalencia-entre-yaml-e-dsl.md)
- [ADR 0010 — codigo em ingles, produto em portugues](docs/adr/0010-idioma-do-codigo.md)
- [Schema do cenario](docs/braunrate.schema.json) — autocompletar e validacao no editor
- [Exemplo de relatorio HTML](docs/exemplo-relatorio.html) — saida real de uma execucao que falhou o SLO
- [Medicao dos prototipos da Fase 0](docs/medicoes-fase0.md)

## Licenca

MIT — Diego Braun.
