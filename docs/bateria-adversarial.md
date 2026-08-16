# Bateria adversarial

Avaliacao feita de fora para dentro, na postura de quem recebe a ferramenta para
decidir se recomenda ao time. Achado vale mais que verde.

> **O que esta bateria nao cobre.** Quem a executou conhece o codigo por dentro e
> completa caminho que uma pessoa de fora abandonaria. Onde alguem desiste, o que
> procura e nao acha, o que entende errado — nada disso aparece aqui. A ultima
> secao lista o que continua descoberto, e ela e o roteiro da sessao com o QA de
> verdade.

Cada registro responde tres perguntas, nesta ordem: **o numero esta certo**, **a
ferramenta se explica**, **da para agir**.

Ambiente: Mac darwin/arm64, 10 nucleos, Go 1.26.6, braunrate 0.4.0.
Alvo embutido em `127.0.0.1:8080`; Redpanda em `127.0.0.1:9092`.

---

## Bloco 1 — Jornadas reais de negocio

### 1.1 — E-commerce: entrar, buscar, carrinho, fechar, consultar status
**Comportou bem, com dois achados**

**Montagem**: `braunrate new` para ver a forma, `braunrate import curl` do login para
ter o esqueleto, e depois **os outros cinco passos escritos a mao**.
**Custo**: 5 comandos, 1 edicao grande (o arquivo inteiro reescrito).

O `import curl` acertou o que importa: viu `"senha"` no corpo e transformou em
`${senha}` vindo de `${SENHA}`, com o aviso na tela. Ninguem versiona credencial por
acidente aqui.

O `debug` fecha a jornada de seis passos mostrando cada captura encadeada:

```
passo 5 — fechar pedido   [ok em 200µs]
  requisicao: POST /carrinhos/car-67055ead/fechar
  resposta:   status 201, 58 bytes
  corpo:      {"pedido": {"id": "ped-3dfd0f9c", "status": "CONFIRMADO"}}
  capturou:
    pedidoId = ped-3dfd0f9c
```

Sob carga, 500 jornadas a 50/s, tudo verde, e a variedade observada prova que a
cadeia de capturas funcionou de verdade:

```
  500 valores distintos de carrinhoId em 500 usos, todos comecando com "car-"
  500 valores distintos de pedidoId em 500 usos, todos comecando com "ped-"
  1 unico valor de sku em 500 usos
```

**O numero esta certo**: sim. Cada iteracao abriu o proprio carrinho e fechou o
proprio pedido — se a captura tivesse congelado, `carrinhoId` apareceria com um
valor so, que e exatamente o bug de identidade da Fase 4.

---

#### Achado 1.1.a — corpo declarado como `{}` virava aviso de campo vazio
**Gravidade**: media — nao afeta o numero, corroi o bloco que carrega os avisos que importam

**Aconteceu**: dois passos legitimos (`corpo: {}` num POST que nao tem corpo) saiam
como defeito, com a evidencia comecando por dois-pontos sem nome de campo:

```
  Atencao: o corpo de "abrir carrinho" saiu com campo vazio; se isso nao for proposital,
           o alvo exercitou um caminho que producao nao ve
            : objeto vazio
```

**Por que importa**: um relatorio saudavel saia com dois avisos que nao tinham nada a
dizer. Quem aprende a pular esse bloco pula junto os avisos reais — e esse bloco e
onde mora a invalidacao de resultado. E a mesma familia do ADR 0007 item 4: nao se
avisa sobre o que a pessoa declarou.

**Corrigido agora**: corpo sem campos vira `corpo sem campos`, que nao e marca de
defeito; so campo **com nome** que chegou vazio conta. Testes
`TestBodyDeclaredEmptyIsNotAnEmptyField` e `TestFieldThatCameBlankIsStillWarned`,
o primeiro provado contra o codigo anterior:

```
--- FAIL: TestBodyDeclaredEmptyIsNotAnEmptyField (0.00s)
    shape_test.go:75: a forma do corpo vazio saiu com nome de campo em branco: ": objeto vazio"
```

---

#### Achado 1.1.b — nao ha caminho de entrada para jornada de varios passos
**Gravidade**: media — nao afeta o numero, atrasa quem monta

**Aconteceu**: `import curl` traduz **uma** requisicao. Uma jornada de negocio tem
cinco ou seis, encadeadas por captura. Nao existe `import curl` que aceite varias
chamadas, nem `record` que ja saia com as capturas ligadas — entao os outros cinco
passos foram escritos a mao, com a estrutura copiada do primeiro.

**Por que importa**: a Fase 2.5 promete que se cria cenario funcional sem ler
documentacao alem das mensagens da ferramenta. Para **um** passo isso e verdade.
Para uma jornada, a pessoa precisa descobrir sozinha a forma de `captura:`, de
`${variavel}` e de `verificar:` — o `new` mostra, mas comentado e so para um passo.

**Nao corrigido**: e mudanca de escopo, nao defeito. Entra na lista de bloqueio de
adocao, e e o primeiro item para observar na sessao com a pessoa de verdade: **o que
ela faz quando o segundo passo precisa do id que o primeiro devolveu.**

---

### 1.2 — Banco: autenticar, saldo, transferir com idempotencia, repetir, extrato
**Achou um defeito grave e depois comportou bem**

**Montagem**: escrito a mao (o `import curl` nao ajuda com cinco passos — achado
1.1.b). **Custo**: 3 comandos, 2 edicoes.

O que a jornada exercita e o valor **estavel dentro da jornada**: a chave de
idempotencia dos passos 3 e 4 tem que ser a mesma, e nova na jornada seguinte. E o
que o `debug` mostra, e o que a variedade confirma sob carga:

```
passo 4 — repetir transferencia   [ok em 100µs]
  requisicao: POST /transferencias
              Idempotency-Key: 2ad3f3c5-6e97-2f73-b612-6b688be1656a
  resposta:   status 200, 96 bytes
  corpo:      {"transferencia": {"id": "tra-a9a33783", ...}, "repetida": true}
```

```
  400 valores distintos de contas.chave em 400 usos
  400 valores distintos de contas.numero em 400 usos
```

Um unico erro em 2.000 requisicoes, e ele era do mock (chave repetida de uma
execucao anterior, que ficou na memoria dele). A ferramenta nomeou com precisao:

```
Erros
  passo                      o que aconteceu                    quantidade   exemplo
  transferir                 status HTTP inesperado                      1   esperava status 201, recebeu 200
```

**Da para agir**: sim. Passo, classe, quantidade e um exemplo concreto na mesma
linha — foi o que permitiu descartar em segundos que o defeito fosse da ferramenta.

---

#### Achado 1.2.a — `padrao(...)` na forma inline gerava vazio, em silencio
**Gravidade**: ALTA — afeta confianca no numero

**Aconteceu**: a jornada quebrou no passo 2, e o `debug` mostrou por que:

```
passo 1 — autenticar   [ok em 5.1ms]
  requisicao: POST /auth/login
              corpo: {"senha":"segredo","usuario":""}

passo 2 — consultar saldo   [FALHOU em 1.5ms]
  requisicao: GET /contas//saldo
  resposta:   status 404
```

