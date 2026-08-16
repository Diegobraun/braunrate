# API do modo servidor

`braunrate serve` expoe por HTTP **o que a CLI ja faz, e nada alem disso**. Toda rota termina em `internal/runner`, o mesmo caminho que o terminal usa — e um teste reprova o build se os dois deixarem de produzir o mesmo documento.

```bash
braunrate serve -addr 127.0.0.1:8080 -dir ./cenarios
```

```
braunrate serve em http://127.0.0.1:8080, servindo cenarios de ./cenarios
Sem autenticacao e sem TLS: qualquer um que alcance esta porta pode disparar carga contra os alvos dos cenarios.
Foi feito para rodar em 127.0.0.1. Expor em outra interface e outra decisao, e ela ainda nao foi tomada.
```

Opcoes: `-addr` (padrao `127.0.0.1:8080`), `-dir` (padrao `.`), `-concurrent` (padrao desligado).

**Nome de rota e campo de JSON em ingles; mensagem em portugues** — a mesma divisao do resto do codigo ([ADR 0010](adr/0010-idioma-do-codigo.md)).

## O que o servidor nao e

- **Nao guarda nada em banco.** O arquivo YAML no `-dir` e a verdade; as execucoes vivem na memoria do processo e somem quando ele reinicia. Quem quiser guardar o resultado busca o JSON e grava onde quiser.
- **Nao tem conta, sessao nem autenticacao.** Foi feito para `127.0.0.1`.
- **Nao tem interface grafica, agendamento nem multiusuario.** Estao fora de escopo, nao "para depois".
- **Nao muda o veredito.** Aviso de saturacao e invalidacao de resultado valem igual: um resultado que a CLI marcaria como invalido chega aqui invalido, com o mesmo codigo de saida no campo `exit_code`.

---

## `GET /health`

```bash
curl -s http://127.0.0.1:8080/health
```

```json
{ "tool": "braunrate", "version": "0.4.0" }
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
    { "name": "http-basico.yaml", "path": "cenarios/http-basico.yaml" }
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
    "Cenario valido: \"Fumaca de CI\", 1 passo(s), 975 iteracoes em 6s."
  ]
}
```

As linhas sao exatamente as que `braunrate validate` imprime — aviso de modelo fechado, ausencia de SLO, broker de mensageria e dependencia de infraestrutura inclusive.

Cenario recusado responde `422`, com a mensagem identica a do terminal e a posicao em campos proprios, que e o que um editor precisa:

```json
{
  "valid": false,
  "message": "erro no cenario: quebrado.yaml:7:16: nao sei de onde vem ${nao_declarada}.\n    declare de onde ela vem:\n      ...",
  "file": "quebrado.yaml",
  "line": 7,
  "column": 16
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
  "text": "\npasso 1 — consultar pedido   [ok em 6.5ms]\n  requisicao: GET /pedidos/1\n              Authorization: Bearer token-… (14 caracteres)\n  resposta:   status 200, 89 bytes\n  corpo:      {\"id\":\"1\",\"status\":\"ABERTO\",...}\n",
  "vars": { "token": "token-de-teste" },
  "observations": [
    { "step": "consultar pedido", "class": "sucesso", "status": 200, "duration": "6.5ms" }
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
  "message": "ja existe uma execucao em andamento. Duas execucoes na mesma maquina disputam a CPU que precisa despachar no instante agendado, e nenhuma das duas mede o que se propos a medir. Espere a atual terminar, ou suba o servidor com -concurrent se a contaminacao for aceitavel neste caso."
}
```

Subir com `-concurrent` aceita a contaminacao, e o aviso de partida passa a dizer isso.

## `GET /runs/{id}/stream`

Texto puro, uma linha por atualizacao — a mesma linha que o terminal imprime. Quem conecta atrasado recebe primeiro o que ja passou, e so depois o que vem.

```bash
curl -sN http://127.0.0.1:8080/runs/r001/stream
```

