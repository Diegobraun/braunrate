# Relatório de experiência de uso

Um capítulo por fase. Cada um começa pela autoverificação, porque é ela que
responde ao objetivo: alguém que nunca fez teste de performance instala o
braunrate e, em dez minutos, entende o que a ferramenta faz, roda contra um
sistema dele e lê o resultado sozinho.

---

# Fase 1 — Onboarding no terminal

## Autoverificação

Percorrida com o binário da árvore, num diretório vazio, sem estado anterior
(`~/.braunrate` não existe; nada foi criado por sessão anterior).

### Caminho A — nunca fez teste de carga

| Passo | Comando | Tempo | A próxima ação era óbvia? |
|---|---|---|---|
| 1 | `braunrate` | instantâneo | sim: a tela oferece `braunrate demo` como primeira linha |
| 2 | `braunrate demo` | 5,6 s | sim: a última linha oferece `--com-falha` |
| 3 | `braunrate demo --com-falha` | 10,1 s | sim |

**Dois comandos até o primeiro relatório lido, ~6 segundos.** Nenhuma edição de
arquivo, nenhum segundo terminal, nenhum conhecimento prévio exigido: taxa,
"95% em até X", critério de aceite e o efeito de dado fixo são explicados na
própria saída, no ponto em que cada número aparece.

### Caminho B — já tem uma API

| Passo | Comando | O que aconteceu |
|---|---|---|
| 1 | `braunrate import curl 'curl http://127.0.0.1:8080/pedidos/1' -output meu.yaml` | gravou e apontou `braunrate debug meu.yaml` |
| 2 | `braunrate debug meu.yaml` (serviço fora do ar) | **achado 1**, corrigido nesta fase |
| 3 | `braunrate target -latency=5ms` + `braunrate debug meu.yaml` | **achado 2**, corrigido nesta fase |
| 4 | `braunrate validate meu.yaml` | aponta `debug` como próximo passo |
| 5 | `braunrate execute meu.yaml` | aponta como guardar o resultado para comparar |

### Achados — os momentos em que faltou informação

**Achado 1 — "connection refused" não diz que falta um alvo.** A depuração de
um cenário cujo serviço não está no ar respondia:

```
  problema:   falha de rede
              connection refused
```

Isso é o que o sistema operacional viu, não o que fazer. Quem está no primeiro
cenário não tem por que saber que um alvo precisa estar rodando em algum lugar,
nem que a ferramenta traz um embutido. Corrigido:

```
Ninguém atendeu em http://127.0.0.1:8080. Confira se o serviço está no ar e se o endereço está certo.
Se você ainda não tem um serviço para testar, suba o embutido em outro terminal:
  braunrate target
```

**Achado 2 — 401 sem dizer onde se declara autenticação.** O corpo da resposta
já aparecia (`{"erro":"token ausente ou inválido"}`), mas nada ligava aquilo ao
bloco `autenticação:` do cenário. Este é o "e só a pessoa saber que...": quem
escreveu o cenário sabendo que o bloco existe não vê o problema. Corrigido com o
bloco impresso pronto para colar.

**Achado 3 — a dica de próximo passo virava ruído em esteira.** A primeira
versão imprimia a linha sempre, e o script de exemplos passou a despejar um
caminho de arquivo temporário a cada execução. Corrigido: a linha desliga com
`-quiet` e não aparece quando o critério de aceite reprovou — ali o próximo
passo é corrigir, e o bloco de SLO já disse o que.

## Saída real

`braunrate` sem argumento:

```
braunrate dev — teste de carga com medição honesta

Nunca usou? Veja funcionando em 30 segundos:

    braunrate demo

Já tem uma API para testar? O caminho é:

    1. braunrate import curl 'curl https://sua-api/pedidos -H "Authorization: ..."'
       (ou: braunrate new cenario.yaml, para começar do zero)

    2. braunrate debug cenario.yaml
       roda uma vez só e mostra tudo: o que foi enviado, o que voltou, o que falhou

    3. braunrate execute cenario.yaml
       agora sim, a carga de verdade

Todos os comandos:  braunrate ajuda
```

