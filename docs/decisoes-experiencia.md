# Decisões de experiência de uso

Toda decisão que muda o que o usuário lê ou vê, contraria um ADR, ou é cara de
reverter. Decisão, alternativa descartada, por que esta, reversibilidade, e se
toca o usuário.

## 1. `vazão` continua existindo como métrica de SLO

- **Decisão**: o [vocabulário](vocabulario.md) fixa **taxa** e proíbe "vazão" no
  texto ao usuário, mas a chave `vazao` do bloco `slo` fica como está.
- **Alternativa**: renomear a chave e aceitar `vazão` como apelido silencioso.
- **Por que esta**: mudar chave do YAML é mudança de formato, fora do escopo de
  experiência de uso; e `vazao` já é apelido de `taxa_efetiva`, então quem
  escreve cenário novo tem o nome certo disponível. O texto que a ferramenta
  imprime diz "taxa efetiva" nos dois casos.
- **Reversibilidade**: alta — é uma linha no `switch` que lê a métrica.
- **Toca o usuário**: não, enquanto ninguém lê a documentação antiga.

## 2. Percentil vira "95% das respostas" na frase do critério de aceite

- **Decisão**: as frases de veredito passam de `todas as requisições (p95) de
  6 ms` para `respondeu 95% em até 6 ms`.
- **Alternativa**: manter `p95` no texto, por ser o termo da área.
- **Por que esta**: quem nunca fez teste de carga não sabe o que é p95, e a
  frase de veredito é onde o número decide se o CI passa. O leitor que já sabe
  não perde nada: o `p95` continua na chave do YAML, no JSON e no CSV.
- **Reversibilidade**: alta — é a construção da frase, não o dado.
- **Toca o usuário**: sim, direto na linha do SLO.

## 3. `saturacao` vira "o gerador não sustentou a taxa"

- **Decisão**: a classe de erro `saturacao` é impressa como "o gerador não
  sustentou a taxa".
- **Alternativa**: "gerador saturado", que era o texto anterior.
- **Por que esta**: "saturado" não diz de quem é a culpa. A frase nova diz que o
  problema está no gerador, não no alvo — que é a leitura que muda a ação.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, na tabela de erros.

## 3b. O exemplo congelado teve as frases reescritas, não os números

- **Decisão**: em `docs/exemplo-resultado.json`, as frases de veredito e a
  mensagem de aviso foram reescritas no texto novo, e `versao` passou a 0.5.0.
  Nenhuma medição foi tocada.
- **Alternativa**: gerar um exemplo novo rodando de novo.
- **Por que esta**: uma execução nova daria números diferentes, e o exemplo
  publicado é citado por número no README. As frases são renderização dos
  mesmos números — o que mudou foi como o código de hoje as escreve.
- **Reversibilidade**: alta, o arquivo está versionado.
- **Toca o usuário**: sim, é a primeira coisa que alguém abre para decidir se a
  ferramenta presta.

## 4. `braunrate demo` sobe alvo, grava cenário e roda, tudo em um comando

- **Decisão**: existe um comando que não pede arquivo, não pede alvo e não pede
  segundo terminal.
- **Alternativa**: só documentar o caminho `target` num terminal e `execute` no
  outro, como o README já fazia.
- **Por que esta**: o próprio autor precisou de dois terminais e não rodou
  nenhuma medição na primeira volta com o binario publicado. Um caminho que o
  autor não percorre sozinho não vai ser percorrido por quem nunca fez teste de
  carga.
- **Reversibilidade**: média — é um comando novo com um pacote próprio, e sai
  inteiro se sair.
- **Toca o usuário**: sim, é a primeira coisa que a tela inicial oferece.

## 5. A demo grava o cenário que ela roda, em vez de rodar de memória

- **Decisão**: `braunrate demo` escreve `demo.yaml` no diretório atual e executa
  esse arquivo, pelo mesmo caminho de código do `execute`.
- **Alternativa**: montar a `Spec` em Go e rodar sem tocar em disco, o que
  deixaria o diretorio limpo.
- **Por que esta**: princípio 1 do produto — o cenário é a verdade. Uma demo que
  roda algo que não existe como arquivo ensina que existe um caminho secreto, e
  deixa quem gostou do resultado sem nada para editar.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, cria arquivo no diretório de trabalho, e a saída diz
  qual.

## 6. A dica de próximo passo do `execute` desliga com `-quiet`

- **Decisão**: `braunrate execute` termina apontando o próximo comando, e a
  linha some com `-quiet` ou quando a execução reprovou.
- **Alternativa**: imprimir sempre.
- **Por que esta**: princípio 4 da hierarquia — quem já sabe não pode ser
  atrapalhado. Numa esteira a linha vira ruído em log, e quando o critério de
  aceite reprovou o próximo passo é corrigir, não comparar.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, no fim de toda execução interativa.

## 7. `debug` que não alcanca o alvo aponta `braunrate target`

- **Decisão**: falha de rede na depuracao imprime que ninguém atendeu naquele
  endereço e oferece o alvo embutido.
- **Alternativa**: manter "falha de rede / connection refused", que é o que o
  sistema operacional viu.
- **Por que esta**: apareceu na autoverificação da Fase 1 — quem está no
  primeiro cenário não tem por que saber que um alvo precisa estar no ar em
  algum lugar, nem que a ferramenta traz um.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim.

