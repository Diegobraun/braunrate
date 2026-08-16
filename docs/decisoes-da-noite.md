# Decisoes tomadas sem consulta

Registro das decisoes tomadas durante o trabalho autonomo da noite de 2026-08-16.
Ordenado por risco: primeiro o que muda o que o usuario ve, o que contraria ADR
existente e o que e caro de reverter; depois o resto.

## A8 — caixa alta identifica variavel de ambiente, em vez de conferir o ambiente

Alternativa considerada: conferir `os.LookupEnv` em cada referencia, e recusar a que nao estivesse definida.

Por que esta: conferir o ambiente tornaria `braunrate validate` impossivel numa maquina sem o segredo, que e exatamente onde alguem confere o cenario antes de commitar. A convencao de caixa alta ja e a que a propria ferramenta escreve no `import curl`, no `record` e nos exemplos de mensageria.

Reversibilidade: medio — trocar a regra em `fromEnvironment` e um lugar so, mas cenario escrito depois disso com variavel de cenario em caixa alta deixaria de ser conferido em silencio.

Toca o usuario: sim. `validate` passa a reprovar (codigo 2) cenario com `${nome}` que nao venha de `variaveis`, `captura`, `dados` ou do ambiente. Cenario que dependia do valor vazio deixa de rodar. Documentado no README em "De onde vem cada `${variavel}`".

## A8 — a referencia quebrada aponta a coluna dela, nao o inicio da linha

Alternativa considerada: usar a posicao do escalar, que e o que o resto do parser faz.

Por que esta: uma linha com tres referencias mandaria a pessoa para a primeira quando a quebrada e a terceira. O calculo soma o deslocamento dentro do escalar e desiste (volta para o inicio) quando o valor atravessa linhas.

Reversibilidade: barato — uma funcao.

Toca o usuario: sim, a coluna do erro fica diferente da que o resto do parser produz para o mesmo tipo de linha.

## A8 — campo de fonte sintetica e conferido; coluna de CSV, nao

Alternativa considerada: abrir o CSV durante a validacao para conferir o cabecalho.

Por que esta: abrir arquivo na validacao transforma `validate` em algo que depende do disco e do caminho relativo de quem roda. A fonte sintetica declara os campos no proprio YAML, entao ali da para conferir sem custo.

Reversibilidade: barato.

Toca o usuario: sim, so na direcao de recusar mais: `${fonte.campo}` de fonte `gerar:` com campo inexistente agora reprova.

## ADR 0003 — preparacao vira regra geral, com protocolo novo obrigado a declarar

Alternativa considerada: deixar como correcao pontual da Fase 7, sem regra no ADR.

Por que esta: o mesmo defeito ja tinha aparecido duas vezes (assinatura do consumidor na Fase 5, aperto de mao na Fase 7). Regra escrita mais teste no motor evita a terceira.

Reversibilidade: barato — o ADR ganhou uma secao 7 e o motor ganhou um teste.

Toca o usuario: nao. O comportamento ja era esse desde a correcao da Fase 7.
