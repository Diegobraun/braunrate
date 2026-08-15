# ADR 0001 — Linguagem e runtime

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 0
- **Relacionados**: [medicoes da Fase 0](../medicoes-fase0.md), [estudo de ferramentas](../estudo-ferramentas.md) §3.1, §3.9, §9.1

## Contexto

O estudo (§9.1) deixa a escolha em aberto entre JVM com virtual threads — que permite cenario linear e bloqueante com concorrencia alta — e Go, que da binario unico e distribuicao trivial, e exige "ADR com experimento, nao com opiniao".

Dois prototipos descartaveis de ~200 linhas resolveram o mesmo problema minimo (chegada aberta, HDR histogram, latencia contada do instante agendado, deteccao de back-pressure) contra o mesmo alvo, na mesma maquina. Metodologia, ambiente e limites do experimento em [medicoes-fase0.md](../medicoes-fase0.md).

## Numeros que decidiram

| Criterio | Java 25 (virtual threads) | Go 1.26 (goroutines) | Razao |
|---|---|---|---|
| Taxa maxima sustentada | **10.000/s** (3/3 reps) — 20.000/s so 1/3 | **30.000/s** (4/5 reps) — 40.000/s 2/5 | 3x |
| Desvio de agendamento p99 a 10.000/s | 3.077 us (G1) / 255 us (ZGC) | 3 us | 85x a 1.000x |
| Usuarios virtuais simultaneos | 5.250 estaveis | 10.121 estaveis | ~2x |
| RSS em repouso | 147,5 MB | 11,0 MB | 13x |
| RSS sob carga a 10.000/s | 596,8 MB (G1) / 2.003,8 MB (ZGC) | 30,1 MB | 20x a 66x |
| Startup ate apto a gerar carga | 587,2 ± 27,8 ms | 42,8 ± 0,9 ms | 14x |
| Custo marginal de CPU por requisicao | 122 us | 76 us | 1,6x |
| Modo de falha ao saturar | espiral: 364 mil a 970 mil requisicoes em voo, 686 mil `Too many open files`, RSS 1,6 GB | degradacao com 0 erros ate o teto; acima dele a execucao as vezes nao conclui, sem causa identificada | qualitativo |

O numero mais importante nao e a vazao: e o **desvio de agendamento**. Uma ferramenta cuja tese e "latencia contada do instante agendado" nao pode errar o instante agendado em 3 ms enquanto mede um alvo de 5 ms — o erro do instrumento fica na mesma ordem de grandeza do fenomeno medido.

**A causa do desvio no Java e pausa de GC, e ZGC corrige a maior parte dela**: a 5.000/s o p99 cai de 1.218 us para 5 us, e a 10.000/s de 2.936 us para 255 us. Isso reduz a vantagem do Go nesse eixo de ~1.000x para ~85x, e cobra RSS: 2.003 MB sob carga a 10.000/s, contra 30 MB do Go. Ou seja: **Java com ZGC seria viavel**, e a decisao nao pode se apoiar so nesse eixo.

O segundo criterio, e onde a diferenca nao tem ajuste que resolva, e o **modo de falha**. Quando o Java nao acompanha, ele nao degrada: entra em espiral, porque cada requisicao atrasada segura uma conexao, e `java.net.http.HttpClient` em HTTP/1.1 abre conexao nova para cada requisicao concorrente. O resultado e exaustao de descritores de arquivo e uma execucao inteira invalidada. Para uma ferramenta de medicao isso e pior do que ser lenta.

O prototipo Go tambem tem um limite mal explicado: acima de ~30.000/s, algumas execucoes travam sem produzir saida. Isso esta declarado em [medicoes-fase0.md](../medicoes-fase0.md) §3 e nao foi resolvido — nao muda a comparacao, porque acontece a 3x a taxa em que o Java ja colapsou, mas entra como risco conhecido para a Fase 1.

## Decisao

**Go.**

Peso de cada criterio, incluindo os que nao sao medicao:

| Criterio | Peso | Vencedor | Observacao |
|---|---|---|---|
| Precisao de agendamento | alto | Go | e a tese do produto |
| Modo de falha sob saturacao | alto | Go | espiral do Java invalida a execucao |
| Distribuicao do binario | alto | Go | binario estatico por plataforma; o alvo e CI e maquina de QA, nao servidor com JVM instalada |
| Custo de recursos (RSS, startup) | medio | Go | importa em CI e em execucao distribuida futura |
| Vazao sustentada | medio | Go | 3x com cliente padrao dos dois lados |
| Ergonomia da DSL para o publico dev | medio | **Java** | DSL fluente com autocomplete e integracao JUnit e melhor em Java; em Go a DSL vira pacote com builders e `go test` |
| Ecossistema Kafka e AMQP | medio | **Java** | clientes oficiais e maduros; Go tem `franz-go` e `rabbitmq/amqp091-go`, suficientes, mas Avro e Schema Registry sao mais fracos |
| SPI de protocolo em runtime | baixo | **Java** | `ServiceLoader` resolve; `plugin` do Go e inviavel na pratica — protocolo entra compilado, como no k6 |
| Custo de GraalVM | — | — | `native-image` corrigiria startup e RSS, mas nao o desvio de agendamento, e adiciona toolchain, tempo de build e configuracao de reflexao para clientes Kafka |

Go perde em tres criterios reais, todos de custo de desenvolvimento nosso. Ganha nos criterios que o usuario sente: precisao da medicao, comportamento sob saturacao, e um binario que roda sem instalar runtime.

### O que aceitamos junto com a decisao

1. **Protocolo novo entra compilado**, nao carregado em runtime. O estudo (§3.9) critica exatamente isso no k6. Mitigacao: o registro de protocolos e uma interface pequena e estavel (ADR 0003 §3), e `braunrate` pode ser reconstruido com protocolos extras sem tocar no motor. SPI em runtime volta a ser avaliada se virar demanda real.
2. **DSL em Go e menos elegante** que uma DSL Java. Mitigacao: o YAML e a interface principal (ADR 0002); a DSL existe para o caso complexo e ganha tipagem e `go test`, nao fluencia maxima.
3. **Avro e Schema Registry** ficam mais caros. Estao em "desejavel (depois)" no backlog do estudo — nao bloqueia a v1.
4. **hdrhistogram-go nao e seguro para escrita concorrente.** O prototipo mostrou o custo do mutex. No produto, a instrumentacao usa histograma por worker com merge periodico — que e exatamente o que a arquitetura mergeavel do ADR 0003 ja exige.

### O que nao pesou

- Preferencia pessoal por qualquer das duas.
- "Java tem mais bibliotecas": verdadeiro e irrelevante para o escopo priorizado (HTTP, GraphQL, Kafka, AMQP), todos bem cobertos em Go.
- Vazao bruta maxima: o estudo declara explicitamente que competir com wrk esta fora de escopo.

## Alternativas descartadas

- **Java 25 com virtual threads, configuracao padrao (G1)**: perdeu no criterio que define o produto (precisao de agendamento) e tem modo de falha destrutivo.
- **Java 25 com ZGC e cliente HTTP alternativo (Netty)**: e a alternativa seria, e a medicao mostra que o eixo de agendamento seria recuperado (255 us no p99 a 10.000/s). Descartada por tres motivos, nesta ordem: o resultado passa a depender de ajuste fino de GC e de troca do cliente padrao — uma ferramenta de medicao precisa ser correta sem isso; o RSS sobe para 2 GB no mesmo ponto em que o Go usa 30 MB; e a distribuicao continua exigindo runtime instalado.
- **Java com GraalVM native-image**: resolve startup e RSS, nao resolve agendamento, e cobra toolchain e configuracao de reflexao.
- **Rust**: nao foi medido. Ganharia em recursos e precisao, perderia em velocidade de desenvolvimento e no ecossistema de clientes Kafka/AMQP maduro. Nao foi considerado porque o estudo (§9.1) delimitou a decisao a JVM e Go — registrar aqui que a alternativa existe e nao foi avaliada e mais honesto do que fingir que foi descartada por merito.

## Consequencias

- O binario e distribuido por plataforma e como imagem Docker; nao ha dependencia de runtime instalado.
- O agendador usa espera hibrida com espera ativa final; o custo e aproximadamente um nucleo dedicado ao agendador, medido e declarado no relatorio.
- A instrumentacao usa histograma por worker com merge, nunca um histograma global sob mutex.
- Protocolo novo exige rebuild; isso vai declarado no README para nao surpreender quem vem do JMeter.
