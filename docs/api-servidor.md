# API do modo servidor

`braunrate serve` expoe por HTTP **o que a CLI ja faz, e nada alem disso**. Toda rota termina em `internal/runner`, o mesmo caminho que o terminal usa — e um teste reprova o build se os dois deixarem de produzir o mesmo documento.

```bash
braunrate serve -addr 127.0.0.1:8080 -dir ./cenarios
```

```
braunrate serve at http://127.0.0.1:8080, serving scenarios from ./cenarios
No authentication and no TLS: anyone who reaches this port can fire load at the targets of the scenarios.
It was made to run on 127.0.0.1. Exposing it on another interface is a separate decision, and it has not been made.

To see what it is serving:
  curl http://127.0.0.1:8080/scenarios
```

Opcoes: `-addr` (padrao `127.0.0.1:8080`), `-dir` (padrao `.`), `-concurrent` (padrao desligado).

**Rota, campo de JSON e mensagem em ingles** — desde o [ADR 0019](adr/0019-formato-em-ingles.md), tudo o que o usuario le esta em ingles; este documento continua em portugues porque o leitor dele e quem mantem a ferramenta.

## O que o servidor nao e

- **Nao guarda nada em banco.** O arquivo YAML no `-dir` e a verdade; as execucoes vivem na memoria do processo e somem quando ele reinicia. Quem quiser guardar o resultado busca o JSON e grava onde quiser.
- **Nao tem conta, sessao nem autenticacao.** Foi feito para `127.0.0.1`.
- **Nao tem interface grafica, agendamento nem multiusuario.** Estao fora de escopo, nao "para depois".
- **Nao muda o veredito.** Aviso de saturacao e invalidacao de resultado valem igual: um resultado que a CLI marcaria como invalido chega aqui invalido, com o mesmo codigo de saida no campo `exitCode`.

---

## `GET /health`

```bash
curl -s http://127.0.0.1:8080/health
```

```json
{ "tool": "braunrate", "version": "0.6.0", "directory": "./cenarios", "writable": false }
```

## `GET /scenarios`

Lista os `.yaml` e `.yml` do diretorio servido.

```bash
curl -s http://127.0.0.1:8080/scenarios
```

```json
{
  "scenarios": [
    { "name": "ci.yaml", "path": "cenarios/ci.yaml" },
    { "name": "http-basic.yaml", "path": "cenarios/http-basic.yaml" }
  ]
}
```

## `POST /scenarios/{name}/validate`

O `{name}` e o nome do arquivo dentro do `-dir`, nunca um caminho: qualquer coisa com barra ou comecando com ponto responde `400`.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/validate
```

```json
{
  "valid": true,
  "lines": [
    "Valid scenario: \"CI smoke\", 1 step, 975 iterations in 6s.",
    "Warning: the step \"look up order\" has no value that varies — every request will be identical.\n    if the target caches by that key, the number comes out optimistic.\n    to make it vary:  data: { orders: { file: orders.csv } }  and then  GET /orders/${orders.id}"
  ]
}
```

As linhas sao exatamente as que `braunrate validate` imprime — aviso de modelo fechado, ausencia de SLO, broker de mensageria e dependencia de infraestrutura inclusive.

Cenario recusado responde `422`, com a mensagem identica a do terminal e a posicao em campos proprios, que e o que um editor precisa:

```json
{
  "valid": false,
  "message": "error in the scenario: broken.yaml:7:23: I do not know where ${undeclared} comes from.\n    declare where it comes from:\n      ...",
  "file": "broken.yaml",
  "line": 7,
  "column": 23
}
```

## `POST /scenarios/{name}/debug`

Uma iteracao, um usuario, sem carga.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/debug
```

```json
{
  "complete": true,
  "text": "\nstep 1 — look up order   [ok in 6.4ms]\n  request:    GET /orders/1\n              Authorization: Bearer test-t… (10 characters)\n  response:   status 200, 85 bytes\n  body:       {\"id\":\"1\",\"status\":\"OPEN\",...}\n",
  "vars": { "token": "test-token" },
  "observations": [
    { "step": "look up order", "class": "success", "status": 200, "duration": "6.4ms" }
  ]
}
```

O campo `text` e a saida do terminal, palavra por palavra, para quem so quer mostrar na tela. Os `observations` sao os mesmos dados em forma de estrutura, para quem vai ler por programa.

## `POST /scenarios/{name}/runs`

Comeca uma execucao e responde `202` na hora. A requisicao nao espera: um teste de carga dura minutos, e um cliente que desiste deixaria a execucao sem dono.

```bash
curl -s -X POST http://127.0.0.1:8080/scenarios/ci.yaml/runs
```

```json
{ "id": "r001", "status": "running", "stream": "/runs/r001/stream" }
```

Uma segunda execucao enquanto a primeira roda responde `409`:

```json
{
  "message": "there is already a run in progress. Two runs on the same machine fight over the CPU that has to dispatch at the scheduled instant, and neither one measures what it set out to measure. Wait for the current one to finish, or start the server with -concurrent if the contamination is acceptable in this case."
}
```

Subir com `-concurrent` aceita a contaminacao, e o aviso de partida passa a dizer isso.

