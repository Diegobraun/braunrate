---
translated_from: 70-guides-troubleshooting.en.md
source_hash: f4e6fd7c45d7
---
# Solução de problemas

Esta página lista os erros mais comuns pelo texto que aparece na tela, a causa de
cada um e o que fazer. Todas as mensagens citadas são saída real da ferramenta.

| Sintoma | Causa provável |
|---|---|
| [`connection refused`](#o-alvo-nao-respondeu) | o alvo não está no ar, ou o endereço está errado |
| [`status 401` ou `403` em tudo](#todas-as-requisicoes-voltam-401-ou-403) | o cenário não declara autenticação |
| [`Invalid result`, código 3](#resultado-invalido-e-codigo-de-saida-3) | a execução não mediu o que se propôs a medir |
| [`I do not know where ${...} comes from`](#referencia-a-variavel-sem-origem) | referência sem origem declarada |
| [`the environment variable X is not set`](#variavel-de-ambiente-nao-definida) | falta a variável, ou falta um valor de reserva |
| [`literal password in the scenario`](#segredo-escrito-no-arquivo) | credencial escrita no arquivo |
| [`unknown key at the top of the scenario`](#chave-desconhecida) | erro de digitação, ou chave que não existe |
| [`certificate signed by a CA this machine does not know`](#certificado-de-ca-interna) | falta declarar a CA interna |
| [tempos do passo 2 baixos demais](#os-tempos-do-segundo-passo-parecem-baixos-demais) | do segundo passo em diante o tempo é de serviço |
| [aviso de que nada varia](#o-relatorio-avisa-que-nenhum-valor-varia) | o cenário manda sempre a mesma requisição |
| [o sistema bloqueia o binário](#o-macos-ou-o-windows-bloqueiam-o-binario) | não há assinatura de código |
| [`braunrate version` responde `dev`](#a-versao-aparece-como-dev) | binário compilado localmente |
| [`409` no modo servidor](#o-modo-servidor-responde-409) | já existe uma execução em andamento |

## O alvo não respondeu

```
step 1 — get orders 1   [FAILED in 3.5ms]
  request:    GET /orders/1
  problem:    network failure
              connection refused

Nobody answered at http://127.0.0.1:9. Check that the service is up and that the address is right.
If you do not have a service to test yet, start the built-in one in another terminal:
  braunrate target
```

**Causa.** Nada está escutando no endereço declarado em `alvo`.

**Solução.** Confira o endereço e suba o serviço. Se você ainda não tem um
serviço para apontar, o braunrate traz um:

```bash
braunrate target -latency=5ms
```

> **Dica** Para só experimentar a ferramenta, `braunrate demo` não precisa de
> alvo nenhum: ele sobe o alvo, roda e explica o resultado em um comando.

## Todas as requisições voltam 401 ou 403

```
  response:   status 401, 36 bytes
  body:       {"error":"missing or invalid token"}
  problem:    unexpected HTTP status
              status 401

The target refused for lack of a credential, and the scenario declares no auth
at all.
```

**Causa.** O alvo exige credencial e o cenário não tem bloco `auth`.

**Solução.** Declare como o token é obtido. O braunrate faz o login uma vez, na
preparação, e injeta o token em todos os passos:

```yaml fragment
auth:
  type: token
  obtain:
    http: { method: POST, path: /auth/token, body: { user: ana } }
    capture: { token: $.access_token }
```

Se o bloco já existe e mesmo assim vem 401, rode `braunrate debug`: ele mostra a
requisição de obtenção do token e o valor que foi capturado.

## Resultado inválido, e código de saída 3

**Causa.** A verificação de sanidade decidiu que a execução não mediu o que se
propôs a medir. Nenhuma regra de critério de aceite chega a ser avaliada.

**Solução.** A própria mensagem diz qual dos
[seis casos](concepts.html#resultado-invalido) aconteceu. Os dois mais comuns:

- **nenhuma jornada chegou ao fim.** Algum passo falha sempre. Rode
  `braunrate debug` para ver onde a iteração para.
- **o gerador não sustentou a taxa.** A máquina que gera não deu conta. Reduza a
  taxa, ou gere de uma máquina maior.

> **Importante** Código 3 não é veredito sobre o alvo. Código 1 quer dizer "o
> alvo não atendeu ao critério"; código 3 quer dizer "esta execução não serve
> para afirmar nada".

## Referência a variável sem origem

```
error in the scenario: cenario.yaml:7:26: I do not know where ${faturald} comes from.
    declare where it comes from:
      variables: { faturald: value }                # fixed in the scenario
      variables: { faturald: "${FATURALD:-fallback}" }  # from the environment, with a fallback
      capture: { faturald: $.field }                # from an earlier response
      data: { orders: { file: orders.csv } }  # and then ${orders.faturald}
    an UPPERCASE name comes from the environment with nothing to declare: ${FATURALD}
```

**Causa.** A referência não vem de `variables`, de `data`, de uma `capture`
anterior nem do ambiente em CAIXA ALTA.

**Solução.** Corrija o nome, ou declare a origem. A recusa existe porque antes
essa referência virava texto vazio em silêncio: a requisição saía com o campo em
branco, o alvo respondia 401 ou 404, e nada na saída ligava uma coisa à outra.

## Variável de ambiente não definida

```
error in the scenario: cenario.yaml:5:23: invalid rate: "${TAXA}/s" (use for example 50/s)
    the environment variable TAXA is not set, so this field kept the raw reference.
    run with TAXA=... , or declare a default in the file: ${TAXA:-value}
```

**Causa.** O campo referencia uma variável que não existe no ambiente e não tem
valor de reserva.

**Solução.** Defina a variável na execução, ou escreva a reserva no arquivo:

```bash
TAXA=100 braunrate execute cenario.yaml
```

## Segredo escrito no arquivo

```
error in the scenario: homolog.yaml:6:65: literal password in the scenario: a credential never goes into the file, because the file goes into the repository.
    replace it with:  password: ${BROKER_PASSWORD}
    and run with:  BROKER_PASSWORD=... braunrate execute cenario.yaml
    a fallback value (${VAR:-something}) does not work either: the fallback would be the secret written in the file
```

**Causa.** Um campo de credencial tem valor literal.

**Solução.** Troque pelo nome de uma variável de ambiente. Valor de reserva
(`${VAR:-algo}`) também é recusado, porque a reserva seria o próprio segredo
escrito no arquivo.

> **Nota** Não há como desligar essa recusa. Credencial só entra por variável de
> ambiente ou pela cadeia padrão da nuvem.

## Chave desconhecida

```
error in the scenario: cenario.yaml:3:1: unknown key at the top of the scenario: "carg"
    available: name, target, requires, variables, auth, tls, messaging, data, load, scenario, slo
    a minimal scenario has four of them:
      name: Order lookup
      target: http://127.0.0.1:8080
      load: { profiles: [ { steady: { rate: 100/s, duration: 1m } } ] }
      scenario:
        - http: GET /orders/1
```

**Causa.** A chave não existe, ou tem erro de digitação.

**Solução.** A lista completa está em [Referência do cenário](reference.html).
Para o editor completar as chaves e apontar o erro antes de rodar, coloque esta
linha no topo do arquivo:

```yaml fragment
# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
```

## Certificado de CA interna

```
look up      network failure   30    certificate signed by a CA this machine does…
  certificate signed by a CA this machine does not know — declare tls: { ca: /path/ca.pem }
```

**Causa.** O alvo serve HTTPS com uma autoridade que a máquina não conhece.

**Solução.** Declare a CA no topo do cenário — veja
[Protocolos](protocols.html#alvo-https-com-certificado-proprio).

> **Nota** Não existe opção para desligar a verificação do certificado. Um
> certificado autoassinado funciona como a própria CA, e a opção seria a saída
> fácil para quem não quer configurar.

## Os tempos do segundo passo parecem baixos demais

**Causa.** Não é engano da ferramenta: só o primeiro passo tem instante agendado
próprio. Do segundo em diante, o tempo é contado de quando o passo anterior
terminou, porque o passo depende de um valor capturado antes dele.

**Solução.** Leia o bloco **"A jornada inteira"**, que conta do instante em que a
jornada deveria ter começado. A explicação completa está em
[Conceitos](concepts.html#o-tempo-do-passo-2-em-diante-nao-e-corrigido).

## O relatório avisa que nenhum valor varia

```
Warning: the step "look up order" has no value that varies — every request will be identical.
    if the target caches by that key, the number comes out optimistic.
    to make it vary:  data: { orders: { file: orders.csv } }  and then  GET /orders/${orders.id}
```

**Causa.** O passo não tem nenhuma referência `${}`, então todas as requisições
são iguais.

**Solução.** Troque o valor fixo por uma referência e declare de onde ela vem, em
[Receitas](recipes.html#cada-jornada-precisa-de-dados-proprios).

> **Nota** Se o caminho fixo é proposital, como num teste de fumaça, o aviso
> continua correto e não invalida nada.

## O macOS ou o Windows bloqueiam o binário

**Causa.** Não há assinatura de código nem notarização, então os dois sistemas
avisam que o desenvolvedor não pode ser verificado.

**Solução.** A instrução para liberar está em
[Instalação](installation.html#1-baixar-o-binario-da-release), junto com o motivo
de a assinatura não existir.

## A versão aparece como `dev`

**Causa.** O binário foi compilado localmente ou instalado com `go install`, e a
versão só é injetada no artefato da release.

**Solução.** Para um número de versão de verdade, baixe o arquivo da release.

> **Atenção** A versão viaja dentro do documento de resultado, e
> `braunrate compare` sai sem veredito quando as duas execuções usaram versões
> diferentes.

## O modo servidor responde 409

**Causa.** Já existe uma execução em andamento.

**Solução.** Espere terminar. Duas execuções na mesma máquina disputam a CPU que
precisa despachar no instante agendado, e nenhuma das duas mede o que se propôs a
medir. A resposta diz como aceitar a contaminação, se for esse o caso.
