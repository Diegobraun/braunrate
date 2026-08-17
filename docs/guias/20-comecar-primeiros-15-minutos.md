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
[2/3] Rodando: 100 requisições por segundo, durante 5s.

      Essa é a taxa: o braunrate dispara nesse ritmo esteja o serviço rápido ou
      lento — como usuários de verdade fazem. Ferramentas que esperam a
      resposta anterior antes de mandar a próxima aliviam o sistema justamente
      quando ele está sofrendo.

[3/3] Pronto. O que os números dizem:

  500 requisições em 5s, 100 por segundo, 0.00% de erro
  Metade das respostas em até 6.5 ms; 95% em até 7.1 ms; a pior levou 16 ms

  ok    Passou: o cenário inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.
```

Ao terminar, dois arquivos ficam no diretório: `demo.yaml`, o cenário comentado
que acabou de rodar, e `demo-relatorio.html`, o relatório completo. Abra os dois.

Para ver a ferramenta pegando um problema de verdade:

```bash
braunrate demo --with-failure
```

## 2. Entender o que você acabou de ler

Cinco ideias, e nenhuma outra é necessária para começar. Cada uma tem a
explicação longa em [Conceitos](conceitos.html):

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
passo 1 — consultar pedido   [ok em 3.4ms]
  requisicao: GET /pedidos/1001
              Authorization: Bearer token-… (14 caracteres)
  resposta:   status 200, 95 bytes
  corpo:      {"id":"1001","status":"ABERTO","ultimaFatura":{"id":"f-1001","status":"ABERTA"}}
  capturou:
    faturaId = f-1001

Iteração completa: 2 passos, tudo certo. Para rodar com carga:
  braunrate execute cenario.yaml
```

> **Importante** Carga só vale depois que a iteração passa. Descobrir que a
> correlação quebrou depois de dez minutos de carga é o erro que o JMeter ensinou
> todo mundo a cometer.

## 5. Declarar o que você considera aceitável

Sem bloco `slo` o cenário roda e reporta, mas não serve de gate. Com ele, o
código de saída decide:

```yaml trecho
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
[Receitas](receitas.html#fazer-o-teste-reprovar-o-build).
