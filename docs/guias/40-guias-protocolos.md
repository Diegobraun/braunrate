# Protocolos

Cinco protocolos compilados no binário: `http`, `graphql`, `kafka`, `amqp` e
`aguardar`. `braunrate version` lista os que o seu binário tem.

O que muda de um para o outro não é só a sintaxe: é **o que conta como uma
unidade de medida** e **o que conta como erro**.

## HTTP

A forma curta cabe em uma linha; a longa aceita método, cabeçalhos, corpo e
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

A unidade de medida é o passo. Erro é status fora do declarado em `verificar`,
falha de rede, timeout, ou asserção que não bateu. Cada um vira uma classe
própria no relatório, porque "falhou 30 vezes" não diz para onde olhar.

### Alvo HTTPS com certificado próprio

Homologação corporativa quase sempre serve HTTPS com uma CA interna, ou exige
certificado de cliente. Se a CA já está no armazenamento de confiança da máquina,
não há nada a declarar. Quando não está:

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

> **Nota** O caminho do arquivo aceita `${VARIAVEL}`. Nenhuma chave privada é
> lida para dentro do cenário: o que vai no arquivo é o caminho, nunca o
> conteúdo.

## GraphQL

Cole a consulta; o nome da operação vira a linha do relatório:

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

Duas coisas que uma ferramenta de HTTP genérico erra em GraphQL, e que aqui são o
padrão.

**A operação é a unidade de medida.** Tudo em GraphQL chega em `POST /graphql`.
Agregar por URL colocaria a consulta mais barata e a mutation mais cara na mesma
linha, e o 99% da mais cara sumiria. Por isso a chave é `graphql ConsultarPedido`,
e operação anônima é recusada na leitura do cenário, com a mensagem mostrando como
dar nome.

**Erro com status 200 é erro.** A especificação manda responder `200` com `errors`
no corpo. Execução real contra o alvo embutido, onde um quarto dos assinantes não
existe:

```
Falhou: o cenario inteiro teve taxa de erro de 14.28%, acima do limite de 0.10%.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  graphql ConsultarPedido    (1)      1.625    4.7 ms    5.1 ms    5.4 ms    5.8 ms     14 ms     406
  graphql PagarFatura        (2)      1.219    4.7 ms    5.0 ms    5.2 ms    5.8 ms    6.0 ms       0

Erros
  erro no corpo da resposta GraphQL (com status 200)  406
```

Todas as 2.844 respostas vieram com status HTTP 200. Uma ferramenta que classifica
por status teria reportado 0% de erro e critério de aceite verde. Resposta parcial
(`data` e `errors` juntos) também conta como erro, e o detalhe diz que foi
parcial.

## Kafka e RabbitMQ

Produzir mede o broker aceitando a mensagem. O que o usuário sente é a cadeia
inteira, e para isso existe o passo `aguardar`:

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

Publicação com confirmação é o padrão (`acks: todos` no Kafka, publisher confirms
no AMQP), **sem lote**: agrupar mensagens mediria o lote, não a mensagem.

### Por que a cadeia inteira

Execução real contra Kafka com o consumidor deliberadamente sobrecarregado — 15 ms
por mensagem, ou seja 66/s de capacidade, contra uma carga de 100/s:

```
A jornada inteira
  Todas as 800 jornadas chegaram ao fim; metade levou ate 1490 ms e 95% ate 3957 ms,
  contados do instante em que deveriam ter comecado.

Por passo
  passo                          requisicoes    metade       95%       99%     99,9%      pior   erros
  aguardar pedidos-lento-pr… (2)        800    1.49 s    3.95 s    4.17 s    4.23 s    4.24 s       0
  kafka produzir pedidos-le… (1)        800    1.2 ms    2.2 ms    3.9 ms    179 ms    228 ms       0
```

Produzir custa 1,2 ms; a cadeia custa 3,96 s no 95%. Uma ferramenta que só mede a
produção teria reportado milissegundo e aprovado o sistema.

### Quanto o consumidor ficou para trás

Quando o consumidor é um serviço de verdade, a cadeia ponta a ponta nem sempre
está disponível. O que dá para ler direto do broker é o **atraso do grupo**:

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

