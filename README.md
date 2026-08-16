# braunrate

Ferramenta de teste de carga com medição honesta: modelo de chegada aberto, HDR
histogram e detecção de back-pressure.

**Documentação: <https://diegobraun.github.io/braunrate/>** — o site é gerado de
`docs/guias/` deste repositório, e todo bloco de código dele passa pela suíte de
testes.

## Ver funcionando

```bash
braunrate demo              # sobe um alvo, roda um cenario e explica cada numero
braunrate demo --com-falha  # o mesmo, contra um alvo que trava no meio
```

Um comando, um terminal, nenhum arquivo antes. `braunrate` sem argumento diz qual
é o próximo comando; `braunrate ajuda` lista todos.

## A medição, em uma comparação

O alvo de teste embutido congela por 1 s no meio da execução. Mesma pausa, mesmo
alvo, dois modelos de medição:

| Modelo | 99% das respostas em até | Amostras |
|---|---|---|
| **braunrate (chegada aberta, tempo contado do instante agendado)** | **983,0 ms** | 600 |
| Laço fechado (um usuário virtual em sequência, como JMeter e Locust medem) | 3,7 ms | 722 |

São 979,4 ms escondidos pelo laço fechado. Ele não erra por defeito: quando o
alvo trava, simplesmente para de enviar, e as requisições que deveriam ter
partido nunca entram na conta. Essa é a omissão coordenada.

A comparação é um teste automatizado que roda no CI a cada push. Se a medição
deixar de ser honesta, o build quebra.

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 983.0 ms sobre 600 amostras
      laço fechado:              p99 3.7 ms sobre 722 amostras
      omissão coordenada: 979.4 ms escondidos pelo laço fechado
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

São três provas, cada uma expondo um ponto cego diferente: a travada acima; o
GraphQL que responde erro com status 200 (406 erros em 2.844 respostas, todas
200); e a cadeia assíncrona que custa 1,2 ms para produzir e 3,96 s ponta a
ponta. As três estão no [início do site](https://diegobraun.github.io/braunrate/),
com a saída real de cada uma.

## Instalação

Baixe o binário da [release](https://github.com/Diegobraun/braunrate/releases),
confira o checksum e rode. Não há runtime para instalar.

```bash
go install github.com/Diegobraun/braunrate/cmd/braunrate@latest   # se voce ja tem Go
```

Os três caminhos, os avisos de primeira execução no macOS e no Windows, a tabela
de plataformas e o que fica de fora estão em
[Instalação](https://diegobraun.github.io/braunrate/instalacao.html).

## Estado

**Fase 8 concluída.** Motor de chegada aberta, HTTP, GraphQL, Kafka, RabbitMQ e
passo `aguardar`, correlação, autenticação, dados, asserções, critério de aceite
com código de saída, ferramentas de autoria (schema no editor, `debug`,
`import curl`, `import jmx` e `record`), relatório (HTML autocontido, JSON, CSV,
comparação entre execuções), variedade observada, cenário em Go equivalente ao
YAML travado por teste, executável de um módulo de fora, modelo fechado
declarado, autenticação de broker com a credencial fora do arquivo e modo
servidor local sem lógica própria.

A decisão da Fase 0 foi **Go**, sustentada por dois critérios apenas: RSS sob
carga (30 MB contra 597 MB do Java com G1, a 10.000/s) e binário único estático,
que para o público de QA significa instalar baixando um arquivo. Números,
metodologia e limites em [medicoes-fase0.md](docs/medicoes-fase0.md); a decisão
com os pesos de cada critério em
[ADR 0001](docs/adr/0001-linguagem-e-runtime.md).

## Desenvolvimento

```bash
go build -o braunrate ./cmd/braunrate
go test ./...
go run ./cmd/site -out site      # gera a documentacao publicada
```

O conteúdo do site fica em [`docs/guias/`](docs/guias); a referência do cenário e
o índice de decisões são gerados do schema e dos ADRs. Editar a documentação é
editar esses arquivos, e o teste reprova o build se um bloco de código publicado
deixar de valer.

## Documentação no repositório

- [Princípios de produto](docs/principios-de-produto.md) — critério de aceitação de toda decisão de interface
- [Vocabulário](docs/vocabulario.md) — uma palavra por conceito, em todo texto ao usuário
- [Decisões de experiência de uso](docs/decisoes-experiencia.md)
- [Relatório de experiência de uso](docs/relatorio-experiencia.md)
- [Roteiro](docs/roteiro.md)
- [Arquitetura](docs/arquitetura.md)
- [Estudo comparativo de ferramentas](docs/estudo-ferramentas.md) — base de todas as decisões
- [ADRs](docs/adr) — 17 decisões, cada uma com o que foi recusado e o critério que reabre
- [API do modo servidor](docs/api-servidor.md) — um exemplo de curl por rota
- [Schema do cenário](docs/braunrate.schema.json) — autocompletar e validação no editor
- [Exemplo de relatório HTML](docs/exemplo-relatorio.html) — saída real de uma execução que falhou o critério de aceite
- [Bateria adversarial](docs/bateria-adversarial.md) — onde a ferramenta falha, mente ou frustra
- [Auditoria de fricção](docs/auditoria-fricao.md) — o que a ferramenta exige e não fornece
- [Medição dos protótipos da Fase 0](docs/medicoes-fase0.md)

## Licença

MIT — Diego Braun.