Isolado num cenario so de geradores, o defeito e este:

```
  amostra.declarado = BR-662044          # { tipo: padrao, formato: "BR-######" }
  amostra.inline_padrao =                # padrao(BR-######)
  amostra.inline_inteiro = 11
  amostra.inline_uuid = 8755a4cd-4d52-eda6-a794-b044930d26ac
  amostra.inline_texto = pwjbaubm
  amostra.inline_cpf = 04160952674
```

Todo gerador inline funciona **menos** `padrao(...)`. A causa: o formato chega de
dois jeitos — `formato:` no mapa e argumento entre parenteses — e o codigo lia so o
primeiro. Sem argumento lido, `fromPattern("")` devolve string vazia, sem erro.

**Por que e grave**: e a familia que a ferramenta existe para matar. Valor em branco
sai para o alvo, o alvo responde 404 ou 401, e nada na saida liga uma coisa a outra.
Numa jornada de um passo so, contra um alvo tolerante, isso **passa verde**: a
verificacao de sanidade so pega depois, pela variedade colapsada, e ai a mensagem
fala de variedade, nao do gerador quebrado. Foi o proprio `debug` que salvou aqui —
mas `debug` e opcional.

**Corrigido agora**: o gerador le o argumento inline, e `padrao()` sem formato
nenhum passa a recusar ensinando as duas formas. Tres testes, os tres provados
contra o codigo anterior:

```
--- FAIL: TestInlinePatternProducesTheValueAndNotSilence (0.00s)
    generators_test.go:231: padrao(BR-######) devolveu vazio: valor em branco sai para o alvo sem ninguem ser avisado
--- FAIL: TestBothShapesOfThePatternAgree (0.00s)
--- FAIL: TestInlinePatternWithoutFormatSaysWhatIsMissing (0.00s)
```

**O que isso diz sobre a suite**: havia teste para `{ tipo: padrao }` sem formato e
teste para `padrao` com `Format` preenchido. Nao havia teste da forma inline, que e
a que os proprios exemplos e mensagens sugerem. Duas formas de declarar a mesma
coisa, uma coberta.

---

### 1.3 — Cadeia assincrona: criar por HTTP, evento no Kafka, esperar o status virar, conferir atraso
**A medicao mais honesta da bateria ate aqui, e o unico numero que ninguem pode barrar**

**Montagem**: `braunrate new` (saida **inteiramente descartada**: nao mostra
`mensageria`, nem `kafka`, nem `aguardar`), depois 34 linhas escritas a mao.
**Custo**: 3 comandos, 1 escrita completa.

Alvo: mock HTTP em 8090, Redpanda em 9092, e um processador em Go que consome
`pedidos-eventos` no grupo `processador` e vira o pedido para PROCESSADO — de
proposito mais lento que o produtor.

O `debug` fecha os tres passos e, no terceiro, **declara a propria imprecisao antes
de alguem perguntar**:

```
passo 3 — esperar processamento   [ok em 503.4ms]
  requisicao: aguardar em GET /pedidos/ped-a7cbfc17 ate $.pedido.status = "PROCESSADO"
              sondando a cada 500ms, desiste depois de 30s
              a latencia medida tem a granularidade da sondagem
```

O processamento real leva 3ms; o passo mede 503ms. A ferramenta nao finge que 503
e o tempo do sistema — e a mesma frase reaparece no relatorio de carga, nao so no
`debug`:

```
  Atencao: o passo "esperar processamento" espera sondando a cada 500ms: a latencia dele
           tem essa granularidade e fica maior que a real, nunca menor
```

**O numero esta certo**: sim, e o motivo e essa frase. Um numero que so pode errar
para cima, dizendo que so pode errar para cima, e utilizavel. O mesmo numero sem a
frase seria uma mentira de duas ordens de grandeza.

#### O atraso do consumidor mede o que promete

Com o produtor a 400/s contra um consumidor de ~200/s:

```
Atraso do consumidor
  grupo processador em pedidos-eventos: no pior momento 1.751 mensagens atras; no fim, 1.751 mensagens
  O consumidor terminou a execucao para tras: a fila cresceu mais rapido do que ele consumiu.
```

E o pico nao e o valor final disfarcado — com carga de 400/s por 5s seguida de 10/s
por 20s, a fila enche e drena, e os dois numeros se separam:

```
  grupo processador em pedidos-eventos: no pior momento 886 mensagens atras; no fim, 0 mensagens
```

A segunda frase, a de terminar para tras, sumiu sozinha. O JSON traz `atraso_maximo`,
`atraso_no_fim`, `atraso_maximo_por_particao` e `leituras`.

#### A concentracao de particao esta certa nos dois sentidos

Testei os dois casos que a ferramenta precisa separar. Topico de 3 particoes com
chave que varia:

```
  3 valores distintos de kafka.particao.eventos-particionado em 400 usos, entre 0 e 2
```

Mesmo topico, chave constante — resultado **invalidado**, exit 3:

```
Resultado invalido: a execucao nao mediu o que se propos a medir.
  - toda a carga caiu numa particao so de eventos-particionado; o resto do cluster ficou
    parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao
    kafka.particao.eventos-particionado tinha 3 valores disponiveis e a execucao usou 1, em 400 usos
```

E num topico que **de fato** tem uma particao so, a mesma concentracao sai como
linha neutra de ambiente, sem aviso. Distinguir "voce colapsou a chave" de "o topico
e assim" e o que separa um aviso util de um alarme que se aprende a ignorar.

---

#### Achado 1.3.a — o atraso do consumidor nao pode virar criterio de aceite
**Gravidade**: media-alta — nao afeta o numero, mas o veredito ignora o unico numero que importa nesta jornada

**Aconteceu**: a execucao a 400/s termina com o consumidor 1.751 mensagens atras, o
relatorio diz isso com todas as letras, e o veredito e:

```
Passou: a unica regra de SLO foi atendida.
```

Exit 0. Em CI, isso sobe.

Tentei declarar o criterio:

```
slo:
  - atraso: { processador: < 100 }
```

```
erro no cenario: lag-slo.yaml:33:28: metrica de slo desconhecida: "processador"
    disponiveis: p50, p75, p90, p95, p99, p99.9, max, erros, sucesso, taxa_efetiva
```

**Por que importa**: numa cadeia assincrona, latencia de resposta e taxa de erro sao
os numeros faceis, e nenhum dos dois reprova um pipeline que nao acompanha. A
pergunta de aceite e "o consumidor aguenta o pico?" — a ferramenta responde e nao
deixa ninguem barrar pela resposta. Pior, o bloco de SLO lista o que ficou sem
criterio (`passos sem criterio declarado`, `regressao`) e **o atraso nao aparece nem
nessa lista**: quem le nao descobre que deixou de gatear alguma coisa.

**Nao corrigido**: e recurso novo, e nada novo entra antes da sessao com o QA. O
paliativo existe e funciona — `execucao.atraso_do_consumidor` esta no JSON, entao da
para gatear por fora com `jq`. Por isso "atrasa", nao "bloqueia". Primeiro item de
recurso da lista priorizada.

