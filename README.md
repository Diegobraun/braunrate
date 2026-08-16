# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR histogram e deteccao de back-pressure.

## Demonstracao de honestidade de medicao

O alvo de teste embutido congela por 1 s no meio da execucao. Mesma pausa, mesmo alvo, dois modelos de medicao:

| Modelo | p99 reportado | Amostras |
|---|---|---|
| **braunrate (chegada aberta, latencia contada do instante agendado)** | **976,4 ms** | 600 |
| Laco fechado (um usuario virtual em sequencia, como JMeter e Locust medem) | 3,3 ms | 793 |

**973,1 ms escondidos pelo laco fechado.** O laco fechado nao mente por bug: quando o alvo trava, ele simplesmente para de enviar, e as requisicoes que deveriam ter partido nunca entram na conta. E a omissao coordenada.

Isso nao e alegacao de marketing: e um teste automatizado que roda no CI a cada push. Se a medicao mentir, o build quebra.

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestMeasurementReflectsTargetFreeze
    modelo aberto: p50 2.7 ms | p99 976.9 ms | max 1005.6 ms | n 600
--- PASS: TestMeasurementReflectsTargetFreeze (3.01s)
=== RUN   TestTargetFreezeIsNotConfusedWithGeneratorSaturation
    aviso correto: a latencia do alvo cresceu ao longo da execucao enquanto o despacho
    continuou pontual; a degradacao e do alvo, nao do gerador | p99 por segundo passou
    de 3.2 ms para 996.9 ms
--- PASS: TestTargetFreezeIsNotConfusedWithGeneratorSaturation (3.01s)
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 976.4 ms sobre 600 amostras
      laco fechado:              p99 3.3 ms sobre 793 amostras
      omissao coordenada: 973.1 ms escondidos pelo laco fechado
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

Reproduza na sua maquina: `go test ./internal/selfcheck/... -v`.

### Tres provas, tres pontos cegos

Esta e a primeira das tres execucoes reais que sustentam a tese. Cada uma expoe um ponto cego diferente, e nenhuma delas e opiniao:

| Prova | Numero | Ponto cego que expoe |
|---|---|---|
| Alvo congelado por 1 s (acima) | 976,4 ms contra 3,3 ms | **Omissao coordenada**: laco fechado para de enviar quando o alvo trava, e a espera some da conta |
| [GraphQL com erro em 200](#graphql) | 406 erros em 2.844 respostas, todas com status 200 | **Erro classificado por status**: quem le o codigo HTTP reporta 0% de erro e SLO verde |
| [Cadeia assincrona](#mensageria-e-cadeia-assincrona) | 1,2 ms para produzir contra 3,96 s de jornada | **Medir so a producao**: o broker aceita rapido, e o efeito que o usuario espera chega segundos depois |

## Estado

**Fase 8 concluida** — motor de chegada aberta, HTTP, GraphQL, Kafka, RabbitMQ e passo `aguardar`, correlacao, autenticacao, dados, assercoes, SLO com codigo de saida, ferramentas de autoria (schema no editor, `debug`, `import curl`, `import jmx` e `record`), relatorio (HTML autocontido, JSON, CSV, comparacao entre execucoes), variedade observada, **cenario em Go equivalente ao YAML travado por teste**, modelo fechado declarado, **autenticacao de broker com a credencial fora do arquivo** e **modo servidor local sem logica propria**.

Decisao da Fase 0: **Go**, sustentada por dois criterios apenas — RSS sob carga (30 MB contra 597 MB do Java com G1, a 10.000/s) e binario unico estatico, que para o publico de QA significa instalar baixando um arquivo. Startup, precisao de agendamento e modo de falha apareceram na primeira analise com peso que nao aguentam, e estao marcados como nao-criterio no ADR. Numeros, metodologia e limites em [medicoes-fase0.md](docs/medicoes-fase0.md); a decisao com os pesos de cada criterio em [ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Como usar

```bash
go build -o braunrate ./cmd/braunrate

braunrate target -latency=5ms &                # alvo de teste embutido
braunrate validate examples/http-basico.yaml   # valida sem executar
braunrate debug examples/http-basico.yaml      # uma iteracao, tudo visivel
braunrate execute examples/http-basico.yaml    # executa e resume no terminal
braunrate execute examples/http-basico.yaml -html=relatorio.html -result=saida.json
braunrate compare antes.json depois.json       # o que mudou entre duas execucoes
braunrate record -output cenario.yaml          # grava navegando por um proxy local
braunrate serve -dir ./cenarios                # os mesmos comandos por HTTP, local
```

Cenario minimo:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
nome: Consulta de pedidos
alvo: http://127.0.0.1:8080

carga:
  modelo: aberto
  perfis:
    - rampa: { de: 100/s, ate: 800/s, durante: 5s }
    - patamar: { taxa: 800/s, durante: 10s }
    - pico: { taxa: 2000/s, durante: 3s }

cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }
```

Codigo de saida: `0` passou, `1` **falhou o SLO**, `2` erro de cenario, `3` **resultado invalido** — a execucao nao mediu o que se propos a medir, entao nao ha o que aprovar ou reprovar ([como isso e verificado](#antes-de-qualquer-veredito-este-resultado-quer-dizer-alguma-coisa)).

Cenario com autenticacao, correlacao, dados e SLO — o exemplo completo esta em [`examples/jornada-autenticada.yaml`](examples/jornada-autenticada.yaml):

```yaml
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: "${usuario}", senha: "${SENHA}" } }
    captura: { token: $.access_token }
  renovar_apos: 25m

dados:
  assinantes: { arquivo: dados/assinantes.csv, consumo: circular }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    verificar: { status: 200, json: { $.ultimaFatura.status: ABERTA } }
    captura: { faturaId: $.ultimaFatura.id }

slo:
  - consultar pedido: { p95: < 150ms }
  - jornada: { p95: < 2s, p99: < 5s }
  - global: { erros: < 0.1 }
```

Saida real dessa execucao:

```
Jornada de cobranca — contra http://127.0.0.1:8080

Passou: as 5 regras de SLO foram atendidas.

O que aconteceu
  4.750 requisicoes em 10s, 475 por segundo, 0% de erro
  Metade das respostas em ate 5.5 ms; 95% em ate 6.2 ms; 99% em ate 6.8 ms; a pior levou 14 ms

A jornada inteira
  Todas as 2375 jornadas chegaram ao fim; metade levou ate 11 ms e 95% ate 12 ms, contados do instante em que deveriam ter comecado.
  metade 11 ms | 95% 12 ms | 99% 13 ms | pior 20 ms

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)      2.375    5.5 ms    6.2 ms    6.9 ms     11 ms     14 ms       0
  pagar fatura               (2)      2.375    5.3 ms    6.2 ms    6.7 ms    7.5 ms     10 ms       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.
  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como
      esse passo depende do valor capturado antes dele, nao existe instante
      agendado proprio. Para a leitura honesta da jornada, use "A jornada inteira".

