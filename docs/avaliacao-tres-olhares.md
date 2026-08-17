# Avaliação em três olhares

Três públicos percorreram a ferramenta com critérios diferentes. Tudo aqui foi
executado: comando rodado, página aberta, saída colada. Nada é raciocínio sobre o
que aconteceria.

**O que este exercício não é.** Quem percorreu conhece o código por dentro e
completou caminhos que uma pessoa abandonaria. O valor está em percorrer com três
réguas distintas e registrar o que cada uma cobra. A lista do que só uma pessoa de
verdade revela está no fim, e é a parte mais importante.

Prioridade dos achados: **bloqueia** (não termina sozinho), **atrasa** (termina por
tentativa e erro), **incomoda** (termina, com atrito).

---

# Olhar 1 — QA especialista em performance

Quinze anos de JMeter. A pergunta dele: isto substitui o JMeter no meu time?

## QA-01 — Descobrir o que a ferramenta faz

**Resultado**: conseguiu | **Custo**: 3 leituras

About, README e site dizem a mesma coisa e na mesma ordem: o comportamento
primeiro, o jargão depois. O about foi reescrito nesta rodada e hoje diz que uma
pausa de 1 s aparece como ~1 s, e não como 4 ms.

**Achado**: o site promete, em Commands, *"Every option takes `-h`"*. Não é
verdade em dois comandos, e um deles tem efeito colateral — ver QA-02.
**Gravidade: bloqueia** (para quem tenta `new -h` antes de qualquer coisa).

## QA-02 — A bandeira que todo mundo tenta primeiro

**Resultado**: falhou | **Custo**: 13 comandos

`-h` foi testado nos treze comandos. Onze respondem certo. Dois não:

```
$ braunrate validate -h
error in the scenario: I could not find the file -h.

$ braunrate new -h
starting scenario at scenario.yaml: swap the target and the path for your service.
```

O primeiro trata a bandeira como nome de arquivo. O segundo **escreve um arquivo
em disco** — quem pediu ajuda ganhou `scenario.yaml` no diretório corrente, sem
ter pedido nada. Um QA que faz isso dentro de um repositório versionado tem um
arquivo novo para explicar.

**Comparação com o JMeter**: `jmeter -h` sempre responde, e nunca cria nada.

**Achado**: `-h` não é uniforme, e o site afirma que é. **Gravidade: bloqueia.**

## QA-03 — Montar teste de um endpoint autenticado

**Resultado**: conseguiu, na primeira | **Custo**: 1 comando

```
$ braunrate import curl 'curl http://127.0.0.1:8300/orders/1001 -H "Authorization: Bearer abc123xyz"' -output qa03.yaml
scenario written to qa03.yaml
warning: the header Authorization became ${token}: run with TOKEN=... in the environment, so a credential does not get versioned
warning: the load and slo numbers are a starting guess, not a measurement: tune them before using this as a gate
```

O token nunca chegou ao arquivo, e o aviso ensina por quê. O bloco `load` sai
preenchido com um chute **declarado como chute**, o que é mais honesto do que sair
vazio.

## QA-04 — Correlacionar token entre dois passos

**Resultado**: conseguiu, em 2 tentativas | **Custo**: 2 comandos, 1 edição

**Primeira tentativa** — referenciou `${access_token}` sem declarar de onde vem:

```
error in the scenario: qa04a.yaml:13:41: I do not know where ${access_token} comes from.
    declare where it comes from:
      variables: { access_token: value }                # fixed in the scenario
      capture: { access_token: $.field }                # from an earlier response
      data: { orders: { file: orders.csv } }  # and then ${orders.access_token}
```

A mensagem lista as quatro origens possíveis e a linha exata. A segunda tentativa
funcionou.

**Comparação com o JMeter**: lá seria um JSON Extractor com quatro campos e
escopo, e o erro apareceria como um cabeçalho vazio em tempo de execução, não como
recusa antes de rodar. **Aqui é melhor**, e por uma margem grande — é o ponto que
mais dói no JMeter.

