# Relatório da Fase 3 — Exemplos na referência do cenário

A referência saía do schema, e o schema tinha 94 chaves sem um único exemplo.
Quem chegava nela lia o tipo da chave e adivinhava a forma do valor. Agora toda
chave tem descrição e exemplo, todo exemplo passa pelo parser em teste, e a
página abre com um cenário inteiro antes da primeira tabela.

- **Commits**: 5, de `12a862a` a `d8e5187`, todos com `go test ./...` e
  `golangci-lint run ./...` verdes.
- **Schema**: 163 chaves, de 1146 para 1723 linhas; 54 exemplos viraram 176.
- **Defeitos encontrados no caminho**: 6, sendo 5 divergências entre o que o
  schema prometia e o que o parser aceita.
- **Decisões registradas**: 13 a 16 em [decisoes-i18n.md](decisoes-i18n.md).

---

## 1. A auditoria

A unidade contada é a chave como o usuário a escreve, e não o `$defs` onde ela
mora: a caminhada resolve `$ref`, então `method` documentada uma vez no tipo
`http` conta uma vez para cada passo que aponta para ele. Mapa livre (`variables`,
`data`, `headers`) conta como uma chave só, porque o nome de dentro é escolhido
por quem escreve.

| | Antes (`b5c63f7`) | Depois (`d8e5187`) |
| --- | ---: | ---: |
| Chaves | 163 | 163 |
| Com `description` | 124 | **163** |
| Sem `description` | 39 | **0** |
| Com `examples` | 69 | **163** |
| Sem `examples` | 94 | **0** |
| Com `default` explícito | 8 | **15** |
| Exemplos publicados | 54 | **176** |

Os 15 `default` são 13 valores distintos, cada um conferido contra o código que
o aplica:

| Chave | `default` | Onde o código o aplica |
| --- | --- | --- |
| `load.model` | `open` | `internal/scenario/yaml.go:92` |
| `http.method` | `GET` | `internal/protocol/http/http.go:181` |
| `http.followRedirects` | `true` | `internal/protocol/protocol.go:229` |
| `auth.header` | `Authorization: Bearer ${token}` | `internal/scenario/yaml_correlation.go:269` |
| `capture.required` | `true` | `internal/scenario/yaml_correlation.go:47` |
| `data.*.consume` | `circular` | `internal/scenario/yaml_correlation.go:288` |
| `graphql.path` | `/graphql` | `internal/protocol/graphql/graphql.go:174` |
| `kafka.acks` | `all` | `internal/protocol/kafka/kafka.go:269` |
| `amqp.persistent` | `true` | `internal/protocol/amqp/amqp.go:169` |
| `amqp.confirm` | `true` | `internal/protocol/amqp/amqp.go:169` |
| `await.timeout` | `30s` | `internal/protocol/wait/wait.go:18` |
| `await.interval` | `500ms` | `internal/protocol/wait/http.go:17` |
| `generate.*.newEvery` | `iteration` | `internal/scenario/yaml_correlation.go:353` |

Nenhum `default` foi inventado para preencher tabela. Chave sem valor implícito
no código continua sem `default` publicado.

## 2. Os seis defeitos

Quatro vieram de ler o schema chave por chave; dois vieram do teste novo, na
primeira vez que ele rodou. Os dois do teste existiam desde que o schema foi
escrito: ninguém confere à mão um arquivo que só o editor lê.

**1. `default` em português.** `kafka.acks` publicava `"todos"` e
`generate.*.newEvery` publicava `"iteracao"`. O parser aceita `all` e
`iteration`. O editor completava com a palavra que a ferramenta recusa.
Commit `12a862a`.

**2. O gerador `pattern` publicado como `default`.** A tradução da Fase 1 passou
por `padrao` e escolheu a palavra errada das duas: aqui `padrao` era molde, não
valor implícito. O `enum` oferecia um gerador chamado `default`, que não existe.
Commit `12a862a`.

**3. `auth.header` documentado como nome.** A descrição dizia "o nome do
cabeçalho, `Authorization` por padrão"; o parser lê a linha inteira, nome e
valor. Quem escrevia `header: X-API-Key` mandava um cabeçalho sem valor.
Commit `e692947`.

**4. `$await.to` não existe.** O schema documentava um campo `to` com a condição
de parada; o parser chama esse campo de `until`. O passo de espera é o único que
fecha a medição da cadeia assíncrona, e ele era o mais difícil de escrever com o
schema aberto ao lado. Commit `001da04`.

**5. `$expect.status` prometia mais do que o parser aceita.** O schema oferecia
inteiro, lista de inteiros, `"2xx"` e `"< 400"`. O parser aceita um inteiro.
Cumprir a promessa maior seria mudança de comportamento, e esta fase não tem
nenhuma: o schema passou a descrever o que existe, com a descrição dizendo o
porquê — um passo que aceita mais de um status são dois passos. Commit `001da04`.

