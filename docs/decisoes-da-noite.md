# Decisoes tomadas sem consulta

Registro das decisoes tomadas durante o trabalho autonomo da noite de 2026-08-16.
Ordenado por risco: primeiro o que muda o que o usuario ve, o que contraria ADR
existente e o que e caro de reverter; depois o resto.

## Revisao dos ADRs — a regra de dependencia de `metrics` passa a dizer o que o codigo faz, com a divida nomeada

Alternativa considerada: cumprir a regra como estava escrita, movendo `ErrorClass` e `ConsumerLag` para `metrics` e invertendo o import.

Por que esta: `docs/arquitetura.md` dizia que um import de `protocol` dentro de `metrics` e erro de arquitetura, e esse import existe desde muito antes desta noite, por causa de `ErrorClass`. Inverter mexeria em todos os protocolos de uma vez, de madrugada, para separar um vocabulario que nao pertence a protocolo nenhum. O que a regra protege de verdade e `metrics` conhecer **um** protocolo — e isso agora e teste, nao frase. Fica registrada a divida que sobra: `metrics/variety.go` escreve a frase de particao do Kafka, e as duas saidas obvias estao fechadas (generalizar perde o conselho util; deixar o protocolo escrever contraria o ADR 0003 §3).

Reversibilidade: barato como texto e teste; a inversao de dependencia continua possivel depois, e mais cara.

Toca o usuario: nao. Nenhuma saida muda.

## Revisao dos ADRs — o cenario em Go so roda de dentro deste modulo, e o README passa a dizer isso

Alternativa considerada: expor motor e documento de resultado como API publica, para o cenario em Go rodar de um modulo de fora.

Por que esta: o README mostrava `motor.Novo(...)` como se qualquer projeto pudesse importar, e o motor vive em `internal/` — o exemplo nao compila fora daqui, e ainda usava os nomes em portugues de antes do ADR 0010. Decidir o que vira API publica e decisao de v1, quando a interface vira contrato versionado (ADR 0004); inventar essa superficie sozinho de madrugada seria a decisao mais cara de reverter da noite. O que da para fazer agora e o exemplo compilar e a limitacao aparecer escrita.

Reversibilidade: barato — e texto; expor o motor depois nao desfaz nada.

Toca o usuario: sim. O trecho em Go do README trocou de nomes (`dsl.New`, `Target`, `Auth`, `Build`, `engine.New`) e ganhou uma limitacao conhecida logo abaixo. O que estava publicado nao compilava em lugar nenhum.

## Item 8 — forma de corpo entra na variedade pelo mesmo cano dos valores

Alternativa considerada: uma metrica separada, com contador, secao e aviso proprios.

Por que esta: a forma do corpo e uma variedade observada como qualquer outra — conta distintos, tem teto, vira frase e vira aviso. Passa-la pelo `RecordUses` que ja existe, com o prefixo `corpo.` no nome, deu a metrica inteira sem maquina nova. O preco e que o nome da variavel virou um espaco reservado: quem tiver uma variavel chamada `corpo.algo` colide.

Reversibilidade: medio — a metrica sai facil, mas o campo `formas_observadas` ja tera sido gravado em resultado de quem rodou.

Toca o usuario: sim. Corpo com campo vazio passa a render aviso de gravidade media, entao execucao que saia limpa passa a sair com uma observacao. Nao muda codigo de saida: media nao invalida.

## Item 8 — forma unica de corpo nao rende linha no relatorio

Alternativa considerada: imprimir a forma de todo passo com corpo, para o relatorio ficar completo.

Por que esta: todo passo com corpo tem exatamente uma forma no caso normal. Uma linha por passo em toda execucao soterraria as linhas que dizem alguma coisa — que e a mesma razao pela qual o ADR 0007 se recusa a avisar sobre fonte que so tem um valor. A forma continua no JSON; o que fica de fora e a frase.

Reversibilidade: barato — `Notable()` e um metodo so, consultado nos dois relatorios.

Toca o usuario: sim, por omissao: a forma de corpo so aparece na tela quando ha mais de uma ou quando algum campo saiu vazio.

## Item 8 — o prefixo comum so e declarado a partir de 4 caracteres

Alternativa considerada: declarar qualquer prefixo comum, por menor que fosse.

Por que esta: dois ids que comecam com o mesmo digito compartilham um prefixo de 1 caractere, e dizer isso em toda execucao com valor numerico em texto encheria o bloco de ambiente de frase sem conteudo. Quatro caracteres e onde um prefixo comeca a parecer com "mesmo cliente", "mesma regiao", "mesmo tenant".

Reversibilidade: barato — uma constante.

Toca o usuario: sim, no que aparece: valores com prefixo curto em comum nao ganham a frase de faixa.

## Conserto — a frase da comparacao parava de afirmar mais do que ela sabe

Alternativa considerada: deixar como estava, porque nenhum teste reclamava.

