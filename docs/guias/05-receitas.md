# Receitas

Casos completos, cada um com o arquivo do repositorio que o exercita no CI.

## Autenticacao com renovacao

O motor obtem o token uma vez, na preparacao, e reaproveita em todas as jornadas.
O aperto de mao nao entra na medicao — se entrasse, a primeira requisicao
carregaria o custo de todas.

```yaml trecho
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: "${usuario}", senha: "${SENHA}" } }
    captura: { token: $.access_token }
  renovar_apos: 25m
```

A senha vem do ambiente. Segredo literal no arquivo reprova a validacao, e a
mensagem ensina a forma certa — o cenario vai para o repositorio, e o repositorio
guarda para sempre.

Exemplo completo: [`examples/jornada-autenticada.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/jornada-autenticada.yaml).

## Dados: um valor por jornada, nao por requisicao

O caso mais comum e a chave de idempotencia: **o mesmo `transactionId` nas duas
requisicoes da mesma jornada, um novo na jornada seguinte**. Esse e o padrao, sem
declarar nada:

```yaml trecho
dados:
  pagamento:
    gerar: { transactionId: uuid }

cenario:
  - http: { metodo: POST, caminho: /pedidos, cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
  - http: { metodo: GET, caminho: "/pedidos/${pedidoId}", cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
```

Se o valor precisar ser novo **a cada uso**, isso e declarado:

```yaml trecho
dados:
  chaves:
    gerar:
      nonce: { tipo: uuid, novo_a_cada: uso }
```

### Formato declarado, e documento que o alvo aceita

```yaml trecho
dados:
  cadastro:
    gerar:
      referencia: { tipo: padrao, formato: "PED-######" }   # PED-481902
      filial:     { tipo: padrao, formato: "@@-####" }      # KQ-3718
      documento:  { tipo: cpf }
      empresa:    { tipo: cnpj }
```

No formato, `#` vira digito e `@` vira letra; o resto sai literal. CPF e CNPJ saem
**com digito verificador valido** — gerar invalido faz o alvo recusar tudo com
erro de validacao, e a execucao passa a medir o caminho da recusa em vez do
trabalho.

Geradores disponiveis: `uuid`, `sequencia`, `numero(min,max)`, `inteiro(min,max)`,
`nome`, `email`, `texto(n)`, `padrao`, `cpf`, `cnpj`.

**Limitacao conhecida:** nao existe leitura de `.xlsx`. CSV cobre o caso e a
dependencia de Excel e pesada demais para o motor.

### Semente: repetivel por padrao, variavel quando voce quiser

Semente fixa no arquivo faz o CI rodar sempre o mesmo caso, e um caso que passa
mil vezes nao prova mais nada depois da primeira:

```yaml trecho
dados:
  pedidos:
    gerar: { id: uuid, valor: "numero(10,500)" }
    semente: ${SEMENTE:-42}
```

Sem a variavel, roda com 42 e nada muda. Com ela, a semente que rodou e a
variavel de onde ela veio vao para o relatorio e para o JSON:

```
  Sementes dos dados: pedidos=8817 (de $SEMENTE) (a mesma semente gera os mesmos valores de novo)
  Para repetir exatamente estes dados, rode de novo com SEMENTE=8817
```

## Mix ponderado: a proporcao entre operacoes

Repetir a mesma chamada mede cache, nao sistema. `peso` reparte a taxa declarada
entre alternativas, e cada iteracao executa **uma** delas:

```yaml trecho
cenario:
  - nome: consultar pedido
    peso: 60
    http: { metodo: GET, caminho: "/pedidos/${pedidos.id}" }
  - nome: criar pedido
    peso: 10
    http: { metodo: POST, caminho: /pedidos }
```

A escolha e por posicao no ciclo, nao sorteio: duas execucoes do mesmo arquivo
aplicam o mesmo mix, entao a diferenca entre elas e do alvo e nao do gerador. O
relatorio mostra a proporcao que de fato saiu, ao lado da declarada:

```
Mix declarado e observado
  consultar pedido             60.0% declarado     60.0% observado (300 de 500)
  consultar fatura             30.0% declarado     30.0% observado (150 de 500)
  criar pedido                 10.0% declarado     10.0% observado (50 de 500)
```

Peso escolhe qual alternativa executar, nao qual passo dentro de uma jornada: um
cenario com captura encadeada e uma jornada so, e a validacao recusa peso nele.
Exemplo completo: [`examples/mix-de-operacoes.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/mix-de-operacoes.yaml).

## Ramificacao por perfil: a coluna decide a rota

Nao existe passo condicional, e nao e falta: qual caminho cada perfil percorre, e
em que proporcao, e conhecimento de negocio — nenhuma observacao de trafego revela
isso, e quem grava uma passagem grava um perfil. A ramificacao honesta e por dado.

```yaml trecho
dados:
  clientes: { arquivo: dados/clientes.csv, consumo: circular }

cenario:
  - nome: consultar limite
    http: { metodo: GET, caminho: "/${clientes.rota}/${clientes.id}/limite" }
```

O CSV declara a proporcao pelas linhas que tem. **O que essa forma ainda nao
resolve:** os dois perfis caem numa linha so do relatorio, entao um perfil caro
aparece como cauda do passo inteiro. Exemplo:
[`examples/ramificacao-por-perfil.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/ramificacao-por-perfil.yaml).

## De onde vem cada `${variavel}`

Quatro origens, e nenhuma outra:

```yaml trecho
variaveis:
  tenant: acme                      # fixa no cenario
  regiao: "${REGIAO:-us-east-1}"    # do ambiente, com reserva

dados:
  pedidos: { gerar: { id: uuid } }  # e entao ${pedidos.id}

cenario:
  - http: GET /faturas
    captura: { faturaId: "$.itens[0].id" }   # e entao ${faturaId} nos passos seguintes
```

**Nome em CAIXA ALTA vem do ambiente sem precisar declarar:** `${API_KEY}`,
`${KAFKA_SENHA}`. E a mesma forma que o `import curl` e o `record` escrevem
sozinhos.

**E vale em qualquer campo escalar do cenario**, nao numa lista de campos
escolhidos — alvo, taxa, duracao, topico, fila, nome de passo, cabecalho, caminho
de certificado:

```yaml trecho
alvo: "${ALVO:-http://127.0.0.1:8080}"
carga:
  perfis:
    - patamar: { taxa: "${TAXA:-100}/s", durante: "${DURACAO:-1m}" }
cenario:
  - kafka: { topico: "${TOPICO:-pedidos}", valor: "{}" }
```

Dentro de `{ }` o valor precisa de aspas, porque o YAML le `{` e `}` como
estrutura. Sem reserva, o campo fica com a referencia crua e a validacao diz qual
variavel faltou:

```
erro no cenario: cenario.yaml:5:24: taxa invalida: "${TAXA}/s" (use por exemplo 50/s)
    a variavel de ambiente TAXA nao esta definida, entao este campo ficou com a referencia crua.
    rode com TAXA=... , ou declare um padrao no arquivo: ${TAXA:-valor}
```

Duas excecoes, e as duas porque o texto cru do campo faz parte do que ele
significa: **credencial** (`senha`, `token`, `chave`), onde a recusa de segredo
literal precisa ver se voce escreveu `${VARIAVEL}` ou o segredo, e **`semente`**,
cuja origem o relatorio publica para a execucao poder ser repetida.

A escolha pela caixa alta em vez de conferir o ambiente e deliberada: conferir o
ambiente tornaria `braunrate validate` impossivel numa maquina sem o segredo, que
e exatamente onde alguem confere o cenario antes de commitar.

## Gate de CI

```bash
braunrate execute cenario.yaml -quiet -result=saida.json
```

Codigo de saida: `0` passou, `1` o criterio de aceite reprovou, `2` erro no
arquivo, `3` resultado invalido. Nao e preciso ler a saida: o codigo basta.

Exemplo que roda no CI deste repositorio:
[`examples/ci.yaml`](https://github.com/Diegobraun/braunrate/blob/main/examples/ci.yaml).

**Exemplo que depende de infraestrutura declara isso**, e o laco do CI pula com
aviso em vez de quebrar em silencio:

```yaml trecho
requer: [kafka]
```

## Comparar com a execucao anterior

```bash
braunrate execute cenario.yaml -baseline=execucao-anterior.json
```

Com o alvo 12 vezes mais lento, o criterio por passo continuou aprovando e a
regressao pegou:

```
  ok    Passou: "consultar pedido" respondeu 95% em ate 61 ms, dentro do limite de 150 ms.
  FALHA Falhou: o tempo de resposta de 95% das jornadas ficou 931.0% pior que execucao-anterior.json, acima do limite de 10% pior (de 12 ms para 122 ms).
```

Quando a comparacao tem ressalva que explica a diferenca sozinha — outra maquina,
outro cenario, outra versao, outro modelo de chegada — **a regra nao reprova**, e
diz por que. Reprovar ali seria culpar o servico por uma diferenca que a propria
comparacao nao consegue atribuir a ele. Variacao abaixo de 5% continua sendo
ruido: duas execucoes nao dao intervalo de confianca.

Fora do gate, a comparacao avulsa:

```bash
braunrate compare antes.json depois.json -html comparacao.html
```

```
Ficou mais lento: jornada inteira (95%): 71 vezes mais lento — de 10 ms para 675 ms. Com 1 ressalva que pode explicar a diferenca sozinha.

Por passo
  passo                        95% antes  95% depois         variacao
  consultar pedido                8.4 ms      598 ms         71x pior
  pagar fatura                  0.601 ms       43 ms         71x pior

O que pode explicar a diferenca sem ser o servico
  - as execucoes usaram versoes diferentes do braunrate: 0.2.0 e 0.3.0 (isso sozinho explica a diferenca)
  - as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao
```

Quando uma das duas execucoes nao vale como medicao, a pagina nao mostra tabela
nenhuma: nao existe comparacao menor, existe comparacao que nao vale.

## O mesmo cenario em Go

Quando o cenario passa do que o YAML expressa — laco sobre uma lista, decisao no
meio da jornada, dado vindo de um sistema seu — o mesmo cenario se escreve em Go,
com o mesmo motor e as mesmas metricas:

```go
// Scenario is the same journey of examples/jornada-autenticada.yaml, written in
// Go: same engine, same metrics, same result document.
func Scenario(alvo string) (braunrate.Scenario, error) {
	return dsl.New("Jornada de cobranca").
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

Esse trecho nao e ilustracao: ele vive em
[`examples/cenario-em-go/cenario.go`](https://github.com/Diegobraun/braunrate/blob/main/examples/cenario-em-go/cenario.go),
o CI compila, roda contra o alvo embutido e confere o proprio criterio de aceite,
e um teste reprova o build se esta pagina derivar do arquivo.

**Migrar de YAML para Go nao e reescrever.** A DSL nao interpreta nada por conta
propria: `"$.ultimaFatura.id"`, `"> 10"` e `"< 150ms"` sao lidos pelas mesmas
funcoes que leem o YAML. Um teste compara a estrutura inteira dos dois caminhos,
caso a caso, e falha se um protocolo registrado, uma chave de topo, uma forma de
cenario ou uma opcao de protocolo ficar sem caso de equivalencia.

**Limitacao declarada:** protocolo novo continua exigindo mudanca neste
repositorio — contribuicao ou fork. Ate a v1 os tipos publicos **nao estao
congelados**: eles seguem a versao que ja os governa, entao campo novo entra sem
aviso e campo que sair ou mudar de nome sai com a versao mudando junto.
