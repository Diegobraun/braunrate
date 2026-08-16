# ADR 0017 — Superfície pública de execução, e o fim do `braunrate build`

- **Status**: aceito
- **Data**: 2026-08-16
- **Relacionados**: [ADR 0002](0002-modelo-de-cenario.md), [ADR 0004](0004-extensao-de-protocolo.md), [ADR 0009](0009-equivalencia-entre-yaml-e-dsl.md), [ADR 0015](0015-superficie-publica-da-dsl.md)
- **Substitui**: o item 3 da decisao do ADR 0015 ("nao se acrescenta superficie publica antes da v1")

## Contexto

Duas pendencias antigas sao a mesma pergunta com dois nomes: **o que atravessa a
fronteira de `internal/`**.

- O ADR 0015 registrou que metade da promessa do ADR 0002 nao existe: `dsl` e
  publico e so **monta** o cenario. Executar exige `internal/engine`, ler o
  resultado exige `internal/metrics`, e um modulo de fora nao alcanca nenhum dos
  dois. A decisao foi adiar para a v1 e declarar a limitacao no README.
- O ADR 0004 prometeu, para a Fase 7, um `braunrate build` que gera um binario
  com protocolos de terceiros declarados num `braunrate.build.yaml`. Nunca foi
  feito, e continua listado como pendente.

Duas fases se passaram. Promessa em ADR que ninguem cumpriu e pior que ausencia
de promessa, porque quem le acredita — e o README continua mostrando "o mesmo
cenario em Go" como coisa disponivel.

## Decisao

### 1. Existe um pacote publico de execucao, agora

`github.com/Diegobraun/braunrate` expoe o minimo para montar, rodar e ler:

```go
spec, err := dsl.New("Jornada").Target(alvo). /* ... */ Build()
resultado, err := braunrate.Run(context.Background(), spec, braunrate.Options{})
os.Exit(braunrate.ExitCode(resultado))
```

Seis funcoes (`Load`, `Parse`, `Run`, `Passed`, `ExitCode`, `Summary`, `HTML`) e
tres apelidos de tipo (`Scenario`, `Result`, `Verdict`). O caminho por dentro e o
mesmo do CLI, sem excecao: mesma validacao, mesmo motor, mesma avaliacao de SLO,
mesmo relatorio. Um segundo caminho aqui daria um numero para quem escreve YAML e
outro para quem escreve Go, que e o que o ADR 0009 proibe.

**O que muda em relacao ao ADR 0015.** A objecao principal la era que um pacote
fino obrigaria a reexportar o documento de resultado inteiro — "duas definicoes
do mesmo dado para manter em dia". Ela cai com **apelido de tipo**:
`type Result = metrics.Document` tem uma definicao so. Nao ha espelho, nao ha
conversao, nao ha o que sair de sincronia.

**O que a objecao tinha de certo, e continua tendo.** O apelido torna publico,
de forma transitiva, tudo que esta em `metrics.Document` e em `scenario.Spec`.
E superficie de verdade, e maior do que parece. A resposta nao e fingir que nao
e — e declarar o regime:

> **Ate a v1, estes tipos seguem a versao que ja os governa e nao estao
> congelados.** `Result` acompanha o formato de resultado (`versao_do_formato`,
> hoje `2`), e `Scenario` acompanha o formato de cenario. Campo novo entra sem
> aviso; campo que sair ou mudar de nome sai com a versao mudando junto — que e
> o mesmo contrato que quem guarda arquivo de resultado ja tem.

Isso e possivel hoje e nao era quando o ADR 0015 foi escrito: o formato de
resultado ganhou versionamento com lista de formatos legiveis e recusa por nome
no mesmo dia. O que faltava para expor nao era estabilidade absoluta — era ter
onde declarar a mudanca.

**O que fica de fora, de proposito**: escrever protocolo novo. Ver o item 2.

