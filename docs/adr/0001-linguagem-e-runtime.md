# ADR 0001 — Linguagem e runtime

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 0
- **Relacionados**: [medicoes da Fase 0](../medicoes-fase0.md), [estudo de ferramentas](../estudo-ferramentas.md) §3.1, §3.9, §9.1

## Contexto

O estudo (§9.1) deixa a escolha em aberto entre JVM com virtual threads — que permite cenario linear e bloqueante com concorrencia alta — e Go, que da binario unico e distribuicao trivial, e exige "ADR com experimento, nao com opiniao".

Dois prototipos descartaveis de ~200 linhas resolveram o mesmo problema minimo (chegada aberta, HDR histogram, latencia contada do instante agendado, deteccao de back-pressure) contra o mesmo alvo, na mesma maquina. Metodologia, ambiente e limites do experimento em [medicoes-fase0.md](../medicoes-fase0.md).

## Numeros medidos

A tabela e o que a bateria produziu. Quais destes numeros efetivamente sustentam a decisao, e quais nao sustentam, esta em [Decisao](#decisao) — a primeira versao deste ADR dava peso alto a criterios que nao aguentam o peso.

| Criterio | Java 25 (virtual threads) | Go 1.26 (goroutines) | Razao |
|---|---|---|---|
| Taxa maxima sustentada | **10.000/s** (3/3 reps) — 20.000/s so 1/3 | **30.000/s** (4/5 reps) — 40.000/s 2/5 | 3x |
| Desvio de agendamento p99 a 10.000/s | 3.077 us (G1) / 255 us (ZGC) | 3 us | 85x a 1.000x |
| Usuarios virtuais simultaneos | 5.250 estaveis | 10.121 estaveis | ~2x |
| RSS em repouso | 147,5 MB | 11,0 MB | 13x |
| RSS sob carga a 10.000/s | 596,8 MB (G1) / 2.003,8 MB (ZGC) | 30,1 MB | 20x a 66x |
| Startup ate apto a gerar carga | 587,2 ± 27,8 ms | 42,8 ± 0,9 ms | 14x |
| Custo marginal de CPU por requisicao | 122 us | 76 us | 1,6x |
| Modo de falha ao saturar | espiral: 364 mil a 970 mil requisicoes em voo, 686 mil `Too many open files`, RSS 1,6 GB | degradacao com 0 erros ate o teto; acima dele a execucao as vezes nao conclui, sem causa identificada | comparacao invalida: nenhum dos dois prototipos tinha limite de voo explicito |

### Qual coletor produziu cada numero

A bateria de taxa rodou o Java com o coletor **padrao do JDK 25, o G1**. A bateria de coletor cobriu apenas **5.000/s e 10.000/s** — nunca as taxas em que o Java colapsou. Logo: **a taxa maxima sustentada do Java (10.000/s) esta subestimada**, porque foi medida na configuracao com pior desvio de agendamento e sem ZGC. Nao refizemos a bateria: a decisao nao muda e o custo nao se paga. Fica registrado que o numero e um piso, nao um teto.

### O teto de taxa mede o par gerador+alvo, nao o gerador

O diagnostico a 40.000/s mostra o processo alvo consumindo **214,9% de CPU** (2,1 dos 10 nucleos), zero erro e 211 requisicoes em voo no gerador. Ou seja: nas taxas altas quem estava perto da saturacao era o alvo local, que divide a mesma maquina. **Os valores de 10.000/s e 30.000/s medem o par, nao a capacidade do gerador.** Qualquer numero de taxa maxima desta fase deve ser lido assim; corrigir isso e item da Fase 1 (alvo externo ou muito mais barato).

## Decisao

**Go.**

### Os dois criterios que sustentam a decisao

| Criterio | Numero | Por que e material |
|---|---|---|
| **RSS sob carga** | 30 MB no Go contra 597 MB no Java (G1) a 10.000/s; 2.004 MB com ZGC | O gerador roda em runner de CI com limite de memoria e em pod de cluster. Um gerador que pede 2 GB para gerar 10 mil requisicoes por segundo restringe onde o teste pode rodar — e isso muda o produto, nao so o benchmark. |
| **Binario unico estatico** | um arquivo por plataforma, sem runtime instalado | Para o publico de QA, instalar e baixar um arquivo. Vale mais que qualquer numero de vazao: e a diferenca entre a ferramenta ser adotada e ficar num README. |

Nada alem disso sustenta a escolha. O que segue e a lista do que **nao** sustenta, apesar de ter aparecido com peso alto na primeira versao deste ADR.

### O que NAO sustenta a decisao

| Criterio | Numero medido | Por que nao conta |
|---|---|---|
| **Startup** | 587 ms contra 43 ms | **Nao-criterio.** Um teste de carga roda minutos. Meio segundo no inicio de uma execucao de 5 minutos e 0,17% do tempo. Nao muda nada para ninguem. |
| **Precisao de agendamento** | 3.077 us (G1) / **255 us (ZGC)** contra 3 us | **Nao-criterio apos ZGC.** 255 us e 0,2% de um sinal medido em dezenas de milissegundos. Nao e perceptivel pelo usuario e nao move percentil de relatorio. O eixo so parecia decisivo enquanto o Java estava com G1. |
| **Modo de falha sob saturacao** | 364 mil em voo e 686 mil `Too many open files` no Java, contra 211 em voo no Go | **Possivel artefato dos prototipos, nao evidencia.** O prototipo Java nao tinha limite de concorrencia em voo; o Go, na pratica, se manteve em 8.031 goroutines e 211 em voo. Comparamos dois harness diferentes, nao dois runtimes. Sem limite de voo dos dois lados, esse numero nao prova nada sobre a JVM. |
| **Taxa maxima sustentada** | 10.000/s contra 30.000/s | Limitado pelo alvo, como registrado acima; e medido no Java so com G1. Serve de sinal, nao de prova. |

### Criterios em que Go perde, e que aceitamos

| Criterio | Vencedor | Custo aceito |
|---|---|---|
| Ergonomia da DSL para o publico dev | Java | DSL fluente com autocomplete e integracao JUnit e melhor em Java. Em Go a DSL vira pacote com builders e `go test`. Ver [ADR 0002](0002-modelo-de-cenario.md) §6 para quem e o publico de cada construtor. |
| Ecossistema Kafka e AMQP | Java | `franz-go` e `rabbitmq/amqp091-go` dao conta do escopo priorizado; Avro e Schema Registry sao mais fracos. Ver [ADR 0004](0004-extensao-de-protocolo.md). |
| Extensao de protocolo em runtime | Java | `ServiceLoader` resolveria; em Go o protocolo entra compilado. Herdamos a friccao do k6 e declaramos isso. Ver [ADR 0004](0004-extensao-de-protocolo.md). |

### A tese de concorrencia barata continua valendo — por outro caminho

A tese de "codigo de cenario linear e bloqueante com concorrencia alta" nasceu como argumento **contra Gatling e contra codigo reativo**, nao contra Go. Escrever cenario com callback, `Flux` ou `CompletableFuture` e o que torna teste de carga ilegivel para quem nao e especialista — e virtual threads eram a resposta da JVM a isso.

Esse sempre foi o modelo nativo do Go: goroutine com codigo sequencial e bloqueante e o jeito normal de escrever, nao um recurso novo. **Escolher Go satisfaz a tese por outro caminho, nao a abandona.** O que muda e de onde vem a concorrencia barata, nao a forma do codigo do cenario.

### O que aceitamos junto com a decisao

1. **Protocolo novo entra compilado.** Registrado em [ADR 0004](0004-extensao-de-protocolo.md).
2. **Avro e Schema Registry sao mais caros em Go.** Estao em "desejavel (depois)" no backlog do estudo — seguimos.
3. **hdrhistogram-go nao e seguro para escrita concorrente.** O prototipo mostrou o custo do mutex. No produto, a instrumentacao usa histograma por worker com merge periodico — que e o que a arquitetura mergeavel do [ADR 0003](0003-modelo-de-execucao-e-metrica.md) ja exige.

### O que nao pesou

- Preferencia pessoal por qualquer das duas.
- "Java tem mais bibliotecas": verdadeiro e irrelevante para o escopo priorizado.
- Vazao bruta maxima: competir com wrk esta fora de escopo por decisao do estudo.

## Alternativas descartadas

- **Java 25 com virtual threads, configuracao padrao (G1)**: descartado por RSS (597 MB contra 30 MB) e por exigir runtime instalado. Nao por agendamento nem por modo de falha — esses dois nao aguentam o peso, como registrado acima.
- **Java 25 com ZGC**: e a alternativa seria. Recupera o agendamento (255 us no p99 a 10.000/s, irrelevante para o usuario) e piora exatamente o criterio que decide: 2.004 MB de RSS sob carga a 10.000/s, contra 30 MB do Go. Somado a distribuicao com runtime, e o que a descarta.
- **Java com GraalVM native-image**: resolve startup e RSS, nao resolve agendamento, e cobra toolchain e configuracao de reflexao.
- **Rust**: nao foi medido. Ganharia em recursos e precisao, perderia em velocidade de desenvolvimento e no ecossistema de clientes Kafka/AMQP maduro. Nao foi considerado porque o estudo (§9.1) delimitou a decisao a JVM e Go — registrar aqui que a alternativa existe e nao foi avaliada e mais honesto do que fingir que foi descartada por merito.

## Consequencias

- O binario e distribuido por plataforma e como imagem Docker; nao ha dependencia de runtime instalado.
- O agendador usa espera hibrida com espera ativa final; o custo e aproximadamente um nucleo dedicado ao agendador, medido e declarado no relatorio.
- A instrumentacao usa histograma por worker com merge, nunca um histograma global sob mutex.
- Protocolo novo exige rebuild; declarado no README como limitacao conhecida ([ADR 0004](0004-extensao-de-protocolo.md)).

### O que a Fase 1 precisa corrigir desta medicao

Tres itens, todos consequencia direta das ressalvas acima:

1. **Alvo externo ou muito mais barato.** Enquanto o alvo consome 2,1 nucleos da mesma maquina, o teto medido e do par, nao do gerador. Sem isso nao existe numero confiavel acima de ~30.000/s.
2. **Limite de requisicoes em voo por construcao no motor**, e nos dois lados de qualquer comparacao futura. O comportamento na borda tem que ser resultado de teste, nao acidente de prototipo — foi essa ausencia que invalidou o criterio de modo de falha.
3. **Reproduzir o travamento do Go acima de 30.000/s** com o agendador de verdade e o alvo novo. Se nao reproduzir, registrar; se reproduzir, investigar antes de seguir.

## Nota de 2026-08-16

Os prototipos e as medicoes brutas que sustentam esta decisao sairam da raiz do repositorio na reorganizacao da Fase 7 e estao preservados na tag [`fase-0-prototipos`](https://github.com/Diegobraun/braunrate/tree/fase-0-prototipos). Os numeros e a metodologia continuam em [medicoes-fase0.md](../medicoes-fase0.md).