Os dois números são lidos do broker (marca d'água alta menos offset confirmado do
grupo), nunca contados deste lado: mensagem que este gerador não enviou também
pesa no serviço. Se não der para ler o offset, o relatório diz que não conseguiu
medir, porque zero ali afirmaria que o consumidor estava em dia.

Para mandar toda a carga de um passo para uma partição só — teste de partição
quente, réplica específica — existe `particao: N`. Ela ignora a chave, e o
relatório marca a execução como concentrada de propósito.

### Quanto o gerador aguenta produzindo

Sem lote, uma mensagem por chegada agendada e com confirmação, a taxa máxima é
menor que a de ferramentas que agrupam. O número, medido:

| Tópico | Última taxa válida | metade / 95% da produção | Primeira taxa inválida |
|---|---|---|---|
| 6 partições | **15.000 msg/s** | 1,4 ms / 41 ms | 18.000/s (4.884 descartadas) |
| 1 partição | **5.000 msg/s** | 0,2 ms / 56 ms | 8.000/s (8.059 descartadas) |

Em 15.000/s o desvio de agendamento ficou em 0,001 ms típico e 0,56 ms no pior
caso: quem saturou primeiro não foi o escalonador do braunrate, foi o caminho de
entrega confirmada contra o broker. O número da tabela é o teto do par
gerador+broker nesta máquina.

> **Atenção** Ambiente da medição: Apple M2 Pro, 10 núcleos, Redpanda v24.2.7 em
> loopback no mesmo host, 10 s por execução. Broker remoto, replicação real e
> mensagem maior mudam o número. Meça no seu ambiente antes de citar este.

### Apontando para um broker real

Credencial **nunca vai para o arquivo**: só nome de variável de ambiente ou a
cadeia padrão da nuvem. O cenário vai para o repositório, e o repositório guarda
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

**AWS MSK com IAM.** Não há campo de chave, e não vai haver:

```yaml trecho
mensageria:
  kafka:
    brokers: [b-1.msk.exemplo:9098, b-2.msk.exemplo:9098]
    autenticacao: { tipo: msk_iam, regiao: us-east-1 }
```

A assinatura vem da cadeia padrão da AWS: `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, ou a
role da máquina. TLS é ligado sozinho, porque a porta 9098 não aceita outra coisa.

Senha escrita no arquivo reprova a validação, e a mensagem ensina a saída:

```
erro no cenario: homolog.yaml:7:77: senha literal no cenario: credencial nunca vai para o arquivo, porque o arquivo vai para o repositorio.
    troque por:  senha: ${BROKER_SENHA}
    e rode com:  BROKER_SENHA=... braunrate execute cenario.yaml
    valor de reserva (${VAR:-algo}) tambem nao serve: a reserva seria o segredo escrito no arquivo
```

Terminal, HTML, JSON e depuração mostram tipo de autenticação e usuário, nunca o
segredo. Senha errada vira erro de classe `autenticacao` e falta de permissão vira
`autorizacao`; nenhuma das duas vira "broker indisponível", que mandaria olhar o
firewall.

**Fora por enquanto:** OAUTHBEARER. **Fora, com motivo:** serviço gerenciado de
nuvem (SQS, SNS, Kinesis, EventBridge, Service Bus, Pub/Sub) não é broker
apontável, e sim SDK com semântica própria de entrega e cobrança; entraria como
protocolo novo, não como autenticação.

## `aguardar`

Mede o tempo até o efeito acontecer, não o tempo de responder. São duas formas:
por mensagem, mostrada acima, e por sondagem de API, para quando o sistema
assíncrono não publica o resultado num tópico:

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

A granularidade é declarada, não escondida. Medir por sondagem só mede em degraus
do intervalo: o valor sai sempre maior ou igual ao real, nunca menor. Por isso o
relatório traz a linha *"o passo X espera sondando a cada 200ms: o tempo dele tem
essa granularidade e fica maior que o real, nunca menor"*, e `braunrate debug`
mostra o mesmo antes de qualquer carga.

Sem `ate` o passo é recusado: a primeira resposta encerraria a espera, e a medição
seria do tempo de responder em vez do tempo até o efeito acontecer.
