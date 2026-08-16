# ADR 0014 — Autenticação de mensageria: credencial fora do arquivo

## Contexto

Broker de homologacao ou de producao nao aceita conexao anonima. Ate a Fase 6 o
braunrate so falava com Kafka e RabbitMQ sem credencial, o que quer dizer: so
falava com o broker do laptop. Um teste de carga que nunca tocou o broker de
homologacao nao decide nada sobre ele.

O jeito comum de resolver isso e o jeito errado: campo de senha no arquivo de
cenario. O arquivo vai para o repositorio, e o repositorio guarda para sempre.

## Decisao

### O que existe

Por ordem de uso real, nao de facilidade de implementacao:

1. **SASL/PLAIN e SASL/SCRAM** (SHA-256 e SHA-512) — o que a maioria dos brokers
   gerenciados e auto-hospedados usa.
2. **TLS**, com CA propria opcional. Homologacao quase sempre tem certificado
   assinado por uma autoridade interna.
3. **AWS MSK com IAM** — assinatura SigV4 pela cadeia padrao de credenciais da
   AWS. Nunca ha campo de chave no cenario.
4. **mTLS** — certificado de cliente, com `tipo: certificado`.
5. **RabbitMQ** com usuario e senha e com mTLS.

**OAUTHBEARER fica fora da v1.** Ele exige um provedor de identidade
configurado, e o caminho muda por provedor: sem um caso concreto para validar,
o que sairia seria um campo que ninguem consegue testar.

### Credencial so por ambiente ou pela cadeia da nuvem

```yaml
mensageria:
  kafka:
    brokers: [kafka.homolog:9092]
    autenticacao: { tipo: scram_sha512, usuario: "${KAFKA_USUARIO}", senha: "${KAFKA_SENHA}" }
    tls: { ca: /caminho/ca.pem }
```

Valor literal de senha no arquivo faz a **validacao recusar o cenario**, com a
mensagem ensinando a forma certa. Valor de reserva (`${VAR:-alguma-coisa}`)
tambem e recusado: a reserva seria o segredo escrito no arquivo.

Campo com nome de chave — `chave`, `token`, `segredo`, `access_key`,
`secret_key` — e recusado pelo nome, apontando a cadeia padrao da AWS. E a mesma
postura do `import curl`, que ja tira o `Authorization` do arquivo.

### Nenhuma credencial na saida

Terminal, HTML, JSON e depuracao passam por uma unica funcao de descricao, que
mostra tipo de autenticacao e usuario e nunca o segredo. Endereco AMQP perde a
senha do userinfo antes de ser impresso. Uma unica funcao porque, se ela algum
dia vazar, vaza em toda parte de uma vez — e um lugar so para consertar.

### Credencial errada nao e broker indisponivel

Autenticacao e autorizacao viram classes proprias de erro no relatorio,
separadas de rede. Senha errada manda a pessoa olhar a variavel de ambiente;
falta de permissao manda olhar a ACL do broker. Reportar as duas como falha de
rede manda olhar o firewall, que e onde o problema nao esta.

### Aperto de mao fora da latencia

TLS e SASL custam centenas de milissegundos e sao pagos uma vez, quando a
conexao abre. A conexao e aberta na preparacao, antes do relogio da execucao
comecar — o mesmo tratamento que a assinatura do consumidor recebeu na Fase 5.
Se entrasse na medicao, a primeira mensagem carregaria o aperto de mao inteiro e
o p99 descreveria a conexao, nao o broker.

## O que o CI exercita, e o que ele nao exercita

O CI sobe um Kafka com **SASL/SCRAM-SHA-512 sobre TLS com CA propria**
(`.github/broker-autenticado.sh`) e roda os testes contra ele: producao pela
credencial certa, senha errada classificada como autenticacao e nunca como rede,
aperto de mao fora da latencia da primeira mensagem, e relatorio descrevendo a
conexao sem o segredo.

**O caminho completo do MSK com IAM nao e exercitado no CI**, porque isso exigiria
uma conta AWS com um cluster de verdade. O que existe e teste de unidade: o
mecanismo assina na regiao declarada, o nome do mecanismo e `AWS_MSK_IAM`, e
nenhuma chave e pedida. Esta declarado aqui de proposito.

## Fora de escopo, com o motivo

**Servico gerenciado de nuvem** — SQS, SNS, Kinesis, EventBridge, Service Bus,
Pub/Sub. Nao sao protocolo de mensageria com broker apontavel: cada um e um SDK
com semantica propria de entrega, cobranca e limite. Entrariam como protocolos
novos, nao como autenticacao, e cada um traz sua propria pergunta sobre o que a
latencia medida quer dizer.

**GoldenGate.** E replicacao de banco por protocolo proprietario. Nao e QA de
aplicacao: quem mede replicacao mede consistencia e atraso de aplicacao de
transacao, que sao outras perguntas e outra ferramenta.

## Consequencias

O cenario continua podendo ir para o repositorio, porque nao carrega segredo. O
custo e que rodar contra homologacao exige variavel de ambiente no shell ou no
CI — friccao declarada, e a friccao certa.