`braunrate demo`:

```

Esta demonstração roda contra um serviço de mentira que sobe aqui mesmo, então
você pode experimentar sem afetar nada.

      (127.0.0.1:8080 está ocupado, então o alvo subiu em 127.0.0.1:62439)

[1/3] Subindo um serviço de exemplo em 127.0.0.1:62439
      Ele responde em ~5 ms, como uma API saudável responderia.

[2/3] Rodando: 100 requisições por segundo, durante 5s.

      Essa é a taxa: o braunrate dispara nesse ritmo esteja o serviço rápido ou
      lento — como usuários de verdade fazem. Ferramentas que esperam a
      resposta anterior antes de mandar a próxima aliviam o sistema justamente
      quando ele está sofrendo.

      O cenário que está rodando ficou em demo.yaml, comentado.

[3/3] Pronto. O que os números dizem:

  500 requisições em 5s, 100 por segundo, 0.00% de erro
  Metade das respostas em até 6.7 ms; 95% em até 7.3 ms; a pior levou 14 ms

      Repare que não existe média nessa linha. Média esconde: se 95 respostas
      levam 5 ms e 5 levam 2 segundos, a média dá 105 ms e ninguém percebe as
      cinco lentas. "95% em até 7.3 ms" quer dizer que 5% das pessoas
      esperaram mais que isso.

  ok    Passou: o cenário inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.

      Isso é um critério de aceite: um limite que você declara no arquivo. Se
      estourar, o braunrate sai com código 1 — dá para usar direto no seu CI.

  Uma ressalva que o próprio relatório levanta:
      o passo "consultar pedido" não tem nenhum valor que varia — toda requisição vai ser idêntica.
      Requisição sempre igual mede o cache do alvo, não o alvo. Em demo.yaml, troque
      /pedidos/1 por /pedidos/${id} e declare de onde ${id} vem.

Relatório completo: demo-relatorio.html
Os dois arquivos ficaram aqui no diretório atual; apague quando quiser.

Quer ver a ferramenta pegando um problema de verdade?

    braunrate demo --com-falha
```

`braunrate demo --com-falha`, a partir de `[4/4]`:

```

Esta demonstração mede o mesmo serviço travado de duas formas, e mostra o que
cada uma reporta.

      (127.0.0.1:8080 está ocupado, então o alvo subiu em 127.0.0.1:62442)

[1/4] Subindo um serviço de exemplo em 127.0.0.1:62442, com uma diferença: ele
      trava por 2 segundos no meio da execução. É o que um GC longo, um lock
      ou um failover fazem com um serviço de verdade.

[2/4] Rodando o braunrate: 100 por segundo durante 5s.

[3/4] Agora um laço fechado, contra um serviço idêntico que trava igual.
      Laço fechado é como JMeter e Locust medem: a próxima requisição só sai
      depois que a anterior responde.

[4/4] Mesma pausa, mesmo tipo de alvo, mesma requisição, duas medições:

      laço fechado (JMeter, Locust):  99% em até 7.5 ms sobre 504 requisições
      braunrate (modelo aberto):      99% em até 1962.0 ms sobre 500 requisições

      1954.5 ms escondidos pelo laço fechado.

      O laço fechado não mente por bug. Quando o alvo trava, ele para de
      enviar, e as requisições que deveriam ter partido nunca entram na conta —
      inclusive as que um usuário de verdade teria mandado. O braunrate conta
      do instante em que a requisição deveria ter partido, então a pausa
      aparece.

  500 requisições em 5s, 100 por segundo, 0.00% de erro
  Metade das respostas em até 7.0 ms; 95% em até 1761.3 ms; a pior levou 2003 ms

      Repare que não existe média nessa linha. Média esconde: se 95 respostas
      levam 5 ms e 5 levam 2 segundos, a média dá 105 ms e ninguém percebe as
      cinco lentas. "95% em até 1761.3 ms" quer dizer que 5% das pessoas
      esperaram mais que isso.

  ok    Passou: o cenário inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.
  FALHA Falhou: o cenário inteiro respondeu 95% em até 1761 ms, acima do limite de 100 ms.

      Isso é um critério de aceite: um limite que você declara no arquivo. Se
      estourar, o braunrate sai com código 1 — dá para usar direto no seu CI.

      Se isto fosse o seu CI, o braunrate teria saído com código 1 e a esteira
      reprovaria. Com a medição de laço fechado, o mesmo critério passaria.

Relatório completo: demo-com-falha-relatorio.html
```

