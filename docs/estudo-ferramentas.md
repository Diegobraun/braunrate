# Estudo — Ferramentas de teste de performance

> Base para decidir o escopo funcional do **braunload**.
> Levantamento feito em agosto de 2026.

---

## 1. Objetivo

Mapear o que as ferramentas existentes fazem, onde elas realmente doem, e definir o conjunto mínimo de capacidades que o braunload precisa ter para substituir o JMeter num time real — sem virar "mais um clone de k6".

Este documento responde três perguntas:
1. Quais capacidades existem no mercado e quais são inegociáveis?
2. Onde há lacuna genuína, e não apenas preferência de sintaxe?
3. Qual o backlog priorizado que sai disso?

---

## 2. Panorama das ferramentas

| Ferramenta | Motor | Autoria | Modelo de execução | Ponto forte | Ponto fraco |
|---|---|---|---|---|---|
| **JMeter** | JVM, thread por VU | GUI → XML `.jmx` | Fechado (thread group) | Amplitude de protocolo; ecossistema de plugins; 20+ anos de uso | Formato ilegível em diff; custo por VU; GUI que não serve para rodar; correlação trabalhosa |
| **Gatling** | JVM, Netty assíncrono | DSL Java/Kotlin/Scala | Aberto e fechado | Vazão por injetor; relatório HTML excelente; modelo aberto por padrão | Curva de DSL; distribuído só no Enterprise |
| **k6** | Go, goroutines | JS/TS | Aberto e fechado (executors) | Binário único; thresholds como gate; integração Grafana/Prometheus | Protocolos além de HTTP exigem recompilar binário com extensão |
| **Locust** | Python, gevent | Python puro | Fechado | Flexibilidade total; qualquer protocolo com cliente Python | Você escreve a integração; vazão menor |
| **Artillery** | Node | YAML + JS | Aberto | YAML declarativo simples; execução serverless | Ecossistema menor |
| **wrk2 / Vegeta** | C / Go | CLI | Aberto, taxa constante | Medição de latência honesta | Só HTTP, sem cenário |
| **Hyperfoil** | JVM | YAML | Aberto | Detecta back-pressure e reporta omissão coordenada | Pouco adotado, curva alta |
| **Taurus** | wrapper | YAML | delega | Unifica JMeter/Gatling/Locust sob um YAML só | Camada a mais; herda os limites de quem executa |

**Leitura**: o mercado se dividiu entre *amplitude de protocolo com ergonomia ruim* (JMeter) e *ergonomia boa com amplitude via extensão* (k6, Gatling). Ninguém entregou os dois com autoria acessível a QA não-programador.

---

## 3. Eixos de capacidade

### 3.1 Modelo de execução — o eixo mais importante

Existem dois modelos, e a diferença não é detalhe:

- **Fechado (closed)**: N usuários virtuais em laço; cada um só faz a próxima requisição depois que a anterior responde. Se o servidor degrada, a carga aplicada **cai**. É o modelo do JMeter e do Locust.
- **Aberto (open)**: chegam X requisições por segundo independentemente do que o servidor faz. É como tráfego real de internet se comporta.

O modelo fechado produz a patologia conhecida como **omissão coordenada**: quando o servidor trava, o gerador para de enviar, os pedidos que deveriam ter ocorrido nunca são registrados, e o p99 sai artificialmente bom. Casos documentados mostram teste verde com p99 de dezenas de milissegundos e produção com p99 na casa dos segundos.

**Implicação para o braunload**: modelo aberto por padrão, medindo latência a partir do instante em que a requisição *deveria* ter partido, e não de quando partiu. Histograma HDR para preservar precisão nas caudas. Detecção e aviso explícito de back-pressure — se o gerador não conseguiu manter a taxa alvo, isso precisa aparecer no relatório em vermelho, não ser omitido.

### 3.2 Autoria do teste

| Abordagem | Quem atende | Custo |
|---|---|---|
| GUI → XML (JMeter) | QA sem código | Diff ilegível, merge impossível, review inviável |
| DSL em código (Gatling, k6, Locust) | Dev | QA sem código fica de fora |
| YAML declarativo (Artillery, Taurus, Hyperfoil) | Ambos, no caso simples | Teto baixo: caso complexo não cabe |

**Implicação**: os dois públicos exigem **YAML declarativo para o caso comum + escape para DSL quando necessário**, com o mesmo motor por baixo e sem reescrever o cenário ao migrar de um para o outro. Este é o requisito estrutural que mais influencia a arquitetura.

