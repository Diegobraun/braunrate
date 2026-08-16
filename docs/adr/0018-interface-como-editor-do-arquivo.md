# ADR 0018 — A interface é um editor do arquivo, servida pelo próprio binário

- **Status**: aceito
- **Data**: 2026-08-16
- **Relacionados**: [ADR 0002](0002-modelo-de-cenario.md), [ADR 0010](0010-idioma-do-codigo.md), [ADR 0017](0017-superficie-publica-de-execucao.md)

## Contexto

Metade do público do braunrate é QA que não programa. Para essa pessoa, o
terminal é uma barreira antes da primeira medição: ela precisa saber onde o
arquivo está, como se chama o comando e o que a saída quer dizer, tudo ao mesmo
tempo.

A resposta óbvia — uma interface gráfica com campos, listas e botões — é
exatamente o que o JMeter fez, e o resultado é conhecido: o arquivo salvo deixa
de ser legível, o diff vira ruído, e o que a pessoa escreveu à mão desaparece na
primeira vez que a ferramenta serializa a árvore de volta. Um cenário que só a
interface consegue escrever é um cenário que o time não versiona.

## Decisão

### 1. A interface edita o texto do arquivo, e nada mais

`braunrate ui` abre o arquivo `.yaml` do diretório apontado por `-dir`, com os
comentários que o autor escreveu, numa área de texto. Salvar grava aquele texto.
Não existe árvore de campos com estado próprio, e não existe nada que a interface
saiba fazer e o arquivo não registre.

O formulário de "começar do zero" é a única exceção, e ele **escreve um arquivo
comentado e sai de cena**: dali em diante o arquivo é a verdade.

### 2. O mesmo servidor do `serve`, com a gravação declarada

A interface consome as rotas que o `serve` já expõe, e acrescenta duas coisas:
`GET /scenarios/{nome}/text`, que devolve o texto cru, e
`PUT /scenarios/{nome}/text`, que grava. A gravação fica atrás de
`Options.Writable`, desligada no `serve` e ligada no `ui`, e o aviso de
inicialização diz que a porta pode alterar arquivos.

A validação do rascunho passa pela mesma leitura do terminal (`runner.Check`), a
partir do texto e não do arquivo gravado. Duas leituras diferentes dariam uma
resposta no editor e outra no terminal, e a do terminal é a que reprova o build.

### 3. Servida pelo próprio binário, sem etapa de build

A interface é embarcada com `go:embed`. Quem baixou o executável não instala
node, não roda bundler e não busca nada da rede: exigir isso de quem só quer
rodar um teste de carga seria a mesma barreira que o binário único existe para
não ter. A regra de não buscar nada da rede é a mesma do relatório HTML.

### 4. O comando equivalente fica visível o tempo todo

Toda tela mostra, no topo, o comando de terminal que faz aquilo. Quem começou
pela interface aprende o CLI sem procurar, e quem já conhece o CLI confere o que
a tela está fazendo.

## O que foi recusado

- **Árvore de campos como formato de autoria.** É o defeito do `.jmx`, e o
  motivo pelo qual o importador do JMeter existe.
- **Estado próprio da interface** (rascunho em `localStorage`, sessão, projeto).
  O que não está no arquivo não vai para o repositório e não roda no CI.
- **Autenticação, multiusuário e exposição fora de 127.0.0.1.** A porta não tem
  autenticação nem TLS; o aviso de inicialização declara isso, e expor é uma
  decisão que ainda não foi tomada.
- **Framework de frontend.** Uma dependência de CDN quebraria a regra de rede
  fechada, e um bundler quebraria a regra de binário único.

## Critério que reabre

- A interface precisar guardar algo que o arquivo não guarda.
- A edição por fora deixar de ser refletida, ou o texto gravado deixar de ser
  byte a byte o que a pessoa escreveu.
- Alguém precisar da interface em uma máquina que não é a dela, o que exige
  autenticação e transporte, e é outro produto.