Flag errada — o erro que o autor cometeu na primeira volta:

```
$ braunrate target -addr :8080
"-addr" não existe. Você quis dizer "-address"?

    braunrate target -address :8080

Todas as opções: braunrate target -h
```

## Critério de aceitação

| Item | Verificado como |
|---|---|
| `braunrate` sem argumento diz qual é o próximo comando | saída colada acima |
| `braunrate demo` funciona sem preparo e ensina taxa, "95% em até X" e critério de aceite | `TestTheDemoRunsWithNoPreparationAndExplainsWhatItMeasured` roda em `t.TempDir()` e exige as três frases |
| `--com-falha` mostra a tese sem precisar do README | `TestTheFailingDemoShowsWhatTheClosedLoopHides` exige a linha de milissegundos escondidos e a reprovação |
| Nenhum comando termina sem apontar o que fazer em seguida | `new`, `import`, `record` e `debug` já apontavam; `validate`, `execute`, `report`, `serve` e o `debug` que falha por rede ou por 401 passaram a apontar |
| Flag errada sugere a certa | `TestUnknownFlagSuggestsTheRightOneAndRebuildsTheCommand`, mais `TestAFlagWithNoRelativeGetsNoGuess` para não virar palpite |
| A demonstração sobrevive a 8080 ocupado | `TestTheDemoSurvivesABusyPort` ocupa a porta e exige o aviso de troca |
| O cenário que a demonstração escreve é um cenário comum | `TestTheDemoWritesScenariosTheToolAccepts` faz parse e validação dos dois |
| Vocabulário respeitado no relatório | `TestTheReportSpeaksTheVocabulary` reprova "latência", "percentil", "saturad" e outros no terminal e no HTML |

## Contagem de linhas

| Arquivo | Linhas de produção |
|---|---|
| `internal/demo/demo.go` | 298 |
| `internal/demo/cenario.go` | 69 |
| `internal/selfcheck/laco_fechado.go` | 66 |
| `cmd/braunrate/main.go` (saldo) | +180 |
| `internal/texto/proximo.go` (regra de abreviação) | +25 |
| **Total** | **~638** |

Alvo 400, teto 700. **Ficou acima do alvo e abaixo do teto.** O que passou do
alvo: cerca de 190 das 367 linhas de `internal/demo` são texto em português
dentro de literais — a narração é o próprio produto desta fase, não acessório
dela. Das 180 de `main.go`, 50 são as duas telas de texto. Cortar escopo
significaria cortar explicação, que é exatamente o que a fase existe para
entregar.

## Decisões registradas

Em [decisões-experiência.md](decisoes-experiencia.md): 1 a 8, incluindo as três
colisões de vocabulário com o formato já publicado, o exemplo congelado que
teve as frases reescritas sem tocar em número, e o desligamento da dica de
próximo passo.

## O que foi recusado

**Uma tela de demonstração com números ilustrativos.** Seria mais rápida, mais
bonita e não precisaria subir alvo. Toda linha da `demo` sai de uma execução de
verdade, inclusive a comparação com o laço fechado — que roda um laço fechado
de verdade contra um segundo alvo, e não uma estimativa do que ele reportaria.

