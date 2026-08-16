# Decisoes de experiencia de uso

Toda decisao que muda o que o usuario le ou ve, contraria um ADR, ou e cara de
reverter. Decisao, alternativa descartada, por que esta, reversibilidade, e se
toca o usuario.

## 1. `vazao` continua existindo como metrica de SLO

- **Decisao**: o [vocabulario](vocabulario.md) fixa **taxa** e proibe "vazao" no
  texto ao usuario, mas a chave `vazao` do bloco `slo` fica como esta.
- **Alternativa**: renomear a chave e aceitar `vazao` como apelido silencioso.
- **Por que esta**: mudar chave do YAML e mudanca de formato, fora do escopo de
  experiencia de uso; e `vazao` ja e apelido de `taxa_efetiva`, entao quem
  escreve cenario novo tem o nome certo disponivel. O texto que a ferramenta
  imprime diz "taxa efetiva" nos dois casos.
- **Reversibilidade**: alta — e uma linha no `switch` que le a metrica.
- **Toca o usuario**: nao, enquanto ninguem le a documentacao antiga.

## 2. Percentil vira "95% das respostas" na frase do criterio de aceite

- **Decisao**: as frases de veredito passam de `todas as requisicoes (p95) de
  6 ms` para `respondeu 95% em ate 6 ms`.
- **Alternativa**: manter `p95` no texto, por ser o termo da area.
- **Por que esta**: quem nunca fez teste de carga nao sabe o que e p95, e a
  frase de veredito e onde o numero decide se o CI passa. O leitor que ja sabe
  perde nada: o `p95` continua na chave do YAML, no JSON e no CSV.
- **Reversibilidade**: alta — e a construcao da frase, nao o dado.
- **Toca o usuario**: sim, direto na linha do SLO.

## 3. `saturacao` vira "o gerador nao sustentou a taxa"

- **Decisao**: a classe de erro `saturacao` e impressa como "o gerador nao
  sustentou a taxa".
- **Alternativa**: "gerador saturado", que era o texto anterior.
- **Por que esta**: "saturado" nao diz de quem e a culpa. A frase nova diz que o
  problema esta no gerador, nao no alvo — que e a leitura que muda a acao.
- **Reversibilidade**: alta.
- **Toca o usuario**: sim, na tabela de erros.

## 3b. O exemplo congelado teve as frases reescritas, nao os numeros

- **Decisao**: em `docs/exemplo-resultado.json`, as frases de veredito e a
  mensagem de aviso foram reescritas no texto novo, e `versao` passou a 0.5.0.
  Nenhuma medicao foi tocada.
- **Alternativa**: gerar um exemplo novo rodando de novo.
- **Por que esta**: uma execucao nova daria numeros diferentes, e o exemplo
  publicado e citado por numero no README. As frases sao renderizacao dos
  mesmos numeros — o que mudou foi como o codigo de hoje as escreve.
- **Reversibilidade**: alta, o arquivo esta versionado.
- **Toca o usuario**: sim, e a primeira coisa que alguem abre para decidir se a
  ferramenta presta.

## 4. `braunrate demo` sobe alvo, grava cenario e roda, tudo em um comando

- **Decisao**: existe um comando que nao pede arquivo, nao pede alvo e nao pede
  segundo terminal.
- **Alternativa**: so documentar o caminho `target` num terminal e `execute` no
  outro, como o README ja fazia.
- **Por que esta**: o proprio autor precisou de dois terminais e nao rodou
  nenhuma medicao na primeira volta com o binario publicado. Um caminho que o
  autor nao percorre sozinho nao vai ser percorrido por quem nunca fez teste de
  carga.
- **Reversibilidade**: media — e um comando novo com um pacote proprio, e sai
  inteiro se sair.
- **Toca o usuario**: sim, e a primeira coisa que a tela inicial oferece.

## 5. A demo grava o cenario que ela roda, em vez de rodar de memoria

- **Decisao**: `braunrate demo` escreve `demo.yaml` no diretorio atual e executa
  esse arquivo, pelo mesmo caminho de codigo do `execute`.
- **Alternativa**: montar a `Spec` em Go e rodar sem tocar em disco, o que
  deixaria o diretorio limpo.
- **Por que esta**: principio 1 do produto — o cenario e a verdade. Uma demo que
  roda algo que nao existe como arquivo ensina que existe um caminho secreto, e
  deixa quem gostou do resultado sem nada para editar.
- **Reversibilidade**: alta.
- **Toca o usuario**: sim, cria arquivo no diretorio de trabalho, e a saida diz
  qual.

## 6. A dica de proximo passo do `execute` desliga com `-quiet`

- **Decisao**: `braunrate execute` termina apontando o proximo comando, e a
  linha some com `-quiet` ou quando a execucao reprovou.