### 3.3 Protocolos

JMeter cobre nativamente HTTP/HTTPS, JDBC, JMS, FTP, LDAP, SMTP, TCP e GraphQL, com Kafka via plugin — é a maior amplitude entre as ferramentas abertas. Gatling cobre HTTP, WebSocket, SSE, gRPC, JMS e MQTT no núcleo, com Kafka por plugin da comunidade. k6 cobre HTTP, WebSocket, gRPC e browser, e o resto (SQL, Kafka, AMQP, Redis) por extensão compilada no binário.

**Implicação**: priorizar por frequência de uso real, não por amplitude:

| Prioridade | Protocolo | Motivo |
|---|---|---|
| 1 | HTTP/HTTPS + REST | 90% dos casos |
| 1 | GraphQL | Tratamento próprio, não é "HTTP com body" — ver §4 |
| 2 | Kafka (produzir e consumir) | Necessidade declarada; modelo de medição próprio — ver §5 |
| 2 | RabbitMQ/AMQP | Idem |
| 3 | gRPC | Crescente em microsserviço interno |
| 3 | JDBC | Útil para preparar massa e validar efeito |
| 4 | WebSocket, SSE, MQTT | Nicho |
| Fora | LDAP, FTP, SMTP, JMS clássico | Legado; não justificam o custo |

### 3.4 Correlação e parametrização

É a operação mais frequente em teste de carga e a que mais dói no JMeter: extrair token de uma resposta e usar na próxima exige post-processor, variável, escopo e às vezes script. É reconhecidamente a causa número um de script que quebra na segunda execução.

**Implicação**: correlação precisa ser **uma linha**. Um passo declara o que captura (`captura: token = $.access_token`) e o valor fica disponível nos passos seguintes por nome. Suporte a JSONPath, regex e header. E um caso de primeira classe: **fluxo de autenticação** — obter token uma vez por usuário virtual, renovar quando expirar, sem o autor precisar orquestrar isso.

### 3.5 Asserções, SLO e gate de CI

k6 popularizou thresholds que falham o processo; Gatling tem assertions equivalentes; JMeter exige montar a verificação por fora ou usar plugin.

**Implicação**: SLO declarado junto do cenário, avaliado no fim, determinando o código de saída. Sem isso não existe gate de CI honesto. Precisa distinguir **falha de SLO** (p95 estourou) de **falha funcional** (500, corpo inválido) — são coisas diferentes e o relatório deve separá-las.

### 3.6 Perfis de carga

Necessários: rampa, patamar constante, pico, degrau, taxa de chegada constante, e execução por duração ou por número de iterações. Gatling e k6 já expressam isso bem; o JMeter exige combinar thread group com timers.

### 3.7 Dados de teste

CSV é o mínimo (o `CSV Data Set Config` do JMeter é dos elementos mais usados). Além dele: geração sintética com semente fixa para reprodutibilidade, política de consumo (sequencial, aleatório, circular, único por VU) e dado por ambiente.

### 3.8 Execução distribuída

JMeter tem modo master/worker próprio. k6 distribui via operator Kubernetes com CRD `TestRun`, definindo paralelismo e saída para Prometheus. Gatling só distribui no Enterprise.

**Implicação**: não é escopo inicial, mas a arquitetura precisa não impedir. Concretamente: agregação de métricas deve ser possível a partir de histogramas parciais mergeáveis — HDR histogram tem essa propriedade, média e percentil pré-calculado não têm. Decidir isso cedo evita reescrita.

### 3.9 Extensibilidade

k6 exige recompilar o binário para adicionar protocolo — barreira real. Gatling, por rodar na JVM, permite trazer qualquer biblioteca Java. JMeter tem 1.000+ plugins.

**Implicação**: SPI de protocolo carregada em runtime, sem recompilar o motor. Um protocolo novo implementa uma interface, declara suas métricas e seus campos de configuração, e o YAML passa a aceitá-lo.

---

## 4. GraphQL — por que precisa de tratamento próprio

Tecnicamente é POST num endpoint único. Na prática, tratá-lo como HTTP genérico produz medição enganosa:

- **Um endpoint, N perfis de custo.** O cliente escolhe campos, profundidade e paginação; a mesma URL esconde desde uma consulta trivial até uma que dispara dezenas de consultas ao banco. Agrupar tudo sob `POST /graphql` no relatório é inútil.
- **Erro com HTTP 200.** GraphQL devolve 200 com erro dentro do corpo, em `errors`. A regra "2xx é sucesso" simplesmente não vale — uma ferramenta que ignora isso reporta 0% de erro num teste que falhou inteiro.
- **N+1 em resolver.** Uma requisição vira dezenas de chamadas ao banco. A métrica que importa não é só latência do endpoint, é como a *forma* da consulta muda o custo no backend.
- **Mix de operações.** O teste precisa refletir a distribuição real de operações da produção, não repetir a mesma consulta.

**Requisitos específicos**:
- Passo do tipo `graphql` com `operationName`, `query` e `variables` separados.
- **Agrupamento de métrica por `operationName`**, nunca por URL. Este é o requisito central.
- Asserção nativa sobre `errors[]` no corpo, com falha por padrão quando o array não está vazio.
- Suporte a mix ponderado de operações (60% consulta leve, 30% pesada, 10% mutação).
- Desejável: alertar quando a profundidade da consulta ultrapassa um limite configurado.

---

## 5. Mensageria — o modelo de medição muda

Kafka e RabbitMQ não cabem no molde requisição→resposta. Extensões existentes (xk6-kafka, plugin Kafka do Gatling) resolvem parcialmente, e o Gatling tem inclusive um modo request-reply para Kafka.

**Três formas distintas de teste, cada uma com métrica própria:**

| Forma | O que mede | Métricas |
|---|---|---|
| **Produzir** | Capacidade de ingestão | msgs/s, latência do ack, taxa de erro, lag no broker |
| **Consumir** | Capacidade de processamento do SUT | msgs/s consumidas, evolução do lag, tempo até drenar backlog |
| **Request-reply** | Fluxo assíncrono ponta a ponta | latência correlacionada entre publicação e resposta, taxa de correlação perdida |

**O cenário mais valioso — e que nenhuma ferramenta faz bem — é o híbrido**: publica no Kafka, espera o efeito aparecer via HTTP (polling com timeout), e mede a cadeia inteira. É exatamente o teste que um sistema orientado a eventos precisa e que hoje ninguém escreve porque é trabalhoso demais.

**Requisitos**:
- Passos `kafka.produzir`, `kafka.consumir`, `amqp.publicar`, `amqp.consumir`.
- **Passo `aguardar`**: condição com timeout, medindo o tempo até ela ser satisfeita. É o que viabiliza o cenário híbrido.
- Correlação por chave entre publicação e efeito observado.
- Serialização: JSON, string, binário; Avro e Schema Registry desejáveis.
- Lag do consumer group como métrica de primeira classe, não como observação externa.

---

## 6. Relatórios

O relatório é o produto final. Onde as ferramentas estão hoje: Gatling gera HTML estático com percentis e distribuição, reconhecidamente o melhor da categoria; JMeter gera HTML a partir do `.jtl`; k6 imprime resumo no terminal e exporta para Prometheus/Grafana, permitindo acompanhamento em tempo real durante a execução.

**O que o braunload precisa entregar:**

**Durante a execução**
- Saída de terminal viva e legível: taxa atual, latência corrente, erros, tempo restante.
- Aviso imediato quando a taxa alvo não está sendo mantida (back-pressure), com a causa provável.

**Ao final — relatório HTML autocontido**
- Um arquivo, sem servidor, sem dependência externa, abre no navegador e pode ser anexado num ticket.
- **Distribuição completa de latência** (p50, p75, p90, p95, p99, p99.9, máximo), não média. Média em latência esconde exatamente o que importa.
- Latência ao longo do tempo, sobreposta à carga aplicada — é assim que se enxerga o ponto de degradação.
- Erros classificados: falha de rede, timeout, status HTTP, asserção funcional, `errors` do GraphQL, correlação perdida.
- **Recorte por passo e por operação**, não só agregado do cenário.
- Bloco de ambiente: máquina do gerador, versão, alvo, hora, duração, semente de dados.
- **Veredito de SLO** no topo: passou ou não, e qual limiar estourou.
- Aviso honesto se houve back-pressure ou se o gerador saturou — resultado sob saturação do próprio gerador não vale.

**Comparação entre execuções**
- Diff entre duas execuções, mostrando regressão por passo. É o que transforma teste de carga em regressão contínua, e quase nenhuma ferramenta aberta faz bem.

**Exportação**
- JSON estruturado (para automação), CSV bruto (para análise própria), e saída para Prometheus/OTLP para quem já tem stack de observabilidade.
- Sumário em markdown pronto para comentário de pull request.