**6. Exemplos em português.** `ConsultarPedido`, `cobranca`, `PROCESSADO`,
`kafka.homolog:9093`, `$.ultimaFatura.id`, `cookie:sessao`. A Fase 1 traduziu
descrições e não olhou os exemplos, que são justamente o que se copia.
Commits `e692947` e `001da04`.

## 3. Os exemplos

De uma a três formas por chave, escolhidas para mostrar o que a descrição não
diz: `$limit` mostra `< 150ms` e `> 250/s` porque o operador muda de sentido
conforme a métrica; `load.model` mostra `open` e `closed` porque cada um leva um
bloco diferente embaixo.

Onde o campo recebe credencial, o exemplo mostra a variável:

```yaml
auth:
  type: header
  header: "X-API-Key: ${API_KEY}"
```

A ferramenta recusa valor literal na validação desde o começo. Documentação que
ensinasse `senha123` transformaria essa recusa em obstáculo, e o primeiro
reflexo de quem esbarra em obstáculo é procurar como desligá-lo.

## 4. A página

- **Um cenário inteiro no topo**, em bloco YAML, antes da primeira tabela. A
  tabela responde "o que essa chave aceita" e nunca respondeu "onde ela vai";
  quem chega pela busca do editor cai no meio da árvore.
- **`default` ao lado do tipo**, na mesma célula: `text · default 500ms`.
- **Obrigatória e opcional distinguidas na tabela** por palavra e por cor: a
  coluna diz `yes` ou `—`, e a cor vem depois disso, não no lugar disso.
- **Clique para copiar em cada valor de exemplo.** Selecionar texto de uma
  célula estreita com o mouse é onde a pessoa desiste e digita de novo.

A página em inglês tem 59 KB, a em português 59 KB, e as duas continuam sem
buscar nada da rede.

## 5. O que trava isso

Quatro testes, todos em `internal/site`:

| Teste | O que reprova |
| --- | --- |
| `TestEverySchemaKeyIsDescribedAndExemplified` | Chave nova sem descrição ou sem exemplo. |
| `TestEverySchemaExampleIsAcceptedByTheParser` | Exemplo que o parser recusa — e exemplo sem cenário onde encaixá-lo. |
| `TestNoSchemaExampleCarriesALiteralCredential` | Aparência de segredo em qualquer exemplo. |
| `TestTheCompleteScenarioAtTheTopOfTheReferenceRuns` | O cenário do topo da página, nas duas línguas. |

O segundo monta, para cada exemplo, o menor cenário que exercita aquela chave e
roda `Parse` mais `Validate` no cenário inteiro. Chave que tem exemplo e não tem
cenário de apoio **reprova**, em vez de ser pulada: um teste que pula em
silêncio o que não sabe montar deixa de provar exatamente o exemplo novo, que é
o que ninguém conferiu ainda. Foi assim que os defeitos 4 e 5 apareceram.

## 6. Critério a critério

| Critério da fase | Estado |
| --- | --- |
| Tabela de auditoria antes de implementar | Feita; ela é a seção 1. |
| Um a três exemplos por chave | 176 exemplos cobrindo as 163 chaves. |
| `default` explícito | 15, todos conferidos contra o código. |
| Nenhuma credencial literal | Teste próprio, sobre todos os exemplos. |
| Teste que passa todo exemplo pelo parser | `TestEverySchemaExampleIsAcceptedByTheParser`. |
| Obrigatória e opcional visualmente distintas | Palavra (`yes` / `—`) e cor. |
| `default` ao lado do tipo | Mesma célula. |
| Botão de copiar | Em cada valor de exemplo da referência. |
| Cenário inteiro no topo | Primeiro bloco da página, validado em teste. |
| Nenhuma mudança de comportamento | Nenhuma. O defeito 5 reduziu a promessa do schema em vez de ampliar o parser. |
| CI verde, lint zero, a cada commit | 5 de 5. |
| Decisões registradas | 13 a 16 em `decisoes-i18n.md`. |

## 7. O que ficou de fora

- **A referência continua em inglês nas duas árvores**, com a moldura traduzida.
  É a decisão 12, da Fase 2: as descrições saem do schema, e o schema é o arquivo
  que o editor lê durante a escrita.
- **`$expect.status` com faixa** (`2xx`) não foi implementada. O schema parou de
  prometê-la; se ela voltar, volta com o parser junto.
- **Nenhum exemplo foi executado contra um alvo de verdade.** Eles passam pelo
  parser e pela validação, que é o que a fase pedia; rodar cada um contra um alvo
  exigiria um alvo por protocolo.