**Rodar a demonstração a partir de um cenário montado em Go.** Deixaria o
diretório limpo. Ensinaria que existe um caminho que não passa por arquivo, e
deixaria quem gostou do resultado sem nada para editar.

---

# Fase 2 — A documentação publicada

## Autoverificação

Percorrida no site publicado em <https://diegobraun.github.io/braunrate/>, sem
abrir o repositório: se a resposta não estivesse na página, ela não existia.

| Pergunta de quem chega | Caminho | Cliques | A resposta estava lá? |
|---|---|---|---|
| "quero ver funcionando antes de instalar" | capa, primeira seção | 0 | sim: `braunrate demo` é a primeira linha |
| "instalar exige o quê?" | capa → Instalação | 1 | sim: três caminhos, e a seção do que fica de fora |
| "meu cenário devolve 401 em tudo" | capa → Solução de problemas → tabela de sintomas | 2 | sim: causa, o bloco `autenticacao` pronto e a saída real |
| "como faço isso reprovar o build?" | capa → Receitas → índice da página | 2 | sim, com os quatro códigos de saída |
| "o que exatamente vai dentro de `carga`?" | capa → Referência do cenário | 2 | sim: a página é gerada do esquema, chave por chave |
| "qual opção do `execute` grava o JSON?" | capa → Comandos → índice de comandos | 2 | sim |

Nenhuma pergunta precisou de três cliques, e nenhuma precisou de busca — que o
site não tem.

### Achados — os momentos em que a página não bastou

**Achado 4 — a primeira versão foi reprovada pelo autor: "não parece
profissional, parece amador".** O conteúdo estava lá; a estrutura de
documentação, não. Nenhuma página tinha índice, aviso era parágrafo comum no
meio do texto, e a página de problemas era uma sequência de sintomas sem porta
de entrada. Reescrita tomando como referência documentação de ferramentas
conhecidas: índice no topo de cada guia, destaques marcados (`Nota`, `Atenção`,
`Importante`, `Dica`), tabela de sintoma → seção na página de problemas, âncora
clicável em cada título, e cada seção de problema com **Causa** e **Solução**
separadas.

**Achado 5 — o texto das Receitas foi reprovado: "parece muito AI".** As
receitas descreviam a solução sem nomear o problema, e uma não levava à outra.
Reescritas com a mesma forma: o problema em uma frase, o YAML, o que ele
produz, e o que ele **não** resolve.

**Achado 6 — bloco de código claro dentro do tema escuro.** O realce de sintaxe
viaja dentro do próprio HTML, porque a página não busca nada da rede, e o tema
claro pintava fundo branco no meio da página escura. Corrigido com uma paleta só
nos dois temas e o fundo do bloco declarado no CSS.

**Achado 7 — link interno quebrado depois de renomear um título.** O link não
avisou nada: abriu a página no topo, como se estivesse certo. Corrigido, e
`TestEveryInternalLinkResolves` passou a reprovar o commit.

**Achado 8 — a varredura de acentuação quebrou o layout.** Ela acentuou
`class="versão"` e `src="página.js"`, que são nome de classe e nome de arquivo,
e o site caiu para uma coluna. Nenhum teste olhava layout; foi visto abrindo a
página. A regra ficou na decisão 9, e a verificação continua sendo abrir.

## Saída real

Geração do site a partir do repositório:

```
$ go run ./cmd/site -out site
site em site
```

Estado das páginas publicadas, conferido por HTTP:

```
$ for p in "" instalacao.html primeiros-15-minutos.html conceitos.html \
           protocolos.html receitas.html comandos.html problemas.html \
           referencia.html decisoes.html estilo.css pagina.js; do
    printf "%-28s %s\n" "$p" "$(curl -s -o /dev/null -w '%{http_code}' \
      https://diegobraun.github.io/braunrate/$p)"
  done
                             200
instalacao.html              200
primeiros-15-minutos.html    200
conceitos.html               200
protocolos.html              200
receitas.html                200
comandos.html                200
problemas.html               200
referencia.html              200
decisoes.html                200
estilo.css                   200
pagina.js                    200
```

