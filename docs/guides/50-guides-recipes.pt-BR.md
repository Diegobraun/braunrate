---
translated_from: 50-guides-recipes.en.md
source_hash: 9ae217c8d639
---
# Receitas

Situações que aparecem quase sempre, e o que escrever no cenário para cada uma.

## Como escolher a taxa

O braunrate não sabe o seu tráfego, então não tem como escolher o número por
você. O que ele tem é a conta.

**Comece pela produção.** Pegue as requisições que o serviço atendeu num dia
movimentado e divida pelos segundos do dia:

```
4.000.000 requisições por dia ÷ 86.400 segundos = 46/s em média
```

Essa média não é o número do teste, porque tráfego não é plano. O pico costuma
ser de 3 a 5 vezes a média diária, então a faixa que vale testar aqui é de 140/s
a 230/s. Se você tem um gráfico de tráfego, leia o pico nele em vez de
multiplicar — o gráfico é medição e o multiplicador é hábito.

Se ninguém tem o número, peça a quem opera o serviço as requisições por minuto na
hora mais cheia e divida por 60. Contar as linhas do log de acesso de uma hora
também serve.

**Depois ache o teto com uma rampa.** A taxa acima é a que você precisa
sustentar; o teto é onde o alvo deixa de sustentar. Uma rampa acha isso numa
execução só:

```yaml fragment
load:
  profiles:
    - ramp: { from: 10/s, to: 400/s, duration: 3m }
```

Rode e abra o relatório. A latência fica plana enquanto o alvo acompanha e sobe
quando ele deixa de acompanhar; a taxa do momento em que ela começou a subir é o
teto. Se ela nunca subir, o teto está acima de 400/s — aumente o `to` e rode de
novo.

Duas coisas que deixam a resposta errada:

- **requisição sempre idêntica.** Ela mede o cache, então o teto sai mais alto do
  que é. O relatório avisa quando o cenário não tem variedade.
