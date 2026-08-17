# ADR 0020 — O site tem duas linguas e uma fonte

- **Status**: aceito
- **Data**: 2026-08-17
- **Contexto de decisao**: internacionalizacao, Fase 2
- **Relacionados**: [ADR 0010](0010-idioma-do-codigo.md), [ADR 0019](0019-formato-em-ingles.md)

## Contexto

Depois da Fase 1 a ferramenta fala ingles inteira: chave, valor, comando, mensagem, relatorio e interface. A documentacao continuava so em portugues, e um QA que nao fala portugues chegava ao site, instalava, rodava a demo em ingles e voltava para uma pagina que ele nao le.

Traduzir a documentacao nao tem o mesmo custo que traduzir mensagem. Mensagem traduzida e compromisso permanente com toda frase futura; pagina traduzida envelhece sozinha, em silencio, e pagina velha e pior do que pagina ausente porque quem le acredita nela.

## Decisao

**Uma fonte, duas saidas.** O guia em ingles e o texto que vale. O guia em portugues declara no cabecalho de qual arquivo ele saiu e com que conteudo:

```
---
translated_from: 30-guides-concepts.en.md
source_hash: fb5f0a39ecbc
---
```

A build recalcula o hash do original. Quando eles divergem, ela avisa no terminal e a propria pagina abre com uma tarja dizendo ao leitor que aquele texto esta atras do ingles. **A build nao reprova**: texto que envelheceu ainda ajuda mais do que pagina que sumiu, e reprovar transformaria toda edicao no original em uma edicao obrigatoria nas duas linguas.

**O ingles fica em `/` e o portugues em `/pt-BR/`.** Quem chega sem escolher nada cai no texto que vale.

**O arquivo tem o mesmo nome nas duas arvores.** `30-guides-concepts.en.md` e `30-guides-concepts.pt-BR.md` viram `concepts.html` e `pt-BR/concepts.html`. E dai que o seletor de lingua sai de graca: ele troca o prefixo do caminho e mantem a pagina, sem tabela de equivalencia entre enderecos para alguem esquecer de atualizar.

**Um indice de busca por lingua.** Um indice unico devolveria trecho em portugues a quem le em ingles, e o resultado levaria para uma pagina que essa pessoa nao consegue ler.

**A moldura do site vive em uma tabela por lingua** (`internal/site/language.go`): menu, rodape, paginacao, rotulos do buscador, cabecalho da tabela de comandos, titulos das paginas geradas. E o unico catalogo de mensagens do projeto, e ele existe porque aqui a lingua e o conteudo da pagina, e nao uma configuracao de quem executa a ferramenta.

**As paginas geradas do schema e dos ADRs tem moldura traduzida e conteudo em ingles.** A referencia do cenario sai das descricoes do schema, que sao inglesas desde o ADR 0019; a pagina de decisoes sai do titulo dos ADRs, que continuam em portugues por decisao da tabela de camadas. Cada uma diz isso na propria introducao.

## Alternativas recusadas

**Traduzir do portugues para o ingles, mantendo o portugues como fonte.** O produto fala ingles; a fonte tem que ser a lingua em que o texto e escrito primeiro, senao toda correcao chega em ingles com um ciclo de atraso.

**Detectar a lingua do navegador e redirecionar.** Quem cai numa traducao sem pedir nao sabe que existe um original, e o `Accept-Language` de uma maquina de trabalho quase nunca descreve o que a pessoa prefere ler sobre tecnologia.

**Uma pagina so, com as duas linguas.** Dobra o tamanho de cada pagina, quebra a busca e obriga o leitor a pular metade do texto.

**Deixar a traducao envelhecer sem sinal.** E o estado natural de todo site bilingue, e e o motivo pelo qual documentacao traduzida tem fama ruim.

## Consequencia

Toda mudanca no guia em ingles deixa o portugues atrasado ate alguem traduzir, e isso aparece: no terminal de quem gera o site e na pagina de quem le. O custo de manter o portugues e visivel em vez de silencioso, e o dia em que ele nao valer mais a pena a decisao de parar sera tomada com a informacao na frente.

## Reabre a discussao

Uma terceira lingua. A estrutura aceita — `Languages` e uma lista — mas o custo de manutencao cresce por lingua, e a resposta hoje seria nao pelo mesmo motivo que a Fase 1 recusou selecao de idioma nas mensagens.
