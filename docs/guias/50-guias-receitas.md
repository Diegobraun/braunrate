# Receitas

Situações que aparecem quase sempre, e o que escrever no cenário para cada uma.

## O endpoint exige login

Declare uma vez como o token é obtido. O braunrate faz o login antes de começar
a medir e injeta o token em todos os passos; nenhum passo precisa declarar o
cabeçalho.

```yaml trecho
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: "${usuario}", senha: "${SENHA}" } }
    captura: { token: $.access_token }
  renovar_apos: 25m
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

Arquivo completo: [`examples/jornada-autenticada.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/jornada-autenticada.yaml).

## Cada jornada precisa de dados próprios

Duas fontes: um CSV que você já tem, ou valores gerados.

```yaml trecho
dados:
  assinantes: { arquivo: dados/assinantes.csv, consumo: circular }
  pedidos: { gerar: { id: uuid, valor: "numero(10,500)" } }
```

O CSV vira `${assinantes.coluna}`; o gerador vira `${pedidos.id}`. Geradores
disponíveis: `uuid`, `sequencia`, `numero(min,max)`, `inteiro(min,max)`, `nome`,
`email`, `texto(n)`, `padrao`, `cpf`, `cnpj`. CPF e CNPJ saem com dígito
verificador válido, senão o alvo recusaria tudo e o teste mediria o caminho da
recusa.

**Um valor por jornada, não por requisição.** É o que a chave de idempotência
precisa: o mesmo `transactionId` nas duas chamadas da mesma jornada, um novo na
jornada seguinte. Esse é o padrão, sem declarar nada:

```yaml trecho
dados:
  pagamento:
    gerar: { transactionId: uuid }

cenario:
  - http: { metodo: POST, caminho: /pedidos, cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
  - http: { metodo: GET, caminho: "/pedidos/${pedidoId}", cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
```

Quando o valor precisa ser novo a cada uso, declare:

```yaml trecho
dados:
  chaves:
    gerar:
      nonce: { tipo: uuid, novo_a_cada: uso }
```

E, se o alvo exige um formato específico, `padrao` monta: `#` vira dígito, `@`
vira letra, o resto sai literal.

```yaml trecho
dados:
  cadastro:
    gerar:
      referencia: { tipo: padrao, formato: "PED-######" }   # PED-481902
      filial:     { tipo: padrao, formato: "@@-####" }      # KQ-3718
```

Não existe leitura de `.xlsx`. CSV cobre o caso, e carregar uma dependência de
Excel dentro do motor de carga custa mais do que resolve.

### Repetir a mesma execução, ou variar de propósito

Semente fixa no arquivo faz o CI rodar sempre o mesmo caso, e um caso que passa
mil vezes não prova mais nada depois da primeira. A semente aceita o ambiente:

```yaml trecho
dados:
  pedidos:
    gerar: { id: uuid, valor: "numero(10,500)" }
    semente: ${SEMENTE:-42}
```

Sem a variável, roda com 42 e nada muda de um dia para o outro. Com ela, o
relatório publica a semente que rodou e a linha que traz o caso de volta:

```
  Sementes dos dados: pedidos=8817 (de $SEMENTE) (a mesma semente gera os mesmos valores de novo)
  Para repetir exatamente estes dados, rode de novo com SEMENTE=8817
```

## O teste precisa exercitar várias rotas

Produção não é a mesma chamada mil vezes. `peso` reparte a taxa entre
alternativas, e cada iteração executa uma delas:

```yaml trecho
cenario:
  - nome: consultar pedido
    peso: 60
    http: { metodo: GET, caminho: "/pedidos/${pedidos.id}" }
  - nome: criar pedido
    peso: 10
    http: { metodo: POST, caminho: /pedidos }
```

A escolha é por posição no ciclo, não sorteio, então duas execuções do mesmo
arquivo aplicam exatamente o mesmo mix. O relatório mostra a proporção que saiu
ao lado da declarada:

```
Mix declarado e observado
  consultar pedido             60.0% declarado     60.0% observado (300 de 500)
  consultar fatura             30.0% declarado     30.0% observado (150 de 500)
  criar pedido                 10.0% declarado     10.0% observado (50 de 500)
```

`peso` escolhe qual alternativa roda, não qual passo dentro de uma jornada. Um
cenário com captura encadeada é uma jornada só, e a validação recusa `peso` nele.

Arquivo completo: [`examples/mix-de-operacoes.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/mix-de-operacoes.yaml).

### Quando cada perfil de cliente segue uma rota diferente

Não existe passo condicional. Qual caminho cada perfil percorre, e em que
proporção, é conhecimento de negócio — nenhuma observação de tráfego revela
isso, e quem gravou uma passagem gravou um perfil. Então a ramificação é por
dado: o CSV declara a proporção pelas linhas que tem.

```yaml trecho
dados:
  clientes: { arquivo: dados/clientes.csv, consumo: circular }

cenario:
  - nome: consultar limite
    http: { metodo: GET, caminho: "/${clientes.rota}/${clientes.id}/limite" }
```

O que essa forma ainda não resolve: os dois perfis caem numa linha só do
relatório, então um perfil caro aparece como cauda do passo inteiro.

Arquivo completo: [`examples/ramificacao-por-perfil.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/ramificacao-por-perfil.yaml).