SLO
  ok    Passou: "consultar pedido" teve latencia p95 de 6 ms, dentro do limite de 150 ms.
  ok    Passou: "pagar fatura" teve latencia p95 de 6 ms, dentro do limite de 200 ms.
  ok    Passou: a jornada inteira teve latencia p95 de 12 ms, dentro do limite de 2000 ms.
  ok    Passou: a jornada inteira teve latencia p99 de 13 ms, dentro do limite de 5000 ms.
  ok    Passou: o cenario inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.
  --    regressao: sem criterio declarado — o gate aprova sem comparar com a execucao anterior

Confiabilidade da medicao
  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.
  Atraso tipico para disparar: 0.001 ms; pior caso: 1.1 ms (o tempo de resposta ja desconta isso)

Ambiente
  Mac darwin/arm64, 10 nucleos | braunrate 0.4.0 | 2026-08-16 03:03:37
  4 valores distintos de assinantes.id em 2.375 usos
  4 valores distintos de faturaId em 2.375 usos
  1 unico valor de token em 2.375 usos
  Autenticacao obtida 1 vez(es) e reaproveitada por todas as jornadas.
  Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.
```

## Escrever um cenario

**Autocompletar no editor.** Todo exemplo comeca com esta linha, e ela vale para o seu arquivo tambem:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
```

Com a extensao YAML do VS Code (ou qualquer editor com yaml-language-server), o editor passa a completar as chaves, mostrar a explicacao de cada uma e marcar erro antes de rodar. O [schema](docs/braunrate.schema.json) tem teste que quebra o build se ele oferecer chave que o parser recusa, ou esquecer chave que o parser aceita.

**Comecar de um curl** em vez de comecar do zero:

```bash
braunrate import curl "curl 'https://api.exemplo.com/v1/pedidos/9912' -X POST -H 'Authorization: Bearer abc.def' -d '{\"valor\": 199.90}'" -output cenario.yaml
```

Sai um cenario que ja carrega, com carga e SLO de partida, e tres avisos honestos no terminal: o token virou variavel (`${token}`, lida de `TOKEN` no ambiente) e nao vai para o repositorio; o id fixo no caminho faz o alvo responder de cache; os numeros de carga e SLO sao chute, nao medicao.

**Comecar de um plano do JMeter**, para quem tem suite pronta:

```bash
braunrate import jmx plano.jmx -output cenario.yaml
```

**A traducao e parcial e o que ficou de fora sai listado no terminal**, um elemento por vez, porque importador que engole o arquivo inteiro em silencio entrega um cenario que mede outra coisa:

| Traduzido | Nao traduzido (sai declarado) |
|---|---|
| `HTTPSamplerProxy` (metodo, caminho, dominio, corpo) | Controladores (If, While, Loop), temporizadores |
| `HeaderManager`, com credencial virando variavel de ambiente | Scripts JSR223/BeanShell |
| `CSVDataSet` (arquivo e reciclagem) | Samplers de JDBC, JMS e outros nao-HTTP |
| `ThreadGroup`, como **aviso**, nunca como taxa | Assercoes: todo passo sai com `status: 200` |
| `JSONPostProcessor` e `RegexExtractor`, como instrucao de captura | Funcoes `${__...}` do JMeter |

**Thread nunca vira taxa.** No JMeter uma thread so envia depois que a resposta anterior chegou: 50 threads sao 50/s se o alvo responde em 1 s e 5/s se responde em 10 s. Converter em silencio importaria a omissao coordenada junto com o plano, entao o bloco `carga` sai como chute declarado e o aviso diz o que o `.jmx` tinha:

```
atencao: o grupo "Usuarios" declara 50 threads, rampa de 30s, 300s de duracao: numero de
thread nao vira taxa de chegada, porque thread so envia depois da resposta anterior. O bloco
'carga' ficou com um chute; troque pela taxa que voce quer sustentar (requisicoes por segundo)
atencao: o .jmx captura "faturaId" de "$.ultimaFatura.id": declare no passo que produz o valor,
como captura: { faturaId: $.ultimaFatura.id }
atencao: 1 elemento(s) do .jmx nao foram traduzidos e ficaram de fora do cenario:
BeanShellPreProcessor (1). Confira se algum deles mudava o que era medido
```

**Gravar navegando**, para quem nao tem nem curl nem plano:

```bash
braunrate record -output cenario.yaml
# aponte o navegador ou o curl para o proxy, navegue pelo fluxo, Ctrl+C
```

O recorder do JMeter transcreve: grava o token daquela sessao e o pedido `9912`, e na segunda execucao o cenario quebra. Este faz quatro coisas a mais, e **declara cada uma**:

```
descartei 1 dominio de fora (example.com)
descartei 1 recurso estatico
3 requisicoes viraram 2 passo(s) em cenario.yaml
2 valor(es) observado(s) de pedidos_id em cenario-pedidos-id.csv
atencao: o campo "senha" do corpo virou ${senha}: rode com SENHA=... no ambiente, para nao versionar credencial
atencao: a sequencia gravada e uma passagem so: o mix de producao tem outras proporcoes entre as rotas
atencao: os numeros de carga e de slo sao um chute de partida, nao uma medicao: ajuste antes de usar como gate

Proximo passo, antes de qualquer carga:
  braunrate debug cenario.yaml
```

E o cenario que sai roda sem edicao — o token virou captura, o id virou dado:

```yaml
  - nome: post auth token
    http:
      metodo: POST
      caminho: /auth/token
      corpo: '{"usuario":"ana","senha": "${senha}"}'
    captura:
      access_token: $.access_token   # sugestao do gravador: confira se e mesmo este valor que a proxima chamada precisa
    verificar:
      status: 200
  - nome: get pedidos
    http:
      metodo: GET
      caminho: /pedidos/${pedidos_id.valor}
      cabecalhos:
        Authorization: "Bearer ${access_token}"
    verificar:
      status: 200
```

**Fora de escopo, com o motivo** ([ADR 0013](docs/adr/0013-gravador-de-trafego.md)): gravar dentro de **HTTPS** exige o braunrate emitir certificado e a sua maquina confiar nele — mexer no armazem de confianca do sistema nao e coisa que ferramenta de carga deve automatizar em silencio. A conexao e encaminhada para o cliente continuar funcionando, e o que nao foi gravado aparece na tela por host. **Trafego de aplicativo movel** fica fora da v1, por causa de pinning de certificado e de configuracao por sistema operacional.

**Ver a iteracao antes da carga**, que e onde a correlacao quebrada aparece:

```
$ braunrate debug examples/jornada-autenticada.yaml
depurando "Jornada de cobranca" contra http://127.0.0.1:8080: 1 usuario, 1 iteracao, sem carga

passo 1 — consultar pedido   [ok em 3.4ms]
  requisicao: GET /pedidos/1001
              Authorization: Bearer token-… (14 caracteres)
  resposta:   status 200, 95 bytes
  corpo:      {"id":"1001","status":"ABERTO","ultimaFatura":{"id":"f-1001","valor":199.90,"status":"ABERTA"}}
  capturou:
    faturaId = f-1001

passo 2 — pagar fatura   [ok em 3.7ms]
  requisicao: POST /faturas/f-1001/pagar
              Authorization: Bearer token-… (14 caracteres)
              Content-Type: application/json
              corpo: {"valor":199.9}
  resposta:   status 200, 63 bytes

variaveis no fim da iteracao
  assinantes.id = 1001

Iteracao completa: 2 passo(s), tudo certo. Para rodar com carga:
  braunrate execute examples/jornada-autenticada.yaml
```

