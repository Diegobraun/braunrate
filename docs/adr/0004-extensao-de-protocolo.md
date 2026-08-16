# ADR 0004 — Extensão de protocolo: paridade com o k6, declarada

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 0, apos a decisao do [ADR 0001](0001-linguagem-e-runtime.md)
- **Relacionados**: [estudo de ferramentas](../estudo-ferramentas.md) §3.9, §9.5

## Contexto

O estudo (§3.9) e explicito: exigir recompilacao do binario para adicionar protocolo e **friccao real** do k6, e uma das razoes pelas quais o JMeter, com mais de mil plugins, continua vivo. A implicacao registrada la era "SPI de protocolo carregada em runtime, sem recompilar o motor".

A escolha de Go no ADR 0001 torna isso inviavel na pratica:

- `plugin` do Go so funciona em Linux e macOS, exige que plugin e binario sejam compilados com a **mesma versao do compilador e as mesmas versoes de todas as dependencias compartilhadas**, e nao funciona com binario estatico — que e justamente o formato de distribuicao que decidiu o ADR 0001.
- WASM ou subprocesso resolveriam o carregamento, ao custo de serializar cada requisicao atraves de uma fronteira — inaceitavel no caminho quente de um gerador de carga.

## Decisao

**Aceitamos paridade com o k6 neste eixo e competimos nos demais.** Protocolo entra compilado no binario.

Consequencias praticas:

1. **A v1 traz compilados**: HTTP/REST, GraphQL, Kafka, AMQP e `aguardar`. Cobrem o backlog essencial e importante do estudo (§8).
2. **O registro de protocolos e uma interface pequena e estavel** (ADR 0003 §3): `executar(passo, contexto) -> resultado`, esquema de configuracao, chave de agregacao, classificacao de erro e metricas proprias. Protocolo novo nao toca em agendador, histograma, formato de resultado nem relatorio.
3. ~~**Protocolo fora-de-arvore e suportado por build reprodutivel**, nao por carregamento dinamico.~~ **Cancelado em 2026-08-16 pelo [ADR 0017](0017-superficie-publica-de-execucao.md)**: a interface que o comando estenderia continua em `internal/` e so vira contrato publico na v1, entao `braunrate build` geraria um binario com exatamente os protocolos que ja vem. Protocolo fora da lista exige mudanca neste repositorio. O texto original, para registro:
   - o autor cria um modulo Go que importa `braunrate/protocolo` e registra a implementacao em `init()`;
   - um arquivo `braunrate.build.yaml` declara os modulos extras e suas versoes;
   - `braunrate build` gera um binario com os protocolos declarados, fixando versoes via `go.sum` — o build e reprodutivel e auditavel;
   - o binario resultante reporta, em `braunrate version` e no bloco de ambiente do relatorio, quais protocolos foram compilados e em que versao.
4. **Isso vai no README como limitacao conhecida**, na secao de escopo, junto com o que esta fora. Nunca como surpresa depois da adocao. Texto obrigatorio: protocolo fora da lista exige rebuild.

## Serializacao: Avro e Schema Registry

Registrado explicitamente: **o suporte a Avro e Schema Registry e mais fraco em Go que na JVM.** Na JVM os serdes da Confluent sao referencia; em Go a combinacao usual (`hamba/avro` mais um cliente de Schema Registry) e funcional, porem menos completa e menos usada.

Como Avro e Schema Registry estao em **"desejavel (depois)"** no backlog do estudo (§8), seguimos. A v1 cobre JSON, string e binario. Se Avro virar requisito real, o custo esta declarado aqui e nao sera descoberto no meio da implementacao.

## Alternativas descartadas

- **`plugin` do Go**: incompativel com binario estatico e com a matriz de plataformas; acoplamento de versao de compilador e dependencia inviabiliza um ecossistema de terceiros.
- **Protocolo via WASM**: fronteira de serializacao no caminho quente; custo de CPU por requisicao no proprio gerador contamina a medicao — exatamente o que a tese do produto proibe.
- **Protocolo via subprocesso com IPC**: mesma objecao, com latencia adicional e uma segunda fonte de relogio.
- **Reescrever em JVM para ter `ServiceLoader`**: reabriria o ADR 0001 em favor de um criterio de peso baixo.

## Consequencias

- `braunrate build` **nao entrou na Fase 7 nem na Fase 8 e nao vai existir**: cancelado pelo [ADR 0017](0017-superficie-publica-de-execucao.md), que registra as tres razoes e o criterio que reabre a decisao.
- O bloco de ambiente do relatorio lista os protocolos compilados — sem isso, dois binarios com o mesmo numero de versao poderiam produzir resultados diferentes sem rastro. **Feito na revisao da Fase 8**; a versao de cada protocolo so faz sentido quando houver protocolo fora-de-arvore, e ate la todos vem do mesmo modulo.
- A interface de protocolo vira contrato publico versionado a partir da v1.
