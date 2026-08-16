# Decisoes tomadas sem consulta

Registro das decisoes tomadas durante o trabalho autonomo da noite de 2026-08-16.
Ordenado por risco: primeiro o que muda o que o usuario ve, o que contraria ADR
existente e o que e caro de reverter; depois o resto.

## A8 — caixa alta identifica variavel de ambiente, em vez de conferir o ambiente

Alternativa considerada: conferir `os.LookupEnv` em cada referencia, e recusar a que nao estivesse definida.

Por que esta: conferir o ambiente tornaria `braunrate validate` impossivel numa maquina sem o segredo, que e exatamente onde alguem confere o cenario antes de commitar. A convencao de caixa alta ja e a que a propria ferramenta escreve no `import curl`, no `record` e nos exemplos de mensageria.

Reversibilidade: medio — trocar a regra em `fromEnvironment` e um lugar so, mas cenario escrito depois disso com variavel de cenario em caixa alta deixaria de ser conferido em silencio.

Toca o usuario: sim. `validate` passa a reprovar (codigo 2) cenario com `${nome}` que nao venha de `variaveis`, `captura`, `dados` ou do ambiente. Cenario que dependia do valor vazio deixa de rodar. Documentado no README em "De onde vem cada `${variavel}`".

## A6 — `validate` avisa e `execute` recusa quando falta variavel de ambiente

Alternativa considerada: recusar nos dois, tratando variavel ausente como erro de cenario.

Por que esta: sao perguntas diferentes. `validate` trata do arquivo e costuma rodar onde o segredo nao esta — laptop antes de commitar, job de lint sem credencial —, e recusar ali tiraria o comando de uso justamente onde ele serve. `execute` e quem manda a requisicao: sair com credencial vazia devolve 401 e nada na saida liga uma coisa a outra.

Reversibilidade: barato — uma chamada a `RequireEnvironment`.

Toca o usuario: sim. `braunrate execute` passa a sair com codigo 2 sem disparar nada quando falta variavel; antes rodava inteiro e devolvia um relatorio de 100% de erro. Cenario que dependia disso precisa declarar reserva ou definir a variavel.

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

## Modo servidor — o nucleo saiu do main para uma biblioteca, e a CLI virou impressao

Alternativa considerada: o servidor chamar `scenario.ParseFile`, `engine.New` e `slo.Evaluate` por conta propria, deixando o main como estava.

Por que esta: a regra da fase e "o servidor nao acrescenta logica". Com carregar, validar, avaliar SLO e decidir codigo de saida morando no main, essa regra so poderia ser intencao — a segunda porta de entrada reimplementaria cada uma delas e as duas divergiriam na primeira mudanca. `internal/runner` torna a regra verificavel, e um teste compara os dois caminhos.

Reversibilidade: caro. Desfazer significa mover de volta cinco responsabilidades para o main e reescrever a CLI inteira em cima delas.

Toca o usuario: nao. Saida, codigos de saida e mensagens verificados iguais antes e depois.

## Modo servidor — execucao guardada so na memoria, sem arquivo de estado

Alternativa considerada: gravar o JSON de cada execucao num diretorio de trabalho, para sobreviver a reinicio.

Por que esta: "o YAML e a verdade, sem banco" e regra da fase, e um diretorio de resultados vira uma segunda fonte de verdade com ciclo de vida proprio — quem limpa, quanto guarda, o que acontece quando o formato de resultado muda de versao. Quem quiser guardar busca o JSON pela rota e grava onde quiser.

Reversibilidade: barato — a store ja e uma interface pequena.

Toca o usuario: sim. Reiniciar o servidor perde as execucoes, e `/runs/{id}` de uma execucao antiga responde 404 dizendo exatamente isso.

## Modo servidor — o stream e texto puro, nao SSE nem WebSocket

Alternativa considerada: server-sent events, que e o formato que um navegador consome sem codigo.

Por que esta: a linha de progresso do terminal ja existe e ja e a forma acordada de mostrar andamento. SSE exigiria um segundo formato dizendo a mesma coisa, e dois formatos divergem. `curl -N` le o que esta la sem biblioteca nenhuma.

Reversibilidade: barato — trocar o cabecalho e prefixar `data: `.

Toca o usuario: sim, no sentido de que nao ha `EventSource` do lado do navegador; ha `fetch` com leitura por linha.

## ADR 0003 — preparacao vira regra geral, com protocolo novo obrigado a declarar

Alternativa considerada: deixar como correcao pontual da Fase 7, sem regra no ADR.

Por que esta: o mesmo defeito ja tinha aparecido duas vezes (assinatura do consumidor na Fase 5, aperto de mao na Fase 7). Regra escrita mais teste no motor evita a terceira.

Reversibilidade: barato — o ADR ganhou uma secao 7 e o motor ganhou um teste.

Toca o usuario: nao. O comportamento ja era esse desde a correcao da Fase 7.
