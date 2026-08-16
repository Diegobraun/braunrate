# Por que as jornadas exigiram edicao manual

A bateria adversarial fechou o bloco 1 com uma frase: **nenhuma das cinco jornadas foi
montada so pelos caminhos de entrada da ferramenta**. Ficou como achado sem diagnostico.
Este documento e o diagnostico: as cinco jornadas refeitas, tentando de novo montar cada
uma apenas com `new`, `import curl` e `record`, com cada lacuna classificada em defeito,
falta de perguntar ou edicao manual legitima.

**A resposta em uma frase**: o gap principal nao e falta de recurso — e **falta de
deteccao**, porque o gravador tinha o dado na mao e nao olhava; o que sobra depois de
corrigir isso e **falta de a ferramenta perguntar**, e recurso mesmo so o mix ponderado.

Ambiente: Mac darwin/arm64, Go 1.26.6. Alvo: o mesmo mock da bateria (`loja.py`, porta
8090) e um portal de sessao por cookie escrito para este documento (porta 8091), porque
o mock da bateria autentica por token no corpo e nao exercita a correlacao mais comum da
web.

> **A bateria nunca tentou `record` para a jornada 1.1.** Ela montou com `import curl` e
> concluiu que nao havia caminho de entrada para jornada de varios passos (achado 1.1.b).
> A conclusao estava errada para o gravador: ele ja correlacionava valor de corpo JSON.
> Refazer o exercicio pelo caminho certo e o que expos os defeitos reais, que sao outros.

---

## 1.1 E-commerce, seis passos — sai inteira pelo `record`, menos a sessao por cookie

### 1.1.a Token no corpo: nao ha lacuna
**Classe: nenhuma — funcionou**

**Tentativa**: `braunrate record`, e pelo proxy: `POST /auth/login` → `GET /produtos?q=` →
`POST /carrinhos` → `POST /carrinhos/{id}/itens` → `POST /carrinhos/{id}/fechar` →
`GET /pedidos/{id}`.

**Resultado**: `6 requisicoes viraram 6 passos`. As quatro correlacoes sairam ligadas —
`access_token` do login para o `Authorization` dos cinco passos seguintes, `sku` da busca
para o corpo do item, `id` do carrinho para dois caminhos, `carrinhos_fechar_id` do
fechamento para a consulta final. A senha do corpo virou `${senha}` vinda de `$SENHA`,
com aviso na tela.

**Custo**: 1 comando, **zero edicao manual**. O `debug` fecha a jornada de seis passos de
primeira.

### 1.1.b Sessao por cookie: o gravador pedia a sessao gravada de volta
**Classe (a) — defeito. Corrigido.**

**Tentativa**: mesmo `record` contra o portal de sessao: `POST /entrar` responde
`Set-Cookie: sessao=<32 hex>`, e as tres chamadas seguintes mandam `Cookie: sessao=...`.

**Onde parou**: o gravador nao ligou os dois. A regra de correlacao olhava so o corpo
JSON da resposta, e `Set-Cookie` e cabecalho. O cabecalho `Cookie` do consumidor caiu na
mascara de credencial e virou `${cookie}` vindo de `$COOKIE`:

```yaml
variaveis:
  cookie: ${COOKIE}
cenario:
  - nome: get perfil
    http:
      cabecalhos:
        Cookie: "${cookie}"
```

**O que precisei escrever na mao**: a captura no passo 1 e a interpolacao nos outros tres.

**Por que e defeito, e por que e grave**: a regra de correlacao existe e funciona para
corpo — o mesmo gravador acertou quatro encadeamentos na jornada 1.1.a. Ela nao olhava
o lugar mais comum de correlacao em aplicacao web. E o resultado nao era so trabalho
manual: o cenario gerado pede que a pessoa cole no ambiente a sessao que foi gravada, e
entao **todas as jornadas da execucao reusam uma sessao so** — o defeito de identidade da
Fase 4, entrando pelo gravador, com aparencia de instrucao de seguranca.

**Corrigido** (`58c69e8`): `Set-Cookie` entra na deteccao de correlacao e o `Cookie` do
consumidor sai correlacionado. A mascara passa a ser por par — o que nasceu numa resposta
gravada vira `${variavel}`, o que nao nasceu continua indo para o ambiente, com o aviso
dizendo qual cookie e por que.

Capturar `cabecalho:Set-Cookie` devolveria `sessao=abc; Path=/; HttpOnly`, e mandar isso
de volta manda tres cookies, dois deles inventados. Por isso entrou a expressao
`cookie:nome`, que le o par e ignora os atributos. **E a unica adicao de formato fora dos
itens 2 e 3, e sem ela a correcao nao tem como ser escrita.**