## QA-05 — Massa de clientes em CSV

**Resultado**: conseguiu, na primeira | **Custo**: 1 arquivo, 1 comando

Equivalente ao CSV Data Set Config, com `consume: circular`. O relatório fecha
dizendo o que de fato variou:

```
5 distinct values of clientes.id across 100 uses, between 1,001 and 1,005
```

**Isto não existe no JMeter.** Lá, um CSV apontado para o arquivo errado roda o
teste inteiro com uma linha e ninguém percebe.

## QA-06 — Critério de aceite e código de saída

**Resultado**: conseguiu, na primeira | **Custo**: 3 execuções

```
SLO
  FAIL  Failed: "look up order" answered 95% within 7 ms, above the limit of 1 ms.

código de saída com SLO reprovado: 1
código de saída com alvo fora do ar: 3
```

Três estados distintos, e o 3 é o que nenhuma outra ferramenta tem: a execução não
mediu o que se propôs, então não afirma nada. Um gate de CI escrito com isto
distingue "o serviço está lento" de "o teste não valeu".

## QA-07 — Jornada de cinco passos

**Resultado**: conseguiu, em 3 tentativas | **Custo**: 3 comandos, 2 edições

**Primeira tentativa** — `${invoiceId}` dentro de um mapa inline:

```
error in the scenario: qa07.yaml:24:43: an inline map that does not close. Inside { }
YAML reads '{' and '}' as structure, and ${variable} carries both.
    quote the value, for example:
      kafka: { topic: orders, key: "${orders.id}" }
```

O diagnóstico é certeiro, mas a armadilha é do formato: a forma inline é
exatamente a que o `import curl` gera e a que os exemplos usam, e ela quebra assim
que entra uma variável. **Gravidade: atrasa.**

**Segunda tentativa** — um passo apontava para uma rota que o alvo não tem. O
`debug` parou no passo 1 e não disse que os outros três nunca rodaram; o
relatório de execução diz. **Gravidade: incomoda.**

## QA-08 — Ler o relatório e decidir

**Resultado**: conseguiu, em menos de dez segundos

O bloco que decide não é o dos números, é este:

```
SLO
  ok    Passed: "open order" answered 95% within 8 ms, within the limit of 300 ms.
  --    journey: no criterion declared — the gate measures isolated steps and leaves out the wait the user feels
  --    steps with no criterion declared (2 of 3): check order, pay invoice
  --    regression: no criterion declared — the gate approves without comparing against the previous run
```

A ferramenta enumera o que o gate **não** cobre. Um QA que leva isto para o
gerente sabe exatamente o tamanho da afirmação que está fazendo.

E fecha declarando o que pode ter inflado o número:

```
1 single value of token across 90 uses
Auth obtained once and reused by every journey.
If the target has caching, rate limiting or sharding by token, this number comes out optimistic.
```

## QA-09 — Trazer um `.jmx` que ele já tem

**Resultado**: conseguiu, na primeira | **Custo**: 1 comando

Converteu, e disse a única coisa que importa nessa conversão:

```
warning: the group "Usuarios" declares 50 threads, ramp of 30s, 300s of duration:
a thread count does not turn into an arrival rate, because a thread only sends
after the previous response. The 'load' block came out as a guess; swap it for
the rate you want to sustain (requests per second)
```

É a diferença conceitual entre as duas ferramentas, dita no momento em que o QA
mais precisa ouvir.

## QA-10 — Comparar com a execução da semana passada

**Resultado**: conseguiu, na primeira | **Custo**: 2 execuções, 1 comando

```
It got slower: the whole journey (95%): 3.7 times slower — from 22 ms to 80 ms.

Per step
  step                        95% before   95% after           change
  check order                     6.8 ms       27 ms       3.9x worse

What could explain the difference other than the service
  - both runs used one token for everything; caching or sharding by identity affects them the same way
  Two runs give no confidence interval: a change below 5% is treated as noise.
```

Detectou a piora que eu introduzi de propósito (latência do alvo de 1 ms para
25 ms), quantificou, e listou o que poderia explicar o número fora do serviço.

