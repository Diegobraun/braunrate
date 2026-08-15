# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR histogram e deteccao de back-pressure.

Estado: **Fase 1 concluida** — motor de chegada aberta, HTTP, HDR histogram, deteccao de back-pressure e resumo de terminal funcionando. Ainda nao existem correlacao, SLO, relatorio HTML, GraphQL nem mensageria.

Decisao da Fase 0: **Go**, sustentada por dois criterios apenas — RSS sob carga (30 MB contra 597 MB do Java com G1, a 10.000/s) e binario unico estatico, que para o publico de QA significa instalar baixando um arquivo. Startup, precisao de agendamento e modo de falha apareceram na primeira analise com peso que nao aguentam, e estao marcados como nao-criterio no ADR. Numeros, metodologia e limites em [medicoes-fase0.md](docs/medicoes-fase0.md); a decisao com os pesos de cada criterio em [ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Demonstracao de honestidade de medicao

O alvo de teste embutido congela por 1 s no meio da execucao. Mesma pausa, mesmo alvo, dois modelos de medicao:

| Modelo | p99 reportado | Amostras |
|---|---|---|
| **braunrate (chegada aberta, latencia contada do instante agendado)** | **977,4 ms** | 600 |
| Laco fechado (um usuario virtual em sequencia, como JMeter e Locust medem) | 2,9 ms | 783 |

**974,6 ms escondidos pelo laco fechado.** O laco fechado nao mente por bug: quando o alvo trava, ele simplesmente para de enviar, e as requisicoes que deveriam ter partido nunca entram na conta. E a omissao coordenada.

Isso nao e alegacao de marketing: e um teste automatizado que roda no CI a cada push. Se a medicao mentir, o build quebra.

```
$ go test ./autovalidacao/... -v
=== RUN   TestMedicaoRefleteCongelamentoDoAlvo
    modelo aberto: p50 2.7 ms | p99 973.8 ms | max 1005.6 ms | n 600
--- PASS: TestMedicaoRefleteCongelamentoDoAlvo (3.01s)
=== RUN   TestCongelamentoDoAlvoNaoEhConfundidoComSaturacaoDoGerador
    aviso correto: a latencia do alvo cresceu ao longo da execucao enquanto o despacho
    continuou pontual; a degradacao e do alvo, nao do gerador | p99 por segundo passou
    de 3.5 ms para 993.3 ms
--- PASS: TestCongelamentoDoAlvoNaoEhConfundidoComSaturacaoDoGerador (3.01s)
=== RUN   TestLacoFechadoEsconderiaAPausaQueOModeloAbertoMostra
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 977.4 ms sobre 600 amostras
      laco fechado:              p99 2.9 ms sobre 783 amostras
      omissao coordenada: 974.6 ms escondidos pelo laco fechado
--- PASS: TestLacoFechadoEsconderiaAPausaQueOModeloAbertoMostra (6.01s)
```

Reproduza na sua maquina: `go test ./autovalidacao/... -v`.

## Como usar

```bash
go build -o braunrate ./cmd/braunrate

braunrate alvo -latencia=5ms &                     # alvo de teste embutido
braunrate validar cenarios/http-basico.yaml        # valida sem executar
braunrate executar cenarios/http-basico.yaml       # executa e resume no terminal
braunrate executar cenarios/http-basico.yaml -resultado=saida.json
```

Cenario minimo:

```yaml
nome: Consulta de pedidos
alvo: http://127.0.0.1:8080

carga:
  modelo: aberto
  perfis:
    - rampa: { de: 100/s, ate: 800/s, durante: 5s }
    - patamar: { taxa: 800/s, durante: 10s }
    - pico: { taxa: 2000/s, durante: 3s }

cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }
```

Codigo de saida: `0` execucao valida, `2` erro de cenario, `3` **resultado invalido** — o gerador saturou e o numero nao vale.

## O que existe hoje

| Recurso | Estado |
|---|---|
| Motor de chegada aberta, latencia do instante agendado | pronto |
| Perfis: rampa, patamar, pico, taxa constante | pronto |
| HDR histogram, agregados mergeaveis, series alinhadas ao epoch | pronto |
| Deteccao de back-pressure com causa provavel (gerador x alvo) | pronto |
| Limite de requisicoes em voo, com descarte declarado no relatorio | pronto |
| HTTP: verbos, cabecalhos, corpo JSON, redirect, timeout, cookies | pronto |
| YAML com erro apontando linha e coluna | pronto |
| Resumo de terminal e progresso ao vivo | pronto |
| Correlacao, autenticacao, dados, SLO | Fase 2 |
| Relatorio HTML, JSON completo, comparacao entre execucoes | Fase 3 |
| GraphQL | Fase 4 |
| Kafka, AMQP, passo `aguardar` | Fase 5 |
| DSL e importador de `.jmx` | Fase 6 |

## Por que existe

Tres razoes, nesta ordem:

1. **Medicao honesta por padrao.** Modelo de chegada aberto; latencia contada a partir do instante em que a requisicao *deveria* ter partido; HDR histogram; aviso explicito quando o gerador nao sustentou a taxa alvo. A omissao coordenada e a falha que faz teste passar com p99 de 47 ms enquanto producao sofre 1,8 s.
2. **Dois publicos, um motor.** YAML declarativo para o caso comum, DSL para o complexo — mesmo motor, mesmas metricas, sem reescrita ao migrar.
3. **Cenario de negocio, nao so requisicao.** GraphQL medido por operacao; Kafka e RabbitMQ com modelo de metrica proprio; passo `aguardar` para medir a cadeia assincrona ponta a ponta.

## Escopo

**Dentro:** HTTP/HTTPS e REST; GraphQL de primeira classe; Kafka e RabbitMQ (produzir e consumir); passo `aguardar` com timeout; correlacao, variaveis e fluxo de autenticacao; CSV com politica de consumo e geracao sintetica com semente; perfis de carga (rampa, patamar, pico, taxa constante); SLO com codigo de saida; relatorio HTML autocontido, JSON, CSV e resumo de terminal; comparacao entre execucoes; importador de `.jmx` para o subconjunto comum.

**Limitacao conhecida:** protocolo fora da lista acima exige recompilar o binario — a mesma friccao que o k6 tem. E consequencia da escolha de Go ([ADR 0004](docs/adr/0004-extensao-de-protocolo.md)), esta declarada aqui de proposito, e o processo de build reprodutivel para protocolo fora-de-arvore sera documentado. Avro e Schema Registry sao mais fracos em Go que na JVM e ficam para depois da v1.

**Fora:** motor de browser real; nuvem gerenciada, dashboard multiusuario, conta de time; LDAP, FTP, SMTP, JMS classico; competir em vazao bruta com wrk; execucao distribuida na v1 — a arquitetura nao pode impedi-la, mas ela nao entra agora.

## Documentacao

- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — base de todas as decisoes
- [Arquitetura](docs/arquitetura.md)
- [ADR 0001 — linguagem e runtime](docs/adr/0001-linguagem-e-runtime.md)
- [ADR 0002 — modelo de cenario](docs/adr/0002-modelo-de-cenario.md)
- [ADR 0003 — modelo de execucao e metrica](docs/adr/0003-modelo-de-execucao-e-metrica.md)
- [ADR 0004 — extensao de protocolo](docs/adr/0004-extensao-de-protocolo.md)
- [Medicao dos prototipos da Fase 0](docs/medicoes-fase0.md)

## Licenca

MIT — Diego Braun.
