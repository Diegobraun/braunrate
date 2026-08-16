# ADR 0003 — Modelo de execucao e metrica

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 0
- **Relacionados**: [ADR 0001](0001-linguagem-e-runtime.md), [ADR 0002](0002-modelo-de-cenario.md), [estudo de ferramentas](../estudo-ferramentas.md) §3.1, §3.8, §5, §6

## Contexto

O estudo (§3.1) identifica o modelo de execucao como o eixo mais importante: modelo fechado produz omissao coordenada, e omissao coordenada faz um teste passar com p99 de dezenas de milissegundos enquanto producao sofre segundos. O estudo tambem exige (§3.8) que agregados sejam mergeaveis desde o inicio, e (§5) que Kafka e AMQP nao sejam espremidos no molde requisicao-resposta.

A medicao da Fase 0 quantificou o problema no proprio prototipo: com o alvo em 5 ms fixos e o gerador sem folga, a diferenca entre latencia contada do instante agendado e latencia contada do instante de envio aparece na cauda e cresce com a taxa. Ver [medicoes-fase0.md](../medicoes-fase0.md).

## Decisao

### 1. Chegada aberta e o agendamento como fonte da verdade

O plano de carga produz, antes da execucao, uma funcao `taxa(t)`. Dela sai a sequencia de **instantes agendados**. O instante agendado de cada requisicao existe antes de qualquer conexao ser aberta e nao depende do alvo.

Consequencias diretas:

- **Latencia = `fim_da_resposta - instante_agendado`.** Sempre. A diferenca `envio - agendado` e registrada em separado como *desvio de agendamento*, nunca subtraida da latencia.
- O gerador nao reduz o ritmo porque o alvo degradou. Se ele nao consegue despachar no instante agendado, isso e uma **falha do gerador** e vira aviso, nao um numero melhor.
- Perfis fechados (N usuarios em laco) sao expressaveis, mas nunca sao o padrao, e o relatorio marca a execucao como fechada — porque o p99 de execucao fechada nao e comparavel com o de execucao aberta.

O agendador usa espera hibrida: dormir ate ~1,5 ms antes do alvo e depois espera ativa. A Fase 0 mediu que so dormir erra na casa de milissegundos nos dois runtimes avaliados.

### 2. Deteccao de back-pressure com causa provavel

Duas condicoes distintas, nunca fundidas numa so:

| Sinal | Significado | Como aparece |
|---|---|---|
| `despacho - agendado` acima do limiar | **gerador saturado** — o proprio braunrate nao acompanhou | aviso em primeiro plano; a execucao inteira e marcada como suspeita |
| latencia do alvo subindo com despacho pontual | **alvo degradado** — o resultado vale | curva de latencia sobre carga no relatorio |

Distinguir os dois e requisito, porque a acao do usuario e oposta: no primeiro caso ele precisa de mais gerador; no segundo ele achou o que procurava.

Quando o gerador satura, o relatorio nao "corrige" nada nem esconde: informa que o resultado nao vale.

### 3. Instrumentacao no motor, nunca no protocolo

Um protocolo implementa apenas `executar(passo, contexto) -> resultado`. Ele nao mede tempo, nao conta erro e nao toca em histograma. O motor:

1. registra o instante agendado;
2. chama o protocolo;
3. classifica o resultado (sucesso, erro de rede, timeout, status, asserção funcional, `errors` de GraphQL, correlacao perdida);
4. grava nos histogramas e contadores.

Isso e o que garante que uma metrica de Kafka e uma de HTTP sejam comparaveis, e e o que impede que um protocolo novo introduza um jeito proprio de contar tempo. Consequencia: adicionar protocolo nao exige tocar em codigo de metrica.

O protocolo **pode** declarar metricas proprias (lag de consumer group, latencia de ack, taxa de correlacao perdida) por meio de um registro tipado, mas quem grava continua sendo o motor.

### 4. Chaves de agregacao

A metrica e agregada por `(nome_do_passo, protocolo, chave_do_protocolo)`, onde `chave_do_protocolo` e:

- HTTP: metodo + rota declarada no cenario, **nunca a URL com os valores interpolados** (senao cada `id` vira uma serie);
- GraphQL: `operationName` — nunca a URL;
- Kafka/AMQP: topico ou exchange + forma (produzir, consumir, request-reply);
- `aguardar`: o nome do passo.

### 5. Agregados mergeaveis

Tudo que o relatorio mostra precisa ser reconstruivel a partir de agregados parciais somaveis:

- **HDR histogram** para latencia (adicao de histogramas e exata, e a precisao da cauda e preservada);
- **contadores** para volume, erros por classe, despachos atrasados;
- **series temporais em bucket de tamanho fixo alinhado ao epoch**, para que buckets de geradores diferentes somem sem interpolacao.

Proibido no formato de resultado: media pre-calculada como fonte de verdade, percentil pre-calculado, amostragem de latencia. Media e derivada de `soma/contagem`; percentil e derivado do histograma.

Isso e o que mantem a execucao distribuida possivel sem reescrita (estudo §3.8): N geradores emitem agregados parciais, um coordenador soma.

