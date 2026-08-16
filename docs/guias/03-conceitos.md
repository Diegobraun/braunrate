# Conceitos

Cinco ideias. Elas nao sao teoria: cada uma corresponde a uma linha que aparece
no relatorio, e saber le-las e a diferenca entre confiar no numero e repetir o
numero.

## Taxa, e por que o modelo aberto

**Taxa** e quantas requisicoes por segundo o gerador dispara. No braunrate ela e
uma decisao sua, declarada no arquivo, e o gerador insiste nela mesmo quando o
alvo demora:

```yaml trecho
carga:
  perfis:
    - rampa: { de: 100/s, ate: 800/s, durante: 5s }
    - patamar: { taxa: 800/s, durante: 10s }
    - pico: { taxa: 2000/s, durante: 3s }
```

Isso e o **modelo aberto**, e e o padrao. O outro jeito de descrever carga e o
**laco fechado**: N usuarios virtuais, cada um pedindo de novo so depois que a
resposta anterior chegou. E como JMeter e Locust medem.

A diferenca importa exatamente quando mais importa. No laco fechado, se o alvo
travar, os usuarios param de pedir junto — e o atraso nunca entra na conta. Um
usuario de verdade nao faz isso: ele chega quando ia chegar, e espera.

O braunrate conta o tempo de resposta **do instante em que a requisicao deveria
ter partido**, nao de quando ela partiu. Por isso uma travada do alvo aparece no
numero em vez de sumir dele. O nome disso e omissao coordenada, e da para ver os
dois lados na sua maquina com `braunrate demo --com-falha`.

### O modelo fechado existe, declarado

```yaml trecho
carga:
  modelo: fechado
  usuarios: 200
  duracao: 5m
  intervalo_entre_iteracoes: 1s
```

**Serve** quando o limite real e de sessao, nao de chegada: pool de conexao,
licenca por assento, fila com numero fixo de trabalhadores, ou quando voce esta
reproduzindo um plano de JMeter escrito em threads.

**Mente** quando a pergunta e "o alvo aguenta X por segundo?". Mesmo alvo,
congelado por 3 s no meio de 12 s — a esquerda 100/s em modelo aberto, a direita
10 usuarios em laco fechado:

```
1.200 requisicoes, 100 por segundo        850 requisicoes, 70 por segundo
metade 6.1 ms | 95% 2.41 s | pior 3.01 s  metade 6.4 ms | 95% 7.0 ms | pior 3.00 s
```

O 95% caiu de **2,41 s para 7,0 ms**. O laco fechado nao errou conta nenhuma: ele
mediu com precisao um evento que ele mesmo deixou de provocar. Repare tambem na
taxa, 100/s contra 70/s — num teste de capacidade, e a carga que deveria ter
continuado.

Por isso o relatorio do modelo fechado abre com aviso, sempre, mesmo quando tudo
passa; o documento JSON **nao tem** campo de tempo corrigido, porque sem instante
agendado nao ha o que corrigir; e `braunrate compare` recusa comparar uma
execucao aberta com uma fechada.

## "95% das respostas em ate X"

O relatorio nao mostra media. Media esconde: se 95 respostas levam 5 ms e 5 levam
2 segundos, a media da 105 ms e ninguem percebe as cinco lentas.

"95% em ate 6,2 ms" quer dizer que 5% das pessoas esperaram mais que 6,2 ms. Os
cortes que aparecem sao metade (50%), 95%, 99%, 99,9% e a pior.

### O tempo do passo 2 em diante nao e corrigido

So o primeiro passo tem instante agendado proprio. Os seguintes dependem de um
valor capturado antes deles, entao comecam quando o passo anterior termina. Esta
execucao real, contra o alvo embutido congelado por 1 s no meio, mostra o tamanho
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
inteiro. Sozinho, esse numero e o mesmo tipo de mentira que uma ferramenta de
laco fechado produz. **A jornada inteira**, contada do instante agendado, mostra
675 ms — e essa e a leitura que vale para quem usa o sistema.

## Criterio de aceite

E o limite que voce declara e que vira codigo de saida. Quatro escopos:

```yaml trecho
slo:
  - consultar pedido: { p95: < 150ms }              # um passo
  - jornada: { p95: < 2s, p99: < 5s }               # a espera inteira
  - global: { sucesso: ">= 99.9", taxa_efetiva: ">= 90/s" }
  - regressao: { jornada_p95: "<= 10% pior" }       # contra uma execucao anterior
```

O relatorio mostra tambem **o que nao foi declarado**, porque um gate que so mede
partes aprova cada pedaco sem dizer nada sobre a espera inteira:

```
SLO
  ok    Passou: "consultar pedido" respondeu 95% em ate 6 ms, dentro do limite de 150 ms.
  ok    Passou: a jornada inteira respondeu 95% em ate 12 ms, dentro do limite de 2000 ms.
  ok    Passou: o cenario inteiro teve taxa de sucesso de 100.00%, no minimo de 99.90%.
  --    regressao: sem criterio declarado — o gate aprova sem comparar com a execucao anterior
```

Nada disso e obrigatorio: cenario sem bloco `slo` continua executando e
reportando, so nao serve de gate.

## Variedade dos dados

Repetir a mesma requisicao mil vezes mede o cache do alvo, nao o alvo. O
relatorio publica **o que aconteceu**, nao o que foi declarado:

```
Ambiente
  100 valores distintos de pedidos.id em 100 usos, todos comecando com "CLI-A-"
  61 valores distintos de pedidos.valor em 100 usos, entre 10 e 89
```

Contagem de distintos responde "um valor ou muitos"; ela nao responde **onde** os
valores cairam, e mil ids diferentes do mesmo cliente exercitam uma fatia so do
alvo. Por isso a linha traz tambem a faixa e o prefixo comum.

Se a fonte tem varios valores e a execucao usou um so, o resultado e **invalido**:

```
RESULTADO INVALIDO: toda a carga caiu numa particao so de pedidos-cadeia; o resto do cluster
ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao
            kafka.particao.pedidos-cadeia tinha 4 valores disponiveis e a execucao usou 1, em 60 usos
```

Isso nasceu de um defeito nosso: a autenticacao congelava os dados da primeira
iteracao, e toda execucao autenticada com CSV rodava sobre a primeira linha
enquanto o relatorio anunciava variedade que nao existiu.

## Resultado invalido

Toda execucao passa por uma verificacao de sanidade **antes** de o criterio de
aceite ser lido. Ela nao pergunta se o alvo foi bem; pergunta se a execucao mediu
o que se propos a medir. Quando a resposta e nao, o criterio nem chega a ser
avaliado e o comando sai com **codigo 3**:

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

| Caso | Por que o numero nao vale |
|---|---|
| nenhuma jornada chegou ao fim | o cenario nao exercitou a sequencia que declarou |
| todos os passos falharam, ou um passo falhou em 100% | o tempo medido e o de recusar, nao o de fazer |
| a carga declarada nao foi aplicada inteira | so o pedaco que rodou ficou medido |
| um passo declarado nao registrou amostra | ele ficou de fora da medicao |
| variedade colapsada em fonte com varios valores | o alvo pode ter respondido de cache |
| o gerador nao sustentou a taxa | os numeros medem o gerador, nao o alvo |

**Codigo 3 e diferente de codigo 1.** `1` e "o alvo nao atendeu o criterio"; `3`
e "esta execucao nao serve para afirmar nada".

A verificacao vale sempre, com ou sem bloco `slo`. Ela nasceu de tres defeitos da
mesma familia — dados congelados na primeira iteracao, o proprio `examples/ci.yaml`
rodando 100% de 401 e passando verde desde a Fase 1, e a variedade que so foi
conferida quando alguem pediu. Os tres eram execucao sintaticamente perfeita,
semanticamente vazia, com a suite inteira verde.

## Uma ressalva permanente: um token para a execucao inteira

O motor faz login uma vez e reaproveita a credencial em todas as jornadas — isso
nao existe em producao. Se o alvo tiver cache por identidade, rate limit por
token ou sharding por usuario, o numero fica otimista (ou falha por 429 que nao
aconteceria). O relatorio declara isso em toda execucao com autenticacao.