**Prova**: `TestAModuleOutsideThisOneCompilesAgainstThePublicSurface` cria um
modulo fora deste, com `replace` para a raiz, e compila um programa que monta
pela DSL e roda pela superficie publica. `TestTheInternalPackagesStayOutOfReach`
faz o oposto e exige que o import de `internal/` continue nao compilando. O
exemplo publicado em Go passou a rodar por esse mesmo caminho: rodar o exemplo
por uma porta que so existe aqui dentro nao provaria que a porta publicada
funciona.

### 2. `braunrate build` nao vai existir

O ADR 0004 previa um comando que le `braunrate.build.yaml`, gera um binario com
modulos de protocolo de terceiros e fixa versoes por `go.sum`. Fica cancelado,
por tres razoes, em ordem de peso:

1. **A interface que ele estenderia nao e publica, e nao vai ser antes da v1.**
   `protocol.Protocol`, `protocol.Config`, `protocol.Response`, `Preparable`,
   `WithBody`, `WithTLS` — todos em `internal/`. Torna-los publicos e assinar o
   contrato versionado que o proprio ADR 0004 marcou para a v1. Sem isso, nao ha
   protocolo de fora para o comando compilar: hoje ele geraria um binario com
   exatamente os protocolos que ja vem.
2. **Ele nao economiza o passo que importa.** Quem escreve protocolo em Go ja tem
   Go instalado e um modulo. Para esse alguem, um `main.go` de seis linhas com
   imports em branco mais `go build` e mais curto do que aprender um formato de
   YAML de build. O comando trocaria uma coisa que a linguagem ja faz por uma
   coisa a manter.
3. **Ele contradiz a entrega.** O caminho de adocao e baixar um binario e rodar,
   sem instalar nada — e o publico principal nao tem Go. Um comando que so
   funciona para quem tem a toolchain instalada nao e o caminho de extensao
   desse publico; e o caminho de quem ja esta dentro do repositorio.

**O que fica no lugar**: protocolo fora da lista compilada exige **mudanca neste
repositorio** — contribuicao ou fork —, nao um build local com plugin. Isso vai
para o README como limitacao declarada, com essas palavras.

**O criterio que reabre**: alguem de fora precisar de um protocolo que este
repositorio nao vai compilar. Nesse dia a decisao a tomar e tornar a interface de
protocolo publica e versionada, e ai o `go build` de sempre resolve — nao um
comando proprio.

## Alternativas descartadas

- **Mover `scenario`, `engine`, `metrics` e `protocol` para fora de `internal/`**
  (a opcao B do ADR 0015): congela quatro pacotes que ainda mudam a cada fase e
  antecipa o compromisso da v1 sem a v1. O apelido de tipo entrega o mesmo uso
  sem entregar o mesmo compromisso.
- **Declarar que a DSL serve so para quem estende a ferramenta** (a opcao C do
  ADR 0015): joga fora a razao de existir da DSL para quem tem cenario que o YAML
  nao expressa, e contraria a leitura natural do ADR 0002. Foi a limitacao
  vigente por duas fases, e o custo dela foi anunciar no README uma coisa que nao
  se podia fazer.
- **Implementar `braunrate build` mesmo assim, so para cumprir o ADR**: cumpriria
  a letra e nao a promessa — o comando existiria e nao haveria protocolo de fora
  para ele compilar.

## Consequencias

- O README para de marcar "cenario em Go" como parcial por causa do alcance, e
  passa a mostrar o programa que roda de fora. A limitacao que fica escrita e
  outra: **protocolo novo exige mudanca no repositorio**.
- O ADR 0015 continua valendo no diagnostico e na escolha da porta (a opcao A);
  o que muda e a data e o item 3.
- O ADR 0004 tem o item 3 das consequencias praticas cancelado, com este ADR
  como motivo.
- A v1 herda uma decisao a menos e uma a mais: a superficie de execucao ja
  existe e sera congelada; a superficie de protocolo continua por decidir, agora
  com o criterio escrito.
