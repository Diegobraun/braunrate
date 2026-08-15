# Medicao dos prototipos — Fase 0

Comparacao entre dois prototipos descartaveis que resolvem exatamente o mesmo problema minimo, para decidir linguagem e runtime com numero em vez de preferencia. Base conceitual: [estudo de ferramentas](estudo-ferramentas.md), §3.1 e §9.1.

## Ambiente declarado

| Item | Valor |
|---|---|
| Maquina | Apple M2 Pro, 10 nucleos (6 performance + 4 efficiency), 16 GB RAM |
| Sistema | macOS 26.5.1 (build 25F80), arm64 |
| Java | GraalVM CE 25.2.4+7.1, OpenJDK 25.0.4, JIT (`mixed mode, sharing`) |
| Go | go1.26.6 darwin/arm64 |
| HdrHistogram | Java `org.hdrhistogram:HdrHistogram:2.2.2`, Go `github.com/HdrHistogram/hdrhistogram-go v1.3.0` |
| Alvo | servidor HTTP proprio em Go, mesmo binario para os dois prototipos, loopback `127.0.0.1` |
| Isolamento | nenhum: gerador e alvo dividem os mesmos 10 nucleos; sem cgroup, sem afinidade de CPU |
| Repeticoes | 3 por ponto (5 no experimento de startup) |

Margem reportada e a **semi-amplitude** entre repeticoes (`(max - min) / 2`), nao desvio padrao — com n=3 o desvio padrao seria pior estimativa do que o proprio intervalo observado.

## O que os prototipos fazem

Identico nos dois, ~200 linhas cada:

1. Modelo de chegada **aberto**: o instante de cada requisicao `i` e `t0 + i / taxa`, calculado antes de comecar e independente do que o alvo faz.
2. Espera hibrida ate o instante agendado: `park`/`sleep` ate 1,5 ms antes, depois espera ativa. Sem a espera ativa o desvio fica na casa de milissegundos nos dois runtimes.
3. Uma unidade de concorrencia por requisicao (virtual thread no Java, goroutine no Go) executando codigo **linear e bloqueante**.
4. Tres HDR histograms: **latencia corrigida** (do instante agendado ate a resposta), **latencia de servico** (do envio ate a resposta) e **desvio de agendamento** (despacho menos agendado).
5. Contagem de despachos atrasados acima de 10 ms — o sinal de back-pressure do gerador.

A diferenca entre *latencia corrigida* e *latencia de servico* no mesmo run e a medida direta da omissao coordenada: e o quanto uma ferramenta de laco fechado deixaria de reportar.

## Criterio de "taxa maxima sustentada"

Um ponto conta como sustentado quando as tres condicoes valem:

- `taxa_efetiva >= 99% da taxa_alvo`
- `despachos_atrasados / medidas < 1%`
- `desvio_de_agendamento p99 < 10 ms`

## O que este experimento NAO mede

- **Nao mede o alvo.** O servidor de teste roda na mesma maquina e disputa os mesmos nucleos; parte da cauda de latencia observada e contencao local, nao capacidade do gerador. Os numeros de taxa maxima valem como *comparacao entre os dois prototipos*, nao como capacidade absoluta do braunrate.
- **Nao mede rede real.** Loopback nao tem perda, reordenacao, MTU nem handshake TLS. Cenario com TLS muda o perfil de CPU dos dois lados.
- **Nao mede o melhor cliente HTTP possivel de cada ecossistema.** Usa o cliente padrao (`java.net.http.HttpClient` e `net/http`). Existem clientes mais rapidos nos dois lados (Netty/Vert.x, `fasthttp`); a escolha aqui reflete o que um time usaria sem otimizar.
- **Nao mede startup em imagem nativa.** `native-image` nao esta instalado nesta maquina; o custo de GraalVM AOT entra no ADR 0001 como custo de ferramenta, sem numero proprio.
- **Nao mede execucao longa.** Janelas de 15 s nao expoem fragmentacao de heap, vazamento de conexao nem o comportamento do GC em teste de 1 hora.
- **Nao mede Kafka nem AMQP.** A ergonomia e a maturidade dos clientes desses protocolos pesam no ADR 0001 por avaliacao qualitativa, nao por medicao.
- **Nao mede maquina ociosa.** Havia sessao de desenvolvimento aberta na mesma maquina; picos isolados na cauda podem vir dai. E por isso que a comparacao usa 3 repeticoes e reporta a amplitude.

## Reproducao

```bash
cd prototipos/alvo && go build -o alvo .
cd ../go && go build -o braunrate-proto-go .
cd ../java && javac -cp lib/HdrHistogram-2.2.2.jar -d classes PrototipoJava.java

python3 medicoes/medir.py startup
python3 medicoes/medir.py taxa
python3 medicoes/medir.py concorrencia
```

## Resultados

_(preenchido apos a bateria — ver secoes abaixo)_