Saida de hoje, sem nenhuma edicao manual:

```yaml
  - nome: post entrar
    captura:
      sessao: cookie:sessao   # sugestao do gravador: confira se e mesmo este valor que a proxima chamada precisa
  - nome: get perfil
    http:
      cabecalhos:
        Cookie: "sessao=${sessao}"
```

Testes `TestSessionCookieBecomesCaptureAndIsSentBackCorrelated` e
`TestCookieThatNoResponseProducedIsMaskedPairByPair` reprovam o codigo anterior.

### 1.1.c Consequencia: o cookie de sessao aparecia inteiro na saida
**Classe (a) — defeito. Corrigido.**

Com a correlacao funcionando, o `debug` passou a imprimir `Cookie:
sessao=eb5b94f531fa41c9ad8e8a4953b59b4b` — credencial inteira numa saida que vai para
ticket e captura de tela. O corte que existia cobria `authorization`, `token`, `senha` e
`secret`, e deixava passar `Cookie` e `X-API-Key`.

**Corrigido** (`e39a1b7`): o nome do par fica, o valor e cortado na mesma forma do Bearer.
Testes `TestSessionCookieIsCutLikeTheBearerAlreadyWas` e `TestApiKeyHeaderIsCutToo`
reprovam o codigo anterior.

---

## 1.2 Banco com idempotencia — a repeticao sumia e a chave nao tem como ser inferida

### 1.2.a A segunda transferencia sumia sem ser nomeada
**Classe (a) — defeito. Corrigido.**

**Tentativa**: `record` de `login` → `saldo` → `transferencia` → **mesma transferencia,
mesma chave** → `extrato`.

**Onde parou**: `5 requisicoes viraram 4 passos`, e nada mais. As duas chamadas a
`POST /transferencias` foram agrupadas, e o passo que sumiu era justamente o que a
jornada existe para exercitar — o reenvio com a mesma chave de idempotencia. Quem le a
contagem sabe que perdeu uma; nao sabe qual.

**Por que e defeito**: agrupar por rota e a decisao certa quando o identificador varia —
e o que da uma linha por operacao no relatorio em vez de uma linha por requisicao. Nao e
certo quando a chamada se repetiu **identica**, porque ai a repeticao era a operacao. O
gravador tem os dois casos na mao e tratava os dois igual.

**Corrigido** (`a8ae14c`): quando as chamadas do grupo sao identicas em caminho e corpo,
o gravador nomeia o passo e diz o que se perdeu.

```
atencao: o passo "post transferencias" foi gravado 2 vezes com a mesma chamada e virou um
passo so: se a repeticao era o que voce queria medir (reenvio, idempotencia, cache), ela
nao esta no cenario
```

Quando o identificador varia, que e o caso para o qual o agrupamento existe, continua sem
aviso — `TestRouteWithVaryingIdentifiersIsNotWarnedAbout` guarda isso, porque um aviso que
dispara no caso normal ensina a pular o bloco onde moram os avisos que importam.

### 1.2.b A chave de idempotencia nao tem como ser inferida
**Classe (b) — falta perguntar. Nao implementado.**

**Onde parou**: o gravador viu `Idempotency-Key: c08a333b-05aa-41ec-9a43-8e7c3c1507dd` e
escreveu o valor literal no arquivo. Correto do ponto de vista do que ele observou: uma
passagem so nao revela que aquele valor deve ser novo a cada jornada e estavel dentro
dela.

**Por que nao e defeito**: nenhuma heuristica separa "identificador que deve variar" de
"token que deve repetir" com uma amostra. Errar nos dois sentidos quebra o cenario de
formas diferentes — um valor fixo faz o alvo devolver a resposta guardada em todas as
iteracoes e a medicao vira medicao de cache de idempotencia; um valor novo por requisicao
faz o reenvio deixar de ser reenvio.

**Proposta, nao implementada**: ao encerrar a gravacao, o `record` lista os cabecalhos que
parecem identificador — nome contendo `idempotency`, `request-id`, `correlation`, `trace`,
ou valor com forma de UUID — e pergunta, um a um: **fixo, novo por jornada, ou novo por
uso?** Tres teclas, e o YAML sai com `gerar: { chave: uuid }` ou `novo_a_cada: uso`, que
ja existem.

