# ADR 0011 — Verificacao de sanidade antes de qualquer veredito

- **Status**: aceito
- **Data**: 2026-08-16
- **Contexto de decisao**: antes da Fase 7
- **Relacionados**: [ADR 0007](0007-variedade-observada.md), [ADR 0003](0003-modelo-de-execucao-e-metrica.md)

## Contexto

Tres defeitos apareceram em fases diferentes e eram o mesmo defeito:

1. **Fase 4** — a autenticacao congelava os dados da primeira iteracao, e toda execucao autenticada com CSV rodava sobre a primeira linha enquanto o relatorio anunciava variedade que nao existiu.
2. **Fase 6** — `examples/ci.yaml` rodava contra o alvo embutido sem token, tomava 100% de 401 e passava verde no CI desde a Fase 1. A unica regra declarada era de latencia, e latencia de requisicao que falha continua baixa.
3. **Auditoria de fricao** — a variedade observada so foi conferida porque alguem pediu; nada no produto obrigava a olhar.

A familia e sempre a mesma: **execucao sintaticamente perfeita, semanticamente vazia, com a suite inteira verde**. O cenario e valido, o motor roda, o histograma enche, o relatorio sai bonito, e o numero nao quer dizer nada. Uma ferramenta de medicao que afirma "passou" sobre execucao vazia perde a razao de existir.

O tratamento ate aqui era pontual: variedade colapsada e gerador saturado viravam aviso de gravidade alta, jornada incompleta virava regra de SLO embutida. Tres mecanismos, tres lugares, e nada que cobrisse o quarto caso quando ele aparecesse.

## Decisao

**Um ponto de decisao so, aplicado sempre, antes de o SLO ser lido**: `metrics.CheckSanity`, chamado de dentro de `BuildDocument`, com resultado no proprio documento (`sanidade` no JSON).

Seis casos invalidam a execucao, cada um com mensagem propria:

| Caso | Verificacao |
|---|---|
| nenhuma jornada chegou ao fim | `jornada_incompleta` |
| todos os passos falharam, ou um passo falhou em 100% | `tudo_falhou` |
| a carga declarada nao foi aplicada inteira | `execucao_curta` |
| passo declarado sem nenhuma amostra | `passo_sem_amostra` |
| variedade colapsada em fonte com varios valores | `medicao_invalidada` |
| gerador saturado | `medicao_invalidada` |

Consequencias no comando: saida **3**, e `slo.Evaluate` nem e chamado. A frase diz que a execucao nao mediu o que se propos a medir e **nao afirma nada sobre o alvo** — uma execucao em que tudo falhou pode muito bem ser o alvo caindo, e dizer o contrario seria uma segunda afirmacao errada em cima da primeira.

Cenario sem bloco `slo` continua executando e reportando. A verificacao nao exige declaracao nenhuma.

## Alternativas descartadas

- **Continuar com avisos de gravidade alta**: funcionava, mas espalhava a regra. Cada caso novo exigia lembrar de marcar a gravidade certa, e o proprio `ci.yaml` mostrou que dava para esquecer.
- **Virar regra de SLO embutida**: confunde dois vereditos diferentes. Saida 1 e "o alvo nao atendeu o criterio"; saida 3 e "esta execucao nao serve para afirmar nada". Misturar os dois faz o CI tratar medicao invalida como regressao de servico.
- **Medir "execucao curta" pelo relogio de parede**: errado, e o primeiro teste real pegou. Um perfil de 3 s a 20/s agenda a ultima requisicao em 2,95 s, entao execucao sadia termina antes de a janela declarada fechar. A regra passou a olhar quanto do plano saiu do gerador — despachadas mais descartadas, porque descarte tambem significa que o laco chegou ali.

## Consequencias

- `Document.Valid()` tem uma fonte de verdade so, a sanidade. O laco sobre avisos graves fica como compatibilidade para arquivo de resultado gravado por versao anterior, que nao tem o bloco.
- Terminal e HTML mostram os achados num lugar so, no topo; o aviso grave nao e mais impresso duas vezes.
- Cada caso tem teste que **prova falhar sem o codigo dele**: o teste roda a mesma execucao vazia com todas as verificacoes menos a sua e exige que ela passe como valida. Sem isso, o teste nao provaria nada sobre a verificacao que diz cobrir.
- O exemplo publicado em `docs/exemplo-relatorio.html` continua byte a byte igual: documento sem bloco de sanidade cai na regra antiga.
