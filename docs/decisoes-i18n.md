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

- **Decisão**: os 132 campos do documento de resultado passam para camelCase em
  inglês, e `versaoDoFormato` sobe de 1 para 2.
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