#### Conformidade: o desvio registrado em 2026-08-16 e a correcao no mesmo dia

O desvio, auditado ao corrigir o crescimento de memoria da serie temporal: em memoria a regra valia — `Aggregate.Add` sempre somou histogramas HDR e contadores —, mas nenhum histograma era serializado. O documento publicava `p50_ms`, `p95_ms`, `p99_ms`, `max_ms` e `media_ms`, que sao exatamente as duas coisas que esta secao proibe como fonte de verdade. Consequencia: dois documentos nao podiam ser somados, nem os de dois geradores nem os de duas janelas da mesma execucao. A preparacao para distribuir existia so em memoria, que e o lugar errado — distribuir passa pela serializacao.

Corrigido no **formato de resultado 2**: cada `Distribution` carrega o campo `histograma`, o histograma HDR na codificacao V2 comprimida, e os percentis e a media continuam publicados como projecao derivada dele. `metrics.Merge` soma passo, agregado global, jornada e agendamento pelos histogramas, nunca pelos percentis.

Decisoes que acompanham a correcao:

- **Formato 1 continua sendo lido** pelo relatorio e pela comparacao, com os percentis que ele ja tem. So a soma e recusada, e a mensagem diz por que: o que falta no arquivo antigo e de onde os percentis vieram.
- **O que `Merge` nao soma, e nao vai somar:** veredito de SLO e verificacao de sanidade sao lidos da soma, nao somados; variedade observada conta valores distintos, e a uniao de dois conjuntos de distintos e desconhecida; a serie temporal guarda so os dois quantis de cada balde fechado, e quantil nao soma. Produzir qualquer um deles por adicao seria inventar numero.
- **Custo medido:** +24% no exemplo publicado (15,1 KB, 3.672 bytes de histograma), +41% numa execucao de 60s a 300/s (28,3 KB). O tamanho do histograma depende de quantos baldes foram populados, nao da duracao — o custo relativo cai conforme a execucao cresce. Compressao adicional nao se justifica neste tamanho.
- **Nada muda no que o usuario ve.** Terminal, HTML e CSV sao identicos, verificado renderizando o mesmo resultado com o binario anterior e com o novo.

O teste que a secao pedia desde a Fase 0 existe em `internal/metrics/merge_test.go`: dois documentos gravados em arquivo, relidos e somados produzem os mesmos percentis de uma execucao unica equivalente. Sem o histograma serializado ele falha — o p50 da soma dava 600 ms onde a execucao unica da 400 ms.

### 6. Formato de resultado como contrato

A execucao produz um documento de resultado versionado (`versao_do_formato`) contendo: bloco de ambiente, plano de carga aplicado, agregados por chave, histogramas serializados, series temporais, classificacao de erros, avisos de saturacao e veredito de SLO. Relatorio HTML, JSON, CSV, sumario markdown e comparacao entre execucoes sao **projecoes** desse documento — nenhum deles recalcula nada por conta propria.

### 7. Todo custo de preparacao e pago antes de o relogio comecar

O relogio da execucao so comeca depois que todos os protocolos terminaram de se preparar. Preparacao e montagem, nao carga: se ela entrar no relogio, os primeiros instantes agendados nascem no passado e a corrida se declara saturada por um atraso que o gerador nunca causou — a ferramenta invalida a propria medicao.

Isso ja aconteceu duas vezes. Na Fase 5, a assinatura do consumidor do passo `aguardar` levava centenas de milissegundos e foi movida para fora do laco. Na Fase 7, o aperto de mao de TLS e SASL repetiu o problema, agora empurrando o agendamento inteiro. A regra geral existe para nao haver uma terceira:

**Protocolo novo nasce obrigado a declarar o que precisa preparar.** Quem tem custo de abertura — conexao, assinatura, aperto de mao, negociacao de esquema — implementa `Preparable` e paga ali. Quem nao implementa esta afirmando que abrir custa o mesmo que operar, e essa afirmacao passa a ser explicita em vez de acidental.

Consequencia aceita: o tempo de preparacao nao aparece em nenhum percentil. Ele e custo de montagem do teste, e reportar montagem como latencia do alvo seria a mesma mentira que a omissao coordenada, so que na outra direcao.

## Alternativas descartadas

- **Corrigir omissao coordenada por pos-processamento** (estilo `recordValueWithExpectedInterval`): funciona como remendo quando nao se controla o gerador, mas inventa amostras. Aqui controlamos o agendamento, entao medir certo desde a origem e melhor do que estimar depois.
- **Medir dentro de cada protocolo**: mais simples no primeiro protocolo, insustentavel no terceiro; o estudo (§9.3) ja aponta isso.
- **Guardar cada amostra bruta e calcular no fim**: memoria proporcional ao volume e nao mergeavel em rede sem custo alto. HDR resolve com erro limitado e configuravel.

## Consequencias

- O agendador vira componente critico com teste proprio de precisao, e o desvio de agendamento e publicado no relatorio de toda execucao.
- Um protocolo novo nao ganha liberdade de medir do seu jeito — isso e limitacao intencional.
- O documento de resultado precisa de versionamento desde a v1, porque a comparacao entre execucoes le resultados antigos.