## `GET /runs/{id}/stream`

Texto puro, uma linha por atualizacao — a mesma linha que o terminal imprime. Quem conecta atrasado recebe primeiro o que ja passou, e so depois o que vem.

```bash
curl -sN http://127.0.0.1:8080/runs/r001/stream
```

```
running "CI smoke" against http://127.0.0.1:8080: 975 iterations in 6s
load 100/s | sent 76 | completed 75 | errors 0 | half within 7.0 ms | 99% within 10.0 ms | 5s left
load 150/s | sent 201 | completed 200 | errors 0 | half within 6.7 ms | 99% within 8.5 ms | 4s left
load 200/s | sent 376 | completed 375 | errors 0 | half within 6.4 ms | 99% within 8.1 ms | 3s left
load 200/s | sent 576 | completed 575 | errors 0 | half within 5.9 ms | 99% within 7.7 ms | 2s left
load 200/s | sent 776 | completed 775 | errors 0 | half within 5.7 ms | 99% within 7.8 ms | 1s left
load 0/s | sent 975 | completed 974 | errors 0 | half within 5.7 ms | 99% within 7.7 ms | 0s left
passed (code 0)
```

A ultima linha e sempre o veredito com o codigo de saida — `passed`, `failed the SLO` ou `invalid result`.

## `GET /runs`

```bash
curl -s http://127.0.0.1:8080/runs
```

```json
{
  "runs": [
    {
      "id": "r001",
      "scenario": "ci.yaml",
      "name": "CI smoke",
      "status": "done",
      "exitCode": 0,
      "verdict": "passed",
      "startedAt": "2026-08-17T01:33:52.159627+01:00",
      "summary": { "errors": 0, "requests": 975, "valid": true }
    }
  ]
}
```

`verdict` e `exitCode` dizem a mesma coisa em dois formatos: um para ler, outro para ramificar. Os codigos sao os da CLI — `0` passou, `1` falhou o SLO, `2` erro de cenario, `3` resultado invalido.

## `GET /runs/{id}`

O documento de resultado, byte a byte o mesmo que `-result` grava.

```bash
curl -s http://127.0.0.1:8080/runs/r001 | jq '.run.scenario, .slo.passed'
```

Enquanto a execucao roda, responde `200` dizendo que ainda esta em andamento e apontando o stream. Execucao que nao existe responde `404` dizendo onde as execucoes vivem.

## `GET /runs/{id}/report`

O HTML autocontido, o mesmo que `-html` grava.

```bash
curl -s http://127.0.0.1:8080/runs/r001/report -o relatorio.html
```

Antes de a execucao terminar, responde `409` — nao existe relatorio de algo que ainda esta acontecendo.

## `GET /runs/{before}/comparison/{after}`

```bash
curl -s http://127.0.0.1:8080/runs/r001/comparison/r002
```

```json
{
  "before": { "scenario": "CI smoke", "target": "http://127.0.0.1:8080", "start": "2026-08-17 01:33", "version": "0.6.0" },
  "after": { "scenario": "CI smoke", "target": "http://127.0.0.1:8080", "start": "2026-08-17 01:35", "version": "0.6.0" },
  "sentence": "No change worth reading: the whole journey (95%): 7 ms against 7 ms — a difference within the noise of two runs. With 1 caveat about what changed outside the service.",
  "comparable": true,
  "caveats": [
    {
      "text": "both runs used one token for everything; caching or sharding by identity affects them the same way, but it does not disappear from the comparison",
      "blocksComparison": false
    }
  ]
}
```

As ressalvas sao as mesmas do terminal. Comparar modelo aberto com modelo fechado sai com `blocksComparison: true`:

```json
{
  "text": "the arrival models are different: open and closed. Closed-loop latency does not compare with latency counted from the scheduled instant — the second includes a delay the first never records",
  "blocksComparison": true
}
```

## `GET /runs/{before}/comparison/{after}/report`

A mesma comparacao como pagina, para anexar na revisao de codigo:

```bash
curl -s http://127.0.0.1:8080/runs/r003/comparison/r004/report -o comparacao.html
```

```
200 text/html; charset=utf-8 7252 bytes
<h1 class="neutral">No change worth reading: the whole journey (95%): 7 ms against 7 ms — a difference within the noise of two runs. With 1 caveat about what changed outside the service.
```

Mesmo veredito e mesmas ressalvas do JSON acima e do terminal — a pagina e uma projecao da mesma comparacao, nao um segundo calculo. Vale a mesma regra do `409`: as duas execucoes precisam ter terminado.

O campo `comparable` responde outra pergunta: ele fica `false` quando **uma das execucoes tem resultado invalido**, e ai nao ha numero nenhum para comparar. Ressalva que impede e o veredito sobre a leitura; `comparable` e o veredito sobre os dados.

---

## Codigos de resposta

| Codigo | Quando |
|---|---|
| `200` | pedido atendido |
| `202` | execucao aceita e comecando |
| `400` | nome de cenario com caminho, ou fora do diretorio servido |
| `404` | execucao que nao existe neste processo |
| `409` | execucao concorrente recusada, ou relatorio pedido antes do fim |
| `422` | cenario recusado — a mensagem e a mesma do terminal |
