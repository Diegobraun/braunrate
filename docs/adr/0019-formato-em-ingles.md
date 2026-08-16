# ADR 0019 — O formato do cenario passa para ingles

- **Status**: aceito
- **Data**: 2026-08-16
- **Contexto de decisao**: internacionalizacao, Fase 1
- **Relacionados**: [ADR 0002](0002-modelo-de-cenario.md), [ADR 0009](0009-equivalencia-entre-yaml-e-dsl.md), [ADR 0010](0010-idioma-do-codigo.md)

## Contexto

O ADR 0010 dividiu o projeto em duas metades: o que o dev le em ingles, o que o usuario le em portugues. A fronteira era "o que sai na tela". O cenario ficou do lado portugues.

Tres coisas mudaram desde entao.

1. **O cenario nao e tela, e artefato.** Ele fica versionado no repositorio de quem usa, entra em revisao de codigo, e e lido por quem chega no time. Codigo em portugues fecha o repositorio do braunrate para contribuicao externa; formato em portugues fecha o repositorio *do usuario*.
2. **YAML nao aceita acento em chave** (decisao 9 do relatorio de experiencia). O formato ja nao era portugues: era `autenticacao`, `cabecalhos`, `variaveis`, `renovar_apos`. Nem portugues correto, nem ingles.
3. **A convencao de nome ja estava misturada.** `renovar_apos` e `intervalo_entre_iteracoes` em snake_case ao lado de `corpo_contem` e `novo_a_cada`; `taxa_efetiva` ao lado de `vazao`, que e a mesma metrica com dois nomes.

## Decisao

**Chave e valor do cenario passam para ingles, sem alternativa de idioma.** O mesmo vale para o documento JSON de resultado, que e o outro artefato que circula: ele e commitado como linha de base e lido por script de CI.

**Convencao unica: camelCase**, para chave composta e para valor composto. Sem excecao, inclusive para nome de mecanismo SASL — a excecao e o que produziu a mistura atual. Metodo HTTP (`GET`, `POST`) continua em maiuscula porque e o valor do protocolo, nao um valor nosso.

**Criterio de escolha do termo**: o que k6, Gatling ou JMeter ja usam, para quem migra reconhecer. `expect` e nao `verify`; `capture` e nao `extract`; `rate` e nao `throughput` na carga; `steady` e nao `plateau`; `thinkTime` porque e assim que JMeter e Gatling chamam a pausa entre iteracoes.

### Topo do cenario

| Portugues | Ingles |
|---|---|
| `nome` | `name` |
| `alvo` | `target` |
| `requer` | `requires` |
| `variaveis` | `variables` |
| `autenticacao` | `auth` |
| `tls` | `tls` |
| `mensageria` | `messaging` |
| `dados` | `data` |
| `carga` | `load` |
| `cenario` | `scenario` |
| `slo` | `slo` |

Valores de `requer`: `kafka`, `amqp`, `credencial` → `kafka`, `amqp`, `credential`.

### Carga

| Portugues | Ingles |
|---|---|
| `carga.modelo` | `load.model` |
| `carga.perfis` | `load.profiles` |
| `carga.usuarios` | `load.users` |
| `carga.duracao` | `load.duration` |
| `carga.intervalo_entre_iteracoes` | `load.thinkTime` |
| `aberto` / `fechado` | `open` / `closed` |

| Perfil | Ingles |
|---|---|
| `rampa` | `ramp` |
| `patamar` | `steady` |
| `pico` | `spike` |
| `constante` | *removido* |
| `de` | `from` |
| `ate` | `to` |
| `taxa` | `rate` |
| `durante` | `duration` |

### Autenticacao

| Portugues | Ingles |
|---|---|
| `tipo` | `type` |
| `obter` | `obtain` |
| `renovar_apos` | `refreshAfter` |
| `cabecalho` | `header` |
| `usuario` | `user` |
| `senha` | `password` |
| `token` / `basica` / `cabecalho` | `token` / `basic` / `header` |

### Dados

