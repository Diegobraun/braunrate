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
