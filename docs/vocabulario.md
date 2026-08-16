# Vocabulario

O publico do braunrate inclui quem nunca fez teste de carga. Quem ve tres nomes
para a mesma coisa conclui que sao tres coisas — e passa a duvidar do numero em
vez de duvidar do texto.

**Uma palavra por conceito, no terminal, no site e na interface.** Esta tabela e
criterio de aceitacao para todo texto que o usuario le.

| Conceito | Termo oficial | Nunca use |
|---|---|---|
| Requisicoes por segundo que o gerador dispara | **taxa** | throughput, vazao, RPS, carga por segundo |
| Tempo ate a resposta chegar | **tempo de resposta** | latencia, response time |
| p95, p99 | **95% das respostas em ate X** | percentil, quantil, p95 solto |
| Limite declarado que vira codigo de saida | **criterio de aceite** (`slo` no YAML) | threshold, gate, SLA |
| Execucao que nao mediu o que se propos | **resultado invalido** | falha, erro, teste quebrado |
| Gerador nao conseguiu manter a taxa | **o gerador nao sustentou a taxa** | saturacao, back-pressure |
| Sequencia de passos de uma iteracao | **jornada** | cenario, fluxo, transacao |
| O arquivo `.yaml` | **cenario** | plano, teste, script |
| Sistema sendo testado | **alvo** | SUT, servidor, aplicacao |

## Onde a regra nao vale

Jargao tecnico continua existindo onde ele e preciso e o leitor e o programa:

- **chaves do JSON de resultado** (`latencia_corrigida`, `p95_ms`) — o formato e
  contrato de maquina, e renomear campo quebra quem ja le o arquivo;
- **codigo, nomes de tipo e ADRs** — leitor e quem trabalha no braunrate;
- **chaves do YAML ja publicadas** — mudar o formato do cenario esta fora de
  escopo de experiencia de uso. Onde uma chave contradiz a tabela, a
  divergencia esta registrada em [decisoes-experiencia.md](decisoes-experiencia.md).

## Termo novo

Escolha um, acrescente aqui, e use em toda parte. Termo novo sem registro e
divergencia esperando acontecer.
