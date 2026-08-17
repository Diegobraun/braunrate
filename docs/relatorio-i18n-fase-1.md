# Relatório da fase 1 — formato e mensagens em inglês

O que esta fase fez: o formato do cenário, o documento de resultado, tudo o que
a ferramenta imprime, o alvo embutido, os exemplos publicados e a interface web
passaram para inglês. Nenhuma mudança de comportamento — renomeação, tradução e
documentação.

O mapa completo de chaves e valores está no
[ADR 0019](adr/0019-formato-em-ingles.md), fechado antes de qualquer código. As
decisões que o mapa não cabe estão em [decisoes-i18n.md](decisoes-i18n.md), oito
delas.

---

## 1. O mapa, e as decisões difíceis

A convenção é **camelCase, sem exceção**. `renovar_apos` vira `refreshAfter`,
`corpo_contem` vira `bodyContains`, `novo_a_cada` vira `newEvery`. Não há chave
com underscore no formato novo.

O critério do termo foi **o que k6, Gatling e JMeter já usam**. Onde as três
divergem, ganhou a que o público de QA encontra primeiro numa busca.

Sete decisões custaram tempo:

**`aguardar` → `await`, e `ate` → `until` dentro dele.** `awaitUntil` foi
recusado porque `awaitUntil.until` repete a palavra. `await` sozinho é o verbo,
e `until` é a condição — leem-se juntos como a frase que descrevem.

**`durante` e `duracao` viram os dois `duration`.** Eram duas palavras para a
mesma coisa em dois lugares (`- patamar: { durante: 1m }` e `carga.duracao`).
Uma palavra por conceito é regra do [vocabulário](vocabulario.md), e o formato
estava desobedecendo a ela em português.

**`intervalo_entre_iteracoes` → `thinkTime`.** É o termo de domínio que JMeter e
Gatling usam há vinte anos. A tradução literal (`intervalBetweenIterations`)
seria correta e ninguém procuraria por ela.

**`constante` foi removido.** Era sinônimo exato de `patamar`; duas grafias para
o mesmo perfil só existiam por acidente histórico. Ficou `steady`, e `migrate`
converte as duas.

**`espera` foi removido.** Era alias de `verificar`. Ficou `expect`.

**`vazao` e `taxa_efetiva` viram os dois `throughput`.** Eram o mesmo número com
dois nomes — um no relatório, outro na regra de slo.

**`cpf` e `cnpj` ficaram.** São o nome da coisa. Traduzir nomearia um documento
que não existe, e o gerador continua gerando um CPF.

O documento JSON de resultado também foi: `formatVersion` sobe de 2 para 3, e o
cenário de 1 para 2. Um documento da 0.5.0 é reconhecido pela chave `ferramenta`
e o leitor recebe a instrução de reexecutar, em vez de "não foi gerado pelo
braunrate", que era errado e não acionável.

---

## 2. `braunrate migrate` contra os cenários da 0.5.0

Os sete cenários publicados na 0.5.0 estão congelados em
`internal/scenario/testdata/formato-0.5.0/` e o teste exige, para cada um: que
converta, que o resultado carregue, que valide, que mantenha comentário por
comentário e o mesmo número de linhas, e que a segunda passada não mude nada.

Saída real de um deles:

```
$ braunrate migrate ci.yaml
ci.yaml: 22 keys change
  line 2: nome -> name
  line 3: alvo -> target
  line 5: autenticacao -> auth
  line 6: tipo -> type
  line 7: obter -> obtain
  line 8: metodo -> method
  line 8: caminho -> path
  line 8: corpo -> body
  line 9: captura -> capture
  line 11: carga -> load
  line 12: perfis -> profiles
  line 13: rampa -> ramp
  line 13: de -> from
  line 13: ate -> to
  line 13: durante -> duration
  line 14: patamar -> steady
  line 14: taxa -> rate
  line 14: durante -> duration
  line 16: cenario -> scenario
  line 21: nome -> name
  line 22: verificar -> expect
  line 25: erros -> errors
  original kept at ci.yaml.bak

1 file converted. Next step:
  braunrate validate ci.yaml
```

