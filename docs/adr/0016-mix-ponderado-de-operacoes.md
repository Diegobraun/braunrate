# ADR 0016 — Mix ponderado de operacoes

Data: 2026-08-16
Status: aceito

## Contexto

Repetir a mesma chamada mede cache, nao sistema. Producao e uma distribuicao de
operacoes, e ate aqui a ferramenta nao tinha como declarar essa distribuicao.

A bateria adversarial registrou isso como bloqueio de adocao (achado 1.4.a). O
contorno que existia era rodar tres cenarios em tres processos, a 60/s, 30/s e
10/s, e tentar ler tres relatorios: **a proporcao saia certa e o numero do mix
nao existia**. Nao ha p95 do conjunto, nao ha taxa de erro do conjunto, e o gate
de SLO decide sobre uma operacao por vez.

O `peso` era recusado com uma mensagem que apontava um marco ja cumprido — "mix
ponderado de operacoes entra junto com o GraphQL", e o GraphQL entrou na Fase 6.

Este ADR decide tres coisas: **como a alternativa e escolhida**, **onde o peso
pode aparecer**, e **o que o relatorio diz sobre a proporcao que de fato saiu**.

## Decisao

### 1. A escolha e deterministica, por posicao no ciclo — nao e sorteio

Os pesos sao reduzidos pelo maximo divisor comum e viram um ciclo de tamanho
igual a soma reduzida. A iteracao N executa a alternativa da posicao
`N mod tamanho`. Para 60/30/10 o ciclo tem dez posicoes, seis de uma, tres de
outra e uma da terceira.

O ciclo **intercala** em vez de agrupar: cada copia de uma alternativa ocupa a
posicao `(k + 0,5) x total / copias`, e as posicoes sao ordenadas. Para 60/30/10
sai `0,1,0,0,1,2,0,0,1,0`. Agrupar daria a proporcao certa no fim e uma carga
que nenhum sistema recebe — durante o primeiro bloco a operacao cara nao existe
e o alvo aquece um caminho so.

**Por que nao sorteio com semente fixa.** O argumento a favor e que o sorteio
reproduz a variancia do trafego real. Ele nao se sustenta aqui, por duas razoes:

- **O gerador ja e deterministico em quando dispara.** O instante agendado sai da
  inversao da integral da funcao de taxa (ADR 0003), justamente para que o
  gerador nao acrescente ao numero uma variacao que o alvo nao causou. Ser
  deterministico no *quando* e aleatorio no *o que* seria incoerente: a mesma
  execucao passaria a carregar ruido de origem propria.
- **A comparacao entre execucoes e gate.** `braunrate compare` reprova regressao
  por diferenca de percentil. Com sorteio, duas execucoes do mesmo arquivo
  aplicam mixes ligeiramente diferentes, e parte da diferenca de p95 passa a ser
  do dado do gerador, nao do alvo. Um gate que treme sozinho e um gate que se
  aprende a ignorar.

A variancia que interessa medir esta no dado e no tempo de chegada, e os dois
ja variam: as fontes de dados variam por iteracao e a chegada segue a funcao de
taxa. Nao e preciso um dado de seis faces para isso.

Consequencia aceita: um alvo que reaja mal a um padrao ciclico especifico —
cache que se alinha com o periodo do ciclo, por exemplo — nao e exercitado por
esta forma. Fica registrado; se aparecer um caso real, a saida e uma opcao
declarada (`mix: sorteado`), nao mudar o padrao para todo mundo.

### 2. Peso e propriedade da alternativa, e alternativa nao tem cadeia dentro

O peso vai no passo, em `cenario:`. Cada iteracao executa **uma** alternativa.

```yaml
cenario:
  - nome: consulta leve
    peso: 60
    http: GET /pedidos
  - nome: consulta pesada
    peso: 30
    http: GET /pedidos/${pedidos.id}/detalhe
  - nome: criacao
    peso: 10
    http: { metodo: POST, caminho: /pedidos }
```

Duas recusas, com mensagem que ensina:

- **Peso em alguns passos e nao em outros.** Alternativa sem proporcao nao tem
  como ser escolhida, e arbitrar um peso para ela seria inventar a carga. Ou
  todos declaram, ou nenhum.
- **Peso onde ha captura encadeada.** O passo que usa `${valor}` rodaria numa
  iteracao em que o passo que captura nao rodou, e a referencia resolveria para
  vazio — o defeito 3.7.a por outro caminho. A mensagem nomeia os dois passos e
  a variavel, e diz as duas saidas: tirar o peso, ou separar as alternativas em
  cenarios diferentes.

