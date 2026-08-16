# ADR 0006 — GraphQL: a operação é a unidade de medida, e erro em 200 é erro

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 4
- **Relacionados**: [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [ADR 0004](0004-extensao-de-protocolo.md), [principios de produto](../principios-de-produto.md) §3

## Contexto

Em GraphQL, tudo chega no mesmo lugar: `POST /graphql`. Duas consequencias quebram uma ferramenta de carga que trata GraphQL como HTTP generico:

1. **Agregar por URL junta tudo numa linha so.** A consulta mais barata do catalogo e a mutation mais cara de pagamento entram na mesma media. Um p99 de 40 ms pode ser a media de 39 ms de consulta e 900 ms de pagamento, e o relatorio nao mostra nada de errado.
2. **O erro vem com status 200.** A especificacao manda responder `200` com um campo `errors` no corpo. Uma ferramenta que classifica por status HTTP aprova um servico que esta respondendo erro em 100% das requisicoes. Esse e o mesmo tipo de falha da omissao coordenada: o numero fica bom porque a medicao foi generosa.

## Decisao

**A chave de agregacao e o nome da operacao. O corpo da resposta decide se foi erro.**

1. **Uma linha de relatorio por operacao**, com a chave `graphql <NomeDaOperacao>` — nunca a URL. O nome sai da propria consulta (`query ConsultarPedido` vira `graphql ConsultarPedido`), entao o caso comum nao exige nenhuma configuracao a mais.
2. **Operacao anonima e recusada na leitura do cenario**, com mensagem que mostra a forma certa. Aceitar operacao anonima seria aceitar que todas as operacoes caissem numa linha so — o problema que este ADR existe para evitar. E o unico caso em que o braunrate recusa algo que o servidor aceitaria.
3. **`errors` nao vazio e erro**, classe `graphql`, mesmo com status 200. O detalhe carrega o codigo (`extensions.code`), a mensagem e o caminho do campo.
4. **Resposta parcial (`data` e `errors` juntos) e erro declarado como parcial.** Contar como sucesso seria dizer que a requisicao entregou o que foi pedido; contar sem dizer que veio parcial esconderia que parte da resposta chegou.
5. **Erro de transporte continua sendo erro de transporte.** Status 4xx/5xx e classificado como `status`, nao como `graphql`: quem le precisa distinguir "o gateway recusou" de "o resolver falhou".
6. **Corpo que nao e JSON de GraphQL** (pagina de erro do balanceador com status 200, por exemplo) e erro, nao sucesso silencioso.

O passo GraphQL usa o mesmo motor, o mesmo modelo de chegada, o mesmo histograma e o mesmo relatorio dos outros protocolos. Nao ha caso especial no motor: tudo isso vive no protocolo, como manda o [ADR 0003](0003-modelo-de-execucao-e-metrica.md).

## Alternativas descartadas

- **Tratar GraphQL como HTTP com corpo JSON**: funciona para disparar carga e falha exatamente onde importa — agrega tudo numa linha e aprova erro em 200. Foi o que motivou o ADR.
- **Deixar o usuario declarar a chave de agregacao**: transferiria para quem escreve o cenario uma decisao que a ferramenta sabe tomar, e o esquecimento produziria relatorio errado em silencio.
- **Aceitar operacao anonima e agregar por hash da consulta**: a linha do relatorio viraria `graphql a3f9c1` — ilegivel para quem precisa ler o resultado.
- **Contar resposta parcial como sucesso** (comportamento de varias ferramentas): otimista por construcao.

## Consequencias

- O `slo` de um cenario GraphQL usa o nome da operacao (`graphql ConsultarPedido`), o que mantem a regra estavel mesmo se o endpoint mudar.
- A classe de erro `graphql` aparece no relatorio como "erro no corpo da resposta GraphQL (com status 200)" — o texto diz o que aconteceu para quem nunca ouviu falar da especificacao.
- Fica pendente: **persisted queries** (envio por hash), **multipart/subscriptions** e **batch de operacoes numa requisicao**. O batch e o caso mais delicado, porque uma requisicao carregaria varias operacoes e a chave de agregacao deixaria de ser unica; entra depois da v1, com modelo proprio.
- O braunrate nao valida a consulta contra o schema: erro de sintaxe volta do servidor como erro de GraphQL, ja classificado.