## Veredito do QA

**Trocaria o JMeter por isto?** Para teste de API em CI, sim. Para o resto, ainda
não.

**O que é melhor, com margem:**
- Correlação recusada antes de rodar, em vez de falhar como 401 no meio da carga.
- A variedade observada publicada no relatório — o defeito mais silencioso de
  todos, e nenhuma ferramenta que ele conhece detecta.
- Código de saída 3: a execução que não mediu não dá veredito.
- O relatório enumera o que o gate não cobre.

**O que faz falta:**
- Interface gráfica para montar o plano. A web existe, mas não é o JMeter.
- Relatório agregado de várias execuções ao longo do tempo — `compare` é de duas
  em duas.
- Sem distributed testing: uma máquina só.
- `-h` não uniforme, e um deles escreve arquivo.

---

# Olhar 2 — Designer de UX/UI

Não sabe o que é teste de carga. A pergunta dele: alguém usa isto sem ser
ensinado?

**Nota de método**: a extensão de navegador travou o renderizador três vezes
seguidas. O percurso foi refeito com Chrome headless — DOM medido, capturas
renderizadas. Clique real e toque real ficaram sem cobrir, e estão na lista do
fim.

## UX-01 — `braunrate` sem argumento

**A próxima ação é óbvia?** Sim. A tela não lista comandos: ela pergunta em que
situação a pessoa está e dá um caminho para cada uma.

```
Never used it? See it working in 30 seconds:
    braunrate demo

Already have an API to test? The path is:
    1. braunrate import curl '...'
    2. braunrate debug scenario.yaml
    3. braunrate execute scenario.yaml
```

## UX-02 / UX-04 — A dobra, no desktop e no celular

No desktop a dobra passa no teste de cinco segundos. **No celular, não.**

Medido em viewport real de 375 px, dentro de um iframe (a janela do headless
mente sobre a própria largura):

```
{"vw":375, "scrollW":375, "hScroll":false, "heroTop":629, "overCount":21, "smallCount":20}
```

- **Não há rolagem horizontal.** O layout responsivo funciona, as tabelas rolam
  dentro de si (`table { display:block; overflow-x:auto }`), o texto quebra.
- **`heroTop: 629`** — o que o produto é começa a 629 px do topo. A primeira tela
  inteira de um celular é o menu: dez links antes de qualquer palavra sobre a
  ferramenta. O teste de cinco segundos falha por completo no celular.
- **20 alvos de toque abaixo de 32 px**: links do menu com 18 px de altura,
  "Português" com 24, a âncora `#` com 20. A recomendação de plataforma é 44.

**Gravidade: atrasa** — a pessoa rola e chega lá. Mas para uma página cuja função
é convencer em cinco segundos, é o pior lugar possível para gastar uma tela.

## UX-03 — Navegação do site

Consistente. Três seções (Start, Guides, Reference), nomes que dizem o conteúdo,
e a paginação no rodapé leva ao próximo na ordem em que alguém aprenderia. A
única página cujo conteúdo não é o que o menu promete é `decisions.html`: o menu
está em inglês e os títulos dos ADRs estão em português — declarado na primeira
linha da própria página.

## UX-05 — Interface web, primeira abertura

**Convida.** O estado vazio não é uma tela em branco com um botão:

```
No scenario in this folder yet
The interface edits the .yaml files of ../uivazio. Three ways to get the first one:
  Start from scratch    a short form that writes a commented scenario
  Import a cURL         paste the command copied from the browser's network panel
  See the demonstration runs a complete example, nothing to configure
```

Diz o que a interface faz com os arquivos, e cada caminho vem com uma linha
explicando o que ele faz. **Isto está certo e não deve ser mexido.**

## UX-06 / UX-07 — Montar cenário e errar a sintaxe

O erro volta com linha e coluna separadas do texto, então o editor consegue
marcar o ponto:

```json
{"valid": false,
 "message": "error in the scenario: quebrado.yaml:4:1: an inline map that does not close.
             check that every key has a value and that keys are separated by commas,
             for example:\n      http: { method: GET, path: /orders }",
 "line": 4, "column": 1}
```

**O erro ensina**, não só aponta: mostra a forma certa. A linha indicada é a 4
(onde o mapa abre) e o erro humano está na 5 (vírgula faltando) — é como o YAML
reporta, e a mensagem compensa mostrando o formato.

## UX-08 — Execução ao vivo

**Saber o que está acontecendo: sim.** Uma linha por segundo, com tudo que
importa:

```
running "Orders by customer" against http://127.0.0.1:8300: 400 iterations in 8s
load 50/s | sent 51 | completed 49 | errors 0 | half within 26.1 ms | 99% within 27.9 ms | 7s left
load 50/s | sent 101 | completed 99 | errors 0 | half within 26.1 ms | 99% within 26.8 ms | 6s left
```

**Cancelar: não existe.** Três rotas plausíveis, três recusas:

```
POST   /runs/r001/cancel  -> HTTP 405 Method Not Allowed
DELETE /runs/r001         -> HTTP 405 Method Not Allowed
POST   /runs/r001/stop    -> HTTP 405 Method Not Allowed
```

O roteador tem doze rotas e nenhuma encerra uma execução. Quem apontou a carga
para o ambiente errado só sai matando o processo — o que derruba o servidor e
qualquer outra execução junto.

**Gravidade: bloqueia** a tarefa "cancelar". Numa ferramenta que dispara carga
contra sistemas de verdade, é a ação que mais urge quando urge.

## UX-09 / UX-10 — Hierarquia do relatório

**O veredito domina**, e domina igual nos dois casos. Válido: verde, tipo maior,
primeira linha. Inválido: vermelho, mesma posição, com a frase que separa as duas
coisas que todo mundo confunde:

> Invalid result: the run did not measure what it set out to measure.
> This is not a verdict on the target — it is the measurement that does not hold,
> and that is why no SLO rule was evaluated.

E dois blocos dizendo por quê: *"no journey reached the end"*, *"the step failed
on 100% of the requests"*.

**Uma tensão**: abaixo do veredito vermelho, o bloco "What happened" mostra
`0.990 ms | 95% of the responses within` com o mesmo peso visual de uma execução
válida. O texto diz que o número não vale; o cartão não. Quem recorta a tela
recorta um número que a página acabou de invalidar. **Gravidade: incomoda.**

## UX-11 — Consistência de vocabulário

O projeto tem `docs/vocabulario.md` como critério de aceitação, com termo oficial
e lista do que nunca usar. A varredura não achou violação real — "gate" aparece
em "turn it into a CI gate", que é o mecanismo, não o nome do conceito.

**Uma divergência entre superfícies**: o relatório HTML abre com *"Passed: all 2
SLO rules were met"*, enquanto a demonstração ensina *"That is an acceptance
criterion"* e a tabela de vocabulário define **acceptance criterion** como termo
oficial, com `slo` sendo só a chave do YAML. Duas superfícies, dois nomes para o
mesmo conceito. **Gravidade: incomoda.**

## UX-12 — Acessibilidade

**O site está certo:**
- `:focus-visible { outline: 2px solid var(--brand) }`
- Controles que aparecem no hover também aparecem no foco:
  `.block:hover .copy, .block .copy:focus` e `h2:hover .anchor, .anchor:focus` —
  quem navega por teclado alcança copiar e âncora.
- `@media (prefers-reduced-motion: reduce)` respeitado.
- Contraste AA travado por teste (`TestEveryColorMeetsAA`, verde).

**A interface web não:**

```
$ grep -c ":focus" internal/ui/app/style.css
0
```

Nenhum estilo de foco em toda a folha da interface. Quem navega por teclado não
vê onde está, num produto cuja tela principal é um editor com botões.
**Gravidade: atrasa**, e é uma inconsistência com o site ao lado.

**No toque**: os controles escondidos atrás de `:hover` (copiar em bloco de
código, âncora de seção) não têm estado de toque. Em celular o botão de copiar
depende do navegador emular hover.

