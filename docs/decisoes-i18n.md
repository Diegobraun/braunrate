# Decisões de internacionalização

Toda decisão que muda o que o usuário vê ou que é cara de reverter. Decisão,
alternativa descartada, por que esta, reversibilidade, e se toca o usuário.

O mapa completo de chaves e valores está no
[ADR 0019](adr/0019-formato-em-ingles.md); aqui ficam as decisões que o mapa não
cabe, e as das fases seguintes.

## 1. Sem mecanismo de seleção de idioma nas mensagens

- **Decisão**: terminal, relatório HTML, erros de validação, avisos, ajuda e
  interface web ficam em inglês, sem catálogo de tradução e sem `-lang`.
- **Alternativa**: extrair todo texto para catálogo com chave e oferecer os dois
  idiomas.
- **Por que esta**: mensagem traduzível é compromisso permanente — cada erro,
  aviso e explicação nova passa a precisar de duas versões, para sempre. E
  catálogo com chave afasta a frase do lugar onde ela aparece; as mensagens do
  braunrate são boas porque foram escritas olhando a saída.
- **Reversibilidade**: baixa depois de escrita a segunda leva de mensagens;
  alta agora, que é quando a decisão está sendo tomada.
- **Toca o usuário**: sim — quem lia em português passa a ler em inglês. O
  README declara que traduzir é possível e será feito se houver demanda.

## 2. O documento JSON de resultado também vai para inglês

- **Decisão**: os campos do documento de resultado passam para camelCase em
  inglês, e `formatVersion` sobe de 2 para 3. O cenário sobe de 1 para 2.
- **Alternativa**: manter os campos em português, como o ADR 0010 decidiu, por
  serem formato publicado e não tela.
- **Por que esta**: é o mesmo argumento que move o YAML. O documento é commitado
  como linha de base para `-baseline`, lido por script de CI e comparado entre
  execuções — circula do mesmo jeito que o cenário.
- **Reversibilidade**: média — são tags de struct, mas quem já commitou uma
  linha de base precisa reexecutar para ter a nova.
- **Toca o usuário**: sim, em quem automatizou leitura do JSON.

## 3. `braunrate migrate` grava por cima, com backup

- **Decisão**: o padrão é gravar por cima do arquivo original deixando
  `cenario.yaml.bak` ao lado; `-output` grava em outro arquivo e `-dry-run`
  mostra o diff sem escrever nada.
- **Alternativa**: exigir `-output` sempre, para nunca tocar no original.
- **Por que esta**: quem tem uma pasta com trinta cenários quer rodar um comando
  e continuar. Exigir destino faria o caso comum ser o mais trabalhoso, e o
  arquivo original está versionado em quem usa a ferramenta como ela pede.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, na primeira vez que ele migra.

## 4. O formato antigo é reconhecido pelo parser, não só pelo `migrate`

- **Decisão**: ao encontrar chave de topo do formato português, o parser para e
  ensina o comando de migração, em vez de dizer "chave desconhecida".
- **Alternativa**: deixar o erro genérico de chave desconhecida, que já sugere a
  chave mais próxima.
- **Por que esta**: "chave desconhecida: `autenticacao`, você quis dizer `auth`?"
  faz a pessoa corrigir onze chaves à mão sem saber que existe saída. A
  sugestão por proximidade resolve erro de digitação, não mudança de formato.
- **Reversibilidade**: alta — é uma verificação antes do laço de chaves.
- **Toca o usuário**: sim, e é o ponto.

## 5. O domínio do alvo embutido também vai para inglês

- **Decisão**: o alvo de teste embutido passa a servir `/orders`,
  `/invoices/{id}/pay`, `/auth/token` e `/health`, com `lastInvoice`, `amount`,
  `OPEN` e `PAID` no corpo; as operações GraphQL viram `LookUpOrder` e
  `PayInvoice`.
- **Alternativa**: manter `/pedidos` e `/faturas`, que são só dados de teste.
- **Por que esta**: é a primeira tela que alguém vê. `braunrate demo` imprime o
  caminho da requisição, e o relatório da demonstração nomeia o passo. Deixar o
  domínio em português colocaria uma palavra em português na tela exatamente de
  quem a pergunta final é sobre.
- **Reversibilidade**: alta, é servidor de teste.
- **Toca o usuário**: sim, na demonstração e nos exemplos.

## 6. Os exemplos publicados são traduzidos e renomeados