O diff:

```diff
-nome: Fumaca de CI
-alvo: http://127.0.0.1:8080
+name: Fumaca de CI
+target: http://127.0.0.1:8080

-autenticacao:
-  tipo: token
-  obter:
-    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ci } }
-    captura: { token: $.access_token }
+auth:
+  type: token
+  obtain:
+    http: { method: POST, path: /auth/token, body: { usuario: ci } }
+    capture: { token: $.access_token }

-carga:
-  perfis:
-    - rampa: { de: 50/s, ate: 200/s, durante: 3s }
-    - patamar: { taxa: 200/s, durante: 3s }
+load:
+  profiles:
+    - ramp: { from: 50/s, to: 200/s, duration: 3s }
+    - steady: { rate: 200/s, duration: 3s }

-cenario:
+scenario:
   # Caminho fixo de proposito: fumaca so pergunta se o servico responde. A
   # ferramenta avisa que o numero fica otimista, e aqui isso esta certo — para
   # medir tempo de resposta de verdade, o caminho precisa variar.
   - http: GET /pedidos/1
-    nome: consultar pedido
-    verificar: { status: 200 }
+    name: consultar pedido
+    expect: { status: 200 }

 slo:
-  - global: { erros: < 0.1 }
+  - global: { errors: < 0.1 }
```

Os três comentários do arquivo estão intactos, na mesma posição, com a mesma
indentação. `usuario: ci` dentro do corpo não foi tocado: é campo da API do
alvo, não chave do formato. `consultar pedido` não foi tocado: é texto do autor.

A conversão reescreve por posição — o documento é lido só para saber que linha e
coluna guardam que chave, e os bytes originais são editados de trás para frente.
Reencodar a árvore do YAML teria sido mais curto e teria jogado fora todo
comentário.

**Um bug encontrado aqui.** Um passo sem `nome` reporta sob a chave que o
protocolo deriva, e três dessas mudaram: `kafka produzir` → `kafka produce`,
`amqp publicar` → `amqp publish`, `aguardar` → `await`. A primeira versão da
migração não as renomeava, e todo cenário de mensageria saía convertido e
inválido, apontando para passo que não existe mais. Corrigido: o prefixo só é
renomeado quando **nenhum passo declara aquele nome** — `aguardar o processador`
escrito pelo autor continua intacto. Foi o teste novo que pegou a primeira
versão, que renomeava os dois.

O formato antigo é reconhecido pelo parser, não só pelo `migrate`:

```
$ braunrate validate old.yaml
error in the scenario: old.yaml:2:1: this scenario uses the Portuguese format, replaced in 0.6.0 ("nome" is now "name").
    braunrate migrate <file>
    converts it to the English format, keeping comments and order. No behavior changes.
```

`-dry-run` lista o que mudaria sem escrever nada; `-output` grava em outro
arquivo; o padrão grava por cima deixando `.bak` ao lado; rodar de novo numa
pasta já convertida responde `nothing to convert: every scenario is already in
the English format`.

**Outro bug encontrado aqui.** Com `-output`, a linha de próximo passo apontava
para o arquivo original — o que não foi convertido. Corrigido.

---

## 3. As frases que carregam a tese, antes e depois

Traduzidas reescrevendo, não convertendo palavra a palavra.

### Laço fechado

**Antes**

> Este teste usou 3 usuarios em laco fechado. Se o alvo travar, os usuarios param
> de pedir e o atraso nao aparece nos numeros. O tempo de resposta abaixo pode
> estar melhor do que o usuario real sente.

**Depois**

> This test used 3 users in a closed loop. If the target freezes, the users stop
> asking and the delay never shows up in the numbers. The response time below may
> look better than what a real user feels.

"nao aparece nos numeros" virou "never shows up in the numbers" e não "does not
appear in the numbers": o ponto é que o atraso nunca chega a entrar na conta,
não que ele esteja escondido em algum lugar dela. E "pode estar melhor do que o
usuario real sente" virou "may look better than what a real user feels" — o
número não está melhor, ele parece melhor.

### Resultado inválido

**Antes**

