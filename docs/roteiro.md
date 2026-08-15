# Roteiro

## Correcao de rota — 2026-08-15, apos a Fase 1

O maior risco do projeto deixou de ser tecnico e passou a ser **adocao**. O publico alvo e QA, que em geral nao tem perfil de desenvolvedor. A ferramenta hoje e de engenheiro: a saida fala em "desvio de agendamento", "limite de voo" e "omissao coordenada", e o erro de YAML diz "ainda nao e suportada" sem ensinar o que fazer.

Tres consequencias, todas obrigatorias daqui em diante:

1. **Cada recurso sera editado por um QA em YAML.** A forma mais simples que resolve o caso comum e a forma principal; a forma poderosa e opcao. Correlacao em uma linha e **requisito**, nao meta.
2. **Entra uma Fase 2.5 — Autoria**, entre a Fase 2 e a Fase 3.
3. **A partir da Fase 3, todo texto visivel tem duas camadas**: o numero tecnico continua, e a leitura vem em portugues comum. O topo do relatorio HTML e uma frase, nao uma tabela — por exemplo: `Falhou: "consultar pedido" teve p95 de 210 ms, acima do limite de 150 ms.`

## Fase 2.5 — Autoria

Escopo:

1. **Schema JSON publicado**, com a linha `# yaml-language-server: $schema=...` no topo dos exemplos, dando autocomplete e validacao no VS Code.
2. **`braunrate depurar cenario.yaml`**: um usuario virtual, uma iteracao, mostrando cada passo — requisicao, resposta, o que foi capturado, valor de cada variavel, resultado de cada assercao. Equivalente ao View Results Tree do JMeter.
3. **`braunrate importar curl`**: converte um comando cURL colado em passo pronto, com cabecalhos, corpo e autenticacao.
4. **Mensagens de erro que ensinam**: linha e coluna, o que esta errado, sugestao por similaridade, lista de opcoes validas e um exemplo minimo.

**Criterio de aceitacao, em termos de usuario e nao de engenheiro**: uma pessoa que nunca viu o braunrate, com o binario, o schema e um exemplo, cria um cenario funcional para um endpoint autenticado **sem ler documentacao alem das mensagens da propria ferramenta**.

Dois pontos de confiabilidade entraram na fase depois da revisao da Fase 2, porque afetam a credibilidade do numero:

5. **Latencia do segundo passo em diante.** So o primeiro passo tem instante agendado proprio. O relatorio distingue visualmente os dois tipos com nota em portugues comum, e passa a medir o **tempo total da iteracao contado do instante agendado** — a metrica que continua honesta para a jornada inteira.
6. **Token compartilhado.** Um token para a execucao inteira nao existe em producao. O padrao continua, com a limitacao declarada no relatorio e no README, e a evolucao registrada no [ADR 0005](adr/0005-identidade-e-token.md).

**Como o criterio de aceitacao foi verificado**: o teste de deriva entre schema e parser cobre o autocompletar; `importar curl` gera cenario que o proprio parser aceita (teste); `depurar` mostra requisicao, resposta, captura e variaveis, e termina imprimindo o comando de carga; sete mensagens de erro tipicas foram checadas por teste quanto a linha, sugestao e exemplo minimo. O que **nao** foi verificado: nenhuma pessoa de fora executou o percurso ainda — isso continua pendente e so uma sessao com um QA real fecha o criterio.

## Fases

| Fase | Nome | Estado |
|---|---|---|
| 0 | Decisao de linguagem, cenario e execucao | concluida |
| 1 | Motor e HTTP | concluida |
| 2 | Correlacao, dados e SLO | concluida |
| 2.5 | Autoria | concluida |
| 3 | Relatorio (com as duas camadas de texto e a prova de auto-validacao no README) | concluida |
| 4 | GraphQL | pendente |
| 5 | Mensageria e passo `aguardar` | pendente |
| 6 | Segundo publico: DSL e importador de `.jmx` | pendente |
| 7 | Acabamento e lancamento | pendente |

## Fase 3 — Relatorio

Entregue: relatorio HTML autocontido com veredito em uma frase no topo, grafico SVG desenhado sem biblioteca (o arquivo nao busca nada na rede), CSV por passo com o tipo de latencia declarado, veredito de SLO dentro do documento JSON, `braunrate relatorio` para gerar o HTML de um resultado ja gravado e `braunrate comparar` entre duas execucoes.

A comparacao trata variacao abaixo de 5% como ruido, porque duas execucoes nao produzem intervalo de confianca; lista o que mudou fora do servico (maquina, plano, versao, cenario, token compartilhado); e se recusa a comparar quando alguma das duas teve o gerador saturado.

## Prova central do produto

A demonstracao de auto-validacao — **974,6 ms escondidos pelo laco fechado no mesmo alvo** — e o argumento central do braunrate. Vai para o topo do README na Fase 3, com a saida real do teste que roda no CI.