## De onde o braunrate tira cada `${variável}`

De quatro lugares, e de nenhum outro:

```yaml trecho
variaveis:
  tenant: acme                      # fixa no cenário
  regiao: "${REGIAO:-us-east-1}"    # do ambiente, com reserva

dados:
  pedidos: { gerar: { id: uuid } }  # e então ${pedidos.id}

cenario:
  - http: GET /faturas
    captura: { faturaId: "$.itens[0].id" }   # e então ${faturaId} nos passos seguintes
```

Nome em CAIXA ALTA vem do ambiente sem precisar declarar nada: `${API_KEY}`,
`${KAFKA_SENHA}`. É a mesma forma que o `import curl` e o `record` escrevem
sozinhos.

Vale em qualquer campo do cenário, não numa lista de campos escolhidos: alvo,
taxa, duração, tópico, fila, nome de passo, cabeçalho, caminho de certificado.

```yaml trecho
alvo: "${ALVO:-http://127.0.0.1:8080}"
carga:
  perfis:
    - patamar: { taxa: "${TAXA:-100}/s", durante: "${DURACAO:-1m}" }
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

```yaml trecho
requer: [kafka]
```

Arquivo completo: [`examples/ci.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/ci.yaml).

## Descobrir se ficou mais lento que ontem

Guarde o resultado de uma execução e passe como base na seguinte:

```bash
braunrate execute cenario.yaml -baseline=execucao-anterior.json
```

Com uma regra de `regressao` declarada, o critério por passo continua aprovando
e a comparação pega o que ele não vê:

```
  ok    Passou: "consultar pedido" respondeu 95% em até 61 ms, dentro do limite de 150 ms.
  FALHA Falhou: o tempo de resposta de 95% das jornadas ficou 931.0% pior que execucao-anterior.json, acima do limite de 10% pior (de 12 ms para 122 ms).
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
Ficou mais lento: jornada inteira (95%): 71 vezes mais lento — de 10 ms para 675 ms. Com 1 ressalva que pode explicar a diferença sozinha.

Por passo
  passo                        95% antes  95% depois         variação
  consultar pedido                8.4 ms      598 ms         71x pior
  pagar fatura                  0.601 ms       43 ms         71x pior

O que pode explicar a diferença sem ser o serviço
  - as execuções usaram versões diferentes do braunrate: 0.2.0 e 0.3.0 (isso sozinho explica a diferença)
```

Variação abaixo de 5% é tratada como ruído: duas execuções não dão intervalo de
confiança.

## Quando o YAML não dá conta

Laço sobre uma lista, decisão no meio da jornada, dado vindo de um sistema seu.
O mesmo cenário se escreve em Go, e roda no mesmo motor:

```go
// Scenario is the same journey of examples/jornada-autenticada.yaml, written in
// Go: same engine, same metrics, same result document.
func Scenario(alvo string) (braunrate.Scenario, error) {
	return dsl.New("Jornada de cobrança").
		Target(alvo).
		Auth(dsl.WithToken(
			dsl.POST("/auth/token").Body(map[string]any{"usuario": "ana", "senha": "${SENHA:-segredo}"}),
			dsl.Capture("token", "$.access_token"),
		).RefreshAfter(25*time.Minute)).
		DataFromFile("assinantes", "dados/assinantes.csv", dsl.Consume(dsl.Circular)).
		Ramp(dsl.PerSecond(50), dsl.PerSecond(300), 5*time.Second).
		Plateau(dsl.PerSecond(300), 5*time.Second).
		Step(dsl.GET("/pedidos/${assinantes.id}"),
			dsl.Name("consultar pedido"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.ultimaFatura.status", "ABERTA"),
			dsl.Capture("faturaId", "$.ultimaFatura.id")).
		Step(dsl.POST("/faturas/${faturaId}/pagar").
			Body(map[string]any{"valor": 199.90}),
			dsl.Name("pagar fatura"),
			dsl.CheckStatus(200),
			dsl.CheckJSON("$.status", "PAGA")).
		SLO("consultar pedido", "p95", "< 150ms").
		SLO("pagar fatura", "p95", "< 200ms").
		JourneySLO("p95", "< 2s").
		JourneySLO("p99", "< 5s").
		OverallSLO("erros", "< 0.1").
		Build()
}
```

Esse trecho não é ilustração: ele é o arquivo
[`examples/cenario-em-go/cenario.go`](https://github.com/Diegobraun/braunrate/blob/main/examples/cenario-em-go/cenario.go),
que o CI compila e roda contra o alvo embutido. Um teste reprova o build se esta
página se afastar do arquivo.

Migrar não é reescrever. A DSL não interpreta nada por conta própria:
`"$.ultimaFatura.id"` e `"< 150ms"` passam pelas mesmas funções que leem o YAML,
e um teste compara a estrutura dos dois caminhos caso a caso.

O que continua exigindo mudança neste repositório é protocolo novo. E, até a v1,
os tipos públicos não estão congelados.
