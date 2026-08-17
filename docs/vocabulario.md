# Vocabulário

O público do braunrate inclui quem nunca fez teste de carga. Quem vê três nomes
para a mesma coisa conclui que são três coisas — e passa a duvidar do número em
vez de duvidar do texto.

**Uma palavra por conceito, no terminal, no site e na interface.** Esta tabela é
critério de aceitação para todo texto que o usuário lê. Desde o
[ADR 0019](adr/0019-formato-em-ingles.md) o texto ao usuário é em inglês, então
o termo oficial é o inglês; a coluna em português fica como ponte para quem
escreve a documentação em português.

| Conceito | Termo oficial | Em português | Nunca use |
|---|---|---|---|
| Requisições por segundo que o gerador dispara | **rate** | taxa | throughput, RPS, load per second |
| Requisições por segundo que o alvo de fato completou | **throughput** | vazão | taxa efetiva, effective rate |
| Tempo até a resposta chegar | **response time** | tempo de resposta | latency, latência |
| p95, p99 | **95% of the responses within X** | 95% das respostas em até X | percentile, quantile, p95 solto |
| Limite declarado que vira código de saída | **acceptance criterion** (`slo` no YAML) | critério de aceite | threshold, gate, SLA |
| Execução que não mediu o que se propôs | **invalid result** | resultado inválido | failure, error, broken test |
| Gerador não conseguiu manter a taxa | **the generator did not sustain the rate** | o gerador não sustentou a taxa | saturation, back-pressure |
| Sequência de passos de uma iteração | **journey** | jornada | scenario, flow, transaction |
| O arquivo `.yaml` | **scenario** | cenário | plan, test, script |
| Sistema sendo testado | **target** | alvo | SUT, server, application |
| Espera pelo efeito de um passo anterior | **await** | aguardar | wait, poll, sleep |
| Valor tirado de uma resposta para o passo seguinte | **capture** | captura | extract, correlation variable |
| Quantos valores distintos a execução de fato usou | **observed variety** | variedade observada | cardinality, uniqueness |

## Onde a regra não vale

Jargão técnico continua existindo onde ele é preciso e o leitor é o programa:

- **chaves de log estruturado** — o leitor é o coletor, não a pessoa;
- **código, nomes de tipo e ADRs em português** — leitor é quem trabalha no
  braunrate. Os identificadores do código são em inglês (ADR 0010); os ADRs, os
  commits e os relatórios internos continuam em português.

Onde uma chave do YAML contradiz a tabela, a divergência está registrada em
[decisoes-experiencia.md](decisoes-experiencia.md) ou em
[decisoes-i18n.md](decisoes-i18n.md).

## Termo novo

Escolha um, acrescente aqui, e use em toda parte. Termo novo sem registro é
divergência esperando acontecer.
