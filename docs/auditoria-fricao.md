# Auditoria de friccao — o que a ferramenta exige e nao fornece

- **Data**: 2026-08-16, entre a Fase 6 e a Fase 7
- **Versao auditada**: braunrate 0.4.0
- **Metodo**: tres jornadas percorridas do zero, cada uma comecando de uma pasta vazia, com acesso apenas ao binario e ao que a propria ferramenta imprime. README, ADR e codigo-fonte fora de alcance durante o percurso — precisar deles ja e achado.
- **Limite declarado**: quem percorreu conhece o formato por dentro. Onde uma pessoa travaria por nao saber, aqui o percurso seguiu. Entao esta auditoria **subestima** a friccao; ela nao a exagera.

Classificacao: **bloqueia** (a pessoa nao termina sozinha), **atrasa** (termina, por tentativa e erro), **incomoda**.

## Resumo

| # | Achado | Jornada | Classe |
|---|---|---|---|
| 1 | Da pasta vazia nao ha caminho ate o primeiro cenario: nenhum comando gera esqueleto e nenhuma saida mostra um | A | bloqueia |
| 2 | Execucao em que nenhuma jornada chegou ao fim sai com "Passou" e codigo 0 | B | bloqueia |
| 3 | Esperar efeito visivel por HTTP e impossivel, e a mensagem nao diz o que fazer | C | bloqueia |
| 4 | Alvo fora do ar vira "erro de configuracao do cenario" — diagnostico errado | B | bloqueia |
| 5 | Erro de chave desconhecida lista as chaves validas mas nao mostra a forma certa | A | atrasa |
| 6 | Variavel de ambiente nao definida vira texto vazio, sem aviso | A | atrasa |
| 7 | Caminho fixo escapa da verificacao de variedade | A | atrasa |
| 8 | Passo que nunca executou some do relatorio | B | atrasa |
| 9 | Linha de erro nao diz o status nem o passo | B | atrasa |
| 10 | Erro de sintaxe YAML sai cru, em ingles, sem arquivo nem coluna | C | atrasa |
| 11 | Timeout do `aguardar` diz "tempo esgotado" e imprime campo vazio | C | atrasa |
| 12 | Arquivo inexistente responde em ingles e nao ensina o proximo passo | A | incomoda |
| 13 | Sugestao de "voce quis dizer" dispara para palavra sem relacao | A | incomoda |
| 14 | Concordancia: "as 1 regras de SLO foram atendidas" | B | incomoda |
| 15 | Cabecalho "Por passo" impresso com tabela vazia | B | incomoda |

Custo do percurso: **jornada A — 12 comandos, 6 edicoes**; **jornada B — 6 comandos, 3 edicoes** (partindo de um cenario que ja funcionava); **jornada C — 4 comandos, 3 edicoes**, e nao terminou.

---

## Jornada A — endpoint autenticado

Objetivo: API com `POST /auth/token` e `GET /pedidos/{id}`, saber se aguenta 100 req/s com p95 abaixo de 200 ms. Da pasta vazia ate o relatorio.

**12 comandos, 6 edicoes.** Terminou.

### A1 — Da pasta vazia nao existe caminho ate o primeiro cenario (bloqueia)

`braunrate` sem argumento lista os comandos. Todos recebem `<cenario.yaml>` como entrada — e nenhum **cria** um. `braunrate novo` e `braunrate exemplo` caem no texto de uso, sem dizer que nao existem.

```
$ braunrate validar cenario.yaml
erro no cenario: open cenario.yaml: no such file or directory
```

Sem consultar o README, nao ha como saber que existe `nome`, `alvo`, `carga`, `cenario`. Este e o unico achado do percurso em que a pessoa **para**, e ele acontece no primeiro minuto.

### A2 — Arquivo inexistente responde em ingles e nao ensina (incomoda)

A mensagem acima e o erro cru do Go, na unica parte do produto que nao fala portugues, e nao diz qual seria o proximo passo.

### A3 e A5 — Chave desconhecida lista as validas mas nao mostra a forma (atrasa)

```
$ braunrate validar cenario.yaml
erro no cenario: cenario.yaml:4:3: chave desconhecida em carga: "taxa"
    disponiveis: modelo, perfis
```

