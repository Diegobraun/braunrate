# ADR 0015 — Superficie publica da DSL: o que um projeto de fora consegue fazer

- **Status**: parcialmente substituido pelo [ADR 0017](0017-superficie-publica-de-execucao.md) em 2026-08-16 — o diagnostico e a escolha da porta (opcao A) continuam valendo; o item 3 da decisao, que adiava qualquer superficie publica para a v1, foi revogado
- **Data**: 2026-08-16
- **Contexto de decisao**: revisao no fim da Fase 8
- **Relacionados**: [ADR 0002](0002-modelo-de-cenario.md), [ADR 0004](0004-extensao-de-protocolo.md), [ADR 0009](0009-equivalencia-entre-yaml-e-dsl.md), [ADR 0010](0010-idioma-do-codigo.md)

## Contexto

O ADR 0002 promete dois publicos com um motor so: QA escreve YAML, dev escreve Go, e o cenario dos dois vai para o mesmo lugar. A Fase 6 entregou a DSL e travou a equivalencia caso a caso. O README anunciava a migracao de YAML para Go como coisa feita.

A revisao do fim da Fase 8 mostrou que **metade da promessa nao existe**. `dsl` e o unico pacote fora de `internal/`, mas ele so **monta** o cenario. Para executar e preciso `internal/engine`, e para ler o resultado e preciso `internal/metrics`. Um projeto de fora que importe `braunrate/dsl` consegue construir um `scenario.Spec` — que tambem e um tipo de `internal/` — e nao consegue rodar nem inspecionar nada:

```
main.go:8:2: use of internal package github.com/Diegobraun/braunrate/internal/engine not allowed
```

O trecho publicado no README nem compilava, porque usava os nomes anteriores ao ADR 0010; ninguem percebeu por quase duas fases. Isso e sintoma do mesmo problema: **nao ha usuario de fora, entao nao ha quem tropece**.

Somam-se dois fatos que limitam a saida:

- O tipo central, `scenario.Spec`, e de `internal/`, e o metodo publico `Build()` ja o devolve. A superficie publica de hoje **ja e incoerente**: expoe um tipo que quem chama nao pode nomear.
- O ADR 0004 registra que a interface de protocolo vira contrato publico versionado **a partir da v1**. Congelar superficie agora e antecipar esse compromisso sem a v1 para sustenta-lo.

## Alternativas consideradas

**A. Expor um pacote fino de execucao.** Algo como `braunrate.Run(ctx, spec) (Result, error)`, mantendo o motor em `internal/` e devolvendo um resultado proprio.
Contra: `Result` teria que reexportar quase todo o documento de resultado — distribuicao, passos, sanidade, veredito, variedade, atraso de consumidor. E o formato inteiro virando API publica por uma porta lateral, com duas definicoes do mesmo dado para manter em dia. E `Build()` continua devolvendo um tipo interno.

**B. Tirar de `internal/` o que a DSL precisa.** Mover `scenario`, `engine`, `metrics` e `protocol` para pacotes publicos.
Contra: congela quatro pacotes que ainda mudam a cada fase — o formato de resultado ganhou tres campos so nesta noite. E o oposto do que o ADR 0004 combinou para a v1, e o custo de errar aqui e alto: nome publico errado se paga com depreciacao, nao com renomeacao.

**C. Assumir que o publico dev usa YAML, e a DSL serve para quem estende a ferramenta.**
Contra: contraria a leitura mais natural do ADR 0002 e joga fora a razao de existir da DSL para quem tem cenario que o YAML nao expressa.

## Decisao

**Fica a A, adiada para a v1, com a limitacao declarada agora.**

1. **Ate a v1, a DSL e para quem trabalha dentro deste repositorio.** Quem versiona o teste de carga junto com o servico, faz fork ou vendora, e constroi o binario a partir daqui. Isso passa a estar escrito no README, na tabela de recursos e na secao de escopo — nao como detalhe, como limitacao.
2. **A porta escolhida para a v1 e a A**, e nao a B: um pacote de execucao fino, com o documento de resultado exposto como **um** tipo publico, e nao um espelho a manter. O que decide entre "reexportar" e "mover `metrics` inteiro" fica para o ADR da v1, com o formato de resultado ja estavel.
3. **Nao se acrescenta superficie publica antes disso.** Nem helper, nem "so um `Run`", nem tipo de conveniencia. Superficie publica acrescentada por conveniencia e a que ninguem consegue tirar depois.
4. **O exemplo publicado em Go vive em `examples/cenario-em-go/`, compila e roda no CI**, contra o alvo embutido, conferindo o proprio SLO. Um teste reprova o build se o README derivar do arquivo. Regra geral: **trecho de codigo Go publicado em README ou em `docs/` vive num arquivo compilavel e executado pelo CI.** Documentacao que nao compila e a unica que ninguem percebe estar errada — foram quatro ocorrencias desta familia (`ci.yaml` verde com 100% de 401, `http-basico.yaml`, os numeros de comparacao no README, e este trecho).

## Consequencias

- O README para de sugerir que dev de fora usa a DSL. A tabela de recursos marca "cenario em Go" como **parcial**, com o motivo e o link para este ADR.
- A promessa do ADR 0002 continua de pe **dentro** do modulo, que e onde a equivalencia e travada por teste; o que estava errado era o alcance anunciado, nao o mecanismo.
- Quem hoje precisa de cenario em Go tem um caminho declarado: trabalhar a partir deste repositorio. Nao e o caminho que a v1 quer, e esta escrito que nao e.
- A v1 nasce com uma decisao a menos para tomar no calor: a porta ja esta escolhida, falta dimensiona-la.