> Resultado invalido: a execucao nao mediu o que se propos a medir. Isto nao e
> veredito sobre o alvo — e a medicao que nao vale, e por isso nenhuma regra de
> SLO foi avaliada.

**Depois**

> Invalid result: the run did not measure what it set out to measure. This is not
> a verdict on the target — it is the measurement that does not hold, and that is
> why no SLO rule was evaluated.

"se propos a medir" → "set out to measure" mantém a intenção declarada, que é o
que a frase acusa de não ter sido cumprida. "que nao vale" → "does not hold", que
é o que se diz de um argumento que não se sustenta, em inglês técnico; "is not
valid" teria soado a erro de formulário.

### Token único

**Antes**

> Autenticacao obtida uma vez e reaproveitada por todas as jornadas. Se o alvo
> tiver cache, rate limit ou sharding por token, este numero fica otimista.

**Depois**

> Auth obtained once and reused by every journey. If the target has caching, rate
> limiting or sharding by token, this number comes out optimistic.

"fica otimista" → "comes out optimistic", não "is optimistic": o número não é
otimista por natureza, ele sai assim daquela execução.

### Variedade colapsada

**Antes**

> a execucao inteira rodou com um unico valor de pedidos.id, embora a fonte tenha
> mais; o alvo pode ter respondido de cache, e o resultado nao representa a carga
> declarada

**Depois**

> the whole run went with a single value of pedidos.id, even though the source
> has more; the target may have answered from cache, and the result does not
> represent the declared load

A evidência que acompanha manteve a forma que torna o aviso verificável — o
disponível contra o usado: `<fonte> had <n> available values and the run used 1,
across <n> uses`.

### As cinco explicações de onboarding

A `braunrate demo` explica cinco conceitos no ponto em que o número aparece. A
tela repete as mesmas cinco, e um teste exige as duas coisas. Duas delas:

**Taxa, antes**

> Essa é a taxa: o braunrate dispara nesse ritmo esteja o serviço rápido ou
> lento — como usuários de verdade fazem. Ferramentas que esperam a resposta
> anterior antes de mandar a próxima aliviam o sistema justamente quando ele
> está sofrendo.

**Depois**

> That is the rate: braunrate fires at that pace whether the service is fast or
> slow — the way real users do. Tools that wait for the previous response before
> sending the next one go easy on the system exactly when it is struggling.

"aliviam o sistema justamente quando ele está sofrendo" → "go easy on the system
exactly when it is struggling". "relieve the system" seria a tradução literal e
perderia a ironia, que é o que faz a frase ficar.

**Média, antes**

> Repare que não existe média nessa linha. Média esconde: se 95 respostas levam
> 5 ms e 5 levam 2 segundos, a média dá 105 ms e ninguém percebe as cinco lentas.
> "95% em até 7,0 ms" quer dizer que 5% das pessoas esperaram mais que isso.

**Depois**

> Notice there is no average on that line. An average hides things: if 95
> responses take 5 ms and 5 take 2 seconds, the average reads 105 ms and nobody
> notices the five slow ones. "95% within 7.0 ms" means 5% of the people waited
> longer than that.

"a média dá 105 ms" → "the average reads 105 ms": em inglês o número que um
instrumento mostra "reads", e é exatamente a acusação da frase — o instrumento
está mostrando outra coisa.

---

## 4. O mesmo cenário, antes e depois

`examples/ci.yaml` contra o alvo embutido, mesma máquina, com o binário da 0.5.0
e com o desta fase.

**0.5.0**

