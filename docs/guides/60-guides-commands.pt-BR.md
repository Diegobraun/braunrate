---
translated_from: 60-guides-commands.en.md
source_hash: 20bdc218afba
---
# Comandos

`braunrate` sem argumento nenhum mostra o caminho; `braunrate help` lista tudo.
Toda opção aceita `-h`, e opção escrita errada recebe a certa de volta:

```
$ braunrate target -addr :8080
"-addr" does not exist. Did you mean "-address"?

    braunrate target -address :8080

Every option: braunrate target -h
```

| Comando | Para quê |
|---|---|
| [`demo`](#demo) | ver a ferramenta funcionando sem preparar nada |
| [`new`](#new) | escrever um esqueleto de cenário |
| [`migrate`](#migrate) | converter um cenário no formato em português |
| [`import`](#import) | partir de um `curl` ou de um plano de JMeter |
| [`record`](#record) | gravar o fluxo navegando |
| [`validate`](#validate) | conferir o arquivo sem executar |
| [`debug`](#debug) | rodar uma iteração e ver o que acontece |
| [`execute`](#execute) | rodar com carga |
| [`report`](#report) | gerar HTML ou CSV de um resultado já gravado |
| [`compare`](#compare) | comparar duas execuções |
| [`serve`](#serve) | expor a CLI como HTTP local |
| [`ui`](#ui) | editar e rodar os cenários no navegador |
| [`target`](#target) | subir o alvo de teste embutido |
| [`version`](#version) | versão, commit e protocolos compilados |

## `demo`

```bash
braunrate demo
braunrate demo --with-failure
```

Sobe o alvo embutido, escreve o cenário que vai rodar, executa e explica cada
número. Não precisa de arquivo, de alvo nem de segundo terminal. `--with-failure`
roda contra um alvo que trava no meio e mede a mesma travada de duas formas.

Deixa `demo.yaml` e `demo-report.html` no diretório atual, e diz que deixou.

## `new`

```bash
braunrate new cenario.yaml
```

Escreve um esqueleto comentado, e nunca sobrescreve arquivo existente. É o
caminho raro: quase sempre é melhor importar um `curl`.

## `migrate`

```bash
braunrate migrate cenario.yaml
braunrate migrate ./cenarios -dry-run
braunrate migrate cenario.yaml -output cenario-en.yaml
```

Converte um cenário escrito no formato em português, substituído na 0.6.0,
preservando comentários e a ordem das chaves. Lista linha por linha o que mudou,
deixa o original como `.bak` e recusa arquivo já convertido. Nenhuma mudança de
comportamento: é renomeação.

## `import`

```bash
braunrate import curl "curl 'https://api.exemplo.com/v1/pedidos/9912' -X POST -H 'Authorization: Bearer abc.def' -d '{\"valor\": 199.90}'" -output cenario.yaml
pbpaste | braunrate import curl
braunrate import jmx plano.jmx -output cenario.yaml
```

Do `curl` sai um cenário que já carrega, com carga e critério de aceite de
partida, e três avisos: o token virou `${token}` e não vai para o repositório; o
id fixo no caminho faz o alvo responder de cache; os números de carga e critério
são chute, não medição.

Do `.jmx` a tradução é parcial, e o que ficou de fora sai listado no terminal.
Importador que engole o arquivo em silêncio entrega um cenário que mede outra
coisa:

| Traduzido | Não traduzido (sai declarado) |
|---|---|
| `HTTPSamplerProxy` (método, caminho, domínio, corpo) | Controladores (If, While, Loop), temporizadores |
| `HeaderManager`, com credencial virando variável de ambiente | Scripts JSR223/BeanShell |
| `CSVDataSet` (arquivo e reciclagem) | Samplers de JDBC, JMS e outros não-HTTP |
| `ThreadGroup`, como **aviso**, nunca como taxa | Asserções: todo passo sai com `status: 200` |
| `JSONPostProcessor` e `RegexExtractor`, como instrução de captura | Funções `${__...}` do JMeter |

> **Importante** Thread nunca vira taxa. No JMeter uma thread só envia depois que
> a resposta anterior chegou: 50 threads são 50/s se o alvo responde em 1 s e 5/s
> se responde em 10 s. Converter em silêncio importaria a omissão coordenada
> junto com o plano.

```
warning: the group "Usuarios" declares 50 threads, ramp of 30s, 300s of duration: a thread count does not
turn into an arrival rate, because a thread only sends after the previous response. The 'load' block
came out as a guess; swap it for the rate you want to sustain (requests per second)
```

## `record`

```bash
braunrate record -output cenario.yaml
# aponte o navegador ou o curl para o proxy, navegue pelo fluxo, Ctrl+C
```

O recorder do JMeter transcreve: grava o token daquela sessão e o pedido `9912`,
e na segunda execução o cenário quebra. Este faz quatro coisas a mais, e declara
cada uma:

```
dropped 1 an outside domain (example.com)
dropped 1 a static asset
3 requests became 2 steps in cenario.yaml
2 observed value(s) of pedidos_id in cenario-pedidos-id.csv
warning: the field "senha" of the body became ${senha}: run with SENHA=... in the environment, so a credential does not get versioned
warning: the recorded sequence is a single pass: the production mix has other proportions between the routes
warning: the load and slo numbers are a starting guess, not a measurement: tune them before using this as a gate

Next step, before any load:
  braunrate debug cenario.yaml
```

> **Nota** Gravar dentro de HTTPS exige o braunrate emitir certificado e a sua
> máquina confiar nele, e mexer no armazém de confiança do sistema não é coisa
> que ferramenta de carga deva automatizar em silêncio. A conexão é encaminhada
> para o cliente continuar funcionando, e o que não foi gravado aparece na tela
> por host. Tráfego de aplicativo móvel fica fora da v1, por causa de pinning de
> certificado.

## `validate`

```bash
braunrate validate cenario.yaml
```

Lê e confere sem executar nada. Diz quantas iterações o cenário produziria, avisa
o que você não declarou, e aponta o próximo passo:

```
Valid scenario: "Jornada com criterios novos", 2 steps, 500 iterations in 5s.
Warning: the gate measures 2 isolated steps and leaves out the whole journey, which is the wait the user feels.
    declare it too:  - journey: { p95: < 2s, p99: < 5s }

Before running the load, check that the scenario does what you expect:
  braunrate debug cenario.yaml
```

## `debug`

```bash
braunrate debug cenario.yaml
```

Um usuário, uma iteração, sem carga. Mostra requisição, resposta, captura,
variável e onde parou. É onde a correlação quebrada aparece, antes dos dez
minutos de carga e não depois:

```
$ braunrate debug examples/authenticated-journey.yaml
debugging "Billing journey" against http://127.0.0.1:8080: 1 user, 1 iteration, no load

step 1 — look up order   [ok in 6.8ms]
  request:    GET /orders/1001
              Authorization: Bearer test-t… (10 characters)
  response:   status 200, 91 bytes
  body:       {"id":"1001","status":"OPEN","lastInvoice":{"id":"f-1001","amount":199.90,"status":"OPEN"}}
  captured:
    invoiceId = f-1001

step 2 — pay invoice   [ok in 6.1ms]
  request:    POST /invoices/f-1001/pay
              Authorization: Bearer test-t… (10 characters)
  response:   status 200, 63 bytes
  body:       {"id":"f-1001","status":"PAID","paidAt":"2026-08-15T00:00:00Z"}

variables at the end of the iteration
  invoiceId = f-1001
  token = test-token

Iteration complete: 2 steps, all good. To run it with load:
  braunrate execute examples/authenticated-journey.yaml
```

## `execute`

```bash
braunrate execute cenario.yaml
braunrate execute cenario.yaml -html=relatorio.html -result=saida.json -csv=passos.csv
braunrate execute cenario.yaml -baseline=execucao-anterior.json
braunrate execute cenario.yaml -quiet
```

| Opção | O que faz |
|---|---|
| `-result <arquivo.json>` | grava o documento de resultado, que é o que `compare` e `report` leem depois |
| `-html <arquivo.html>` | relatório autocontido, que abre em rede fechada e sobrevive anexado em ticket |
| `-csv <arquivo.csv>` | uma linha por passo, para planilha |
| `-baseline <arquivo.json>` | execução anterior, para as regras de `regression` |
| `-max-concurrent <n>` | máximo de requisições simultâneas antes de desistir de disparar |
| `-late-threshold <dur>` | a partir daqui o gerador conta como não tendo sustentado a taxa |
| `-quiet` | não imprime progresso nem a dica de próximo passo |

## `report`

```bash
braunrate report saida.json -html relatorio.html
braunrate report saida.json -csv passos.csv
```

Gera a saída a partir de um resultado já gravado, sem rodar nada de novo.

## `compare`

```bash
braunrate compare antes.json depois.json
braunrate compare antes.json depois.json -html comparacao.html
```

Diz o que mudou, lista tudo que mudou fora do serviço (máquina, plano de carga,
versão, cenário) e se recusa a comparar quando alguma das duas execuções não vale
como medição. Código de saída `3` quando não há veredito possível.

## `serve`

```bash
braunrate serve -addr 127.0.0.1:8080 -dir ./cenarios
```

```
braunrate serve at http://127.0.0.1:8080, serving scenarios from ./cenarios
No authentication and no TLS: anyone who reaches this port can fire load at the targets of the scenarios.
It was made to run on 127.0.0.1. Exposing it on another interface is a separate decision, and it has not been made.

To see what it is serving:
  curl http://127.0.0.1:8080/scenarios
```

Validar, depurar, executar, acompanhar, listar, buscar o JSON e o HTML, comparar
duas execuções: o que a CLI já faz, e nada além disso. Toda rota termina no mesmo
código que o terminal usa, e um teste reprova o build se os dois deixarem de
produzir o mesmo documento.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/runs
curl -sN http://127.0.0.1:8080/runs/r001/stream
```

> **Atenção** É uma execução por vez, por padrão. Duas execuções na mesma máquina
> disputam a CPU que precisa despachar no instante agendado, e nenhuma das duas
> mede o que se propôs a medir. A segunda responde `409` e diz como aceitar a
> contaminação.

O YAML continua sendo a verdade. Não há banco: os cenários são os arquivos do
`-dir`, e as execuções vivem na memória do processo.

## `ui`

```bash
braunrate ui -dir ./cenarios
braunrate ui -addr 127.0.0.1:8080 -dir ./cenarios -open=false
```

```
braunrate ui at http://127.0.0.1:8080, editing the scenarios in ./cenarios
No authentication and no TLS: anyone who reaches this port can fire load at the targets of the scenarios.
It was made to run on 127.0.0.1. Exposing it on another interface is a separate decision, and it has not been made.
Writing enabled: whoever reaches this port can change the scenario files in ./cenarios.

Open it in the browser:
  http://127.0.0.1:8080
```

Abre no navegador o mesmo que a CLI faz: lista os cenários do diretório, edita o
arquivo, valida enquanto você digita, roda uma iteração, executa com carga e
mostra o relatório. O comando de terminal equivalente fica no topo da tela, em
toda tela.

> **Importante** A interface é um editor do arquivo, não um formulário que gera
> um. O texto que você salva é o que o terminal lê, com os comentários que você
> escreveu, e editar o arquivo por fora continua valendo. O motivo está em
> [ADR 0018](decisions.html).

`-open=false` não abre o navegador sozinho, para quem roda em terminal remoto ou
não quer que a janela suba.

## `target`

```bash
braunrate target -latency=5ms
braunrate target -freeze-after=2s -freeze-for=2s
braunrate target -raw
braunrate target -kafka=127.0.0.1:9092 -input=pedidos -output=pedidos-processados
```

O alvo de teste embutido, para quem ainda não tem serviço para apontar. `-raw`
sobe um alvo mínimo que responde sem interpretar a requisição, para medir o teto
do gerador; medir o teto contra o alvo completo mediria o par gerador+alvo.

| Rota | Credencial |
|---|---|
| `/orders`, `/orders/{id}` | não pede: é o passo que o `braunrate new` escreve, e ele roda de primeira |
| `/invoices/{id}`, `/graphql` | pedem token obtido em `POST /auth/token` |
| `/auth/token`, `/health` | não pedem |

> **Dica** Para exercitar a jornada autenticada, aponte um passo para `/invoices`
> e declare o bloco `auth` — é o que os exemplos do repositório fazem.

## `version`

```bash
braunrate version
```

```
braunrate 0.6.0
commit: e1dca9c279e1ec653ea52cec1f4325e04ec21599
date: 2026-08-17T02:01:38Z
compiled protocols: [amqp await graphql http kafka]
```

Os protocolos compilados aparecem porque dois binários com a mesma versão
poderiam produzir resultados diferentes sem deixar rastro do motivo.
