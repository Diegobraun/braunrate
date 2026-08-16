# Protocolos

Cinco protocolos compilados no binario: `http`, `graphql`, `kafka`, `amqp` e
`aguardar`. `braunrate version` lista os que o seu binario tem.

O que muda de um para o outro nao e so a sintaxe: e **o que conta como uma
unidade de medida** e **o que conta como erro**.

## HTTP

A forma curta cabe em uma linha; a longa aceita metodo, cabecalhos, corpo e
timeout:

```yaml trecho
cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    verificar: { status: 200, json: { $.ultimaFatura.status: ABERTA } }
    captura: { faturaId: $.ultimaFatura.id }

  - nome: pagar fatura
    http:
      metodo: POST
      caminho: "/faturas/${faturaId}/pagar"
      cabecalhos: { X-Tenant: "${tenant}" }
      corpo: { valor: 199.90 }
    verificar: { status: 200 }
```

A unidade de medida e o passo. Erro e status fora do declarado em `verificar`,
falha de rede, timeout, ou assercao que nao bateu — cada um vira uma classe
propria no relatorio, porque "falhou 30 vezes" nao diz para onde olhar.

### Alvo HTTPS com certificado proprio

Homologacao corporativa quase sempre serve HTTPS com uma CA interna, ou exige
certificado de cliente. Se a CA ja esta no armazenamento de confianca da maquina,
nao ha nada a declarar. Quando nao esta:

```yaml trecho
alvo: https://api.homolog.interno

tls:
  ca: /etc/ssl/homolog/ca.pem
```

Para mTLS, o certificado de cliente entra junto:

```yaml trecho
tls:
  ca: /etc/ssl/homolog/ca.pem
  certificado: /etc/ssl/homolog/cliente.pem
  chave: /etc/ssl/homolog/cliente.key
```

Vale para `http`, `graphql` e para a sondagem do `aguardar`. Sem o bloco, o erro
diz o que declarar em vez de repassar o texto do x509:

```
  consultar                  falha de rede                              30   certificado assinado por CA que esta maquin…
    certificado assinado por CA que esta maquina nao conhece — declare tls: { ca: /caminho/ca.pem }
```

O caminho do arquivo aceita `${VARIAVEL}`. Nenhuma chave privada e lida para
dentro do cenario: o que vai no arquivo e o caminho, nunca o conteudo.

## GraphQL

Cole a consulta; o nome da operacao vira a linha do relatorio:

```yaml trecho
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

Duas coisas que uma ferramenta de HTTP generico erra em GraphQL, e que aqui sao o
padrao:

**A operacao e a unidade de medida.** Tudo em GraphQL chega em `POST /graphql`.
Agregar por URL colocaria a consulta mais barata e a mutation mais cara na mesma
linha, e o 99% da mais cara sumiria. Por isso a chave e
`graphql ConsultarPedido`, e operacao anonima e recusada na leitura do cenario —
com a mensagem mostrando como dar nome.

**Erro com status 200 e erro.** A especificacao manda responder `200` com
`errors` no corpo. Execucao real contra o alvo embutido, onde um quarto dos
assinantes nao existe:

```
Falhou: o cenario inteiro teve taxa de erro de 14.28%, acima do limite de 0.10%.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  graphql ConsultarPedido    (1)      1.625    4.7 ms    5.1 ms    5.4 ms    5.8 ms     14 ms     406
  graphql PagarFatura        (2)      1.219    4.7 ms    5.0 ms    5.2 ms    5.8 ms    6.0 ms       0

Erros
  erro no corpo da resposta GraphQL (com status 200)  406
```

**Todas as 2.844 respostas vieram com status HTTP 200.** Uma ferramenta que
classifica por status teria reportado 0% de erro e criterio de aceite verde.
Resposta parcial (`data` e `errors` juntos) tambem conta como erro, e o detalhe
diz que foi parcial.

## Kafka e RabbitMQ

Produzir mede o broker aceitando a mensagem. O que o usuario sente e a cadeia
inteira — e para isso existe o passo `aguardar`:

```yaml trecho
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

RabbitMQ segue a mesma forma:

```yaml trecho
cenario:
  - amqp:
      fila: pedidos
      identidade: "${pedidos.id}"
      corpo: { pedido: "${pedidos.id}" }
```

Publicacao com confirmacao e o padrao (`acks: todos` no Kafka, publisher confirms
no AMQP), **sem lote** — agrupar mensagens mediria o lote, nao a mensagem.

### Por que a cadeia inteira

Execucao real contra Kafka com o consumidor deliberadamente sobrecarregado —
15 ms por mensagem, ou seja 66/s de capacidade, contra uma carga de 100/s:

```
A jornada inteira
  Todas as 800 jornadas chegaram ao fim; metade levou ate 1490 ms e 95% ate 3957 ms,
  contados do instante em que deveriam ter comecado.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  aguardar pedidos-lento-pr… (2)        800    1.49 s    3.95 s    4.17 s    4.23 s    4.24 s       0
  kafka produzir pedidos-le… (1)        800    1.2 ms    2.2 ms    3.9 ms    179 ms    228 ms       0
```

**Produzir custa 1,2 ms; a cadeia custa 3,96 s no 95%.** Uma ferramenta que so
mede a producao teria reportado milissegundo e aprovado o sistema.

### Quanto o consumidor ficou para tras

