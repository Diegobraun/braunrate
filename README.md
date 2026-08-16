# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR histogram e deteccao de back-pressure.

**Documentacao: <https://diegobraun.github.io/braunrate/>** — o site e gerado de `docs/guias/` deste repositorio, e todo bloco de codigo dele passa pela suite de testes.

## Ver funcionando

```bash
braunrate demo              # sobe um alvo, roda um cenario e explica cada numero
braunrate demo --com-falha  # o mesmo, contra um alvo que trava no meio
```

Um comando, um terminal, nenhum arquivo antes. `braunrate` sem argumento diz qual e o proximo comando; `braunrate ajuda` lista todos.

## Demonstracao de honestidade de medicao

O alvo de teste embutido congela por 1 s no meio da execucao. Mesma pausa, mesmo alvo, dois modelos de medicao:

| Modelo | 99% das respostas em ate | Amostras |
|---|---|---|
| **braunrate (chegada aberta, tempo contado do instante agendado)** | **976,4 ms** | 600 |
| Laco fechado (um usuario virtual em sequencia, como JMeter e Locust medem) | 3,3 ms | 793 |

**973,1 ms escondidos pelo laco fechado.** O laco fechado nao mente por bug: quando o alvo trava, ele simplesmente para de enviar, e as requisicoes que deveriam ter partido nunca entram na conta. E a omissao coordenada.

Isso nao e alegacao de marketing: e um teste automatizado que roda no CI a cada push. Se a medicao mentir, o build quebra.

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 976.4 ms sobre 600 amostras
      laco fechado:              p99 3.3 ms sobre 793 amostras
      omissao coordenada: 973.1 ms escondidos pelo laco fechado
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

Sao **tres provas**, cada uma expondo um ponto cego diferente: a travada acima; o GraphQL que responde erro com status 200 (406 erros em 2.844 respostas, todas 200); e a cadeia assincrona que custa 1,2 ms para produzir e 3,96 s ponta a ponta. As tres estao no [inicio do site](https://diegobraun.github.io/braunrate/), com a saida real de cada uma.

## Instalacao

Baixe o binario da [release](https://github.com/Diegobraun/braunrate/releases), confira o checksum e rode. Nao ha runtime para instalar.

```bash
go install github.com/Diegobraun/braunrate/cmd/braunrate@latest   # se voce ja tem Go
```

Os tres caminhos, os avisos de primeira execucao no macOS e no Windows, a tabela de plataformas e o que fica de fora estao em [Instalacao](https://diegobraun.github.io/braunrate/instalacao.html).

## Estado

**Fase 8 concluida** — motor de chegada aberta, HTTP, GraphQL, Kafka, RabbitMQ e passo `aguardar`, correlacao, autenticacao, dados, assercoes, criterio de aceite com codigo de saida, ferramentas de autoria (schema no editor, `debug`, `import curl`, `import jmx` e `record`), relatorio (HTML autocontido, JSON, CSV, comparacao entre execucoes), variedade observada, cenario em Go equivalente ao YAML travado por teste, executavel de um modulo de fora, modelo fechado declarado, autenticacao de broker com a credencial fora do arquivo e modo servidor local sem logica propria.

Decisao da Fase 0: **Go**, sustentada por dois criterios apenas — RSS sob carga (30 MB contra 597 MB do Java com G1, a 10.000/s) e binario unico estatico, que para o publico de QA significa instalar baixando um arquivo. Numeros, metodologia e limites em [medicoes-fase0.md](docs/medicoes-fase0.md); a decisao com os pesos de cada criterio em [ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Desenvolvimento

```bash
go build -o braunrate ./cmd/braunrate
go test ./...
go run ./cmd/site -out site      # gera a documentacao publicada
```

O conteudo do site fica em [`docs/guias/`](docs/guias); a referencia do cenario e o indice de decisoes sao gerados do schema e dos ADRs. Editar a documentacao e editar esses arquivos, e o teste reprova o build se um bloco de codigo publicado deixar de valer.

## Documentacao no repositorio

- [Principios de produto](docs/principios-de-produto.md) — criterio de aceitacao de toda decisao de interface
- [Vocabulario](docs/vocabulario.md) — uma palavra por conceito, em todo texto ao usuario
- [Decisoes de experiencia de uso](docs/decisoes-experiencia.md)
- [Relatorio de experiencia de uso](docs/relatorio-experiencia.md)
- [Roteiro](docs/roteiro.md)
- [Arquitetura](docs/arquitetura.md)
- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — base de todas as decisoes
- [ADRs](docs/adr) — 17 decisoes, cada uma com o que foi recusado e o criterio que reabre
- [API do modo servidor](docs/api-servidor.md) — um exemplo de curl por rota
- [Schema do cenario](docs/braunrate.schema.json) — autocompletar e validacao no editor
- [Exemplo de relatorio HTML](docs/exemplo-relatorio.html) — saida real de uma execucao que falhou o criterio de aceite
- [Bateria adversarial](docs/bateria-adversarial.md) — onde a ferramenta falha, mente ou frustra
- [Auditoria de friccao](docs/auditoria-fricao.md) — o que a ferramenta exige e nao fornece
- [Medicao dos prototipos da Fase 0](docs/medicoes-fase0.md)

## Licenca

MIT — Diego Braun.
