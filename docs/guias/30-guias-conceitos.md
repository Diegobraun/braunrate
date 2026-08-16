# Conceitos

Cinco ideias. Nenhuma delas é teoria: cada uma corresponde a uma linha que
aparece no relatório, e saber lê-las é a diferença entre confiar no número e
repetir o número.

## Taxa, e por que o modelo aberto

**Taxa** é quantas requisições por segundo o gerador dispara. No braunrate ela é
uma decisão sua, declarada no arquivo, e o gerador insiste nela mesmo quando o
alvo demora:

```yaml trecho
carga:
  perfis:
    - rampa: { de: 100/s, ate: 800/s, durante: 5s }
    - patamar: { taxa: 800/s, durante: 10s }
    - pico: { taxa: 2000/s, durante: 3s }
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
dois lados na sua máquina com `braunrate demo --com-falha`.

### O modelo fechado existe, declarado

```yaml trecho
carga:
  modelo: fechado
  usuarios: 200
  duracao: 5m
  intervalo_entre_iteracoes: 1s
```

**Serve** quando o limite real é de sessão, não de chegada: pool de conexão,
licença por assento, fila com número fixo de trabalhadores, ou quando você está
reproduzindo um plano de JMeter escrito em threads.

**Mente** quando a pergunta é "o alvo aguenta X por segundo?". Mesmo alvo,
congelado por 3 s no meio de 12 s — à esquerda 100/s em modelo aberto, à direita
10 usuários em laço fechado:

```
1.200 requisicoes, 100 por segundo        850 requisicoes, 70 por segundo
metade 6.1 ms | 95% 2.41 s | pior 3.01 s  metade 6.4 ms | 95% 7.0 ms | pior 3.00 s
```

O 95% caiu de **2,41 s para 7,0 ms**. O laço fechado não errou conta nenhuma: ele
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
Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)      2.375     40 ms    598 ms    954 ms    1.03 s    1.04 s       0
  pagar fatura               (2)      2.375     40 ms     43 ms     43 ms    1.04 s    1.04 s       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.
  (2) tempo de resposta puro, contado de quando o passo anterior terminou.
```

Repare no `pagar fatura`: **43 ms no 95%**, com o alvo congelado por um segundo
inteiro. Sozinho, esse número é o mesmo tipo de mentira que uma ferramenta de laço
fechado produz.

> **Importante** A leitura que vale para quem usa o sistema é **A jornada
> inteira**, contada do instante agendado. Na mesma execução acima, ela mostra
> 675 ms.

## Critério de aceite

É o limite que você declara e que vira código de saída. Quatro escopos:

```yaml trecho
slo:
  - consultar pedido: { p95: < 150ms }              # um passo
  - jornada: { p95: < 2s, p99: < 5s }               # a espera inteira
  - global: { sucesso: ">= 99.9", taxa_efetiva: ">= 90/s" }
  - regressao: { jornada_p95: "<= 10% pior" }       # contra uma execucao anterior
```

O relatório mostra também **o que não foi declarado**, porque um gate que só mede
partes aprova cada pedaço sem dizer nada sobre a espera inteira:

```
SLO
  ok    Passou: "consultar pedido" respondeu 95% em ate 6 ms, dentro do limite de 150 ms.
  ok    Passou: a jornada inteira respondeu 95% em ate 12 ms, dentro do limite de 2000 ms.
  ok    Passou: o cenario inteiro teve taxa de sucesso de 100.00%, no minimo de 99.90%.
  --    regressao: sem criterio declarado — o gate aprova sem comparar com a execucao anterior
```

Nada disso é obrigatório: cenário sem bloco `slo` continua executando e
reportando, só não serve de gate.

## Variedade dos dados

Repetir a mesma requisição mil vezes mede o cache do alvo, não o alvo. O
relatório publica **o que aconteceu**, não o que foi declarado:

```
Ambiente
  100 valores distintos de pedidos.id em 100 usos, todos comecando com "CLI-A-"
  61 valores distintos de pedidos.valor em 100 usos, entre 10 e 89
```

Contagem de distintos responde "um valor ou muitos"; ela não responde **onde** os
valores caíram, e mil ids diferentes do mesmo cliente exercitam uma fatia só do
alvo. Por isso a linha traz também a faixa e o prefixo comum.

Se a fonte tem vários valores e a execução usou um só, o resultado é **inválido**:

```
RESULTADO INVALIDO: toda a carga caiu numa particao so de pedidos-cadeia; o resto do cluster
ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao
            kafka.particao.pedidos-cadeia tinha 4 valores disponiveis e a execucao usou 1, em 60 usos
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
Resultado invalido: a execucao nao mediu o que se propos a medir. Isto nao e veredito sobre o
alvo — e a medicao que nao vale, e por isso nenhuma regra de SLO foi avaliada.

  - nenhuma jornada chegou ao fim, entao o cenario nao exercitou a sequencia que declarou.
    Rode 'braunrate debug' para ver onde a iteracao para
    60 jornadas iniciadas, 0 completas
  - o passo "consultar pedido" falhou em 100% das requisicoes; nenhuma resposta bem-sucedida
    entrou na medicao dele
    60 requisicoes, 60 erros (status: 60)
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