---

#### Achado 1.3.b — `validate` diz "Cenario valido" para um cenario que nao tem como rodar
**Gravidade**: media — `validate` e o portao barato do CI, e ele aprova o que `debug` reprova

**Aconteceu**: tirei o bloco `mensageria` do cenario, deixando um passo Kafka sem
broker em lugar nenhum — nem no passo, nem em `mensageria`, e com `alvo` HTTP.

```
$ braunrate validate sem-mensageria.yaml
Cenario valido: "Pedido assincrono", 3 passos, 400 iteracoes em 10s.
exit=0
```

```
$ braunrate debug sem-mensageria.yaml
passo 2 — publicar evento   [FALHOU em 0s]
  problema:   erro de configuracao do cenario
              sem broker: declare 'brokers' no passo ou aponte o alvo do cenario para kafka://host:9092
exit=1
```

**Por que importa**: os tres fatos que provam a impossibilidade sao estaticos — o
passo nao tem `brokers`, nao existe `mensageria.kafka`, e o `alvo` nao e `kafka://`.
`validate` tem os tres na mao e ainda assim assina embaixo. E a mesma familia do
A8 (variavel nao declarada), que ja e recusada na validacao. A mensagem do `debug`
ensina bem; ela so chega no portao errado.

**Corrigido agora** — ver commit; a regra do A8 passa a valer tambem para broker.

---

### 1.4 — GraphQL com mix de operacoes 60/30/10
**Nao executavel como pedido. O contorno funciona e custa tres relatorios que ninguem consegue somar**

**Montagem tentada**: um cenario, tres passos, `peso: 60 / 30 / 10`.

```
$ braunrate validate mix.yaml
erro no cenario: mix.yaml:8:5: a chave "peso" ainda nao existe: mix ponderado de operacoes entra junto com o GraphQL
exit=2
```

O GraphQL ja entrou — esta na lista de protocolos compilados de todo relatorio
desta bateria. A mensagem promete um marco que ja passou.

**Montagem que rodou**: tres cenarios separados a 60/s, 30/s e 10/s, tres processos
simultaneos. **Custo**: 3 arquivos escritos a mao, 7 comandos.

```
Mix - consulta leve      1.200 requisicoes em 20s, 60 por segundo, 0% de erro
Mix - consulta pesada      600 requisicoes em 20s, 30 por segundo, 0% de erro
Mix - mutacao              200 requisicoes em 19.9s, 10 por segundo, 0% de erro
```

A proporcao sai exata e cada processo respeita a propria taxa. O GraphQL agrega por
nome de operacao, que e o que permite ler passo a passo:

```
passo 1 — graphql RelatorioMensal   [ok em 9.1ms]
  requisicao: query RelatorioMensal em POST /graphql
              variaveis: {"mes":"1"}
```

**O numero esta certo**: cada um dos tres esta. O numero do mix nao existe.

---

#### Achado 1.4.a — nao ha mix ponderado, e a mensagem que recusa aponta o marco errado
**Gravidade**: media-alta — bloqueia o cenario mais comum de teste de capacidade

**O estado real**: `peso` e recusado; `escolher:`, que o [ADR 0002](adr/0002-modelo-de-cenario.md)
promete para a Fase 4 como o lugar certo do peso, **nao existe em lugar nenhum do
codigo**. E `docs/estudo-ferramentas.md` lista "suporte a mix ponderado de operacoes
(60% consulta leve, 30% pesada, 10% mutacao)" como requisito, com esses numeros.

Tres textos, tres estados diferentes:
- ADR 0002: entra na Fase 4, com `escolher:`, e `peso` solto vira erro apontando `escolher:`
- a mensagem do codigo: entra junto com o GraphQL (que ja entrou), e nao cita `escolher:`
- o estudo de ferramentas: e requisito

**Por que importa**: nao e so a falta do recurso. E a mensagem que manda a pessoa
esperar por um marco que ja passou — quem le conclui que a proxima versao resolve, e
nao ha proxima versao para isso. Uma recusa que ensina o caminho errado custa mais
que uma recusa seca.

**O contorno e real e tem preco**: tres processos dao a proporcao exata, mas
- nao existe p95 do mix; existem tres p95, e capacidade se decide pelo do conjunto
- cada relatorio aprova o proprio SLO sozinho; nada reprova a soma
- `compare` recebe duas execucoes, nao tres, e `report` recebe uma

**Nao corrigido**: recurso novo, e nada novo entra antes da sessao com o QA. A
mensagem, essa, e correcao de texto e nao de comportamento — mas mexe no que a
pessoa le, e por isso vai na lista, nao no commit de hoje.

---

#### Achado 1.4.b — argumento a mais e ignorado em silencio
**Gravidade**: media — a ferramenta faz menos do que foi pedida e nao diz

Tres comandos, o mesmo padrao:

```
$ braunrate report mix-leve.json mix-pesada.json
relatorio em relatorio.html          # so o primeiro arquivo entrou; nada avisa
exit=0

$ braunrate compare mix-leve.json mix-pesada.json mix-mutacao.json
  antes:  Mix - consulta leve ...    # o terceiro sumiu sem uma palavra
  depois: Mix - consulta pesada ...

$ braunrate new --help
cenario de partida em cenario.yaml   # a flag foi ignorada e um arquivo foi criado
```

**Por que importa**: quem passa tres arquivos para `compare` acredita que comparou
tres. O relatorio nao mente sobre o que mediu — ele so nao diz que mediu menos do que
foi pedido. E o `new --help`, especifico, e a unica forma da ferramenta criar arquivo
quando a pessoa estava pedindo ajuda. (Ele nao sobrescreve: com `cenario.yaml` ja
existente, recusa e sugere outro nome.)

---

#### O que se comportou bem: comparar duas execucoes que nao se comparam

`compare` entre dois cenarios diferentes nao finge:

```
O que pode explicar a diferenca sem ser o servico
  - os cenarios sao diferentes: "Mix - consulta leve" e "Mix - mutacao" (isso sozinho explica a diferenca)
  - os planos de carga sao diferentes: patamar ate 60/s por 20s e patamar ate 10/s por 20s (isso sozinho explica a diferenca)
  Duas execucoes nao dao intervalo de confianca: variacao abaixo de 5% e tratada como ruido.
```

E a tabela por passo marca `(nao existe mais)` e `(passo novo)` em vez de exibir
"100% melhor" solto.

---

### 1.5 — Ramificacao por dado: pessoa fisica e pessoa juridica no mesmo cenario
**Roda. O relatorio junta duas populacoes numa linha so e nao avisa**

Nao existe passo condicional (`escolher:` do ADR 0002 nao existe no codigo — achado
1.4.a). O contorno honesto e por dado: a rota vem do CSV.

```csv
id,tipo,rota,limite
1,pf,pessoas,5000
2,pj,empresas,90000
```

```yaml
  - nome: consultar limite
    http: { metodo: GET, caminho: /${clientes.rota}/${clientes.id}/limite }
    verificar:
      status: 200
      json: { $.tipo: "${clientes.tipo}" }
```

