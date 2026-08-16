# Vocabulário

O público do braunrate inclui quem nunca fez teste de carga. Quem vê três nomes
para a mesma coisa conclui que são três coisas — e passa a duvidar do número em
vez de duvidar do texto.

**Uma palavra por conceito, no terminal, no site e na interface.** Esta tabela é
critério de aceitação para todo texto que o usuário lê.

| Conceito | Termo oficial | Nunca use |
|---|---|---|
| Requisições por segundo que o gerador dispara | **taxa** | throughput, vazão, RPS, carga por segundo |
| Tempo até a resposta chegar | **tempo de resposta** | latência, response time |
| p95, p99 | **95% das respostas em até X** | percentil, quantil, p95 solto |
| Limite declarado que vira código de saída | **critério de aceite** (`slo` no YAML) | threshold, gate, SLA |
| Execução que não mediu o que se propôs | **resultado inválido** | falha, erro, teste quebrado |
| Gerador não conseguiu manter a taxa | **o gerador não sustentou a taxa** | saturacao, back-pressure |
| Sequência de passos de uma iteração | **jornada** | cenário, fluxo, transacao |
| O arquivo `.yaml` | **cenário** | plano, teste, script |
| Sistema sendo testado | **alvo** | SUT, servidor, aplicação |

## Onde a regra não vale

Jargão técnico continua existindo onde ele é preciso e o leitor é o programa:

- **chaves do JSON de resultado** (`latencia_corrigida`, `p95_ms`) — o formato é
  contrato de máquina, e renomear campo quebra quem já lê o arquivo;
- **código, nomes de tipo e ADRs** — leitor é quem trabalha no braunrate;
- **chaves do YAML já publicadas** — mudar o formato do cenário está fora de
  escopo de experiência de uso. Onde uma chave contradiz a tabela, a
  divergência está registrada em [decisoes-experiencia.md](decisoes-experiencia.md).

## Termo novo

Escolha um, acrescente aqui, e use em toda parte. Termo novo sem registro é
divergencia esperando acontecer.