**Por que perguntar e nao gerar**: perguntar tres coisas no fim da gravacao custa vinte
segundos; descobrir na terceira execucao que a idempotencia nunca foi testada custa uma
tarde. E a ferramenta ja tem postura de perguntar em vez de adivinhar em outros lugares —
o `requer:` do exemplo, o `novo_a_cada: uso` declarado.

### 1.2.c Duas formas para o mesmo identificador no mesmo arquivo
**Classe (b) — falta perguntar. Nao implementado, e menor.**

O mesmo numero de conta saiu como fonte de dados num passo (`/contas/${contas_id.valor}/saldo`,
porque o segmento parece identificador) e como variavel correlacionada no outro
(`/contas/${numero}/extrato`, porque o corpo do saldo devolveu o numero). Os dois
funcionam e concordam em execucao. O arquivo fica confuso: duas mecanicas para a mesma
coisa, sem nada explicando.

Entra na mesma pergunta de fim de gravacao: quando um valor aparece como dado e como
correlacao, perguntar qual das duas a pessoa quer.

---

## 1.3 Cadeia assincrona — o gravador nao pode ver, e o `new` nao mostrava

### 1.3.a O `new` ensinava um protocolo de cinco
**Classe (a) — defeito. Corrigido.**

**Onde parou**: a bateria descartou a saida do `new` inteira. Ela mostra `http` e nada de
`graphql`, `kafka`, `amqp` ou `aguardar` — que estao compilados e aparecem na linha
"Protocolos compilados" de **todo relatorio que a ferramenta imprime**.

**Por que e defeito e nao falta de recurso**: o esqueleto ja documenta em comentario o
bloco `dados:` e o bloco `autenticacao:`. Mostrar os outros quatro protocolos nao exige
saber nada sobre o alvo, nao pergunta nada e nao muda formato. A ferramenta anunciava
cinco protocolos e ensinava um.

**Corrigido** (`c51df92`): o esqueleto sai com as formas comentadas dos cinco. As formas
vivem em `ProtocolShapes()`, fora do texto, porque `TestCommentedProtocolShapesParse` as
passa pelo parser — a primeira versao deste bloco escrevia `valor` no passo `amqp`, que
usa `corpo`, e teria ensinado errado exatamente quem nao tem outra referencia.

### 1.3.b Topico, grupo e efeito esperado nao estao no trafego HTTP
**Classe (b) — falta perguntar. Nao implementado.**

**Onde parou**: `record` e proxy HTTP. Ele nao ve Kafka nem AMQP, e nao ha como ver: seria
outro gravador, dentro do protocolo do broker, com o consumidor do grupo real. Mesmo que
existisse, nome de topico, grupo de consumo e **qual efeito conta como "processado"** sao
decisoes de quem monta, nao observacoes.

**Por que nao e defeito**: nada no que a ferramenta observa diz que o pedido criado por
HTTP deveria virar `PROCESSADO` em outro topico dentro de dez segundos. Esse e o criterio,
e criterio ninguem infere.

**Proposta, nao implementada**: `braunrate new -protocolo kafka` (e `amqp`, `graphql`,
`aguardar`) escrevendo o esqueleto do protocolo pedido descomentado, com os campos
obrigatorios vazios e o comentario de cada um. Uma flag, sem interatividade. Com o
esqueleto de hoje ja comentado, isso passa de necessario a conveniente — por isso fica
como proposta e nao como correcao.

---

## 1.4 GraphQL — tres operacoes viravam um passo, e uma mutation sumia

### 1.4.a O gravador tratava GraphQL como POST numa rota
**Classe (a) — defeito. Corrigido.**

**Tentativa**: `record` de tres chamadas ao mesmo `/graphql`: `query ConsultarPedido`,
`query RelatorioMensal`, `mutation PagarFatura`.

**Onde parou**: `3 requisicoes viraram 1 passo`.

```yaml
cenario:
  - nome: post graphql
    http:
      metodo: POST
      caminho: /graphql
      corpo: '{"query":"query ConsultarPedido($id: ID!) { ... }","variables":{"id":"ped-1"}}'
```

O relatorio de um mes inteiro e a mutation de pagamento **sumiram do cenario**, e o unico
sinal foi a contagem.

**Por que e defeito**: em GraphQL toda operacao chega no mesmo endereco. O ADR 0006 diz
isso e diz a consequencia: a unidade de medida e a operacao, nunca a URL, porque agregar
por URL poe a consulta mais barata e a mutation mais cara na mesma linha. O gravador
tinha o envelope da consulta no corpo — nome da operacao inclusive — e agrupava por rota.