**Por que nao um bloco `jornadas:` no topo.** A forma obvia para "60% da jornada
A, 30% da jornada B" seria uma lista de jornadas nomeadas, cada uma com peso e
passos. Foi considerada e recusada por enquanto:

- **O caso medido e outro.** O bloqueio registrado na bateria e mix de operacoes
  independentes. Mix de jornadas de varios passos e plausivel e nunca foi
  exercitado — e o projeto nao aposta no que alguem talvez queira.
- **Custo alto e espalhado.** Um segundo bloco de topo teria que ser mantido
  equivalente na DSL, no schema, no modo servidor, na comparacao e no relatorio,
  e mudaria o significado de "a jornada inteira" em todos eles.
- **Nao e beco sem saida.** `peso` esta definido como *proporcao da alternativa*,
  e um passo e o caso degenerado de uma jornada de um passo. Se `jornadas:`
  entrar, `peso` ali significa exatamente a mesma coisa — mesma palavra, mesma
  semantica, sem dois caminhos para a mesma ideia.

O criterio que destrava a decisao: alguem precisar de mix entre jornadas com
captura encadeada, com o caso concreto na mao. Ate la, a recusa acima e a
resposta, e ela diz o contorno.

### 3. A proporcao observada vai para o relatorio, ao lado da declarada

Peso de 60% que virou 45% na execucao e informacao, nao detalhe: e o que separa
o mix declarado do mix que de fato foi aplicado. Uma execucao cortada no meio de
um ciclo, ou requisicoes derrubadas pelo limite de concorrencia, deslocam a
proporcao — e sem essa linha ninguem descobre.

```
Mix declarado e observado
  consulta leve                60.0% declarado     60.0% observado (300 de 500)
  consulta pesada              30.0% declarado     30.0% observado (150 de 500)
  criacao                      10.0% declarado     10.0% observado (50 de 500)
```

O bloco so aparece quando o cenario declara mix. Sem mix, todo passo roda em
toda iteracao e a proporcao seria 100% em todas as linhas — informacao nenhuma,
ocupando espaco no bloco que as pessoas leem.

O JSON ganha `proporcao_declarada` por passo. O CSV nao muda: cabecalho de CSV e
contrato de quem processa a saida, e a informacao esta no JSON.

### 4. "A jornada inteira" passa a declarar que junta populacoes

Com mix, cada iteracao e uma alternativa, e o percentil de jornada junta
populacoes de custo diferente. E o achado 1.5.a da bateria acontecendo por
construcao, entao o relatorio diz:

```
  Cada jornada aqui e uma das 3 alternativas do mix, entao estes percentis juntam
  populacoes de custo diferente. Para ler cada uma, use a tabela por passo.
```

Nao suprimir o numero: ele e o tempo que a carga como um todo levou, e para taxa
de erro e volume o agregado esta certo. O que nao pode e ser lido como o tempo de
uma jornada tipica quando nao existe jornada tipica.

## Alternativas descartadas

- **Sorteio com semente fixa** — secao 1.
- **Bloco `jornadas:` no topo** — secao 2.
- **Manter tres cenarios em tres processos**: e o que existia, e produz tres
  relatorios que ninguem consegue somar. Desde o formato de resultado 2 eles
  *poderiam* ser somados (ADR 0003 §5), mas somar tres execucoes de cenarios
  diferentes continua recusado, e com razao: seriam tres planos de carga
  independentes, cada um com o proprio relogio.
- **Peso como taxa absoluta por passo** (`taxa: 60/s` no passo): duplicaria o
  plano de carga dentro do cenario e faria a soma dos passos contradizer
  `carga:`. A taxa e do cenario; a proporcao e do passo.

## Consequencias

- Com mix, uma iteracao e uma requisicao. A soma das contagens dos passos passa a
  ser igual ao total, e nao um multiplo dele — coberto por teste.
- O `debug` continua mostrando todas as alternativas, uma vez cada. E o que se
  quer ver antes de rodar carga, e a mensagem final ja diz quantos passos foram.
- Um cenario com mix nao tem jornada de varios passos. Quem precisa das duas
  coisas escreve dois cenarios, que e o que a mensagem de recusa diz.
- `examples/mix-de-operacoes.yaml` entra no laco do CI.
