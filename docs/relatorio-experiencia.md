# Relatorio de experiencia de uso

Um capitulo por fase. Cada um comeca pela autoverificacao, porque e ela que
responde ao objetivo: alguem que nunca fez teste de performance instala o
braunrate e, em dez minutos, entende o que a ferramenta faz, roda contra um
sistema dele e le o resultado sozinho.

---

# Fase 1 — Onboarding no terminal

## Autoverificacao

Percorrida com o binario da arvore, num diretorio vazio, sem estado anterior
(`~/.braunrate` nao existe; nada foi criado por sessao anterior).

### Caminho A — nunca fez teste de carga

| Passo | Comando | Tempo | A proxima acao era obvia? |
|---|---|---|---|
| 1 | `braunrate` | instantaneo | sim: a tela oferece `braunrate demo` como primeira linha |
| 2 | `braunrate demo` | 5,6 s | sim: a ultima linha oferece `--com-falha` |
| 3 | `braunrate demo --com-falha` | 10,1 s | sim |

**Dois comandos ate o primeiro relatorio lido, ~6 segundos.** Nenhuma edicao de
arquivo, nenhum segundo terminal, nenhum conhecimento previo exigido: taxa,
"95% em ate X", criterio de aceite e o efeito de dado fixo sao explicados na
propria saida, no ponto em que cada numero aparece.

### Caminho B — ja tem uma API

| Passo | Comando | O que aconteceu |
|---|---|---|
| 1 | `braunrate import curl 'curl http://127.0.0.1:8080/pedidos/1' -output meu.yaml` | gravou e apontou `braunrate debug meu.yaml` |
| 2 | `braunrate debug meu.yaml` (servico fora do ar) | **achado 1**, corrigido nesta fase |
| 3 | `braunrate target -latency=5ms` + `braunrate debug meu.yaml` | **achado 2**, corrigido nesta fase |
| 4 | `braunrate validate meu.yaml` | aponta `debug` como proximo passo |
| 5 | `braunrate execute meu.yaml` | aponta como guardar o resultado para comparar |

### Achados — os momentos em que faltou informacao

**Achado 1 — "connection refused" nao diz que falta um alvo.** A depuracao de
um cenario cujo servico nao esta no ar respondia:

```
  problema:   falha de rede
              connection refused
```

Isso e o que o sistema operacional viu, nao o que fazer. Quem esta no primeiro
cenario nao tem por que saber que um alvo precisa estar rodando em algum lugar,
nem que a ferramenta traz um embutido. Corrigido:

```
Ninguem atendeu em http://127.0.0.1:8080. Confira se o servico esta no ar e se o endereco esta certo.
Se voce ainda nao tem um servico para testar, suba o embutido em outro terminal:
  braunrate target
```

**Achado 2 — 401 sem dizer onde se declara autenticacao.** O corpo da resposta
ja aparecia (`{"erro":"token ausente ou invalido"}`), mas nada ligava aquilo ao
bloco `autenticacao:` do cenario. Este e o "e so a pessoa saber que...": quem
escreveu o cenario sabendo que o bloco existe nao ve o problema. Corrigido com o
bloco impresso pronto para colar.

**Achado 3 — a dica de proximo passo virava ruido em esteira.** A primeira
versao imprimia a linha sempre, e o script de exemplos passou a despejar um
caminho de arquivo temporario a cada execucao. Corrigido: a linha desliga com
`-quiet` e nao aparece quando o criterio de aceite reprovou — ali o proximo
passo e corrigir, e o bloco de SLO ja disse o que.

## Saida real

`braunrate` sem argumento:

```
braunrate 0.5.0 — teste de carga com medicao honesta

Nunca usou? Veja funcionando em 30 segundos:

    braunrate demo

Ja tem uma API para testar? O caminho e:

    1. braunrate import curl 'curl https://sua-api/pedidos -H "Authorization: ..."'
       (ou: braunrate new cenario.yaml, para comecar do zero)

    2. braunrate debug cenario.yaml
       roda uma vez so e mostra tudo: o que foi enviado, o que voltou, o que falhou

    3. braunrate execute cenario.yaml
       agora sim, a carga de verdade

Todos os comandos:  braunrate ajuda
```

`braunrate demo`:

```
Esta demonstracao roda contra um servico de mentira que sobe aqui mesmo, entao
voce pode experimentar sem afetar nada.

[1/3] Subindo um servico de exemplo em 127.0.0.1:8080
      Ele responde em ~5 ms, como uma API saudavel responderia.

[2/3] Rodando: 100 requisicoes por segundo, durante 5s.

      Essa e a taxa: o braunrate dispara nesse ritmo esteja o servico rapido ou
      lento — como usuarios de verdade fazem. Ferramentas que esperam a
      resposta anterior antes de mandar a proxima aliviam o sistema justamente
      quando ele esta sofrendo.

      O cenario que esta rodando ficou em demo.yaml, comentado.

[3/3] Pronto. O que os numeros dizem:

  500 requisicoes em 5s, 100 por segundo, 0.00% de erro
  Metade das respostas em ate 6.0 ms; 95% em ate 6.6 ms; a pior levou 15 ms

      Repare que nao existe media nessa linha. Media esconde: se 95 respostas
      levam 5 ms e 5 levam 2 segundos, a media da 105 ms e ninguem percebe as
      cinco lentas. "95% em ate 6.6 ms" quer dizer que 5% das pessoas
      esperaram mais que isso.

  ok    Passou: o cenario inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.

      Isso e um criterio de aceite: um limite que voce declara no arquivo. Se
      estourar, o braunrate sai com codigo 1 — da para usar direto no seu CI.

  Uma ressalva que o proprio relatorio levanta:
      o passo "consultar pedido" nao tem nenhum valor que varia — toda requisicao vai ser identica.
      Requisicao sempre igual mede o cache do alvo, nao o alvo. Em demo.yaml, troque
      /pedidos/1 por /pedidos/${id} e declare de onde ${id} vem.

Relatorio completo: demo-relatorio.html
Os dois arquivos ficaram aqui no diretorio atual; apague quando quiser.

Quer ver a ferramenta pegando um problema de verdade?

    braunrate demo --com-falha
```