| Portugues | Ingles |
|---|---|
| `arquivo` | `file` |
| `consumo` | `consume` |
| `semente` | `seed` |
| `gerar` | `generate` |
| `sequencial` | `sequential` |
| `aleatorio` | `random` |
| `circular` | `circular` |
| `unico_por_usuario` | `uniquePerUser` |

Campo gerado:

| Portugues | Ingles |
|---|---|
| `tipo` | `type` |
| `formato` | `format` |
| `novo_a_cada` | `newEvery` |
| `iteracao` / `uso` | `iteration` / `use` |

Receitas de geracao: `uuid`, `sequencia`, `numero`, `inteiro`, `nome`, `email`, `texto`, `padrao`, `cpf`, `cnpj` → `uuid`, `sequence`, `number`, `integer`, `name`, `email`, `text`, `pattern`, `cpf`, `cnpj`.

### Passo

| Portugues | Ingles |
|---|---|
| `nome` | `name` |
| `peso` | `weight` |
| `captura` | `capture` |
| `verificar` | `expect` |
| `espera` | *removido* (era apelido de `verificar`) |
| `aguardar` | `await` |

Verificacoes:

| Portugues | Ingles |
|---|---|
| `status` | `status` |
| `corpo_contem` | `bodyContains` |
| `corpo_casa` | `bodyMatches` |
| `json` | `json` |
| `cabecalho` | `header` |

Captura, forma completa: `de` → `from`, `padrao` → `default`, `obrigatoria` → `required`.
Captura, expressao: `status` → `status`, `corpo` → `body`, `cabecalho:` → `header:`, `cookie:` → `cookie:`, `/regex/` sem mudanca.
Comparacao: `existe` → `exists`, `contem` → `contains`.

### Protocolos

`http`: `metodo` → `method`, `caminho` → `path`, `url` → `url`, `cabecalhos` → `headers`, `corpo` → `body`, `timeout` → `timeout`, `seguir_redirect` → `followRedirects`.

`graphql`: `consulta` e `query` → `query` (apelido removido), `operacao` → `operation`, `variaveis` → `variables`, `caminho` → `path`, `cabecalhos` → `headers`, `timeout` → `timeout`.

`kafka`: `topico` → `topic`, `chave` → `key`, `valor` → `value`, `cabecalhos` → `headers`, `brokers` → `brokers`, `acks` → `acks`, `particao` → `partition`, `grupo` → `group`, `timeout` → `timeout`. Valores de `acks`: `todos` / `lider` / `nenhum` → `all` / `leader` / `none`.

`amqp`: `fila` → `queue`, `troca` → `exchange`, `rota` → `routingKey`, `corpo` → `body`, `identidade` → `messageId`, `cabecalhos` → `headers`, `url` → `url`, `persistente` → `persistent`, `confirmar` → `confirm`, `timeout` → `timeout`.

`await` (era `aguardar`): `ate` → `until`, `intervalo` → `interval`, `chave` → `key`, `campo` → `field`, `igual_a` → `equals`, `timeout` → `timeout`. Fonte: `topico` / `fila` → `topic` / `queue`, `enderecos` → `addresses`, `caminho` → `path`.

### SLO

Escopo: `global` → `global`, `jornada` → `journey`, `regressao` → `regression`.

Metricas: `erros` → `errors`, `sucesso` → `success`, `vazao` e `taxa_efetiva` → `throughput` (apelido removido), percentis (`p50`…`p99.9`, `max`) sem mudanca.

Regressao: `jornada_p95` → `journeyP95`, `global_p99` → `globalP99`, e assim por diante. Sufixo `pior` → `worse`.

### Mensageria

`brokers` e `enderecos` → `brokers` e `addresses`, `autenticacao` → `auth`, `tls` → `tls`.

Autenticacao de broker: `tipo` → `type`, `usuario` → `user`, `senha` → `password`, `regiao` → `region`.
Mecanismos: `sasl_plain` → `saslPlain`, `scram_sha256` → `scramSha256`, `scram_sha512` → `scramSha512`, `msk_iam` → `mskIam`, `certificado` → `certificate`.

TLS: `ca` → `ca`, `certificado` → `certificate`, `chave` → `key`.

### As decisoes dificeis