Saber que existe `perfis` nao ensina que `perfis` e uma lista de mapas com um tipo de perfil dentro. A mesma forma aparece em `autenticacao`: a chave errada lista `obter` sem mostrar que `obter` carrega uma requisicao inteira mais uma captura. Quando o bloco esta **ausente**, a mensagem ensina; quando esta **errado**, ensina menos:

```
erro no cenario: cenario.yaml:11:3: chave desconhecida em autenticacao: "url"
    disponiveis: tipo, obter, renovar_apos, cabecalho, usuario, senha
```

Erros que ja mostram exemplo (perfil desconhecido, passo que nao e mapa) resolveram em uma edicao. Os que so listam chave custaram duas.

### A4 — Sugestao dispara para palavra sem relacao (incomoda)

```
tipo de perfil desconhecido: "taxa"
    voce quis dizer "rampa"?
```

`taxa` nao e erro de digitacao de `rampa`. A distancia de edicao aceita 3 sem olhar o tamanho da palavra.

### A6 — Variavel de ambiente nao definida vira texto vazio (atrasa)

O cenario declara `senha: "${SENHA}"`. Sem `SENHA` no ambiente, a requisicao sai com senha vazia e nada e dito — nem em `validar`, nem em `depurar`, nem no relatorio. Contra uma API real, o resultado e 401 sem explicacao.

### A7 — Caminho fixo escapa da verificacao de variedade (atrasa)

O cenario roda `GET /pedidos/1` 3.000 vezes. O bloco de ambiente informa a variedade de `token`, mas **nada** sobre o caminho: como `/pedidos/1` e literal, ele nunca passa pela interpolacao e por isso nao entra na variedade observada. O alvo responde de cache e o relatorio nao tem uma linha sobre isso — exatamente o ponto cego que o [ADR 0007](adr/0007-variedade-observada.md) fecha para dados que variam.

---

## Jornada B — cenario que quebra

Partindo do cenario da jornada A, ja funcionando, tres defeitos, um por vez.

**6 comandos, 3 edicoes.**

### Defeito 1 — captura apontando para campo que nao existe

`braunrate depurar` resolve sozinho, e bem:

```
passo 1 — consultar pedido   [FALHOU em 6.5ms]
  resposta:   status 200, 89 bytes
  corpo:      {"id":"1","status":"ABERTO","ultimaFatura":{"id":"f-1",...}}
  problema:   nao consegui capturar "faturaId" com $.fatura.identificador: caminho nao encontrado no corpo da resposta
```

Corpo inteiro na tela e o caminho que falhou: suficiente. **Sem achado.**

### B2 — Quem foi direto para `executar` recebeu um verde falso (bloqueia)

```
Passou: as 1 regras de SLO foram atendidas.

O que aconteceu
  60 requisicoes em 3s, 20 por segundo, 100.00% de erro

A jornada inteira
  0 de 60 jornadas chegaram ao fim
...
$ echo $?
0
```

**Nenhuma jornada chegou ao fim, 100% de erro, veredito "Passou", codigo de saida 0.** Um pipeline com esse cenario fica verde. A regra declarada era so de latencia, e latencia de requisicao que falhou continua baixa. E o caso mais grave do percurso: nao e ausencia de informacao, e afirmacao errada.

### B8 — Passo que nunca executou some do relatorio (atrasa)

O passo 2 dependia da captura que falhou. Ele nao aparece em "Por passo" — nem com zero. Quem le nao descobre que existia um segundo passo.

### B9 — Linha de erro nao diz o status nem o passo (atrasa)

```
Erros
  status HTTP inesperado                             60
```

Qual status, em qual passo? O JSON tem; o terminal, nao.

### Defeito 2 — SLO impossivel de atingir

```
Falhou: "consultar pedido" teve latencia p95 de 7 ms, acima do limite de 1 ms.
```

Uma frase, o valor obtido, o limite e o passo, com codigo de saida 1. **Sem achado.**

### Defeito 3 — alvo fora do ar

`depurar` acerta o diagnostico, sem dizer o endereco que tentou:

```
nao consegui chegar ao primeiro passo: nao consegui obter a autenticacao: connection refused
```

`executar` **erra o diagnostico** (B4, bloqueia):

```
Erros
  erro de configuracao do cenario                    60
```

