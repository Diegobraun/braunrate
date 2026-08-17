---
translated_from: 30-guides-concepts.en.md
source_hash: fb5f0a39ecbc
---
# Conceitos

Cinco ideias. Nenhuma delas é teoria: cada uma corresponde a uma linha que
aparece no relatório, e saber lê-las é a diferença entre confiar no número e
repetir o número.

## Taxa, e por que o modelo aberto

**Taxa** é quantas requisições por segundo o gerador dispara. No braunrate ela é
uma decisão sua, declarada no arquivo, e o gerador insiste nela mesmo quando o
alvo demora:

```yaml fragment
load:
  profiles:
    - ramp: { from: 100/s, to: 800/s, duration: 5s }
    - steady: { rate: 800/s, duration: 10s }
    - spike: { rate: 2000/s, duration: 3s }
```

Isso é o **modelo aberto**, e é o padrão. O outro jeito de descrever carga é o
**laço fechado**: N usuários virtuais, cada um pedindo de novo só depois que a
resposta anterior chegou. É como JMeter e Locust medem.

A diferença importa exatamente quando mais importa. No laço fechado, se o alvo
travar, os usuários param de pedir junto, e o atraso nunca entra na conta. Um
usuário de verdade não faz isso: ele chega quando ia chegar, e espera.

O braunrate conta o tempo de resposta **do instante em que a requisição deveria
ter partido**, não de quando ela partiu. Por isso uma travada do alvo aparece no
número em vez de sumir dele. O nome disso é omissão coordenada, e dá para ver os
dois lados na sua máquina com `braunrate demo --with-failure`.

### O modelo fechado existe, declarado

```yaml fragment
load:
  model: closed
  users: 200
  duration: 5m
  thinkTime: 1s
```

**Serve** quando o limite real é de sessão, não de chegada: pool de conexão,
licença por assento, fila com número fixo de trabalhadores, ou quando você está
reproduzindo um plano de JMeter escrito em threads.

**Mente** quando a pergunta é "o alvo aguenta X por segundo?". Mesmo alvo,
congelado por 3 s no meio de 12 s — à esquerda 100/s em modelo aberto, à direita
10 usuários em laço fechado:

```
1,200 requests, 100 per second             850 requests, 70 per second
half 6.7 ms | 95% 2.41 s | worst 3.01 s    half 6.9 ms | 95% 7.9 ms | worst 2.96 s
```

O 95% caiu de **2,41 s para 7,9 ms**. O laço fechado não errou conta nenhuma: ele
mediu com precisão um evento que ele mesmo deixou de provocar. Repare também na
taxa, 100/s contra 70/s — num teste de capacidade, é a carga que deveria ter
continuado.

Por isso o relatório do modelo fechado abre com aviso, sempre, mesmo quando tudo
passa; o documento JSON **não tem** campo de tempo corrigido, porque sem instante
agendado não há o que corrigir; e `braunrate compare` recusa comparar uma execução
aberta com uma fechada.

## "95% das respostas em até X"

O relatório não mostra média. Média esconde: se 95 respostas levam 5 ms e 5 levam
2 segundos, a média dá 105 ms e ninguém percebe as cinco lentas.

"95% em até 6,2 ms" quer dizer que 5% das pessoas esperaram mais que 6,2 ms. Os
cortes publicados são metade (50%), 95%, 99%, 99,9% e a pior.

### O tempo do passo 2 em diante não é corrigido

Só o primeiro passo tem instante agendado próprio. Os seguintes dependem de um
valor capturado antes deles, então começam quando o passo anterior termina. Esta
execução real, contra o alvo embutido congelado por 1 s no meio, mostra o tamanho
do problema:

```
Per step
  step                             requests      half       95%       99%     99.9%     worst  errors
  look up order              (1)      2,400    5.5 ms    416 ms    900 ms    1.00 s    1.01 s       0
  pay invoice                (2)      2,400    5.3 ms    7.2 ms     12 ms     15 ms    1.01 s       0

  (1) time counted from the instant the request should have gone out — it includes
      any delay, and for that reason it does not hide a freeze in the target.
  (2) plain response time, counted from when the previous step finished. Because
      that step depends on a value captured before it, it has no scheduled
      instant of its own. For the honest reading of the journey, use "The whole journey".
```

Repare no `pay invoice`: **7,2 ms no 95%**, com o alvo congelado por um segundo
inteiro. Sozinho, esse número é o mesmo tipo de mentira que uma ferramenta de laço
fechado produz.

> **Importante** A leitura que vale para quem usa o sistema é **A jornada
> inteira**, contada do instante agendado. Na mesma execução acima, ela mostra
> 428 ms.