## As três piores fricções, na ordem

1. **Não dá para cancelar uma execução.** Não há caminho, e a ação é urgente por
   natureza. É a única das três que bloqueia.
2. **O celular gasta a primeira tela inteira com o menu.** A página existe para
   convencer em cinco segundos e usa esses cinco segundos numa lista de links.
3. **A interface web não tem foco de teclado.** Nenhum estilo, em nenhum
   elemento, num editor.

## As três decisões que estão certas

1. **O estado vazio da interface.** Explica o que ela faz com os arquivos e
   oferece três caminhos, cada um com o que ele faz.
2. **O veredito de resultado inválido.** Separa "o alvo está ruim" de "a medição
   não vale" — a distinção que nenhuma outra ferramenta faz, no lugar de mais
   destaque da página.
3. **`braunrate` sem argumento pergunta em que situação a pessoa está**, em vez
   de listar comandos.

---

# Olhar 3 — Desenvolvedor que nunca fez teste de performance

Conhece Go, Docker e CI. Recebeu "descubra se nossa API aguenta a Black Friday".
A pergunta dele: eu consigo fazer isso hoje, sozinho?

## DEV-01 — Escolher a ferramenta

**O que convence**: a comparação de omissão coordenada na dobra, com número dos
dois lados e o comando que a reproduz. Ele não precisa entender o conceito para
entender que uma ferramenta mostrou 972 ms onde a outra mostrou 3,5 ms.

**O que não convence**: nada compara braunrate com k6 ou Gatling por escrito. O
site cita JMeter e Locust ao explicar laço fechado, e o about cita
`k6-alternative` como tópico, mas quem chega decidindo entre três ferramentas não
acha uma tabela. Ele decide pelo que entendeu da dobra.

## DEV-02 — Instalar

**Resultado**: conseguiu | **Custo**: 1 comando

`go install` funciona, e a página de instalação explica os três caminhos e o
aviso de primeira execução no macOS. O binário sai com versão `dev`, e a página
diz por quê — não é o artefato de release.

## DEV-03 — Entender o que medir

**A ferramenta ensina.** `braunrate demo` explica cada número no momento em que
ele aparece, incluindo por que não há média:

```
Notice there is no average on that line. An average hides things: if 95
responses take 5 ms and 5 take 2 seconds, the average reads 105 ms and
nobody notices the five slow ones.
```

Isto responde a pergunta que o iniciante nem sabia fazer.

## DEV-04 — Escrever o primeiro cenário

**Resultado**: conseguiu | **Custo**: 1 comando

`braunrate new` escreve um arquivo comentado, com a linha do `$schema` no topo
que liga o autocomplete do editor. Os blocos opcionais vêm comentados embaixo,
com dados, autenticação e os outros protocolos.

## DEV-05 — Escolher a carga: **quanto é muito?**

**Resultado**: não conseguiu. Nada na ferramenta responde.

Este é o achado que o percurso do desenvolvedor existe para encontrar.

**1. O arquivo inicial entrega um número sem dizer de onde ele veio:**

```yaml
# Arrival rate, in requests per second. Not a number of users: the generator
# fires on schedule even when the target is slow, which is what keeps the
# measurement from hiding a freeze.
load:
  profiles:
    - ramp: { from: 1/s, to: 20/s, duration: 30s }
    - steady: { rate: 20/s, duration: 1m }
```

O comentário explica **o que a taxa é**, com precisão. Não explica **como
escolher a sua**. Por que 20? O arquivo não diz, e o iniciante não tem como
saber se 20 é o tráfego de uma terça-feira ou de uma Black Friday.

**2. O site nunca ensina a escolher.** Varredura por `peak`, `busiest`,
`production traffic`, `how do I know`, `start with`: **zero ocorrências** nos
oito guias. As receitas cobrem login, dados, várias rotas, reprovar o build e
comparar execuções — a mecânica inteira, e nada sobre quanto. Não existe receita
de "achar o teto": subir a carga até o alvo quebrar é o exercício mais comum de
quem recebeu a tarefa da Black Friday, e ele não está documentado.