## Critério de aceitação

| Item | Verificado como |
|---|---|
| Todo bloco de cenário publicado é um cenário que a ferramenta aceita | `TestEveryScenarioBlockIsAScenarioTheToolAccepts` passa cada bloco pelo parser |
| Todo trecho solto usa chave que ainda existe | `TestEveryFragmentUsesKeysThatStillExist` |
| Toda chave do formato chega à referência | `TestEverySchemaKeyReachesTheReference`, contra `docs/braunrate.schema.json` |
| Todo comando publicado nos guias existe, com as opções que ele imprime | `TestEveryPublishedCommandExists`, contra o binário de verdade |
| Nenhuma página busca nada da rede | `TestThePagesFetchNothingFromTheNetwork` |
| Toda página tem título e corpo | `TestEveryPageHasATitleAndABody` |
| Todo link interno resolve | `TestEveryInternalLinkResolves` |
| O site gera no portão, antes de publicar | passo "o site de documentacao gera" no `ci.yml` |
| A publicação sai do mesmo repositório | workflow `paginas.yml`, e as doze respostas 200 acima |

## Contagem de linhas

| Arquivo | Linhas de produção |
|---|---|
| `internal/site/site.go` | 326 |
| `internal/site/referencia.go` | 266 |
| `internal/site/decisoes.go` | 66 |
| `cmd/site/main.go` | 24 |
| **Go** | **682** |
| `internal/site/estilo.css` | 214 |
| `internal/site/pagina.js` | 44 |
| Guias (`docs/guias/*.md`, 8 páginas) | 1714 |

Alvo 300, teto 500. **Ficou acima do teto, em 682 linhas de Go.**

O que passou: 332 dessas linhas não transformam markdown em HTML. `referencia.go`
gera a referência do formato a partir de `docs/braunrate.schema.json`, e
`decisoes.go` monta a lista de decisões a partir dos arquivos de `docs/adr`.
Escritas à mão, seriam uma terceira lista de chaves e uma segunda lista de ADRs
— e a terceira lista é sempre a que envelhece. Sem elas, o gerador tem 350
linhas: ainda acima do alvo, dentro do teto.

O que eu cortaria se o teto fosse rígido: nada em `site.go` sem perder a
verificação de que todo link interno resolve, que já pegou um link quebrado
neste mesmo ciclo.

## Decisões registradas

Em [decisões-experiência.md](decisoes-experiencia.md): a decisão 9 (fronteira da
acentuação) e, já na autoverificação da Fase 3, a decisão 11 (exemplo de YAML
dentro da mensagem sai em ASCII).

## O que foi recusado

**Gerador de site pronto (Hugo, MkDocs, Docusaurus).** Ganharia busca, tema e
navegação de graça. Custaria outro runtime e outro passo de build para publicar
a documentação de um binário único, e a documentação deixaria de ser conferida
pelos mesmos testes que conferem o produto.

**Busca no cliente com índice carregado da página.** É a falta mais sentida do
site. Foi recusada nesta fase porque a alternativa disponível é buscar um script
de fora, o que quebra a regra de rede fechada; um índice próprio é escopo de
outra fase, e o `Ctrl+F` da página resolve o caso comum enquanto isso.

**Página de referência escrita à mão.** Seria mais bonita e mais curta. Seria
também a terceira lista de chaves do projeto.

**Documentação versionada por release.** Só existe uma versão suportada, e
diretório por versão sem ninguém para mantê-lo vira link morto.

---

# Fase 3 — `braunrate ui`

## Autoverificação

Percorrida como quem não abre o terminal por vontade própria: só o comando de
subir a interface, e dali em diante o navegador.