- **o gerador acabar antes do alvo.** O modelo de chegada foi medido sustentando
  20.000/s em todas as repetições e falhando em algumas acima disso ([os números
  da fase 0](https://github.com/Diegobraun/braunrate/blob/main/docs/medicoes-fase0.md)).
  A validação pergunta quando o cenário passa dessa linha. Acima dela, o número
  do relatório pode ser o braunrate e não o alvo.

**Por fim, declare o que você achou.** O teto entra no critério de aceite, e a
taxa que você precisa sustentar entra no perfil:

```yaml fragment
load:
  profiles:
    - ramp: { from: 1/s, to: 230/s, duration: 30s }
    - steady: { rate: 230/s, duration: 5m }

slo:
  - global: { p95: < 400ms, errors: < 1 }
```

## O endpoint exige login

Declare uma vez como o token é obtido. O braunrate faz o login antes de começar
a medir e injeta o token em todos os passos; nenhum passo precisa declarar o
cabeçalho.

```yaml fragment
auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { usuario: "${usuario}", senha: "${SENHA}" } }
    capture: { token: $.access_token }
  refreshAfter: 25m
```

A senha vem de uma variável de ambiente. Se você escrever o valor no arquivo, a
validação recusa e explica como fazer — o cenário vai para o repositório, e o
repositório guarda para sempre.

Duas coisas para saber antes de acreditar no número:

- o login acontece na preparação, fora do relógio. Se entrasse na medição, a
  primeira requisição carregaria o custo de todas.
- **é um token só para a execução inteira.** Se o alvo tiver cache por
  identidade, limite por token ou sharding por usuário, o resultado fica
  otimista. O relatório repete isso toda vez.

Arquivo completo: [`examples/authenticated-journey.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/authenticated-journey.yaml).

### Cabeçalho, query string e caminho

`obtain` é uma requisição inteira: método, caminho com query string, cabeçalhos e
corpo. E tudo o que a captura produz vira variável nos passos seguintes, então o
token entra onde você escrever `${token}` — cabeçalho, query ou caminho.

```yaml fragment
auth:
  type: token
  header: "Authorization: Bearer ${token}"
  obtain:
    http:
      method: POST
      path: /auth/token?origem=braunrate&tenant=${TENANT}
      headers: { X-Cliente: teste-de-carga }
      body: { usuario: ana, senha: "${SENHA}" }
    capture: { token: $.access_token }

scenario:
  - http:
      method: POST
      path: /faturas/f-${pedidos.id}/pagar?tk=${token}&tenant=${TENANT}
      headers: { X-Correlacao: "${pedidos.id}" }
    name: pagar fatura
```

O `braunrate debug` mostra o que saiu, com o segredo cortado:

```
step 1 — pagar fatura   [ok in 5.9ms]
  request:    POST /faturas/f-3958/pagar?tk=token-de-teste&tenant=acme
              Authorization: Bearer test-t… (10 characters)
              X-Correlacao: 3958
  response:   status 200, 63 bytes
```

> **Nota** A injeção automática é sempre um cabeçalho, e `header:` aceita
> qualquer `Nome: valor` — o padrão é `Authorization: Bearer ${token}`. Token na
> query ou no caminho é você quem escreve, com `${token}` no passo.

Duas coisas que economizam uma volta:

- **Dentro de `obtain` só cabem ambiente e valor fixo.** O login acontece uma vez,
  antes das jornadas, então `${pedidos.id}` ali não existe — e a validação recusa
  em vez de mandar a requisição com o campo vazio.
- **`${TENANT}` em caixa alta vem do ambiente sem declarar nada.** Nome em
  minúscula precisa de `variables`, `data` ou `capture`.

### Uma chave de API, sem login

Quando não há requisição de login — a credencial já está no ambiente — o bloco
tem uma linha:

```yaml fragment
auth: { type: header, header: "X-API-Key: ${API_KEY}" }
```

O cabeçalho vai em todos os passos, e a saída mostra que ele foi enviado sem
mostrar o valor:

```
step 1 — consultar pedido   [ok in 7.4ms]
  request:    GET /pedidos/1?origem=braunrate
              X-API-Key: ***
```

## Cada jornada precisa de dados próprios

Duas fontes: um CSV que você já tem, ou valores gerados.

```yaml fragment
data:
  assinantes: { file: dados/assinantes.csv, consume: circular }
  pedidos: { generate: { id: uuid, valor: "numero(10,500)" } }
```

O CSV vira `${assinantes.coluna}`; o gerador vira `${pedidos.id}`. Geradores
disponíveis: `uuid`, `sequence`, `number(min,max)`, `integer(min,max)`, `name`,
`email`, `text(n)`, `pattern`, `cpf`, `cnpj`. CPF e CNPJ saem com dígito
verificador válido, senão o alvo recusaria tudo e o teste mediria o caminho da
recusa.

**Um valor por jornada, não por requisição.** É o que a chave de idempotência
precisa: o mesmo `transactionId` nas duas chamadas da mesma jornada, um novo na
jornada seguinte. Esse é o padrão, sem declarar nada:

```yaml fragment
data:
  pagamento:
    generate: { transactionId: uuid }

scenario:
  - http: { method: POST, path: /pedidos, headers: { X-Idempotency-Key: "${pagamento.transactionId}" } }
  - http: { method: GET, path: "/pedidos/${pedidoId}", headers: { X-Idempotency-Key: "${pagamento.transactionId}" } }
```

Quando o valor precisa ser novo a cada uso, declare:

```yaml fragment
data:
  chaves:
    generate:
      nonce: { type: uuid, newEvery: use }
```

E, se o alvo exige um formato específico, `pattern` monta: `#` vira dígito, `@`
vira letra, o resto sai literal.

```yaml fragment
data:
  cadastro:
    generate:
      referencia: { type: pattern, format: "PED-######" }   # PED-481902
      filial:     { type: pattern, format: "@@-####" }      # KQ-3718
```

Não existe leitura de `.xlsx`. CSV cobre o caso, e carregar uma dependência de
Excel dentro do motor de carga custa mais do que resolve.

### Repetir a mesma execução, ou variar de propósito

Semente fixa no arquivo faz o CI rodar sempre o mesmo caso, e um caso que passa
mil vezes não prova mais nada depois da primeira. A semente aceita o ambiente:

```yaml fragment
data:
  pedidos:
    generate: { id: uuid, valor: "numero(10,500)" }
    seed: ${SEMENTE:-42}
```

Sem a variável, roda com 42 e nada muda de um dia para o outro. Com ela, o
relatório publica a semente que rodou e a linha que traz o caso de volta:

```
  Data seeds: pedidos=8817 (from $SEMENTE) (the same seed generates the same values again)
  To repeat exactly this data, run again with SEMENTE=8817
```

## O teste precisa exercitar várias rotas

Produção não é a mesma chamada mil vezes. `weight` reparte a taxa entre
alternativas, e cada iteração executa uma delas:

```yaml fragment
scenario:
  - name: consultar pedido
    weight: 60
    http: { method: GET, path: "/pedidos/${pedidos.id}" }
  - name: criar pedido
    weight: 10
    http: { method: POST, path: /pedidos }
```

A escolha é por posição no ciclo, não sorteio, então duas execuções do mesmo
arquivo aplicam exatamente o mesmo mix. O relatório mostra a proporção que saiu
ao lado da declarada:

```
Mix declared and observed
  consultar pedido             60.0% declared     60.0% observed (300 of 500)
  consultar fatura             30.0% declared     30.0% observed (150 of 500)
  criar pedido                 10.0% declared     10.0% observed (50 of 500)
```

`weight` escolhe qual alternativa roda, não qual passo dentro de uma jornada. Um
cenário com captura encadeada é uma jornada só, e a validação recusa `weight` nele.

Arquivo completo: [`examples/operation-mix.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/operation-mix.yaml).

### Quando cada perfil de cliente segue uma rota diferente

Não existe passo condicional. Qual caminho cada perfil percorre, e em que
proporção, é conhecimento de negócio — nenhuma observação de tráfego revela
isso, e quem gravou uma passagem gravou um perfil. Então a ramificação é por
dado: o CSV declara a proporção pelas linhas que tem.

```yaml fragment
data:
  clientes: { file: dados/clientes.csv, consume: circular }

scenario:
  - name: consultar limite
    http: { method: GET, path: "/${clientes.rota}/${clientes.id}/limite" }
```

O que essa forma ainda não resolve: os dois perfis caem numa linha só do
relatório, então um perfil caro aparece como cauda do passo inteiro.

Arquivo completo: [`examples/branching-by-profile.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/branching-by-profile.yaml).

## De onde o braunrate tira cada `${variável}`

De quatro lugares, e de nenhum outro:

```yaml fragment
variables:
  tenant: acme                      # fixa no cenário
  regiao: "${REGIAO:-us-east-1}"    # do ambiente, com reserva

data:
  pedidos: { generate: { id: uuid } }  # e então ${pedidos.id}

scenario:
  - http: GET /faturas
    capture: { faturaId: "$.itens[0].id" }   # e então ${faturaId} nos passos seguintes
```

Nome em CAIXA ALTA vem do ambiente sem precisar declarar nada: `${API_KEY}`,
`${KAFKA_SENHA}`. É a mesma forma que o `import curl` e o `record` escrevem
sozinhos.

Vale em qualquer campo do cenário, não numa lista de campos escolhidos: alvo,
taxa, duração, tópico, fila, nome de passo, cabeçalho, caminho de certificado.

```yaml fragment
target: "${ALVO:-http://127.0.0.1:8080}"
load:
  profiles:
    - steady: { rate: "${TAXA:-100}/s", duration: "${DURACAO:-1m}" }
```

Duas armadilhas comuns:

- **dentro de `{ }` o valor precisa de aspas.** O YAML lê `{`, `}` e `[` como
  estrutura, então `caminho: /pedidos/${id}` dentro de um mapa em linha não
  carrega.
- **sem valor de reserva, a validação reclama** em vez de deixar o campo vazio.
  Ou defina a variável, ou escreva `${TAXA:-100}`.

## Fazer o teste reprovar o build

```bash
braunrate execute cenario.yaml -quiet -result=saida.json
```

O código de saída basta, não é preciso ler nada: `0` passou, `1` o critério de
aceite reprovou, `2` o arquivo do cenário tem erro, `3` a execução não mediu o
que se propôs a medir.

Se o cenário depende de infraestrutura que nem sempre existe, declare — o laço
de exemplos pula com aviso visível em vez de quebrar:

```yaml fragment
requires: [kafka]
```

Arquivo completo: [`examples/ci.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/ci.yaml).

## Descobrir se ficou mais lento que ontem

Guarde o resultado de uma execução e passe como base na seguinte:

```bash
braunrate execute cenario.yaml -baseline=execucao-anterior.json
```

Com uma regra de `regression` declarada, o critério por passo continua aprovando
e a comparação pega o que ele não vê:

```
  ok    Passed: "consultar pedido" answered 95% within 61 ms, within the limit of 150 ms.
  FAIL  Failed: the response time of 95% of the journeys came out 931.0% worse than execucao-anterior.json, above the limit of 10% worse (from 12 ms to 122 ms).
```

Quando alguma coisa fora do serviço mudou entre as duas — outra máquina, outra
versão do braunrate, outro modelo de chegada — a regra **não reprova**, e diz o
motivo. Culpar o serviço por uma diferença que a comparação não consegue
atribuir a ele seria pior que não comparar.

Fora do gate, para olhar com calma:

```bash
braunrate compare antes.json depois.json -html comparacao.html
```

```
It got slower: the whole journey (95%): 215 times slower — from 11 ms to 2435 ms. With 1 caveat about what changed outside the service.

Per step
  step                        95% before   95% after           change
  consultar pedido                5.8 ms      2.41 s       420x worse
  pagar fatura                    5.6 ms       11 ms       2.1x worse

What could explain the difference other than the service
  - both runs used one token for everything; caching or sharding by identity affects them the same way, but it does not disappear from the comparison
  Two runs give no confidence interval: a change below 5% is treated as noise.
```

Variação abaixo de 5% é tratada como ruído: duas execuções não dão intervalo de
confiança.

## Quando o YAML não dá conta

Laço sobre uma lista, decisão no meio da jornada, dado vindo de um sistema seu.
O mesmo cenário se escreve em Go, e roda no mesmo motor:

```go
// Scenario is the same journey of examples/authenticated-journey.yaml, written
// in Go: same engine, same metrics, same result document.
func Scenario(target string) (braunrate.Scenario, error) {
	return dsl.New("Billing journey").
		Target(target).
		Auth(dsl.WithToken(
			dsl.POST("/auth/token").Body(map[string]any{"user": "ana", "password": "${PASSWORD:-secret}"}),
			dsl.Capture("token", "$.access_token"),
		).RefreshAfter(25*time.Minute)).
		DataFromFile("subscribers", "data/subscribers.csv", dsl.Consume(dsl.Circular)).
		Ramp(dsl.PerSecond(50), dsl.PerSecond(300), 5*time.Second).
		Steady(dsl.PerSecond(300), 5*time.Second).
		Step(dsl.GET("/orders/${subscribers.id}"),
			dsl.Name("look up order"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.lastInvoice.status", "OPEN"),
			dsl.Capture("invoiceId", "$.lastInvoice.id")).
		Step(dsl.POST("/invoices/${invoiceId}/pay").
			Body(map[string]any{"amount": 199.90}),
			dsl.Name("pay invoice"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.status", "PAID")).
		SLO("look up order", "p95", "< 150ms").
		SLO("pay invoice", "p95", "< 200ms").
		JourneySLO("p95", "< 2s").
		JourneySLO("p99", "< 5s").
		OverallSLO("errors", "< 0.1").
		Build()
}
```

Esse trecho não é ilustração: ele é o arquivo
[`examples/scenario-in-go/scenario.go`](https://github.com/Diegobraun/braunrate/blob/main/examples/scenario-in-go/scenario.go),
que o CI compila e roda contra o alvo embutido. Um teste reprova o build se esta
página se afastar do arquivo.

Migrar não é reescrever. A DSL não interpreta nada por conta própria:
`"$.lastInvoice.id"` e `"< 150ms"` passam pelas mesmas funções que leem o YAML,
e um teste compara a estrutura dos dois caminhos caso a caso.

O que continua exigindo mudança neste repositório é protocolo novo. E, até a v1,
os tipos públicos não estão congelados.