Quando o consumidor e um servico de verdade, a cadeia ponta a ponta nem sempre
esta disponivel. O que da para ler direto do broker e o **atraso do grupo**:

```yaml trecho
cenario:
  - kafka:
      topico: pedidos
      grupo: cobranca              # so observa; nao consome nada
      chave: "${pedidos.id}"
      valor: { pedido: "${pedidos.id}" }
```

```
Atraso do consumidor
  grupo demo-lag-grupo em demo-lag: no pior momento 885 mensagens atras; no fim, 885 mensagens
  O consumidor terminou a execucao para tras. O atraso diz a distancia, nao a causa: consumidor lento, parado ou em rebalanceamento produzem o mesmo numero.
```

Os dois numeros sao lidos do broker (marca d'agua alta menos offset confirmado do
grupo), nunca contados deste lado: mensagem que este gerador nao enviou tambem
pesa no servico. Se nao der para ler o offset, o relatorio diz que nao conseguiu
medir — zero ali afirmaria que o consumidor estava em dia.

Para mandar toda a carga de um passo para uma particao so — teste de particao
quente, replica especifica — existe `particao: N`. Ela ignora a chave, e o
relatorio marca a execucao como concentrada de proposito.

### Quanto o gerador aguenta produzindo

Sem lote, uma mensagem por chegada agendada e com confirmacao, a taxa maxima e
menor que a de ferramentas que agrupam. O numero, medido:

| Topico | Ultima taxa valida | metade / 95% da producao | Primeira taxa invalida |
|---|---|---|---|
| 6 particoes | **15.000 msg/s** | 1,4 ms / 41 ms | 18.000/s (4.884 descartadas) |
| 1 particao | **5.000 msg/s** | 0,2 ms / 56 ms | 8.000/s (8.059 descartadas) |

Em 15.000/s o desvio de agendamento ficou em 0,001 ms tipico e 0,56 ms no pior
caso: **quem saturou primeiro nao foi o escalonador do braunrate, foi o caminho
de entrega confirmada contra o broker.** O numero da tabela e o teto do par
gerador+broker nesta maquina.

**Ambiente:** Apple M2 Pro, 10 nucleos, Redpanda v24.2.7 em loopback no mesmo
host, 10 s por execucao. Broker remoto, replicacao real e mensagem maior mudam o
numero. Meca no seu ambiente antes de citar este.

### Apontando para um broker real

Credencial **nunca vai para o arquivo** — so nome de variavel de ambiente ou a
cadeia padrao da nuvem. O cenario vai para o repositorio; o repositorio guarda
para sempre.

```yaml trecho
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
Mensageria: kafka em kafka.homolog:9093: scram_sha512, usuario ana + TLS com CA propria
```

Tipos aceitos: `sasl_plain`, `scram_sha256`, `scram_sha512`, `msk_iam` e
`certificado` (mTLS). `tls: true` liga TLS com as autoridades do sistema;
`tls: { ca: ... }` usa uma autoridade interna.

**AWS MSK com IAM** — nao ha campo de chave, e nao vai haver:

```yaml trecho
mensageria:
  kafka:
    brokers: [b-1.msk.exemplo:9098, b-2.msk.exemplo:9098]
    autenticacao: { tipo: msk_iam, regiao: us-east-1 }
```

A assinatura vem da cadeia padrao da AWS — `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, ou
a role da maquina. TLS e ligado sozinho: a porta 9098 nao aceita outra coisa.

Senha escrita no arquivo **reprova a validacao**, e a mensagem ensina a saida:

```
erro no cenario: homolog.yaml:7:77: senha literal no cenario: credencial nunca vai para o arquivo, porque o arquivo vai para o repositorio.
    troque por:  senha: ${BROKER_SENHA}
    e rode com:  BROKER_SENHA=... braunrate execute cenario.yaml
    valor de reserva (${VAR:-algo}) tambem nao serve: a reserva seria o segredo escrito no arquivo
```

Terminal, HTML, JSON e depuracao mostram tipo de autenticacao e usuario, nunca o
segredo. Senha errada vira erro de classe `autenticacao` e falta de permissao
vira `autorizacao` — nenhuma das duas vira "broker indisponivel", que mandaria
olhar o firewall.

**Fora por enquanto:** OAUTHBEARER. **Fora, com motivo:** servico gerenciado de
nuvem (SQS, SNS, Kinesis, EventBridge, Service Bus, Pub/Sub) nao e broker
apontavel, e sim SDK com semantica propria de entrega e cobranca; entraria como
protocolo novo, nao como autenticacao.

## `aguardar`

Mede o tempo ate o efeito acontecer, nao o tempo de responder. Duas formas: por
mensagem, mostrada acima, e por sondagem de API — para quando o sistema
assincrono nao publica o resultado num topico:

```yaml trecho
cenario:
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

**A granularidade e declarada, nao escondida.** Medir por sondagem so mede em
degraus do intervalo: o valor sai sempre maior ou igual ao real, nunca menor. Por
isso o relatorio traz a linha *"o passo X espera sondando a cada 200ms: o tempo
dele tem essa granularidade e fica maior que o real, nunca menor"*, e
`braunrate debug` mostra o mesmo antes de qualquer carga.

Sem `ate` o passo e recusado: a primeira resposta encerraria a espera, e a
medicao seria do tempo de responder em vez do tempo ate o efeito acontecer.