---

## 7. Lacunas do mercado — onde há espaço real

1. **Autoria em dois níveis com um motor só.** YAML para o caso comum, DSL para o complexo, sem reescrever. Ninguém entrega isso bem.
2. **Migração do acervo `.jmx`.** É o que trava todo time que quer sair do JMeter. Um importador do subconjunto comum é o que faz alguém trocar de ferramenta de verdade.
3. **Cenário híbrido HTTP + mensageria com passo de espera.** Necessidade concreta de sistema orientado a eventos, mal atendida hoje.
4. **GraphQL como cidadão de primeira classe**, com métrica por operação e erro em corpo 200.
5. **Honestidade de medição como padrão**, não como configuração avançada: modelo aberto, HDR, aviso de back-pressure.
6. **Comparação entre execuções** embutida.

---

## 8. Backlog priorizado

**Essencial (v1 não existe sem isso)**
- Motor com modelo aberto, HDR histogram, detecção de back-pressure
- HTTP/REST com todos os verbos, headers, corpo, autenticação, cookies, redirect
- GraphQL com métrica por `operationName` e asserção sobre `errors`
- Correlação em uma linha (JSONPath, regex, header) e variáveis por ambiente
- Fluxo de autenticação com renovação de token
- Dados: CSV com política de consumo e geração sintética com semente
- Perfis de carga: rampa, patamar, pico, taxa constante
- Asserções funcionais e SLO com código de saída
- Relatório HTML autocontido + JSON + resumo no terminal
- YAML declarativo como formato primário

**Importante (v1.x)**
- Kafka produzir/consumir e AMQP publicar/consumir
- Passo `aguardar` com timeout, viabilizando o cenário híbrido
- DSL Java como segundo nível de autoria, mesmo motor
- Importador de `.jmx` para o subconjunto comum
- Comparação entre execuções
- Exportação Prometheus/OTLP

**Desejável (depois)**
- gRPC, JDBC, WebSocket
- Execução distribuída
- SPI de protocolo carregada em runtime
- Avro + Schema Registry
- Gravação de tráfego para gerar cenário inicial

**Fora de escopo — declarar explicitamente**
- Motor de browser real (é outro produto)
- Nuvem gerenciada, dashboard multiusuário
- LDAP, FTP, SMTP, JMS clássico
- Competir em vazão bruta com wrk

---

## 9. Decisões em aberto para a fase de arquitetura

1. **Linguagem.** JVM com virtual threads permite código de cenário linear e legível com concorrência alta — tese central do projeto. Go dá binário único e distribuição trivial. A escolha determina tudo o mais e precisa de ADR com experimento, não com opinião.
2. **YAML e DSL sobre o mesmo modelo.** O YAML desserializa para o mesmo objeto que a DSL constrói, ou são caminhos separados? A primeira opção é mais difícil e é a única que sustenta a promessa dos dois públicos.
3. **Onde a medição acontece.** Instrumentar no motor, e não em cada protocolo, é o que garante que Kafka e HTTP produzam métricas comparáveis.
4. **Distribuição.** Não implementar agora, mas garantir que os agregados sejam mergeáveis desde o início.
5. **Extensão de protocolo.** SPI em runtime desde a v1, ou acoplado até estabilizar a interface?

---

## 10. Referências

- Apache JMeter — Manual do usuário e referência de componentes: https://jmeter.apache.org/usermanual/component_reference.html
- Grafana k6 — Protocolos e extensões: https://k6.io/docs/using-k6/protocols
- Grafana k6 — Execução distribuída com operator: https://grafana.com/docs/k6/latest/testing-guides/running-distributed-tests/
- xk6-kafka: https://github.com/mostafa/xk6-kafka
- Gatling — Documentação e SDK Java: https://docs.gatling.io/ e https://gatling.io/java
- Plugin Kafka para Gatling: https://github.com/galax-io/gatling-kafka-plugin
- Gil Tene — HdrHistogram e omissão coordenada: https://hdrhistogram.org
- Omissão coordenada, demonstração reproduzível: https://idle-ti.me/blog/coordinated-omission/
- Hyperfoil sobre omissão coordenada: https://redhatperf.github.io/post/coordinated-omission/
- Taurus: https://gettaurus.org/docs/K6/
- jmeter-java-dsl e conversor `jmx2dsl`: https://abstracta.github.io/jmeter-java-dsl/guide/