**Custo**: 2 arquivos escritos a mao, 2 comandos. Roda de primeira, verde, exit 0.

No alvo, pessoa juridica custa 20ms a mais que pessoa fisica — de proposito. O
relatorio:

```
Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar limite           (1)        500    1.6 ms     31 ms     32 ms     33 ms     33 ms       0
```

Uma linha. E a ferramenta **sabe** que a rota variou, porque diz isso na variedade:

```
  2 valores distintos de clientes.rota em 500 usos
  2 valores distintos de clientes.tipo em 500 usos
```

Rodei os dois lados separados para ver o que a linha unica escondia:

```
pf     Metade das respostas em ate 1.1 ms; 95% em ate 1.9 ms; 99% em ate 2.8 ms
pj     Metade das respostas em ate 29 ms;  95% em ate 31 ms;  99% em ate 32 ms
```

**O numero esta certo**: sim, os percentis do conjunto estao corretos. **A leitura,
nao.** Quem le "metade 1.6 ms, 95% 31 ms" conclui cauda ruim num servico rapido, e
vai procurar contencao, GC ou vizinho barulhento. Nao ha cauda: ha duas populacoes,
uma de 1.9 ms e outra de 31 ms, e a proporcao entre elas foi declarada no CSV. O 31
ms da linha unica e o p95 de pessoa juridica usando o nome do passo inteiro.

---

#### Achado 1.5.a — cauda que na verdade e mistura de populacoes nao e sinalizada
**Gravidade**: media-alta — nao afeta o numero, direciona a investigacao para o lugar errado

**Por que a agregacao esta certa e o silencio nao**: agregar pelo caminho do
template e a decisao correta — resolver `${clientes.id}` daria 10 passos, e com um
UUID daria 500. O que falta e a frase. A ferramenta tem os dois lados na mao no
mesmo relatorio: sabe que `${clientes.rota}` tem 2 valores e sabe que p50 e p95 estao
a 20x de distancia. Nao existe hoje nada que ligue as duas coisas.

Nao existe tambem como declarar criterio por ramo: `- consultar limite: { p95: < 5ms }`
reprovaria pessoa juridica por ser pessoa juridica, e `< 50ms` aprovaria uma
regressao de 25x em pessoa fisica.

**Nao corrigido**: sinalizar mistura de populacoes e recurso, e vale a pena discutir
o desenho com o QA antes — a alternativa obvia (agregar por caminho resolvido)
e justamente a que o ADR 0002 recusa com razao. Segundo item de recurso da lista.

**O que salva hoje**: a variedade observada esta no mesmo relatorio, tres linhas
abaixo. Quem sabe procurar, acha. Quem nao sabe, nao tem por que procurar.

---

### Bloco 1 — fechamento

| jornada | executou? | comandos | arquivos escritos a mao | achado mais grave |
|---|---|---|---|---|
| 1.1 e-commerce, 6 passos | sim | 5 | 1 (5 dos 6 passos) | 1.1.a media (corrigida) |
| 1.2 banco com idempotencia | sim | 3 | 1 | **1.2.a ALTA (corrigida)** |
| 1.3 cadeia assincrona | sim | 3 | 1 (34 linhas) | 1.3.a media-alta |
| 1.4 mix 60/30/10 | **nao, como pedido** | 7 | 3 | 1.4.a media-alta |
| 1.5 ramificacao por dado | sim, com contorno | 2 | 2 | 1.5.a media-alta |

Cinco jornadas, **nenhuma** montada so pelos caminhos de entrada da ferramenta. O
`import curl` cobriu um passo de trinta e um; o `new` cobriu a forma do arquivo e
nada dos protocolos de mensageria. Todo o resto foi escrito a mao — em quatro das
cinco, com estrutura que so o codigo-fonte ou os exemplos do repositorio ensinam.

Duas correcoes entraram com teste que reprova o codigo anterior: o gerador `padrao`
inline (alta) e o aviso de corpo vazio (media). Uma terceira entrou por consequencia
do 1.3.b: `validate` deixa de aprovar passo de broker sem endereco.

---

## Bloco 2 — O que quebra na vida real

Doze modos de falha que aparecem em homologacao de verdade. Alvos dedicados por
porta, para nao interferirem entre si.

### 2.1 — Alvo fora do ar antes da execucao
**Correto.** exit 3, resultado invalidado, causa nomeada.

```
Resultado invalido: a execucao nao mediu o que se propos a medir.
  - nenhuma jornada chegou ao fim ...  100 jornadas iniciadas, 0 completas
  - o passo "consultar" falhou em 100% das requisicoes ...  100 requisicoes, 100 erros (rede: 100)

Erros
  consultar                  falha de rede                             100   connection refused
```

### 2.2 — Alvo morre no meio da execucao
**Correto.** 50% de erro, exit 1 pelo criterio de erro declarado, `connection refused`
com quantidade exata (300 de 600).

### 2.3, 2.4 — Alvo degradado: falha rapida com resposta lenta no que sobra
**Aqui saiu o pior achado da bateria.** Ver 2.4.a abaixo.

### 2.5 — Host que nao resolve
**Correto.** exit 3, `no such host`.

### 2.6 — Certificado nao confiavel
**Bloqueia adocao.** Ver 2.6.a.

---

#### Achado 2.4.a — o gate aprovava latencia num alvo 98% quebrado
**Gravidade**: ALTA — afeta confianca no numero, e o veredito era verde

**Aconteceu**: alvo que responde 503 instantaneo em 98% das requisicoes e 200 em
300ms nos 2% restantes. Cenario com um criterio so: `- consultar: { p95: < 200ms }`.

```
Passou: a unica regra de SLO foi atendida.

  500 requisicoes em 10s, 50 por segundo, 98.00% de erro
  Metade das respostas em ate 0.424 ms; 95% em ate 1.4 ms; 99% em ate 301 ms

SLO
  ok    Passou: "consultar" teve latencia p95 de 1 ms, dentro do limite de 200 ms.
exit=0
```

Em CI isso sobe. O p95 de 1 ms e o tempo de servir uma pagina de erro.

**Por que a verificacao de sanidade nao pegou**: ela invalida passo com **100%** de
erro — "a latencia acima e o tempo que o alvo levou para recusar, nao o tempo do
trabalho", diz o proprio comentario no codigo. O principio nao tem penhasco em 100%.
Com 98%, dez jornadas completas em quinhentas, nada disparou.

**Por que o p95 e nao so o p50**: as falhas sao rapidas e ocupam a base da
distribuicao. Com 90% de falha os sucessos ainda ocupam de p90 para cima e o p95
reprovou corretamente (307 ms). Com 98% eles ocupam so de p98 para cima, e o p95 cai
inteiro dentro das falhas. Nao existe percentil seguro: existe a fracao de sucesso
que o empurra para fora.

**Corrigido agora**: enquanto as requisicoes que funcionaram forem **maioria** da
amostra, o percentil continua descrevendo-as e nada muda. Abaixo disso o criterio de
latencia nao e avaliado, e o gate nao aprova o que nao verificou:

```
Nao avaliada: consultar teve 98% de falha, entao a latencia p95 acima e sobretudo o
tempo de falhar, nao o tempo do trabalho. A regra "p95: < 200ms" nao pode ser
verificada sobre esta amostra, e sem verificacao o gate nao aprova.
exit=1
```

Vale igual para a jornada: o histograma de jornada registra tambem as que foram
interrompidas, entao um cenario que aborta no primeiro passo em 1 ms aprovaria um
`jornada: { p95: < 2s }` que ninguem esperou.

Quatro testes; os dois que provam o defeito reprovam o codigo anterior:

```
--- FAIL: TestLatencyRuleIsNotApprovedOverASampleOfFailures (0.00s)
    slo_test.go:138: o gate aprovou latencia num passo que falhou em 98% das requisicoes
--- FAIL: TestJourneyRuleIsNotApprovedWhenMostJourneysAbort (0.00s)
    slo_test.go:205: o gate aprovou a jornada com 490 de 500 jornadas interrompidas
```

Os outros dois travam o contrario: 30% de erro continua avaliando latencia
normalmente, e criterio de **erro** continua sendo avaliado justamente quando tudo
falha — se ele tambem parasse, nao sobraria criterio nenhum.

**O que nao foi mudado, de proposito**: o percentil continua sendo calculado sobre
todas as requisicoes. Separar sucesso de falha exigiria um segundo histograma por
passo e mudaria numero ja publicado; e uma decisao de desenho para a sessao com o
QA, nao para uma correcao de bateria.

---

#### Achado 2.6.a — nao existe configuracao de TLS para HTTP
**Gravidade**: ALTA para adocao — homologacao com CA interna nao pode ser testada

Alvo HTTPS com certificado autoassinado:

```
$ braunrate debug tls.yaml
  problema:   falha de rede
              Get "https://127.0.0.1:8443/produtos": tls: failed to verify certificate:
              x509: certificate signed by unknown authority
```

Nao ha saida. Nao existe `tls:` no topo do cenario:

```
erro no cenario: tls-ca.yaml:4:1: chave desconhecida no topo do cenario: "tls"
    disponiveis: nome, alvo, requer, variaveis, autenticacao, mensageria, dados, carga, cenario, slo
```

Nem no passo, nem flag de linha de comando. O cliente HTTP compartilhado
(`internal/protocol/transport/client.go`) nao tem nenhum campo de TLS alem do
timeout de handshake.

**A assimetria**: Kafka e AMQP tem `tls: { ca, certificado, chave }` com leitura de
arquivo. HTTP, que e o protocolo principal e o unico que o `import curl` produz, nao
tem nada.

**Por que bloqueia**: a maioria dos ambientes de homologacao corporativos serve
HTTPS com CA interna. Para essa pessoa a ferramenta nao roda — e a mensagem que ela
recebe e o texto cru do Go, que nao diz que a ferramenta nao tem por onde resolver.
Uma recusa que dissesse "nao ha como declarar CA hoje" custaria menos que uma que
parece problema do certificado dela.

**Nao corrigido**: recurso novo. Primeiro item da lista de bloqueio de adocao.

---

#### Achado 2.6.b — a coluna de exemplo corta a mensagem justo onde esta a causa
**Gravidade**: media — obriga um `debug` a cada falha de rede de mensagem longa

Mesma falha, sob carga:

```
Erros
  passo                      o que aconteceu                    quantidade   exemplo
  consultar                  falha de rede                              30   Get "https://127.0.0.1:8443/produtos": tls:…
```

O corte preserva a URL — que ja esta no cabecalho do relatorio — e joga fora
`failed to verify certificate: x509: certificate signed by unknown authority`, que e
a unica parte acionavel. Cortar pela esquerda, ou cortar o prefixo repetido, daria a
mesma largura com a informacao certa.

---

### 2.7 — Broker inalcancavel
**Correto.** exit 3, `dial tcp 127.0.0.1:9199: connect: connection refused`, passo
nomeado, e a mensagem manda rodar `debug`. Custo: a execucao gasta os 10s inteiros
falhando antes de dizer isso — num cenario de 30 minutos, gastaria 30 minutos.

### 2.8 — Consumidor morre no meio
**Afirmava causa nao apurada. Corrigido.** Ver 2.8.a.

### 2.9 — Alvo corta conexao em um terco das requisicoes
**Correto na classificacao, discutivel no veredito.**

```
Passou: a unica regra de SLO foi atendida.
  500 requisicoes em 10s, 50 por segundo, 33.40% de erro
  consultar    falha de rede    167    connection reset
```

`connection reset` sai como classe propria. A palavra "Passou" com um terco das
requisicoes falhando so se sustenta porque nenhum criterio de erro foi declarado — e
o relatorio diz isso na linha seguinte (`global: sem criterio declarado`). Correto
pela letra; ver bloco 5 sobre o que o titulo comunica.

### 2.10 — Resposta que nao e JSON onde havia captura
**Exemplar.** O `debug` mostra o corpo e explica:

```
  resposta:   status 200, 81 bytes
  corpo:      <html><head><title>502 Bad Gateway</title></head><body><h1>502</h1></body></html>
  problema:   nao consegui capturar "pedidoId" com $.pedido.id: a resposta nao e JSON valido
```

Sob carga, exit 3, e o segundo passo aparece explicitamente como ausente da medicao:

```
  - o passo "usar captura" foi declarado e nao registrou nenhuma amostra; ele ficou de fora da medicao
    passos com amostra: consultar
```

### 2.11 — Alvo que nunca responde
**Correto, com e sem timeout declarado.** Com `timeout: 1s`, classe `tempo esgotado`
e `timeout: 50` no resumo por classe. Sem timeout nenhum, o cliente padrao corta em
30s: uma execucao de 5s levou 35s no total e terminou — nao trava.

---

#### Achado 2.8.a — o atraso do consumidor afirmava a causa que nao apurou
**Gravidade**: media — nao afeta o numero, afirma o que nao verificou

**Aconteceu**: matei o processador aos 8 segundos de uma execucao de 20s a 200/s.

```
Atraso do consumidor
  grupo processador em pedidos-eventos: no pior momento 2.399 mensagens atras; no fim, 2.399 mensagens
  O consumidor terminou a execucao para tras: a fila cresceu mais rapido do que ele consumiu.
```

O consumidor nao ficou para tras: ele deixou de existir. A medicao e a distancia
entre a ponta do topico e o offset comitado do grupo; consumidor lento, consumidor
parado e consumidor em rebalanceamento produzem exatamente o mesmo numero.

**Corrigido agora** — a frase passa a dizer a distancia e recusar o diagnostico:

```
  O consumidor terminou a execucao para tras. O atraso diz a distancia, nao a causa:
  consumidor lento, parado ou em rebalanceamento produzem o mesmo numero.
```

Teste `TestLagSentenceDoesNotClaimACauseItDidNotCheck`, provado contra o codigo
anterior no terminal e no HTML. Saida publicada no README corrigida junto.

**O que continua aberto**: a execucao terminou **verde, exit 0**, com o consumidor
morto ha 12 segundos. E o achado 1.3.a de novo, agora com um alvo que nao existe
mais.