## 7b. `debug` que toma 401 mostra o bloco `autenticação`

- **Decisão**: quando o alvo responde 401 ou 403 e o cenário não declara
  autenticação, a depuracao imprime o bloco pronto.
- **Alternativa**: deixar como estava — o corpo da resposta já aparecia, com
  `{"erro":"token ausente ou inválido"}` visível.
- **Por que esta**: apareceu na autoverificação, no caminho `import curl` de
  quem tem uma API. O corpo diz o que faltou; nada dizia onde declarar. Quem
  escreveu o cenário sabendo que a ferramenta tem esse bloco não vê o problema,
  e essa é a definição do "é só a pessoa saber que...".
- **Reversibilidade**: alta.
- **Toca o usuário**: sim.

## 8. Flag desconhecida sugere a certa em vez de despejar a lista

- **Decisão**: `braunrate target -addr :8080` responde `"-addr" não existe. Você
  quis dizer "-address"?`, com o comando corrigido pronto para copiar.
- **Alternativa**: o comportamento padrão do pacote `flag`, que imprime a lista
  inteira de opções.
- **Por que esta**: é o mesmo erro que o autor cometeu na primeira volta, e a
  ferramenta já tinha a resposta — a sugestao por distância de edicao existia
  desde a validação de cenário, presa dentro do pacote `scenario`.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, em todo comando com opções.
- **Efeito colateral aceito**: a função de semelhanca ganhou regra de
  abreviação (`addr` casa `address`), e ela é a mesma que sugere chave de
  cenário. Agora `car` sugere `carga`, o que antes não acontecia. Prefixo de
  três letras ou mais é sinal forte o bastante para não virar palpite errado.

## 9. Tudo que a pessoa lê sai acentuado; o que ela copia para o arquivo, não

- **Decisão**: terminal, relatório HTML, demonstração, mensagens de recusa,
  site e README passam a sair em português acentuado. Chave de YAML, campo de
  JSON, nome de rota, valor de enum (`lider`, `basica`, `aleatorio`, `contem`) e
  nome de arquivo continuam em ASCII.
- **Alternativa**: manter tudo sem acento, como estava desde a Fase 0.
- **Por que esta**: texto sem acento em português lê como rascunho, e o produto
  pede confiança no número que ele mostra. A fronteira é o que a pessoa **copia
  de volta para o arquivo**: se a mensagem ensina `senha: ${BROKER_SENHA}`, o que
  ela ensina precisa carregar no parser.
- **Reversibilidade**: média. A varredura foi automática sobre os literais de
  string, com as exceções acima conferidas uma a uma pela suíte.
- **Toca o usuário**: sim, em toda saída.
- **Efeito colateral**: as saídas coladas na documentação ficaram desatualizadas
  no mesmo instante. Foram regeradas rodando os comandos, e não reescritas à mão.

## 10. A interface é um editor do arquivo, e mostra o comando equivalente

- **Decisão**: `braunrate ui` abre o `.yaml` do diretório numa área de texto,
  valida o rascunho pela mesma leitura do terminal e grava o texto como ele está.
  O comando de terminal equivalente fica no topo de toda tela.
- **Alternativa**: uma árvore de campos, com formulário por bloco do cenário.
- **Por que esta**: registrada em [ADR 0018](adr/0018-interface-como-editor-do-arquivo.md).
  Formulário que serializa de volta apaga comentário e destrói o diff, que é o
  defeito do `.jmx` e o motivo do importador existir.
- **Reversibilidade**: alta enquanto a interface não guardar nada que o arquivo
  não guarde.
- **Toca o usuário**: sim, e é a primeira tela de quem não usa terminal.

## 11. Exemplo de YAML dentro da mensagem sai em ASCII, mesmo quando a frase é acentuada

- **Decisão**: dentro de uma mensagem, o trecho que a pessoa copia para o
  arquivo — nome de chave, lista de chaves aceitas, valor de enum — sai em ASCII,
  ainda que a frase ao redor esteja acentuada. `rampa precisa de 'de' e 'ate',
  por exemplo: - rampa: { de: 50/s, ate: 300/s, durante: 30s }`.
- **Alternativa**: acentuar a mensagem inteira, incluindo o exemplo.
- **Por que esta**: a decisão 9 já traçava essa fronteira, e a varredura passou
  por cima dela em nove lugares: o erro de rampa ensinava `até:`, o de kafka
  ensinava `tópico:` e `partição:`, o de graphql ensinava `operação:` e
  `variáveis:`, o de captura ensinava `padrão`, e o esqueleto do `braunrate new`
  saía com `# autenticação:` e `corpo: { usuário: ana }`. Nenhum desses carrega
  no parser: a mensagem que existe para desbloquear estava ensinando o erro
  seguinte.
- **Reversibilidade**: alta, e agora auditável — a regra é comparar cada palavra
  acentuada de um trecho de exemplo com o conjunto de chaves que o parser aceita.
- **Toca o usuário**: sim, e exatamente no momento em que ele já está travado.
- **Efeito colateral**: o exemplo congelado voltou a dizer `usuario: texto` na
  forma de corpo observada, porque ali não é frase: é o nome do campo que o
  cenário enviou.
