# ADR 0022 — gRPC e WebSocket como protocolos de primeira classe

- **Status**: aceito
- **Data**: 2026-08-18
- **Contexto de decisao**: comparacao com k6, Gatling e JMeter — os tres tem gRPC e WebSocket, braunrate nao tinha
- **Relacionados**: [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [ADR 0007](0007-variedade-observada.md), [ADR 0009](0009-equivalencia-entre-yaml-e-dsl.md), [ADR 0019](0019-formato-em-ingles.md)

## Contexto

braunrate media REST, GraphQL, Kafka e AMQP. As ferramentas de carga com que ele é comparado — k6, Gatling, JMeter — cobrem dois protocolos que faltavam e que aparecem em arquitetura de microserviço moderna:

- **gRPC**: o RPC padrão entre serviços internos. Sem ele, a carga entre serviços internos — justamente onde o gargalo costuma estar — fica fora do alcance da ferramenta.
- **WebSocket**: a conexão persistente de tempo real. Streaming de cotação, notificação, chat.

Não tê-los era uma lacuna real de mercado, não uma escolha de design.

## Decisao

**gRPC e WebSocket entram como protocolos registrados, com a mesma fronteira dos demais (ADR 0003): o protocolo traz o domínio, a medição decide.** Cada um é um pacote em `internal/protocol/` com `init()` que se registra, e o step do cenário usa o nome como chave, como todo protocolo já faz.

Os dois saem completos. O que os separa é a dependência:

1. **WebSocket sai completo.** O transporte usa `golang.org/x/net/websocket`, que já está no grafo do módulo — nenhuma dependência nova. O step disca, envia a mensagem, e opcionalmente lê uma resposta:

   ```yaml
   - websocket:
       path: /stream
       send: '{"subscribe":"orders"}'
       awaitReply: true
       timeout: 10s
       headers: { Authorization: "Bearer ${TOKEN}" }
   ```

2. **gRPC sai completo, pela reflexão do servidor.** O passo não pede `.proto` nem stub gerado: o cliente disca, lê o descritor do método pela **server reflection** do alvo, monta a mensagem a partir do JSON com mensagem dinâmica e invoca. É o mesmo caminho do grpcurl, sobre a API v2 do `google.golang.org/protobuf` (descritores dinâmicos, sem stub gerado).

   ```yaml
   - grpc:
       method: order.OrderService/Lookup
       message: '{"id":"1"}'
       metadata: { authorization: "Bearer ${TOKEN}" }
       timeout: 10s
   ```

   A escolha por reflexão, e não por descriptor set, segue a superfície sem-código (ADR 0018): a pessoa não sobe artefato nenhum, o esquema vem do próprio alvo. O custo é exigir reflexão ligada no alvo — comum em dev e homologação, que é para onde o teste de carga aponta. A conexão e o descritor são resolvidos uma vez por alvo/método e reusados pelas iterações; método de streaming é recusado, porque o modelo de carga envia uma mensagem por iteração.

O código do gRPC vira classe de erro pela própria semântica: `Unauthenticated` para credencial, `PermissionDenied` para autorização, `DeadlineExceeded` para timeout, `Unavailable` para rede. Aprovar um erro gRPC como sucesso porque o transporte respondeu é o erro que o ADR 0003 §3 mais teme.

## Alternativas descartadas

- **Esperar os dois ficarem completos antes de mexer no código**: adiaria o WebSocket, que não custa dependência nenhuma, por causa do gRPC, que custa. Encenar entrega o que já dá para entregar.
- **gRPC com stub gerado por cenário**: exigiria o `.proto` e um passo de codegen no fluxo — atrito que a superfície "sem código" (ADR 0018) existe para não ter. A transcodificação por reflexão é o caminho escolhido.
- **WebSocket com biblioteca mais nova (coder/nhooyr)**: melhor lib, mas dependência nova para o mesmo resultado no escopo de carga. `x/net/websocket` já está no módulo e disca, envia e recebe — suficiente para o passo de carga.

## O que reabre esta decisao

- ~~Um alvo de produção sem reflexão precisar ser testado: aí entra o descriptor set como caminho alternativo.~~ **Entregue.** O passo `grpc` aceita `descriptorSet: arquivo.protoset` (um `FileDescriptorSet` de `protoc --descriptor_set_out --include_imports`); quando presente, os descritores vêm do arquivo em vez da reflexão. A reflexão segue o padrão quando a chave é omitida.
- Streaming bidirecional de verdade (server-stream, client-stream) precisar virar medição própria — hoje o modelo é uma mensagem por iteração, como o resto.