```
executando "Fumaca de CI" contra http://127.0.0.1:8080: 975 iteracoes em 6s

Fumaca de CI — contra http://127.0.0.1:8080

Passou: a unica regra de SLO foi atendida.

O que aconteceu
  975 requisicoes em 6s, 162 por segundo, 0% de erro
  Metade das respostas em ate 5.5 ms; 95% em ate 6.9 ms; 99% em ate 7.6 ms; a pior levou 17 ms

A jornada inteira
  Todas as 975 jornadas chegaram ao fim; metade levou ate 6 ms e 95% ate 7 ms, contados do instante em que deveriam ter comecado.
  metade 5.5 ms | 95% 6.9 ms | 99% 7.6 ms | pior 17 ms

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)        975    5.5 ms    6.9 ms    7.6 ms     15 ms     17 ms       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.

SLO
  ok    Passou: o cenario inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.
  --    passos sem criterio declarado (1 de 1): consultar pedido
  --    regressao: sem criterio declarado — o gate aprova sem comparar com a execucao anterior

Confiabilidade da medicao
  Atencao: o passo "consultar pedido" nao tem nenhum valor que varia — toda requisicao vai ser identica.
            nenhum ${} no passo, entao ele nao entra na variedade observada
  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.
  Atraso tipico para disparar: 0.001 ms; pior caso: 6.0 ms (o tempo de resposta ja desconta isso)

Ambiente
  Mac darwin/arm64, 10 nucleos | braunrate dev | 2026-08-17 01:49:04
  Protocolos compilados: aguardar, amqp, graphql, http, kafka
  1 unico valor de token em 975 usos
  Autenticacao obtida uma vez e reaproveitada por todas as jornadas.
  Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.
```

**Agora**

```
running "CI smoke" against http://127.0.0.1:8080: 975 iterations in 6s

CI smoke — against http://127.0.0.1:8080

Passed: the single SLO rule was met.

What happened
  975 requests in 6s, 162 per second, 0% of them errors
  Half the responses within 5.6 ms; 95% within 7.0 ms; 99% within 7.9 ms; the worst took 17 ms

The whole journey
  All 975 journeys reached the end; half took up to 6 ms and 95% up to 7 ms, counted from the instant they should have started.
  half 5.6 ms | 95% 7.0 ms | 99% 7.9 ms | worst 17 ms

Per step
  step                             requests      half       95%       99%     99.9%     worst  errors
  look up order              (1)        975    5.6 ms    7.0 ms    7.9 ms     11 ms     17 ms       0

  (1) time counted from the instant the request should have gone out — it includes
      any delay, and for that reason it does not hide a freeze in the target.

SLO
  ok    Passed: the whole scenario had the error rate of 0.00%, within the limit of 0.10%.
  --    steps with no criterion declared (1 of 1): look up order
  --    regression: no criterion declared — the gate approves without comparing against the previous run

How trustworthy the measurement is
  Warning: the step "look up order" has no value that varies — every request will be identical.
            no ${} in the step, so it does not enter the observed variety
  Every request went out on schedule, so the numbers above reflect the target, not the generator.
  Typical delay to fire: 0.001 ms; worst case: 5.1 ms (the response time already discounts it)

Environment
  Mac darwin/arm64, 10 cores | braunrate dev | 2026-08-17 01:49:22
  Compiled protocols: amqp, await, graphql, http, kafka
  1 single value of token across 975 uses
  Auth obtained once and reused by every journey.
  If the target has caching, rate limiting or sharding by token, this number comes out optimistic.
```

A diferença nos números é ruído entre duas execuções na mesma máquina; a
estrutura do relatório é a mesma linha por linha.

O nome do passo mudou de `consultar pedido` para `look up order` porque o
exemplo foi traduzido junto — os exemplos apontam para o alvo embutido, que
passou a servir `/orders`, e em português eles deixariam de rodar.

---

## 5. O caminho do zero, cronometrado

Pasta vazia, alvo embutido no ar, binário desta fase:

| Comando | Tempo |
|---|---|
| `braunrate new cenario.yaml` | 0,02 s |
| `braunrate debug cenario.yaml` | 0,04 s |
| `braunrate execute cenario.yaml -html report.html -result result.json` | 1m30s (a duração declarada pelo cenário que o `new` escreve) |

Tudo em inglês, do primeiro caractere ao último. O `new` escreve o cenário
comentado, o `debug` roda uma iteração e aponta o `execute`, o `execute` termina
apontando o `compare`.

**Um bug encontrado aqui.** O `debug` imprimia o título `variables at the end of
the iteration` com nada embaixo quando o cenário não tinha captura nem variável
— que é exatamente o cenário que o `new` escreve. Corrigido em commit próprio.

