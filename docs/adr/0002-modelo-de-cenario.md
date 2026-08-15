# ADR 0002 — Modelo de cenario: YAML e DSL desembocando na mesma representacao

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 0
- **Relacionados**: [ADR 0001](0001-linguagem-e-runtime.md), [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [estudo de ferramentas](../estudo-ferramentas.md) §3.2, §7.1, §9.2

## Contexto

O estudo (§3.2) mostra que cada ferramenta escolheu um publico: GUI/XML atende QA e inviabiliza review; DSL em codigo atende dev e exclui QA; YAML declarativo atende os dois no caso simples e tem teto baixo. A lacuna nomeada em §7.1 e "autoria em dois niveis com um motor so".

A decisao em aberto (§9.2) e binaria: **o YAML desserializa para o mesmo objeto que a DSL constroi, ou sao caminhos separados?** Caminhos separados sao mais faceis e matam a promessa: qualquer recurso implementado num lado vira divergencia, e migrar de YAML para DSL vira reescrita.

## Decisao

**Existe um unico modelo de cenario interno. YAML e DSL sao dois construtores dele. Nada mais.**

### 1. O modelo e o contrato

Uma arvore imutavel e serializavel:

```
Cenario
 ├ metadados        (nome, alvo padrao, versao do formato)
 ├ variaveis        (nome -> expressao, com origem: literal, ambiente, dado, captura)
 ├ autenticacao?    (Passo de obtencao + politica de renovacao + injecao)
 ├ fontesDeDados    (csv, sintetico com semente; politica de consumo)
 ├ planoDeCarga     (lista de fases: rampa, patamar, pico, taxa constante)
 ├ passos           (lista de Passo, com peso opcional)
 └ slo              (por passo e global)

Passo = { nome, protocolo, configuracao, captura[], asserção[], peso?, condicaoDeExecucao? }
```

Regras que sustentam a promessa:

- **Nenhum comportamento fora do modelo.** Se um recurso nao pode ser expresso como no da arvore, ele nao existe — nem na DSL. Isso e o que impede a DSL de virar um segundo produto.
- **O motor so conhece o modelo.** Ele nao sabe se o cenario veio de YAML, DSL ou importador de `.jmx`. Nao existe caminho de execucao alternativo.
- **Validacao roda sobre o modelo**, nao sobre o texto YAML. Mensagem de erro, checagem de referencia de variavel, SLO apontando para passo inexistente: um lugar so, mesma mensagem nos dois publicos.
- **O modelo serializa de volta para YAML.** Isso da tres coisas de graca: saida do importador de `.jmx`, teste de equivalencia entre os dois caminhos, e diff legivel de cenario.

### 2. O que a DSL pode a mais, e como isso continua dentro do modelo

Um sistema de dois niveis so funciona se o nivel de cima puder mais. O ponto onde a DSL vai alem e a **expressao**: valor calculado, condicao, transformacao, geracao de payload.

O modelo trata expressao como no tipado com duas realizacoes:

- `ExpressaoDeclarada` — interpolacao, JSONPath, regex, funcao de biblioteca. Exprimivel em YAML, serializavel, comparavel.
- `ExpressaoDeCodigo` — funcao fornecida pela DSL. Existe no modelo como no, com nome e assinatura, e **nao serializa para YAML** — serializa como referencia opaca com nome.

Consequencia aceita e explicita: um cenario com `ExpressaoDeCodigo` nao volta para YAML, e o relatorio de exportacao diz qual passo impediu. O contrario — YAML sempre carrega na DSL — vale sempre. Essa assimetria e a unica; a estrutura de passos, carga, dados, captura, asserção e SLO e identica nos dois.

### 3. Migracao sem reescrita

A DSL **carrega um cenario YAML existente e o modifica**:

```
carregar("cenarios/cobranca.yaml")
  .passo("consultar assinatura").comCabecalho("X-Trace", contexto -> gerarTrace())
  .executar()
```

Quem comeca no YAML e esbarra no teto nao reescreve nada: importa e estende. Esse e o requisito da tese "dois publicos, um motor".

### 4. Prova obrigatoria

Teste da Fase 6: constroi o **mesmo** cenario pelos dois caminhos e compara a representacao normalizada do modelo, campo a campo. Se divergirem, o build quebra. Sem esse teste a decisao vira intencao.

### 5. Chaves do YAML

Chaves em portugues sem acento (`cenario`, `carga`, `captura`, `espera`, `aguardar`), nomes de passo livres, e o exemplo do prompt vale como forma canonica. Ajustes feitos sobre a proposta inicial:

- `espera: { status: 200 }` e asserção sobre a resposta; `aguardar:` e passo de espera por condicao. Nomes proximos demais para coisas diferentes — na Fase 1 o primeiro passa a se chamar `verificar`, mantendo `espera` como sinonimo aceito com aviso.
- `alvo` no topo e URL base; passo pode sobrescrever com URL absoluta.
- `peso` so tem sentido dentro de um bloco de escolha ponderada; solto na lista de passos ele e ambiguo (o exemplo do prompt o usa assim). Na Fase 4 entra `escolher:` com lista de alternativas ponderadas, e `peso` solto vira erro de validacao com mensagem apontando o `escolher:`.

## Alternativas descartadas

- **Dois caminhos independentes** (YAML interpretado, DSL compilada): mais rapido de entregar, e a divergencia comeca no primeiro recurso novo. Mata a razao 2 da tese.
- **YAML gerado a partir da DSL** (DSL como fonte unica): exclui o QA da autoria — ele passa a editar artefato gerado.
- **DSL gerada a partir do YAML** (transpilacao): o dev passa a editar codigo gerado e perde o diff util; alem disso quebra na primeira `ExpressaoDeCodigo`.
- **Modelo dinamico sem tipo** (mapas aninhados): barato no comeco, empurra todo erro para o tempo de execucao e destroi o autocomplete que justifica a DSL.

## Consequencias

- Todo recurso novo custa mais: precisa de no no modelo, gramatica YAML, metodo na DSL e teste de equivalencia.
- O importador de `.jmx` nao precisa de caminho proprio — ele produz o modelo e imprime YAML.
- A serializacao do modelo vira formato publico e precisa de versionamento desde a v1.