| Passo | Onde | A próxima ação era óbvia? |
|---|---|---|
| 1 | `braunrate ui` no diretório dos cenários | sim: o terminal imprime o endereço, e o navegador abre sozinho |
| 2 | tela inicial, lista de cenários da pasta | sim, e a pasta vazia diz "nenhum nesta pasta" com dois botões: começar do zero e ver a demonstração |
| 3 | formulário de "começar do zero" | sim: nove campos com valor de partida, e o arquivo gravado sai comentado |
| 4 | editor, com validação do rascunho enquanto digita | sim: o erro aparece com linha e coluna, o mesmo texto do terminal |
| 5 | "rodar uma vez" (a depuração) antes da carga | sim: é o botão à esquerda do de carga, e a ordem está na tela |
| 6 | execução, saída chegando ao vivo | sim: a linha de situação vira `terminou: passou` |
| 7 | relatório dentro da página | sim, e o comando `braunrate execute … -html relatorio.html` fica no topo |

O comando equivalente aparece em todas as telas: `braunrate ui -dir …`,
`braunrate new cenario.yaml`, `braunrate validate <nome>`, `braunrate debug
<nome>`, `braunrate execute <nome>`, `braunrate compare antes.json depois.json`.
Quem começou pelo navegador sai sabendo o CLI sem ter procurado.

### Achados

**Achado 9 — o convite para abrir o navegador saía antes da porta existir.** Com
`8080` ocupado, a interface imprimia o aviso inteiro, inclusive `Abra no
navegador: http://127.0.0.1:8080`, e só depois `bind: address already in use` —
e com `-open`, o navegador chegava a abrir na porta de outro processo. Corrigido:
o bind acontece antes do aviso, e porta ocupada ganhou mensagem que ensina a
escolher outra, no `ui`, no `serve` e no `target`.

**Achado 10 — a mensagem que ensina estava ensinando errado.** A validação do
rascunho na interface é a mesma leitura do terminal, então ela herdou nove
mensagens em que a varredura de acentuação tinha acentuado nome de chave: quem
copiasse `- rampa: { de: 50/s, até: 300/s }` da própria mensagem tomaria o erro
seguinte. Corrigido, e a regra virou teste contra o esquema publicado.

## Saída real

Subida da interface:

```
$ braunrate ui -dir ./cenarios -addr 127.0.0.1:8124
braunrate ui em http://127.0.0.1:8124, editando os cenários de ./cenarios
Sem autenticação e sem TLS: qualquer um que alcance esta porta pode disparar carga contra os alvos dos cenários.
Foi feito para rodar em 127.0.0.1. Expor em outra interface é outra decisão, e ela ainda não foi tomada.
Gravação ligada: quem alcançar esta porta pode alterar os arquivos de cenário de ./cenarios.

Abra no navegador:
  http://127.0.0.1:8124
```

Porta ocupada, depois da correção:

```
$ braunrate ui -dir ./cenarios -addr 127.0.0.1:8125
127.0.0.1:8125 já está ocupado por outro processo. Escolha outra porta:
  braunrate ui -addr 127.0.0.1:8081
```

O que a interface faz, visto pelas rotas que ela usa. O rascunho é conferido
pela leitura do terminal, a partir do texto e não do arquivo gravado:

```
$ curl -s -X POST --data-binary @rascunho.yaml \
    http://127.0.0.1:8124/scenarios/cenario.yaml/validate
{
  "valid": false,
  "message": "erro no cenário: cenario.yaml:2:1: chave desconhecida no topo do cenário: \"passos\"\n    disponíveis: nome, alvo, requer, variaveis, autenticacao, tls, mensageria, dados, carga, cenario, slo\n...",
  "line": 2,
  "column": 1
}
```

Gravar é gravar o texto, e o arquivo em disco fica com os mesmos bytes:

```
$ curl -s -X PUT --data-binary @novo.yaml \
    http://127.0.0.1:8124/scenarios/cenario.yaml/text
{
  "bytes": 499,
  "name": "cenario.yaml"
}
$ wc -c cenarios/cenario.yaml
     499 cenarios/cenario.yaml
```

