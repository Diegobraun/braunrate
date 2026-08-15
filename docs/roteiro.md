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

## Fases

| Fase | Nome | Estado |
|---|---|---|
| 0 | Decisao de linguagem, cenario e execucao | concluida |
| 1 | Motor e HTTP | concluida |
| 2 | Correlacao, dados e SLO | em andamento |
| 2.5 | **Autoria** | nova, antes da Fase 3 |
| 3 | Relatorio (com as duas camadas de texto e a prova de auto-validacao no README) | pendente |
| 4 | GraphQL | pendente |
| 5 | Mensageria e passo `aguardar` | pendente |
| 6 | Segundo publico: DSL e importador de `.jmx` | pendente |
| 7 | Acabamento e lancamento | pendente |

## Prova central do produto

A demonstracao de auto-validacao — **974,6 ms escondidos pelo laco fechado no mesmo alvo** — e o argumento central do braunrate. Vai para o topo do README na Fase 3, com a saida real do teste que roda no CI.