**3. Não há guarda-corpo nenhum contra um número absurdo:**

```
$ braunrate validate dev-absurdo.yaml     # rate: 50000/s
Valid scenario: "My first scenario", 1 step, 3750015 iterations in 1m30s.
Warning: the step "look up order" has no value that varies — every request will be identical.
```

A ferramenta avisa sobre o caminho fixo e **não diz uma palavra** sobre estar
prestes a disparar cinquenta mil requisições por segundo. Ela chega a imprimir
`3750015 iterations`, que é a informação certa no formato que o iniciante não
lê como "isto é enorme".

Vale registrar o que **existe** e quase resolve: se o gerador não sustentar a
taxa, o resultado sai inválido com a explicação. Ou seja, a ferramenta protege o
número contra a máquina dela, e não o alvo contra o usuário.

**Gravidade: bloqueia** o critério declarado deste olhar. Ele termina com um
número, mas não com um número que consiga justificar numa reunião — e
justificar era a tarefa.

## DEV-06 / DEV-07 — Rodar e decidir

Com uma taxa escolhida no chute, o resto funciona: o relatório explica p95 em
palavras ("95% das respostas em até X"), separa taxa disparada de vazão
completada, e o bloco de SLO enumera o que não foi declarado. Sem critério
declarado, a ferramenta **diz** que sem critério ela descreve mas não aprova nem
reprova, e ensina a declarar um.

## DEV-08 — Explicar para o time

O que ele leva para a reunião é forte: percentil em vez de média, com a frase que
justifica a escolha; o aviso de que um token só foi usado em todas as jornadas;
e a variedade observada. O que ele **não** consegue defender é a pergunta óbvia
do time — "por que 50 por segundo?" — pelo motivo do DEV-05.

## DEV-09 — Colocar no CI

Direto: `braunrate execute` sai com 0, 1 ou 3, e a receita "Make the test fail the
build" está escrita. Os três códigos foram verificados no percurso do QA.

## DEV-10 — Usar a DSL em Go de fora do módulo

**Resultado**: conseguiu | **Custo**: 3 tentativas

Módulo novo, `go.mod` próprio, importando só a superfície pública:

```
$ go run .
válido: true | passou: true
```

**Uma armadilha no caminho.** O teste publicado ao lado do exemplo tem
`_ "github.com/Diegobraun/braunrate/internal/protocol/http"`. Quem copiar essa
linha recebe:

```
main.go:11:2: use of internal package github.com/Diegobraun/braunrate/internal/protocol/http not allowed
```

O trecho que o README publica não tem essa linha, e sem ela funciona — o pacote
público já registra os protocolos. Mas quem abre o arquivo de teste ao lado copia
uma linha que não pode existir fora do módulo, e o erro não diz que ela é
desnecessária. **Gravidade: incomoda.**

## Em que momento ele pensou em desistir

No DEV-05, e não por erro nenhum: por não ter em que se apoiar. Todos os outros
passos falham com uma mensagem que ensina; este não falha — ele simplesmente
aceita qualquer número e segue, e a dúvida fica com a pessoa.

## O que ele ainda não entende no fim

- Se o número que ele escolheu tem relação com o tráfego real da aplicação.
- O que fazer quando o alvo aguenta: subir quanto, de quanto em quanto, até onde.
- A diferença entre "meu serviço aguenta" e "meu serviço aguentou este cenário".

---

# Síntese

## 1. Fricções que aparecem em mais de uma persona

**A carga sem referência (DEV-05, e o QA no QA-09).** O desenvolvedor não sabe
que número usar. O QA recebe do `import jmx` o aviso de que 50 threads não viram
taxa — e nenhum dos dois recebe ajuda para chegar à taxa certa. É a mesma
lacuna vista de dois lados, e a mais cara das três.

**`${variável}` dentro de mapa inline (QA-07, e o DEV ao editar o arquivo do
`new`).** A forma inline é a que a ferramenta gera e a que os exemplos ensinam, e
ela quebra assim que entra a primeira variável.

