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
| Repeticoes | 3 por ponto no experimento de taxa; 2 nos de concorrencia e GC; 5 no de startup |

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
python3 medicoes/medir.py gc
python3 medicoes/analisar.py

bash medicoes/diagnostico.sh 40000
```

## Resultados

### Startup

| Prototipo | Startup (ms) | Amostras |
|---|---|---|
| java | 587.2 ± 27.8 | 5 |
| go | 42.8 ± 0.9 | 5 |

### Taxa de chegada — sustentacao e precisao do agendamento

| Taxa alvo | Prototipo | Repeticoes sustentadas | Taxa efetiva (reps validas) | Desvio p50 (us) | Desvio p99 (us) | Desvio max (us) | Erros (pior rep) | Pico em voo (pior rep) |
|---|---|---|---|---|---|---|---|---|
| 1.000 /s | java | 3/3 | 1.000.1 ± 0.0 | 1 ± 0 | 3 ± 340 | 820 ± 9.240 | 0 | 47 |
| 1.000 /s | go | 3/3 | 1.000.1 ± 0.0 | 1 ± 0 | 1 ± 0 | 3.293 ± 1.736 | 0 | 22 |
| 5.000 /s | java | 3/3 | 5.000.1 ± 0.0 | 1 ± 0 | 5 ± 1.050 | 14.551 ± 8.364 | 0 | 187 |
| 5.000 /s | go | 3/3 | 5.000.1 ± 0.0 | 1 ± 0 | 4 ± 2 | 3.793 ± 2.657 | 0 | 154 |
| 10.000 /s | java | 3/3 | 10.000.0 ± 0.0 | 1 ± 0 | 3.077 ± 306 | 14.647 ± 400 | 0 | 1.005 |
| 10.000 /s | go | 3/3 | 10.000.1 ± 0.0 | 1 ± 0 | 3 ± 0 | 616 ± 2.510 | 0 | 334 |
| 20.000 /s | java | 1/3 | 20.000.0 ± 0.0 | 1 ± 0 | 564 ± 0 | 8.543 ± 0 | 385.566 | 364.567 |
| 20.000 /s | go | 3/3 | 20.000.1 ± 0.0 | 1 ± 0 | 41 ± 980 | 7.095 ± 26.262 | 0 | 1.912 |
| 30.000 /s | java | 0/3 | — | — | — | — | 595.939 | 570.324 |
| 30.000 /s | go | 2/3 | 30.000.4 ± 0.0 | 1 ± 0 | 14 ± 4 | 6.975 ± 856 | 0 | 2.697 |
| 40.000 /s | java | 0/3 | — | — | — | — | 799.933 | 772.060 |
| 40.000 /s | go | 1/3 | 40.000.1 ± 0.0 | 1 ± 0 | 29 ± 0 | 5.391 ± 0 | 0 | 2.365 |

### Taxa de chegada — recursos do gerador

| Taxa alvo | Prototipo | RSS repouso (MB) | RSS sob carga, pior rep (MB) | CPU (% de 1 nucleo) | Latencia corrigida p99 (us) | Latencia de servico p99 (us) | Delta corrigida-servico (us) |
|---|---|---|---|---|---|---|---|
| 1.000 /s | java | 147.5 ± 0.8 | 315.9 | 120.1 ± 0.8 | 5.235 ± 3.730 | 5.219 ± 2.748 | 16 ± 982 |
| 1.000 /s | go | 11.0 ± 0.0 | 18.9 | 106.3 ± 0.1 | 5.247 ± 194 | 5.215 ± 126 | 32 ± 68 |
| 5.000 /s | java | 138.3 ± 10.5 | 488.5 | 147.2 ± 3.2 | 5.479 ± 3.742 | 5.347 ± 386 | 132 ± 3.356 |
| 5.000 /s | go | 11.1 ± 0.2 | 24.0 | 130.4 ± 0.6 | 5.347 ± 68 | 5.287 ± 60 | 60 ± 8 |
| 10.000 /s | java | 148.5 ± 12.0 | 596.8 | 249.1 ± 3.9 | 13.127 ± 888 | 6.807 ± 2.370 | 6.232 ± 1.526 |
| 10.000 /s | go | 11.0 ± 0.2 | 30.1 | 160.8 ± 0.5 | 5.239 ± 18 | 5.195 ± 14 | 44 ± 4 |
| 20.000 /s | java | 147.5 ± 1.1 | 1.598.9 | 328.7 ± 0.0 | 593.919 ± 0 | 593.919 ± 0 | 0 ± 0 |
| 20.000 /s | go | 11.3 ± 0.2 | 90.5 | 239.3 ± 0.8 | 8.575 ± 23.804 | 7.799 ± 10.424 | 776 ± 13.380 |
| 30.000 /s | java | 147.5 ± 11.7 | 1.440.1 | — | — | — | — |
| 30.000 /s | go | 11.2 ± 0.2 | 3.055.8 | 315.1 ± 4.0 | 8.175 ± 1.280 | 7.349 ± 786 | 826 ± 494 |
| 40.000 /s | java | 148.6 ± 0.5 | 2.638.8 | — | — | — | — |
| 40.000 /s | go | 11.2 ± 0.1 | 3.207.3 | 414.4 ± 0.0 | 9.231 ± 0 | 7.619 ± 0 | 1.612 ± 0 |

### Taxa de chegada — colapso e erros

| Taxa alvo | Prototipo | Colapsos | Erros por classe |
|---|---|---|---|
| 1.000 /s | java | 0 | nenhum |
| 1.000 /s | go | 0 | nenhum |
| 5.000 /s | java | 0 | nenhum |
| 5.000 /s | go | 0 | nenhum |
| 10.000 /s | java | 0 | nenhum |
| 10.000 /s | go | 0 | nenhum |
| 20.000 /s | java | 0 | `IOException: Too many open files`: 686.054, `ConnectException: assign requested address`: 81.927, `HttpConnectTimeoutException: timed out`: 2.432, `HttpTimeoutException: timed out`: 82 |
| 20.000 /s | go | 0 | nenhum |
| 30.000 /s | java | 0 | `IOException: Too many open files`: 603.673, `ConnectException: assign requested address`: 39.078, `HttpConnectTimeoutException: timed out`: 4.698, `HttpTimeoutException: timed out`: 1.967 |
| 30.000 /s | go | 1 | nenhum |
| 40.000 /s | java | 0 | `IOException: Too many open files`: 940.492, `ConnectException: assign requested address`: 39.379, `HttpConnectTimeoutException: timed out`: 5.567, `HttpTimeoutException: timed out`: 4.711 |
| 40.000 /s | go | 2 | nenhum |

### Taxa de chegada — custo marginal de CPU

| Prototipo | Execucoes validas | Custo marginal (us de CPU/req) | Piso do agendador (nucleos) |
|---|---|---|---|
| go | 15 | 76.3 | 0.91 |
| java | 10 | 122.2 | 1.06 |

### Coletor de lixo (Java) — sustentacao e precisao do agendamento

| Taxa alvo | Prototipo | Repeticoes sustentadas | Taxa efetiva (reps validas) | Desvio p50 (us) | Desvio p99 (us) | Desvio max (us) | Erros (pior rep) | Pico em voo (pior rep) |
|---|---|---|---|---|---|---|---|---|
| 5.000 /s | java | 2/2 | 5.000.1 ± 0.0 | 1 ± 0 | 1.218 ± 1.214 | 9.752 ± 8.007 | 0 | 260 |
| 5.000 /s | java-zgc | 2/2 | 5.000.1 ± 0.0 | 1 ± 0 | 5 ± 1 | 3.158 ± 1.025 | 0 | 251 |
| 5.000 /s | java-parallel | 2/2 | 5.000.1 ± 0.0 | 1 ± 0 | 3.488 ± 3.484 | 38.072 ± 37.576 | 0 | 412 |
| 10.000 /s | java | 2/2 | 10.000.0 ± 0.0 | 1 ± 0 | 2.936 ± 181 | 14.627 ± 500 | 0 | 1.332 |
| 10.000 /s | java-zgc | 2/2 | 10.000.0 ± 0.1 | 1 ± 0 | 255 ± 237 | 12.701 ± 4.530 | 0 | 1.747 |
| 10.000 /s | java-parallel | 0/2 | — | — | — | — | 0 | 1.238 |

### Coletor de lixo (Java) — recursos do gerador

| Taxa alvo | Prototipo | RSS repouso (MB) | RSS sob carga, pior rep (MB) | CPU (% de 1 nucleo) | Latencia corrigida p99 (us) | Latencia de servico p99 (us) | Delta corrigida-servico (us) |
|---|---|---|---|---|---|---|---|
| 5.000 /s | java | 147.5 ± 0.2 | 479.0 | 152.2 ± 4.2 | 9.351 ± 4.152 | 5.641 ± 462 | 3.710 ± 3.690 |
| 5.000 /s | java-zgc | 127.4 ± 0.1 | 1.238.6 | 153.3 ± 1.8 | 5.519 ± 288 | 5.467 ± 252 | 52 ± 36 |
| 5.000 /s | java-parallel | 136.6 ± 10.4 | 967.3 | 156.4 ± 7.4 | 11.751 ± 6.584 | 5.943 ± 792 | 5.808 ± 5.792 |
| 10.000 /s | java | 138.2 ± 12.2 | 587.8 | 258.2 ± 0.4 | 13.103 ± 328 | 7.115 ± 280 | 5.988 ± 48 |
| 10.000 /s | java-zgc | 127.4 ± 0.1 | 2.003.8 | 262.6 ± 5.3 | 14.421 ± 6.234 | 12.909 ± 4.978 | 1.512 ± 1.256 |
| 10.000 /s | java-parallel | 148.1 ± 0.2 | 1.993.2 | — | — | — | — |

### Coletor de lixo (Java) — colapso e erros

| Taxa alvo | Prototipo | Colapsos | Erros por classe |
|---|---|---|---|
| 5.000 /s | java | 0 | nenhum |
| 5.000 /s | java-zgc | 0 | nenhum |
| 5.000 /s | java-parallel | 0 | nenhum |
| 10.000 /s | java | 0 | nenhum |
| 10.000 /s | java-zgc | 0 | nenhum |
| 10.000 /s | java-parallel | 0 | nenhum |

### Coletor de lixo (Java) — custo marginal de CPU

| Prototipo | Execucoes validas | Custo marginal (us de CPU/req) | Piso do agendador (nucleos) |
|---|---|---|---|
| java | 4 | 212.2 | 0.46 |
| java-parallel | 2 | 0.0 | 1.56 |
| java-zgc | 4 | 218.6 | 0.44 |

### Concorrencia — alvo com 1 s de latencia

| Taxa alvo | Prototipo | Reps sustentadas | Pico em voo | Taxa efetiva | Desvio p99 (us) | RSS sob carga (MB) | Erros |
|---|---|---|---|---|---|---|---|
| 1.000 /s | java | 2/2 | 1.014 ± 2 | 1.000.1 ± 0.0 | 4 ± 0 | 364.6 ± 10.0 | 0 |
| 1.000 /s | go | 2/2 | 1.009 ± 6 | 1.000.1 ± 0.0 | 2 ± 0 | 65.0 ± 0.2 | 0 |
| 5.000 /s | java | 2/2 | 5.250 ± 46 | 5.000.1 ± 0.0 | 291 ± 270 | 741.0 ± 10.0 | 0 |
| 5.000 /s | go | 2/2 | 5.100 ± 10 | 5.000.1 ± 0.0 | 2 ± 0 | 259.5 ± 0.3 | 0 |
| 10.000 /s | java | 0/2 | — | — | — | 1.039.8 ± 248.9 | 189.728 |
| 10.000 /s | go | 1/2 | 10.121 ± 0 | 10.000.1 ± 0.0 | 3 ± 0 | 1.171.5 ± 674.3 | 183.055 |
| 20.000 /s | java | 0/2 | — | — | — | 1.264.4 ± 42.4 | 390.183 |
| 20.000 /s | go | 0/2 | — | — | — | 1.991.8 ± 0.9 | 0 |
| 50.000 /s | java | 0/2 | — | — | — | 1.676.5 ± 378.0 | 999.868 |
| 50.000 /s | go | 0/2 | — | — | — | 2.833.9 ± 309.0 | 0 |


## Interpretacao

### 1. Precisao do agendamento e o numero que decide

A 10.000/s o prototipo Java erra o instante de despacho em **3.077 us no p99**; o Go erra em **3 us**. Medindo um alvo de 5 ms, o erro do instrumento Java fica na mesma ordem de grandeza do fenomeno medido. Para uma ferramenta cuja tese e "latencia contada do instante agendado", isso e desqualificante.

A bateria de GC mostra que a causa e pausa de coletor, nao a linguagem:

| Configuracao | Desvio p99 a 5.000/s | Desvio p99 a 10.000/s |
|---|---|---|
| Java, G1 (padrao) | 1.218 us | 2.936 us |
| Java, ZGC | **5 us** | **255 us** |
| Java, ParallelGC | 3.488 us | nao sustentou |
| Go | 4 us | 3 us |

ZGC resolve quase tudo — e cobra RSS: 2.003 MB sob carga a 10.000/s contra 587 MB no G1 e 30 MB no Go. A leitura honesta e que **Java com ZGC seria viavel**, e que a vantagem do Go em agendamento cai de ~1.000x para ~85x. A decisao do [ADR 0001](adr/0001-linguagem-e-runtime.md) nao se apoia so nesse eixo.

### 2. Modo de falha, que e onde a diferenca e maior

Quando o gerador Java nao acompanha, ele nao degrada — entra em espiral. `java.net.http.HttpClient` em HTTP/1.1 usa uma conexao por requisicao concorrente; requisicao atrasada segura conexao; conexao presa aumenta o atraso. A 20.000/s isso levou a **364.567 requisicoes em voo**, 686.054 `IOException: Too many open files` e RSS de 1,6 GB, com a execucao inteira invalidada. O mesmo ponto no Go: 1.912 em voo, zero erro.

O teto de descritor de arquivo desta maquina (`kern.maxfilesperproc = 61440`) e compartilhado com o processo alvo, o que torna esse limite parte do ambiente, nao do runtime — mas quem chega la primeiro, e por muito, e o Java.

### 3. Colapso do Go em taxa alta — o que sabemos e o que nao sabemos

Nos pontos de 30.000/s e 40.000/s houve execucoes do Go que **nao produziram saida dentro do teto de 200 s** e foram contadas como nao sustentadas. Nao ha erro registrado nelas, entao o rastro e pobre. O que a investigacao mostrou:

- Rodando com a maquina limpa e com heartbeat ligado, o Go a 40.000/s completou 800.001 requisicoes com **zero erro**, ~205 requisicoes em voo e o processo alvo consumindo **2,1 nucleos**. Saida real em `medicoes/diagnostico.sh`.
- Parte dos colapsos e contaminacao entre execucoes: a bateria roda Java antes de Go no mesmo ponto, e o colapso do Java deixa centenas de milhares de sockets em `TIME_WAIT` (MSL de 15 s nesta maquina), estreitando as portas efemeras da execucao seguinte.
- Repetindo os pontos altos do Go com a maquina limpa e esperando `TIME_WAIT` drenar: 30.000/s sustentou 2/2; 40.000/s sustentou 1/2 — uma execucao ainda travou sem saida.

**Nao identificamos a causa dessa travada.** Fica declarada assim: o Go sustenta **30.000/s** de forma reproduzivel (4 de 5 execucoes somando as duas baterias) e **40.000/s de forma nao confiavel** (2 de 5). Acima de ~30.000/s, com o alvo local ja consumindo 2,1 nucleos dos 10, o que se mede e o par gerador+alvo, nao o gerador.

### 4. Concorrencia

Com alvo de 1 s de latencia, o pico de requisicoes simultaneas sustentado foi de **5.250 no Java** e **10.121 no Go**. Acima disso os dois batem no teto de descritores da maquina. O modelo de concorrencia dos dois runtimes da conta; quem limita e o sistema operacional — com a diferenca de que o Java chega la com 741 MB de RSS e o Go com 259 MB no mesmo ponto de 5.000 usuarios.

### 5. Custo de CPU

O piso do agendador e de aproximadamente **1 nucleo** nos dois prototipos: e a espera ativa que sustenta o desvio abaixo de 100 us. Esse custo e constante, nao por requisicao. O custo **marginal** por requisicao, obtido por regressao sobre as execucoes validas, ficou em **76 us de CPU no Go** e **122 us no Java** (G1).

### 6. Omissao coordenada medida no proprio experimento

A coluna `Delta corrigida-servico` e a omissao coordenada em numero: e quanto uma ferramenta de laco fechado deixaria de reportar no p99. A 10.000/s no Java com G1 o delta e de **5.988 us** — a latencia real e mais que o dobro da que seria reportada contando do envio. No Go, no mesmo ponto, o delta e de **44 us**, porque o gerador nao atrasa o despacho.

Isso e uma demonstracao lateral do que o braunrate precisa fazer de proposito: **um gerador que atrasa e um gerador que mente, a menos que conte a partir do instante agendado.**