## O mesmo cenario em Go

Quando o cenario passa do que o YAML expressa — laco sobre uma lista, decisao no meio da jornada, dado vindo de um sistema seu — o mesmo cenario se escreve em Go, com o mesmo motor e as mesmas metricas:

```go
c, err := dsl.Novo("Jornada autenticada").
	Alvo("https://api.exemplo.com").
	Autenticacao(dsl.PorToken(
		dsl.POST("/auth/token").Corpo(map[string]any{"usuario": "ana", "senha": "${SENHA}"}),
		dsl.Capturar("token", "$.access_token"),
	).RenovarApos(25 * time.Minute)).
	DadosDeArquivo("assinantes", "dados/assinantes.csv").
	Rampa(dsl.PorSegundo(10), dsl.PorSegundo(100), 30*time.Second).
	Patamar(dsl.PorSegundo(100), time.Minute).
	Passo(dsl.GET("/pedidos/${assinantes.id}"),
		dsl.Nome("consultar pedido"),
		dsl.VerificarStatus(200),
		dsl.Capturar("faturaId", "$.ultimaFatura.id")).
	SLO("consultar pedido", "p95", "< 150ms").
	SLOGlobal("erros", "< 0.1").
	Construir()

m, err := motor.Novo(c, motor.OpcoesPadrao())
documento := m.Executar(context.Background())
```

**Migrar de YAML para Go nao e reescrever.** A DSL nao interpreta nada por conta propria: `"$.ultimaFatura.id"`, `"> 10"` e `"< 150ms"` sao lidos pelas mesmas funcoes que leem o YAML, e cada protocolo aplica seus padroes num lugar so. Um teste compara a estrutura inteira dos dois caminhos, caso a caso, e falha se um protocolo registrado, uma chave de topo ou uma forma de cenario ficar sem caso de equivalencia:

```
$ go test ./dsl/ -run TestYAMLEDSL -v
--- PASS: TestYAMLEDSLProduzemOMesmoCenario (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/http_com_variaveis,_dados,_autenticacao_por_token,_capturas_e_slo (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/graphql_por_operacao (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/kafka_com_aguardar_fechando_a_cadeia (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/amqp_em_fila_e_em_troca_com_rota (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/autenticacao_basica_e_consumo_unico_por_usuario (0.00s)
    --- PASS: TestYAMLEDSLProduzemOMesmoCenario/autenticacao_por_cabecalho_fixo (0.00s)
```

## Dados: um valor por jornada, nao por requisicao

O caso mais comum e a chave de idempotencia: **o mesmo `transactionId` nas duas requisicoes da mesma jornada, um novo na jornada seguinte**. Esse e o padrao, sem declarar nada:

```yaml
dados:
  pagamento:
    gerar: { transactionId: uuid }

cenario:
  - http: { metodo: POST, caminho: /pedidos, cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
  - http: { metodo: GET, caminho: "/pedidos/${pedidoId}", cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
```

Se o valor precisar ser novo **a cada uso**, isso e declarado:

```yaml
gerar:
  nonce: { tipo: uuid, novo_a_cada: uso }
```

Os dois lados sao cobertos por teste: `TestGeneratedValueIsStableWithinTheIterationAndNewInTheNext` reprova se a mesma jornada usar dois valores ou se duas jornadas usarem o mesmo, e `TestNewPerUseIsExplicitAndChangesAtEveryOccurrence` reprova se o `novo_a_cada: uso` nao renovar.

### De onde vem cada `${variavel}`

Quatro origens, e nenhuma outra:

```yaml
variaveis:
  tenant: acme                      # fixa no cenario
  regiao: "${REGIAO:-us-east-1}"    # do ambiente, com reserva

dados:
  pedidos: { gerar: { id: uuid } }  # e entao ${pedidos.id}

cenario:
  - http: GET /faturas
    captura: { faturaId: $.itens[0].id }   # e entao ${faturaId} nos passos seguintes
```

**Nome em CAIXA ALTA vem do ambiente sem precisar declarar:** `${API_KEY}`, `${KAFKA_SENHA}`. E a mesma forma que o `import curl` e o `record` escrevem sozinhos.

Referencia que nao cai em nenhum desses casos **reprova a validacao**, apontando a coluna da referencia — nao o inicio da linha:

```
erro no cenario: cenario.yaml:14:26: nao sei de onde vem ${faturald}.
    voce quis dizer "faturaId"?
    disponiveis: faturaId, tenant
```

Antes ela virava texto vazio em silencio: a requisicao saia com o campo em branco, o alvo respondia 401 ou 404, e nada na saida ligava uma coisa a outra. O caso esta na [auditoria de friccao](docs/auditoria-fricao.md) como A8.

A escolha pela caixa alta em vez de conferir o ambiente e deliberada: conferir o ambiente tornaria `braunrate validate` impossivel numa maquina sem o segredo, que e exatamente onde alguem confere o cenario antes de commitar.

### Formato declarado, e documento que o alvo aceita

```yaml
gerar:
  referencia: { tipo: padrao, formato: "PED-######" }   # PED-481902
  filial:     { tipo: padrao, formato: "@@-####" }      # KQ-3718
  documento:  { tipo: cpf }
  empresa:    { tipo: cnpj }
```

No formato, `#` vira digito e `@` vira letra; o resto sai literal. CPF e CNPJ saem **com digito verificador valido** — gerar invalido faz o alvo recusar tudo com erro de validacao, e a execucao passa a medir o caminho da recusa em vez do trabalho. Coberto por teste que recalcula os dois digitos de 200 documentos de cada tipo.

Geradores disponiveis: `uuid`, `sequencia`, `numero(min,max)`, `inteiro(min,max)`, `nome`, `email`, `texto(n)`, `padrao`, `cpf`, `cnpj`.

**Exemplo que depende de infraestrutura declara isso**, e o laco do CI pula com aviso em vez de quebrar em silencio:

```yaml
requer: [kafka]
```

**Limitacao conhecida:** nao existe leitura de `.xlsx`. CSV cobre o caso e a dependencia de Excel e pesada demais para o motor. Se aparecer necessidade, sera um `braunrate import planilha` que converte para CSV — nunca leitura direta durante a execucao.

## O que serve de criterio

Um gate feito so de regra por passo aprova cada pedaco e nao diz nada sobre a espera que o usuario sente, que e a soma deles. Por isso o bloco `slo` tem quatro escopos, e o relatorio mostra tambem **o que nao foi declarado**:

```yaml
slo:
  - consultar pedido: { p95: < 150ms }              # um passo
  - jornada: { p95: < 2s, p99: < 5s }               # a espera inteira, ponta a ponta
  - global: { sucesso: ">= 99.9", taxa_efetiva: ">= 90/s" }
  - regressao: { jornada_p95: "<= 10% pior" }       # contra uma execucao anterior
```

Saida real dessa declaracao, contra o alvo embutido:

