# Solucao de problemas

Os erros mais provaveis, pelo que aparece na tela. Todos eles ja aconteceram com
alguem aqui.

## "Ninguem atendeu em http://..."

```
passo 1 — get pedidos 1   [FALHOU em 500µs]
  problema:   falha de rede
              connection refused

Ninguem atendeu em http://127.0.0.1:8080. Confira se o servico esta no ar e se o endereco esta certo.
Se voce ainda nao tem um servico para testar, suba o embutido em outro terminal:
  braunrate target
```

O alvo do cenario nao esta rodando, ou o endereco esta errado. Se voce so quer
experimentar a ferramenta, `braunrate demo` nao precisa de alvo nenhum.

## Tudo respondeu 401 ou 403

```
  resposta:   status 401, 36 bytes
  corpo:      {"erro":"token ausente ou invalido"}

O alvo recusou por credencial, e o cenario nao declara autenticacao nenhuma.
```

Declare de onde vem o token. O braunrate obtem uma vez e reaproveita em todas as
jornadas:

```yaml trecho
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
    captura: { token: $.access_token }
```

Se ja existe bloco de autenticacao e mesmo assim vem 401, rode `braunrate debug`:
ele mostra a requisicao de obtencao do token e o que ela capturou.

## "Resultado invalido" e codigo de saida 3

A execucao nao mediu o que se propos a medir, entao nenhuma regra de criterio de
aceite foi avaliada. Nao e veredito sobre o alvo. A propria mensagem lista qual
dos [seis casos](conceitos.html#resultado-invalido) aconteceu; os dois mais
comuns:

- **nenhuma jornada chegou ao fim** — algum passo esta falhando sempre. Rode
  `braunrate debug` para ver onde a iteracao para.
- **o gerador nao sustentou a taxa** — a maquina que gera nao deu conta. Baixe a
  taxa, ou gere de uma maquina maior. Os numeros dessa execucao medem o gerador,
  nao o alvo.

## "nao sei de onde vem ${...}"

```
erro no cenario: cenario.yaml:14:26: nao sei de onde vem ${faturald}.
    voce quis dizer "faturaId"?
    disponiveis: faturaId, tenant
```

Uma referencia que nao vem de `variaveis`, de `dados`, de uma `captura` anterior
nem do ambiente em CAIXA ALTA. Antes ela virava texto vazio em silencio: a
requisicao saia com o campo em branco, o alvo respondia 401 ou 404, e nada na
saida ligava uma coisa a outra.

## "a variavel de ambiente X nao esta definida"

```
erro no cenario: cenario.yaml:5:24: taxa invalida: "${TAXA}/s" (use por exemplo 50/s)
    a variavel de ambiente TAXA nao esta definida, entao este campo ficou com a referencia crua.
    rode com TAXA=... , ou declare um padrao no arquivo: ${TAXA:-valor}
```

Ou defina a variavel, ou escreva a reserva no arquivo. Nao existe terceira
opcao: apagar a referencia em silencio produziria um campo vazio.

## "senha literal no cenario"

```
erro no cenario: homolog.yaml:7:77: senha literal no cenario: credencial nunca vai para o arquivo, porque o arquivo vai para o repositorio.
    troque por:  senha: ${BROKER_SENHA}
    e rode com:  BROKER_SENHA=... braunrate execute cenario.yaml
    valor de reserva (${VAR:-algo}) tambem nao serve: a reserva seria o segredo escrito no arquivo
```

Nao ha como desligar. Credencial so por variavel de ambiente ou pela cadeia
padrao da nuvem.

## "chave desconhecida no topo do cenario"

```
erro no cenario: cenario.yaml:3:1: chave desconhecida no topo do cenario: "carg"
    voce quis dizer "carga"?
    disponiveis: nome, alvo, requer, variaveis, autenticacao, tls, mensageria, dados, carga, cenario, slo
```

A lista completa de chaves aceitas esta em
[Referencia do cenario](referencia.html), gerada do mesmo schema que o seu editor
usa para completar. Ligue o autocompletar colocando esta linha no topo do
arquivo:

```yaml trecho
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
```

## "certificado assinado por CA que esta maquina nao conhece"

Declare a autoridade interna no bloco `tls` do topo do cenario — veja
[Protocolos](protocolos.html#alvo-https-com-certificado-proprio). Nao existe
opcao para desligar a verificacao: certificado autoassinado funciona como a
propria CA, e a opcao seria a saida facil para quem nao quer configurar.

## O passo 2 parece rapido demais

Nao e engano da ferramenta, e o que o numero significa. So o primeiro passo tem
instante agendado proprio; do segundo em diante o tempo e contado de quando o
passo anterior terminou. Para a leitura honesta, use o bloco **"A jornada
inteira"** — a explicacao completa esta em
[Conceitos](conceitos.html#o-tempo-do-passo-2-em-diante-nao-e-corrigido).

## O relatorio avisa que nao ha variedade

```
Atencao: o passo "consultar pedido" nao tem nenhum valor que varia — toda requisicao vai ser identica.
    se o alvo guardar resposta por essa chave, o numero sai otimista.
```

Requisicao sempre igual mede o cache do alvo. Troque o valor fixo por uma
referencia e declare de onde ela vem — CSV ou gerador, em
[Receitas](receitas.html#dados-um-valor-por-jornada-nao-por-requisicao). Se o
caminho fixo for proposital, como num teste de fumaca, o aviso continua correto e
nao invalida nada.

## O macOS ou o Windows bloqueiam o binario

Os dois avisam que o desenvolvedor nao pode ser verificado, porque nao ha
assinatura de codigo. A instrucao para liberar esta em
[Instalacao](instalacao.html#1-baixar-o-binario-da-release), junto com o motivo
de a assinatura nao existir.

## `braunrate version` responde `dev`

O binario foi compilado localmente ou instalado com `go install`, entao nao tem
versao injetada. Isso viaja para o documento de resultado, e `braunrate compare`
sai sem veredito quando as duas execucoes usaram versoes diferentes. Para um
numero de versao de verdade, baixe o artefato da release.

## `braunrate serve` responde 409

Ja existe uma execucao em andamento. Duas execucoes na mesma maquina disputam a
CPU que precisa despachar no instante agendado, e nenhuma das duas mede o que se
propos a medir. A resposta diz como aceitar a contaminacao, se for esse o caso.
