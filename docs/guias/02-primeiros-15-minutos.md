# Primeiros 15 minutos

Do zero ate um relatorio do seu servico, lido e entendido. Se voce nunca fez
teste de carga, nao pule os dois primeiros passos: eles explicam os termos que o
resto do caminho usa.

## 1. Ver funcionando (30 segundos)

```bash
braunrate demo
```

Nao precisa de arquivo, de alvo nem de segundo terminal. O comando sobe um
servico de mentira em `127.0.0.1:8080`, roda um cenario contra ele e explica cada
numero:

```
[2/3] Rodando: 100 requisicoes por segundo, durante 5s.

      Essa e a taxa: o braunrate dispara nesse ritmo esteja o servico rapido ou
      lento — como usuarios de verdade fazem. Ferramentas que esperam a
      resposta anterior antes de mandar a proxima aliviam o sistema justamente
      quando ele esta sofrendo.

[3/3] Pronto. O que os numeros dizem:

  500 requisicoes em 5s, 100 por segundo, 0.00% de erro
  Metade das respostas em ate 6.0 ms; 95% em ate 6.6 ms; a pior levou 15 ms

  ok    Passou: o cenario inteiro teve taxa de erro de 0.00%, dentro do limite de 0.10%.
```

Ele deixa dois arquivos no diretorio: `demo.yaml`, o cenario comentado que
acabou de rodar, e `demo-relatorio.html`, o relatorio completo. Abra os dois.

E, para ver a ferramenta pegando um problema de verdade:

```bash
braunrate demo --com-falha
```

## 2. Entender o que voce acabou de ler

Cinco ideias, e nenhuma outra e necessaria para comecar. Cada uma tem a
explicacao longa em [Conceitos](conceitos.html):

- **taxa** — quantas requisicoes por segundo o gerador dispara. Ele dispara nesse
  ritmo esteja o alvo rapido ou lento.
- **"95% das respostas em ate X"** — 5% das pessoas esperaram mais que X. Media
  nao aparece no relatorio de proposito: ela esconde a cauda.
- **criterio de aceite** — o limite que voce declara no bloco `slo` do arquivo.
  Se estourar, o comando sai com codigo 1 e o seu CI reprova.
- **dado fixo distorce** — mil requisicoes identicas medem o cache do alvo. O
  relatorio avisa quando o cenario nao varia nada.
- **resultado invalido** — a execucao nao mediu o que se propos. Codigo de saida
  3, e nenhum numero dela vale como resposta.

## 3. Partir do seu servico

Nao comece de folha em branco. Copie um `curl` do painel de rede do navegador e
transforme em cenario:

```bash
braunrate import curl 'curl https://sua-api/pedidos/9912 -H "Authorization: Bearer abc.def"' -output cenario.yaml
```

O token vira `${TOKEN}`, lido do ambiente, e nao vai para o repositorio. Se voce
tem um plano de JMeter, `braunrate import jmx plano.jmx -output cenario.yaml`
traduz o subconjunto comum e **lista no terminal o que ficou de fora**. Se nao
tem nem um nem outro, `braunrate record -output cenario.yaml` grava enquanto voce
navega, e `braunrate new cenario.yaml` escreve um esqueleto comentado.

## 4. Rodar uma vez antes de rodar mil

```bash
braunrate debug cenario.yaml
```

Uma iteracao, um usuario, sem carga. Mostra o que foi enviado, o que voltou, o
que foi capturado e onde parou:

```
passo 1 — consultar pedido   [ok em 3.4ms]
  requisicao: GET /pedidos/1001
              Authorization: Bearer token-… (14 caracteres)
  resposta:   status 200, 95 bytes
  corpo:      {"id":"1001","status":"ABERTO","ultimaFatura":{"id":"f-1001","status":"ABERTA"}}
  capturou:
    faturaId = f-1001

Iteracao completa: 2 passos, tudo certo. Para rodar com carga:
  braunrate execute cenario.yaml
```

Este passo existe porque descobrir que a correlacao quebrou depois de dez minutos
de carga e o erro que o JMeter ensinou a todo mundo a cometer. **Carga so vale
depois que a iteracao passa.**

## 5. Declarar o que voce considera aceitavel

Sem bloco `slo` o cenario roda e reporta, mas nao serve de gate. Com ele, o
codigo de saida decide:

```yaml trecho
slo:
  - consultar pedido: { p95: < 150ms }   # um passo
  - jornada: { p95: < 2s }               # a espera inteira, ponta a ponta
  - global: { erros: < 0.1 }             # a execucao toda
```

Um gate feito so de regra por passo aprova cada pedaco e nao diz nada sobre a
espera que o usuario sente, que e a soma deles. O `braunrate validate` avisa
quando o seu gate esta assim.

## 6. Rodar com carga e ler o resultado

```bash
braunrate execute cenario.yaml -html=relatorio.html -result=saida.json
```

Leia nesta ordem:

1. **A primeira frase.** "Passou", "Falhou" ou "Resultado invalido". Se for a
   terceira, pare aqui: nada abaixo vale.
2. **"O que aconteceu"** — quantas requisicoes, a taxa efetiva, a taxa de erro.
   Taxa efetiva bem abaixo da declarada tem duas causas opostas, e a ferramenta
   diz qual foi.
3. **"A jornada inteira"** — o tempo que o usuario espera de ponta a ponta. E a
   leitura que vale quando o cenario tem mais de um passo.
4. **"Confiabilidade da medicao"** — se o gerador atrasou, se o alvo degradou ao
   longo do tempo, se algum dado nao variou. Aqui aparecem as ressalvas que
   mudam a leitura de tudo acima.
5. **"SLO"** — o veredito, regra por regra, e tambem o que voce **nao** declarou.

## 7. Virar gate no CI

```bash
braunrate execute cenario.yaml -quiet -result=saida.json
```

Codigo de saida: `0` passou, `1` o criterio de aceite reprovou, `2` erro no
arquivo do cenario, `3` resultado invalido — a execucao nao mediu o que se propos
a medir, entao nao ha o que aprovar ou reprovar.

A receita completa, com comparacao contra a execucao anterior, esta em
[Receitas](receitas.html#gate-de-ci).