```
SLO
  ok    Passou: "consultar pedido" teve latencia p95 de 6 ms, dentro do limite de 150 ms.
  ok    Passou: a jornada inteira teve latencia p95 de 12 ms, dentro do limite de 2000 ms.
  ok    Passou: a jornada inteira teve latencia p99 de 13 ms, dentro do limite de 5000 ms.
  ok    Passou: o cenario inteiro teve taxa de sucesso de 100.00%, no minimo de 99.90%.
  ok    Passou: o cenario inteiro teve taxa efetiva de 200/s, no minimo de 90/s.
  --    regressao: sem criterio declarado — o gate aprova sem comparar com a execucao anterior
```

`sucesso: >= 99.9` e `erros: < 0.1` sao a mesma regra lida de dois lados, e cada uma aparece no relatorio do jeito que foi declarada.

**Sem criterio de jornada, o `validate` avisa** — cenario de mais de um passo cujo gate mede so as partes:

```
$ braunrate validate cenario.yaml
Cenario valido: "Jornada com criterios novos", 2 passo(s), 500 iteracoes em 5s.
Atencao: o gate mede 2 passos isolados e deixa de fora a jornada inteira, que e o tempo que o usuario espera.
    declare tambem:  - jornada: { p95: < 2s, p99: < 5s }
```

Quando nao ha criterio nenhum, o relatorio diz isso em vez de calar:

```
SLO
  --    nenhum criterio declarado — o cenario roda e reporta, mas nao serve de gate
```

**Taxa efetiva abaixo do alvo tem duas causas opostas**: o alvo nao aguentou, ou o gerador nao produziu. A segunda e medicao invalida e sai com codigo 3 antes de qualquer SLO ser lido — nunca vira falha de servico.

### Comparar com a execucao anterior como gate

```bash
braunrate execute cenario.yaml -baseline=execucao-anterior.json
```

Com o alvo 12 vezes mais lento, o criterio por passo continuou aprovando e a regressao pegou:

```
  ok    Passou: "consultar pedido" teve latencia p95 de 61 ms, dentro do limite de 150 ms.
  FALHA Falhou: a jornada inteira (p95) ficou 931.0% pior que a base, acima do limite de 10% pior (de 12 ms para 122 ms).
```

Quando a comparacao tem ressalva que explica a diferenca sozinha — outra maquina, outro cenario, outra versao, outro modelo de chegada — **a regra nao reprova**, e diz por que:

```
  ?     Sem veredito: a jornada inteira (p95) esta 931.0% pior que a base, mas a comparacao com
        base-outra-maquina.json nao e confiavel (as maquinas geradoras sao diferentes: ...;
        as execucoes usaram versoes diferentes do braunrate: 0.3.0 e 0.4.0), entao a regra
        "jornada_p95: <= 10% pior" nao reprova.
```

Reprovar ali seria culpar o servico por uma diferenca que a propria comparacao nao consegue atribuir a ele. Variacao abaixo de 5% continua sendo ruido: duas execucoes nao dao intervalo de confianca.

Nada disso e obrigatorio. **Cenario sem bloco `slo` continua executando e reportando** — so nao serve de gate.

## O relatorio

```bash
braunrate execute examples/jornada-autenticada.yaml -html=relatorio.html -result=saida.json -csv=passos.csv
braunrate report saida.json -html=relatorio.html     # gera depois, a partir do resultado gravado
braunrate compare ontem.json hoje.json                 # o que mudou entre duas execucoes
```

O topo do HTML e uma frase, nao uma tabela. [Exemplo real](docs/exemplo-relatorio.html) (baixe e abra: o GitHub nao renderiza HTML), gerado da execucao abaixo. O arquivo nao busca script, fonte nem imagem: abre em rede fechada e sobrevive anexado em ticket.

## Como ler o numero (e onde ele e otimista)

Duas coisas mudam a leitura do relatorio, e as duas estao impressas na saida em vez de escondidas na documentacao.

**Latencia do passo 2 em diante nao e corrigida.** So o primeiro passo tem instante agendado proprio; os seguintes dependem de um valor capturado antes deles e por isso comecam quando o passo anterior termina. Esta execucao real, contra o alvo embutido congelado por 1 s no meio, mostra o tamanho do problema:

```
Falhou: "consultar pedido" teve latencia p95 de 598 ms, acima do limite de 150 ms.

A jornada inteira
  Todas as 2375 jornadas chegaram ao fim; metade levou ate 81 ms e 95% ate 675 ms, contados do instante em que deveriam ter comecado.
  metade 81 ms | 95% 675 ms | 99% 1.03 s | pior 1.09 s

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  consultar pedido           (1)      2.375     40 ms    598 ms    954 ms    1.03 s    1.04 s       0
  pagar fatura               (2)      2.375     40 ms     43 ms     43 ms    1.04 s    1.04 s       0

  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui
      qualquer atraso e por isso nao esconde travada do alvo.
  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como
      esse passo depende do valor capturado antes dele, nao existe instante
      agendado proprio. Para a leitura honesta da jornada, use "A jornada inteira".

Confiabilidade da medicao
  Atencao: a latencia do alvo cresceu ao longo da execucao enquanto o despacho continuou pontual; a degradacao e do alvo, nao do gerador
            p99 por segundo passou de 64.1 ms para 1039.4 ms
```

Repare no `pagar fatura`: **43 ms no 95%**, com o alvo congelado por um segundo inteiro no meio da execucao. Sozinho, esse numero e o mesmo tipo de mentira que uma ferramenta de laco fechado produz. A jornada inteira, contada do instante agendado, mostra **675 ms** — e essa e a leitura que vale para quem usa o sistema.

**Um token para a execucao inteira.** Hoje o motor faz login uma vez e reaproveita a credencial em todas as jornadas — isso nao existe em producao. Se o alvo tiver cache por identidade, rate limit por token ou sharding por usuario, o numero fica otimista (ou, no caso do rate limit, falha por 429 que nao aconteceria). O relatorio declara isso em toda execucao com autenticacao. `pool de tokens` e `token por usuario virtual` sao evolucao prevista, com a forma do YAML ja desenhada no [ADR 0005](docs/adr/0005-identidade-e-token.md).

## Modelo fechado: quando serve e quando mente

O padrao e o modelo aberto: a carga e uma taxa declarada e o gerador insiste nela mesmo quando o alvo demora. O modelo fechado e o outro jeito de descrever carga — N usuarios em laco, cada um so pedindo de novo depois da resposta anterior — e existe declarado:

```yaml
carga:
  modelo: fechado
  usuarios: 200
  duracao: 5m
  intervalo_entre_iteracoes: 1s
```

**Serve** quando o limite real e de sessao, nao de chegada: pool de conexao, licenca por assento, fila com numero fixo de trabalhadores, ou quando voce esta reproduzindo um cenario de JMeter escrito em threads e quer o mesmo formato antes de converter.

**Mente** quando a pergunta e "o alvo aguenta X por segundo?". No laco fechado o alvo decide a carga: se ele travar, os usuarios param de pedir junto, e o atraso nunca entra na conta.

### A mesma travada, medida dos dois jeitos