**O que a ferramenta não cobre é dito, o que ela não sabe escolher não é.** As
duas personas elogiaram o mesmo bloco — a lista do que não foi declarado — e as
duas esbarraram na ausência de orientação sobre a carga.

## 2. Conflitos entre personas

**Explicação inline.** A demonstração explica cada número enquanto ele aparece, e
foi o que salvou o DEV-03. O QA com quinze anos de estrada lê as mesmas linhas
como ruído entre ele e a tabela. Hoje a explicação está sempre ligada. As opções
são deixar como está (o iniciante ganha, o veterano rola), um `--brief`, ou
lembrar da primeira execução — **não resolvo sozinho: é decisão de produto.**

**Recusar credencial literal.** O QA quer colar um curl e rodar; a recusa custa
uma variável de ambiente a mais no primeiro minuto. O ganho é não versionar
segredo. O `import curl` já resolve o conflito bem — mascara e explica — mas a
recusa no `validate` continua sendo atrito para quem está testando local contra
um alvo de mentira.

**Cancelar execução.** Só o UX cobrou. O QA roda no terminal e usa Ctrl-C; o DEV
roda no CI, onde ninguém cancela. Resolver serve à interface web, que é a
superfície com menos uso hoje — o que não torna o achado menor, torna a
prioridade discutível.

## 3. Fricções específicas de uma persona

- **Sem foco de teclado na interface web** (só UX). Vale resolver: é barato e o
  site ao lado já faz certo.
- **Menu ocupando a primeira tela no celular** (só UX). Vale resolver: a página
  existe para convencer em cinco segundos.
- **Sem interface gráfica de montagem tipo JMeter** (só QA). Não vale: é o
  produto que o projeto recusou ser, e está declarado.
- **Import do `internal/` no arquivo de teste do exemplo** (só DEV). Barato.

## 4. O que cada persona elogiou — não estragar

| Persona | O que elogiou |
|---|---|
| QA | Correlação recusada antes de rodar, com as quatro origens listadas |
| QA | Variedade observada no relatório: 5 valores em 100 usos, dito sem ser perguntado |
| QA | Código de saída 3 — a execução que não mediu não dá veredito |
| QA | O `import jmx` dizendo que thread não vira taxa |
| UX | Estado vazio da interface: três caminhos, cada um com o que faz |
| UX | Veredito de resultado inválido separando alvo de medição |
| UX | `braunrate` sem argumento perguntando em que situação a pessoa está |
| DEV | A demonstração explicando por que não há média |
| DEV | O relatório enumerando o que **não** foi declarado |

## 5. A lista honesta

**O que só consegui por conhecer o código:**
- Achei a ausência de rota de cancelamento lendo o roteador, não tentando
  cancelar pela tela.
- Medi o layout do celular injetando script na página, porque a extensão de
  navegador travou; uma pessoa teria simplesmente aberto no telefone.
- Descobri que a DSL funciona sem o import `internal/` porque fui ler o que o
  pacote público registra — o desenvolvedor teria parado no erro de compilação.
- Escrevi o cenário de cinco passos sabendo quais rotas o alvo embutido tem.
- Sabia que `${var}` em mapa inline quebra antes de escrever, e mesmo assim
  errei — o que sugere que uma pessoa erra sempre.

**O que só uma pessoa de verdade revela:**
- Se a explicação inline da demonstração ajuda ou irrita **na segunda execução**.
- Onde a atenção vai primeiro no relatório HTML: no veredito ou nos cartões.
- Se "acceptance criterion" e "SLO" sendo a mesma coisa confunde de fato.
- Quanto tempo alguém aguenta antes de desistir de escolher a taxa sozinho.
- Se o menu do celular faz a pessoa fechar a página ou rolar.
- Toque real: se o botão de copiar aparece no celular.
- Se o QA confia no número na primeira vez, ou roda o JMeter em paralelo para
  conferir — e o que ele conclui se os dois discordarem.