---

## 6. A varredura

`internal/i18n/sweep_test.go` lê as cadeias de caracteres pelo AST — comentário
e nome de identificador ficam de fora, e o que sobra é exatamente o que chega na
tela. Confere também o schema publicado, chave por chave e valor por valor, e as
chaves dos cenários de exemplo.

A lista de exceções é explícita e cada entrada diz por que existe:

| Exceção | Por quê |
|---|---|
| `cpf`, `cnpj` | é o nome da coisa; traduzir nomearia um documento que não existe |
| `senha`, `segredo` | nomes de campo que a ferramenta reconhece **para poder recusar** uma credencial literal |
| `internal/scenario/migration.go` | carrega o mapa do formato antigo de propósito |
| `internal/site`, `cmd/site` | o site é bilíngue na fase 2 |

A varredura achou quatro sobras que a revisão manual não tinha achado:
`CheckBody` ainda valia `"corpo_contem"`, o status de execução do servidor dizia
`"passou"` e `"falhou o SLO"`, e o schema publicado tinha `"jornada_p99.9"`,
`"global_p99.9"` e o exemplo `"cabecalho:Location"`.

Outras duas só apareceram rodando: o mascaramento de credencial dizia
`(14 caracteres)`, e o rótulo `request:` do `debug` estava desalinhado das outras
linhas do bloco porque a coluna tinha sido acertada para `requisicao:`.

---

## 7. Critério item por item

| Critério | Estado |
|---|---|
| Mapa completo de chaves **e valores** em ADR, antes do código | ✅ [ADR 0019](adr/0019-formato-em-ingles.md), commit `f62fe2d`, antes de qualquer renomeação |
| Uma convenção de nomes, sem exceção | ✅ camelCase; não há chave com underscore no formato novo |
| Fonte única para os oito produtores de YAML | ✅ parser, schema, DSL, `new`, `import curl`, `import jmx`, `record` e interface web leem o mesmo mapa |
| `migrate` obrigatório: arquivo ou pasta | ✅ |
| grava por cima com `.bak`, ou `-output` | ✅ |
| preserva comentários e ordem das chaves | ✅ testado comentário por comentário e por número de linhas |
| lista o que mudou | ✅ linha, de, para |
| recusa arquivo já convertido | ✅ `nothing to convert` |
| `--dry-run` | ✅ `-dry-run` |
| testado contra todos os exemplos da 0.5.0 | ✅ os sete, congelados em testdata |
| Formato antigo detectado com mensagem que ensina | ✅ pelo parser, não só pelo `migrate` |
| Mensagens reescritas, não convertidas | ✅ seção 3 |
| Varredura sem português, com exceções explícitas | ✅ `internal/i18n` |
| conferida contra o schema | ✅ chave, valor e descrição |
| Nenhuma mudança de comportamento | ✅ três bugs corrigidos em commits próprios: a chave de slo derivada, o bloco de variáveis vazio e o próximo passo do `-output` |
| CI verde, lint zero, exemplos rodando, a cada commit | ✅ `go test ./...` e `golangci-lint run ./...` em cada um dos 19 commits da fase |
| Exemplo publicado travado por CI | ✅ `docs/exemplo-resultado.json` e `docs/exemplo-relatorio.html` regenerados de uma execução real, no formato 3 |
| Commits pequenos, em português, Conventional Commits | ✅ |
| Decisões registradas | ✅ oito em [decisoes-i18n.md](decisoes-i18n.md) |

## O que ficou de fora, e por quê

- **`internal/site` e `cmd/site`** continuam em português: são o gerador do site,
  que vira bilíngue na fase 2.
- **`docs/guias/`** teve os blocos YAML migrados para o formato novo — CI verde a
  cada commit exige isso — mas a prosa continua em português até a fase 2.
- **ADRs, commits e relatórios internos** continuam em português por decisão da
  tabela de camadas.
- **`docs/auditoria-fricao.md` e `docs/relatorio-experiencia.md`** não foram
  tocados: são registros de um momento, e reescrevê-los seria falsificar o que
  foi observado naquele dia.