- **Alternativa**: imprimir sempre.
- **Por que esta**: principio 4 da hierarquia — quem ja sabe nao pode ser
  atrapalhado. Numa esteira a linha vira ruido em log, e quando o criterio de
  aceite reprovou o proximo passo e corrigir, nao comparar.
- **Reversibilidade**: alta.
- **Toca o usuario**: sim, no fim de toda execucao interativa.

## 7. `debug` que nao alcanca o alvo aponta `braunrate target`

- **Decisao**: falha de rede na depuracao imprime que ninguem atendeu naquele
  endereco e oferece o alvo embutido.
- **Alternativa**: manter "falha de rede / connection refused", que e o que o
  sistema operacional viu.
- **Por que esta**: apareceu na autoverificacao da Fase 1 — quem esta no
  primeiro cenario nao tem por que saber que um alvo precisa estar no ar em
  algum lugar, nem que a ferramenta traz um.
- **Reversibilidade**: alta.
- **Toca o usuario**: sim.

## 7b. `debug` que toma 401 mostra o bloco `autenticacao`

- **Decisao**: quando o alvo responde 401 ou 403 e o cenario nao declara
  autenticacao, a depuracao imprime o bloco pronto.
- **Alternativa**: deixar como estava — o corpo da resposta ja aparecia, com
  `{"erro":"token ausente ou invalido"}` visivel.
- **Por que esta**: apareceu na autoverificacao, no caminho `import curl` de
  quem tem uma API. O corpo diz o que faltou; nada dizia onde declarar. Quem
  escreveu o cenario sabendo que a ferramenta tem esse bloco nao ve o problema,
  e essa e a definicao do "e so a pessoa saber que...".
- **Reversibilidade**: alta.
- **Toca o usuario**: sim.

## 8. Flag desconhecida sugere a certa em vez de despejar a lista

- **Decisao**: `braunrate target -addr :8080` responde `"-addr" nao existe. Voce
  quis dizer "-address"?`, com o comando corrigido pronto para copiar.
- **Alternativa**: o comportamento padrao do pacote `flag`, que imprime a lista
  inteira de opcoes.
- **Por que esta**: e o mesmo erro que o autor cometeu na primeira volta, e a
  ferramenta ja tinha a resposta — a sugestao por distancia de edicao existia
  desde a validacao de cenario, presa dentro do pacote `scenario`.
- **Reversibilidade**: alta.
- **Toca o usuario**: sim, em todo comando com opcoes.
- **Efeito colateral aceito**: a funcao de semelhanca ganhou regra de
  abreviacao (`addr` casa `address`), e ela e a mesma que sugere chave de
  cenario. Agora `car` sugere `carga`, o que antes nao acontecia. Prefixo de
  tres letras ou mais e sinal forte o bastante para nao virar palpite errado.

## 9. Tudo que a pessoa le sai acentuado; o que ela copia para o arquivo, nao

- **Decisao**: terminal, relatorio HTML, demonstracao, mensagens de recusa,
  site e README passam a sair em portugues acentuado. Chave de YAML, campo de
  JSON, nome de rota, valor de enum (`lider`, `basica`, `aleatorio`, `contem`) e
  nome de arquivo continuam em ASCII.
- **Alternativa**: manter tudo sem acento, como estava desde a Fase 0.
- **Por que esta**: texto sem acento em portugues le como rascunho, e o produto
  pede confianca no numero que ele mostra. A fronteira e o que a pessoa **copia
  de volta para o arquivo**: se a mensagem ensina `senha: ${BROKER_SENHA}`, o que
  ela ensina precisa carregar no parser.
- **Reversibilidade**: media. A varredura foi automatica sobre os literais de
  string, com as excecoes acima conferidas uma a uma pela suite.
- **Toca o usuario**: sim, em toda saida.
- **Efeito colateral**: as saidas coladas na documentacao ficaram desatualizadas
  no mesmo instante. Foram regeradas rodando os comandos, e nao reescritas a mao.

## 10. A interface e um editor do arquivo, e mostra o comando equivalente

- **Decisao**: `braunrate ui` abre o `.yaml` do diretorio numa area de texto,
  valida o rascunho pela mesma leitura do terminal e grava o texto como ele esta.
  O comando de terminal equivalente fica no topo de toda tela.
- **Alternativa**: uma arvore de campos, com formulario por bloco do cenario.
- **Por que esta**: registrada em [ADR 0018](adr/0018-interface-como-editor-do-arquivo.md).
  Formulario que serializa de volta apaga comentario e destroi o diff, que e o
  defeito do `.jmx` e o motivo do importador existir.
- **Reversibilidade**: alta enquanto a interface nao guardar nada que o arquivo
  nao guarde.
- **Toca o usuario**: sim, e e a primeira tela de quem nao usa terminal.