## Critério de aceite

É o limite que você declara e que vira código de saída. Quatro escopos:

```yaml fragment
slo:
  - look up order: { p95: < 150ms }                 # um passo
  - journey: { p95: < 2s, p99: < 5s }               # a espera inteira
  - global: { success: ">= 99.9", throughput: ">= 90/s" }
  - regression: { journeyP95: "<= 10% worse" }      # contra uma execucao anterior
```

O relatório mostra também **o que não foi declarado**, porque um gate que só mede
partes aprova cada pedaço sem dizer nada sobre a espera inteira:

```
SLO
  FAIL  Failed: "look up order" answered 95% within 416 ms, above the limit of 150 ms.
  ok    Passed: the whole journey answered 95% within 428 ms, within the limit of 2000 ms.
  ok    Passed: the whole scenario had the success rate of 100.00%, at the minimum of 99.90%.
  --    steps with no criterion declared (1 of 2): pay invoice
  --    regression: no criterion declared — the gate approves without comparing against the previous run
```

Nada disso é obrigatório: cenário sem bloco `slo` continua executando e
reportando, só não serve de gate.

## Variedade dos dados

Repetir a mesma requisição mil vezes mede o cache do alvo, não o alvo. O
relatório publica **o que aconteceu**, não o que foi declarado:

```
Environment
  4 distinct values of orders.id across 100 uses, between 1,001 and 1,004
  1 single value of token across 100 uses
```

Contagem de distintos responde "um valor ou muitos"; ela não responde **onde** os
valores caíram, e mil ids diferentes do mesmo cliente exercitam uma fatia só do
alvo. Por isso a linha traz também a faixa e o prefixo comum.

Se a fonte tem vários valores e a execução usou um só, o resultado é **inválido**:

```
INVALID RESULT: the whole load landed on a single partition of orders-chain; the rest of the
cluster stood still and the number does not represent production. Make the message key vary per iteration
            kafka.partition.orders-chain had 4 available values and the run used 1, across 60 uses
```

Essa regra nasceu de um defeito nosso: a autenticação congelava os dados da
primeira iteração, e toda execução autenticada com CSV rodava sobre a primeira
linha enquanto o relatório anunciava variedade que não existiu.

## Resultado inválido

Toda execução passa por uma verificação de sanidade **antes** de o critério de
aceite ser lido. Ela não pergunta se o alvo foi bem; pergunta se a execução mediu
o que se propôs a medir. Quando a resposta é não, o critério nem chega a ser
avaliado e o comando sai com **código 3**:

```
Invalid result: the run did not measure what it set out to measure. This is not a verdict on the
target — it is the measurement that does not hold, and that is why no SLO rule was evaluated.

  - no journey reached the end, so the scenario never exercised the sequence it declared. Run
    'braunrate debug' to see where the iteration stops
    60 journeys started, 0 completed
  - the step "look up order" failed on 100% of the requests; no successful response entered its
    measurement
    60 requests, 60 errors (status: 60)
```

Os seis casos que invalidam:

| Caso | Por que o número não vale |
|---|---|
| nenhuma jornada chegou ao fim | o cenário não exercitou a sequência que declarou |
| todos os passos falharam, ou um passo falhou em 100% | o tempo medido é o de recusar, não o de fazer |
| a carga declarada não foi aplicada inteira | só o pedaço que rodou ficou medido |
| um passo declarado não registrou amostra | ele ficou de fora da medição |
| variedade colapsada em fonte com vários valores | o alvo pode ter respondido de cache |
| o gerador não sustentou a taxa | os números medem o gerador, não o alvo |

> **Atenção** Código 3 é diferente de código 1. `1` quer dizer "o alvo não
> atendeu ao critério"; `3` quer dizer "esta execução não serve para afirmar
> nada".

A verificação vale sempre, com ou sem bloco `slo`. Ela nasceu de três defeitos da
mesma família: dados congelados na primeira iteração, o próprio `examples/ci.yaml`
rodando 100% de 401 e passando verde desde a Fase 1, e a variedade que só foi
conferida quando alguém pediu. Os três eram execução sintaticamente perfeita,
semanticamente vazia, com a suíte inteira verde.

## Uma ressalva permanente: um token para a execução inteira

O motor faz login uma vez e reaproveita a credencial em todas as jornadas, e isso
não existe em produção. Se o alvo tiver cache por identidade, rate limit por token
ou sharding por usuário, o número fica otimista, ou falha por 429 que não
aconteceria. O relatório declara essa ressalva em toda execução com autenticação.