E a execução disparada pela tela é a mesma do terminal, com o mesmo veredito:

```
$ curl -s http://127.0.0.1:8124/runs
{
  "runs": [
    {
      "id": "r001",
      "scenario": "cenario.yaml",
      "name": "Consulta de pedidos",
      "status": "done",
      "exit_code": 0,
      "verdict": "passou",
      "started_at": "2026-08-16T21:12:53.412991+01:00",
      "summary": { "errors": 0, "requests": 500, "valid": true }
    }
  ]
}
```

## Critério de aceitação

| Item | Verificado como |
|---|---|
| A interface viaja dentro do binário, sem etapa de build | `TestTheInterfaceTravelsInsideTheBinary`, sobre os arquivos embarcados |
| Recarregar em qualquer rota devolve a página | `TestAnyRouteAnswersWithThePage` |
| A página não busca nada da rede | `TestThePageFetchesNothingFromTheNetwork` |
| O editor é uma área de texto sobre o arquivo, não uma árvore de campos | `TestTheEditorIsATextAreaOverTheFile` |
| O texto que volta é o arquivo em disco | `TestTheTextThatComesBackIsTheFileOnDisk` |
| Edição por fora é o que a interface lê | `TestAnEditFromOutsideIsWhatTheInterfaceReads` |
| Sem `Writable`, nada é gravado | `TestWithoutWritableNothingIsWritten` |
| O rascunho é conferido pela mesma leitura do terminal | `TestTheDraftIsCheckedByTheSameReadingAsTheTerminal` |
| Só arquivo de cenário é gravado | `TestOnlyScenarioFilesAreWritten` |
| Porta ocupada não convida a abrir o navegador | `TestABusyPortSaysHowToChooseAnother` |
| A tela ensina os cinco conceitos, e desliga como o `-quiet` | `TestTheScreenTeachesTheSameFiveIdeasTheTerminalTeaches` |
| Vazio, carregando e erro são tela, não silêncio | `nenhum nesta pasta`, `carregando…`, `rodando uma iteração…`, `O cenário não fecha`, `Não gravei`, `Não comecei` |

## Contagem de linhas

| Arquivo | Linhas de produção |
|---|---|
| `internal/ui/app/app.js` | 485 |
| `internal/ui/app/estilo.css` | 112 |
| `internal/ui/app/index.html` | 39 |
| `internal/ui/ui.go` | 31 |
| `internal/server/server.go` (saldo) | +98 |
| `cmd/braunrate/main.go` (saldo) | +63 |
| `internal/runner/runner.go` (saldo) | +18 |
| **Total** | **847** |

Alvo 1500, teto 2500. **Ficou abaixo do alvo**, e o motivo é a decisão de
formato: como a interface edita o texto do arquivo, não existe camada que
traduza árvore de campos para YAML e de volta — que é onde mora a maior parte do
código de uma interface de teste de carga.

Testes: `internal/ui/ui_test.go` (115) e o acréscimo em
`internal/server/server_test.go` (116). Documentação:
[ADR 0018](adr/0018-interface-como-editor-do-arquivo.md) (75) e a seção `ui` no
guia de comandos (31).

## Decisões registradas

Decisão 10 em [decisões-experiência.md](decisoes-experiencia.md) e a
[ADR 0018](adr/0018-interface-como-editor-do-arquivo.md).

## O que foi recusado

**Árvore de campos como formato de autoria.** É o defeito do `.jmx`: o arquivo
salvo deixa de ser legível, o diff vira ruído e o comentário que a pessoa
escreveu some na primeira serialização. Um cenário que só a interface consegue
escrever é um cenário que o time não versiona.

**Estado próprio da interface** — rascunho guardado no navegador, sessão,
projeto. O que não está no arquivo não vai para o repositório e não roda no CI.

