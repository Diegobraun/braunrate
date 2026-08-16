# Comandos

`braunrate` sem argumento nenhum mostra o caminho; `braunrate ajuda` lista tudo.
Toda opcao aceita `-h`, e opcao escrita errada recebe a certa de volta:

```
$ braunrate target -addr :8080
"-addr" nao existe. Voce quis dizer "-address"?

    braunrate target -address :8080

Todas as opcoes: braunrate target -h
```

## `demo`

```bash
braunrate demo
braunrate demo --com-falha
```

Sobe o alvo embutido, escreve o cenario que vai rodar, executa e explica cada
numero. Nao precisa de arquivo, de alvo nem de segundo terminal. `--com-falha`
roda contra um alvo que trava no meio e mede a mesma travada de duas formas.

Deixa `demo.yaml` e `demo-relatorio.html` no diretorio atual, e diz que deixou.

## `new`

```bash
braunrate new cenario.yaml
```

Escreve um esqueleto comentado. Nunca sobrescreve arquivo existente. E o caminho
raro: quase sempre e melhor importar um `curl`.

## `import`

```bash
braunrate import curl "curl 'https://api.exemplo.com/v1/pedidos/9912' -X POST -H 'Authorization: Bearer abc.def' -d '{\"valor\": 199.90}'" -output cenario.yaml
pbpaste | braunrate import curl
braunrate import jmx plano.jmx -output cenario.yaml
```

Do `curl` sai um cenario que ja carrega, com carga e criterio de aceite de
partida, e tres avisos honestos: o token virou `${token}` e nao vai para o
repositorio; o id fixo no caminho faz o alvo responder de cache; os numeros de
carga e criterio sao chute, nao medicao.

Do `.jmx` a traducao e parcial, e **o que ficou de fora sai listado no terminal**
— importador que engole o arquivo em silencio entrega um cenario que mede outra
coisa:

| Traduzido | Nao traduzido (sai declarado) |
|---|---|
| `HTTPSamplerProxy` (metodo, caminho, dominio, corpo) | Controladores (If, While, Loop), temporizadores |
| `HeaderManager`, com credencial virando variavel de ambiente | Scripts JSR223/BeanShell |
| `CSVDataSet` (arquivo e reciclagem) | Samplers de JDBC, JMS e outros nao-HTTP |
| `ThreadGroup`, como **aviso**, nunca como taxa | Assercoes: todo passo sai com `status: 200` |
| `JSONPostProcessor` e `RegexExtractor`, como instrucao de captura | Funcoes `${__...}` do JMeter |

**Thread nunca vira taxa.** No JMeter uma thread so envia depois que a resposta
anterior chegou: 50 threads sao 50/s se o alvo responde em 1 s e 5/s se responde
em 10 s. Converter em silencio importaria a omissao coordenada junto com o plano:

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

O recorder do JMeter transcreve: grava o token daquela sessao e o pedido `9912`,
e na segunda execucao o cenario quebra. Este faz quatro coisas a mais, e
**declara cada uma**:

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

**Fora de escopo, com o motivo:** gravar dentro de **HTTPS** exige o braunrate
emitir certificado e a sua maquina confiar nele — mexer no armazem de confianca
do sistema nao e coisa que ferramenta de carga deve automatizar em silencio. A
conexao e encaminhada para o cliente continuar funcionando, e o que nao foi
gravado aparece na tela por host. **Trafego de aplicativo movel** fica fora da
v1, por causa de pinning de certificado.

## `validate`

```bash
braunrate validate cenario.yaml
```

Le e confere sem executar nada. Diz quantas iteracoes o cenario produziria,
avisa o que voce nao declarou, e aponta o proximo passo:

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

Um usuario, uma iteracao, sem carga. Mostra requisicao, resposta, captura,
variavel e onde parou. E onde a correlacao quebrada aparece — antes dos dez
minutos de carga, nao depois:

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

| Opcao | O que faz |
|---|---|
| `-result <arquivo.json>` | grava o documento de resultado, que e o que `compare` e `report` leem depois |
| `-html <arquivo.html>` | relatorio autocontido, que abre em rede fechada e sobrevive anexado em ticket |
| `-csv <arquivo.csv>` | uma linha por passo, para planilha |
| `-baseline <arquivo.json>` | execucao anterior, para as regras de `regressao` |
| `-max-concurrent <n>` | maximo de requisicoes simultaneas antes de desistir de disparar |
| `-late-threshold <dur>` | a partir daqui o gerador conta como nao tendo sustentado a taxa |
| `-quiet` | nao imprime progresso nem a dica de proximo passo |

## `report`

```bash
braunrate report saida.json -html relatorio.html
braunrate report saida.json -csv passos.csv
```

Gera a saida a partir de um resultado ja gravado, sem rodar nada de novo.

## `compare`

```bash
braunrate compare antes.json depois.json
braunrate compare antes.json depois.json -html comparacao.html
```

Diz o que mudou, lista tudo que mudou fora do servico (maquina, plano de carga,
versao, cenario) e se recusa a comparar quando alguma das duas execucoes nao vale
como medicao. Codigo de saida `3` quando nao ha veredito possivel.

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
duas execucoes — **o que a CLI ja faz, e nada alem disso.** Toda rota termina no
mesmo codigo que o terminal usa, e um teste reprova o build se os dois deixarem
de produzir o mesmo documento.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/runs
curl -sN http://127.0.0.1:8080/runs/r001/stream
```

**Uma execucao por vez, por padrao.** Duas execucoes na mesma maquina disputam a
CPU que precisa despachar no instante agendado, e nenhuma das duas mede o que se
propos a medir. A segunda responde `409` e diz como aceitar a contaminacao.

**O YAML continua sendo a verdade.** Nao ha banco: os cenarios sao os arquivos do
`-dir`, e as execucoes vivem na memoria do processo.

## `target`

```bash
braunrate target -latency=5ms
braunrate target -freeze-after=2s -freeze-for=2s
braunrate target -raw
braunrate target -kafka=127.0.0.1:9092 -input=pedidos -output=pedidos-processados
```

O alvo de teste embutido, para quem ainda nao tem servico para apontar. `-raw`
sobe um alvo minimo que responde sem interpretar a requisicao, para medir o teto
do gerador — medir o teto contra o alvo completo mediria o par gerador+alvo.

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

Os protocolos compilados aparecem porque dois binarios com a mesma versao
poderiam produzir resultados diferentes sem deixar rastro do motivo.
