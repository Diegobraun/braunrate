# 23. Streaming como medição própria (gRPC e WebSocket)

Data: 2026-08-18
Status: aceito

## Contexto

O modelo de carga era uma iteração = uma requisição = uma resposta. gRPC de
streaming era recusado (ADR 0022) e o passo WebSocket enviava uma mensagem e, no
máximo, lia uma resposta. Um alvo real de streaming — server-stream (o servidor
empurra N mensagens) — não cabe nesse molde, e é onde k6/Gatling têm suporte.

O que muda não é o transporte, é a **unidade de medida**: um stream não tem uma
latência, tem N mensagens ao longo de um tempo.

## Decisão

Um passo de streaming continua sendo uma iteração. Como ele já bloqueia até o
stream terminar, a **latência da iteração já é a duração total do stream** — sem
métrica nova para isso. O que falta é a contagem, então a `protocol.Response`
ganha um campo opcional `Messages int64`, somado pelo agregador ao lado de
`Bytes`; os protocolos unários deixam em zero e nada muda para eles. A coluna
`messages` entra no CSV e no JSON do resultado (`omitempty`).

O stream fecha, encerrando a iteração, no primeiro que ocorrer:
- o servidor fecha o stream;
- `maxMessages: N` é atingido;
- o `timeout` do passo chega — para um stream, chegar ao fim da janela é um
  encerramento limpo, não uma falha.

### gRPC

`resolve` deixa de recusar métodos **server-streaming**: o passo envia a única
requisição, faz `CloseSend` e drena as respostas contando mensagens e somando
bytes, até `maxMessages`/fim do stream/timeout. Client-streaming e bidi seguem
recusados com mensagem clara.

    - grpc:
        method: grpc.health.v1.Health/Watch
        message: '{}'
        maxMessages: 20

### WebSocket

`maxMessages: N` liga o modo drain: depois do `send`, o passo recebe até N frames
(ou até o servidor fechar / o timeout chegar), contando cada um. `awaitReply`
(uma resposta só) segue existindo para o caso simples.

    - websocket:
        path: /feed
        send: '{"sub":"orders"}'
        maxMessages: 50
        timeout: 10s

## Consequências

- Métrica nova (`Messages`) sem tocar nas antigas nem na tabela do terminal —
  campo opcional, some para quem não usa.
- gRPC server-streaming deixa de ser recusado.
- Duração e volume do stream vêm de graça da latência e de `Bytes`.

## O que reabre

- **Latência da primeira mensagem** (time-to-first-message) como distribuição
  própria: hoje só a duração total do stream é medida.
- **gRPC client-streaming e bidi**: exigem um modelo de envio de múltiplas
  mensagens por iteração, recusados por ora.
- **Percentis de intervalo entre mensagens** (jitter), não só contagem e total.
- **Exposição no formulário da interface** (`maxMessages`): a chave existe no
  YAML; o form ainda não a mostra.
