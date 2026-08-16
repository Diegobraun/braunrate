# ADR 0013 — Gravador de tráfego: o que ele infere e como declara isso

## Contexto

Escrever o primeiro cenario a partir de uma tela e trabalho manual: abrir o
devtools, copiar requisicao por requisicao, descobrir de onde veio cada
identificador. E o passo onde a maioria desiste antes de chegar na medicao.

Gravador de trafego resolve isso, e e tambem onde ferramenta de carga costuma
produzir lixo. O recorder do JMeter transcreve: grava `GET /pedidos/9912` com o
token daquela sessao e, na segunda execucao, o cenario quebra — o token expirou
e o pedido 9912 pertence a outra pessoa. O que sai de la parece um cenario e nao
e.

## Decisao

`braunrate record -output cenario.yaml` sobe um proxy HTTP local. O que ele
entrega vai alem da transcricao, e cada inferencia e declarada:

1. **Correlacao sugerida.** Valor que nasceu em uma resposta e reaparece em uma
   chamada seguinte vira `captura` no passo que o produziu e `${variavel}` no
   que o consome. A linha sai com comentario dizendo que e sugestao a conferir.
   Sem isso, o cenario gravado quebra na segunda execucao.
2. **Filtro de ruido.** Recurso estatico, favicon, telemetria, preflight de CORS
   e dominio de fora ficam de fora, e a contagem por motivo aparece na tela.
   Ruido gravado vira carga contra CDN, e o relatorio passa a descrever algo que
   ninguem quis medir.
3. **Agrupamento por rota.** `/pedidos/9912` e `/pedidos/8123` viram um passo so,
   com os valores observados indo para um CSV ao lado do cenario. Um passo por
   identificador daria uma linha por requisicao no relatorio, em vez de uma
   linha por operacao.
4. **Segredo nunca no arquivo.** Mesma regra do `import curl`, sem excecao — e a
   regra ganhou o que faltava: campo de corpo com nome de senha, token ou
   segredo tambem vira `${variavel}` de ambiente. O `import curl` vazava senha
   de corpo desde a Fase 6.
5. **Aviso na partida.** Carga e SLO saem como chute, e uma sequencia gravada uma
   vez nao e o mix de producao.
6. **Nunca termina com "pronto".** Termina sempre com o proximo passo:
   `braunrate debug cenario.yaml`.

O passo tambem sai com `verificar: { status: N }` com o status que o alvo de
fato respondeu. Escrever 200 para todo passo reprovaria o cenario na primeira
execucao contra o proprio servico de onde ele foi gravado.

## Fora de escopo, com o motivo

**HTTPS com certificado proprio.** Gravar dentro de TLS exige o braunrate emitir
certificado e a maquina confiar nele. Instalar autoridade certificadora e uma
mudanca no armazem de confianca do sistema — e o tipo de coisa que uma
ferramenta de carga nao deve automatizar em silencio. A conexao e encaminhada
para o cliente continuar funcionando, e o numero de conexoes nao gravadas
aparece na tela por host. Documentar a instalacao da CA fica para quando houver
demanda; automatizar, nao.

**Trafego de aplicativo movel.** Fora da v1. Depende de pinning de certificado e
de configuracao por sistema operacional, e cada combinacao e um caminho
diferente de suporte.

## Consequencias

O gravador nunca afirma: ele sugere e marca. Quem le o arquivo distingue o que
passou pelo fio do que a ferramenta inferiu.

O `record` nao escreve cenario vazio. Se nada foi gravado, ele diz que nao vai
escrever e aponta as duas causas provaveis — trafego HTTPS ou dominio fora do
filtro.
