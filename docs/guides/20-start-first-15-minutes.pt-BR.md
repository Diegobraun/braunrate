---
translated_from: 20-start-first-15-minutes.en.md
source_hash: 46a4cc6bb454
---
# Primeiros 15 minutos

Do zero até um relatório do seu serviço, lido e entendido. Se você nunca fez
teste de carga, não pule os dois primeiros passos: eles explicam os termos que o
resto do caminho usa.

## 1. Ver funcionando

```bash
braunrate demo
```

Não precisa de arquivo, de alvo nem de segundo terminal. O comando sobe um
serviço de mentira em `127.0.0.1:8080`, roda um cenário contra ele e explica cada
número:

```
[2/3] Running: 100 requests per second, for 5s.

      That is the rate: braunrate fires at that pace whether the service is
      fast or slow — the way real users do. Tools that wait for the previous
      response before sending the next one go easy on the system exactly when
      it is struggling.

[3/3] Done. What the numbers say:

  500 requests in 5s, 100 per second, 0.00% of them errors
  Half the responses within 6.5 ms; 95% within 7.0 ms; the worst took 14 ms

  ok    Passed: the whole scenario had the error rate of 0.00%, within the limit of 0.10%.
```

Ao terminar, dois arquivos ficam no diretório: `demo.yaml`, o cenário comentado
que acabou de rodar, e `demo-report.html`, o relatório completo. Abra os dois.

Para ver a ferramenta pegando um problema de verdade:

```bash
braunrate demo --with-failure
```

## 2. Entender o que você acabou de ler

Cinco ideias, e nenhuma outra é necessária para começar. Cada uma tem a
explicação longa em [Conceitos](concepts.html):

| Ideia | Em uma linha |
|---|---|
| **taxa** | quantas requisições por segundo o gerador dispara, esteja o alvo rápido ou lento |
| **"95% das respostas em até X"** | 5% das pessoas esperaram mais que X; a média não aparece no relatório porque esconde a cauda |
| **critério de aceite** | o limite que você declara no bloco `slo`; se estourar, o comando sai com código 1 e o CI reprova |
| **dado fixo distorce** | mil requisições idênticas medem o cache do alvo, e o relatório avisa quando o cenário não varia nada |
| **resultado inválido** | a execução não mediu o que se propôs; código de saída 3, e nenhum número dela vale como resposta |

## 3. Partir do seu serviço

Não comece de folha em branco. Copie um `curl` do painel de rede do navegador e
transforme em cenário:

```bash
braunrate import curl 'curl https://sua-api/pedidos/9912 -H "Authorization: Bearer abc.def"' -output cenario.yaml
```

O token vira `${TOKEN}`, lido do ambiente, e não vai para o repositório.

As outras portas de entrada, para quando não há um `curl` à mão:

| Você tem | Comando |
|---|---|
| um plano de JMeter | `braunrate import jmx plano.jmx -output cenario.yaml` |
| o navegador aberto no fluxo | `braunrate record -output cenario.yaml` |
| nada ainda | `braunrate new cenario.yaml` |

> **Nota** O importador de JMeter traduz o subconjunto comum e lista no terminal
> o que ficou de fora, em vez de traduzir pela metade em silêncio.

## 4. Rodar uma vez antes de rodar mil

```bash
braunrate debug cenario.yaml
```

Uma iteração, um usuário, sem carga. Mostra o que foi enviado, o que voltou, o
que foi capturado e onde parou:

```
step 1 — look up order   [ok in 6.8ms]
  request:    GET /orders/1001
              Authorization: Bearer test-t… (10 characters)
  response:   status 200, 91 bytes
  body:       {"id":"1001","status":"OPEN","lastInvoice":{"id":"f-1001","amount":199.90,"status":"OPEN"}}
  captured:
    invoiceId = f-1001

step 2 — pay invoice   [ok in 6.1ms]
  request:    POST /invoices/f-1001/pay
              Authorization: Bearer test-t… (10 characters)
  response:   status 200, 63 bytes
  body:       {"id":"f-1001","status":"PAID","paidAt":"2026-08-15T00:00:00Z"}

Iteration complete: 2 steps, all good. To run it with load:
  braunrate execute cenario.yaml
```

> **Importante** Carga só vale depois que a iteração passa. Descobrir que a
> correlação quebrou depois de dez minutos de carga é o erro que o JMeter ensinou
> todo mundo a cometer.

## 5. Declarar o que você considera aceitável

Sem bloco `slo` o cenário roda e reporta, mas não serve de gate. Com ele, o
código de saída decide:

```yaml fragment
slo:
  - consultar pedido: { p95: < 150ms }   # um passo
  - journey: { p95: < 2s }               # a espera inteira, ponta a ponta
  - global: { errors: < 0.1 }             # a execucao toda
```

Um gate feito só de regra por passo aprova cada pedaço e não diz nada sobre a
espera que o usuário sente, que é a soma deles. O `braunrate validate` avisa
quando o seu gate está assim.

## 6. Rodar com carga e ler o resultado

```bash
braunrate execute cenario.yaml -html=relatorio.html -result=saida.json
```

Leia nesta ordem:

1. **A primeira frase.** "Passou", "Falhou" ou "Resultado inválido". Se for a
   terceira, pare aqui: nada abaixo vale.
2. **"O que aconteceu"** — quantas requisições, a taxa efetiva, a taxa de erro.
   Taxa efetiva bem abaixo da declarada tem duas causas opostas, e a ferramenta
   diz qual foi.
3. **"A jornada inteira"** — o tempo que o usuário espera de ponta a ponta. É a
   leitura que vale quando o cenário tem mais de um passo.
4. **"Confiabilidade da medição"** — se o gerador atrasou, se o alvo degradou ao
   longo do tempo, se algum dado não variou. Aqui aparecem as ressalvas que mudam
   a leitura de tudo acima.
5. **"SLO"** — o veredito, regra por regra, e também o que você **não** declarou.

## 7. Virar gate no CI

```bash
braunrate execute cenario.yaml -quiet -result=saida.json
```

| Código de saída | Significado |
|---|---|
| `0` | passou |
| `1` | o critério de aceite reprovou |
| `2` | erro no arquivo do cenário |
| `3` | resultado inválido: a execução não mediu o que se propôs a medir |

A receita completa, com comparação contra a execução anterior, está em
[Receitas](recipes.html#fazer-o-teste-reprovar-o-build).
