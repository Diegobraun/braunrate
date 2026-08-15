# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR histogram e deteccao de back-pressure.

Estado: **Fase 0 — decisao de arquitetura**. Nao existe binario utilizavel ainda.

## Por que existe

Tres razoes, nesta ordem:

1. **Medicao honesta por padrao.** Modelo de chegada aberto; latencia contada a partir do instante em que a requisicao *deveria* ter partido; HDR histogram; aviso explicito quando o gerador nao sustentou a taxa alvo. A omissao coordenada e a falha que faz teste passar com p99 de 47 ms enquanto producao sofre 1,8 s.
2. **Dois publicos, um motor.** YAML declarativo para o caso comum, DSL para o complexo — mesmo motor, mesmas metricas, sem reescrita ao migrar.
3. **Cenario de negocio, nao so requisicao.** GraphQL medido por operacao; Kafka e RabbitMQ com modelo de metrica proprio; passo `aguardar` para medir a cadeia assincrona ponta a ponta.

## Escopo

**Dentro:** HTTP/HTTPS e REST; GraphQL de primeira classe; Kafka e RabbitMQ (produzir e consumir); passo `aguardar` com timeout; correlacao, variaveis e fluxo de autenticacao; CSV com politica de consumo e geracao sintetica com semente; perfis de carga (rampa, patamar, pico, taxa constante); SLO com codigo de saida; relatorio HTML autocontido, JSON, CSV e resumo de terminal; comparacao entre execucoes; importador de `.jmx` para o subconjunto comum.

**Fora:** motor de browser real; nuvem gerenciada, dashboard multiusuario, conta de time; LDAP, FTP, SMTP, JMS classico; competir em vazao bruta com wrk; execucao distribuida na v1 — a arquitetura nao pode impedi-la, mas ela nao entra agora.

## Documentacao

- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — base de todas as decisoes
- [Arquitetura](docs/arquitetura.md)
- [ADR 0001 — linguagem e runtime](docs/adr/0001-linguagem-e-runtime.md)
- [ADR 0002 — modelo de cenario](docs/adr/0002-modelo-de-cenario.md)
- [ADR 0003 — modelo de execucao e metrica](docs/adr/0003-modelo-de-execucao-e-metrica.md)
- [Medicao dos prototipos da Fase 0](docs/medicoes-fase0.md)

## Licenca

MIT — Diego Braun.
