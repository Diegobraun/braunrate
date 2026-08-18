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

A entrega é **encenada**, e a razão é o custo de dependência:

1. **WebSocket sai completo.** O transporte usa `golang.org/x/net/websocket`, que já está no grafo do módulo — nenhuma dependência nova. O step disca, envia a mensagem, e opcionalmente lê uma resposta:

   ```yaml
   - websocket:
       path: /stream
       send: '{"subscribe":"orders"}'
       awaitReply: true
       timeout: 10s
       headers: { Authorization: "Bearer ${TOKEN}" }
   ```

2. **gRPC sai com a superfície pronta e o transporte por vir.** O `Decode` valida o cenário inteiro — método, mensagem JSON, metadados — e o `Describe` mostra o que rodaria. O `Execute` recusa com mensagem clara enquanto o transporte não é compilado:

   ```yaml
   - grpc:
       method: order.OrderService/Lookup
       message: '{"id":"1"}'
       metadata: { authorization: "Bearer ${TOKEN}" }
       timeout: 10s
   ```

   O transporte gRPC exige `google.golang.org/grpc` mais o runtime de protobuf e reflexão de servidor para transcodar JSON sem stub gerado — peso de dependência e uma decisão de design (reflexão vs. descriptor set) que merecem o próprio passo. Registrar a superfície agora fixa o formato do cenário e a validação; o transporte é um follow-up focado que não mexe em nenhum arquivo de cenário.

A recusa do `Execute` do gRPC é **honesta, não silenciosa**: retorna `ErrConfig` dizendo que o passo foi declarado mas este build não traz o transporte, apontando este ADR. Um cenário gRPC valida e salva; ao rodar, diz exatamente o que falta em vez de aprovar um serviço que nunca foi tocado — o erro que o ADR 0003 §3 mais teme.

## Alternativas descartadas

- **Esperar os dois ficarem completos antes de mexer no código**: adiaria o WebSocket, que não custa dependência nenhuma, por causa do gRPC, que custa. Encenar entrega o que já dá para entregar.
- **gRPC com stub gerado por cenário**: exigiria o `.proto` e um passo de codegen no fluxo — atrito que a superfície "sem código" (ADR 0018) existe para não ter. A transcodificação por reflexão é o caminho, e é o que o follow-up decide.
- **WebSocket com biblioteca mais nova (coder/nhooyr)**: melhor lib, mas dependência nova para o mesmo resultado no escopo de carga. `x/net/websocket` já está no módulo e disca, envia e recebe — suficiente para o passo de carga.

## O que reabre esta decisao

- O transporte gRPC entrar: sai a recusa, entra a dependência, e este ADR ganha o registro do que foi decidido sobre reflexão vs. descriptor set.
- Streaming bidirecional de verdade (server-stream, client-stream) precisar virar medição própria — hoje o modelo é uma mensagem por iteração, como o resto.
