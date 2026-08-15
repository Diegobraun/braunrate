# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR histogram e deteccao de back-pressure.

## Demonstracao de honestidade de medicao

O alvo de teste embutido congela por 1 s no meio da execucao. Mesma pausa, mesmo alvo, dois modelos de medicao:

| Modelo | p99 reportado | Amostras |
|---|---|---|
| **braunrate (chegada aberta, latencia contada do instante agendado)** | **977,4 ms** | 600 |
| Laco fechado (um usuario virtual em sequencia, como JMeter e Locust medem) | 2,9 ms | 783 |

**974,6 ms escondidos pelo laco fechado.** O laco fechado nao mente por bug: quando o alvo trava, ele simplesmente para de enviar, e as requisicoes que deveriam ter partido nunca entram na conta. E a omissao coordenada.

Isso nao e alegacao de marketing: e um teste automatizado que roda no CI a cada push. Se a medicao mentir, o build quebra.

```
$ go test ./autovalidacao/... -v
=== RUN   TestMedicaoRefleteCongelamentoDoAlvo
    modelo aberto: p50 2.7 ms | p99 973.8 ms | max 1005.6 ms | n 600
--- PASS: TestMedicaoRefleteCongelamentoDoAlvo (3.01s)
=== RUN   TestCongelamentoDoAlvoNaoEhConfundidoComSaturacaoDoGerador
    aviso correto: a latencia do alvo cresceu ao longo da execucao enquanto o despacho
    continuou pontual; a degradacao e do alvo, nao do gerador | p99 por segundo passou
    de 3.5 ms para 993.3 ms
--- PASS: TestCongelamentoDoAlvoNaoEhConfundidoComSaturacaoDoGerador (3.01s)
=== RUN   TestLacoFechadoEsconderiaAPausaQueOModeloAbertoMostra
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 977.4 ms sobre 600 amostras
      laco fechado:              p99 2.9 ms sobre 783 amostras
      omissao coordenada: 974.6 ms escondidos pelo laco fechado
--- PASS: TestLacoFechadoEsconderiaAPausaQueOModeloAbertoMostra (6.01s)
```

Reproduza na sua maquina: `go test ./autovalidacao/... -v`.

## Estado

**Fase 3 concluida** — motor de chegada aberta, HTTP, correlacao, autenticacao, dados, assercoes, SLO com codigo de saida, ferramentas de autoria (schema no editor, `depurar`, `importar curl`) e relatorio (HTML autocontido, JSON, CSV, comparacao entre execucoes). Ainda nao existem GraphQL nem mensageria.

Decisao da Fase 0: **Go**, sustentada por dois criterios apenas — RSS sob carga (30 MB contra 597 MB do Java com G1, a 10.000/s) e binario unico estatico, que para o publico de QA significa instalar baixando um arquivo. Startup, precisao de agendamento e modo de falha apareceram na primeira analise com peso que nao aguentam, e estao marcados como nao-criterio no ADR. Numeros, metodologia e limites em [medicoes-fase0.md](docs/medicoes-fase0.md); a decisao com os pesos de cada criterio em [ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Como usar

```bash
go build -o braunrate ./cmd/braunrate

braunrate alvo -latencia=5ms &                     # alvo de teste embutido
braunrate validar cenarios/http-basico.yaml        # valida sem executar
braunrate depurar cenarios/http-basico.yaml        # uma iteracao, tudo visivel
braunrate executar cenarios/http-basico.yaml       # executa e resume no terminal
braunrate executar cenarios/http-basico.yaml -html=relatorio.html -resultado=saida.json
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

Cenario com autenticacao, correlacao, dados e SLO — o exemplo completo esta em [`cenarios/jornada-autenticada.yaml`](cenarios/jornada-autenticada.yaml):

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
  Metade das respostas em ate 4.4 ms; 95% em ate 5.1 ms; 99% em ate 5.5 ms; a pior levou 13 ms

A jornada inteira
  Todas as 2375 jornadas chegaram ao fim; metade levou ate 9 ms e 95% ate 10 ms, contados do instante em que deveriam ter comecado.
  metade 8.8 ms | 95% 10 ms | 99% 11 ms | pior 19 ms

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)      2.375    4.4 ms    5.2 ms    5.6 ms    6.3 ms     13 ms       0
  pagar fatura               (2)      2.375    4.4 ms    5.1 ms    5.4 ms    6.0 ms    6.8 ms       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.
  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como
      esse passo depende do valor capturado antes dele, nao existe instante
      agendado proprio. Para a leitura honesta da jornada, use "A jornada inteira".

Confiabilidade da medicao
  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.
  Atraso tipico para disparar: 0.001 ms; pior caso: 0.476 ms (o tempo de resposta ja desconta isso)

Ambiente
  Mac darwin/arm64, 10 nucleos | braunrate 0.2.0 | 2026-08-15 22:59:13
  Sementes dos dados: assinantes=1 (mesma semente, mesmos dados)
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

**Ver a iteracao antes da carga**, que e onde a correlacao quebrada aparece:

```
$ braunrate depurar cenarios/jornada-autenticada.yaml
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
  braunrate executar cenarios/jornada-autenticada.yaml
```

## O relatorio

```bash
braunrate executar cenarios/jornada-autenticada.yaml -html=relatorio.html -resultado=saida.json -csv=passos.csv
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
| GraphQL | Fase 4 |
| Kafka, AMQP, passo `aguardar` | Fase 5 |
| DSL e importador de `.jmx` | Fase 6 |

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
- [Schema do cenario](docs/braunrate.schema.json) — autocompletar e validacao no editor
- [Exemplo de relatorio HTML](docs/exemplo-relatorio.html) — saida real de uma execucao que falhou o SLO
- [Medicao dos prototipos da Fase 0](docs/medicoes-fase0.md)

## Licenca

MIT — Diego Braun.
