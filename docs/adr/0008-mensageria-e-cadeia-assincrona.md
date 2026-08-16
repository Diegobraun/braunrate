# ADR 0008 — Mensageria: a entrega confirmada é a unidade, e a cadeia se fecha com `aguardar`

- **Status**: aceito
- **Data**: 2026-08-16
- **Contexto de decisao**: Fase 5
- **Relacionados**: [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [ADR 0006](0006-graphql-como-unidade-de-medida.md), [ADR 0007](0007-variedade-observada.md)

## Contexto

Medir mensageria com a cabeca de HTTP produz numero bonito e inutil:

1. **Publicar sem esperar confirmacao mede a rede local.** Um `send` assincrono volta em microssegundos porque escreveu num buffer. O broker pode nem ter aceitado a mensagem.
2. **Agrupar mensagens em lote mede o lote, nao a mensagem.** A vazao sobe, a latencia reportada vira a do lote, e a relacao com o instante agendado se perde.
3. **Produzir sem consumir nao mede nada do que o usuario sente.** Em arquitetura assincrona, o que importa e quanto tempo leva do pedido ate o efeito — e o efeito acontece do outro lado da fila.
4. **Chave de particao fixa concentra a carga.** Todas as mensagens caem na mesma particao, um broker trabalha, os outros ficam parados, e o resultado parece bom.

## Decisao

**A unidade de medida e a entrega confirmada pelo broker; a cadeia inteira se mede com um passo `aguardar`.**

1. **Confirmacao e o padrao.** Kafka usa `acks: todos`; AMQP usa `publisher confirms` e espera o `ack`. Desligar e possivel (`acks: nenhum`, `confirmar: false`) e fica declarado no relatorio como escolha de quem escreveu.
2. **Uma mensagem por chegada agendada, sem lote.** `BatchSize: 1` no produtor. Agrupar melhoraria a vazao do gerador e destruiria a relacao entre o instante agendado e a mensagem — que e a base de toda a medicao honesta do projeto ([ADR 0003](0003-modelo-de-execucao-e-metrica.md)).
3. **A chave de agregacao e o destino de negocio**: `kafka produzir <topico>`, `amqp publicar <troca>/<rota>`, `aguardar <topico>`. Nunca o broker, nunca a conexao.
4. **O passo `aguardar` fecha a cadeia.** Ele espera a mensagem **daquela iteracao**, identificada por um valor de correlacao obrigatorio. Sem correlacao a medicao pegaria a primeira mensagem que aparecesse — mediria o consumidor mais rapido do topico, e nao a jornada. Por isso a correlacao e obrigatoria, com mensagem de erro explicando o motivo.
5. **A assinatura abre antes da carga, sem grupo de consumo.** O offset de cada particao e lido no momento da abertura e a leitura segue dali. Grupo de consumo negocia particao com o broker no primeiro `poll`, e a mensagem produzida durante a negociacao se perde: o timeout resultante seria do braunrate, nao do servico. Foi um defeito real, encontrado quando os testes rodaram contra o Kafka oficial em vez de so contra o Redpanda.
6. **Mensagem que chega antes da espera nao se perde.** A assinatura guarda chegadas recentes; sem isso, a resposta rapida perderia a corrida contra o registro da espera e viraria timeout — acusando lentidao que nao existe.
7. **Concentracao de particao e resultado invalido**, pela regra do [ADR 0007](0007-variedade-observada.md): se o topico tem N particoes e a execucao usou 1, o resultado nao representa producao.
8. **Quando o efeito so aparece por API, a espera e por sondagem, com a granularidade declarada.** `aguardar: { http: ..., ate: ... }` repete a consulta ate a condicao valer. Sondagem mede em degraus do intervalo, entao o numero e sempre maior ou igual ao real — e por isso o relatorio imprime o intervalo usado em vez de deixar o degrau passar por latencia do alvo. Sem `ate` o passo e recusado: a primeira resposta encerraria a espera e mediria o tempo de responder, nao o tempo ate o efeito.
9. **Timeout do `aguardar` e erro, com explicacao**: diz qual valor era esperado, onde, e por quanto tempo se esperou.

## Alternativas descartadas

- **Medir so a producao**: e o que a maioria das ferramentas faz. Mede o broker aceitando bytes, nao o sistema entregando efeito.
- **Consumir com grupo e `auto.offset.reset=latest`**: parecia natural e produziu perda de mensagem por corrida de rebalance — virou o item 5 acima.
- **Esperar "qualquer mensagem nova"**: mediria o consumidor mais rapido e ficaria otimista sob carga, que e quando o resultado importa.
- **Lote com `linger.ms`**: melhora vazao do gerador e quebra a relacao com o instante agendado.

## Consequencias

- O relatorio ganha linhas por topico e por fila, e a jornada passa a incluir producao mais espera — a leitura honesta da cadeia assincrona.
- A vazao maxima do gerador cai em relacao a uma ferramenta que usa lote. E o preco declarado de medir mensagem a mensagem.
- Testes de mensageria exigem broker de verdade: rodam contra Kafka e RabbitMQ no CI, e **pulam declarando** quando nao ha broker, em vez de passar sem medir nada.
- Fica pendente: **lag do consumidor** como metrica propria, **particao explicita** por passo, **Avro/Schema Registry** (ja registrado como ponto fraco no [ADR 0004](0004-extensao-de-protocolo.md)), e transacoes.