`braunrate demo --com-falha`, a partir de `[4/4]`:

```
[4/4] Mesma pausa, mesmo tipo de alvo, mesma requisicao, duas medicoes:

      laco fechado (JMeter, Locust):  99% em ate 7.2 ms sobre 503 requisicoes
      braunrate (modelo aberto):      99% em ate 1953.8 ms sobre 500 requisicoes

      1946.6 ms escondidos pelo laco fechado.

      O laco fechado nao mente por bug. Quando o alvo trava, ele para de
      enviar, e as requisicoes que deveriam ter partido nunca entram na conta —
      inclusive as que um usuario de verdade teria mandado. O braunrate conta
      do instante em que a requisicao deveria ter partido, entao a pausa
      aparece.

  500 requisicoes em 5s, 100 por segundo, 0.00% de erro
  Metade das respostas em ate 6.3 ms; 95% em ate 1761.3 ms; a pior levou 2008 ms

  ok    Passou: o cenario inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.
  FALHA Falhou: o cenario inteiro respondeu 95% em ate 1761 ms, acima do limite de 100 ms.

      Se isto fosse o seu CI, o braunrate teria saido com codigo 1 e a esteira
      reprovaria. Com a medicao de laco fechado, o mesmo criterio passaria.
```

Flag errada — o erro que o autor cometeu na primeira volta:

```
$ braunrate target -addr :8080
"-addr" nao existe. Voce quis dizer "-address"?

    braunrate target -address :8080

Todas as opcoes: braunrate target -h
```

## Criterio de aceitacao

| Item | Verificado como |
|---|---|
| `braunrate` sem argumento diz qual e o proximo comando | saida colada acima |
| `braunrate demo` funciona sem preparo e ensina taxa, "95% em ate X" e criterio de aceite | `TestTheDemoRunsWithNoPreparationAndExplainsWhatItMeasured` roda em `t.TempDir()` e exige as tres frases |
| `--com-falha` mostra a tese sem precisar do README | `TestTheFailingDemoShowsWhatTheClosedLoopHides` exige a linha de milissegundos escondidos e a reprovacao |
| Nenhum comando termina sem apontar o que fazer em seguida | `new`, `import`, `record` e `debug` ja apontavam; `validate`, `execute`, `report`, `serve` e o `debug` que falha por rede ou por 401 passaram a apontar |
| Flag errada sugere a certa | `TestUnknownFlagSuggestsTheRightOneAndRebuildsTheCommand`, mais `TestAFlagWithNoRelativeGetsNoGuess` para nao virar palpite |
| A demonstracao sobrevive a 8080 ocupado | `TestTheDemoSurvivesABusyPort` ocupa a porta e exige o aviso de troca |
| O cenario que a demonstracao escreve e um cenario comum | `TestTheDemoWritesScenariosTheToolAccepts` faz parse e validacao dos dois |
| Vocabulario respeitado no relatorio | `TestTheReportSpeaksTheVocabulary` reprova "latencia", "percentil", "saturad" e outros no terminal e no HTML |

## Contagem de linhas

| Arquivo | Linhas de producao |
|---|---|
| `internal/demo/demo.go` | 298 |
| `internal/demo/cenario.go` | 69 |
| `internal/selfcheck/laco_fechado.go` | 66 |
| `cmd/braunrate/main.go` (saldo) | +180 |
| `internal/texto/proximo.go` (regra de abreviacao) | +25 |
| **Total** | **~638** |

Alvo 400, teto 700. **Ficou acima do alvo e abaixo do teto.** O que passou do
alvo: cerca de 190 das 367 linhas de `internal/demo` sao texto em portugues
dentro de literais — a narracao e o proprio produto desta fase, nao acessorio
dela. Das 180 de `main.go`, 50 sao as duas telas de texto. Cortar escopo
significaria cortar explicacao, que e exatamente o que a fase existe para
entregar.

## Decisoes registradas

Em [decisoes-experiencia.md](decisoes-experiencia.md): 1 a 8, incluindo as tres
colisoes de vocabulario com o formato ja publicado, o exemplo congelado que
teve as frases reescritas sem tocar em numero, e o desligamento da dica de
proximo passo.

## O que foi recusado

**Uma tela de demonstracao com numeros ilustrativos.** Seria mais rapida, mais
bonita e nao precisaria subir alvo. Toda linha da `demo` sai de uma execucao de
verdade, inclusive a comparacao com o laco fechado — que roda um laco fechado
de verdade contra um segundo alvo, e nao uma estimativa do que ele reportaria.

**Rodar a demonstracao a partir de um cenario montado em Go.** Deixaria o
diretorio limpo. Ensinaria que existe um caminho que nao passa por arquivo, e
deixaria quem gostou do resultado sem nada para editar.