- **Decisão**: `examples/*.yaml` vai para inglês, com os arquivos renomeados
  (`jornada-autenticada.yaml` vira `authenticated-journey.yaml`, e assim por
  diante), `examples/dados/` vira `examples/data/`, as colunas dos CSV viram
  `id,name` e `id,type,route`, e `examples/cenario-em-go/` vira
  `examples/scenario-in-go/`.
- **Alternativa**: traduzir só as chaves e deixar nomes de passo, de fonte e de
  arquivo em português, por serem texto do autor.
- **Por que esta**: os exemplos apontam para o alvo embutido, que passou a servir
  caminhos em inglês — em português eles deixariam de rodar. E são o primeiro
  cenário que alguém lê: um `consultar pedido` ali ensina que o formato aceita
  português, que é o contrário do que a fase decidiu.
- **Reversibilidade**: média — o caminho do módulo Go do exemplo mudou, e links
  de fora do repositório para os arquivos quebram.
- **Toca o usuário**: sim, em quem tinha o caminho de um exemplo salvo.

## 7. A chave de agregação de mensageria vira `kafka produce` e `amqp publish`

- **Decisão**: o passo sem nome declarado é agregado sob `kafka produce <tópico>`
  e `amqp publish <destino>`, no lugar de `kafka produzir` e `amqp publicar`.
- **Alternativa**: manter, por ser nome derivado e não mensagem.
- **Por que esta**: a chave aparece na tabela por passo do relatório e é o nome
  que uma regra de slo precisa escrever. É texto que o usuário lê e digita.
- **Reversibilidade**: baixa para quem já tem regra de slo escrita — a regra
  passa a não casar com passo nenhum, e a validação diz isso antes da execução.
- **Toca o usuário**: sim. `braunrate migrate` não renomeia a regra, porque o
  nome do passo é texto do autor; o comando avisa quando o cenário convertido
  deixa de validar.

## 8. Data no relatório em `2006-01-02`

- **Decisão**: o relatório HTML e a comparação passam a escrever a data como
  `2006-01-02 15:04:05`, no lugar de `02/01/2006`.
- **Alternativa**: manter o formato brasileiro, ou escolher pelo idioma do
  navegador.
- **Por que esta**: `03/04/2026` tem duas leituras dependendo do país de quem lê,
  e o relatório é um arquivo que viaja anexado a um ticket. A ordem
  ano-mês-dia tem uma leitura só.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, no cabeçalho do relatório.

## 9. O site em duas linguas, com o ingles na raiz

- **Decisão**: o inglês fica em `/` e o português em `/pt-BR/`, com o mesmo nome
  de arquivo nas duas árvores. Registrada em
  [ADR 0020](adr/0020-site-bilingue.md).
- **Alternativa**: detectar a língua do navegador e redirecionar, ou publicar as
  duas línguas na mesma página.
- **Por que esta**: quem chega sem escolher nada cai no texto que vale, e o
  seletor de língua troca de idioma sem trocar de página porque o endereço é o
  mesmo dos dois lados.
- **Reversibilidade**: baixa. Os endereços das páginas mudaram de português para
  inglês (`instalacao.html` virou `installation.html`), e link de fora do
  repositório quebra.
- **Toca o usuário**: sim, em quem tinha uma página da documentação salva.

## 10. A tradução declara de onde saiu, e a build avisa quando ela atrasa

- **Decisão**: o guia em português carrega `translated_from` e `source_hash` no
  cabeçalho; a build recalcula o hash do original, avisa no terminal quando
  diverge e a página abre com uma tarja dizendo isso ao leitor.
- **Alternativa**: reprovar a build, ou não conferir nada.
- **Por que esta**: reprovar transformaria toda edição no original em edição
  obrigatória nas duas línguas, e o efeito seria parar de editar o original.
  Não conferir nada é o estado natural de todo site bilíngue, e é o motivo pelo
  qual documentação traduzida tem fama ruim.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, quando a tarja aparece.

## 11. A moldura do site é o único catálogo de mensagens do projeto

- **Decisão**: menu, rodapé, paginação, rótulos da busca e títulos das páginas
  geradas vivem em uma tabela por língua, em `internal/site/language.go`.
- **Alternativa**: duplicar o gerador por língua, ou manter a moldura só em
  inglês nas duas árvores.
- **Por que esta**: a Fase 1 recusou catálogo de mensagens porque lá a língua é
  configuração de quem executa. Aqui a língua é o conteúdo da página: a página
  em português com menu em inglês seria metade traduzida.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, na moldura da página em português.