---

## Bloco 3 — Erro humano

Doze erros que quem escreve cenario comete. Cada mensagem classificada em
**ensina** (diz o que fazer), **aponta** (diz que esta errado) ou **abandona**
(nao diz, ou pior, aceita).

| # | erro | antes | depois |
|---|---|---|---|
| 3.1 | `carrga:` no lugar de `carga:` | ensina | — |
| 3.2 | `taxa: 100` sem unidade | **abandona: aceitava como 100/s** | ensina (corrigido) |
| 3.3 | `durante: 30` sem unidade | ensina | — |
| 3.4 | `${naoexiste}` nao declarada | ensina | — |
| 3.5 | `variaveis: { senha: p4ssw0rd }` | **abandona: aceitava e ia para o repositorio** | ensina (corrigido) |
| 3.6 | CSV que nao existe | aponta (e `validate` aprova) | — |
| 3.7 | coluna que nao existe no CSV | **abandona: interpolava vazio** | ensina (corrigido) |
| 3.8 | captura sem `$.` | ensina | — |
| 3.9 | `status: "duzentos"` | ensina | — |
| 3.10 | indentacao YAML errada | aponta | — |
| 3.11 | slo apontando passo inexistente | aponta (so no fim da execucao) | ensina na validacao (corrigido) |
| 3.12 | `braunrate new --help` | **abandona: cria arquivo** | — |

### O que ja ensinava bem

```
erro no cenario: e01.yaml:3:1: chave desconhecida no topo do cenario: "carrga"
    voce quis dizer "carga"?
    disponiveis: nome, alvo, requer, variaveis, autenticacao, mensageria, dados, carga, cenario, slo
    um cenario minimo tem quatro delas: ...
```

```
erro no cenario: e04.yaml:8:25: nao sei de onde vem ${naoexiste}.
    declare de onde ela vem:
      variaveis: { naoexiste: valor }                 # fixa no cenario
      variaveis: { naoexiste: "${NAOEXISTE:-reserva}" }   # do ambiente, com reserva
      captura: { naoexiste: $.campo }                 # de uma resposta anterior
      dados: { pedidos: { arquivo: dados.csv } }  # e entao ${pedidos.naoexiste}
    nome em CAIXA ALTA vem do ambiente sem precisar declarar: ${NAOEXISTE}
```

Nome proximo vira sugestao, a lista completa vem depois, e o exemplo minimo fecha.
Tres dos quatro erros mais comuns caem nesse padrao.

---

#### Achado 3.2.a — `taxa: 100` era lida como 100/s, sem aviso
**Gravidade**: ALTA — afeta o numero, e o numero fica sessenta vezes errado

`readRate` aceita `/s`, `/m` e `/h`. Um numero pelado caia no caso omisso e virava
por segundo. Quem escreveu `taxa: 100` querendo por minuto recebeu **sessenta vezes
a carga**, o relatorio saiu inteiro sobre uma carga que ninguem pediu, e nada em
lugar nenhum disse isso.

**Corrigido agora**:

```
erro no cenario: e02.yaml:5:24: taxa sem unidade: "100"
    diga em que intervalo: 100/s (por segundo), 100/m (por minuto) ou 100/h (por hora)
```

Dois testes, o primeiro provado contra o codigo anterior:

```
--- FAIL: TestRateWithoutUnitIsRefusedInsteadOfAssumedPerSecond (0.00s)
    yaml_test.go:244: taxa sem unidade foi aceita: 100 virou 100/s sem ninguem ser avisado
```

O segundo trava o contrario: `taxa: rapido` continua com "taxa invalida", em vez de
sugerir `rapido/s`.

---

#### Achado 3.5.a — credencial literal em `variaveis:` era aceita
**Gravidade**: ALTA — viola a regra de credencial declarada para o projeto

```yaml
variaveis:
  senha: p4ssw0rd-de-verdade
```

```
Cenario valido: "Teste", 1 passo, 50 iteracoes em 5s.
exit=0
```

O importador de curl **ja** mascara exatamente esses nomes antes de escrever o
arquivo (`secretFields` em `internal/importer/render.go`), e o bloco `mensageria`
recusa senha literal de broker com mensagem que ensina. Escrito a mao, no bloco que
todo mundo usa, passava.

**Corrigido agora** — mesma lista de nomes, mesma postura:

```
erro no cenario: e05.yaml:4:10: senha literal em 'variaveis': credencial nunca vai
para o arquivo, porque o arquivo vai para o repositorio.
    troque por:  variaveis: { senha: "${SENHA}" }
    e rode com:  SENHA=... braunrate execute cenario.yaml
```

Tres testes; o primeiro cobre `senha`, `password`, `token`, `api_key` e
`client_secret` e reprova o codigo anterior nos cinco. Os outros dois travam o que
**nao** pode mudar: credencial vinda do ambiente continua aceita, e variavel comum
(`usuario: ana`) continua aceitando literal — a regra e sobre segredo, nao sobre
escrever valor no arquivo.

