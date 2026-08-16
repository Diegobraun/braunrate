# Comandos

`braunrate` sem argumento nenhum mostra o caminho; `braunrate ajuda` lista tudo.
Toda opção aceita `-h`, e opção escrita errada recebe a certa de volta:

```
$ braunrate target -addr :8080
"-addr" nao existe. Voce quis dizer "-address"?

    braunrate target -address :8080

Todas as opcoes: braunrate target -h
```

| Comando | Para quê |
|---|---|
| [`demo`](#demo) | ver a ferramenta funcionando sem preparar nada |
| [`new`](#new) | escrever um esqueleto de cenário |
| [`import`](#import) | partir de um `curl` ou de um plano de JMeter |
| [`record`](#record) | gravar o fluxo navegando |
| [`validate`](#validate) | conferir o arquivo sem executar |
| [`debug`](#debug) | rodar uma iteração e ver o que acontece |
| [`execute`](#execute) | rodar com carga |
| [`report`](#report) | gerar HTML ou CSV de um resultado já gravado |
| [`compare`](#compare) | comparar duas execuções |
| [`serve`](#serve) | expor a CLI como HTTP local |
| [`target`](#target) | subir o alvo de teste embutido |
| [`version`](#version) | versão, commit e protocolos compilados |

## `demo`

```bash
braunrate demo
braunrate demo --com-falha
```

Sobe o alvo embutido, escreve o cenário que vai rodar, executa e explica cada
número. Não precisa de arquivo, de alvo nem de segundo terminal. `--com-falha`
roda contra um alvo que trava no meio e mede a mesma travada de duas formas.

Deixa `demo.yaml` e `demo-relatorio.html` no diretório atual, e diz que deixou.

## `new`

```bash
braunrate new cenario.yaml
```

Escreve um esqueleto comentado, e nunca sobrescreve arquivo existente. É o
caminho raro: quase sempre é melhor importar um `curl`.

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
atencao: o grupo "Usuarios" declara 50 threads, rampa de 30s, 300s de duracao: numero de
thread nao vira taxa de chegada, porque thread so envia depois da resposta anterior. O bloco
'carga' ficou com um chute; troque pela taxa que voce quer sustentar (requisicoes por segundo)
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
descartei 1 dominio de fora (example.com)
descartei 1 recurso estatico
3 requisicoes viraram 2 passo(s) em cenario.yaml
2 valor(es) observado(s) de pedidos_id em cenario-pedidos-id.csv
atencao: o campo "senha" do corpo virou ${senha}: rode com SENHA=... no ambiente, para nao versionar credencial
atencao: a sequencia gravada e uma passagem so: o mix de producao tem outras proporcoes entre as rotas
atencao: os numeros de carga e de slo sao um chute de partida, nao uma medicao: ajuste antes de usar como gate

Proximo passo, antes de qualquer carga:
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
Cenario valido: "Jornada com criterios novos", 2 passo(s), 500 iteracoes em 5s.
Atencao: o gate mede 2 passos isolados e deixa de fora a jornada inteira, que e o tempo que o usuario espera.
    declare tambem:  - jornada: { p95: < 2s, p99: < 5s }

Antes de rodar a carga, veja se o cenario faz o que voce espera:
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
$ braunrate debug examples/jornada-autenticada.yaml
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
              corpo: {"valor":199.9}
  resposta:   status 200, 63 bytes

variaveis no fim da iteracao
  assinantes.id = 1001

Iteracao completa: 2 passos, tudo certo. Para rodar com carga:
  braunrate execute examples/jornada-autenticada.yaml
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
| `-baseline <arquivo.json>` | execução anterior, para as regras de `regressao` |
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
braunrate serve em http://127.0.0.1:8080, servindo cenarios de ./cenarios
Sem autenticacao e sem TLS: qualquer um que alcance esta porta pode disparar carga contra os alvos dos cenarios.
Foi feito para rodar em 127.0.0.1. Expor em outra interface e outra decisao, e ela ainda nao foi tomada.

Para ver o que ele esta servindo:
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

## `version`

```bash
braunrate version
```

```
braunrate 0.5.0
commit: e1dca9c279e1ec653ea52cec1f4325e04ec21599
data: 2026-08-16T17:01:38Z
protocolos compilados: [aguardar amqp graphql http kafka]
```

Os protocolos compilados aparecem porque dois binários com a mesma versão
poderiam produzir resultados diferentes sem deixar rastro do motivo.