## 12. Página gerada tem moldura traduzida e conteúdo em inglês

- **Decisão**: a referência do cenário e a página de decisões existem nas duas
  línguas, com o texto ao redor traduzido e o conteúdo em inglês — as descrições
  saem do schema, e os títulos saem dos ADRs.
- **Alternativa**: traduzir o schema e os ADRs, ou publicar as duas páginas só
  em inglês.
- **Por que esta**: o schema é o arquivo que o editor lê durante a escrita do
  cenário, e ele é inglês desde o ADR 0019; traduzi-lo criaria uma segunda
  descrição livre para divergir. Os ADRs são registro interno, e a tabela de
  camadas já os deixou em português.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim: as duas páginas dizem isso na própria introdução.

## 13. Todo exemplo publicado passa pelo parser antes de virar documentação

- **Decisão**: um teste extrai os `examples` de cada chave do schema, monta um
  cenário mínimo em volta do valor e roda `Parse` mais `Validate`. Chave com
  exemplo e sem cenário de apoio reprova o teste; não é pulada.
- **Alternativa**: revisar os exemplos à mão, ou confiar no schema por ele ser o
  arquivo que o editor lê.
- **Por que esta**: a referência é gerada do schema, então exemplo errado não é
  um detalhe do arquivo — é a página que ensina errado, com o autocomplete do
  editor concordando. O teste achou três divergências entre o que o schema
  prometia e o que o parser aceita já na primeira execução.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, indiretamente: o que ele copia da referência roda.

## 14. Onde o campo recebe credencial, o exemplo mostra `${VARIAVEL}`

- **Decisão**: nenhum exemplo do schema carrega valor literal de senha, chave ou
  token, e um segundo teste varre os exemplos atrás de aparência de segredo.
- **Alternativa**: escrever `senha123` e confiar em quem lê para trocar.
- **Por que esta**: a ferramenta recusa literal na validação. Documentação que
  ensina o contrário transforma a recusa em obstáculo em vez de regra, e o
  primeiro reflexo de quem esbarra num obstáculo é procurar como desligá-lo.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, no formato que ele copia.

## 15. O schema promete o que o parser aceita, nem mais

- **Decisão**: `$expect.status` passou a documentar um inteiro só, e `$await`
  documenta `until`. O schema prometia lista, `"2xx"` e `"< 400"` para o
  primeiro e um campo `to` inexistente para o segundo.
- **Alternativa**: ampliar o parser para cumprir o que o schema prometia.
- **Por que esta**: ampliar é mudança de comportamento, e a fase não tem
  nenhuma. O schema descreve o que existe hoje; a promessa maior volta junto com
  a implementação, se voltar.
- **Reversibilidade**: alta. Só reduz o autocomplete.
- **Toca o usuário**: sim: quem tinha copiado `status: 2xx` do autocomplete
  recebia erro de validação e nenhuma explicação de onde a ideia veio.

## 16. A referência abre com um cenário inteiro

- **Decisão**: a primeira coisa da página de referência é um cenário completo em
  bloco YAML, antes de qualquer tabela, e cada chave passou a mostrar o `default`
  ao lado do tipo, com obrigatória e opcional distinguidas na tabela.
- **Alternativa**: manter a página só como tabela de chaves, com o cenário
  inteiro em outro guia.
- **Por que esta**: a tabela responde "o que essa chave aceita" e não responde
  "onde ela vai". Quem chega na referência pela busca do editor cai no meio da
  árvore, e sem uma forma inteira por perto o encaixe é adivinhado.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim, no topo da página.

## 17. O que é nome de credencial se decide num lugar só

- **Decisão**: `transport.IsSecretName` responde por toda a ferramenta — a recusa
  no parse, a máscara do cabeçalho, o corte do parâmetro de consulta, o do campo
  de corpo e o da variável capturada.
- **Alternativa**: manter a lista de nomes de cada lado, como estava.
- **Por que esta**: estavam em dois lugares e discordavam. `apiToken` era cortado
  na impressão e aceito literal no arquivo, porque um comparava por pedaço do
  nome e o outro por nome inteiro. Foi a terceira vez que uma proteção cobriu uma
  saída e deixou a vizinha de fora.
- **Reversibilidade**: alta.
- **Toca o usuário**: sim: nome de variável que contém `token`, `password`,
  `secret` ou `api-key` passou a ser recusado com valor literal, e antes passava.
