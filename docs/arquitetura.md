# Arquitetura — esboco da Fase 0

Esboco, nao contrato. O que aqui esta fixo sao as fronteiras que os ADRs [0002](adr/0002-modelo-de-cenario.md) e [0003](adr/0003-modelo-de-execucao-e-metrica.md) tornam obrigatorias: um modelo de cenario unico, medicao no motor, agregados mergeaveis.

## Componentes

```mermaid
flowchart TB
    subgraph autoria["Autoria"]
        yaml["YAML declarativo"]
        dsl["DSL"]
        jmx["Importador .jmx"]
    end

    modelo["Modelo de cenario<br/>(arvore imutavel, serializavel)"]
    validador["Validador<br/>(referencias, SLO, dados)"]

    subgraph motor["Motor"]
        plano["Plano de carga<br/>rampa, patamar, pico, taxa constante"]
        agendador["Agendador de chegada aberta<br/>instante agendado e fonte da verdade"]
        executor["Executor de passo<br/>uma unidade de concorrencia por requisicao"]
        contexto["Contexto de execucao<br/>variaveis, capturas, dados, sessao"]
        instrumentacao["Instrumentacao<br/>HDR + contadores + series"]
    end

    subgraph protocolos["Protocolos"]
        http["HTTP/REST"]
        gql["GraphQL"]
        kafka["Kafka"]
        amqp["AMQP"]
        aguardar["aguardar (condicao + polling)"]
    end

    resultado["Documento de resultado<br/>versionado e mergeavel"]

    subgraph saidas["Saidas"]
        terminal["Terminal vivo"]
        html["HTML autocontido"]
        json["JSON / CSV"]
        prom["Prometheus / OTLP"]
        pr["Sumario markdown"]
        diff["Comparacao entre execucoes"]
    end

    yaml --> modelo
    dsl --> modelo
    jmx --> modelo
    modelo --> validador --> motor
    plano --> agendador --> executor
    executor <--> contexto
    executor --> protocolos
    executor --> instrumentacao
    instrumentacao --> resultado
    instrumentacao -.tempo real.-> terminal
    resultado --> html
    resultado --> json
    resultado --> prom
    resultado --> pr
    resultado --> diff
```

Duas fronteiras valem repetir:

- **O protocolo nao mede.** Ele recebe passo e contexto, devolve resultado. Quem cronometra e classifica e a instrumentacao do motor (ADR 0003 §3).
- **A saida nao calcula.** HTML, JSON, terminal e comparacao sao projecoes do documento de resultado. Nenhuma delas recalcula percentil por conta propria (ADR 0003 §6).

## Fluxo de uma execucao

```mermaid
sequenceDiagram
    participant U as Usuario
    participant C as CLI
    participant M as Modelo
    participant A as Agendador
    participant E as Executor
    participant P as Protocolo
    participant I as Instrumentacao
    participant R as Relatorio

    U->>C: braunrate executar cenario.yaml
    C->>M: carregar e validar
    M-->>C: cenario valido (ou erro apontando linha)
    C->>A: plano de carga + cenario
    A->>A: calcula instantes agendados de t0
    loop cada instante agendado
        A->>A: espera hibrida ate o instante
        A->>I: registra desvio (despacho - agendado)
        A->>E: despacha usuario virtual (carrega o instante agendado)
        E->>P: executa passo
        P-->>E: resultado bruto
        E->>I: resultado + instante agendado
        I->>I: HDR do agendado ate o fim, contadores, classificacao
    end
    I-->>C: amostragem periodica
    C-->>U: terminal vivo (taxa, latencia, erros, aviso de back-pressure)
    A->>I: fim da janela
    I->>R: documento de resultado
    R->>R: avalia SLO
    R-->>U: HTML + JSON + resumo, codigo de saida do SLO
```

O ponto que nao pode ser negociado: o **instante agendado viaja junto com o usuario virtual** ate a instrumentacao. Se ele ficar so no agendador, a latencia volta a ser contada do envio e a omissao coordenada volta.

## Onde entra um protocolo novo

```mermaid
flowchart LR
    subgraph contrato["O que um protocolo implementa"]
        exec["executar(passo, contexto) -> resultado"]
        esquema["esquema de configuracao<br/>(valida o YAML do passo)"]
        chave["chave de agregacao<br/>(rota, operationName, topico)"]
        metricas["metricas proprias declaradas<br/>(lag, ack, correlacao perdida)"]
        classificar["classificacao de erro propria<br/>(errors do GraphQL, nack, timeout)"]
    end

    contrato --> registro["Registro de protocolos"]
    registro --> motor["Motor (mede, agrega, reporta)"]
    registro --> gramatica["Gramatica do YAML e metodos da DSL"]
```

Um protocolo novo nao toca em: agendador, histograma, formato de resultado, relatorio. Ele declara chave de agregacao e classificacao de erro; o resto e do motor. E o que faz Kafka e HTTP produzirem numero comparavel (estudo §9.3).

O cenario hibrido — `kafka.produzir` seguido de `aguardar` — usa o mesmo contrato: `aguardar` e um protocolo cujo `executar` faz polling ate a condicao ou timeout, e cuja latencia medida e a da cadeia inteira, a partir do instante agendado do proprio passo.

## Estrutura de modulos

```
braunrate/
├── cenario/     modelo, validacao, parser YAML, serializacao
├── dsl/         construtores sobre o mesmo modelo
├── motor/       plano de carga, agendador, executor, contexto
├── metrica/     HDR, contadores, series, documento de resultado, merge
├── protocolo/   registro + http, graphql, kafka, amqp, aguardar
├── relatorio/   terminal, html, json, csv, markdown, comparacao
├── dados/       csv com politica de consumo, geracao com semente
├── slo/         avaliacao e codigo de saida
└── importador/  jmx -> modelo
```

Dependencia permitida em uma direcao so: `relatorio` e `protocolo` dependem de `metrica` e `cenario`; `motor` depende de `cenario`, `metrica` e do registro de protocolos; `cenario` e `metrica` nao dependem de ninguem acima. Um `import` de `protocolo` dentro de `metrica` e erro de arquitetura, porque e o comeco de metrica especifica de protocolo.

## Preparacao para execucao distribuida

Nao entra na v1 (estudo §3.8), e a arquitetura nao pode impedi-la. O que ja fica pronto:

```mermaid
flowchart TB
    coord["Coordenador (futuro)"]
    g1["Gerador 1<br/>documento parcial"]
    g2["Gerador 2<br/>documento parcial"]
    g3["Gerador N<br/>documento parcial"]
    soma["Merge<br/>HDR + contadores + buckets alinhados ao epoch"]
    rel["Relatorio unico"]

    coord --> g1 & g2 & g3
    g1 & g2 & g3 --> soma --> rel
```

Requisitos que isso impoe hoje: histograma serializavel e somavel, buckets de serie temporal alinhados ao epoch (nao ao inicio local do processo), plano de carga divisivel por fracao de taxa, e nenhuma media ou percentil pre-calculado no documento parcial.