Por que esta: duas frases afirmavam coisa que a comparacao nao tinha apurado. "Nao da para comparar ... porque o gerador saturou" era dito para qualquer execucao invalida, inclusive as que reportaram jornada incompleta ou passo 100% falho — causa errada na tela. E "Com N ressalvas que podem explicar a diferenca sozinhas" era dito de toda ressalva, quando o campo `impede_comparacao` existe justamente para separar as que explicam das que so mudaram.

Reversibilidade: barato — duas funcoes em `comparison.go`, com teste cobrindo cada uma.

Toca o usuario: sim, no texto. A frase de execucao invalida passa a citar o achado que a execucao registrou; a de ressalva nao impeditiva vira "Com 1 ressalva sobre o que mudou fora do servico". Saidas publicadas no README e em `docs/api-servidor.md` foram regeradas com o texto novo.

## Item 7 — comparacao que nao vale nao mostra tabela nenhuma

Alternativa considerada: mostrar os numeros com um aviso em cima, como o relatorio de execucao invalida faz.

Por que esta: o relatorio de uma execucao invalida ainda tem numeros que descrevem o que aconteceu com o gerador. Uma comparacao entre duas execucoes em que uma nao vale nao tem nada: cada linha seria uma diferenca entre um numero e um numero que nao existe. A pagina mostra a identificacao das duas, as ressalvas e para por ai.

Reversibilidade: barato — e um `{{if .Comparable}}` no template.

Toca o usuario: sim. Quem abrir a comparacao em HTML de um par com execucao invalida ve identificacao e ressalvas, e nenhuma tabela. O terminal ja se comportava assim; a pagina segue o terminal.

## Item 7 — a folha de estilo virou template compartilhado entre as duas paginas

Alternativa considerada: copiar o CSS para o template da comparacao.

Por que esta: duas paginas da mesma ferramenta com aparencia diferente fazem quem le se perguntar qual das duas e a certa. Copia envelhece em dobro.

Reversibilidade: barato — `estiloDaPagina` e um `define` num arquivo so.

Toca o usuario: nao. O HTML gerado e o mesmo, fora duas quebras de linha; `docs/exemplo-relatorio.html` foi regerado.

## Item 6 — particao declarada deixa de invalidar a execucao e passa a avisar

Alternativa considerada: manter a gravidade alta, que e o que a concentracao numa particao produz hoje quando a chave nao varia.

Por que esta: com `particao: N` a concentracao foi pedida, e a mensagem antiga mandava variar a chave — conselho que nao resolve nada quando a chave esta sendo ignorada de proposito. A variedade passa a ser contada com outro nome, `kafka.particao.declarada.<topico>`, e o aviso diz que o numero e o de uma particao e nao o do topico.

Reversibilidade: medio — sao duas linhas em `VarietyWarnings` e o nome do atributo em `kafka.go`, mas resultado ja gravado em JSON carrega o nome antigo e nao ganha o novo aviso ao ser reprocessado com `braunrate report`.

Toca o usuario: sim. Execucao com `particao` declarada sai com codigo 0 onde a regra anterior daria alta gravidade, e o nome que aparece em "variedade observada" e no JSON muda de `kafka.particao.<topico>` para `kafka.particao.declarada.<topico>`. Chave que nao varia continua invalidando como antes.

## Item 6 — `grupo` observa o consumidor, e nunca consome

Alternativa considerada: subir um consumidor proprio no grupo declarado para medir o atraso lendo as mensagens.

Por que esta: entrar no grupo tiraria particao do servico que esta sendo medido, e o numero mediria a ferramenta. O atraso e lido do broker (marca d'agua alta menos offset confirmado), sem participar do grupo, entao a medicao nao muda o que mede. Particao em que o grupo nunca confirmou fica de fora: zero ali afirmaria que esta em dia.

Reversibilidade: barato — `grupo` e opcional e a secao some do relatorio quando ninguem declara.

Toca o usuario: sim, so somando. Duas chaves novas no passo `kafka` (`particao` e `grupo`), autorizadas pelo item 6 da lista, e uma secao "Atraso do consumidor" no terminal e no HTML quando `grupo` existe.

## Item 6 — o relogio do atraso e fechado na hora de reportar, nao no fim do processo

Alternativa considerada: deixar a ultima leitura para o `Close()` do protocolo, que e onde o cancelamento ja acontecia.

Por que esta: o motor monta o documento antes de fechar os protocolos, entao o "atraso no fim" publicado seria uma leitura do meio da execucao. `ConsumerLag()` fecha a observacao e espera a ultima amostra — o numero que interessa e o de depois que a carga parou.

Reversibilidade: barato — e uma funcao no protocolo Kafka, sem formato envolvido.

Toca o usuario: sim, no valor: "no fim" passa a ser o atraso depois do fim da carga, e nao o da penultima amostra.

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