Mesmo alvo, congelado por 3 s no meio de uma execucao de 12 s. A esquerda, 100/s em modelo aberto; a direita, 10 usuarios em laco fechado com 100 ms entre iteracoes (~95/s enquanto o alvo responde).

```
# braunrate execute aberto.yaml           # braunrate execute fechado.yaml
1.200 requisicoes, 100 por segundo        850 requisicoes, 70 por segundo
metade 6.1 ms | 95% 2.41 s | pior 3.01 s  metade 6.4 ms | 95% 7.0 ms | pior 3.00 s
```

O 95% caiu de **2,41 s para 7,0 ms** — mesma travada, mesmo alvo. O laco fechado nao errou conta nenhuma: ele mediu com precisao um evento que ele mesmo deixou de provocar, porque os 10 usuarios ficaram parados esperando em vez de continuar chegando.

Repare tambem na taxa: **100/s de um lado, 70/s do outro**. A carga do modelo fechado caiu junto com o alvo. Num teste de capacidade, e a carga que deveria ter continuado.

Por isso o relatorio do modelo fechado abre com o aviso, sempre, mesmo quando tudo passa:

```
ATENCAO: Este teste usou 10 usuarios em laco fechado. Se o alvo travar, os usuarios
param de pedir e o atraso nao aparece nos numeros. O tempo de resposta abaixo pode
estar melhor do que o usuario real sente.
```

E o documento JSON **nao tem** campo `latencia_corrigida`: sem instante agendado nao ha o que corrigir, e escrever zero ali seria afirmar que nao havia atraso escondido. O `braunrate validate` avisa antes de rodar, com a taxa aproximada que aquele cenario produziria; e `braunrate compare` recusa comparar uma execucao aberta com uma fechada. Detalhes no [ADR 0012](docs/adr/0012-modelo-fechado-como-opcao-declarada.md).

## GraphQL

Cole a consulta; o nome da operacao vira a linha do relatorio:

```yaml
cenario:
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) {
          pedido(id: $id) { id status ultimaFatura { id status } }
        }
      variaveis: { id: "${assinantes.id}" }
    verificar:
      json: { $.data.pedido.status: ABERTO }
    captura:
      faturaId: $.data.pedido.ultimaFatura.id

slo:
  - graphql ConsultarPedido: { p95: < 150ms }
```

Duas coisas que uma ferramenta de HTTP generico erra em GraphQL, e que aqui sao o padrao:

**A operacao e a unidade de medida.** Tudo em GraphQL chega em `POST /graphql`. Agregar por URL colocaria a consulta mais barata e a mutation mais cara na mesma linha, e o p99 da mais cara sumiria na media. Por isso a chave e `graphql ConsultarPedido`, e operacao anonima e recusada na leitura do cenario — com a mensagem mostrando como dar nome.

**Erro com status 200 e erro.** A especificacao manda responder `200` com `errors` no corpo. Execucao real contra o alvo embutido, onde um quarto dos assinantes nao existe:

```
Falhou: o cenario inteiro teve taxa de erro de 14.28%, acima do limite de 0.10%.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  graphql ConsultarPedido    (1)      1.625    4.7 ms    5.1 ms    5.4 ms    5.8 ms     14 ms     406
  graphql PagarFatura        (2)      1.219    4.7 ms    5.0 ms    5.2 ms    5.8 ms    6.0 ms       0

Erros
  erro no corpo da resposta GraphQL (com status 200)  406
```

**Todas as 2.844 respostas vieram com status HTTP 200.** Uma ferramenta que classifica por status teria reportado 0% de erro e SLO verde. Resposta parcial (`data` e `errors` juntos) tambem conta como erro, e o detalhe diz que foi parcial. Detalhes em [ADR 0006](docs/adr/0006-graphql-como-unidade-de-medida.md).

## Mensageria e cadeia assincrona

Produzir mede o broker aceitando a mensagem. O que o usuario sente e a cadeia inteira — e para isso existe o passo `aguardar`:

```yaml
cenario:
  - kafka:
      topico: pedidos
      chave: "${pedidos.id}"          # chave fixa concentra tudo numa particao
      valor: { pedido: "${pedidos.id}", valor: "${pedidos.valor}" }

  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${pedidos.id}"          # espera a mensagem desta iteracao, nao qualquer uma
      timeout: 10s
```

Execucao real contra Kafka com o consumidor **deliberadamente sobrecarregado** — 15 ms por mensagem, ou seja 66/s de capacidade, contra uma carga de 100/s:

```bash
braunrate target -kafka=127.0.0.1:9092 -input=pedidos -output=pedidos-processados -processor-delay=15ms &
braunrate execute cadeia-100-por-segundo.yaml
```

```
A jornada inteira
  Todas as 800 jornadas chegaram ao fim; metade levou ate 1490 ms e 95% ate 3957 ms, contados do instante em que deveriam ter comecado.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  aguardar pedidos-lento-pr… (2)        800    1.49 s    3.95 s    4.17 s    4.23 s    4.24 s       0
  kafka produzir pedidos-le… (1)        800    1.2 ms    2.2 ms    3.9 ms    179 ms    228 ms       0
```

**Produzir custa 1,2 ms; a cadeia custa 3,96 s no 95%.** Uma ferramenta que so mede a producao teria reportado milissegundo e aprovado o sistema. O consumidor nao acompanha 100/s, a fila cresce, e so a medicao ponta a ponta mostra isso.

O exemplo que acompanha o repositorio, [`examples/cadeia-assincrona.yaml`](examples/cadeia-assincrona.yaml), roda a 40/s — dentro da capacidade do processador embutido — porque exemplo publicado tem que passar quando alguem copia. A sobrecarga acima e cenario proprio, feito para mostrar o ponto cego.

Detalhes das decisoes: [ADR 0008](docs/adr/0008-mensageria-e-cadeia-assincrona.md). Publicacao com confirmacao e o padrao (`acks: todos` no Kafka, publisher confirms no AMQP), sem lote — agrupar mensagens mediria o lote, nao a mensagem.

### Apontando para um broker real

Broker de homologacao nao aceita conexao anonima, e **credencial nunca vai para o arquivo** — so nome de variavel de ambiente ou a cadeia padrao da nuvem. O cenario vai para o repositorio; o repositorio guarda para sempre.

**Kafka com SASL/SCRAM sobre TLS** — o caso mais comum:

```yaml
mensageria:
  kafka:
    brokers: [kafka.homolog:9093]
    autenticacao: { tipo: scram_sha512, usuario: "${KAFKA_USUARIO}", senha: "${KAFKA_SENHA}" }
    tls: { ca: /etc/ssl/homolog/ca.pem }
```

```bash
KAFKA_USUARIO=ana KAFKA_SENHA=... braunrate validate homolog.yaml
```

```
Cenario valido: "Pedidos em homologacao", 1 passo(s), 6000 iteracoes em 2m0s.
Sem slo declarado: a execucao nunca vai falhar por lentidao. Adicione um bloco 'slo' para virar gate de CI.
Mensageria: kafka em kafka.homolog:9093: scram_sha512, usuario ana + TLS com CA propria
```