Nao ha erro de configuracao: o cenario esta certo e o alvo esta fora do ar. A pessoa vai procurar defeito onde nao tem. Alem de mandar para o lugar errado, a classificacao errada contamina a metrica — a ferramenta cujo argumento e classificar erro com honestidade classifica falha de rede como erro de cenario.

### B15 — Cabecalho "Por passo" com tabela vazia (incomoda)

Nenhuma requisicao chegou a ser feita, e o cabecalho da tabela e impresso mesmo assim, sem linha nenhuma.

---

## Jornada C — cadeia assincrona

Objetivo: publicar no Kafka e esperar o efeito **via HTTP**, partindo so do que a ferramenta ensina.

**4 comandos, 3 edicoes.** Nao terminou.

### C10 — Erro de sintaxe YAML sai cru, em ingles, sem arquivo nem coluna (atrasa)

```
$ braunrate validar cenario.yaml
erro no cenario: yaml: line 18: did not find expected ',' or '}'
```

A causa era `${pedidos.id}` sem aspas dentro de um mapa em linha — o caso mais comum de todos, porque `${` e `}` sao justamente o que o YAML em linha usa. Todo o resto do produto responde em portugues, com `arquivo:linha:coluna` e um exemplo; este caminho nao.

### C3 — Esperar efeito por HTTP e impossivel, e a mensagem nao diz o que fazer (bloqueia)

```
erro no cenario: cenario.yaml:19:7: chave desconhecida no passo aguardar: "http"
(use kafka, amqp, chave, campo, igual_a ou timeout)
```

A mensagem esta correta e a jornada acaba ali. `aguardar` so escuta topico e fila; nao existe passo que repita uma requisicao HTTP ate o efeito aparecer. Como boa parte dos sistemas assincronos so expoe o efeito por API, esta jornada — que e a terceira prova da tese — nao se escreve para eles.

### C11 — Timeout do `aguardar` diz "tempo esgotado" e imprime campo vazio (atrasa)

```
passo 2 — aguardar pedidos-auditoria-processados   [FALHOU em 5.213s]
  requisicao: aguardar em kafka pedidos-auditoria-processados por chave da mensagem = "2ad3f..."
              desiste depois de 5s
              enderecos:
  problema:   tempo esgotado
```

O cabecalho da requisicao diz o que era esperado, onde e por quanto tempo — isso esta certo. Falta o resto: nada chegou, entao o proximo passo e conferir se ha consumidor rodando e se os dois lados usam o mesmo valor de correlacao. E `enderecos:` sai vazio quando o endereco vem do alvo.

---

## Onde deu vontade de escrever "e so a pessoa saber que..."

Anotado durante o percurso, como manda a regra — cada um destes e achado:

- "e so saber que `perfis` e uma lista" → A3
- "e so saber que `obter` e uma requisicao inteira" → A5
- "e so exportar `SENHA` antes" → A6
- "e so olhar a linha de erro e ver que 100% falhou" → B2
- "e so saber que `${...}` precisa de aspas dentro de `{ }`" → C10
- "e so saber que `aguardar` nao fala HTTP" → C3

## O que foi feito

Corrigidos antes da Fase 7 (bloqueiam):

- **1** — `braunrate novo [cenario.yaml]` grava um cenario de partida comentado, com os blocos de dados e autenticacao no fim, comentados, e termina apontando `braunrate depurar`.
- **2** — execucao em que nenhuma jornada chegou ao fim falha com frase propria, mesmo sem SLO declarado que a pegue. Cenario sem bloco `slo` continua rodando e reportando como antes. Depois virou um dos seis casos da verificacao de sanidade, e a saida passou de 1 para 3: nao e o alvo que reprovou, e a medicao que nao vale ([ADR 0011](adr/0011-verificacao-de-sanidade.md)).
- **3** — `aguardar` passa a esperar por HTTP (`http:` mais `ate:`), sondando ate a condicao valer, com o intervalo de sondagem declarado no relatorio como granularidade da medicao.
- **4** — falha ao autenticar ganhou classe de erro propria ("nao consegui autenticar"), com o caminho da requisicao de token na mensagem.

Entram na Fase 7 (atrasam): **5**, **6**, **7**, **8**, **9**, **10** e **11**.

Ficam na lista, sem data (incomodam): **12**, **13**, **14** e **15**.