**`aguardar` → `await`, e `ate` → `until`.** `wait` seria esperar tempo; o passo espera uma *condicao*, com timeout. `await` diz isso, e a condicao fica `until: { $.status: PROCESSED }`, que se le como frase. A sugestao original era `awaitUntil` numa chave so; foi descartada porque a condicao ja e uma chave separada e `awaitUntil.until` seria a mesma palavra duas vezes.

**`durante` e `duracao` viram os dois `duration`.** O portugues usava duas palavras para a mesma coisa em dois lugares. `duration` e o que o k6 usa em `stages`.

**`intervalo_entre_iteracoes` → `thinkTime`.** Nao e traducao, e o termo do dominio: JMeter e Gatling chamam assim ha vinte anos. `intervalBetweenIterations` seria correto e ninguem reconheceria.

**`constante` sai.** Era sinonimo exato de `patamar` — mesmo tipo de fase, mesma matematica. `pico` fica porque nomeia uma intencao que o relatorio imprime; `constante` nao nomeia nada que `steady` ja nao diga.

**`espera` sai.** Era apelido de `verificar` no passo, e apelido em formato publicado e chave que existe so para quem leu o codigo.

**`vazao` e `taxa_efetiva` viram `throughput`.** Duas chaves para a mesma metrica, com a diferenca so na cabeca de quem escreveu.

**`cpf` e `cnpj` ficam.** Sao documentos brasileiros; traduzir viraria `taxId`, que nao gera nem um nem outro.

**`slo` fica `slo`.** Sigla em ingles desde sempre.

**Mecanismo SASL em camelCase, nao `SCRAM-SHA-512`.** A grafia oficial e maiuscula com hifen, e teria sido a unica familia de valores fora da convencao. Um formato com uma excecao por familia e o formato de hoje.

### Documento JSON de resultado

O ADR 0010 deixou os campos do JSON em portugues por serem "formato publicado". Isso se inverte agora, pela mesma razao que move o YAML: o documento e commitado como linha de base para `-baseline`, lido por script de CI e comparado entre execucoes. Todos os campos passam para camelCase em ingles (`versao_do_formato` → `formatVersion`, `latencia_corrigida` → `correctedLatency`, `variedade_observada` → `observedVariety`, e assim para os 132 campos).

`formatVersion` do resultado sobe de `2` para `3`, e `formatVersion` do cenario de `1` para `2`. Um documento da 0.5.0 e reconhecido pela chave `ferramenta` e recebe a mensagem que ensina o caminho, nao um campo vazio.

## Alternativas descartadas

- **Manter o formato em portugues e traduzir so as mensagens**: resolveria a leitura e nao o artefato. O YAML e o que sai do time de quem usa.
- **Aceitar os dois formatos, com apelido em ingles para cada chave**: dobra a superficie do parser para sempre, e produz cenario metade em cada idioma dentro do mesmo arquivo.
- **snake_case**: coerente com `renovar_apos`, mas o schema JSON, o k6 e o Gatling usam camelCase, e o `yaml-language-server` sugere o que o schema declara.
- **Adiar para depois da v1**: cada semana publicada e mais cenario escrito por ai no formato antigo, e o `migrate` precisa existir de qualquer forma.

## Consequencias

- **Todo cenario existente quebra.** `braunrate migrate` converte arquivo ou pasta, preservando comentario e ordem, e o parser reconhece o formato antigo para ensinar o caminho em vez de dizer "chave desconhecida".
- Oito produtores de YAML seguem este mapa: parser, `docs/braunrate.schema.json`, DSL em Go, `new`, `import curl`, `import jmx`, `record` e interface web.
- Nenhum comportamento muda. Renomear e traduzir sao a mudanca inteira.
- A DSL em Go nao muda de superficie: os metodos ja estavam em ingles (ADR 0009). O que muda sao os valores literais que ela passa para o parser (`padrao`, `unico_por_usuario`).
- O teste de varredura que hoje protege a decisao 9 (nenhuma chave acentuada em mensagem) passa a proteger tambem esta: nenhuma chave ou valor em portugues sobrevive fora da lista de excecoes.
