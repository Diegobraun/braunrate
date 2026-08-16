# braunrate

Ferramenta de teste de carga com medicao honesta: modelo de chegada aberto, HDR
histogram e deteccao de back-pressure. Binario unico, sem runtime para instalar,
cenario em YAML que vive no repositorio ao lado do servico.

**Para quem nunca fez teste de carga.** Um comando, um terminal, nenhum arquivo
antes:

```bash
braunrate demo
```

Ele sobe um servico de mentira, roda um cenario contra ele e explica cada numero
enquanto eles aparecem. Depois, [Primeiros 15 minutos](primeiros-15-minutos.html)
leva do zero ate o primeiro relatorio do seu proprio servico.

## Tres provas, tres pontos cegos

Nada aqui e alegacao: sao tres execucoes reais, cada uma expondo um ponto cego
diferente das ferramentas que existem.

| Prova | Numero | Ponto cego que expoe |
|---|---|---|
| Alvo congelado por 1 s | 976,4 ms contra 3,3 ms | **Omissao coordenada**: laco fechado para de enviar quando o alvo trava, e a espera some da conta |
| [GraphQL com erro em 200](protocolos.html#graphql) | 406 erros em 2.844 respostas, todas com status 200 | **Erro classificado por status**: quem le o codigo HTTP reporta 0% de erro e criterio de aceite verde |
| [Cadeia assincrona](protocolos.html#kafka-e-rabbitmq) | 1,2 ms para produzir contra 3,96 s de jornada | **Medir so a producao**: o broker aceita rapido, e o efeito que o usuario espera chega segundos depois |

### A primeira, em detalhe

O alvo de teste embutido congela por 1 s no meio da execucao. Mesma pausa, mesmo
alvo, dois modelos de medicao:

| Modelo | 99% das respostas em ate | Amostras |
|---|---|---|
| **braunrate (chegada aberta, tempo contado do instante agendado)** | **976,4 ms** | 600 |
| Laco fechado (um usuario virtual em sequencia, como JMeter e Locust medem) | 3,3 ms | 793 |

**973,1 ms escondidos pelo laco fechado.** O laco fechado nao mente por bug:
quando o alvo trava, ele simplesmente para de enviar, e as requisicoes que
deveriam ter partido nunca entram na conta. E a omissao coordenada.

Isso e um teste automatizado que roda no CI a cada push. Se a medicao mentir, o
build quebra:

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 976.4 ms sobre 600 amostras
      laco fechado:              p99 3.3 ms sobre 793 amostras
      omissao coordenada: 973.1 ms escondidos pelo laco fechado
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

E da para ver a mesma coisa acontecer na sua maquina, sem clonar nada:

```bash
braunrate demo --com-falha
```

## Para quem e

**QA que nao programa.** O caso comum e um arquivo YAML de dez linhas com valor
padrao para tudo o que nao foi declarado. Sempre existe um caminho de entrada —
importar um `curl`, gravar navegando, partir de um exemplo — porque folha em
branco e o motivo pelo qual ninguem monta cenario do zero.

**Time que versiona o teste junto do servico.** O cenario e um arquivo de texto
que da diff legivel e merge possivel, o mesmo motor roda no terminal e no CI, e
o criterio de aceite vira codigo de saida sem cola no meio.

## Por que existe

1. **Medicao honesta por padrao.** Modelo de chegada aberto; tempo de resposta
   contado a partir do instante em que a requisicao *deveria* ter partido; HDR
   histogram; aviso explicito quando o gerador nao sustentou a taxa. A omissao
   coordenada e a falha que faz teste passar com 99% em ate 47 ms enquanto
   producao sofre 1,8 s.
2. **Dois publicos, um motor.** YAML declarativo para o caso comum, DSL em Go
   para o complexo — mesmo motor, mesmas metricas, sem reescrita ao migrar.
3. **Cenario de negocio, nao so requisicao.** GraphQL medido por operacao; Kafka
   e RabbitMQ com modelo de metrica proprio; passo `aguardar` para medir a
   cadeia assincrona ponta a ponta.

## Escopo

**Dentro:** HTTP/HTTPS e REST; GraphQL de primeira classe; Kafka e RabbitMQ
(produzir e consumir); passo `aguardar` com timeout; correlacao, variaveis e
fluxo de autenticacao; CSV com politica de consumo e geracao sintetica com
semente; perfis de carga e modelo fechado declarado; criterio de aceite com
codigo de saida; relatorio HTML autocontido, JSON, CSV e resumo de terminal;
comparacao entre execucoes; importador de `.jmx` para o subconjunto comum;
gravador de trafego HTTP; modo servidor local sem logica propria; autenticacao
de broker, sempre com a credencial fora do arquivo.

**Fora:** motor de browser real; nuvem gerenciada, dashboard multiusuario, conta
de time; agendamento e persistencia alem dos arquivos; LDAP, FTP, SMTP, JMS
classico; competir em taxa bruta com wrk; execucao distribuida na v1.

**Limitacoes conhecidas**, com o motivo em [Decisoes](decisoes.html): protocolo
fora da lista exige mudanca neste repositorio; um unico token para a execucao
inteira; e o tempo dos passos seguintes ao primeiro e tempo de servico, nao
tempo corrigido — a leitura honesta da jornada esta no bloco "A jornada
inteira".