**Framework de frontend.** Uma dependência de CDN quebraria a regra de rede
fechada; um empacotador quebraria a regra de binário único.

**Autenticação, multiusuário e exposição fora de 127.0.0.1.** A porta não tem
autenticação nem TLS, o aviso de inicialização declara isso, e expor é uma
decisão que ainda não foi tomada.

---

# O que continua difícil para quem está começando

Fecho pelo que as três fases **não** resolveram. Cada item foi observado nesta
autoverificação, não imaginado. Dois dos seis foram fechados depois da revisão,
e ficam registrados aqui com o que foi feito.

**Fechado — experimentar contra o alvo embutido dava 401 em tudo.** `braunrate
new` escreve um cenário que consulta `/pedidos/1`, e o alvo embutido pedia token
nessa rota: `new`, `target` e `execute`, o caminho mais provável de quem chega,
devolvia 401 em toda requisição, e a explicação só aparecia para quem tivesse
rodado a depuração. `/pedidos` deixou de pedir credencial; `/faturas` e
`/graphql` continuam pedindo, e é por elas que a jornada autenticada segue sendo
exercitada. Hoje a sequência inteira passa:

```
passo 1 — consultar pedido   [ok em 7.7ms]
  requisição: GET /pedidos/1
  resposta:   status 200, 89 bytes

Iteração completa: 1 passo, tudo certo. Para rodar com carga:
  braunrate execute cenario.yaml
```

```
SLO
  ok    Passou: "consultar pedido" respondeu 95% em até 7 ms, dentro do limite de 200 ms.
  ok    Passou: o cenário inteiro teve taxa de erro de 0.00%, dentro do limite de 1.00%.
```

Junto foi o aviso do `target`, que mandava rodar `examples/ci.yaml` — arquivo que
só existe para quem clonou o repositório. Agora ele aponta `braunrate new`.
Decisão 12, e `TestTheScenarioNewWritesAnswersOnTheEmbeddedTarget` reprova o
commit que voltar a exigir token nessa rota.

**Fechado — a interface não ensinava termo nenhum.** O terminal explica cinco
conceitos no ponto em que o número aparece; a tela não explicava nenhum. Os
cinco passaram a ter uma linha no campo em que aparecem: taxa e dado fixo no
formulário de carga e de caminho, "95% em até X" e critério de aceite nos dois
limites do SLO, e resultado inválido no veredito da execução. Desligam juntas na
caixa "explicações" do topo — por sessão, como o `-quiet`, sem guardar
preferência que o arquivo não guarda. Decisão 13.

**Fechado — o site não tinha busca.** As seis perguntas do roteiro foram
respondidas em no máximo dois cliques, mas todas eram perguntas cuja página dava
para adivinhar pelo nome; quem chega com a mensagem de erro na mão procura pelo
texto do erro. O site passou a ter busca com índice gerado na build e servido
junto — `/` ou `Ctrl+K`, resultado com o trecho onde a palavra apareceu, nada
buscado da rede. Buscar `401` traz "Todas as requisições voltam 401 ou 403" em
primeiro lugar. Decisão 16.

**1. Ler o relatório inteiro ainda pede vocabulário.** Jornada, sanidade,
variedade observada e taxa efetiva são explicados no lugar em que aparecem, mas
são muitos blocos numa tela só, e a ordem de leitura está no guia — não na
página.

**2. Kafka e RabbitMQ continuam exigindo um broker de verdade.** O alvo embutido
sobe HTTP e sobe o processador assíncrono, mas só se você já tiver um Kafka
apontado. Para os dois protocolos de mensageria não existe caminho de dez
minutos.

**3. Windows não foi verificado.** As três fases foram percorridas em macOS. O
binário é publicado para Windows, o `-open` tem o ramo do `rundll32`, e nada
disso foi aberto numa máquina Windows por uma pessoa que nunca usou a
ferramenta. Enquanto não for, a promessa dos dez minutos vale para macOS e
Linux.