Tipos aceitos: `sasl_plain`, `scram_sha256`, `scram_sha512`, `msk_iam` e `certificado` (mTLS, com `tls: { certificado: ..., chave: ... }`). `tls: true` liga TLS com as autoridades do sistema; `tls: { ca: ... }` usa uma autoridade interna.

**AWS MSK com IAM** — nao ha campo de chave, e nao vai haver:

```yaml
mensageria:
  kafka:
    brokers: [b-1.msk.exemplo:9098, b-2.msk.exemplo:9098]
    autenticacao: { tipo: msk_iam, regiao: us-east-1 }
```

```
Mensageria: kafka em b-1.msk.exemplo:9098, b-2.msk.exemplo:9098: msk_iam (regiao us-east-1, credencial da cadeia padrao da AWS) + TLS
```

A assinatura vem da cadeia padrao da AWS — `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, `AWS_PROFILE`, ou a role da maquina. TLS e ligado sozinho: a porta 9098 nao aceita outra coisa.

**RabbitMQ com usuario e senha:**

```yaml
mensageria:
  amqp:
    autenticacao: { tipo: sasl_plain, usuario: "${RABBIT_USUARIO}", senha: "${RABBIT_SENHA}" }
    tls: { ca: /etc/ssl/homolog/ca.pem }
```

```
Mensageria: amqp em endereco do alvo: sasl_plain, usuario ana + TLS com CA propria
```

Senha escrita no arquivo **reprova a validacao**, e a mensagem ensina a saida:

```
erro no cenario: homolog.yaml:7:77: senha literal no cenario: credencial nunca vai para o arquivo, porque o arquivo vai para o repositorio.
    troque por:  senha: ${BROKER_SENHA}
    e rode com:  BROKER_SENHA=... braunrate execute cenario.yaml
    valor de reserva (${VAR:-algo}) tambem nao serve: a reserva seria o segredo escrito no arquivo
```

Terminal, HTML, JSON e depuracao mostram tipo de autenticacao e usuario, nunca o segredo. Senha errada vira erro de classe `autenticacao` e falta de permissao vira `autorizacao` — nenhuma das duas vira "broker indisponivel", que mandaria olhar o firewall. O aperto de mao de TLS e SASL e pago na preparacao, antes do relogio comecar: se entrasse na medicao, a primeira mensagem carregaria o aperto de mao inteiro.

**O que o CI exercita:** um Kafka com SCRAM-SHA-512 sobre TLS com CA propria ([`.github/broker-autenticado.sh`](.github/broker-autenticado.sh)). **O caminho completo do MSK com IAM nao roda no CI** — exigiria uma conta AWS com cluster de verdade. Ha teste de unidade cobrindo a assinatura na regiao declarada e a ausencia de qualquer pedido de chave, e so.

**Fora por enquanto:** OAUTHBEARER (depende de provedor de identidade, e o caminho muda por provedor). **Fora, com motivo:** servico gerenciado de nuvem — SQS, SNS, Kinesis, EventBridge, Service Bus, Pub/Sub — nao e broker apontavel, e sim SDK com semantica propria de entrega e cobranca; entraria como protocolo novo, nao como autenticacao. GoldenGate tambem esta fora: e replicacao de banco por protocolo proprietario, nao QA de aplicacao. Decisoes em [ADR 0014](docs/adr/0014-autenticacao-de-mensageria.md).

### Quanto o gerador aguenta produzindo (medido)

Sem lote, uma mensagem por chegada agendada e com confirmacao, a vazao maxima e menor que a de ferramentas que agrupam. O numero, medido:

| Topico | Ultima taxa valida | p50 / p95 da producao | Primeira taxa invalida |
|---|---|---|---|
| 6 particoes | **15.000 msg/s** | 1,4 ms / 41 ms | 18.000/s (4.884 descartadas) |
| 1 particao | **5.000 msg/s** | 0,2 ms / 56 ms | 8.000/s (8.059 descartadas) |

Em 15.000/s o desvio de agendamento ficou em 0,001 ms tipico e 0,56 ms no pior caso, com pico de 861 mensagens em voo: **quem saturou primeiro nao foi o escalonador do braunrate, foi o caminho de entrega confirmada contra o broker.** Por isso o numero da tabela e o teto do par gerador+broker nesta maquina, e nao um teto puro do gerador.

**Ambiente:** Apple M2 Pro, 10 nucleos, Redpanda v24.2.7 em loopback no mesmo host, 10 s por execucao, um passo de producao por iteracao. Broker remoto, replicacao real e mensagem maior mudam o numero. Meça no seu ambiente antes de citar este.

Se o gerador nao sustentar a producao, o resultado sai **invalido com codigo de saida 3**, exatamente como em HTTP — a deteccao de back-pressure acontece no escalonador, antes de qualquer protocolo, e esta coberta por teste com Kafka de verdade (`TestGeradorSaturadoProduzindoInvalidaOResultado`).

### Quando o efeito so aparece por API

Nem todo sistema assincrono publica o resultado num topico: muitos so mostram o efeito numa consulta. Para esses, `aguardar` sonda ate a condicao valer:

```yaml
  - kafka:
      topico: pedidos
      chave: "${pedidos.id}"
      valor: { pedido: "${pedidos.id}" }

  - aguardar:
      http: { caminho: "/pedidos/${pedidos.id}" }
      ate: { $.status: PROCESSADO }     # ou { status: 200 }, ou { corpo_contem: PAGO }
      intervalo: 200ms
      timeout: 30s
```

**A granularidade e declarada, nao escondida.** Medir por sondagem so mede em degraus do intervalo: o valor sai sempre maior ou igual ao real, nunca menor. Por isso o relatorio traz a linha *"o passo X espera sondando a cada 200ms: a latencia dele tem essa granularidade e fica maior que a real, nunca menor"*, e `braunrate debug` mostra o mesmo antes de qualquer carga. Intervalo menor mede mais fino e pesa mais no alvo — a escolha e de quem escreve, com o efeito impresso.

Sem `ate` o passo e recusado: a primeira resposta encerraria a espera e a medicao seria do tempo de responder, nao do tempo ate o efeito acontecer.

RabbitMQ segue a mesma forma:

```yaml
  - amqp:
      fila: pedidos
      identidade: "${pedidos.id}"
      corpo: { pedido: "${pedidos.id}" }
```

## Antes de qualquer veredito: este resultado quer dizer alguma coisa?

Toda execucao passa por uma verificacao de sanidade **antes** de o SLO ser lido. Ela nao pergunta se o alvo foi bem; pergunta se a execucao mediu o que se propos a medir. Quando a resposta e nao, o SLO nem chega a ser avaliado e o comando sai com **codigo 3**:

```
$ braunrate execute cenario.yaml
Consulta sem autenticacao — contra http://127.0.0.1:8099

Resultado invalido: a execucao nao mediu o que se propos a medir. Isto nao e veredito sobre o
alvo — e a medicao que nao vale, e por isso nenhuma regra de SLO foi avaliada.

  - nenhuma jornada chegou ao fim, entao o cenario nao exercitou a sequencia que declarou.
    Rode 'braunrate debug' para ver onde a iteracao para
    60 jornadas iniciadas, 0 completas
  - o passo "consultar pedido" falhou em 100% das requisicoes; nenhuma resposta bem-sucedida
    entrou na medicao dele
    60 requisicoes, 60 erros (status: 60)