```
executando "Fumaca de CI" contra http://127.0.0.1:8080: 975 iteracoes em 6s
carga 150/s | enviadas 201 | concluidas 200 | erros 0 | metade em 6.7 ms | 99% em 7.7 ms | faltam 4s
carga 200/s | enviadas 376 | concluidas 375 | erros 0 | metade em 6.4 ms | 99% em 8.4 ms | faltam 3s
carga 200/s | enviadas 576 | concluidas 575 | erros 0 | metade em 5.8 ms | 99% em 7.6 ms | faltam 2s
carga 200/s | enviadas 776 | concluidas 775 | erros 0 | metade em 5.6 ms | 99% em 7.5 ms | faltam 1s
carga 0/s | enviadas 975 | concluidas 974 | erros 0 | metade em 5.5 ms | 99% em 7.4 ms | faltam 0s
passou (codigo 0)
```

A ultima linha e sempre o veredito com o codigo de saida — `passou`, `falhou o SLO` ou `resultado invalido`.

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
      "name": "Fumaca de CI",
      "status": "done",
      "exit_code": 0,
      "verdict": "passou",
      "started_at": "2026-08-16T06:20:55.258965+01:00",
      "summary": { "errors": 0, "requests": 975, "valid": true }
    }
  ]
}
```

`verdict` e `exit_code` dizem a mesma coisa em dois formatos: um para ler, outro para ramificar. Os codigos sao os da CLI — `0` passou, `1` falhou o SLO, `2` erro de cenario, `3` resultado invalido.

## `GET /runs/{id}`

O documento de resultado, byte a byte o mesmo que `-result` grava.

```bash
curl -s http://127.0.0.1:8080/runs/r001 | jq '.execucao.cenario, .veredito_slo.passou'
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
  "antes": { "cenario": "Fumaca de CI", "alvo": "http://127.0.0.1:8080", "inicio": "16/08/2026 06:20" },
  "depois": { "cenario": "Fumaca de CI", "alvo": "http://127.0.0.1:8080", "inicio": "16/08/2026 06:21" },
  "frase": "Sem mudanca que valha leitura: jornada inteira (95%): 7 ms contra 7 ms — diferenca dentro do ruido de duas execucoes. Com 1 ressalva(s) que podem explicar a diferenca sozinhas.",
  "comparavel": true,
  "ressalvas": [
    {
      "texto": "as duas execucoes usaram um token para tudo; cache ou sharding por identidade afeta as duas do mesmo jeito, mas nao some da comparacao",
      "impede_comparacao": false
    }
  ]
}
```

As ressalvas sao as mesmas do terminal. Comparar modelo aberto com modelo fechado sai com `impede_comparacao: true`:

```json
{
  "texto": "os modelos de chegada sao diferentes: aberto e fechado. Latencia de laco fechado nao se compara com latencia contada do instante agendado — a segunda inclui um atraso que a primeira nao chega a registrar",
  "impede_comparacao": true
}
```

## `GET /runs/{before}/comparison/{after}/report`

A mesma comparacao como pagina, para anexar na revisao de codigo:

```bash
curl -s http://127.0.0.1:8080/runs/r003/comparison/r004/report -o comparacao.html
```

```
200 text/html; charset=utf-8 7116 bytes
<h1 class="passou">Ficou mais rapido: jornada inteira (95%): 8% mais rapido — de 9 ms para 8 ms.
```

Mesmo veredito e mesmas ressalvas do JSON acima e do terminal — a pagina e uma projecao da mesma comparacao, nao um segundo calculo. Vale a mesma regra do `409`: as duas execucoes precisam ter terminado.

O campo `comparavel` responde outra pergunta: ele fica `false` quando **uma das execucoes tem resultado invalido**, e ai nao ha numero nenhum para comparar. Ressalva que impede e o veredito sobre a leitura; `comparavel` e o veredito sobre os dados.

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