**O que fica aberto**: `${SENHA:-segredo}` continua aceito em corpo de passo, e os
tres exemplos publicados usam essa forma. Para credencial de broker a regra do
projeto ja recusa reserva com todas as letras ("a reserva seria o segredo escrito no
arquivo"). Estender isso para corpo de passo quebra os tres exemplos e e decisao de
produto, nao correcao de bateria. Entra na lista.

---

#### Achado 3.7.a — coluna que nao existe no CSV interpolava para vazio
**Gravidade**: ALTA — mesma familia do 1.2.a, e a terceira ocorrencia dela

```yaml
dados:
  clientes: { arquivo: clientes.csv, consumo: circular }   # colunas: id,tipo,rota,limite
cenario:
  - http: GET /pessoas/${clientes.identificador}/limite
```

```
Cenario valido: "Teste", 1 passo, 30 iteracoes em 3s.
exit=0
```

```
passo 1 — consultar   [FALHOU em 6.7ms]
  requisicao: GET /pessoas//limite
```

A regra de variavel nao declarada (A8) checa o **nome da fonte** e para ali: o
comentario no codigo dizia que as colunas vem do arquivo, "que nao esta aberto neste
ponto". Verdade na leitura do YAML — mas o arquivo e aberto logo depois, no motor, e
la a lista de colunas esta pronta.

**Corrigido agora**, no ponto em que as fontes sao abertas:

```
a fonte de dados "clientes" nao tem o campo "identificador".
    campos disponiveis: id, limite, rota, tipo
```

Dois testes, o primeiro provado contra o codigo anterior. Junto veio um defeito que
so aparece nesse caminho: `debug` imprimia a mesma mensagem de erro duas vezes.

---

#### Achado 3.11.a — `validate` aprova o que ja da para ler que esta errado
**Gravidade**: media — o portao barato do CI aprova tres coisas diferentes que nao rodam

Tres casos independentes, o mesmo padrao:

| caso | `validate` | quando o erro aparece |
|---|---|---|
| passo Kafka sem broker em lugar nenhum (1.3.b) | `Cenario valido` | primeira iteracao |
| slo apontando passo que nao existe | `Cenario valido` | **no fim da execucao**, carga inteira ja gasta |
| arquivo CSV que nao existe | `Cenario valido` | ao abrir a fonte |

Os dois primeiros foram corrigidos: os nomes e os enderecos estao todos no arquivo.

```
cenario invalido:
  - o slo "consultar" nao casa com nenhum passo do cenario
    disponiveis: consultar produtos
```

O terceiro — arquivo que nao existe — nao foi corrigido: `validate` teria que tocar
o disco, e ha um argumento legitimo de que validar o texto e validar o ambiente sao
coisas diferentes. E decisao, nao defeito, e vai para a lista.

---

#### Achado 3.12.a — `braunrate new --help` cria um arquivo
**Gravidade**: baixa — mas e a ferramenta escrevendo no disco de quem pediu ajuda

```
$ braunrate new --help
cenario de partida em cenario.yaml: troque o alvo e o caminho pelo seu servico.
```

A flag e ignorada e o arquivo padrao e criado. Nao sobrescreve (com `cenario.yaml`
ja existente, recusa e sugere outro nome), entao nao ha perda — mas e a unica forma
de a ferramenta escrever no disco de alguem que estava pedindo documentacao. Mesma
familia do 1.4.b: argumento a mais ignorado em silencio.

---

### 2.12 — Gerador saturado: taxa que a maquina nao sustenta
**A propriedade mais importante da ferramenta, e ela funciona.**

60.000/s numa maquina de 10 nucleos, contra um alvo que nao responde:

```
Resultado invalido: a execucao nao mediu o que se propos a medir.
  - nenhuma jornada chegou ao fim ...  74.890 jornadas iniciadas, 0 completas
  - o passo "consultar" falhou em 100% das requisicoes ...  74.659 requisicoes, 74.659 erros (timeout: 74.243, rede: 416)
  - o gerador atingiu o limite de requisicoes em voo e deixou de enviar requisicoes agendadas; o resultado nao vale
    405110 requisicoes descartadas, pico de 20000 em voo
  - o gerador nao sustentou a taxa alvo: despachos sairam depois do instante agendado; o resultado nao vale
    80.43% dos despachos atrasaram mais de 10.0 ms (desvio p99 de 11567.1 ms)
exit=3
```

Quatro achados independentes, e os dois ultimos acusam **o proprio gerador**. Uma
ferramenta de carga que confunde a propria saturacao com lentidao do alvo produz o
numero mais caro que existe — um bug de performance inventado. Esta nao confunde.

---

### 2.11 — Execucao de trinta minutos: RSS e descritores
**Descritores estaveis. Memoria cresce com o tempo, e o crescimento tem causa medida.**

Cenario de dois passos com autenticacao, 400 requisicoes por segundo contra o alvo
embutido. Amostragem de RSS, descritores e threads a cada 60 segundos.

```
  720.000 requisicoes em 30m0s, 400 por segundo, 0% de erro
  Metade das respostas em ate 2.5 ms; 95% em ate 3.0 ms; 99% em ate 3.5 ms; a pior levou 121 ms
  Todas as 360000 jornadas chegaram ao fim
```

| instante | RSS | descritores | threads |
|---|---|---|---|
| 0 min | 29,7 MB | 13 | 15 |
| 8 min | 97 MB | 15 | 15 |
| 15 min | 170 MB | 14 | 15 |
| 22 min | 245 MB | 35 | 17 |
| 29 min | **313 MB** | 26 | 17 |

**Descritores e threads: sem vazamento.** 13 a 35 descritores e 15 a 17 threads em
trinta minutos e 720 mil requisicoes. Nada acumula.

**RSS: 10,5 vezes em trinta minutos**, com serrilha de coleta e tendencia monotona.

#### A causa, medida e nao suposta

A serie temporal cria um balde por segundo e cada balde guarda um histograma HDR
proprio, retido ate o fim da execucao (`internal/metrics/collector.go`, `bucketDe`).
Medi o tamanho de um histograma na configuracao que o codigo usa
(`hdrhistogram.New(1, 600_000_000, 3)`):

```
100 histogramas: 16812 KB, ou 168 KB cada
```

168 KB por segundo de execucao = **10,1 MB por minuto, independente da taxa**.

A previsao bate nos dois pontos, e o controle e o que separa "cresce com o tempo" de
"cresce com o numero de requisicoes":

| execucao | duracao | taxa | baldes | previsto | medido |
|---|---|---|---|---|---|
| longa | 29 min | 400/s | 1.740 | 292 MB | 313 MB |
| controle | 5 min | **20/s** | 300 | 50 MB | 47 MB (pico) |

Vinte vezes menos requisicoes, a mesma memoria por minuto. Nao e a carga: e o
relogio.

---

#### Achado 2.11.a — a memoria cresce 10 MB por minuto de execucao, qualquer que seja a taxa
**Gravidade**: ALTA para adocao — teste de resistencia e caso de uso central e e o unico que nao cabe

Uma execucao de 4 horas — soak test comum — pediria cerca de **2,4 GB** so de
histogramas de serie temporal. Em runner de CI com limite de memoria, morre.

E o numero que sustenta a decisao de linguagem do [ADR 0001](adr/0001-linguagem-e-runtime.md)
e **30 MB de RSS sob carga** — que e o que esta ferramenta consome no primeiro
minuto, e nao no trigesimo. O numero nao esta errado; ele so nao e sobre execucao
longa, e nada no repositorio diz isso.

**Saidas possiveis, com o custo de cada uma medido:**

```
atual (1us..600s, 3 digitos)     168 KB por balde  ->  295,6 MB em 30 minutos
1us..600s, 2 digitos              24 KB por balde  ->   42,4 MB
1ms..600s, 2 digitos              16 KB por balde  ->   28,3 MB
1ms..60s,  2 digitos              12 KB por balde  ->   21,4 MB
```

Baixar a precisao do balde de 3 para 2 digitos custa **um decimo da memoria** e
afeta apenas o p50 e o p99 por segundo da serie temporal — os percentis do relatorio
vem de outro histograma, o do passo, que nao muda. A alternativa melhor e fechar o
balde quando o segundo dele passa: calcular os dois percentis e soltar o histograma.

**Nao corrigido**: escolher entre as duas muda numero ja publicado (a serie temporal
do HTML) e e decisao de desenho, igual a separacao de histograma do 2.4.a. Vai para
a lista com a medicao pronta.

**Ressalva de metodologia**: rodei outros testes desta bateria em paralelo durante os
trinta minutos, e o relatorio da execucao longa acusou degradacao do alvo ao longo do
tempo (`p99 por segundo passou de 4.2 ms para 104.5 ms`). Essa degradacao e
provavelmente contaminacao minha, nao do alvo embutido. A medicao de RSS nao depende
disso — o controle a 20/s rodou com a maquina ociosa e confirmou a mesma curva por
minuto.

---

## Bloco 4 — Combinacoes que ninguem testou

### 4.1 — Modelo fechado com criterio de jornada
**Exemplar.** O aviso vem antes de qualquer numero, e a frase da jornada muda de
sentido junto com o modelo:

```
ATENCAO: Este teste usou 20 usuarios em laco fechado. Se o alvo travar, os usuarios
param de pedir e o atraso nao aparece nos numeros. O tempo de resposta abaixo pode
estar melhor do que o usuario real sente.

  Todas as 1980 jornadas chegaram ao fim; metade levou ate 1 ms e 95% ate 4 ms,
  contados de quando o usuario virtual comecou a jornada, que e so depois de ter
  terminado a anterior.

  (2) tempo de resposta puro. No laco fechado nao existe instante agendado: o
      usuario virtual so pede de novo depois da resposta anterior, entao nenhum
      atraso de fila aparece nestes numeros.
```

Tres lugares dizendo a mesma verdade em vez de um numero que finge ser comparavel
com o do modelo aberto.

### 4.3 — CSV sequencial que acaba no meio
**Correto.** exit 3, e o consumo de dados aparece como passo proprio nos erros:

```
  - o passo "dados: clientes" falhou em 100% das requisicoes
    90 requisicoes, 90 erros (configuracao: 90)

  dados: clientes      erro de configuracao do cenario      90   os dados de "clientes" acabaram na linha 10…
```

A saida — `use consumo circular para repetir do inicio` — foi cortada pela coluna de
exemplo (achado 2.6.b, terceira ocorrencia).

### 4.5 — Captura de cabecalho e captura por regex alimentando chave de Kafka
**Funciona inteiro.** Duas capturas de tipos diferentes no mesmo passo, as duas
usadas no passo seguinte, em outro protocolo:

```
  capturou:
    idPorRegex = ped-be3b74c6
    tipoConteudo = application/json

passo 2 — publicar com chave capturada   [ok em 15.9ms]
  requisicao: produzir em eventos-particionado (chave "ped-be3b74c6")
```

```
  100 valores distintos de idPorRegex em 100 usos, todos comecando com "ped-"
  3 valores distintos de kafka.particao.eventos-particionado em 100 usos, entre 0 e 2
  1 unico valor de tipoConteudo em 100 usos
```

**Atrito real no caminho**: a regex `/"id": "(ped-[a-f0-9]+)"/` contem `": "`, que o
YAML le como separador. A mensagem acerta o diagnostico e erra o exemplo:

```
erro no cenario: c05.yaml:15:1: ha dois-pontos dentro de um valor que nao esta entre aspas.
    ponha o valor entre aspas, por exemplo:  cabecalho: "X-API-Key: ${API_KEY}"
```

Aspas duplas nao resolvem: o valor **tem** aspas duplas dentro. So aspas simples
funcionam, e o exemplo nao mostra isso. O caso mais provavel de cair nesse erro e
justamente a captura por regex.

### 4.7 — Criterio de regressao com base
**Funciona, e o gate reprova de verdade.** Base e depois com o alvo dez vezes mais
lento:

```
Falhou: a jornada inteira (p95) ficou 1347.9% pior que a base, acima do limite de
20% pior (de 2 ms para 31 ms).
exit=1
```

Numeros antes e depois na propria frase. Ver 4.7.a para o que essa comparacao **nao**
checa.

### 4.8 — Modo servidor
**As rotas fazem o que a documentacao diz.** `GET /scenarios`, `POST .../validate`,
`POST .../debug`, `POST .../runs`, `GET /runs`, `GET /runs/{id}`, `GET /runs/{id}/report`
conferidos a mao contra `docs/api-servidor.md`. O aviso de subida nao pede desculpas:

```
Sem autenticacao e sem TLS: qualquer um que alcance esta porta pode disparar carga contra os alvos dos cenarios.
Foi feito para rodar em 127.0.0.1. Expor em outra interface e outra decisao, e ela ainda nao foi tomada.
```

E a recusa de execucao simultanea e a melhor mensagem da ferramenta:

```
HTTP 409
ja existe uma execucao em andamento. Duas execucoes na mesma maquina disputam a CPU
que precisa despachar no instante agendado, e nenhuma das duas mede o que se propos a
medir. Espere a atual terminar, ou suba o servidor com -concurrent se a contaminacao
for aceitavel neste caso.
```

Diz o que aconteceu, **por que isso importa para a medicao**, e como sair. Ver 4.8.a
para a contradicao que ela expoe.

### 4.9 — Fonte CSV e fonte sintetica no mesmo passo
**Funciona, e a variedade separa as duas:**

```
  10 valores distintos de clientes.id em 100 usos, entre 1 e 10
  2 valores distintos de clientes.tipo em 100 usos
  100 valores distintos de novos.chave em 100 usos
  100 valores distintos de novos.valor em 100 usos, entre 13.66 e 498.94
  Semente das fontes sinteticas: novos=1 (a mesma semente gera os mesmos valores de novo)
```

---

#### Achado 4.7.a — a comparacao afirmava que nada explicava a diferenca
**Gravidade**: media-alta — culpa o servico por mudanca que foi no teste. Corrigido.

Troquei o **conteudo inteiro do CSV** entre duas execucoes do mesmo cenario. O p95
mudou 15 vezes:

```
Ficou mais lento: jornada inteira (95%): 15 vezes mais lento — de 2 ms para 31 ms.

O que pode explicar a diferenca sem ser o servico
  Nada: mesmo cenario, mesmo alvo, mesma maquina, mesmo plano de carga e mesma versao.
```

"Nada" e afirmacao absoluta sobre cinco campos conferidos. O conteudo dos arquivos de
dados nao e um deles — e num CI, arquivo de dados mudando entre execucoes e causa
comum de regressao falsa.

**Corrigido agora**:

```
  Nada do que da para comparar: cenario, alvo, maquina, plano de carga e versao sao os
  mesmos. O conteudo dos arquivos de dados nao entra nesta lista — se ele mudou entre
  as duas, a diferenca pode ser dele.
```

Teste `TestNoCaveatSaysWhatItCompared`, provado contra o codigo anterior. Mesma
familia do 2.8.a e do achado de comparacao da noite anterior: afirmar o que nao se
apurou.

---

#### Achado 4.8.a — o servidor recusa como invalido o unico contorno que a ferramenta oferece para o mix
**Gravidade**: media — nao e defeito de codigo, e uma contradicao que o produto ainda nao resolveu

O modo servidor recusa duas execucoes simultaneas com o argumento certo: elas
disputam a CPU que precisa despachar no instante agendado, e **nenhuma das duas mede
o que se propos a medir**.

Na jornada 1.4, sem mix ponderado, o unico jeito de exercitar 60/30/10 foi rodar
**tres processos simultaneos** pela linha de comando. A CLI nao avisa nada. Os tres
relatorios sairam com "O gerador disparou todas as requisicoes na hora certa" — o que
e verdade para cada processo isolado, e nao responde a pergunta que o servidor faz.

Ou o argumento do servidor vale e o contorno do 1.4 produz numero contaminado, ou o
argumento e conservador demais. Os dois nao podem estar certos. Isso reforca 1.4.a: o
mix ponderado nao e conforto, e a unica forma de medir um mix sem contaminar.