$ echo $?
3
```

Os seis casos que invalidam:

| Caso | Por que o numero nao vale |
|---|---|
| nenhuma jornada chegou ao fim | o cenario nao exercitou a sequencia que declarou |
| todos os passos falharam, ou um passo falhou em 100% | a latencia medida e o tempo de recusar, nao o de fazer |
| a carga declarada nao foi aplicada inteira | so o pedaco que rodou ficou medido |
| um passo declarado nao registrou amostra | ele ficou de fora da medicao |
| variedade colapsada em fonte com varios valores | o alvo pode ter respondido de cache |
| gerador saturado | os numeros medem o gerador, nao o alvo |

Os tres primeiros sao novos; os tres ultimos ja existiam soltos e agora passam pelo mesmo ponto de decisao, entao ha um lugar so no codigo que diz que um resultado nao conta.

A verificacao vale **sempre**, com ou sem bloco `slo` — cenario sem SLO continua executando e reportando, so nao serve de gate. Codigo 3 e diferente de codigo 1: **1** e "o alvo nao atendeu o criterio", **3** e "esta execucao nao serve para afirmar nada".

Isso nasceu de tres bugs da mesma familia: dados congelados na primeira iteracao, o proprio `examples/ci.yaml` rodando 100% de 401 e passando verde desde a Fase 1, e a variedade que so foi conferida quando alguem pediu. Os tres eram execucao sintaticamente perfeita, semanticamente vazia, com a suite inteira verde ([ADR 0011](docs/adr/0011-verificacao-de-sanidade.md)).

## Variedade observada: o relatorio diz o que aconteceu, nao o que foi declarado

```
Ambiente
  4 valores distintos de kafka.particao.pedidos-cadeia em 800 usos
  800 valores distintos de pedidos.id em 800 usos
```

Se a fonte tem varios valores e a execucao usou um so, o resultado e **invalido** e o comando sai com codigo 3:

```
RESULTADO INVALIDO: toda a carga caiu numa particao so de pedidos-cadeia; o resto do cluster
ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao
            kafka.particao.pedidos-cadeia tinha 4 valores disponiveis e a execucao usou 1, em 60 usos
```

Isso nasceu de um bug nosso: a autenticacao congelava os dados da primeira iteracao e toda execucao autenticada com CSV rodava sobre a primeira linha, com o relatorio anunciando variedade que nao existiu. A medicao passa a verificar isso em caminho, corpo, cabecalho, variavel de GraphQL e chave de mensagem — um ponto so de instrumentacao, entao protocolo novo entra coberto ([ADR 0007](docs/adr/0007-variedade-observada.md)).

## Comparar duas execucoes

```
$ braunrate compare antes.json depois.json

Ficou mais lento: jornada inteira (95%): 71 vezes mais lento — de 10 ms para 675 ms. Com 2 ressalva(s) que podem explicar a diferenca sozinhas.

Por passo
  passo                        95% antes  95% depois         variacao
  consultar pedido                4.9 ms      598 ms        123x pior
  pagar fatura                    4.8 ms       43 ms        8.9x pior

O que pode explicar a diferenca sem ser o servico
  - as execucoes usaram versoes diferentes do braunrate: 0.2.0 e 0.3.0
  - as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao
  Duas execucoes nao dao intervalo de confianca: variacao abaixo de 5% e tratada como ruido.
```

A comparacao nunca chama de regressao o que pode ser ruido, lista tudo que mudou fora do servico (maquina, plano de carga, versao, cenario), e se recusa a comparar quando alguma das duas execucoes teve o gerador saturado.

## Modo servidor: os mesmos comandos por HTTP

```bash
braunrate serve -addr 127.0.0.1:8080 -dir ./cenarios
```

```
braunrate serve em http://127.0.0.1:8080, servindo cenarios de ./cenarios
Sem autenticacao e sem TLS: qualquer um que alcance esta porta pode disparar carga contra os alvos dos cenarios.
Foi feito para rodar em 127.0.0.1. Expor em outra interface e outra decisao, e ela ainda nao foi tomada.
```

Validar, depurar, executar, acompanhar, listar, buscar o JSON e o HTML, comparar duas execucoes — **o que a CLI ja faz, e nada alem disso.** Toda rota termina no mesmo `internal/runner` que o terminal usa, e um teste reprova o build se os dois deixarem de produzir o mesmo documento.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/runs
```

```json
{ "id": "r001", "status": "running", "stream": "/runs/r001/stream" }
```

```bash
curl -sN http://127.0.0.1:8080/runs/r001/stream
```

```
executando "Fumaca de CI" contra http://127.0.0.1:8080: 975 iteracoes em 6s
carga 200/s | enviadas 576 | concluidas 575 | erros 0 | metade em 5.8 ms | 99% em 7.6 ms | faltam 2s
carga 0/s | enviadas 975 | concluidas 974 | erros 0 | metade em 5.5 ms | 99% em 7.4 ms | faltam 0s
passou (codigo 0)
```

**Uma execucao por vez, por padrao.** Duas execucoes na mesma maquina disputam a CPU que precisa despachar no instante agendado, e nenhuma das duas mede o que se propos a medir. A segunda responde `409` e diz como aceitar a contaminacao (`-concurrent`), se for esse o caso.

**O YAML continua sendo a verdade.** Nao ha banco: os cenarios sao os arquivos do `-dir`, e as execucoes vivem na memoria do processo. Sem interface grafica, sem conta de usuario, sem multiusuario, sem agendamento — isso esta fora de escopo, nao para depois.

Um exemplo de `curl` por rota, com a resposta real, esta em [docs/api-servidor.md](docs/api-servidor.md).

## O que existe hoje