**Corrigido** (`a8ae14c`): o gravador reconhece o envelope, agrupa por nome de operacao e
escreve passo `graphql` com `consulta` e `variaveis`, com o SLO apontando a chave que o
relatorio usa.

```yaml
cenario:
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { id status ultimaFatura { id valor } } }
      variaveis: {"id":"ped-1"}
slo:
  - graphql ConsultarPedido: { p95: < 500ms }
  - graphql RelatorioMensal: { p95: < 500ms }
  - graphql PagarFatura: { p95: < 500ms }
```

E o `debug` fecha os tres como operacoes, nao como tres POSTs:

```
passo 1 — graphql ConsultarPedido   [ok em 2.5ms]
passo 2 — graphql RelatorioMensal   [ok em 500µs]
passo 3 — graphql PagarFatura       [ok em 300µs]
```

`TestEachGraphQLOperationBecomesItsOwnStep` reprova o codigo anterior.

### 1.4.b A proporcao 60/30/10 continua sem forma de ser declarada
**Classe: recurso — item 2.**

Depois da correcao acima, o cenario sai com as tres operacoes e cada iteracao executa
**as tres em sequencia**, que e uma jornada, nao um mix. Declarar que 60% das chamadas
sao a consulta leve continua impossivel. E o unico ponto do bloco 1 que e falta de
recurso de verdade.

---

## 1.5 Ramificacao por perfil — edicao manual legitima

**Classe (c) — manual legitimo. Virou exemplo publicado.**

**Tentativa**: `record` de `/pessoas/1001/limite`, `/empresas/2002/limite`,
`/pessoas/1002/limite`.

**Onde parou**: `3 requisicoes viraram 2 passos`, um por rota, com um CSV de
identificadores para cada. Correto para o que foi observado — e nao e o que se queria. O
cenario gerado executa **as duas rotas em toda iteracao**; o que se queria e que cada
iteracao escolha um perfil, na proporcao da populacao real.

**Por que e manual legitimo**: saber que existem dois perfis com caminhos distintos, e em
que proporcao, e conhecimento de negocio. Quem grava uma passagem grava um perfil, e
nenhuma quantidade de gravacao revela a proporcao — ela e uma decisao sobre o que se quer
medir.

**Vira exemplo**: `examples/ramificacao-por-perfil.yaml`, com `examples/dados/clientes.csv`,
mostrando a coluna que decide a rota e o passo que a usa. Entra no laco do CI como os
outros. A verificacao confere o caminho que o alvo devolveu, para que uma coluna vazia
nao interpole em silencio — o defeito 3.7.a da bateria, agora coberto tambem pelo exemplo.

**Uma nota que o exemplo carrega**: os dois perfis caem numa linha so do relatorio, entao
um perfil caro aparece como cauda do passo inteiro (achado 1.5.a da bateria). Ligar isso a
variedade observada continua aberto para a sessao com o QA.

---

## Fechamento

| jornada | so pelos caminhos de entrada, hoje | classe da lacuna |
|---|---|---|
| 1.1 e-commerce, 6 passos, token no corpo | **sim, zero edicao** | — |
| 1.1 portal com sessao por cookie | **sim, zero edicao** (era nao) | (a) corrigida |
| 1.2 banco com idempotencia | nao: a chave continua literal | (a) corrigida + (b) proposta |
| 1.3 cadeia assincrona | nao: gravador nao ve broker | (a) corrigida + (b) proposta |
| 1.4 GraphQL, tres operacoes | **sim, zero edicao** (era nao) | (a) corrigida |
| 1.4 mix 60/30/10 | nao | recurso — item 2 |
| 1.5 ramificacao por perfil | nao, e nao deve ser | (c) virou exemplo |

Quatro defeitos corrigidos, cada um com teste que reprova o codigo anterior. Duas
propostas de desenho registradas sem implementar. Um exemplo publicado. Um recurso, que e
o item 2.

**O que isso muda para a sessao com o QA**: a pergunta "o que ela faz quando o segundo
passo precisa do id que o primeiro devolveu" — que a bateria tinha marcado como o primeiro
item a observar — deixa de ser a pergunta certa, porque o gravador resolve isso sozinho
nos tres formatos comuns (corpo JSON, cookie de sessao, cabecalho). A pergunta que fica e
outra, e mais dificil de responder de dentro: **a pessoa encontra o `record`?** Nada nas
mensagens da ferramenta diz que ele existe no momento em que ela esta olhando para uma
pasta vazia.
