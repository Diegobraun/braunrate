# ADR 0010 — Codigo em ingles, produto em portugues

- **Status**: aceito (revisa a decisao de idioma vigente desde a Fase 0)
- **Data**: 2026-08-16
- **Contexto de decisao**: reorganizacao estrutural anterior a Fase 7
- **Relacionados**: [ADR 0002](0002-modelo-de-cenario.md), [ADR 0004](0004-extensao-de-protocolo.md)

## Contexto

Ate a Fase 6 o projeto inteiro era escrito em portugues sem acento: pacote, tipo, funcao, campo, teste, comentario, chave de YAML e mensagem. A razao original era coerencia com o publico — um time brasileiro, com QA que nao programa.

Com o codigo maior, tres problemas ficaram visiveis:

1. **O dominio do braunrate e tecnico, nao de negocio.** `metrica`, `protocolo`, `contexto`, `correlacao` e `latencia` nao sao termos de negocio traduzidos: sao termos tecnicos que ja existem em ingles e que todo dev le em ingles. A traducao nao acrescenta clareza, e sem acento produz palavra que nao e portugues correto nem ingles.
2. **O repositorio e publico e candidato a contribuicao externa.** Quem chega de fora le `motor.Executar` e precisa traduzir de volta antes de entender.
3. **A mistura ja estava acontecendo.** `ChaveDeAgregacao` convivia com `Timeout`, `Status`, `Bytes` e `Header` — termos que nunca foram traduzidos porque traduzir teria piorado.

## Decisao

**O que o dev le e ingles. O que o usuario le e portugues.**

Em ingles: nome de pacote, tipo, funcao, metodo, campo, constante e variavel; nome de arquivo; nome de teste; comentario, inclusive o de doc de identificador exportado.

Em portugues: chave de YAML (`nome`, `alvo`, `carga`, `cenario`, `slo`, `dados`, `autenticacao`), toda mensagem ao usuario (terminal, HTML, erro de validacao, aviso, ajuda), README, ADR, documentacao, exemplos, schema e mensagem de commit.

A fronteira e o que sai na tela. Nenhum identificador mistura os dois idiomas.

**Emenda de 2026-08-16 — a linha de comando fica em ingles.** Subcomando e opcao passam a ser `execute`, `validate`, `debug`, `report`, `compare`, `new`, `import`, `target`, `version`, `-result`, `-quiet`, `-max-concurrent`, `-late-threshold`, `-body`, `-address`, `-latency`, `-freeze-after`, `-freeze-for`, `-input`, `-output`, `-processor-delay`. O motivo e convencao de ferramenta de linha de comando: `k6 run`, `jmeter`, `ab` e `go test` usam verbo em ingles, e quem digita o comando esta num terminal, nao lendo um relatorio. **O que o comando imprime continua em portugues**, e as chaves do YAML tambem — o cenario nao mudou uma letra. Custou renomear README, docs, ADRs e CI de uma vez; nao ha usuario externo com script para quebrar, e adiar so aumentaria o custo.

Ao lado da traducao, tres regras de nome passaram a valer: o pacote ja e contexto e nao se repete no tipo (`metrics.Document`, nao `metrics.MetricsDocument`); nome diz o que a coisa e, nao como foi feita; e interface pequena, nomeada pelo comportamento (`Describable`, `Preparable`, `WithHeaders`).

## Alternativas descartadas

- **Manter tudo em portugues**: coerente, mas fecha o repositorio para contribuicao externa e obriga a traduzir termo tecnico que so existe bem em ingles.
- **Traduzir tambem as chaves de YAML e as mensagens**: seria trocar o publico da ferramenta. O QA que nao programa le a mensagem de erro, nao o identificador.
- **Deixar para depois da v1**: renomear com o dobro do codigo custaria o dobro, e a suite que protege a decisao (equivalencia YAML x DSL, exemplo publicado, mensagens de erro) ja existia agora.

## Consequencias

- A reorganizacao foi feita em quatro commits — mover para `internal/`, traduzir, melhorar nomes, limpar comentarios — com a suite verde em cada um.
- **Nenhuma saida ao usuario mudou.** O teste do exemplo publicado compara o HTML gerado com o arquivo commitado e continuou verde; os cenarios de exemplo continuam rodando; as mensagens de erro continuam em portugues, cobertas por teste proprio.
- Os campos `json` do documento de resultado continuam em portugues: eles sao formato publicado, nao codigo.
- O ADR 0004 continua valendo: protocolo fora-de-arvore implementa a mesma interface, agora com nomes em ingles (`protocol.Protocol`, `protocol.Config`).