| Recurso | Estado |
|---|---|
| Motor de chegada aberta, latencia do instante agendado | pronto |
| Modelo fechado declarado, com aviso permanente e sem latencia corrigida | pronto |
| Perfis: rampa, patamar, pico, taxa constante | pronto |
| HDR histogram, agregados mergeaveis, series alinhadas ao epoch | pronto |
| Deteccao de back-pressure com causa provavel (gerador x alvo) | pronto |
| Limite de requisicoes em voo, com descarte declarado no relatorio | pronto |
| HTTP: verbos, cabecalhos, corpo JSON, redirect, timeout, cookies | pronto |
| YAML com erro apontando linha e coluna | pronto |
| Resumo de terminal e progresso ao vivo | pronto |
| Correlacao em uma linha: JSON, cabecalho e regex | pronto |
| Autenticacao por token com renovacao, e basica | pronto |
| Dados: CSV com politica de consumo e geracao com semente | pronto |
| Valor gerado estavel na jornada, com `novo_a_cada: uso` explicito | pronto |
| Formato declarado, e CPF/CNPJ com digito verificador valido | pronto |
| Assercoes funcionais e SLO por passo e global, com codigo de saida | pronto |
| Criterio sobre a jornada inteira, taxa de sucesso e taxa efetiva | pronto |
| Comparacao com execucao anterior como gate, com ressalva que tira o veredito | pronto |
| Todo exemplo publicado roda no CI; exemplo que nao roda quebra o build | pronto |
| Verificacao de sanidade do resultado antes de qualquer veredito | pronto |
| Tempo total da jornada, contado do instante agendado | pronto |
| Autoria: schema no editor, `debug`, `import curl`, `import jmx`, erros que ensinam | pronto |
| Gravador de trafego com correlacao sugerida, agrupamento e filtro de ruido | pronto |
| Relatorio HTML autocontido, com veredito em uma frase | pronto |
| JSON versionado, CSV por passo, comparacao entre execucoes | pronto |
| GraphQL: uma linha por operacao, erro em 200 contado como erro | pronto |
| Kafka e RabbitMQ com entrega confirmada, sem lote | pronto |
| Passo `aguardar`: mede a cadeia assincrona ponta a ponta | pronto |
| Modo servidor local: os mesmos comandos por HTTP, sem logica nova | pronto |
| Autenticacao de broker: SASL/PLAIN, SCRAM, TLS com CA propria, mTLS | pronto |
| AWS MSK com IAM pela cadeia padrao da AWS, sem chave no cenario | pronto, sem CI |
| Segredo literal no cenario reprova a validacao, e a saida nunca mostra credencial | pronto |
| Variedade observada, com resultado invalido quando a carga concentra | pronto |
| Cenario em Go, com equivalencia YAML x DSL travada por teste | pronto |
| `importar jmx`: requisicao, cabecalho, CSV e correlacao do plano do JMeter | parcial, declarado |

## Por que existe

Tres razoes, nesta ordem:

1. **Medicao honesta por padrao.** Modelo de chegada aberto; latencia contada a partir do instante em que a requisicao *deveria* ter partido; HDR histogram; aviso explicito quando o gerador nao sustentou a taxa alvo. A omissao coordenada e a falha que faz teste passar com p99 de 47 ms enquanto producao sofre 1,8 s.
2. **Dois publicos, um motor.** YAML declarativo para o caso comum, DSL para o complexo — mesmo motor, mesmas metricas, sem reescrita ao migrar.
3. **Cenario de negocio, nao so requisicao.** GraphQL medido por operacao; Kafka e RabbitMQ com modelo de metrica proprio; passo `aguardar` para medir a cadeia assincrona ponta a ponta.

## Escopo

**Dentro:** HTTP/HTTPS e REST; GraphQL de primeira classe; Kafka e RabbitMQ (produzir e consumir); passo `aguardar` com timeout; correlacao, variaveis e fluxo de autenticacao; CSV com politica de consumo e geracao sintetica com semente; perfis de carga (rampa, patamar, pico, taxa constante) e modelo fechado declarado; SLO com codigo de saida; relatorio HTML autocontido, JSON, CSV e resumo de terminal; comparacao entre execucoes; importador de `.jmx` para o subconjunto comum; gravador de trafego HTTP; modo servidor local sem logica propria; autenticacao de broker (SASL/PLAIN, SCRAM, TLS com CA propria, mTLS e AWS MSK com IAM), sempre com a credencial fora do arquivo.

**Limitacao conhecida:** protocolo fora da lista acima exige recompilar o binario — a mesma friccao que o k6 tem. E consequencia da escolha de Go ([ADR 0004](docs/adr/0004-extensao-de-protocolo.md)), esta declarada aqui de proposito, e o processo de build reprodutivel para protocolo fora-de-arvore sera documentado. Avro e Schema Registry sao mais fracos em Go que na JVM e ficam para depois da v1.

**Limitacao conhecida:** um unico token para a execucao inteira, com a consequencia declarada no relatorio ([ADR 0005](docs/adr/0005-identidade-e-token.md)). E a latencia dos passos seguintes ao primeiro e tempo de servico, nao latencia corrigida — a leitura honesta da jornada esta no bloco "A jornada inteira".

**Limitacao conhecida:** o caminho completo do AWS MSK com IAM nao e exercitado no CI — o que roda la e SCRAM sobre TLS contra um broker de verdade, e a assinatura IAM tem cobertura de unidade. Servico gerenciado de nuvem (SQS, SNS, Kinesis, EventBridge, Service Bus, Pub/Sub) e GoldenGate ficam fora, com o motivo em [ADR 0014](docs/adr/0014-autenticacao-de-mensageria.md): os primeiros nao sao broker apontavel e entrariam como protocolos novos; o ultimo e replicacao de banco, nao QA de aplicacao. OAUTHBEARER fica para depois da v1.

**Fora:** motor de browser real; nuvem gerenciada, dashboard multiusuario, conta de time; interface grafica, agendamento e persistencia alem dos arquivos no modo servidor; LDAP, FTP, SMTP, JMS classico; competir em vazao bruta com wrk; execucao distribuida na v1 — a arquitetura nao pode impedi-la, mas ela nao entra agora.

## Documentacao

- [Principios de produto](docs/principios-de-produto.md) — criterio de aceitacao de toda decisao de interface
- [Roteiro](docs/roteiro.md)
- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — base de todas as decisoes
- [Arquitetura](docs/arquitetura.md)
- [ADR 0001 — linguagem e runtime](docs/adr/0001-linguagem-e-runtime.md)
- [ADR 0002 — modelo de cenario](docs/adr/0002-modelo-de-cenario.md)
- [ADR 0003 — modelo de execucao e metrica](docs/adr/0003-modelo-de-execucao-e-metrica.md)
- [ADR 0004 — extensao de protocolo](docs/adr/0004-extensao-de-protocolo.md)
- [ADR 0005 — identidade e token](docs/adr/0005-identidade-e-token.md)
- [ADR 0006 — GraphQL como unidade de medida](docs/adr/0006-graphql-como-unidade-de-medida.md)
- [ADR 0007 — variedade observada](docs/adr/0007-variedade-observada.md)
- [ADR 0008 — mensageria e cadeia assincrona](docs/adr/0008-mensageria-e-cadeia-assincrona.md)
- [ADR 0009 — equivalencia entre YAML e DSL](docs/adr/0009-equivalencia-entre-yaml-e-dsl.md)
- [ADR 0010 — codigo em ingles, produto em portugues](docs/adr/0010-idioma-do-codigo.md)
- [ADR 0011 — verificacao de sanidade antes do veredito](docs/adr/0011-verificacao-de-sanidade.md)
- [ADR 0012 — modelo fechado como opcao declarada](docs/adr/0012-modelo-fechado-como-opcao-declarada.md)
- [ADR 0013 — gravador de trafego](docs/adr/0013-gravador-de-trafego.md)
- [ADR 0014 — autenticacao de mensageria](docs/adr/0014-autenticacao-de-mensageria.md)
- [API do modo servidor](docs/api-servidor.md) — um exemplo de curl por rota
- [Schema do cenario](docs/braunrate.schema.json) — autocompletar e validacao no editor
- [Exemplo de relatorio HTML](docs/exemplo-relatorio.html) — saida real de uma execucao que falhou o SLO
- [Medicao dos prototipos da Fase 0](docs/medicoes-fase0.md)

## Licenca

MIT — Diego Braun.
